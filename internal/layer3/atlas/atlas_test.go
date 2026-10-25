package atlas_test

import (
	"slices"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"

	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestRoutesAndPlace(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	town := db.Locations["town"].Map
	routes := allRoutes(st, town, "out")
	// out -> square (road 4) -> shop (road 0); the gate is closed without the key
	if got := routes["shop"]; !slices.Equal(got, []int{4, 0}) {
		t.Fatalf("the way to the shop: %v", got)
	}
	if _, ok := routes["gate"]; ok {
		t.Fatal("a closed point is reached")
	}
	st.Items["key"] = 1
	if got := allRoutes(st, town, "out")["gate"]; !slices.Equal(got, []int{4, 1, 3}) {
		t.Fatalf("the way to the gate: %v", got)
	}
	// arriving: the named point, else the point back to where one came from, else the start
	for _, c := range []struct{ from, at, want string }{
		{"plaza", "", "square"}, {"shop", "", "shop"}, {"world", "", "out"}, {"camp", "", "square"}, {"plaza", "fork", "fork"},
	} {
		st.Location = c.from
		atlas.Place(db, st, c.from, "town", c.at)
		if st.Spot != c.want {
			t.Errorf("from %s at %q: on %s, want %s", c.from, c.at, st.Spot, c.want)
		}
		if !atlas.Visited(db, st, "town", c.want) {
			t.Errorf("%s is not marked visited", c.want)
		}
	}
	atlas.Place(db, st, "town", "plaza", "north")
	if st.Spot != "north" {
		t.Fatalf("plaza at north: %s", st.Spot)
	}
	atlas.Place(db, st, "town", "plaza", "")
	if st.Spot != "gate" {
		t.Fatalf("plaza without a spot: %s", st.Spot)
	}
	atlas.Place(db, st, "plaza", "shop", "")
	if st.Spot != "" {
		t.Fatalf("a location without spots: %q", st.Spot)
	}
}

// allRoutes makes every point a destination to exercise the routes offered to consumers.
func allRoutes(st *rpg.State, a *data.Atlas, from string) map[string][]int {
	if a.Points[from] == nil {
		return nil
	}
	copyAtlas := *a
	copyAtlas.Points = make(map[string]*data.MapPoint, len(a.Points))
	for id, p := range a.Points {
		cp := *p
		cp.To = "destination"
		cp.Locked = ""
		copyAtlas.Points[id] = &cp
	}
	copyState := *st
	copyState.Location, copyState.Spot = "map", from
	db := &data.DB{Locations: map[string]*data.Location{"map": {Map: &copyAtlas}}}
	out := map[string][]int{}
	for _, c := range atlas.Choices(db, &copyState) {
		out[c.Point] = c.Roads
	}
	return out
}

func TestHideClosedChoices(t *testing.T) {
	for _, activation := range []string{"travel", "instant"} {
		db := testgame.LoadScenes(t, map[string]string{"data/menu.yaml": testgame.AtlasMenuData, "assets/maps/menu.png": string(testgame.Picture(64, 36))})
		st := testgame.State(db, 1)
		st.Location, st.Spot = "selection", "town"
		a := db.Locations[st.Location].Map
		a.Activation = activation
		p := a.Points["tower"]
		offered := func() bool {
			for _, c := range atlas.Choices(db, st) {
				if c.Point == "tower" {
					return true
				}
			}
			return false
		}
		if !offered() {
			t.Fatalf("%s: default locked point missing", activation)
		}
		p.Hide = "closed"
		if offered() {
			t.Fatalf("%s: hidden point offered", activation)
		}
		st.Flags["heard"] = true
		if !offered() {
			t.Fatalf("%s: opened point missing", activation)
		}
		st.Flags["heard"] = false
		if offered() {
			t.Fatalf("%s: reclosed point offered", activation)
		}
	}
}
