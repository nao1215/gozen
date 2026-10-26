package play

import (
	"strings"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

const btlMeters = `
meters:
  - {label: Energy, value: "var.energy", max: 10, marks: [3, 20], color: "#44aa44"}
  - {label: Charge, value: "var.charge", max_value: "var.cap", full: READY, full_color: "#ffcc00"}
  - {label: Secret, value: "var.energy", when: "flag.show", actor: scout}
`

// TestMetersShownByCondition: a meter with a "when" is shown only while it holds, and a
// meter tied to an actor is left out of the other member's page but not of the field list.
func TestMetersShownByCondition(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, btlMeters, "")
	if got := len(g.shownMeters("", false)); got != 2 {
		t.Fatalf("field meters %d, want 2 (the third waits for its flag)", got)
	}
	g.st.Flags["show"] = true
	if got := len(g.shownMeters("", false)); got != 3 {
		t.Fatalf("with the flag: %d meters", got)
	}
	if got := g.shownMeters("guide", true); len(got) != 2 {
		t.Fatalf("guide's page: %d meters", len(got))
	}
	if got := g.shownMeters("scout", true); len(got) != 3 {
		t.Fatalf("scout's page: %d meters", len(got))
	}
	// 3 meters of 44 high with 8 between.
	if h := g.metersHeight(); h != 3*(meterH+8)-8 {
		t.Fatalf("height %v", h)
	}
	g.st.Flags["show"] = false
	g.db.Game.Meters = nil
	if h := g.metersHeight(); h != 0 {
		t.Fatalf("no meters, height %v", h)
	}
}

// TestMeterValuesAreClampedAndDrawn: the value is held between 0 and the limit, a failing
// value is 0, and the rows draw (the label, the number or the full text) in each state.
func TestMeterValuesAreClampedAndDrawn(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, btlMeters, "")
	g.st.Vars["energy"] = 4.0
	g.st.Vars["charge"] = 6.0
	g.st.Vars["cap"] = 6.0
	ms := g.shownMeters("", false)
	if v, mx := g.meterValue(ms[0]); v != 4 || mx != 10 {
		t.Fatalf("energy %v/%v", v, mx)
	}
	if v, mx := g.meterValue(ms[1]); v != 6 || mx != 6 {
		t.Fatalf("charge %v/%v (full at the limit)", v, mx)
	}
	g.st.Vars["charge"] = 99.0
	if v, _ := g.meterValue(ms[1]); v != 6 {
		t.Fatalf("a value past the limit is %v", v)
	}
	g.st.Vars["charge"] = -5.0
	if v, _ := g.meterValue(ms[1]); v != 0 {
		t.Fatalf("a negative value is %v", v)
	}
	g.st.Vars["cap"] = "not a number"
	if _, mx := g.meterValue(ms[1]); mx != 100 {
		t.Fatalf("a failing limit falls back to the fixed max (100), got %v", mx)
	}
	g.db.Game.Meters[0].Value.Expr = nil
	if v, _ := g.meterValue(ms[0]); v != 0 {
		t.Fatalf("a failing value is %v", v)
	}
	g.drawMeters(btlImage(), ms, 10, 10, 400)
	g.st.Vars["cap"] = 6.0
	g.st.Vars["charge"] = 6.0
	g.drawMeters(btlImage(), ms, 10, 100, 400)
}

// A number meter shows its whole value, past max too (money), and ui.hide_gold hides the
// windows of the money at the kinds of place it lists.
func TestMeterNumberAndHiddenGold(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "meters:\n  - {label: Gold, value: gold, style: number}\n", "")
	g.st.Gold = 2500
	ms := g.shownMeters("", false)
	if got := g.meterNumber(ms[0]); got != "2500" {
		t.Fatalf("the number meter shows %q, want 2500", got)
	}
	g.drawMeters(btlImage(), ms, 10, 10, 260)
	if !g.goldShown() {
		t.Fatal("the money is hidden with no ui.hide_gold")
	}
	g.db.Game.UI.HideGold = []string{"dungeon"}
	if !g.goldShown() {
		t.Fatal("the money is hidden in a field when only dungeon is listed")
	}
	loc := g.db.Locations[g.st.Location]
	loc.Dungeon = &data.Dungeon{}
	if g.goldShown() {
		t.Fatal("the money shows in a dungeon listed in ui.hide_gold")
	}
	loc.Dungeon = nil
	g.db.Game.UI.HideGold = []string{"field"}
	if g.goldShown() {
		t.Fatal("the money shows in a field listed in ui.hide_gold")
	}
	m := newMenu(g)
	m.draw(g, btlImage())
}

// TestFrameMeterSecond: the meter counts updates and draws and, once a second has gone by,
// turns them into the text it shows, starting a new second.
func TestFrameMeterSecond(t *testing.T) {
	var m frameMeter
	m.toggle()
	if !m.on {
		t.Fatal("toggle did not turn it on")
	}
	m.drawn(time.Now().Add(-3 * time.Millisecond))
	m.updated(time.Now().Add(-2 * time.Millisecond))
	if m.text != "" || m.updates != 1 || m.draws != 1 {
		t.Fatalf("before a second: %+v", m)
	}
	if m.updMax < 2*time.Millisecond || m.drawMax < 3*time.Millisecond {
		t.Fatalf("maxima %v %v", m.updMax, m.drawMax)
	}
	m.since = time.Now().Add(-2 * time.Second)
	m.updated(time.Now())
	if !strings.Contains(m.text, "TPS") || !strings.Contains(m.text, "Update") || !strings.Contains(m.text, "Draw") {
		t.Fatalf("text %q", m.text)
	}
	if !m.on || m.updates != 0 || m.draws != 0 || m.updMax != 0 {
		t.Fatalf("a new second did not start: %+v", m)
	}
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
	g.meter = m
	g.drawMeter(btlImage())
	g.meter.toggle()
	if g.meter.on || g.meter.text != "" {
		t.Fatal("toggle off kept the old numbers")
	}
	g.drawMeter(btlImage())
	if ms(1500*time.Microsecond) != 1.5 {
		t.Fatal("ms")
	}
}
