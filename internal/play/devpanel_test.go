package play

import (
	"bytes"
	"slices"
	"testing"
)

// TestPanelEditsAndJumps: under --dev, F8 opens the panel and holds the game; flipping a
// flag goes through set (dev_set), the scenes tab jumps to an event (jump), the frames
// tab shows the meter and lets the game go on, and F8 closes it (ADR 0029).
func TestPanelEditsAndJumps(t *testing.T) {
	g := storyGame(t, Options{
		Location: "camp", Event: "first", Development: true,
		Script: "wait 10\npress panel\nwait 3\npress right\nwait 3\npress right\nwait 3\npress down 3\nwait 3\npress ok\nwait 30\nquit\n",
	}, "schema_version: 4\n")
	g.st.Flags["probe"] = false
	var out bytes.Buffer
	g.tr = newTracer(&out)
	runToEnd(t, g)
	jumps := traceLines(t, out.Bytes(), "jump")
	if len(jumps) != 1 || jumps[0]["event"] != "second" || g.panel != nil {
		t.Fatalf("jumps %v, panel %v", jumps, g.panel != nil)
	}
	if at(g) != "Four." || g.evName != "second" {
		t.Fatalf("at %q in %q", at(g), g.evName)
	}
	g.panel = &devPanel{}
	rows := g.stateRows()
	i := slices.Index(rows, "flag.probe")
	if i < 0 {
		t.Fatalf("no flag.probe in %v", rows)
	}
	g.panel.sel = i
	g.panelAct(1)
	if !g.st.Flags["probe"] {
		t.Fatal("the flag did not flip")
	}
	if sets := traceLines(t, out.Bytes(), "dev_set"); len(sets) != 1 || sets[0]["do"] != "- set: {flag.probe: true}" {
		t.Fatalf("dev_set lines %v", sets)
	}
	g.panelTab(false) // to Frames (the last tab)
	if !g.meter.on || g.panelModal() {
		t.Fatalf("frames: meter %v, modal %v", g.meter.on, g.panelModal())
	}
	g.togglePanel()
	if g.panel != nil || g.meter.on {
		t.Fatal("F8 did not close the panel and the meter")
	}
}

// TestScriptJumpAndDo: a script's jump starts an event at a line and do runs YAML.
func TestScriptJumpAndDo(t *testing.T) {
	g := storyGame(t, Options{
		Location: "camp", Event: "first", Development: true,
		Script: "wait 10\ndo - set: {var.way: 7}\njump second 2\nwait 30\nquit\n",
	}, "schema_version: 4\n")
	runToEnd(t, g)
	if g.st.Vars["way"] != 7.0 || at(g) != "Five." {
		t.Fatalf("way %v, at %q", g.st.Vars["way"], at(g))
	}
	for _, bad := range []string{"jump", "do"} {
		if _, err := parseScript(bad + "\n"); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
}
