package play

import (
	"bytes"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// storyData is two events: the first sets a flag after its lines and a choice; the second
// starts by itself when the flag holds. A third crosses a boundary (a move).
const storyData = `events:
  first:
    do:
      - say: "One."
      - say: "Two."
      - choice:
          - {label: Left, do: [{set: {var.way: 1}}]}
          - {label: Right, do: [{set: {var.way: 2}}]}
        id: way
      - say: "Three."
      - set: {flag.first_done: true}
  second:
    trigger: auto
    once: true
    when: "flag.first_done"
    do:
      - say: "Four."
      - say: "Five."
  away:
    do:
      - say: "Here."
      - move: field
      - say: "There."
`

func storyGame(t *testing.T, opts Options, head string) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte(head), fsys["game.yaml"].Data...)}
	fsys["data/story.yaml"] = &fstest.MapFile{Data: []byte(storyData)}
	opts.FS, opts.NoSound = fsys, true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.ShotDir == "" {
		opts.ShotDir = t.TempDir()
	}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

// at reports the line on the screen.
func at(g *Game) string {
	if g.talk.choice != nil {
		return "choice"
	}
	if !g.talk.active {
		return ""
	}
	return g.talk.say.Text
}

// TestRollBackInAnEvent: rolling back shows the line before, the choice again with the
// state before it was answered, and the backlog loses the lines rolled back.
func TestRollBackInAnEvent(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Event: "first", Script: "wait 10\npress ok\nwait 10\npress ok\nwait 10\npress down\nwait 10\npress ok\nwait 10\nquit\n"}, "schema_version: 4\n")
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	if at(g) != "Three." || g.st.Vars["way"] != 2.0 || g.st.Chose["way"] != "1" {
		t.Fatalf("at %q, way %v, chose %v", at(g), g.st.Vars["way"], g.st.Chose)
	}
	logged := len(g.backlog)
	g.rollBack()
	if at(g) != "choice" || g.evLines != 3 || g.st.Vars["way"] != nil || g.st.Chose["way"] != "" {
		t.Fatalf("back to %q (line %d), way %v, chose %v", at(g), g.evLines, g.st.Vars["way"], g.st.Chose)
	}
	if len(g.backlog) >= logged {
		t.Fatalf("the backlog kept the lines rolled back: %d of %d", len(g.backlog), logged)
	}
	g.rollBack()
	g.rollBack()
	if at(g) != "One." || g.canRollBack() {
		t.Fatalf("back to %q, can roll back %v", at(g), g.canRollBack())
	}
	rb := traceLines(t, out.Bytes(), "rollback")
	if len(rb) != 3 || rb[0]["from"] != 4.0 || rb[0]["to"] != 3.0 || rb[2]["to"] != 1.0 {
		t.Fatalf("rollback lines %v", rb)
	}
}

// TestRollBackIntoTheEventBefore: at the first line of an event, rolling back goes to the
// last line of the event before, with its state.
func TestRollBackIntoTheEventBefore(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Event: "first", Script: "wait 10\npress ok\nwait 10\npress ok\nwait 10\npress ok\nwait 10\npress ok\nwait 20\nquit\n"}, "schema_version: 4\n")
	runToEnd(t, g)
	if at(g) != "Four." || !g.st.Flags["first_done"] {
		t.Fatalf("at %q, flags %v", at(g), g.st.Flags)
	}
	g.rollBack()
	if at(g) != "Three." || g.st.Flags["first_done"] || g.evName != "first" || g.st.Chose["way"] != "0" {
		t.Fatalf("back to %q in %q, flags %v, chose %v", at(g), g.evName, g.st.Flags, g.st.Chose)
	}
	if len(g.backRing) != 0 {
		t.Fatalf("the ring kept %d events", len(g.backRing))
	}
}

// TestRollBackStopsAtABoundary: an event that moved cannot be rolled back.
func TestRollBackStopsAtABoundary(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Event: "away", Script: "wait 10\npress ok\nwait 30\nquit\n"}, "schema_version: 4\n")
	runToEnd(t, g)
	if at(g) != "There." || g.canRollBack() {
		t.Fatalf("at %q, can roll back %v", at(g), g.canRollBack())
	}
}

// TestRollBackOff: ui.rollback: 0 turns rolling back off.
func TestRollBackOff(t *testing.T) {
	off := storyGame(t, Options{Location: "camp", Event: "first", Script: "wait 10\npress ok\nwait 10\nquit\n"}, "schema_version: 4\nui: {rollback: 0}\n")
	runToEnd(t, off)
	if at(off) != "Two." || off.canRollBack() {
		t.Fatalf("at %q, can roll back %v with ui.rollback 0", at(off), off.canRollBack())
	}
}

// TestSaveInTheMiddleOfAnEvent: a save made at a line goes on at that line, with the
// state there; a choice answered before it is answered again.
func TestSaveInTheMiddleOfAnEvent(t *testing.T) {
	saves := t.TempDir()
	g := storyGame(t, Options{Location: "camp", Event: "first", SaveDir: saves, Script: "wait 10\npress ok\nwait 10\npress ok\nwait 10\npress ok\nwait 10\nquit\n"}, "schema_version: 4\n")
	runToEnd(t, g)
	if at(g) != "Three." {
		t.Fatalf("at %q", at(g))
	}
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	b, err := g.store.Read(g.slotPath(0))
	if err != nil || !bytes.Contains(b, []byte(`"cursor"`)) || !bytes.Contains(b, []byte(`"event": "first"`)) {
		t.Fatalf("no cursor in %s (%v)", b, err)
	}
	st, err := g.load(0)
	if err != nil || st.Resume == nil {
		t.Fatalf("load: %v", err)
	}
	g.loadSavedGame(st)
	if at(g) != "Three." || g.evLines != 4 || g.st.Vars["way"] != 1.0 || g.st.Chose["way"] != "0" {
		t.Fatalf("resumed at %q (line %d), way %v", at(g), g.evLines, g.st.Vars["way"])
	}
	g.rollBack()
	if at(g) != "choice" {
		t.Fatalf("after loading, rolling back reached %q", at(g))
	}
}

// A save in the lines after a battle in the same event cannot hold where the event goes
// on: it is refused with the reason instead of keeping the state without the event's rest.
func TestSaveRefusedInTalkAfterABoundary(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "field"}, "", `
events:
  fight_talk: {do: [{battle: one}, {say: {text: "Your reward."}}, {say: {text: "Here is the key."}}, {set: {flag.key: true}}]}
`)
	g.events.Start(g.db.Events["fight_talk"].Do)
	for range 20000 {
		if g.talk.active && g.currentBattle() == nil && len(g.stack) == 1 {
			break
		}
		if s := g.currentBattle(); s != nil && s.phase == bpInput && s.target == nil && s.sub == nil {
			btlKey(g, btnOK)
			btlKey(g, btnOK)
			continue
		}
		if s := g.currentBattle(); s != nil && s.phase == bpResult {
			g.in.pressed[btnOK] = true
			g.tick++
			s.update(g)
			g.in.pressed = [numButtons]bool{}
		}
		btlFrame(t, g)
	}
	if !g.talk.active || g.st.Flags["key"] {
		t.Fatalf("not in the lines after the battle (talk %v, key %v)", g.talk.active, g.st.Flags["key"])
	}
	if g.saveBlocked() == "" {
		t.Fatal("a save after the battle is not refused")
	}
	g.in.pressed[btnSave] = true
	g.novelInput()
	g.in.pressed = [numButtons]bool{}
	m, ok := g.stack[len(g.stack)-1].(*saveMenu)
	if !ok || m.msgT == 0 || m.msg != g.saveBlocked() {
		t.Fatalf("the save button opened %T without the reason", g.stack[len(g.stack)-1])
	}
	btlKey(g, btnOK)
	if g.storeHas(g.slotPath(0)) {
		t.Fatal("a save was written that would lose the key")
	}
}

// The sortie screen cannot be read again by a loaded save, so it is a boundary: the lines
// after a march chosen there neither roll back into it nor save a cursor that would stop
// on it.
func TestSortieIsABoundary(t *testing.T) {
	g, _ := campaignOpen(t, nil)
	g.db.Events["march"] = &data.Event{Do: []data.Command{{Order: &data.OrderCmd{ID: "storm", Target: "keep"}}, {Say: &data.Say{Text: "We marched."}}, {Say: &data.Say{Text: "And came back."}}}}
	g.events.Start(g.db.Events["march"].Do)
	wldUntil(t, g, 300, func() bool { _, ok := g.stack[len(g.stack)-1].(*sortieScene); return ok })
	s, ok := g.stack[len(g.stack)-1].(*sortieScene)
	if !ok {
		t.Fatalf("the order opened %T, not the sortie", g.stack[len(g.stack)-1])
	}
	s.toggle(g, "hero")
	s.close(g, true)
	wldUntil(t, g, 3000, func() bool {
		if _, ok := g.stack[len(g.stack)-1].(*clashScene); ok {
			wldPlay(t, g, "press ok\nwait 3\n", 50)
		}
		return g.talk.active && len(g.stack) == 1
	})
	if !g.talk.active {
		t.Fatal("the lines after the march did not come")
	}
	if _, _, _, ok := g.cursor(); ok || g.saveBlocked() == "" {
		t.Fatalf("after the sortie a save keeps a cursor (%v) or is not refused", ok)
	}
}
