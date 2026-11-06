package play

import (
	"encoding/json"
	"image"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestCalendarHiddenClosedReservation(t *testing.T) {
	p := &data.Planning{Calendar: data.Calendar{WeekdaysOff: []int{0, 6}}, Schedule: []data.Reservation{{Date: "2030-08-03", Label: "Visitor", HideClosed: true}}}
	for _, hidden := range []bool{true, false} {
		p.Schedule[0].HideClosed = hidden
		for _, c := range calendarCells(p, data.PlanningView{}, "2030-08-01", 0, 0, 700, 600) {
			if c.date == "2030-08-03" && (len(c.labels) == 0) != hidden {
				t.Fatalf("hidden=%v labels=%v", hidden, c.labels)
			}
		}
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
	s := newAuthorScreen(g, "calendar", nil, nil)
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
	if newAuthorScreen(g, "calendar", nil, nil).calendarDate(g) != "2030-08-31" {
		t.Fatal("reopening must show current month")
	}
}

func TestCalendarDateLinksPreserveState(t *testing.T) {
	for _, parameterized := range []bool{false, true} {
		t.Run(strconv.FormatBool(parameterized), func(t *testing.T) {
			db := testgame.Load(t, nil)
			db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-11-30", Phases: []string{"morning"}}}
			view := data.PlanningView{Fullscreen: true, Markers: []data.TimelineMarker{{Date: "2030-08-02", Label: "Fair", Screen: "detail", Args: map[string]data.Expr{"day": data.MustExpr(`"2030-08-02"`)}}}}
			db.Game.Screens = map[string]*data.AuthorScreen{
				"calendar": {Kind: "calendar", View: view}, "timeline": {Kind: "timeline", View: view},
				"detail": {Params: map[string]string{"day": "date"}, Title: "Day {arg.day}", Entries: []data.ScreenEntry{{Label: "Read schedule"}}},
			}
			expectedTitle := "Day 2030-08-02"
			if !parameterized {
				db.Game.Screens["detail"].Params = nil
				db.Game.Screens["detail"].Title = "Day detail"
				view.Markers[0].Args = nil
				expectedTitle = "Day detail"
			}
			g := &Game{db: db, st: testgame.State(db, 1)}
			before, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			w, h := g.screen()
			calendar := newAuthorScreen(g, "calendar", nil, nil)
			g.stack = []scene{calendar}
			cells := calendarCells(db.Game.Planning, view, calendar.calendarDate(g), 0, 108, w, h-226)
			g.in.click = true
			g.in.mouse = cells[1].bounds.Min.Add(image.Pt(8, 8))
			if !calendar.updateCalendar(g) || len(g.stack) != 2 {
				t.Fatal("date did not open detail")
			}
			if detail, ok := g.stack[len(g.stack)-1].(*authorScreen); !ok || detail.title != expectedTitle {
				t.Fatalf("date binding: %#v", detail)
			}
			g.pop()
			g.in.mouse = cells[0].bounds.Min.Add(image.Pt(8, 8))
			if calendar.updateCalendar(g) || len(g.stack) != 1 {
				t.Fatal("unlinked date opened screen")
			}
			timeline := newAuthorScreen(g, "timeline", nil, nil)
			g.in.mouse = image.Pt(int(260+(w-280)*1.5/122), 150)
			if timeline.timelinePointerDate(g) != "2030-08-02" || !timeline.updateGroupedTimeline(g) || len(g.stack) != 2 {
				t.Fatal("band did not open same date")
			}
			if detail, ok := g.stack[len(g.stack)-1].(*authorScreen); !ok || detail.title != expectedTitle {
				t.Fatalf("date binding: %#v", detail)
			}
			g.pop()
			for _, pt := range []image.Point{image.Pt(259, 150), image.Pt(int(w)-20, 150), image.Pt(270, 164)} {
				g.in.mouse = pt
				if timeline.timelinePointerDate(g) != "" {
					t.Fatal("outside band maps to date")
				}
			}
			after, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("browsing changed saved state")
			}
		})
	}
}

func TestSelectedDateLinkAndScheduleLabels(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-08-04", Phases: []string{"morning"}, WeekdaysOff: []int{0, 6}}, Schedule: []data.Reservation{{Label: "Gathering", Weekdays: []int{5, 6}, HideClosed: true}}}
	view := data.PlanningView{DateLink: &data.ScreenCall{Screen: "detail", Args: map[string]data.Expr{"day": data.MustExpr("selected.date"), "offset": data.MustExpr("selected.offset"), "closed": data.MustExpr("selected.closed"), "summary": data.MustExpr("selected.summary"), "weekday": data.MustExpr("selected.weekday")}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"calendar": {Kind: "calendar", View: view}, "detail": {Params: map[string]string{"day": "date", "offset": "number", "closed": "bool", "summary": "string", "weekday": "number"}, Title: "{arg.day} / {arg.offset} / {arg.closed} / {arg.summary} / {arg.weekday}"}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "calendar", nil, nil)
	before, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ date, title string }{
		{"2030-08-02", "2030-08-02 / 1 / false / Gathering / 5"},
		{"2030-08-03", "2030-08-03 / 2 / true /  / 6"},
		{"2030-08-04", "2030-08-04 / 3 / true /  / 0"},
	} {
		g.stack = []scene{s}
		if !s.openCalendarDate(g, tc.date) {
			t.Fatal(tc.date)
		}
		detail, ok := g.stack[1].(*authorScreen)
		if !ok || detail.title != tc.title {
			t.Fatalf("%s: %#v", tc.date, detail)
		}
	}
	for _, date := range []string{"2030-07-31", "2030-08-05", "bad"} {
		if s.openCalendarDate(g, date) {
			t.Fatal("out-of-range date opened", date)
		}
	}
	// Date offsets, unlike time.Duration, must cover the full ISO year range.
	db.Game.Planning.Calendar.Start = "0001-01-01"
	g.stack = []scene{s}
	if !s.openCalendarDate(g, "2030-08-02") {
		t.Fatal("wide range did not open")
	}
	wide, ok := g.stack[1].(*authorScreen)
	if !ok || !strings.Contains(wide.title, " / 741290 / ") {
		t.Fatalf("wide date offset: %#v", wide)
	}
	db.Game.Planning.Calendar.Start = "2030-08-01"
	cells := calendarCells(db.Game.Planning, view, "2030-08-01", 0, 0, 1000, 600)
	if !slices.Equal(cells[1].labels, []string{"Gathering"}) || slices.Contains(cells[2].labels, "Gathering") {
		t.Fatal(cells[:3])
	}
	after, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("browsing changed state")
	}
}

func BenchmarkPlanningMarkers(b *testing.B) {
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, mode := range []string{"none", "unnamed", "named"} {
		b.Run(mode, func(b *testing.B) {
			p := &data.Planning{Calendar: data.Calendar{WeekdaysOff: []int{0, 6}}}
			if mode != "none" {
				p.Schedule = []data.Reservation{{Date: "2030-06-15", Event: "meeting"}}
			}
			if mode == "named" {
				p.Schedule[0].Label = "Meeting"
			}
			v := data.PlanningView{Markers: []data.TimelineMarker{{Date: "2030-03-01", Label: "Delivery"}}}
			b.ReportAllocs()
			for b.Loop() {
				planningMarkers(p, v, start, start.AddDate(1, 0, -1))
			}
		})
	}
}

func TestPlanningMarkersOwnResultsAndKeepOrder(t *testing.T) {
	start := time.Date(2030, 8, 1, 0, 0, 0, 0, time.UTC)
	for _, named := range []bool{false, true} {
		p := &data.Planning{Schedule: []data.Reservation{{Date: "2030-08-02", Event: "internal_event"}}}
		if named {
			p.Schedule[0].Label = "Visit"
		}
		v := data.PlanningView{Markers: []data.TimelineMarker{{Date: "2030-08-03", Label: "Delivery"}}}
		got := planningMarkers(p, v, start, start.AddDate(0, 0, 2))
		want := []data.TimelineMarker{{Date: "2030-08-03", Label: "Delivery"}}
		if named {
			want = append(want, data.TimelineMarker{Date: "2030-08-02", Label: "Visit"})
		}
		if len(got) != len(want) {
			t.Fatalf("named=%v: %v", named, got)
		}
		for i := range want {
			if got[i].Date != want[i].Date || got[i].Label != want[i].Label {
				t.Fatalf("marker order changed: %v", got)
			}
		}
		got[0].Label = "Changed"
		if v.Markers[0].Label != "Delivery" {
			t.Fatal("result aliases authored markers")
		}
	}
}
