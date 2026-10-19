package balance

import (
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestResourceReturnsAndMeasurement(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml": testgame.Game + "  before: prepare\n  after: finish\n",
		"data/hooks.yaml": `events:
  arrive:
    do: [{set: {var.energy: 2}}]
  prepare:
    do: [{set: {var.energy: "var.energy - 1", var.prepared: "var.prepared + 1"}}]
  finish:
    do:
      - set: {var.finished: "var.finished + 1"}
      - if: "var.energy == 0"
        then: [{move: camp}]
  rest:
    do: [{set: {var.energy: 2}}, {move: camp}]
`})
	cfg := Config{Location: "field", Floors: 1, Battles: 5, Tries: 3, Events: true, FloorEvent: "arrive", RestEvent: "rest"}
	result, err := Run(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Floors) != 1 {
		t.Fatalf("floors=%v", result.Floors)
	}
	f := result.Floors[0]
	if f.Battles != 5 || f.Backs != 2 || f.Lost != 0 || f.Stranded != 0 {
		t.Fatalf("counts=%+v", f)
	}
	st := testgame.State(db, 41)
	st.Location, st.Vars["energy"] = "field", 0.
	r := &runner{db: db, o: cfg, used: map[string]map[string]int{}, fl: &Floor{}}
	r.o.MaxTurns = 60
	r.rate(st, "one", 1)
	if r.err != nil || r.fl.Backs != 0 || st.Vars["energy"] != 0. || st.Vars["prepared"] != nil {
		t.Fatalf("measurement changed exploration: %v %+v %v", r.err, r.fl, st.Vars)
	}
}

func TestBeforeReturnResumesAndDefeatRestoresLocation(t *testing.T) {
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.Game + "  before: prepare\n", "data/hooks.yaml": `events:
  prepare:
    do:
      - if: "var.energy <= 0"
        then: [{move: camp}]
        else: [{set: {var.energy: "var.energy - 1"}}]
  rest:
    do: [{set: {var.energy: 3}}, {move: camp}]
  defeat:
    do: [{gold: -9}, {move: camp}]
`})
	db.Game.GameOver.Event = "defeat"
	st := testgame.State(db, 42)
	st.Location = "field"
	r := &runner{db: db, o: Config{Location: "field", Events: true, RestEvent: "rest", MaxTurns: 60}, used: map[string]map[string]int{}, fl: &Floor{}}
	r.fight(st, "one")
	if st.Location != "field" || st.Vars["energy"] != 2. || r.fl.Backs != 1 || r.fl.Stranded != 0 {
		t.Fatalf("return %+v %v", r.fl, st.Vars)
	}
	r.lost(st)
	if st.Location != "field" || st.Vars["energy"] != 3. || r.fl.GoldLost != 9 || r.err != nil {
		t.Fatalf("defeat %+v %v err=%v", r.fl, st.Vars, r.err)
	}
}
