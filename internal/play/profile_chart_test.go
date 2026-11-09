package play

import (
	"fmt"
	"image"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
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
	if got := chartCrossings(chartEdges(ids, profiles), chartLayout(ids, profiles)); got != 0 {
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

func TestChartPointerZoomAndClose(t *testing.T) {
	g := bookGame(t, Options{Location: "camp"}, "")
	closed := 0
	s := newProfiles(g, func() { closed++ })
	s.chart = true
	g.push(s)
	depth := len(g.stack)
	// Drawing every animation stage must not consume simulation randomness.
	before, err := g.pcg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	dst := covScreen()
	s.drawChart(g, dst)
	points := chartPoints(s.ids, g.db.Profiles)
	x, y := s.chartPoint(g, points[1])
	g.in = input{click: true, mouse: image.Pt(int(x), int(y))}
	s.updateChart(g)
	if s.focus != -1 {
		t.Fatal("pointer opened a hidden profile")
	}
	for _, tc := range []struct{ wheel, want float64 }{{1, 1.8}, {-1, .55}} {
		g.in = input{wheel: tc.wheel}
		for range 30 {
			s.updateChart(g)
		}
		if s.zoom != tc.want {
			t.Fatalf("zoom = %v, want %v", s.zoom, tc.want)
		}
	}
	x, y = s.chartPoint(g, points[0])
	g.in = input{click: true, mouse: image.Pt(int(x), int(y))}
	s.updateChart(g)
	if s.focus != 0 {
		t.Fatalf("visible pointer target did not focus: %d", s.focus)
	}
	g.in = input{}
	for range chartFocusFrames {
		s.updateChart(g)
		s.drawChart(g, dst)
	}
	shown, _ := s.chartDisplay(g, points)
	x, y = s.chartPoint(g, shown[0])
	g.in = input{click: true, mouse: image.Pt(int(x), int(y))}
	s.updateChart(g)
	if s.chart || !s.chartPage || len(g.stack) != depth || closed != 0 {
		t.Fatal("second click did not enter detail in place")
	}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	s.update(g)
	g.in = input{}
	for range chartFocusFrames {
		s.updateChart(g)
		s.drawChart(g, dst)
	}
	if !s.chart || s.focus != -1 {
		t.Fatal("detail cancel did not restore overview")
	}
	after, err := g.pcg.MarshalBinary()
	if err != nil || string(before) != string(after) {
		t.Fatal("chart browsing consumed simulation randomness", err)
	}
	g.in = input{pressed: [numButtons]bool{btnBook: true}}
	s.updateChart(g)
	if len(g.stack) != depth-1 || closed != 1 {
		t.Fatal("book shortcut did not close exactly once")
	}
}

func BenchmarkChartLayout(b *testing.B) {
	for _, count := range []int{4, 8, 10, 12} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			ids := make([]string, count)
			profiles := map[string]*data.Profile{}
			for i := range ids {
				ids[i] = fmt.Sprintf("person-%d", i)
			}
			for i, id := range ids {
				profiles[id] = &data.Profile{Affiliation: "Guild", Relations: []data.ProfileRelation{{Who: ids[(i+3)%count]}, {Who: ids[(i+5)%count]}}}
			}
			b.ReportAllocs()
			for b.Loop() {
				chartLayout(ids, profiles)
			}
		})
	}
}

func TestChartLayoutPreservesGraph(t *testing.T) {
	rng := rand.New(rand.NewPCG(71, 29)) //nolint:gosec // Reproducible graph inputs, not security randomness.
	for count := 1; count <= 14; count++ {
		for trial := range 20 {
			ids := make([]string, count)
			profiles := map[string]*data.Profile{}
			for i := range ids {
				ids[i] = strconv.Itoa(i)
			}
			for i, id := range ids {
				profiles[id] = &data.Profile{Affiliation: strconv.Itoa(i % 3)}
				for range 3 {
					profiles[id].Relations = append(profiles[id].Relations, data.ProfileRelation{Who: ids[rng.IntN(count)]})
				}
			}
			base := chartLayout(ids, nil)
			got := chartLayout(ids, profiles)
			if !slices.Equal(got, chartLayout(ids, profiles)) {
				t.Fatalf("n=%d trial=%d: nondeterministic layout", count, trial)
			}
			edges := chartEdges(ids, profiles)
			if chartCrossings(edges, got) > chartCrossings(edges, base) {
				t.Fatalf("n=%d trial=%d: optimization increased crossings", count, trial)
			}
			used := map[image.Point]bool{}
			for i, point := range got {
				original := slices.Index(base, point)
				if original < 0 || used[point] || profiles[ids[i]].Affiliation != profiles[ids[original]].Affiliation {
					t.Fatalf("n=%d trial=%d: layout lost a slot or crossed affiliations: %v", count, trial, got)
				}
				used[point] = true
			}
			if count > 10 && !slices.Equal(got, base) {
				t.Fatal("large chart exceeded its bounded layout search")
			}
		}
	}
}

func TestChartEdgesIgnoreMissingAndSelfRelations(t *testing.T) {
	ids := []string{"first", "missing", "last"}
	profiles := map[string]*data.Profile{
		"first": {Relations: []data.ProfileRelation{{Who: "first"}, {Who: "outside"}, {Who: "last"}}},
		"last":  {Relations: []data.ProfileRelation{{Who: "first"}}},
	}
	if got := chartEdges(ids, profiles); !slices.Equal(got, []chartEdge{{0, 2}, {2, 0}}) {
		t.Fatalf("resolved edges: %v", got)
	}
}
