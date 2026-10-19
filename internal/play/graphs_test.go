package play

import (
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
	for i := 0; !g.graphs.done(); i++ {
		if i > 100000 {
			t.Fatal("the panel never finished")
		}
		g.tick++
		g.updateGraphs()
	}
}

// 画面の数は gozen sim -state と同じ関数・同じ種で測った数と一致する。
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
	if len(p.here) != 1 || p.here[0].Troop != "one" || len(p.jobs) != 1+graphLevels {
		t.Fatalf("here %+v jobs %+v", p.here, p.jobs)
	}
	save, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range p.jobs {
		want, err := balance.Sim(g.db, j.troop, graphTries, graphTurns, balance.SavedStart(g.db, save, graphSeed, balance.Prep{Level: j.level, Tree: true}), nil)
		if err != nil {
			t.Fatal(err)
		}
		got := p.res[j]
		if got == nil || got.Won != want.Won || got.Battles != want.Battles || got.Turns != want.Turns || len(got.HP) != len(want.HP) {
			t.Fatalf("%+v: got %+v want %+v", j, got, want)
		}
	}
	if p.n != p.total() {
		t.Fatalf("progress %d/%d", p.n, p.total())
	}
	if g.st.Members()[0].HP != 3 {
		t.Fatal("measuring changed the game state")
	}
}

// データの読み直し（DB が替わる）・場所の移動・Lv の上昇で測り直し、閉じている間は測らない。
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
	g.st.Location = "camp"
	g.tick = graphCheck * 20
	g.updateGraphs()
	if len(g.graphs.here) != 0 || !g.graphs.done() {
		t.Fatalf("camp %+v", g.graphs.here)
	}
	a := g.st.Members()[0]
	a.GainExp(a.ExpFor(3) - a.Exp)
	g.st.Location = "field"
	g.tick = graphCheck * 30
	g.updateGraphs()
	if g.graphs.base != 3 {
		t.Fatalf("base level %d", g.graphs.base)
	}
	g.graphs.on = false
	n := g.graphs.n
	g.tick = graphCheck * 40
	g.updateGraphs()
	if g.graphs.n != n {
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
	if o := p.res[graphJob{troop: "boss"}]; o == nil || o.Battles != graphTries {
		t.Fatalf("boss %+v", o)
	}
	if troopLabel(g.db, "two") != "Mote×2" || troopLabel(g.db, "absent") != "absent" {
		t.Fatal(troopLabel(g.db, "two"))
	}
}
