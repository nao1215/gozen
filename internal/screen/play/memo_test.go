package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/base/memo"
	data "github.com/nao1215/gozen/internal/model/gamedata"
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
	for range 40 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if g.in.script.pc != 1 {
		t.Fatal("disabled memo key was acknowledged")
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

const sysMemoYAML = "memo: {kinds: [{id: bug, label: Bug}, {id: idea, label: Idea}], vars: [{name: coins, value: \"gold\"}]}\n"

// sysQuickNote keeps a note through the quick path and returns it as written.
func sysQuickNote(t *testing.T, g *Game, kind string) (memo.Memo, []byte) {
	t.Helper()
	before := len(sysMemoFiles(t, g.opts.MemoDir))
	g.quickMemo(kind, "a note", memo.Human)
	sysFrames(t, g, 2)
	files := sysMemoFiles(t, g.opts.MemoDir)
	if len(files) != before+1 {
		t.Fatalf("%d notes, want %d", len(files), before+1)
	}
	m, save, err := memo.Read(files[len(files)-1])
	if err != nil {
		t.Fatal(err)
	}
	return m, save
}

// TestMemoKeepsTheDungeonPosition: a note taken in a dungeon records the floor map, where the
// player stands and faces, what has been seen and the numbers the game asked to keep.
func TestMemoKeepsTheDungeonPosition(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen4(t, Options{Seed: 3, Location: "hall", MemoDir: dir}, map[string]string{
		"data/hall.yaml": sysCorridor, "game.yaml+": sysMemoYAML,
	})
	m, _ := sysQuickNote(t, g, "idea")
	if m.Kind != "idea" || m.Label != "Idea" {
		t.Fatalf("kind %q label %q", m.Kind, m.Label)
	}
	if len(m.Pos) != 2 || m.Pos[0] != 4 || m.Pos[1] != 1 || m.Facing != "east" {
		t.Fatalf("position %v facing %q", m.Pos, m.Facing)
	}
	if len(m.Map) != 3 || m.Map[1] != "#E..S..F#" || len(m.Seen) != 3 || len(m.Seen[1]) != len(m.Map[1]) {
		t.Fatalf("map %v seen %v", m.Map, m.Seen)
	}
	if m.Seen[1][4] != '1' {
		t.Fatalf("the cell stood on is not seen: %q", m.Seen[1])
	}
	if m.Place != "Corridor" || m.Location != "hall" {
		t.Fatalf("place %q at %q", m.Place, m.Location)
	}
	if m.Vars["coins"] != 100.0 {
		t.Fatalf("vars %v", m.Vars)
	}
	if len(m.Party) != 1 || m.Party[0] == "" {
		t.Fatalf("party %v", m.Party)
	}
	// the box shows the floor and position when drawn.
	g.openMemo()
	if len(g.memo.kinds) != 2 {
		t.Fatalf("kinds %v", g.memo.kinds)
	}
	x0, _ := g.memoCaret()
	g.memo.text = "hello"
	if x1, _ := g.memoCaret(); x1 <= x0 {
		t.Fatal("the caret did not follow the text")
	}
	g.memo.text = ""
	sysSafeDraw(g)
	g.closeMemo()
}

// TestMemoAtAChoiceKeepsTheOptions: a note taken at a choice lists the options and the
// resume cursor counts the answers given.
func TestMemoAtAChoiceKeepsTheOptions(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{Seed: 3, Location: "camp", Event: "story", Line: 4, MemoDir: dir})
	for range 400 {
		if g.fast == nil {
			break
		}
		sysFrames(t, g, 1)
	}
	if g.talk.choice == nil {
		t.Fatal("the choice is not open")
	}
	m, _ := sysQuickNote(t, g, "note")
	if len(m.Choices) != 2 || m.Choices[0] != "Wait" || m.Choices[1] != "Go" {
		t.Fatalf("choices %v", m.Choices)
	}
	if m.Resume == nil || m.Resume.From != "event" || m.Resume.Line != 4 {
		t.Fatalf("resume %+v", m.Resume)
	}
}

// TestMemoAfterAnEventKeepsTheLastLine: a note taken when no line is open lists the last line
// of the backlog.
func TestMemoAfterAnEventKeepsTheLastLine(t *testing.T) {
	dir := t.TempDir()
	g := sysNovel(t, Options{Seed: 3, Location: "camp", MemoDir: dir})
	g.backlog = append(g.backlog, logLine{name: "Courier", text: "Last said"})
	m, _ := sysQuickNote(t, g, "note")
	if m.Who != "Courier" || m.Line != "Last said" {
		t.Fatalf("who %q line %q", m.Who, m.Line)
	}
	if m.Resume == nil || m.Resume.From != "here" {
		t.Fatalf("resume %+v", m.Resume)
	}
}

// TestMemoInABattleRestartsTheBattle: a note taken in a battle resumes by starting the same
// battle.
func TestMemoInABattleRestartsTheBattle(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen4(t, Options{Seed: 3, Location: "camp", Battle: "one", MemoDir: dir}, nil)
	sysFrames(t, g, 3)
	m, _ := sysQuickNote(t, g, "note")
	if m.Resume == nil || m.Resume.From != "battle" || m.Resume.Battle != "one" {
		t.Fatalf("resume %+v", m.Resume)
	}
	files := sysMemoFiles(t, dir)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysOpen4(t, Options{FromMemo: files[0]}, nil)
	if g2.currentBattle() == nil || g2.currentBattle().troop != "one" {
		t.Fatal("the battle did not start again")
	}
}

// TestMemoWithoutANamedEventResumesFromTheArrival: the arrival event of a place has no name, the
// note says "arrival" and starting from it reads on to the same line.
func TestMemoWithoutANamedEventResumesFromTheArrival(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen4(t, Options{Seed: 3, Location: "camp", MemoDir: dir}, map[string]string{"data/enter.yaml": "events:\n  hello:\n    trigger: enter\n    location: room\n    do:\n      - say: \"Welcome to the room.\"\n      - say: \"Second.\"\n"})
	g.Move("room")
	for range 200 {
		sysFrames(t, g, 1)
		if g.talk.active {
			break
		}
	}
	if !g.talk.active {
		t.Fatal("the arrival event did not open")
	}
	g.evName = ""
	m, _ := sysQuickNote(t, g, "note")
	if m.Resume == nil || m.Resume.From != "arrival" || m.Resume.Line != 1 {
		t.Fatalf("resume %+v", m.Resume)
	}
	files := sysMemoFiles(t, dir)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysOpen4(t, Options{FromMemo: files[0]}, map[string]string{"data/enter.yaml": "events:\n  hello:\n    trigger: enter\n    location: room\n    do:\n      - say: \"Welcome to the room.\"\n      - say: \"Second.\"\n"})
	if g2.fast == nil {
		t.Fatal("not reading on")
	}
}

// TestFromMemoBrokenSave: a note whose save is damaged is refused with the file's path, and
// one without a resume record restores as it was.
func TestFromMemoBrokenSave(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen4(t, Options{Seed: 3, Location: "camp", MemoDir: dir}, nil)
	m, _ := sysQuickNote(t, g, "note")
	files := sysMemoFiles(t, dir)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "broken")
	if _, err := memo.WriteAt(out, &m, nil, []byte("{not a save")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{FS: gameFiles(t), NoSound: true, SaveDir: t.TempDir(), FromMemo: out + ".json"}); err == nil {
		t.Fatal("accepted a damaged save")
	}
	// without resume and without rand the note starts from here with a fresh random source.
	_, save, err := memo.Read(files[0])
	if err != nil {
		t.Fatal(err)
	}
	m.Resume, m.Rand = nil, nil
	plain := filepath.Join(t.TempDir(), "plain")
	if _, err := memo.WriteAt(plain, &m, nil, save); err != nil {
		t.Fatal(err)
	}
	g2 := sysOpen4(t, Options{FromMemo: plain + ".json"}, nil)
	if g2.st == nil || g2.st.Location != "camp" || g2.fast != nil {
		t.Fatalf("state %+v fast %v", g2.st, g2.fast)
	}
}

// TestMemoBuildInfoAndDefaultDirectory: with build info the note names the trace; the default
// directory is used when none is given.
func TestMemoBuildInfoAndDefaultDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	g := sysOpen4(t, Options{Seed: 3, Location: "camp", MemoDir: "x", CollectBuildInfo: true, TracePath: "/tmp/run.jsonl"}, nil)
	var info memo.Memo
	g.memoResume(&info)
	if info.Trace != "run.jsonl" {
		t.Fatalf("trace %q", info.Trace)
	}
	g.opts.MemoDir = ""
	g.openMemo()
	g.memo.typed = true
	g.memo.text = "kept in notes"
	g.saveMemo()
	files, err := filepath.Glob(filepath.Join(DefaultMemoDir, "*", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("notes in the default directory: %v %v", files, err)
	}
}

// TestMemoReadInputPrefersTheComposer: text typed through the input method wins over plain
// characters in the same frame; plain characters are appended; nothing typed is not handled.
func TestMemoReadInputPrefersTheComposer(t *testing.T) {
	m := &memoBox{}
	handled, err := m.readInput(func() (bool, error) { return false, nil }, func([]rune) []rune { return nil })
	if handled || err != nil {
		t.Fatalf("nothing typed: %v %v", handled, err)
	}
	handled = sysReadInput(t, m, func() (bool, error) { return false, nil }, func([]rune) []rune { return []rune("ab") })
	if !handled || m.text != "ab" {
		t.Fatalf("plain characters: %v %q", handled, m.text)
	}
	handled = sysReadInput(t, m, func() (bool, error) { m.committed = true; return false, nil }, func([]rune) []rune { return []rune("zz") })
	if !handled || m.text != "ab" {
		t.Fatalf("committed frame: %v %q", handled, m.text)
	}
	m.comp = "kana"
	handled = sysReadInput(t, m, func() (bool, error) { return false, nil }, func([]rune) []rune { return []rune("zz") })
	if !handled || m.text != "ab" {
		t.Fatalf("composing frame: %v %q", handled, m.text)
	}
	m.comp = ""
	handled = sysReadInput(t, m, func() (bool, error) { return true, nil }, func([]rune) []rune { return nil })
	if !handled {
		t.Fatal("the composer's own handling is lost")
	}
}

// TestMemoUnfocusedRunsUntilTheNoteIsDone: the game runs while its window is not focused until the
// note is written and its toast has gone.
func TestMemoUnfocusedRunsUntilTheNoteIsDone(t *testing.T) {
	g := sysOpen4(t, Options{Location: "camp"}, nil)
	was := g.memoUnfocused
	orig := ebiten.IsRunnableOnUnfocused()
	t.Cleanup(func() { ebiten.SetRunnableOnUnfocused(orig) })
	ebiten.SetRunnableOnUnfocused(false)
	g.keepRunningUnfocused()
	if !g.memoUnfocused {
		t.Fatal("the game was not set to run unfocused")
	}
	g.memoToastT = 5
	g.stepMemoFocus()
	if !g.memoUnfocused {
		t.Fatal("the focus setting was dropped while the toast shows")
	}
	g.memoToastT = 0
	g.stepMemoFocus()
	if g.memoUnfocused != was {
		t.Fatal("the focus setting was kept after the note")
	}
}

// TestShownPicture: the file of a CG's page, the last page for a page past the end, the file
// otherwise.
func TestShownPicture(t *testing.T) {
	for _, c := range []struct {
		cg   data.CG
		want string
	}{
		{data.CG{File: "one.png"}, "one.png"},
		{data.CG{Files: []string{"a.png", "b.png"}}, "a.png"},
		{data.CG{Files: []string{"a.png", "b.png"}, Page: 2}, "b.png"},
		{data.CG{Files: []string{"a.png", "b.png"}, Page: 9}, "b.png"},
	} {
		if got := shownPicture(c.cg); got != c.want {
			t.Errorf("%+v: %q, want %q", c.cg, got, c.want)
		}
	}
}

func sysOpenMemoBox(t *testing.T) *Game {
	t.Helper()
	g := sysOpen4(t, Options{Seed: 1, Location: "hall", MemoDir: t.TempDir()}, map[string]string{"data/hall.yaml": sysCorridor, "game.yaml+": sysMemoYAML})
	g.openMemo()
	g.memo.age = 10
	return g
}

// TestMemoBoxDrawsEveryStateOfTheInput: the box is drawn while choosing the kind, while typing, with
// text in composition, with long text, and while the note is typed in another window.
func TestMemoBoxDrawsEveryStateOfTheInput(t *testing.T) {
	covSlow(t)
	g := sysOpenMemoBox(t)
	sysSafeDraw(g)
	g.memo.typed = true
	g.memo.text = "plain"
	sysSafeDraw(g)
	g.memo.comp = "かな"
	sysSafeDraw(g)
	g.memo.text = ""
	for range 80 {
		g.memo.text += "long note "
	}
	g.memo.comp = ""
	sysSafeDraw(g)
	g.memo.ext = make(chan extInput, 1)
	sysSafeDraw(g)
	if g.memo == nil {
		t.Fatal("the box vanished while drawing")
	}
}

// TestMemoTypedInAnotherWindow: a note typed in another window is saved when it returns, dropped
// when it is cancelled, and waited for while it has not come back.
func TestMemoTypedInAnotherWindow(t *testing.T) {
	g := sysOpenMemoBox(t)
	g.memo.typed = true
	g.memo.ext = make(chan extInput, 1)
	g.updateMemo()
	if g.memo == nil || g.memo.ext == nil {
		t.Fatal("the box closed before the other window answered")
	}
	g.memo.ext <- extInput{text: "from outside", ok: true}
	g.updateMemo()
	if g.memo != nil {
		t.Fatal("the box stayed open after the note came back")
	}
	files := sysMemoFiles(t, g.opts.MemoDir)
	if len(files) != 1 {
		t.Fatalf("notes %v", files)
	}
	m, _, err := memo.Read(files[0])
	if err != nil || m.Text != "from outside" {
		t.Fatalf("note %+v (%v)", m, err)
	}

	g.openMemo()
	g.memo.age = 10
	g.memo.typed = true
	g.memo.ext = make(chan extInput, 1)
	g.memo.ext <- extInput{ok: false}
	g.updateMemo()
	if g.memo != nil || len(sysMemoFiles(t, g.opts.MemoDir)) != 1 {
		t.Fatal("a cancelled note was kept")
	}
}

// TestMemoKindChoiceWaitsFourTicks: the first ticks of the box ignore input so the key that opened
// it does not choose a kind.
func TestMemoKindChoiceWaitsFourTicks(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "hall", MemoDir: t.TempDir()}, map[string]string{"data/hall.yaml": sysCorridor})
	g.openMemo()
	g.in.script = nil
	sysKey(g, btnOK)
	g.updateMemo()
	if g.memo.typed {
		t.Fatal("a kind was chosen on the first tick")
	}
	g.memo.age = 10
	sysKey(g, btnOK)
	g.updateMemo()
	if !g.memo.typed {
		t.Fatal("ok did not choose the kind")
	}
	g.closeMemo()
	if g.memo != nil {
		t.Fatal("closeMemo left the box open")
	}
}

// TestTypeMemoGoesToTheBoxThatAsksForIt: text typed by a script goes to the memo box once a kind is
// chosen, and nowhere before.
func TestTypeMemoGoesToTheBoxThatAsksForIt(t *testing.T) {
	g := sysOpenMemoBox(t)
	g.typeMemo("early")
	if g.memo.text != "" {
		t.Fatal("text typed before a kind was chosen")
	}
	g.memo.typed = true
	g.typeMemo("late")
	if g.memo.text != "late" {
		t.Fatalf("text %q", g.memo.text)
	}
	g.closeMemo()
	g.typeMemo("nobody listens")
}

// TestMemoLeavesOutVarsThatAreNotValues: a memo.vars entry whose expression is the bare name of a
// function (floor instead of var.floor) is not a value. The note is still written, with the
// other vars, instead of failing to encode.
func TestMemoLeavesOutVarsThatAreNotValues(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen4(t, Options{Seed: 3, Location: "hall", MemoDir: dir}, map[string]string{
		"data/hall.yaml": sysCorridor,
		"game.yaml+":     "memo: {kinds: [{id: bug, label: Bug}], vars: [{name: coins, value: \"gold\"}, {name: depth, value: \"floor\"}]}\n",
	})
	m, _ := sysQuickNote(t, g, "bug")
	if m.Vars["coins"] != 100.0 {
		t.Fatalf("vars %v", m.Vars)
	}
	if _, ok := m.Vars["depth"]; ok {
		t.Fatalf("a function was kept as a var: %v", m.Vars)
	}
}
