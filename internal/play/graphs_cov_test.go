package play

import (
	"image/color"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/headless/balance"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// btlGraphs is a development game in the field with a hurt first member, with fonts for drawing.
func btlGraphs(t *testing.T) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "field", Development: true}, "", "")
	g.st.Members()[0].HP = 3
	return g
}

// TestTroopLabel: a troop shows its own name, else the names of its foes with a count when a
// foe comes more than once, in the order they first appear; an unknown id shows itself.
func TestTroopLabel(t *testing.T) {
	db := testgame.Load(t, nil)
	for _, tc := range []struct{ id, want string }{
		{"one", "Mote"},
		{"two", "Mote×2"},
		{"boss", "Sentinel"},
		{"missing", "missing"},
	} {
		if got := troopLabel(db, tc.id); got != tc.want {
			t.Errorf("troopLabel(%s) = %q, want %q", tc.id, got, tc.want)
		}
	}
	db.Troops["named"] = db.Troops["two"]
	named := *db.Troops["two"]
	named.Name = "A pair"
	db.Troops["named"] = &named
	if got := troopLabel(db, "named"); got != "A pair" {
		t.Errorf("a named troop shows %q", got)
	}
	mixed := *db.Troops["two"]
	mixed.Members = []string{"mote", "boss", "mote", "ghost"}
	db.Troops["mixed"] = &mixed
	if got := troopLabel(db, "mixed"); got != "Mote×2 Sentinel ghost" {
		t.Errorf("mixed troop %q", got)
	}
}

// TestGraphShade: the tier for the current HP is the troop's color darkened, never brighter and
// with the same alpha.
func TestGraphShade(t *testing.T) {
	for _, c := range graphColors {
		d := shade(c)
		if d.R > c.R || d.G > c.G || d.B > c.B || d.A != c.A {
			t.Errorf("shade(%v) = %v", c, d)
		}
		if c.R > 10 && d.R >= c.R {
			t.Errorf("shade(%v) did not darken red: %v", c, d)
		}
	}
	if got := shade(color.NRGBA{255, 100, 0, 200}); got != (color.NRGBA{153, 60, 0, 200}) {
		t.Errorf("shade = %v", got)
	}
}

// TestGraphsDrawStates: the panel draws in each state it can be in (nothing to measure,
// measuring, an error, hurt and healed, in a fight) without changing the game or its results.
func TestGraphsDrawStates(t *testing.T) {
	g := btlGraphs(t)
	scr := btlImage()
	g.drawGraphs(scr) // closed: nothing

	g.graphs.on = true
	g.tick = graphCheck
	g.updateGraphs()
	g.drawGraphs(scr) // measuring, partial
	n, total := g.graphs.progress()
	if n >= total {
		t.Fatalf("progress %d/%d right after the first check", n, total)
	}
	runGraphs(t, g)
	g.drawGraphs(scr) // all measured, hurt
	if n, total := g.graphs.progress(); n != total {
		t.Fatalf("progress %d/%d after measuring", n, total)
	}
	hp := g.st.Members()[0].HP
	g.st.Members()[0].HP = g.st.Members()[0].Stat("mhp")
	for _, m := range g.st.Members() {
		m.HP, m.MP = m.Stat("mhp"), m.Stat("mmp")
	}
	g.tick += graphCheck * 5
	g.graphsCheck()
	g.tick += graphSettle + graphCheck
	g.graphsCheck()
	g.drawGraphs(scr) // healed
	if g.graphs.hurt {
		t.Fatal("a healed party is shown as hurt")
	}
	g.st.Members()[0].HP = hp
	g.graphs.fight = "one"
	g.drawGraphs(scr) // the troop in battle is lit

	g.graphs.err = "boom"
	g.drawGraphs(scr)
	g.graphs.err = ""
	g.graphs.here = nil
	g.drawGraphs(scr) // nothing met here
	g.st = nil
	g.drawGraphs(scr) // before the game starts
}

// TestGraphsErrorStopsMeasuring: a start that cannot be read stops the measuring with the
// message; nothing is measured while the error stands, and a change of data starts again.
func TestGraphsErrorStopsMeasuring(t *testing.T) {
	g := btlGraphs(t)
	g.graphs.on = true
	g.tick = graphCheck
	g.updateGraphs()
	g.graphs.save = []byte("not a save")
	g.graphs.now = g.graphs.save
	g.graphs.res = map[graphJob]*graphResult{}
	g.updateGraphs()
	if g.graphs.err == "" {
		t.Fatal("a broken start did not stop the measuring")
	}
	before := len(g.graphs.res)
	g.tick++
	g.updateGraphs()
	if len(g.graphs.res) != before {
		t.Fatal("measured while the error stands")
	}
	g.drawGraphs(btlImage())
	db, err := data.Load(g.opts.FS)
	if err != nil {
		t.Fatal(err)
	}
	g.db = db
	g.tick = graphCheck * 3
	g.updateGraphs()
	if g.graphs.err != "" {
		t.Fatalf("a reload did not clear the error: %s", g.graphs.err)
	}
}

// TestGraphsPanelWanted: what is measured, in what order: healed bars first, then the current
// HP bars only when someone is hurt, the streaks, then the levels above; at most six troops.
func TestGraphsPanelWanted(t *testing.T) {
	p := &graphPanel{base: 4}
	for i := range 9 {
		p.here = append(p.here, balance.Encounter{Troop: string(rune('a' + i))})
	}
	jobs := p.wanted()
	rows := graphRows
	if want := rows + rows + rows*graphLevels; len(jobs) != want {
		t.Fatalf("%d jobs, want %d", len(jobs), want)
	}
	if jobs[0].kind != graphHealed || jobs[rows].kind != graphRun || jobs[2*rows].level != 5 {
		t.Fatalf("order %+v", jobs[:2*rows+1])
	}
	p.hurt = true
	jobs = p.wanted()
	if len(jobs) != 3*rows+rows*graphLevels || jobs[rows].kind != graphNow {
		t.Fatalf("hurt: %d jobs, second block %+v", len(jobs), jobs[rows])
	}
	if p.has("c") != true || p.has("zz") {
		t.Fatal("has")
	}
	p.res = map[graphJob]*graphResult{}
	if j, ok := p.next(); !ok || j != jobs[0] {
		t.Fatalf("next %+v %v", j, ok)
	}
	for _, j := range jobs {
		p.res[j] = &graphResult{odds: balance.Odds{Battles: graphTries}}
	}
	if !p.done() {
		t.Fatal("not done when everything is measured")
	}
	p.res[jobs[0]] = &graphResult{odds: balance.Odds{Battles: 3}}
	if p.done() {
		t.Fatal("done with a partial result")
	}
	if r := (&graphResult{odds: balance.Odds{Battles: 5}, wins: []int{1, 2}}); r.tries() != 7 {
		t.Fatalf("tries %d", r.tries())
	}
	if !strings.Contains(troopLabel(testgame.Load(t, nil), "two"), "2") {
		t.Fatal("label")
	}
}
