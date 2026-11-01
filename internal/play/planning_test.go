package play

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestNativeViewport(t *testing.T) {
	for _, tc := range []struct {
		w, h, dsf float64
		pw, ph    int
		k         float64
	}{{1920, 1080, 1, 1920, 1080, 1.5}, {3840, 2160, 1, 3840, 2160, 3}, {2560, 1080, 1, 1920, 1080, 1.5}, {1280, 720, 1.5, 1920, 1080, 1.5}, {1920, 1080, 2, 3840, 2160, 3}} {
		v := nativeView(tc.w, tc.h, tc.dsf)
		if v.w != tc.pw || v.h != tc.ph || v.ui != tc.k {
			t.Fatalf("%+v: %+v", tc, v)
		}
	}
	if nativeView(0, 720, 1) != (gameView{}) {
		t.Fatal("invalid window accepted")
	}
}

func TestPlanningCellsStayInsideAndDisjoint(t *testing.T) {
	for _, count := range []int{1, 8, 9, 16} {
		for _, cols := range []int{1, 4} {
			cells := planningCells(24, 148, 1232, 452, count, cols)
			for i, r := range cells {
				if r.Min.X < 24 || r.Min.Y < 148 || r.Max.X > 1256 || r.Max.Y > 600 || r.Empty() {
					t.Fatalf("outside: %v", r)
				}
				for _, other := range cells[:i] {
					if r.Overlaps(other) {
						t.Fatalf("overlap: %v %v", r, other)
					}
				}
			}
		}
	}
	// Eight member rows reserve a gauge's text, bar and threshold labels.
	for _, r := range planningCells(24, 148, 1232, 502, 8, 1) {
		if r.Dy() < 50 {
			t.Fatalf("member row cannot fit labels: %v", r)
		}
	}
}

func TestPlanningSeedDerivation(t *testing.T) {
	g := &Game{opts: Options{Seed: 17}}
	baseline := g.newPCG().Uint64()
	g.db = &data.DB{}
	xor := uint64(12345)
	g.db.Game.Planning = &data.Planning{SeedXor: &xor}
	want := rand.NewPCG(17, 17^xor)
	got := g.newPCG()
	for range 20 {
		if got.Uint64() != want.Uint64() {
			t.Fatal("imported stream diverged")
		}
	}
	if g.newPCG().Uint64() == baseline {
		t.Fatal("stream setting ignored")
	}
}

func TestEnsembleCellsLeaveDialogueClear(t *testing.T) {
	// The box dialogue starts at 470; the view reserves its heading and margins.
	cells := planningCells(36, 76, 1208, 344, 8, 4)
	for _, r := range cells {
		if r.Max.Y >= 446 || r.Dy() < 100 {
			t.Fatalf("participant overlaps dialogue or loses name: %v", r)
		}
	}
}

func TestPlanningAuthoredDateLabels(t *testing.T) {
	v := data.PlanningView{DateFormat: "1/2({weekday})", Weekdays: []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}, Phases: map[string]string{"dawn": "Early"}}
	if got := planningDate("2030-04-01", "dawn", v); got != "4/1(Mo) Early" {
		t.Fatal(got)
	}
	if got := planningDate("2030-04-01", "", v); got != "4/1(Mo)" {
		t.Fatal(got)
	}
	if got := planningEventLabel(data.PlanningView{}); got != "events" {
		t.Fatal(got)
	}
}

func TestTimelineScrollReachesLastTask(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Activities: map[string]data.Activity{}}
	for i := range 28 {
		db.Game.Planning.Activities[strconv.Itoa(i)] = data.Activity{Effort: 1}
	}
	db.Game.Screens = map[string]*data.AuthorScreen{"tasks": {
		Kind: "timeline", View: data.PlanningView{Fullscreen: true},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "tasks", nil)
	g.in.wheel = -1
	for range 40 {
		s.updateSurface(g)
	}
	_, sh := g.screen()
	if s.timelineTop+timelineRows(sh-250) != 28 {
		t.Fatalf("last task cannot be reached: %d", s.timelineTop)
	}
	s.refresh(g)
	if s.timelineTop == 0 {
		t.Fatal("refresh lost scroll position")
	}
	g.in.wheel = 1
	for range 40 {
		s.updateSurface(g)
	}
	if s.timelineTop != 0 {
		t.Fatal("cannot return to first task")
	}
}

func TestResidentVisibilityUsesPublicState(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"potter": {}, "weaver": {}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Entities["potter"].Observations["shown"] = true
	g.st.Planning.Entities["weaver"].Attributes["secret"] = true
	db.Game.Planning.Attributes = map[string]data.Attribute{"secret": {Hidden: true}}
	if got := g.visibleResidents(data.PlanningView{Visible: "secret"}); len(got) != 0 {
		t.Fatal(got)
	}
	v := data.PlanningView{Order: []string{"weaver", "potter"}, Visible: "shown"}
	if got := g.visibleResidents(v); !slices.Equal(got, []string{"potter"}) {
		t.Fatal(got)
	}
	g.st.Planning.Entities["weaver"].Observations["shown"] = true
	if got := g.visibleResidents(v); !slices.Equal(got, v.Order) {
		t.Fatal(got)
	}
	delete(g.st.Planning.Entities["potter"].Observations, "shown")
	if got := g.visibleResidents(v); !slices.Equal(got, []string{"weaver"}) {
		t.Fatal(got)
	}
	v.Visible = ""
	if got := g.visibleResidents(v); !slices.Equal(got, v.Order) {
		t.Fatal(got)
	}
}

func TestGroupedTimelineCalendarFitAndPrivacy(t *testing.T) {
	db := testgame.Load(t, nil)
	p := &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}}, Activities: map[string]data.Activity{}}
	v := data.PlanningView{Fullscreen: true}
	for i := range 12 {
		row := data.TimelineRow{Label: strconv.Itoa(i)}
		for j := range 3 {
			id := strconv.Itoa(i*3 + j)
			p.Activities[id] = data.Activity{Name: id, Effort: 10, From: p.Calendar.Start, To: p.Calendar.End}
			row.Items = append(row.Items, data.TimelineItem{Activity: id, Label: id})
		}
		v.Rows = append(v.Rows, row)
	}
	db.Game.Planning = p
	db.Game.Screens = map[string]*data.AuthorScreen{"tasks": {Kind: "timeline", View: v}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Date = "2030-09-01"
	s := newAuthorScreen(g, "tasks", nil)
	groups, ax, aw := s.groupedTimeline(g, 0, 108, 1280, 494)
	if len(groups) != 12 {
		t.Fatalf("only %d groups fit", len(groups))
	}
	for _, group := range groups {
		if group.y+group.height > 602 {
			t.Fatal("group overlaps footer")
		}
		for i, lane := range group.lanes {
			if float64(lane.bounds.Min.X) < ax || float64(lane.bounds.Max.X) > ax+aw {
				t.Fatal("bar outside fixed calendar")
			}
			if lane.reported != 0 || lane.expected <= 0 {
				t.Fatal("zero-report delay lost")
			}
			for _, other := range group.lanes[:i] {
				if lane.bounds.Overlaps(other.bounds) {
					t.Fatal("lanes overlap")
				}
			}
		}
	}
	before := s.timelineDescription(g)
	a := g.st.Planning.Activities["0"]
	a.Progress = 9
	g.st.Planning.Activities["0"] = a
	if got := s.timelineDescription(g); got != before {
		t.Fatalf("hidden progress leaked: %s", got)
	}
	// Pointer and keyboard selection share the rendered lane rectangles.
	db.Game.Screens["detail"] = &data.AuthorScreen{Title: "Detail", Entries: []data.ScreenEntry{{Label: "Inspect"}}}
	db.Game.Screens["tasks"].View.Rows[0].Items[0].Screen = "detail"
	g.in.click = true
	g.in.mouse = groups[0].lanes[0].bounds.Min
	s.updateSurface(g)
	if len(g.stack) != 1 {
		t.Fatal("bar did not open detail")
	}
	if top, ok := g.stack[0].(*authorScreen); !ok || top.name != "detail" {
		t.Fatal("wrong detail screen")
	}
	g.in.click = false
	g.in.wheel = -1
	s.updateSurface(g)
	if s.timelineFocus != 1 || s.timelineTop != 0 {
		t.Fatal("keyboard did not select next lane")
	}
	s.refresh(g)
	if s.timelineFocus != 1 {
		t.Fatal("refresh lost selected lane")
	}
	// Live deadlines clip to the fixed calendar, not a different axis per row.
	a.Deadline = "2031-01-01"
	g.st.Planning.Activities["0"] = a
	groups, _, _ = s.groupedTimeline(g, 0, 108, 1280, 494)
	if groups[0].lanes[0].right != 1 {
		t.Fatal("extended deadline is not clipped")
	}
	p.Activities["extra"] = data.Activity{Name: "Extra", Effort: 1}
	rows := timelineGroups(p, v)
	if len(rows) != 13 || rows[12].Items[0].Activity != "extra" {
		t.Fatal("unlisted task disappeared")
	}
}
