package designeval

import (
	"math/rand/v2"
	"testing"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

func returnBot(t *testing.T) *bot {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/room.yaml": `locations:
  room:
    name: Room
    dungeon:
      map: |
        #######
        #SG.H.#
        #######
      tiles:
        S: {start: true, facing: east}
        G: {do: [{set: {var.guards: "var.guards + 1"}}, {hurt: {who: all, amount: 30}}]}
        H: {do: [{set: {flag.gate: true}}, {move: camp}]}
events:
  home: {do: [{set: {flag.returned: true}}, {move: camp}]}
`})
	st := testgame.State(db, 51)
	st.Location = "room"
	dungeon.Ensure(db, st)
	b := &bot{db: db, st: st, d: headless.New(db, st), c: &Config{}, run: &Run{}, pcg: rand.NewPCG(51, 1), errs: map[string]bool{}, know: map[string]map[string]bool{}, seen: map[string]bool{}, prev: map[*Rule]float64{}, used: map[string]int{}, rewardCounts: map[string]int{}}
	b.c.Game = Game{Dungeon: "room", Gate: "room", BackEvent: "home", GateTile: "H", Floor: data.Expr{Expr: testgame.Expr("1")}, Deepest: data.Expr{Expr: testgame.Expr("1")}, WornHP: 0.5}
	b.prime()
	return b
}

func TestReturnCostAndGatePreference(t *testing.T) {
	for _, tc := range []struct {
		name, item      string
		count, stranded int
		gate            bool
	}{
		{name: "free return skips gate"},
		{name: "paid return uses gate", item: "key", count: 1, gate: true},
		{name: "paid return consumes item", item: "key", count: 1},
		{name: "missing item counts stranded", item: "key", stranded: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := returnBot(t)
			b.db.Locations["room"].Dungeon.Tiles["G"].Do = nil
			b.c.Game.BackItem = tc.item
			if tc.item != "" {
				b.st.Items[tc.item] = tc.count
			}
			if tc.item != "" && !tc.gate {
				b.c.Game.GateTile = ""
			}
			b.goHome()
			if b.st.Location != "camp" || b.run.Stranded != tc.stranded || b.st.Flags["gate"] != tc.gate || len(b.run.Problems) > 0 {
				t.Fatalf("return loc=%s flags=%v result=%+v", b.st.Location, b.st.Flags, b.run)
			}
			want := 0
			if tc.gate {
				want = tc.count
			}
			if tc.item != "" && b.st.ItemCount(tc.item, false) != want {
				t.Fatalf("item count=%d", b.st.ItemCount(tc.item, false))
			}
		})
	}
}

func TestExhaustionStopsGuardAttempts(t *testing.T) {
	b := returnBot(t)
	b.c.Game.GuardTile = "G"
	b.db.Locations["room"].Dungeon.Tiles["G"].Solid = data.Expr{Expr: testgame.Expr("true")}
	b.db.Locations["room"].Dungeon.Tiles["G"].Trigger = "bump"
	b.dive()
	if b.st.Vars["guards"] != 1. || b.st.Location != "camp" || b.run.Stranded != 0 || len(b.run.Stuck) != 0 || len(b.run.Problems) != 0 {
		t.Fatalf("vars=%v location=%s run=%+v", b.st.Vars, b.st.Location, b.run)
	}
}
