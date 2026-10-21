package play

import (
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer1/memo"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

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
