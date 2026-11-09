package rpg_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func workshopPlanning() *data.Planning {
	return &data.Planning{
		Calendar:   data.Calendar{Start: "2030-07-31", End: "2030-08-10", Phases: []string{"morning", "evening"}},
		Attributes: map[string]data.Attribute{"vigor": {Type: "number", Initial: 5.0}},
		Entities:   map[string]data.Entity{"potter": {Name: "Potter"}, "weaver": {Name: "Weaver"}},
		Activities: map[string]data.Activity{"clay": {Name: "Clay", Effort: 4, Assignee: "weaver"}, "loom": {Name: "Loom", Effort: 2}},
		Ledgers:    map[string]data.Ledger{"hours": {Retain: 2}},
	}
}

// TestPlanningSaveSurvivesRevisedDefinitions reads a save made before a
// release that added and removed planning definitions and shortened the
// calendar: the save opens, new records take their initial values, removed
// ones are dropped, and each repair is reported.
func TestPlanningSaveSurvivesRevisedDefinitions(t *testing.T) {
	old := testgame.Load(t, nil)
	old.Game.Planning = workshopPlanning()
	st := testgame.State(old, 3)
	st.Planning.Date, st.Planning.Phase = "2030-08-08", 1
	st.Planning.Activities["clay"].Progress = 3
	st.Planning.Activities["loom"].Progress = 2
	st.Planning.Activities["loom"].Completed = "2030-08-07"
	st.Planning.Ledgers["hours"]["2030-08"] = map[string]float64{"potter": 2, "weaver": 3}
	st.Planning.WorkHistory = map[string]rpg.WorkSample{"2030-08-01": {Total: 6, Observed: 1}, "2030-08-08": {Total: 6, Observed: 4}}
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}

	next := testgame.Load(t, nil)
	p := workshopPlanning()
	p.Calendar.End = "2030-08-05"
	delete(p.Entities, "weaver")
	p.Entities["glazer"] = data.Entity{Name: "Glazer", Attributes: map[string]any{"vigor": 2.0}}
	p.Attributes["focus"] = data.Attribute{Type: "number", Initial: 1.0}
	delete(p.Activities, "loom")
	p.Activities["kiln"] = data.Activity{Name: "Kiln", Effort: 3, Assignee: "glazer"}
	p.Activities["clay"] = data.Activity{Name: "Clay", Effort: 3, Assignee: "potter"}
	delete(p.Ledgers, "hours")
	p.Ledgers["clay_used"] = data.Ledger{Retain: 1}
	next.Game.Planning = p

	got, err := rpg.Parse(next, b, rand.New(rand.NewPCG(1, 1))) //nolint:gosec // Deterministic test.
	if err != nil {
		t.Fatalf("revised definitions refused the save: %v", err)
	}
	ps := got.Planning
	if ps.Date != "2030-08-05" || ps.Phase != 1 {
		t.Fatalf("date not moved into the shortened calendar: %s/%d", ps.Date, ps.Phase)
	}
	if _, ok := ps.WorkHistory["2030-08-08"]; ok || len(ps.WorkHistory) != 1 {
		t.Fatalf("work history outside the calendar kept: %v", ps.WorkHistory)
	}
	if ps.Entities["weaver"] != nil || ps.Entities["glazer"] == nil {
		t.Fatalf("entities not reconciled: %v", ps.Entities)
	}
	if v := ps.Entities["glazer"].Attributes["vigor"]; v != 2.0 {
		t.Fatalf("added entity did not take its authored value: %v", v)
	}
	if v := ps.Entities["potter"].Attributes["focus"]; v != 1.0 {
		t.Fatalf("added attribute did not take its initial value: %v", v)
	}
	if ps.Activities["loom"] != nil || ps.Activities["kiln"] == nil || ps.Activities["kiln"].Assignee != "glazer" {
		t.Fatalf("activities not reconciled: %+v", ps.Activities)
	}
	clay := ps.Activities["clay"]
	if clay.Assignee != "" || clay.Progress != 3 || clay.Completed != "2030-08-05" {
		t.Fatalf("clay not reconciled with its removed assignee and smaller effort: %+v", clay)
	}
	if ps.Ledgers["hours"] != nil || ps.Ledgers["clay_used"] == nil {
		t.Fatalf("ledgers not reconciled: %v", ps.Ledgers)
	}
	if !got.LoadReport.Changed() {
		t.Fatal("repairs not reported")
	}
	var report []string
	for _, c := range got.LoadReport.Changes {
		report = append(report, c.Path+"/"+c.ID)
	}
	joined := strings.Join(report, " ")
	for _, want := range []string{"planning/entities/weaver", "planning/entities/glazer", "planning/activities/loom", "planning/activities/kiln", "planning/ledgers/hours", "planning/date/2030-08-08"} {
		if !strings.Contains(joined, want) {
			t.Errorf("report misses %s: %s", want, joined)
		}
	}
	if _, err := got.Save(); err != nil {
		t.Fatalf("reconciled state does not save: %v", err)
	}

	// Without planning definitions the saved state is dropped, not refused.
	plain := testgame.Load(t, nil)
	got, err = rpg.Parse(plain, b, rand.New(rand.NewPCG(1, 1))) //nolint:gosec // Deterministic test.
	if err != nil || got.Planning != nil || !got.LoadReport.Changed() {
		t.Fatalf("planning removed: %v %+v", err, got)
	}
}

// TestPlanningSaveStillRefusesDamage keeps the validation for values that
// no definition change explains.
func TestPlanningSaveStillRefusesDamage(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = workshopPlanning()
	st := testgame.State(db, 3)
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ from, to string }{
		{`"progress": 0`, `"progress": -1`},
		{`"progress": 0`, `"progress": 999`},
		{`"completed": ""`, `"completed": "2030-07-31"`},
		{`"date": "2030-07-31"`, `"date": "2030-13-45"`},
	} {
		damaged := strings.Replace(string(b), tc.from, tc.to, 1)
		if damaged == string(b) {
			t.Fatalf("fixture lacks %s", tc.from)
		}
		if _, err := rpg.Parse(db, []byte(damaged), rand.New(rand.NewPCG(1, 1))); err == nil { //nolint:gosec // Deterministic test.
			t.Errorf("damaged save accepted: %s", tc.to)
		}
	}
}
