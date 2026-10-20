package play

import (
	"bytes"
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/layer5/gfx"
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

// TestPanelCommandFromScript: a script types a line into the Command tab and presses OK,
// which runs it as Enter does for a player.
func TestPanelCommandFromScript(t *testing.T) {
	g := storyGame(t, Options{
		Location: "camp", Event: "first", Development: true,
		Script: "wait 10\npress panel\nwait 3\npress right 3\nwait 3\ntype set: {flag.typed: true}\nwait 3\npress ok\nwait 3\nquit\n",
	}, "schema_version: 4\n")
	runToEnd(t, g)
	if !g.st.Flags["typed"] {
		t.Fatalf("the typed line did not run: %+v", g.panel)
	}
}

// TestPanelColumnsLineUp: the state and scenes tabs are tables measured in the font: every
// row's value starts at the same x, past the widest name, however long a name is (a
// fixed number of characters put the values at different places for names of different
// widths).
func TestPanelColumnsLineUp(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Event: "first", Development: true}, "schema_version: 4\n")
	g.st.Flags["a"] = true
	g.st.Flags["a_flag_with_a_name_far_longer_than_twenty_eight_characters"] = false
	g.st.Vars["WWWW"] = 1.0
	for name, cells := range map[string][][]string{"state": g.stateTable(), "scenes": g.scenesTable()} {
		const size = 18.0
		xs := panelColumns(cells, 40, size)
		if len(cells) < 2 || len(xs) != len(cells[0]) {
			t.Fatalf("%s: %d rows, columns at %v", name, len(cells), xs)
		}
		for j := 1; j < len(xs); j++ {
			for _, row := range cells {
				if end := xs[j-1] + gfx.TextWidth(row[j-1], size); end+panelGap > xs[j]+0.001 {
					t.Fatalf("%s: %q ends at %.1f, past column %d at %.1f", name, row[j-1], end, j, xs[j])
				}
			}
		}
	}
	// Proportional glyphs: names of the same length in characters differ in width, and the
	// values still share one column.
	if gfx.TextWidth("WWWW", 18) <= gfx.TextWidth("iiii", 18) {
		t.Fatal("the font measures W no wider than i")
	}
	cells := [][]string{{"iiii", "1"}, {"WWWW", "2"}}
	xs := panelColumns(cells, 0, 18)
	if w := gfx.TextWidth("WWWW", 18); xs[1] < w+panelGap {
		t.Fatalf("the value column at %.1f, the wide name ends at %.1f", xs[1], w)
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
