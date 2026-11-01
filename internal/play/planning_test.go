package play

import (
	"encoding/json"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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
		row := data.TimelineRow{Label: strconv.Itoa(i), Text: "Inspected {var.inspected}"}
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
	g.st.Vars["inspected"] = 3.0
	s := newAuthorScreen(g, "tasks", nil)
	groups, ax, aw := s.groupedTimeline(g, 0, 108, 1280, 494)
	if len(groups) != 12 {
		t.Fatalf("only %d groups fit", len(groups))
	}
	for _, group := range groups {
		if group.text != "Inspected 3" {
			t.Fatalf("row summary: %q", group.text)
		}
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
	a := g.st.Planning.Activities["0"]
	a.Observed = 5
	g.st.Planning.Activities["0"] = a
	db.Game.Screens["tasks"].View.Rows[0].Items[0].ForecastLabel = "Reported pace"
	before := s.timelineDescription(g)
	if !strings.Contains(before, "Reported pace") {
		t.Fatal(before)
	}
	a.Progress = 9
	g.st.Planning.Activities["0"] = a
	if got := s.timelineDescription(g); got != before {
		t.Fatalf("hidden progress leaked: %s", got)
	}
	// A stored sample stays fixed while hidden progress, effort and dates change.
	item := &db.Game.Screens["tasks"].View.Rows[0].Items[0]
	item.Sample = data.MustExpr("var.sample")
	item.SampleLabel = "Checked {var.sample_date}"
	g.st.Vars["sample"] = .4
	g.st.Vars["sample_date"] = "2030-08-20"
	before = s.timelineDescription(g)
	if !strings.Contains(before, "Checked 2030-08-20 40%") {
		t.Fatal(before)
	}
	a.Progress = 2
	g.st.Planning.Activities["0"] = a
	if got := s.timelineDescription(g); got != before {
		t.Fatal("live progress replaced saved sample", got)
	}
	for _, value := range []any{-1.0, 1.1, "unknown", nil} {
		g.st.Vars["sample"] = value
		if strings.Contains(s.timelineDescription(g), "Checked") {
			t.Fatal("invalid sample displayed", value)
		}
	}
	g.st.Vars["sample"] = 0.0
	if !strings.Contains(s.timelineDescription(g), "Checked 2030-08-20 0%") {
		t.Fatal("zero sample lost")
	}
	// Aging changes only the marker tint, not its historical position or value.
	item.SampleFreshness = data.MustExpr("var.freshness")
	item.SampleLabel = "Checked {var.sample_date} ({var.age} days ago)"
	g.st.Vars["age"] = 14.0
	for _, tc := range []struct {
		value any
		want  float64
	}{
		{1.0, 1},
		{.5, .5},
		{0.0, 0},
		{-1.0, 0},
		{2.0, 1},
		{"unknown", 0},
		{nil, 0},
		{math.NaN(), 0},
		{math.Inf(1), 0},
	} {
		g.st.Vars["freshness"] = tc.value
		lane := s.timelineLanes(g)[0]
		if lane.sampleFreshness != tc.want || lane.sample != 0 {
			t.Fatalf("freshness %v: %+v", tc.value, lane)
		}
	}
	if !strings.Contains(s.timelineDescription(g), "14 days ago") {
		t.Fatal("age explanation lost")
	}
	item.SampleFreshness = data.Expr{}
	if s.timelineLanes(g)[0].sampleFreshness != 1 {
		t.Fatal("old author data no longer uses full brightness")
	}
	fresh, old := color.NRGBA{240, 240, 240, 255}, color.NRGBA{160, 160, 160, 255}
	if timelineSampleTint(fresh, old, 0) != old || timelineSampleTint(fresh, old, 1) != fresh || timelineSampleTint(fresh, old, .5).R != 200 {
		t.Fatal("historical marker tint must remain readable at every age")
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

func TestEntityTableInlineDetailAndNavigation(t *testing.T) {
	db := testgame.Load(t, nil)
	p := &data.Planning{Entities: map[string]data.Entity{}}
	order := []string{}
	for i := range 12 {
		id := strconv.Itoa(i)
		order = append(order, id)
		p.Entities[id] = data.Entity{Name: id}
	}
	db.Game.Planning = p
	db.Game.Screens = map[string]*data.AuthorScreen{"roster": {Kind: "entities", View: data.PlanningView{Fullscreen: true, Order: order, Detail: "record"}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "roster", nil)
	ids, cells, detail := s.entityTableLayout(g)
	if len(ids) < 8 || !detail.Empty() {
		t.Fatalf("initial layout: %v %v", ids, detail)
	}
	_, sh := g.screen()
	for _, r := range cells {
		if r.Min.X != 16 || r.Dx() < 1000 || r.Max.Y > int(sh)-112 {
			t.Fatalf("table does not fit: %v", r)
		}
	}
	// Open the last visible row: its detail must be fully revealed, without
	// consuming a phase or obscuring any other row.
	g.in.click = true
	g.in.mouse = cells[len(cells)-1].Min.Add(image.Pt(8, 8))
	s.updateEntityTable(g)
	if s.target != ids[len(ids)-1] {
		t.Fatal("wrong row opened")
	}
	_, expanded, detail := s.entityTableLayout(g)
	if detail.Dy() != 180 {
		t.Fatalf("detail clipped: %v", detail)
	}
	for _, r := range expanded {
		if r.Overlaps(detail) {
			t.Fatal("detail covers a row")
		}
	}
	g.in.mouse = expanded[0].Min.Add(image.Pt(8, 8))
	s.updateEntityTable(g)
	if s.target != "" {
		t.Fatal("second click did not close detail")
	}
	g.in.click = false
	g.in.wheel = -1
	for range 20 {
		s.updateEntityTable(g)
	}
	ids, _, _ = s.entityTableLayout(g)
	if !slices.Contains(ids, "11") {
		t.Fatal("cannot reach final resident")
	}
	s.refresh(g)
	if s.residentTop == 0 {
		t.Fatal("refresh lost scroll position")
	}
	g.in.wheel = 1
	for range 20 {
		s.updateEntityTable(g)
	}
	if s.residentTop != 0 {
		t.Fatal("cannot return to first resident")
	}
	g.in.wheel = 0
	g.in.pressed[btnDown] = true
	for range order {
		s.updateEntityTable(g)
		visible, _, _ := s.entityTableLayout(g)
		if !slices.Contains(visible, order[s.focus]) {
			t.Fatal("keyboard focus moved offscreen")
		}
	}
	g.in.pressed[btnDown] = false
	g.in.pressed[btnOK] = true
	s.updateEntityTable(g)
	if s.target != order[s.focus] {
		t.Fatal("keyboard opened wrong resident")
	}
}

func TestCalendarNamesClosuresAndSixWeekLayout(t *testing.T) {
	p := &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-11-30", WeekdaysOff: []int{0, 6}, Closed: []string{"2030-09-02"}}, Schedule: []data.Reservation{{Date: "2030-09-03", Event: "private_event"}}}
	v := data.PlanningView{WeekStart: 1, Markers: []data.TimelineMarker{{Date: "2030-09-02", Label: "Fair"}, {Date: "2030-09-02", Label: "Fair"}, {Date: "2030-09-02", Label: "Delivery"}}}
	cells := calendarCells(p, v, "2030-09-30", 0, 108, 1280, 494)
	if len(cells) != 30 || cells[0].bounds.Min.X < 1000 || cells[29].bounds.Min.X != 20 || cells[29].bounds.Min.Y <= cells[22].bounds.Min.Y {
		t.Fatalf("Sunday start / Monday end must occupy six weeks: %+v", cells)
	}
	if !cells[0].closed || !cells[1].closed || cells[2].closed || !slices.Equal(cells[1].labels, []string{"Fair", "Delivery"}) || !slices.Equal(cells[2].labels, []string{"1 events"}) {
		t.Fatalf("closures or public names wrong: %+v", cells[:3])
	}
	for i, cell := range cells {
		if cell.bounds.Max.Y > 602 || cell.bounds.Dy() < 72 {
			t.Fatalf("cell cannot fit date and three 15px lines: %v", cell)
		}
		for _, other := range cells[:i] {
			if cell.bounds.Overlaps(other.bounds) {
				t.Fatal("overlapping cells")
			}
		}
	}
	v.WeekStart = 0
	if calendarCells(p, v, "2030-09-01", 0, 108, 1280, 494)[0].bounds.Min.X != 20 {
		t.Fatal("default Sunday column changed")
	}
}

func TestCalendarBrowsePreservesDateAndRefresh(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-08-31", End: "2030-11-30", Phases: []string{"morning"}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"calendar": {Kind: "calendar", View: data.PlanningView{Fullscreen: true, WeekStart: 1}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "calendar", nil)
	g.in.click = true
	width, _ := g.screen()
	g.in.mouse = calendarButtons(width)[1].Min.Add(image.Pt(20, 20))
	s.updateSurface(g)
	if s.calendarMonth != "2030-09-01" {
		t.Fatalf("month-end rolled over incorrectly: %s", s.calendarMonth)
	}
	s.refresh(g)
	if s.calendarMonth != "2030-09-01" {
		t.Fatal("refresh reset browsing")
	}
	for range 6 {
		s.updateSurface(g)
	}
	if s.calendarMonth != "2030-11-01" {
		t.Fatal("past calendar end")
	}
	g.in.mouse = calendarButtons(width)[0].Min.Add(image.Pt(20, 20))
	for range 6 {
		s.updateSurface(g)
	}
	if s.calendarMonth != "2030-08-01" || g.st.Planning.Date != "2030-08-31" || g.st.Planning.Phase != 0 {
		t.Fatal("browsing changed state or escaped calendar range")
	}
	if newAuthorScreen(g, "calendar", nil).calendarDate(g) != "2030-08-31" {
		t.Fatal("reopening must show current month")
	}
}

func TestTimelineForecast(t *testing.T) {
	date := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	start, end := date("2030-08-01"), date("2030-08-31")
	for _, tc := range []struct {
		name, today string
		report      float64
		cancelled   bool
		label       string
		position    float64
		text        string
	}{
		{"half", "2030-08-11", .5, false, "Reported pace", 20.0 / 31, "Reported pace 2030-08-20"},
		{"beyond", "2030-08-11", .1, false, "Reported pace", 1, "Reported pace > 2030-08-31"},
		{"tiny", "2030-08-11", math.SmallestNonzeroFloat64, false, "Reported pace", 1, "Reported pace > 2030-08-31"},
		{"zero", "2030-08-11", 0, false, "Reported pace", -1, ""},
		{"complete", "2030-08-11", 1, false, "Reported pace", -1, ""},
		{"cancelled", "2030-08-11", .5, true, "Reported pace", -1, ""},
		{"start", "2030-08-01", .5, false, "Reported pace", -1, ""},
		{"future", "2030-07-31", .5, false, "Reported pace", -1, ""},
		{"disabled", "2030-08-11", .5, false, "", -1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, text := timelineForecast(start, date(tc.today), start, end, tc.report, tc.cancelled, tc.label)
			if got != tc.position || text != tc.text {
				t.Fatalf("got %v %q", got, text)
			}
		})
	}
}

func TestTimelineAssigneeFilter(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar:   data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}},
		Entities:   map[string]data.Entity{"a": {Name: "Weaver"}, "b": {Name: "Potter"}},
		Activities: map[string]data.Activity{"cloth": {Name: "Cloth", Effort: 10, Assignee: "a"}, "vase": {Name: "Vase", Effort: 10, Assignee: "b"}},
	}
	v := data.PlanningView{Fullscreen: true, FilterLabel: "All residents", Order: []string{"b", "a"}, Visible: "shown", Rows: []data.TimelineRow{{Label: "Crafts", Items: []data.TimelineItem{{Activity: "cloth"}, {Activity: "vase"}}}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"tasks": {Kind: "timeline", View: v}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	for _, id := range v.Order {
		g.st.Planning.Entities[id].Observations["shown"] = true
	}
	s := newAuthorScreen(g, "tasks", nil)
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	original := s.timelineLanes(g)
	click := func(button int) {
		t.Helper()
		g.in.click = true
		g.in.mouse = timelineFilterBounds(0, 108)[button].Min.Add(image.Pt(4, 4))
		s.updateSurface(g)
		g.in.click = false
	}
	click(2)
	if s.timelineAssignee != "b" {
		t.Fatal("authored order ignored")
	}
	lanes := s.timelineLanes(g)
	if !lanes[0].dimmed || lanes[1].dimmed {
		t.Fatal("wrong assignment emphasized")
	}
	for i, lane := range lanes {
		if lane.bounds != original[i].bounds || lane.reported != original[i].reported {
			t.Fatal("filter changed timeline geometry or report")
		}
	}
	s.refresh(g)
	if s.timelineAssignee != "b" {
		t.Fatal("refresh lost filter")
	}
	click(2)
	if s.timelineAssignee != "a" {
		t.Fatal("cannot select second resident")
	}
	click(2)
	if s.timelineAssignee != "" {
		t.Fatal("cannot cycle to all")
	}
	click(0)
	if s.timelineAssignee != "a" {
		t.Fatal("reverse cycle broken")
	}
	click(1)
	if s.timelineAssignee != "" {
		t.Fatal("name did not reset")
	}
	after, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || len(g.stack) != 0 {
		t.Fatal("viewing changed game state")
	}
	click(2)
	a := g.st.Planning.Activities["cloth"]
	a.Assignee = "b"
	g.st.Planning.Activities["cloth"] = a
	if s.timelineLanes(g)[0].dimmed {
		t.Fatal("reassignment not reflected")
	}
	delete(g.st.Planning.Entities["b"].Observations, "shown")
	if got := s.timelineAssignees(g); !slices.Equal(got, []string{"", "a"}) || s.timelineAssignee != "" {
		t.Fatal("hidden resident retained", got)
	}
	if newAuthorScreen(g, "tasks", nil).timelineAssignee != "" {
		t.Fatal("reopening retained filter")
	}
	db.Game.Screens["tasks"].View.FilterLabel = ""
	if s.timelineAssignees(g) != nil {
		t.Fatal("selector enabled without label")
	}
}

func TestTimelineRowResidentsUsePublicIdentity(t *testing.T) {
	db := testgame.Load(t, nil)
	p := &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}}, Entities: map[string]data.Entity{}, Activities: map[string]data.Activity{}}
	row := data.TimelineRow{Label: "Workshop", Text: "Public summary"}
	for i, id := range []string{"a", "b", "c", "d", "a"} {
		p.Entities[id] = data.Entity{Name: "Resident " + id}
		task := strconv.Itoa(i)
		p.Activities[task] = data.Activity{Name: task, Effort: 10, Assignee: id}
		row.Items = append(row.Items, data.TimelineItem{Activity: task})
	}
	db.Game.Planning = p
	v := data.PlanningView{Fullscreen: true, Portrait: "icon", Visible: "shown", Rows: []data.TimelineRow{row}}
	db.Game.Screens = map[string]*data.AuthorScreen{"tasks": {Kind: "timeline", View: v}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	p.Attributes = map[string]data.Attribute{"icon": {Hidden: true}}
	for id, e := range g.st.Planning.Entities {
		e.Observations["shown"] = true
		e.Observations["icon"] = "neutral-" + id
		e.Attributes["icon"] = "private-expression"
	}
	s := newAuthorScreen(g, "tasks", nil)
	groups, _, _ := s.groupedTimeline(g, 0, 108, 1280, 494)
	if len(groups[0].residents) != 4 || groups[0].residents[0].portrait != "neutral-a" {
		t.Fatalf("deduplication or public portrait: %+v", groups[0].residents)
	}
	g.in.mouse = image.Pt(200, 170)
	if got := s.timelineDescription(g); !strings.Contains(got, "Resident d") || !strings.Contains(got, "Public summary") {
		t.Fatal("overflow identities and row text must remain readable", got)
	}
	g.st.Planning.Entities["b"].Observations["shown"] = false
	a := g.st.Planning.Activities["2"]
	a.Cancelled = true
	g.st.Planning.Activities["2"] = a
	a = g.st.Planning.Activities["3"]
	a.Assignee = "a"
	g.st.Planning.Activities["3"] = a
	groups, _, _ = s.groupedTimeline(g, 0, 108, 1280, 494)
	if len(groups[0].residents) != 1 || groups[0].residents[0].id != "a" {
		t.Fatal("hidden, cancelled or reassigned residents retained", groups[0].residents)
	}
	delete(g.st.Planning.Entities["a"].Observations, "icon")
	groups, _, _ = s.groupedTimeline(g, 0, 108, 1280, 494)
	if groups[0].residents[0].portrait != "" {
		t.Fatal("missing observation fell back to hidden portrait")
	}
	db.Game.Screens["tasks"].View.Portrait = ""
	groups, _, _ = s.groupedTimeline(g, 0, 108, 1280, 494)
	if len(groups[0].residents) != 0 {
		t.Fatal("portrait omission must preserve text-only rows")
	}
}

func TestTimelineInlineExpansionPreservesCalendarAndState(t *testing.T) {
	db := testgame.Load(t, nil)
	p := &data.Planning{
		Calendar:   data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}},
		Entities:   map[string]data.Entity{"maker": {Name: "Weaver"}},
		Activities: map[string]data.Activity{},
	}
	v := data.PlanningView{Fullscreen: true}
	for i := range 12 {
		row := data.TimelineRow{Label: "Craft " + strconv.Itoa(i)}
		for j := range 3 {
			id := strconv.Itoa(i*3 + j)
			p.Activities[id] = data.Activity{Name: "Item " + id, Assignee: "maker", Effort: 10}
			row.Items = append(row.Items, data.TimelineItem{Activity: id, Screen: "detail"})
		}
		v.Rows = append(v.Rows, row)
	}
	db.Game.Planning = p
	db.Game.Screens = map[string]*data.AuthorScreen{
		"tasks":  {Kind: "timeline", View: v},
		"detail": {Title: "Details", Entries: []data.ScreenEntry{{Label: "Inspect"}}},
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	a := g.st.Planning.Activities["33"]
	a.Progress, a.Observed = 9, 2
	g.st.Planning.Activities["33"] = a
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	s := newAuthorScreen(g, "tasks", nil)
	groups, ax, aw := s.groupedTimeline(g, 0, 108, 1280, 494)
	g.in.click, g.in.mouse = true, image.Pt(24, int(groups[11].y)+4)
	s.updateSurface(g)
	expanded, bx, bw := s.groupedTimeline(g, 0, 108, 1280, 494)
	if s.timelineTop != 11 || len(expanded) != 1 || !expanded[0].expanded || ax != bx || aw != bw {
		t.Fatal("last group was not revealed on the fixed axis")
	}
	for i, lane := range expanded[0].lanes {
		if lane.labelBounds.Max.Y > 602 || lane.labelBounds.Empty() {
			t.Fatal("detail clipped", lane.labelBounds)
		}
		for _, other := range expanded[0].lanes[:i] {
			if lane.labelBounds.Overlaps(other.labelBounds) {
				t.Fatal("overlapping labels")
			}
		}
	}
	if title, text := timelineLaneSummary(g, expanded[0].lanes[0]); title != "Item 33" || text != "Weaver · 20%" {
		t.Fatal("summary leaked hidden work", title, text)
	}
	s.refresh(g)
	if s.timelineExpanded != "33" {
		t.Fatal("refresh lost expansion")
	}
	g.in.mouse = expanded[0].lanes[1].labelBounds.Min
	if !strings.Contains(s.timelineDescription(g), "Item 34") {
		t.Fatal("hover did not reveal full activity detail")
	}
	s.updateSurface(g)
	if len(g.stack) != 1 {
		t.Fatal("label did not open authored detail")
	}
	g.in.mouse = image.Pt(24, int(expanded[0].y)+4)
	s.updateSurface(g)
	groups, _, _ = s.groupedTimeline(g, 0, 108, 1280, 494)
	if s.timelineExpanded != "" || s.timelineTop != 0 || len(groups) != 12 {
		t.Fatal("collapse did not restore overview")
	}
	after, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("browsing mutated planning")
	}
}
