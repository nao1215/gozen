package atlas_test

import (
	"fmt"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestHereAndOpen(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	st.Location, st.Spot = "town", ""
	if got := atlas.Here(db, st); got != "square" {
		t.Errorf("a map with no spot starts at its start point, got %q", got)
	}
	st.Spot = "shop"
	if got := atlas.Here(db, st); got != "shop" {
		t.Errorf("standing at a point: %q", got)
	}
	st.Spot = "no-such-point"
	if got := atlas.Here(db, st); got != "square" {
		t.Errorf("a spot that is not on the map falls back to the start, got %q", got)
	}
	st.Location, st.Spot = "plaza", ""
	if got := atlas.Here(db, st); got != "gate" {
		t.Errorf("a stage starts at its start spot, got %q", got)
	}
	st.Location = "camp"
	if got := atlas.Here(db, st); got != "" {
		t.Errorf("a place that is neither: %q", got)
	}
	st.Location = "nowhere"
	if got := atlas.Here(db, st); got != "" {
		t.Errorf("an unknown place: %q", got)
	}
	town := db.Locations["town"].Map
	if atlas.Open(st, nil) || !atlas.Open(st, town.Points["square"]) || atlas.Open(st, town.Points["gate"]) {
		t.Error("a point is open when it exists and its when holds")
	}
	st.Items["key"] = 1
	if !atlas.Open(st, town.Points["gate"]) {
		t.Error("the gate opens with the key")
	}
}

func TestRoadsOpenAndOther(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	town := db.Locations["town"].Map
	road := &town.Roads[3] // fork - gate
	if atlas.Usable(st, town, road) {
		t.Error("a road to a closed point is closed")
	}
	st.Items["key"] = 1
	if !atlas.Usable(st, town, road) {
		t.Error("open ends make an open road")
	}
	if atlas.Usable(st, town, &data.Road{Between: []string{"fork"}}) || atlas.Other(&data.Road{Between: []string{"fork"}}, "fork") != "" {
		t.Error("a road needs two ends")
	}
	if atlas.Other(road, "fork") != "gate" || atlas.Other(road, "gate") != "fork" || atlas.Other(road, "stranger") != "fork" {
		t.Error("Other")
	}
	if allRoutes(st, town, "nowhere") != nil {
		t.Error("no routes from a point that does not exist")
	}
	if got := allRoutes(st, town, "fork"); len(got["fork"]) != 0 {
		t.Errorf("the way to where one stands is empty: %v", got["fork"])
	}
}

// TestGoAndReach: going to a point crosses each road, then stands on the point and runs its do and its move.
func TestGoAndReach(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	st.Location, st.Spot, st.Calm = "town", "", true
	choices := atlas.Choices(db, st)
	byPoint := map[string]atlas.Choice{}
	for _, c := range choices {
		byPoint[c.Point] = c
	}
	if _, ok := byPoint["gate"]; !ok || !byPoint["gate"].Locked {
		t.Fatalf("the closed gate is offered as locked: %+v", byPoint)
	}
	if _, ok := byPoint["fork"]; ok {
		t.Error("a point with neither a do nor a move is not worth choosing")
	}
	cmds := atlas.Go(db, st, byPoint["gate"])
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "The gate is shut." {
		t.Fatalf("a locked choice only says why: %+v", cmds)
	}
	sign := byPoint["sign"]
	cmds = atlas.Go(db, st, sign)
	if len(cmds) < 2 || cmds[len(cmds)-1].Reach != "sign" {
		t.Fatalf("roads, then the point: %+v", cmds)
	}
	for _, c := range cmds[:len(cmds)-1] {
		if c.Cross == "" {
			t.Errorf("each road before the point is crossed: %+v", c)
		}
	}
	// run it by hand
	steps := st.Steps
	for _, c := range cmds {
		switch {
		case c.Cross != "":
			atlas.CrossRoad(db, st, c.Cross)
		case c.Reach != "":
			run := atlas.Reach(db, st, c.Reach)
			if len(run) != 2 { // the sign's say and set
				t.Errorf("the sign's do: %+v", run)
			}
		}
	}
	if st.Steps <= steps {
		t.Error("crossing roads counts steps")
	}
	if st.Spot != "sign" || !atlas.Visited(db, st, "town", "sign") || !st.Done["town/sign"] {
		t.Errorf("spot %q done %v", st.Spot, st.Done)
	}
	if again := atlas.Reach(db, st, "sign"); len(again) != 0 {
		t.Errorf("a once point does not run twice: %+v", again)
	}
	// a point that leads on moves the party
	run := atlas.Reach(db, st, "shop")
	if len(run) != 1 || run[0].Move != "shop" {
		t.Errorf("a point with a destination: %+v", run)
	}
	// only passing through: no do, no move
	if run := atlas.Reach(db, st, "~shop"); run != nil {
		t.Errorf("passing: %+v", run)
	}
	// nonsense is ignored
	if atlas.Reach(db, st, "ghost") != nil || atlas.CrossRoad(db, st, "x>y") != nil || atlas.CrossRoad(db, st, "99>sign") != nil || atlas.CrossRoad(db, st, "-1>sign") != nil || atlas.CrossRoad(db, st, "nogreater") != nil {
		t.Error("bad arguments run nothing")
	}
	st.Location = "camp"
	if atlas.Go(db, st, byPoint["sign"]) != nil || atlas.Reach(db, st, "sign") != nil || atlas.CrossRoad(db, st, "0>sign") != nil || atlas.Choices(db, st) != nil {
		t.Error("a place with no map has no roads")
	}
	st.Location = "town"
	if atlas.Go(db, st, atlas.Choice{Point: "ghost"}) != nil {
		t.Error("going to a point that does not exist")
	}
}

func TestCrossingARoadMeetsBattlesOnlyWhenNotCalm(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	world := db.Locations["world"].Map
	road := &world.Roads[0] // town - ruins, encounters rate 1
	for _, calm := range []bool{true, false} {
		st := testgame.State(db, 5)
		st.Calm = calm
		got := atlas.Cross(db, st, road)
		battles := 0
		for _, c := range got {
			if c.Battle != "" {
				battles++
			}
		}
		if calm && battles != 0 {
			t.Error("a calm party meets no battles")
		}
		if !calm && battles != road.StepsOrOne() {
			t.Errorf("rate 1 meets a battle on each of the %d steps, got %d", road.StepsOrOne(), battles)
		}
		if st.Steps != road.StepsOrOne() {
			t.Errorf("steps %d", st.Steps)
		}
	}
}

func TestPickTroopWeightsAndConditions(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	picks := []data.TroopPick{{Troop: "one", Weight: 3}, {Troop: "two"}, {Troop: "boss", When: testgameWhen(t, "false")}}
	count := map[string]int{}
	for range 2000 {
		count[atlas.PickTroop(st, picks)]++
	}
	if count["boss"] != 0 || count[""] != 0 {
		t.Errorf("a troop whose when fails, or nothing, was drawn: %v", count)
	}
	if count["one"] < 2*count["two"] {
		t.Errorf("weights 3 and 1 (the default) give about 3:1, got %v", count)
	}
	if got := atlas.PickTroop(st, []data.TroopPick{{Troop: "boss", When: testgameWhen(t, "false")}}); got != "" {
		t.Errorf("nothing holds: %q", got)
	}
	if got := atlas.PickTroop(st, nil); got != "" {
		t.Errorf("no troops: %q", got)
	}
	if got := atlas.PickTroop(st, []data.TroopPick{{Troop: "one", Weight: -5}}); got != "one" {
		t.Errorf("a weight of 0 or less counts as 1: %q", got)
	}
}

func testgameWhen(t *testing.T, src string) data.Expr {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/w.yaml": fmt.Sprintf("events: {w: {when: %q, do: []}}", src)})
	return db.Events["w"].When
}

func TestVisitGrowsWithTheMap(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	st.Mapped = nil
	atlas.Visit(db, st, "town", "fork")
	if st.Mapped == nil || !atlas.Visited(db, st, "town", "fork") || atlas.Visited(db, st, "town", "square") {
		t.Fatalf("%v", st.Mapped)
	}
	st.Mapped["town"] = st.Mapped["town"][:2] // an older save with fewer points
	atlas.Visit(db, st, "town", "gate")
	if len(st.Mapped["town"]) != len(db.Locations["town"].Map.PointIDs()) || !atlas.Visited(db, st, "town", "gate") {
		t.Errorf("the record grows to the points now written: %v", st.Mapped["town"])
	}
	atlas.Visit(db, st, "town", "ghost")
	atlas.Visit(db, st, "camp", "square")
	atlas.Visit(db, st, "nowhere", "square")
	if atlas.Visited(db, st, "camp", "square") || atlas.Visited(db, st, "nowhere", "x") || atlas.Visited(db, st, "town", "ghost") {
		t.Error("visits to what does not exist are not recorded")
	}
	if _, ok := st.Mapped["camp"]; ok {
		t.Error("a place with no map has no record")
	}
}
