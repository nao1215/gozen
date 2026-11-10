package play

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"

	"github.com/nao1215/gozen/internal/screen/gfx"
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

// TestDevPanelCommandAtTheTitle: at the title no game runs, so a line typed into the Command
// tab is refused with a message that says so, and traced with it, not with an empty one.
func TestDevPanelCommandAtTheTitle(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "-", Development: true}, "", "")
	if g.st != nil {
		t.Fatal("not at the title")
	}
	var out strings.Builder
	g.tr = newTracer(&out)
	g.togglePanel()
	g.panel.tab = tabCommand
	g.devDo("set: {flag.typed: true}")
	if strings.Trim(g.panel.msg, "[] ") == "" {
		t.Fatalf("message %q", g.panel.msg)
	}
	ls := btlLinesFrom(t, &out, "dev_do")
	if len(ls) != 1 || ls[0]["error"] != g.panel.msg {
		t.Fatalf("trace: %s", out.String())
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

// TestDevPanelHoldsTheGame: while the panel's body is shown the game's screens do not update but the
// tick goes on; on the Frames tab the game goes on; the panel always opens on the State tab; and
// it is only there under --dev.
func TestDevPanelHoldsTheGame(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one", Development: true, ShotDir: t.TempDir(), Script: "wait 5\npress panel\nwait 100\npress panel\nwait 100\nquit\n"}, "", "")
	s := btlBattle(t, g)
	var opened, held, ticks0 int
	for range 400 {
		btlFrame(t, g)
		switch {
		case g.panel != nil && opened == 0:
			opened, held, ticks0 = 1, s.age, g.tick
		case g.panel != nil && g.tick-ticks0 == 40:
			if s.age != held {
				t.Fatalf("the battle moved while the panel was open: age %d -> %d", held, s.age)
			}
			opened = 2
		}
		if opened == 2 && g.panel == nil {
			break
		}
	}
	if opened != 2 {
		t.Fatalf("the panel did not stay open for 40 ticks (state %d)", opened)
	}
	if g.panel != nil {
		t.Fatal("the second press did not close the panel")
	}
	for range 10 {
		btlFrame(t, g)
	}
	if s.age == held {
		t.Fatal("the battle did not go on after the panel closed")
	}
}

// TestDevPanelOpensOnTheFirstTab: every time it opens, the panel is on the State tab, and it is not
// opened at all without --dev.
func TestDevPanelOpensOnTheFirstTab(t *testing.T) {
	g := btlPanel(t)
	g.togglePanel()
	g.panelTab(true)
	g.panelTab(true)
	if g.panel.tab != tabScenes {
		t.Fatalf("tab %d", g.panel.tab)
	}
	g.closePanel()
	g.togglePanel()
	if g.panel.tab != tabState || g.panel.sel != 0 {
		t.Fatalf("reopened on tab %d row %d", g.panel.tab, g.panel.sel)
	}
}

// TestDevPanelNeedsDevelopmentMode: the panel key does nothing in a game started without --dev.
func TestDevPanelNeedsDevelopmentMode(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp", ShotDir: t.TempDir(), Script: "wait 3\npress panel\nwait 10\nquit\n"}, "", "")
	for range 40 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if g.in.script.pc != 1 {
		t.Fatal("disabled panel key was acknowledged")
	}
	if g.panel != nil {
		t.Fatal("the panel opened without --dev")
	}
}

func TestPanelReadOnlyOKRemainsPending(t *testing.T) {
	g := sysInn(t, Options{Development: true})
	g.panel = &devPanel{tab: tabFlags}
	g.st.Flags["probe"] = false
	s := sysScript(t, "press ok")
	s.acknowledge = true
	for range 3 {
		sysKey(g)
		s.step(&g.in)
		g.updatePanel()
		g.in.finishOffer()
		if s.pc != 0 {
			t.Fatal("read-only panel acknowledged OK")
		}
	}
	g.panel.tab = tabState
	g.panel.sel = slices.Index(g.stateRows(), "flag.probe")
	if g.panel.sel < 0 {
		t.Fatal("missing editable flag")
	}
	sysKey(g)
	s.step(&g.in)
	g.updatePanel()
	g.in.finishOffer()
	if s.pc != 1 || !g.st.Flags["probe"] {
		t.Fatal("pending OK did not edit the flag")
	}
}

func TestOverlayModel(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Event: "first", Seed: 71, Development: true}, "schema_version: 4\n")
	g.evName, g.evLines, g.tick = "first", 2, 42
	g.db.Events["first"].When = data.Expr{Expr: testgame.Expr("flag.ready and var.count > 0")}
	g.st.Flags["ready"], g.st.Flags["unrelated"] = true, true
	g.st.Vars["count"] = 3.0
	f := &layoutFrame{cast: []standFig{{ID: "actor:guide", Side: "left", X: 0.25, Face: "guide.png"}}}
	before, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Join(g.overlayLines(f), "\n")
	for _, want := range []string{"42", "first", "Line 2", "camp", "71", "guide.png", "left x=0.25", "flag.ready = true", "var.count = 3", "Phone: closed"} {
		if !strings.Contains(lines, want) {
			t.Errorf("missing %q in %s", want, lines)
		}
	}
	if strings.Contains(lines, "unrelated") {
		t.Fatal(lines)
	}
	g.phone = &phoneView{cue: data.PhoneCue{App: "page"}, age: 12, closing: 2, result: -1}
	if got := strings.Join(g.overlayLines(f), "\n"); !strings.Contains(got, "Phone: page | age 12 | closing 2 | result -1") {
		t.Fatal(got)
	}
	after, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("observation mutated saved state or random stream")
	}
	c := screenControl{Label: "Trail", Reason: "Gate closed", Focused: true}
	if got := overlayControlText(c); !strings.Contains(got, "not pressable: Gate closed") || !strings.Contains(got, "focused") {
		t.Fatal(got)
	}
	c.Enabled = true
	if got := overlayControlText(c); strings.Contains(got, "Gate closed") || !strings.Contains(got, "pressable") {
		t.Fatal(got)
	}
	g.st = nil
	g.evName = ""
	if got := strings.Join(g.overlayLines(&layoutFrame{}), "\n"); !strings.Contains(got, "Location -") {
		t.Fatal(got)
	}
}

func TestOverlayCollectorOff(t *testing.T) {
	g := &Game{}
	if n := testing.AllocsPerRun(100, func() {
		if g.overlayActive() {
			t.Fatal("release overlay active")
		}
		g.traceControl("button", [4]float64{1, 2, 3, 4}, false, false, "blocked")
		g.traceStandFigures(nil)
	}); n != 0 {
		t.Fatalf("closed overlay allocated %g objects", n)
	}
	g.panel = &devPanel{tab: tabFrames}
	if g.overlayActive() {
		t.Fatal("Frames enabled outside development")
	}
	g.opts.Development = true
	g.panel.collecting = true
	g.traceControl("button", [4]float64{1, 2, 3, 4}, false, true, "blocked")
	g.transformScreenRecords(0, 0, func(x, y float64) (float64, float64) { return x + 10, y + 20 })
	if c := g.panel.frame.controls[0]; c.Rect != [4]float64{11, 22, 3, 4} || c.Reason != "blocked" || !c.Focused {
		t.Fatal(c)
	}
	g.closePanel()
	if g.controlFrame() != nil {
		t.Fatal("closed collector retained")
	}
}

func TestOverlayRecentChanges(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Development: true}, "schema_version: 4\n")
	g.panel = &devPanel{tab: tabFrames}
	g.evName = ""
	g.st.Flags["ready"] = false
	g.observeOverlayChanges()
	if len(g.panel.recent) != 0 {
		t.Fatal("initial values treated as recent changes")
	}
	g.st.Flags["ready"] = true
	g.tick += 6
	g.st.Vars["count"] = 2.0
	g.observeOverlayChanges()
	lines := strings.Join(g.overlayLines(&layoutFrame{}), "\n")
	if !strings.Contains(lines, "Changed: flag.ready = true") || !strings.Contains(lines, "Changed: var.count = 2") {
		t.Fatal(lines)
	}
	g.observeOverlayChanges()
	if len(g.panel.recent) != 2 {
		t.Fatal("unchanged values duplicated")
	}
}

// The regular --dev path has no tracer. Real drawing helpers must still populate
// control boxes; constructing a model by hand would not catch trace-only guards.
func TestOverlayDrawCollectsWithoutTrace(t *testing.T) {
	g := storyGame(t, Options{Location: "camp", Event: "first", Development: true, Seed: 7}, "schema_version: 4\nui: {message: novel}\n")
	if g.tr != nil {
		t.Fatal("fixture unexpectedly tracing")
	}
	for range 30 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	g.togglePanel()
	g.panelTab(false)
	g.Draw(btlImage())
	if len(g.panel.frame.controls) == 0 {
		t.Fatal("overlay without trace collected no controls")
	}
	if g.panel.collecting {
		t.Fatal("collector left enabled between frames")
	}
	g.closePanel()
	g.Draw(btlImage())
	if g.controlFrame() != nil {
		t.Fatal("closed overlay collected drawing")
	}
}

func TestOverlaySharesSectionTiming(t *testing.T) {
	g := &Game{opts: Options{Development: true}, tr: newTracer(nil)}
	g.tr.performance = newTracePerformance(0)
	p := g.tr.performance
	g.togglePanel()
	g.panelTab(false)
	if g.panel.performance != p || p.durations == nil {
		t.Fatal("Frames did not attach to section collector")
	}
	p.updated(time.Now())
	p.drawn(time.Now())
	if p.durations.update.count != 1 || p.durations.draw.count != 1 {
		t.Fatal("missing timing samples")
	}
	g.closePanel()
	if p.durations != nil {
		t.Fatal("closed overlay left timing collection enabled")
	}
	p.updated(time.Now())
	if p.whole.ticks != 2 {
		t.Fatal("closing overlay disturbed section counts")
	}
}

// In the Command tab a letter that stands for left or right elsewhere is text of the
// command: it does not change the tab. The arrows and a pad do.
func TestDevPanelCommandTabKeepsTypedLetters(t *testing.T) {
	g := btlPanel(t)
	g.togglePanel()
	g.panel.tab = tabCommand
	g.in.pressed[btnLeft] = true
	g.commandTabKeys(true)
	if g.panel.tab != tabCommand {
		t.Fatalf("a typed letter changed the tab to %d", g.panel.tab)
	}
	g.commandTabKeys(false)
	g.in.pressed = [numButtons]bool{}
	if g.panel.tab != tabScenes {
		t.Fatalf("left went to tab %d, want the Scenes tab", g.panel.tab)
	}
}

// The Flags tab keeps rows for the flags' numbers and names: its cursor stays on a row
// drawn however far it goes down.
func TestDevPanelFlagsCursorStaysShown(t *testing.T) {
	var b strings.Builder
	b.WriteString("\nevents:\n")
	for i := range 40 {
		b.WriteString("  filler_" + string(rune('a'+i/26)) + string(rune('a'+i%26)) + ": {do: [{set: {flag.seen: true}}]}\n")
	}
	g, _ := btlOpen(t, Options{Location: "camp", Development: true}, "", b.String())
	g.togglePanel()
	g.panel.tab = tabState
	state := g.panelShownRows()
	g.panel.tab = tabFlags
	if shown := g.panelShownRows(); shown != state-2 {
		t.Fatalf("the Flags tab shows %d rows, the State tab %d: want two fewer", shown, state)
	}
	g.panel.sel = g.panelRows() - 1
	g.Draw(btlImage())
	if p := g.panel; p.sel < p.top || p.sel >= p.top+g.panelShownRows() {
		t.Fatalf("the cursor on row %d is not drawn: rows %d to %d are", p.sel, p.top, p.top+g.panelShownRows()-1)
	}
}
