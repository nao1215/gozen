package play

import (
	"fmt"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// TestScreenEffectsShorterWhenFast: holding OK or fast-forward halves a flash across the screen.
func TestScreenEffectsShorterWhenFast(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.addScreen(g, "flash", 0, 0, colorWhite, 0.5, 40)
	g.in.held[btnOK] = true
	s.addScreen(g, "flash", 0, 0, colorWhite, 0.5, 40)
	g.in.held[btnOK] = false
	if s.scr[1].dur >= s.scr[0].dur || s.scr[1].dur < 6 {
		t.Fatalf("durations %d and %d", s.scr[0].dur, s.scr[1].dur)
	}
}

// TestEffectBatchFlushesWhenFull: a scene of very many effects is drawn in several batches and ends
// with nothing left over.
func TestEffectBatchFlushesWhenFull(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.fx = nil
	for range 400 {
		s.addFx(g, "ice", 300, 200, 240, 240, fxColor("ice", data.Color{}), 2)
		s.fx[len(s.fx)-1].t = 12
	}
	s.drawEffects(g, btlImage())
	if len(s.fxb.vs) != 0 || len(s.fxb.is) != 0 {
		t.Fatalf("%d vertices left", len(s.fxb.vs))
	}
}

// TestMeterValueThatFails: a value that is not a number is shown as empty rather than stopping the
// screen.
func TestMeterValueThatFails(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, btlMeters, "")
	g.st.Vars["energy"] = "words"
	ms := g.shownMeters("", false)
	if v, mx := g.meterValue(ms[0]); v != 0 || mx != 10 {
		t.Fatalf("value %v max %v", v, mx)
	}
	g.drawMeters(btlImage(), ms, 10, 10, 300)
}

// TestDevPanelBeforeTheGameStarts: with no game state the panel lists only the declared names, and
// a command cannot run.
func TestDevPanelBeforeTheGameStarts(t *testing.T) {
	g := btlPanel(t)
	g.togglePanel()
	g.st = nil
	g.stack = []scene{newTitle(g)}
	if rows := g.stateRows(); len(rows) != len(g.db.Game.State.Names) {
		t.Fatalf("rows %v", rows)
	}
	if g.stateValue("flag.x") != "" {
		t.Fatal("a value with no state")
	}
	g.devDo("call: mark")
	if g.events.Running() {
		t.Fatal("a command ran with no game state")
	}
	g.panel.tab = tabScenes
	g.panelAct(1)
	if !strings.Contains(g.panel.msg, g.panelEvents()[0]) {
		t.Fatalf("a jump with no state: %q", g.panel.msg)
	}
	for tab := range int(numTabs) {
		g.panel.tab = panelTab(tab)
		g.Draw(btlImage())
	}
}

// TestDevPanelFlagTable: the flag columns show the current value of each flag and count the
// columns that do not fit.
func TestDevPanelFlagTable(t *testing.T) {
	var b strings.Builder
	b.WriteString("events:\n  many:\n    do:\n")
	for i := range 60 {
		fmt.Fprintf(&b, "      - {set: {flag.z%02d: true}}\n", i)
	}
	g, _ := btlOpen(t, Options{Location: "camp", Development: true}, "", b.String()+`
  one: {do: [{if: "flag.alpha", then: [{set: {flag.alpha: false}}]}]}
`)
	g.st.Flags["alpha"] = true
	g.togglePanel()
	g.panel.tab = tabFlags
	uses := g.panelUses()
	if uses["flag.alpha"] == nil || uses["flag.z59"] == nil {
		t.Fatalf("uses %v", uses)
	}
	u := uses["flag.alpha"]
	if len(u.Sets) != 1 || len(u.Reads) != 1 {
		t.Fatalf("alpha: %d sets %d reads", len(u.Sets), len(u.Reads))
	}
	g.Draw(btlImage())
	g.panel.top = 1
	g.evName = "one"
	g.Draw(btlImage())
}
