package atlas_test

import (
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestRoutesAndPlace(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	st := testgame.State(db, 1)
	town := db.Locations["town"].Map
	routes := atlas.Routes(st, town, "out")
	// out -> square (road 4) -> shop (road 0); the gate is closed without the key
	if got := routes["shop"]; !slices.Equal(got, []int{4, 0}) {
		t.Fatalf("the way to the shop: %v", got)
	}
	if _, ok := routes["gate"]; ok {
		t.Fatal("a closed point is reached")
	}
	st.Items["key"] = 1
	if got := atlas.Routes(st, town, "out")["gate"]; !slices.Equal(got, []int{4, 1, 3}) {
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
