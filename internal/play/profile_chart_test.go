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
			x := size.X/2 + p.X - 600
			y := size.Y/2 + p.Y - 180
			if x < 50 || x > size.X-50 || y < 100 || y > size.Y-100 {
				t.Fatalf("%v outside %v", p, size)
			}
		}
	}
}

func TestChartNavigationAndHidden(t *testing.T) {
	g := bookGame(t, Options{Location: "camp"}, "")
	s := newProfiles(g, nil)
	points := chartLayout(s.ids, g.db.Profiles)
	if s.opened(g, "stranger") {
		t.Fatal("locked profile opened")
	}
	if s.chartEdgeVisible(g, "keeper", "stranger") { t.Fatal("locked relationship visible") }
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
	if !s.chartEdgeVisible(g, "keeper", "stranger") { t.Fatal("--dev did not reveal relationship") }
}
