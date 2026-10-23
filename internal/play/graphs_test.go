package play

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/headless/balance"
	"github.com/nao1215/gozen/internal/testgame"
)

func graphGame(t *testing.T) *Game {
	t.Helper()
	db := testgame.Load(t, nil)
	g := &Game{db: db, st: testgame.State(db, 5), opts: Options{Development: true}}
	g.st.Location = "field"
	g.st.Members()[0].HP = 3
	return g
}

func runGraphs(t *testing.T, g *Game) {
	t.Helper()
	for i := 0; i == 0 || !g.graphs.done(); i++ {
		if i > 100000 {
			t.Fatal("the panel never finished")
		}
		g.tick++
		g.updateGraphs()
	}
}

// The number of screens matches the number measured with the same function and seed as gozen sim battle --state (full recovery), --as-is (current HP) and --as-is --streak.
func TestGraphsMatchSim(t *testing.T) {
	g := graphGame(t)
	g.in.pressed[btnGraphs] = true
	g.updateGraphs()
	g.in.pressed[btnGraphs] = false
	if !g.graphs.on {
		t.Fatal("F7 did not open the panel")
	}
	runGraphs(t, g)
	p := &g.graphs
	if len(p.here) != 1 || p.here[0].Troop != "one" || !p.hurt || len(p.wanted()) != 3+graphLevels {
		t.Fatalf("here %+v wanted %+v", p.here, p.wanted())
	}
	save, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range p.wanted() {
		prep := balance.Prep{Level: j.level, Tree: true, AsIs: j.kind != graphHealed}
		start := balance.SavedStart(g.db, save, graphSeed, prep)
		got := p.res[j]
		if j.kind == graphRun {
			want, err := balance.Streak(g.db, j.troop, graphTries, graphTurns, graphStreak, start)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.wins, want) {
				t.Fatalf("streak %v want %v", got.wins, want)
			}
			continue
		}
		want, err := balance.Sim(g.db, j.troop, graphTries, graphTurns, start, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.odds, want) {
			t.Fatalf("%+v: got %+v want %+v", j, got.odds, want)
		}
	}
	healed, now := p.res[graphJob{troop: "one"}].odds, p.res[graphJob{troop: "one", kind: graphNow}].odds
	if reflect.DeepEqual(healed, now) {
		t.Fatal("the hurt party measured like the healed one")
	}
	if n, total := p.progress(); n != total {
		t.Fatalf("progress %d/%d", n, total)
	}
	if g.st.Members()[0].HP != 3 {
		t.Fatal("measuring changed the game state")
	}
}

// Re-measure everything on a data reload (the DB changes), a location move or a level-up; once HP changes settle,
// re-measure only for the current HP. At full health, the current-HP stage is not measured. Nothing is measured while closed.
func TestGraphsRemeasure(t *testing.T) {
	g := graphGame(t)
	g.graphs.on = true
	runGraphs(t, g)
	first := g.graphs.key
	g.db = testgame.Load(t, nil)
	g.tick = graphCheck * 10
	g.updateGraphs()
	if g.graphs.key == first || g.graphs.done() {
		t.Fatal("a reloaded DB kept the old numbers")
	}
	runGraphs(t, g)
	healed := g.graphs.res[graphJob{troop: "one"}]
	a := g.st.Members()[0]
	a.HP = 20
	g.tick = graphCheck * 20
	g.updateGraphs()
	if !g.graphs.done() {
		t.Fatal("remeasured before the HP settled")
	}
	g.push(&battleScene{troop: "one"})
	g.tick = graphCheck*20 + graphSettle
	g.updateGraphs()
	if !g.graphs.done() {
		t.Fatal("remeasured during a battle")
	}
	g.stack = nil
	g.tick = graphCheck*20 + graphSettle + graphCheck
	g.updateGraphs()
	if g.graphs.done() || g.graphs.res[graphJob{troop: "one"}] != healed {
		t.Fatal("a settled HP change did not remeasure only the current HP")
	}
	runGraphs(t, g)
	a.Recover()
	for i := range 6 {
		g.tick = graphCheck * (40 + i)
		g.updateGraphs()
	}
	if g.graphs.hurt || len(g.graphs.wanted()) != 2+graphLevels {
		t.Fatalf("healed party still measured as hurt: %+v", g.graphs.wanted())
	}
	runGraphs(t, g)
	g.st.Location = "camp"
	g.tick = graphCheck * 60
	g.updateGraphs()
	if len(g.graphs.here) != 0 || !g.graphs.done() {
		t.Fatalf("camp %+v", g.graphs.here)
	}
	a.GainExp(a.ExpFor(3) - a.Exp)
	g.st.Location = "field"
	g.tick = graphCheck * 70
	g.updateGraphs()
	if g.graphs.base != 3 {
		t.Fatalf("base level %d", g.graphs.base)
	}
	g.graphs.on = false
	n, _ := g.graphs.progress()
	g.tick = graphCheck * 80
	g.updateGraphs()
	if m, _ := g.graphs.progress(); m != n {
		t.Fatal("measured while closed")
	}
}

func TestGraphsAddTheBattleTroop(t *testing.T) {
	g := graphGame(t)
	g.graphs.on = true
	runGraphs(t, g)
	g.push(&battleScene{troop: "boss"})
	g.tick = graphCheck * 10
	g.updateGraphs()
	p := &g.graphs
	if p.fight != "boss" || p.here[0].Troop != "boss" || p.res[graphJob{troop: "one"}] == nil {
		t.Fatalf("fight %q here %+v", p.fight, p.here)
	}
	runGraphs(t, g)
	if o := p.res[graphJob{troop: "boss"}]; o == nil || o.odds.Battles != graphTries {
		t.Fatalf("boss %+v", o)
	}
	if troopLabel(g.db, "two") != "Mote×2" || troopLabel(g.db, "absent") != "absent" {
		t.Fatal(troopLabel(g.db, "two"))
	}
}

func TestScriptWaitGraphs(t *testing.T) {
	if _, err := parseScript("wait-graphs extra"); err == nil {
		t.Fatal("accepted arguments")
	}
	s, err := parseScript("wait-graphs\nshot complete\nquit")
	if err != nil {
		t.Fatal(err)
	}
	g := graphGame(t)
	g.graphs.on = true
	s.graphsReady = g.scriptGraphsReady
	in := &input{}
	s.step(in)
	if s.pc != 0 || s.shot != "" {
		t.Fatal("advanced before initialization")
	}
	g.updateGraphs()
	// Use an incomplete result even if a fast machine already finished this fixture.
	for _, result := range g.graphs.res {
		result.odds.Battles = 0
		result.wins = nil
	}
	s.step(in)
	if s.pc != 0 || s.shot != "" {
		t.Fatal("advanced during measurement")
	}
	runGraphs(t, g)
	s.step(in)
	s.step(in)
	if s.shot != "complete" || g.failure != nil {
		t.Fatalf("completion: shot %q, error %v", s.shot, g.failure)
	}
	for _, tc := range []struct {
		name    string
		dev, on bool
		err     string
	}{
		{"not development", false, true, ""},
		{"closed", true, false, ""},
		{"measurement error", true, true, "broken save"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g.opts.Development, g.graphs.on, g.graphs.err = tc.dev, tc.on, tc.err
			g.failure = nil
			if g.scriptGraphsReady() || g.failure == nil {
				t.Fatal("invalid panel accepted")
			}
		})
	}
}
