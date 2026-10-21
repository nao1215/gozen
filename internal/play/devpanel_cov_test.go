package play

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// btlPanel opens a development game with a panel and an event that sets and reads flags.
func btlPanel(t *testing.T) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "camp", Development: true}, "", `
events:
  gate:
    do:
      - if: "flag.opened"
        then: [{say: "Open."}]
        else: [{set: {flag.opened: true}}, {say: "Closed."}]
  mark: {do: [{set: {var.marks: "var.marks + 1"}}]}
`)
	return g
}

// TestDevPanelTabsCycle: left and right move through the five tabs and wrap round both ways;
// each change resets the cursor and the message and is traced; the Frames tab turns the
// meter on, and any other turns it off.
func TestDevPanelTabsCycle(t *testing.T) {
	g := btlPanel(t)
	g.tr = newTracer(&strings.Builder{})
	g.togglePanel()
	if g.panel == nil || g.panel.tab != tabState || !g.panelModal() {
		t.Fatalf("panel %+v", g.panel)
	}
	g.panel.sel, g.panel.msg = 3, "old"
	var seen []panelTab
	for range int(numTabs) {
		g.in.pressed[btnRight] = true
		g.updatePanel()
		g.in.pressed = [numButtons]bool{}
		seen = append(seen, g.panel.tab)
		if g.panel.sel != 0 || g.panel.msg != "" {
			t.Fatalf("tab %d kept the cursor %d / message %q", g.panel.tab, g.panel.sel, g.panel.msg)
		}
		if on := g.meter.on; on != (g.panel.tab == tabFrames) {
			t.Fatalf("meter %v on tab %d", on, g.panel.tab)
		}
	}
	if !slices.Equal(seen, []panelTab{tabFlags, tabScenes, tabCommand, tabFrames, tabState}) {
		t.Fatalf("order %v", seen)
	}
	g.in.pressed[btnLeft] = true
	g.updatePanel()
	g.in.pressed = [numButtons]bool{}
	if g.panel.tab != tabFrames || g.panelModal() {
		t.Fatalf("left from the first tab went to %d", g.panel.tab)
	}
	g.closePanel()
	if g.meter.on || g.panel != nil {
		t.Fatal("closing from Frames left the meter on")
	}
}

// TestDevPanelStateTabEditsValues: OK on a flag flips it, on a number adds one; names that
// are not numbers are left alone; every change is a set traced as dev_set.
func TestDevPanelStateTabEditsValues(t *testing.T) {
	g := btlPanel(t)
	var out strings.Builder
	g.tr = newTracer(&out)
	g.st.Vars["energy"] = 4.0
	g.st.Items["potion"] = 2
	g.togglePanel()
	rows := g.stateRows()
	for _, want := range []string{"var.energy", "gold", "item.potion"} {
		if !slices.Contains(rows, want) {
			t.Fatalf("no %s in %v", want, rows)
		}
	}
	act := func(name string, d int) {
		t.Helper()
		g.panel.sel = slices.Index(g.stateRows(), name)
		if g.panel.sel < 0 {
			t.Fatalf("no row %s", name)
		}
		g.panelAct(d)
	}
	act("var.energy", 1)
	act("gold", 1)
	act("item.potion", 1)
	if g.st.Vars["energy"] != 5.0 || g.st.Gold != 101 || g.st.Items["potion"] != 3 {
		t.Fatalf("energy %v gold %d potion %d", g.st.Vars["energy"], g.st.Gold, g.st.Items["potion"])
	}
	g.st.Flags["probe"] = true
	act("flag.probe", 1)
	if g.st.Flags["probe"] {
		t.Fatal("OK on a true flag did not clear it")
	}
	if g.stateValue("flag.probe") != "false" || g.stateValue("var.none") != "-" || g.stateValue("gold") != "101" {
		t.Fatal("stateValue")
	}
	g.st.Chose = map[string]string{"pick": "left"}
	if g.stateValue("chose.pick") != `"left"` || g.stateValue("other.x") != "" {
		t.Fatalf("chose/other: %q %q", g.stateValue("chose.pick"), g.stateValue("other.x"))
	}
	g.panel.sel = 9999 // past the rows: nothing happens
	g.panelAct(1)
	if n := len(btlLinesFrom(t, &out, "dev_set")); n != 4 {
		t.Fatalf("%d dev_set lines", n)
	}
	table := g.stateTable()
	if len(table) != len(rows)+1 && len(table) != len(g.stateRows()) {
		t.Fatalf("table %d rows for %d names", len(table), len(g.stateRows()))
	}
	for _, row := range table {
		if len(row) != 4 || !strings.HasPrefix(row[2], "writes ") || !strings.HasPrefix(row[3], "reads ") {
			t.Fatalf("row %v", row)
		}
	}
}

func btlLinesFrom(t *testing.T, out *strings.Builder, kinds ...string) []traceLine {
	t.Helper()
	return traceLines(t, []byte(out.String()), kinds...)
}

// TestDevPanelCommandErrors: a bad line of YAML is refused with a message and traced with
// its error; a good set runs at once and a line with other verbs waits for the panel to close.
func TestDevPanelCommandErrors(t *testing.T) {
	g := btlPanel(t)
	var out strings.Builder
	g.tr = newTracer(&out)
	g.togglePanel()
	g.panel.tab = tabCommand
	g.devDo("nonsense: 1")
	if g.panel.msg == "" {
		t.Fatal("a bad command left no message")
	}
	g.devDo("set: {flag.typed: true}")
	if !g.st.Flags["typed"] || g.panel.msg == "" {
		t.Fatal("a set did not run at once")
	}
	g.devDo("- set: {flag.a: true}\n- set: {flag.b: true}")
	if !g.st.Flags["a"] || !g.st.Flags["b"] {
		t.Fatal("a list of sets did not all run")
	}
	g.devDo("call: mark")
	if g.st.Vars["marks"] != nil {
		t.Fatal("a call ran before the panel closed")
	}
	if !g.events.Running() {
		t.Fatal("the call was not queued")
	}
	if len(btlLinesFrom(t, &out, "dev_do")) != 2 || len(btlLinesFrom(t, &out, "dev_set")) != 2 {
		t.Fatalf("trace: %s", out.String())
	}
	g.panel.text = "set: {flag.by_enter: true}"
	g.panelAct(1)
	if !g.st.Flags["by_enter"] {
		t.Fatal("OK on the Command tab did not run the line")
	}
	g.panel = nil
	g.devMessage("ignored without a panel")
}

// TestDevPanelJump: OK on the Scenes tab starts that event from the state as it is, closes
// the panel and holds the game to the line; a missing event says so.
func TestDevPanelJump(t *testing.T) {
	g := btlPanel(t)
	var out strings.Builder
	g.tr = newTracer(&out)
	g.togglePanel()
	g.panel.tab = tabScenes
	evs := g.panelEvents()
	i := slices.Index(evs, "mark")
	if i < 0 {
		t.Fatalf("events %v", evs)
	}
	g.panel.sel = i
	g.panelAct(1)
	if g.panel != nil {
		t.Fatal("the jump left the panel open")
	}
	for range 40 {
		btlFrame(t, g)
	}
	if g.st.Vars["marks"] != 1.0 {
		t.Fatalf("marks %v", g.st.Vars["marks"])
	}
	if j := btlLinesFrom(t, &out, "jump"); len(j) != 1 || j[0]["event"] != "mark" {
		t.Fatalf("jump lines %v", j)
	}
	g.togglePanel()
	g.devJump("no_such_event", 1)
	if !strings.Contains(g.panel.msg, "no_such_event") {
		t.Fatalf("message %q", g.panel.msg)
	}
	table := g.scenesTable()
	if len(table) != len(evs) {
		t.Fatalf("scenes table %d for %d events", len(table), len(evs))
	}
	for _, row := range table {
		if row[0] == "greet" && (row[2] != "once" || !strings.Contains(row[1], "enter@field")) {
			t.Fatalf("greet row %v", row)
		}
		if row[0] == "mark" && row[3] != "ran 1" {
			t.Fatalf("mark row %v", row)
		}
	}
	g.st.Done["greet"] = true
	for _, row := range g.scenesTable() {
		if row[0] == "greet" && row[2] != "done" {
			t.Fatalf("a finished once-event is %q", row[2])
		}
	}
}

// TestDevPanelCursorStaysInRange: up and down stop at the first and last row; X and Esc close.
func TestDevPanelCursorStaysInRange(t *testing.T) {
	g := btlPanel(t)
	g.togglePanel()
	n := g.panelRows()
	for range n + 3 {
		g.in.pressed[btnDown] = true
		g.updatePanel()
		g.in.pressed = [numButtons]bool{}
	}
	if g.panel.sel != n-1 {
		t.Fatalf("cursor %d of %d rows", g.panel.sel, n)
	}
	for range n + 3 {
		g.in.pressed[btnUp] = true
		g.updatePanel()
		g.in.pressed = [numButtons]bool{}
	}
	if g.panel.sel != 0 {
		t.Fatalf("cursor %d", g.panel.sel)
	}
	g.panel.tab = tabCommand
	if g.panelRows() != 0 {
		t.Fatal("the Command tab has rows")
	}
	g.panel.tab = tabState
	g.in.pressed[btnCancel] = true
	g.updatePanel()
	g.in.pressed = [numButtons]bool{}
	if g.panel != nil {
		t.Fatal("cancel did not close the panel")
	}
	g.togglePanel()
	g.in.esc = true
	g.updatePanel()
	g.in.esc = false
	if g.panel != nil {
		t.Fatal("Esc did not close the panel")
	}
}

// TestDevPanelDraws: every tab draws, with a game that has events and flags and with a long
// row list (the cursor scrolls), without changing the state it shows.
func TestDevPanelDraws(t *testing.T) {
	g := btlPanel(t)
	for i := range 60 {
		g.st.Flags["flag_number_"+string(rune('a'+i%26))+string(rune('a'+i/26))] = true
	}
	g.togglePanel()
	before := len(g.stateRows())
	for tab := range int(numTabs) {
		g.panel.tab = panelTab(tab)
		g.panel.msg = "message"
		g.panel.sel = g.panelRows() - 1
		g.panel.sel = max(g.panel.sel, 0)
		g.Draw(btlImage())
		if tab == int(tabState) && g.panel.top == 0 && g.panelRows() > 20 {
			t.Fatal("the list did not scroll to the cursor")
		}
	}
	if len(g.stateRows()) != before {
		t.Fatal("drawing changed the state")
	}
	g.panel.tab = tabCommand
	g.panel.text = "set: {flag.x: true}"
	g.Draw(btlImage())
	g.panel.tab = tabFlags
	g.evName = "gate"
	g.Draw(btlImage())
	// The panel is only for development.
	g.opts.Development = false
	g.Draw(btlImage())
}

// TestPanelUsesIsCachedPerData: the index of names is made once for a DB and again for another.
func TestPanelUsesIsCachedPerData(t *testing.T) {
	g := btlPanel(t)
	a := g.panelUses()
	if a == nil || a["flag.opened"] == nil {
		t.Fatalf("no use of flag.opened: %v", a)
	}
	if u := a["flag.opened"]; len(u.Sets) != 1 || len(u.Reads) != 1 {
		t.Fatalf("flag.opened: %d sets %d reads", len(u.Sets), len(u.Reads))
	}
	b := g.panelUses()
	if reflect.ValueOf(a).Pointer() != reflect.ValueOf(b).Pointer() {
		t.Fatal("the index was made twice for the same data")
	}
}
