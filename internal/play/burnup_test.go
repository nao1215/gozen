package play

import (
	"testing"

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
