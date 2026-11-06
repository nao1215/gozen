package play

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestBurnupKeepsUnknownPastAndFutureUnobserved(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-08-04", Phases: []string{"morning"}}, Activities: map[string]data.Activity{"bowls": {Effort: 10, From: "2030-08-01", To: "2030-08-04"}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Date = "2030-08-03"
	g.st.Planning.Activities["bowls"].Progress = 9
	g.st.Planning.WorkHistory = map[string]rpg.WorkSample{"2030-08-03": {Total: 10, Observed: 2}}
	s := &authorScreen{}
	chart := s.workChart(g, &data.BurnupView{})
	if len(chart.points) != 4 || chart.points[0].known || chart.points[3].known || chart.points[2].observed != 2 {
		t.Fatalf("fabricated observations: %+v", chart.points)
	}
	if chart.points[0].planned != 0 || chart.points[3].planned != 10 {
		t.Fatal("plan endpoints")
	}
	if len(g.st.Planning.WorkHistory) != 1 || g.st.Planning.Activities["bowls"].Progress != 9 {
		t.Fatal("drawing changed state")
	}
}

func TestBurnupLongCalendarRetainsEndAndObservedDates(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-01-01", End: "2050-12-31", Phases: []string{"morning"}}, Activities: map[string]data.Activity{"orchard": {Effort: 100}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Planning.Date = "2031-01-02"
	g.st.Planning.WorkHistory = map[string]rpg.WorkSample{"2031-01-02": {Total: 100, Observed: 5}}
	s := &authorScreen{}
	chart := s.workChart(g, &data.BurnupView{})
	last := chart.points[len(chart.points)-1]
	if last.date != "2050-12-31" || last.position != 1 || len(chart.points) > 725 {
		t.Fatalf("truncated or unbounded plan: %d %+v", len(chart.points), last)
	}
	found := false
	for _, p := range chart.points {
		if p.date == g.st.Planning.Date {
			found = p.known && p.observed == 5 && p.position > 0 && p.position < .1
		}
	}
	if !found {
		t.Fatal("sparse sampling lost an observation")
	}
}

func TestBurnupPlanExpressionsAndStateReplacement(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		want             float64
	}{
		{"constant", "17", 17},
		{"selected date", `selected.date == "2030-08-03" ? 7 : 2`, 7},
		{"negative clamped", "-5", 0},
		{"evaluation error", "missing.field", 10},
		{"wrong type", `"unknown"`, 10},
		{"not finite", "var.plan", 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testgame.Load(t, nil)
			db.Game.Planning = &data.Planning{
				Calendar: data.Calendar{Start: "2030-08-01", End: "2030-08-03", Phases: []string{"morning"}},
				Activities: map[string]data.Activity{
					"bowls":     {Effort: 10, From: "2030-08-01", To: "2030-08-03"},
					"cups":      {Effort: 4, Parent: "bowls"},
					"cancelled": {Effort: 80},
				},
			}
			g := &Game{db: db, st: testgame.State(db, 1)}
			g.st.Planning.Date = "2030-08-03"
			g.st.Vars["plan"] = math.NaN()
			g.st.Planning.Activities["cancelled"].Cancelled = true
			v := &data.BurnupView{Planned: data.MustExpr(tc.expression)}
			s := &authorScreen{}
			chart := s.workChart(g, v)
			last := chart.points[len(chart.points)-1]
			if last.total != 10 || last.planned != tc.want || chart.maximum != max(10, tc.want) {
				t.Fatalf("wrong plan: %+v maximum=%v", last, chart.maximum)
			}
			cached := s.workChart(g, v)
			if &cached.points[0] != &chart.points[0] {
				t.Fatal("unchanged state rebuilt the chart")
			}
			// Planning transitions publish a new state pointer. Its observation
			// must replace the cached chart without mutating the previous result.
			next := *g.st.Planning
			next.WorkHistory = map[string]rpg.WorkSample{"2030-08-03": {Total: 12, Observed: 8}}
			g.st.Planning = &next
			rebuilt := s.workChart(g, v)
			point := rebuilt.points[len(rebuilt.points)-1]
			if !point.known || point.total != 12 || point.observed != 8 || chart.points[len(chart.points)-1].known {
				t.Fatalf("stale or mutated chart: %+v", point)
			}
		})
	}
}

func TestBurnupSamplingPreservesCalendarContract(t *testing.T) {
	for _, dates := range [][2]string{
		{"2030-08-01", "2030-08-01"},
		{"2028-02-28", "2028-03-01"},
		{"0001-01-01", "3001-01-01"},
		{"invalid", "2030-08-01"},
		{"2030-08-01", "invalid"},
		{"2030-08-03", "2030-08-01"},
	} {
		t.Run(dates[0]+"/"+dates[1], func(t *testing.T) {
			db := testgame.Load(t, nil)
			db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: dates[0], End: dates[1], Phases: []string{"morning"}}}
			g := &Game{db: db, st: testgame.State(db, 1)}
			g.st.Planning.Date = dates[0]
			g.st.Planning.WorkHistory = map[string]rpg.WorkSample{
				dates[0]:     {Total: 3, Observed: 2},
				"2028-02-29": {Total: 5, Observed: 4},
				"9999-12-31": {Total: 900, Observed: 800},
				"not a date": {Total: 900, Observed: 800},
			}
			s := &authorScreen{}
			chart := s.workChart(g, &data.BurnupView{})
			start, e1 := time.Parse(time.DateOnly, dates[0])
			end, e2 := time.Parse(time.DateOnly, dates[1])
			if e1 != nil || e2 != nil || end.Before(start) {
				if len(chart.points) != 0 {
					t.Fatal("invalid calendar fabricated points")
				}
				return
			}
			if len(chart.points) == 0 || len(chart.points) > 724 || chart.points[0].date != dates[0] || chart.points[len(chart.points)-1].date != dates[1] {
				t.Fatalf("lost endpoints or unbounded sample: %+v", chart)
			}
			for i, p := range chart.points {
				if p.position < 0 || p.position > 1 || i > 0 && (p.date <= chart.points[i-1].date || p.position <= chart.points[i-1].position) {
					t.Fatalf("unordered point %d: %+v", i, p)
				}
				if p.known != (p.date == dates[0]) {
					t.Fatalf("future observation revealed: %+v", p)
				}
			}
			if chart.maximum != 3 {
				t.Fatalf("unobserved history changed scale: %v", chart.maximum)
			}
		})
	}
}

// Drawing exercises layout and cache behavior; it does not assert GPU pixels.
func TestBurnupDrawingPreservesObservations(t *testing.T) {
	for _, compact := range []bool{false, true} {
		t.Run(strconv.FormatBool(compact), func(t *testing.T) {
			g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
			g.db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-08-01", End: "2030-08-04", Phases: []string{"morning"}}}
			v := &data.BurnupView{Compact: compact, Title: "Work", Total: "Scope", PlannedLabel: "Plan", Observed: "Done", Today: "Today", Deadline: "End", Unit: "h", Planned: data.MustExpr("2")}
			g.db.Game.Screens = map[string]*data.AuthorScreen{"work": {View: data.PlanningView{Burnup: v}}}
			g.st = testgame.State(g.db, 1)
			g.st.Planning.Date = "2030-08-03"
			g.st.Planning.WorkHistory = map[string]rpg.WorkSample{
				"2030-08-01": {Total: 10, Observed: 1},
				"2030-08-02": {Total: 10, Observed: 2},
			}
			s := newAuthorScreen(g, "work", nil, nil)
			before, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			random, err := g.pcg.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			dst := covScreen()
			for range 3 {
				s.drawBurnup(g, dst, 20, 40, 800, 300)
			}
			after, err := json.Marshal(g.st)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("drawing changed observations", err)
			}
			randomAfter, err := g.pcg.MarshalBinary()
			if err != nil || !bytes.Equal(random, randomAfter) {
				t.Fatal("drawing consumed randomness", err)
			}
			if len(s.chart.points) != 4 || s.chart.points[2].known || s.chart.maximum != 10 {
				t.Fatalf("wrong drawing data: %+v", s.chart)
			}
		})
	}
}
