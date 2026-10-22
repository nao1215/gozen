package play

import (
	"image"
	"reflect"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestChartLayoutStableAndSeparated(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f", "g"}
	profiles := map[string]*data.Profile{}
	for _, id := range ids {
		profiles[id] = &data.Profile{}
	}
	profiles["f"].Affiliation = "Guild"
	profiles["g"].Affiliation = "Guild"
	points := chartLayout(ids, profiles)
	if !reflect.DeepEqual(points, chartLayout(ids, profiles)) {
		t.Fatal("unstable layout")
	}
	for i, p := range points {
		for j := i + 1; j < len(points); j++ {
			q := points[j]
			if absInt(p.X-q.X) < 100 && absInt(p.Y-q.Y) < 120 {
				t.Fatalf("overlap %v %v", p, q)
			}
		}
	}
	for _, size := range []image.Point{{X: 1280, Y: 720}, {X: 1920, Y: 1080}} {
		// The initial three-person chart fits inside either supported display size.
		for _, p := range chartLayout(ids[:3], profiles) {
			x := size.X/2 + p.X
			y := (size.Y+profileBand-50)/2 + p.Y
			if x < 50 || x > size.X-50 || y < 100 || y > size.Y-100 {
				t.Fatalf("%v outside %v", p, size)
			}
		}
	}
}

func TestChartPlayerAndCompactRows(t *testing.T) {
	ids := []string{"maren", "ox", "tilly", "kes", "pomm", "vance"}
	profiles := map[string]*data.Profile{}
	for _, id := range ids {
		profiles[id] = &data.Profile{}
	}
	profiles["maren"].Relations = []data.ProfileRelation{{Who: "you"}}
	points := chartPoints(ids, profiles)
	if len(points) != len(ids)+1 || !reflect.DeepEqual(points, chartPoints(ids, profiles)) {
		t.Fatal("player node missing or layout unstable")
	}
	for i, p := range points {
		for j := i + 1; j < len(points); j++ {
			q := points[j]
			if absInt(p.X-q.X) < 100 && absInt(p.Y-q.Y) < 120 {
				t.Fatalf("overlap %v %v", p, q)
			}
		}
	}
	s := &profileScreen{ids: ids, sel: len(ids) - 1}
	for _, dir := range []image.Point{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		s.chartMove(points, dir.X, dir.Y)
		if s.sel >= len(ids) {
			t.Fatal("player node became selectable")
		}
	}
}

func TestChartLayoutReducesCrossings(t *testing.T) {
	ids := []string{"a", "b", "c", "d"}
	profiles := map[string]*data.Profile{}
	for _, id := range ids {
		profiles[id] = &data.Profile{Affiliation: "Party"}
	}
	profiles["a"].Relations = []data.ProfileRelation{{Who: "d"}}
	profiles["b"].Relations = []data.ProfileRelation{{Who: "c"}}
	if got := chartCrossings(ids, profiles, chartLayout(ids, profiles)); got != 0 {
		t.Fatalf("avoidable crossing remains: %d", got)
	}
}

func TestChartNavigationAndHidden(t *testing.T) {
	g := bookGame(t, Options{Location: "camp"}, "")
	s := newProfiles(g, nil)
	points := chartLayout(s.ids, g.db.Profiles)
	if s.opened(g, "stranger") {
		t.Fatal("locked profile opened")
	}
	if s.chartEdgeVisible(g, "keeper", "stranger") {
		t.Fatal("locked relationship visible")
	}
	s.chartMove(points, 1, 0)
	if s.current() != "stranger" {
		t.Fatalf("selected %q", s.current())
	}
	if s.opened(g, s.current()) {
		t.Fatal("locked profile became selectable")
	}
	g.opts.Development = true
	if !s.opened(g, s.current()) {
		t.Fatal("--dev did not reveal profile")
	}
	if !s.chartEdgeVisible(g, "keeper", "stranger") {
		t.Fatal("--dev did not reveal relationship")
	}
}
