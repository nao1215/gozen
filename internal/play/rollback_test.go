package play

import (
	"bytes"
	"testing"
	"testing/fstest"
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
