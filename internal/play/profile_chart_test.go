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

func TestChartFocusSetAndPositions(t *testing.T) {
	ids := []string{"ox", "outgoing", "incoming", "other", "you"}
	profiles := map[string]*data.Profile{
		"ox":       {Relations: []data.ProfileRelation{{Who: "outgoing"}, {Who: "you"}}},
		"outgoing": {},
		"incoming": {Relations: []data.ProfileRelation{{Who: "ox"}}},
		"other":    {},
	}
	set := chartFocusSet(ids, profiles, 0)
	if !reflect.DeepEqual(set, []bool{true, true, true, false, true}) {
		t.Fatalf("focus set = %v", set)
	}
	base := chartLayout(ids, profiles)
	if !reflect.DeepEqual(chartFocusPoints(base, set, 0, 0), base) {
		t.Fatal("animation does not start at full chart")
	}
	end := chartFocusPoints(base, set, 0, chartFocusFrames)
	if end[0] != (image.Point{}) || end[3] != base[3] {
		t.Fatalf("end positions = %v", end)
	}
	if chartOpacity(false, chartFocusFrames) > 16 || chartOpacity(true, chartFocusFrames) != 255 {
		t.Fatal("unrelated faces did not fade")
	}
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}} {
		for i, p := range end {
			if !set[i] {
				continue
			}
			x, y := size.X/2+p.X, (size.Y+int(profileBand)-50)/2+p.Y
			if x < 65 || x > size.X-65 || y < int(profileBand)+65 || y > size.Y-100 {
				t.Fatalf("focused %s at %v outside %v", ids[i], p, size)
			}
		}
	}
}

func TestChartNineNodesFitPanel(t *testing.T) {
	ids := []string{"ox", "a", "b", "c", "d", "e", "f", "g", "you"}
	profiles := map[string]*data.Profile{}
	for _, id := range ids[:8] {
		profiles[id] = &data.Profile{}
		if id != "ox" {
			profiles[id].Relations = []data.ProfileRelation{{Who: "ox"}}
		}
	}
	profiles["ox"].Relations = []data.ProfileRelation{{Who: "you"}}
	base := chartLayout(ids, profiles)
	set := chartFocusSet(ids, profiles, 0)
	for _, points := range [][]image.Point{base, chartFocusPoints(base, set, 0, chartFocusFrames)} {
		for _, size := range []image.Point{{1280, 720}, {1920, 1080}} {
			for i, p := range points {
				x, y := size.X/2+p.X, (size.Y+int(profileBand)-50)/2+p.Y
				if x < 65 || x > size.X-65 || y < int(profileBand)+65 || y > size.Y-100 {
					t.Fatalf("%s at %v outside %v", ids[i], p, size)
				}
			}
		}
	}
}

func TestChartFocusAndBack(t *testing.T) {
	g := bookGame(t, Options{Location: "camp", Development: true}, "")
	s := newProfiles(g, nil)
	s.chart = true
	g.in.pressed[btnOK] = true
	s.updateChart(g)
	if s.focus != s.sel || !s.chart {
		t.Fatalf("first confirm: focus=%d chart=%t", s.focus, s.chart)
	}
	g.in.pressed[btnOK] = false
	for range chartFocusFrames {
		s.updateChart(g)
	}
	if s.focusFrame != chartFocusFrames {
		t.Fatalf("focus stopped at %d", s.focusFrame)
	}
	g.in.pressed[btnCancel] = true
	s.updateChart(g)
	g.in.pressed[btnCancel] = false
	for range chartFocusFrames {
		s.updateChart(g)
	}
	if s.focus != -1 || s.focusFrame != 0 || !s.chart {
		t.Fatalf("return: focus=%d frame=%d chart=%t", s.focus, s.focusFrame, s.chart)
	}
	g.in.pressed[btnOK] = true
	s.updateChart(g)
	g.in.pressed[btnOK] = false
	for range chartFocusFrames {
		s.updateChart(g)
	}
	g.in.pressed[btnOK] = true
	s.updateChart(g)
	if s.chart || !s.chartPage {
		t.Fatal("second confirm did not open detail")
	}
	g.in.pressed[btnOK] = false
	g.in.pressed[btnCancel] = true
	s.update(g)
	if !s.chart || !s.focusBack {
		t.Fatal("back from detail did not start chart return")
	}
	g.in.pressed[btnCancel] = false
	for range chartFocusFrames {
		s.updateChart(g)
	}
	if s.focus != -1 || s.focusFrame != 0 {
		t.Fatalf("detail return: focus=%d frame=%d", s.focus, s.focusFrame)
	}
}

func TestChartRouteAvoidsUnrelatedFace(t *testing.T) {
	points := []image.Point{{-210, 0}, {210, 0}, {0, 0}}
	route := chartRoute(points, 0, 1)
	if len(route) != 3 || !reflect.DeepEqual(route, chartRoute(points, 0, 1)) {
		t.Fatalf("route is straight or unstable: %v", route)
	}
	for i := 0; i+1 < len(route); i++ {
		if d := chartSegmentDistance(route[i], route[i+1], points[2]); d < 54 {
			t.Fatalf("line crosses unrelated face: %v (distance %v)", route, d)
		}
	}
}
