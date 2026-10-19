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

// 画面の数は gozen sim --state（全回復）・--as-is（今の HP）・--as-is --streak と同じ関数・同じ種で測った数と一致する。
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

// データの読み直し（DB が替わる）・場所の移動・Lv の上昇で全部を測り直し、HP の変化は落ち着いてから
// 今の HP の分だけ測り直す。全快なら今の HP の段を測らない。閉じている間は測らない。
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
