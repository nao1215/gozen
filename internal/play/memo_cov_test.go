package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/memo"
)

// sysMemoFiles are the notes written under dir, sorted by name.
func sysMemoFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func sysNovel(t *testing.T, opts Options) *Game {
	t.Helper()
	return sysOpen(t, opts, map[string]string{"data/novel.yaml": novelData})
}

// TestMemoKeyWritesANoteInTheMiddleOfAnEvent: the memo key stops the game, a kind and a word
// are chosen, and a note is written with the line on screen, the event and the save it can
// resume from (ADR 0025's resume by replaying the event).
func TestMemoKeyWritesANoteInTheMiddleOfAnEvent(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{
		Seed: 5, MemoDir: dir, Location: "camp", Event: "story", Line: 2,
		Script: "wait 5\npress memo\nwait 10\npress z\nwait 3\ntype  needs a lamp \npress z\nwait 5\nquit",
	})
	if !sysFrames(t, g, 600) {
		t.Fatal("the script did not end")
	}
	files := sysMemoFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("notes written: %v", files)
	}
	m, save, err := memo.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if m.Text != "needs a lamp" || m.Kind != "note" || m.Source != memo.Human {
		t.Fatalf("note %q kind %q source %q", m.Text, m.Kind, m.Source)
	}
	if m.Event != "story" || m.Location != "camp" || m.Line != "Gold 100." || m.Who != "Courier" {
		t.Fatalf("where: event %q location %q who %q line %q", m.Event, m.Location, m.Who, m.Line)
	}
	if m.Resume == nil || m.Resume.From != "event" || m.Resume.Event != "story" || m.Resume.Line != 2 {
		t.Fatalf("resume %+v", m.Resume)
	}
	if len(save) == 0 {
		t.Fatal("no save kept with the note")
	}
	if m.Seed != 5 || m.Game != "Camp expedition" {
		t.Fatalf("seed %d game %q", m.Seed, m.Game)
	}
}

// TestMemoCancelWritesNothing: Escape (cancel) on the kind list or on the text closes the box
// and writes no file.
func TestMemoCancelWritesNothing(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{
		MemoDir: dir, Location: "camp", Event: "story", Line: 1,
		Script: "wait 5\npress memo\nwait 10\npress x\nwait 5\npress memo\nwait 10\npress z\nwait 4\ntype nope\npress x\nwait 5\nquit",
	})
	if !sysFrames(t, g, 600) {
		t.Fatal("the script did not end")
	}
	if g.memo != nil {
		t.Fatal("the box is still open")
	}
	if files := sysMemoFiles(t, dir); len(files) != 0 {
		t.Fatalf("a cancelled note was written: %v", files)
	}
}

// TestMemoNeedsADirectory: without --memos the key does nothing.
func TestMemoNeedsADirectory(t *testing.T) {
	g := sysNovel(t, Options{Location: "camp", Event: "story", Line: 1, Script: "wait 5\npress memo\nwait 10\nquit"})
	if !sysFrames(t, g, 200) {
		t.Fatal("the script did not end")
	}
	if g.memo != nil {
		t.Fatal("the memo box opened without a directory")
	}
}

// TestMemoWriteFailureShowsAToast: a note that cannot be written is reported on screen and
// the game goes on.
func TestMemoWriteFailureShowsAToast(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := sysNovel(t, Options{
		MemoDir: filepath.Join(blocked, "notes"), Location: "camp", Event: "story", Line: 1,
		Script: "wait 5\npress memo\nwait 10\npress z\nwait 4\ntype x\npress z\nwait 3\nquit",
	})
	if !sysFrames(t, g, 400) {
		t.Fatal("the script did not end")
	}
	if g.memoToast == "" || g.memoToastT <= 0 {
		t.Fatalf("no toast for a failed write: %q", g.memoToast)
	}
}

// TestQuickMemoFromAScriptAndTheLimit: `memo KIND TEXT` in a script keeps a note as an LLM
// note; the bug kind is labelled; at most 40 are kept in one run.
func TestQuickMemoFromAScriptAndTheLimit(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{
		MemoDir: dir, Location: "camp", Event: "story", Line: 1,
		Script: "wait 3\nmemo bug the lamp is wrong\nwait 3\nquit",
	})
	if !sysFrames(t, g, 200) {
		t.Fatal("the script did not end")
	}
	files := sysMemoFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("notes %v", files)
	}
	m, _, err := memo.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != memo.LLM || m.Kind != "bug" || m.Text != "the lamp is wrong" {
		t.Fatalf("note %+v", m)
	}
	for range 60 {
		g.quickMemo("note", "x", memo.Bot)
	}
	if g.quickN != 40 {
		t.Fatalf("%d quick notes kept, want the limit 40", g.quickN)
	}
	g.opts.MemoDir = ""
	n := g.quickN
	g.quickMemo("note", "x", memo.Bot)
	if g.quickN != n {
		t.Fatal("a note was kept without a directory")
	}
}

// TestFromMemoReturnsToTheNote: starting from a note taken in the middle of an event reads the
// event on to the same line; one taken outside an event restores the state as it was.
func TestFromMemoReturnsToTheNote(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{
		Seed: 9, MemoDir: dir, Location: "camp", Event: "story", Line: 3,
		Script: "wait 5\npress memo\nwait 10\npress z\nwait 3\ntype third line\npress z\nwait 5\nquit",
	})
	if !sysFrames(t, g, 600) {
		t.Fatal("the script did not end")
	}
	files := sysMemoFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("notes %v", files)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	m, _, err := memo.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if m.Resume == nil || m.Resume.Line != 3 || m.Line != "A bell rings far off." {
		t.Fatalf("note: %+v line %q", m.Resume, m.Line)
	}
	g2 := sysNovel(t, Options{Seed: 9, FromMemo: files[0]})
	if g2.fast == nil {
		t.Fatal("not reading on to the note's line")
	}
	for range 3000 {
		if g2.fast == nil {
			break
		}
		if err := g2.Update(); err != nil {
			t.Fatal(err)
		}
		sysSafeDraw(g2)
	}
	if g2.fast != nil {
		t.Fatal("never reached the line")
	}
	if !g2.talk.active || g2.talk.say.Text != "A bell rings far off." || g2.evLines != 3 {
		t.Fatalf("stopped at %q (line %d)", g2.talk.say.Text, g2.evLines)
	}
}

// TestFromMemoOutsideAnEventAndBrokenFiles: a note taken on the map comes back as it was; a
// file that is not a note, and a note whose save is damaged, are refused with the path.
func TestFromMemoOutsideAnEventAndBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen(t, Options{MemoDir: dir, Location: "camp", Gold: 321}, nil)
	g.quickMemo("note", "on the map", memo.Human)
	sysFrames(t, g, 3)
	files := sysMemoFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("notes %v", files)
	}
	m, _, err := memo.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if m.Resume == nil || m.Resume.From != "here" {
		t.Fatalf("resume %+v", m.Resume)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysOpen(t, Options{FromMemo: files[0]}, nil)
	if g2.st.Gold != 321 || g2.fast != nil {
		t.Fatalf("restored gold %d fast %v", g2.st.Gold, g2.fast)
	}
	if err := g2.Close(); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: t.TempDir(), FromMemo: bad}); err == nil {
		t.Fatal("accepted a file that is not a note")
	}
}

// TestFromMemoWithAVanishedEventStartsWhereItBegan: the data changed so the event is gone;
// the game still opens (the note's save is restored).
func TestFromMemoWithAVanishedEventStartsWhereItBegan(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{MemoDir: dir, Location: "camp", Event: "story", Line: 1})
	for range 400 {
		if g.fast == nil {
			break
		}
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	g.quickMemo("note", "x", memo.Human)
	sysFrames(t, g, 2)
	files := sysMemoFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("notes %v", files)
	}
	m, save, err := memo.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	m.Resume.Event = "gone"
	out := filepath.Join(t.TempDir(), "n")
	if _, err := memo.WriteAt(out, &m, nil, save); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysNovel(t, Options{FromMemo: out + ".json"})
	if g2.st == nil || g2.events.Running() {
		t.Fatal("the vanished event was started")
	}
	if !strings.Contains(g2.opts.FromMemo, "n.json") {
		t.Fatal("option lost")
	}
}
