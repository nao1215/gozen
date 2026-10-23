package atlas

import (
	"math"
	"strconv"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

func TestRouteOverflow(t *testing.T) {
	a := &data.Atlas{Points: map[string]*data.MapPoint{"a": {}, "b": {}, "c": {}}, Roads: []data.Road{
		{Between: []string{"a", "b"}, Steps: math.MaxInt},
		{Between: []string{"b", "c"}, Steps: 1},
	}}
	r := findRoutes(&rpg.State{}, a, "a")
	if got := r.dist["b"]; got != math.MaxInt {
		t.Fatalf("distance: %d", got)
	}
	if _, ok := r.path("c"); ok {
		t.Fatal("overflow must not create a route")
	}
	if p, ok := r.path("a"); !ok || len(p) != 0 {
		t.Fatalf("start: %v %v", p, ok)
	}
	a.Roads = append(a.Roads, data.Road{Between: []string{"a", "c"}, Steps: 2})
	r = findRoutes(&rpg.State{}, a, "a")
	if r.dist["b"] != 3 {
		t.Fatal("an alternative representable route must win")
	}
}

func chain(n int) (*data.DB, *rpg.State) {
	a := &data.Atlas{Points: map[string]*data.MapPoint{}}
	for i := range n {
		id := strconv.Itoa(i)
		a.Points[id] = &data.MapPoint{}
		if i > 0 {
			a.Roads = append(a.Roads, data.Road{Between: []string{strconv.Itoa(i - 1), id}})
		}
	}
	a.Points[strconv.Itoa(n-1)].To = "destination"
	return &data.DB{Locations: map[string]*data.Location{"map": {Map: a}}}, &rpg.State{Location: "map", Spot: "0"}
}

func TestLargeMapChoices(t *testing.T) {
	db, st := chain(3000)
	c := Choices(db, st)
	if len(c) != 1 || len(c[0].Roads) != 2999 {
		t.Fatalf("choices: count=%d", len(c))
	}
	for i, r := range c[0].Roads {
		if i != r {
			t.Fatalf("road %d: %d", i, r)
		}
	}
	st.Steps = math.MaxInt
	Cross(db, st, &data.Road{Steps: math.MaxInt})
	if st.Steps != math.MaxInt {
		t.Fatal("step count wrapped")
	}
}

func BenchmarkLargeMapChoices(b *testing.B) {
	for _, n := range []int{100, 1000, 3000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			db, st := chain(n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				Choices(db, st)
			}
		})
	}
}
