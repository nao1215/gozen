package play

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

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
	s := newAuthorScreen(g, "tasks", nil, nil)
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
	s := newAuthorScreen(g, "tasks", nil, nil)
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
	if groups[0].lanes[0].bounds.Max.X != 1260 || groups[0].lanes[0].right <= 1 {
		t.Fatal("extended deadline is not clipped")
	}
	p.Activities["extra"] = data.Activity{Name: "Extra", Effort: 1}
	rows := timelineGroups(p, v)
	if len(rows) != 13 || rows[12].Items[0].Activity != "extra" {
		t.Fatal("unlisted task disappeared")
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
	s := newAuthorScreen(g, "tasks", nil, nil)
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
	if newAuthorScreen(g, "tasks", nil, nil).timelineAssignee != "" {
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
	s := newAuthorScreen(g, "tasks", nil, nil)
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
			row.Items = append(row.Items, data.TimelineItem{Activity: id, Screen: "detail", Text: "Planned {2*5} units"})
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
	s := newAuthorScreen(g, "tasks", nil, nil)
	groups, ax, aw := s.groupedTimeline(g, 0, 108, 1280, 494)
	g.in.click, g.in.mouse = true, image.Pt(24, int(groups[11].y)+4)
	s.updateSurface(g)
	expanded, bx, bw := s.groupedTimeline(g, 0, 108, 1280, 494)
	if s.timelineTop != 11 || len(expanded) != 1 || !expanded[0].expanded || ax != bx || aw != bw {
		t.Fatal("last group was not revealed on the fixed axis")
	}
	for i, lane := range expanded[0].lanes {
		if lane.labelBounds.Max.Y > 602 || lane.labelBounds.Dy() != 64 || lane.text != "Planned 10 units" {
			t.Fatal("detail clipped", lane.labelBounds, lane.text)
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
	if description := s.timelineDescription(g); !strings.Contains(description, "Item 34") || !strings.Contains(description, "Planned 10 units") {
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

func TestTimelineCandidatePaneBrowsing(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar:   data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}},
		Activities: map[string]data.Activity{"kiln": {Name: "Kiln", Effort: 10, From: "2030-08-01", To: "2030-11-30"}},
	}
	db.Game.Screens = map[string]*data.AuthorScreen{
		"tasks": {Kind: "timeline", View: data.PlanningView{Fullscreen: true, Rows: []data.TimelineRow{{Label: "Pottery", Items: []data.TimelineItem{{Activity: "kiln", Screen: "pick"}}}}}},
		"pick":  {View: data.PlanningView{Portrait: "icon"}},
	}
	for range 12 {
		db.Game.Screens["pick"].Entries = append(db.Game.Screens["pick"].Entries, data.ScreenEntry{Label: "Maker"})
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	parent := newAuthorScreen(g, "tasks", nil, nil)
	g.stack = []scene{parent}
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	lane := parent.timelineLanes(g)[0]
	g.in = input{click: true, mouse: lane.bounds.Min}
	if !parent.updateGroupedTimeline(g) || len(g.stack) != 2 {
		t.Fatal("lane did not open picker")
	}
	child, ok := g.stack[1].(*authorScreen)
	if !ok {
		t.Fatal("candidate screen missing")
	}
	if child.timelineContext != parent || child.timelineActivity != "kiln" {
		t.Fatal("lost selected activity")
	}
	child.list.sel = 11
	child.list.scroll()
	child = child.refreshed(g)
	g.stack[1] = child
	r := child.list.rowRect(11)
	if r.Min.X < 920 || r.Max.X > 1280 || r.Max.Y > 688 || child.list.top == 0 {
		t.Fatal("pane rows inaccessible", r)
	}
	groups, ax, aw := parent.groupedTimeline(g, 0, 108, 920, 494)
	if len(groups) != 1 || groups[0].lanes[0].bounds.Min.X != int(ax) || groups[0].lanes[0].bounds.Max.X > int(ax+aw)+1 {
		t.Fatal("date range clipped")
	}
	g.in = input{click: true, mouse: image.Pt(r.Min.X+20, r.Min.Y+20)}
	chosen, cancel := child.list.update(g)
	if !chosen || cancel || child.list.sel != 11 {
		t.Fatal("last candidate not selectable")
	}
	g.in = input{rclick: true}
	child.update(g)
	if len(g.stack) != 1 || g.stack[0] != parent {
		t.Fatal("cancel lost timeline")
	}
	after, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("browsing changed planning")
	}
	if newAuthorScreen(g, "pick", nil, nil).timelineContext != nil {
		t.Fatal("standalone picker changed")
	}
	db.Game.Screens["pick"].Params = map[string]string{"task": "activity"}
	db.Game.Screens["pick"].Title = "Choose for {arg.task}"
	db.Game.Screens["tasks"].View.Rows[0].Items[0].Args = map[string]data.Expr{"task": data.MustExpr(`"kiln"`)}
	g.in = input{click: true, mouse: lane.bounds.Min}
	if !parent.updateGroupedTimeline(g) {
		t.Fatal("bound lane did not open")
	}
	bound, ok := g.stack[len(g.stack)-1].(*authorScreen)
	if !ok {
		t.Fatal("bound candidate screen missing")
	}
	if bound.title != "Choose for kiln" || bound.args["task"] != "kiln" || bound.timelineContext != parent {
		t.Fatal("lane lost bound destination or context")
	}
}

func TestTimelineZoomBrowsing(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar: data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}},
		Activities: map[string]data.Activity{
			"kiln":  {Name: "Kiln", Effort: 100, From: "2030-08-01", To: "2030-11-30"},
			"early": {Name: "Clay", Effort: 10, From: "2030-08-01", To: "2030-08-02"},
			"late":  {Name: "Glaze", Effort: 10, From: "2030-11-29", To: "2030-11-30"},
		},
	}
	db.Game.Screens = map[string]*data.AuthorScreen{
		"tasks": {Kind: "timeline", View: data.PlanningView{Fullscreen: true, Rows: []data.TimelineRow{
			{Label: "Pottery", Items: []data.TimelineItem{{Activity: "kiln", Screen: "pick"}, {Activity: "early"}, {Activity: "late"}}},
		}}},
		"pick": {View: data.PlanningView{Portrait: "icon"}, Entries: []data.ScreenEntry{{Label: "Maker"}}},
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Date = "2030-09-15"
	g.st.Planning.Activities["kiln"].Observed = 50
	s := newAuthorScreen(g, "tasks", nil, nil)
	g.stack = []scene{s}
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	overview := s.timelineLanes(g)
	g.in.pressed[btnMapZoomIn] = true
	s.updateSurface(g)
	g.in.pressed[btnMapZoomIn] = false
	start, end, err := s.timelineRange(g)
	if err != nil || start.Format(time.DateOnly) != "2030-09-05" || end.Format(time.DateOnly) != "2030-09-25" {
		t.Fatalf("wrong focus window: %s %s %v", start, end, err)
	}
	lanes := s.timelineLanes(g)
	if lanes[0].left >= 0 || lanes[0].right <= 1 || lanes[0].reported != .5 || lanes[0].expected != overview[0].expected {
		t.Fatalf("zoom distorted progress: %+v", lanes[0])
	}
	if lanes[0].bounds.Min.X != 260 || lanes[0].bounds.Max.X != 1260 || !lanes[1].bounds.Empty() || !lanes[2].bounds.Empty() {
		t.Fatalf("off-window hit areas: %+v", lanes)
	}
	g.in.mouse = image.Pt(260, 140)
	if s.timelinePointerDate(g) != "2030-09-05" {
		t.Fatal("left date does not match zoom")
	}
	g.in.mouse = image.Pt(1259, 140)
	if s.timelinePointerDate(g) != "2030-09-25" {
		t.Fatal("right date does not match zoom")
	}
	s.refresh(g)
	if !s.timelineZoom {
		t.Fatal("refresh lost zoom")
	}
	// Candidate browsing and cancel retain the same parent range.
	g.in.pressed[btnOK] = true
	s.updateSurface(g)
	g.in.pressed[btnOK] = false
	child, ok := g.stack[len(g.stack)-1].(*authorScreen)
	if !ok || child == s || child.timelineContext != s {
		t.Fatal("focused lane did not open candidates")
	}
	g.in.pressed[btnCancel] = true
	child.update(g)
	g.in.pressed[btnCancel] = false
	if g.stack[len(g.stack)-1] != s || !s.timelineZoom {
		t.Fatal("cancel lost zoom")
	}
	g.in.click = true
	g.in.mouse = timelineZoomBounds(0, 108, 1280)[1].Min.Add(image.Pt(4, 4))
	if s.timelinePointerDate(g) != "" {
		t.Fatal("zoom button is a date target")
	}
	s.updateSurface(g)
	g.in.click = false
	if s.timelineZoom {
		t.Fatal("minus button failed")
	}
	restored := s.timelineLanes(g)
	for i, lane := range restored {
		if lane.bounds != overview[i].bounds {
			t.Fatal("overview geometry not restored")
		}
	}
	after, err := json.Marshal(g.st.Planning)
	if err != nil || string(after) != string(before) {
		t.Fatal("browsing mutated planning")
	}
	// Boundary windows retain 21 days, and a short calendar stays complete.
	s.timelineZoom = true
	for _, tc := range []struct{ today, start, end string }{
		{"2030-08-01", "2030-08-01", "2030-08-21"},
		{"2030-11-30", "2030-11-10", "2030-11-30"},
	} {
		g.st.Planning.Date = tc.today
		start, end, err = s.timelineRange(g)
		if err != nil || start.Format(time.DateOnly) != tc.start || end.Format(time.DateOnly) != tc.end {
			t.Fatalf("boundary: %s %s", start, end)
		}
	}
	db.Game.Planning.Calendar.End = "2030-08-05"
	start, end, err = s.timelineRange(g)
	if err != nil || start.Format(time.DateOnly) != "2030-08-01" || end.Format(time.DateOnly) != "2030-08-05" {
		t.Fatal("short calendar expanded")
	}
}

// On a light surface the timeline is pale with filled bars: the planned span
// is not a dark color, the shortfall is orange and the report deep blue; the
// dark palette of other surfaces stays as it was.
func TestTimelineLightPalette(t *testing.T) {
	dark := timelinePalette(false, color.NRGBA{230, 230, 230, 255}, color.NRGBA{150, 150, 150, 255}, color.NRGBA{90, 160, 255, 255})
	if dark.inside != (color.NRGBA{30, 34, 40, 255}) || dark.holiday != (color.NRGBA{48, 53, 62, 255}) || dark.lateFill {
		t.Fatal("the dark timeline changed", dark)
	}
	light := timelinePalette(true, color.NRGBA{40, 49, 58, 255}, color.NRGBA{91, 104, 116, 255}, color.NRGBA{37, 105, 109, 255})
	luma := func(c color.NRGBA) int { return (299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000 }
	if luma(light.inside) < 180 || luma(light.holiday) < 180 || luma(light.line) < 160 {
		t.Fatal("the light timeline is dark inside", light)
	}
	if !light.lateFill || light.late.R <= light.late.B || luma(light.reported) > 120 {
		t.Fatal("the shortfall is not orange or the report not deep", light)
	}

	fsys := wldWorldFiles(t, `planning:
  calendar: {start: '2030-08-01', end: '2030-11-30', phases: [morning], weekdays_off: [0, 6]}
  entities: {a: {name: Weaver}}
  activities:
    cloth: {name: Cloth, effort: 10, assignee: a, from: '2030-08-01'}
    vase: {name: Vase, effort: 10, assignee: a, from: '2030-08-01'}
screens:
  tasks:
    title: Tasks
    kind: timeline
    view: {fullscreen: true, light: true, rows: [{label: Crafts, items: [{activity: cloth}, {activity: vase}]}]}
`, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.st.Planning.Date = "2030-09-10"
	c := g.st.Planning.Activities["vase"]
	c.Cancelled = true
	g.st.Planning.Activities["vase"] = c
	g.openScreen("tasks", func() {})
	wldFrames(t, g, 4)
	s := top(t, g)
	if lanes := s.timelineLanes(g); len(lanes) != 2 || lanes[0].reported >= lanes[0].expected {
		t.Fatal("the fixture has no shortfall", lanes)
	}
	g.Draw(covScreen())
}

func TestTimelineWideCalendarGeometryAndPointer(t *testing.T) {
	// YAML dates cover four-digit years; time.Duration only spans about 292.
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar:   data.Calendar{Start: "0001-01-01", End: "3001-01-01", Phases: []string{"morning"}},
		Activities: map[string]data.Activity{"archive": {Name: "Archive", From: "1001-01-01", To: "2001-01-01", Effort: 10}},
	}
	db.Game.Screens = map[string]*data.AuthorScreen{"timeline": {Kind: "timeline", View: data.PlanningView{Fullscreen: true, Rows: []data.TimelineRow{{Items: []data.TimelineItem{{Activity: "archive"}}}}}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Date = "1501-01-01"
	s := newAuthorScreen(g, "timeline", nil, nil)
	lanes := s.timelineLanes(g)
	if len(lanes) != 1 {
		t.Fatalf("lanes = %d", len(lanes))
	}
	lane := lanes[0]
	if math.Abs(lane.left-1.0/3) > .001 || math.Abs(lane.right-2.0/3) > .001 || math.Abs(lane.expected-.5) > .001 {
		t.Fatalf("wide calendar distorted geometry or progress: %+v", lane)
	}
	g.in.mouse = image.Pt(760, 150) // Midpoint of the 260..1260 date axis.
	date := s.timelinePointerDate(g)
	if date < "1500-12-30" || date > "1501-01-02" {
		t.Fatalf("midpoint date = %s, want around 1501-01-01", date)
	}
	parse := func(value string) time.Time {
		d, err := time.Parse(time.DateOnly, value)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	at, label := timelineForecast(parse("1001-01-01"), parse("1501-01-01"), parse("0001-01-01"), parse("3001-01-01"), .5, false, "Estimate")
	if math.Abs(at-2.0/3) > .001 || !strings.HasPrefix(label, "Estimate 200") {
		t.Fatalf("wide forecast = %v, %q", at, label)
	}
}

func TestTimelineDrawingKeepsPublicReportAndSelection(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, zoom := range []bool{false, true} {
			t.Run(fmt.Sprintf("light=%t/zoom=%t", light, zoom), func(t *testing.T) {
				g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
				p := &data.Planning{
					Calendar: data.Calendar{Start: "2030-08-01", End: "2030-08-31", Phases: []string{"morning"}},
					Entities: map[string]data.Entity{}, Activities: map[string]data.Activity{},
				}
				v := data.PlanningView{Fullscreen: true, Light: light, Portrait: "portrait", FilterLabel: "All makers", Rows: []data.TimelineRow{{Label: "Crafts", Text: "Shared workshop"}}}
				for i := range 5 {
					id := fmt.Sprintf("maker%d", i)
					p.Entities[id] = data.Entity{Name: "Maker " + strconv.Itoa(i)}
					p.Activities[id] = data.Activity{Name: "Item " + strconv.Itoa(i), Assignee: id, Effort: 10, From: "2030-08-01", To: "2030-08-31"}
					v.Rows[0].Items = append(v.Rows[0].Items, data.TimelineItem{Activity: id, Text: "Ten units", Sample: data.MustExpr("0.4"), SampleLabel: "Inspection", ForecastLabel: "Reported pace"})
				}
				v.Markers = []data.TimelineMarker{{Date: "2030-08-20", Label: "Review", Milestone: true}, {Date: "2030-08-19"}, {Date: "invalid", Label: "Invalid"}, {Date: "2031-01-01", Label: "Outside"}}
				g.db.Game.Planning = p
				g.db.Game.Screens = map[string]*data.AuthorScreen{"tasks": {Kind: "timeline", View: v}}
				g.st = testgame.State(g.db, 19)
				g.st.Planning.Date = "2030-08-20"
				for id, a := range g.st.Planning.Activities {
					a.Observed, a.Progress = 2, 9
					g.st.Planning.Entities[id].Observations["portrait"] = "x.png"
				}
				g.st.Planning.Activities["maker4"].Cancelled = true
				s := newAuthorScreen(g, "tasks", nil, nil)
				s.timelineZoom, s.timelineAssignee = zoom, "maker0"
				before, err := json.Marshal(g.st)
				if err != nil {
					t.Fatal(err)
				}
				dst := covScreen()
				for _, expanded := range []string{"", "maker0"} {
					s.timelineExpanded = expanded
					lanes := s.timelineLanes(g)
					for range 2 {
						s.draw(g, dst)
					}
					if !reflect.DeepEqual(lanes, s.timelineLanes(g)) || s.timelineAssignee != "maker0" {
						t.Fatal("drawing changed lane geometry, reports or filter")
					}
					for _, lane := range lanes {
						if lane.reported != .2 || lane.sample != .4 {
							t.Fatalf("hidden work leaked: %+v", lane)
						}
					}
				}
				// Hovering the axis exposes authored annotations, without opening
				// the date or changing the underlying planning state.
				start, end, err := s.timelineRange(g)
				if err != nil {
					t.Fatal(err)
				}
				day, err := time.Parse(time.DateOnly, "2030-08-20")
				if err != nil {
					t.Fatal(err)
				}
				sw, sh := g.screen()
				_, ax, aw := s.groupedTimeline(g, 0, 108, sw, sh-226)
				g.in.mouse = image.Pt(int(ax+aw*(planningDays(start, day)+.5)/(planningDays(start, end)+1)), 140)
				if got := s.timelineDescription(g); !strings.Contains(got, "Review") {
					t.Fatal("annotation lost", got)
				}
				after, err := json.Marshal(g.st)
				if err != nil || string(before) != string(after) {
					t.Fatal("drawing or hovering changed saved state", err)
				}
			})
		}
	}
}

func TestTimelineGroupClickAcknowledgesExpansion(t *testing.T) {
	for _, count := range []int{2, 20} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			db := testgame.Load(t, nil)
			db.Game.Planning = &data.Planning{
				Calendar:   data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}},
				Activities: map[string]data.Activity{},
			}
			row := data.TimelineRow{Label: "Crafts"}
			for i := range count {
				id := strconv.Itoa(i)
				db.Game.Planning.Activities[id] = data.Activity{Name: "Item " + id, Effort: 10}
				row.Items = append(row.Items, data.TimelineItem{Activity: id})
			}
			db.Game.Screens = map[string]*data.AuthorScreen{"tasks": {Kind: "timeline", View: data.PlanningView{Fullscreen: true, Rows: []data.TimelineRow{row}}}}
			g := &Game{db: db, st: testgame.State(db, 1)}
			a := newAuthorScreen(g, "tasks", nil, nil)
			s := sysScript(t, "click 24 168\nwait 1000")
			s.acknowledge = true
			for range 6002 {
				g.in = input{}
				s.step(&g.in)
				a.toggleTimelineGroup(g)
				g.in.finishOffer()
				if count == 2 {
					if s.pc != 1 || a.timelineExpanded == "" {
						t.Fatal("expansion did not act and acknowledge")
					}
					return
				}
				if s.pc != 0 || a.timelineExpanded != "" {
					t.Fatal("oversized group acknowledged without expanding")
				}
				if s.err != nil {
					return
				}
			}
			t.Fatal("unhandled group click did not expire")
		})
	}
}
