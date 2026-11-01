package rpg_test

import (
	"bytes"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestPlanningCalendarCounts(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-07-31", End: "2030-08-04", Phases: []string{"day"}, WeekdaysOff: []int{0, 6}, Closed: []string{"2030-08-01", "2030-08-03"}}}
	st := testgame.State(db, 17)
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source string
		want   float64
	}{
		{`calendar_days("2030-07-31", "2030-07-31")`, 0},
		{`calendar_days("2030-07-31", "2030-08-04")`, 4},
		{`calendar_days("2030-08-04", "2030-07-31")`, -4},
		{`calendar_days("2028-02-28", "2028-03-01")`, 2},
		{`calendar_workdays("2030-07-31", "2030-08-04")`, 2},
		{`calendar_workdays("2030-07-31", "2030-08-06")`, 4},
		{`calendar_workdays("0001-01-01", "9999-12-31")`, 2608614},
		{`calendar_workdays("2030-08-02", "2030-08-02")`, 1},
		{`calendar_workdays("2030-08-01", "2030-08-01")`, 0},
		{`calendar_workdays("2030-08-04", "2030-07-31")`, 0},
		{`calendar_workdays("2030-08-05", "2030-08-11")`, 5},
		{`calendar_days("0001-01-01", "9999-12-31")`, 3652058},
	} {
		got, err := mustExpr(t, tc.source).Eval(st.Env())
		if err != nil || got != tc.want {
			t.Errorf("%s = %v, %v; want %v", tc.source, got, err, tc.want)
		}
	}
	for _, source := range []string{`calendar_days("2030-02-30", "2030-03-01")`, `calendar_days(0, "2030-03-01")`, `calendar_days("2030-03-01")`, `calendar_workdays("2030-03-01", false)`, `calendar_workdays("2030-03-01", "2030-3-02")`} {
		if _, err := mustExpr(t, source).Eval(st.Env()); err == nil {
			t.Errorf("%s accepted", source)
		}
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("calendar reads changed saved state: %v", err)
	}
	// Duplicate closures and a closure already on a weekend count only once.
	db.Game.Planning.Calendar.WeekdaysOff = []int{0, 6, 6}
	db.Game.Planning.Calendar.Closed = []string{"2030-08-01", "2030-08-01", "2030-08-03"}
	got, err := mustExpr(t, `calendar_workdays("2030-07-31", "2030-08-04")`).Eval(st.Env())
	if err != nil || got != 2.0 {
		t.Fatalf("duplicate closures: %v, %v", got, err)
	}
}

func TestPlanningCalendarShift(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-01-01", End: "2030-01-02", Phases: []string{"day"}}}
	st := testgame.State(db, 17)
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, want string }{
		{`calendar_shift("2028-02-28", 1)`, "2028-02-29"},
		{`calendar_shift("2028-03-01", -1)`, "2028-02-29"},
		{`calendar_shift("2030-12-31", 1)`, "2031-01-01"},
		{`calendar_shift("0001-01-01", 3652058)`, "9999-12-31"},
		{`calendar_shift("9999-12-31", -3652058)`, "0001-01-01"},
		{`calendar_shift("2030-01-01", 0)`, "2030-01-01"},
	} {
		got, err := mustExpr(t, tc.source).Eval(st.Env())
		if err != nil || got != tc.want {
			t.Errorf("%s = %v, %v; want %s", tc.source, got, err, tc.want)
		}
	}
	for _, source := range []string{
		`calendar_shift("0001-01-01", -1)`, `calendar_shift("9999-12-31", 1)`,
		`calendar_shift("2030-01-01", 1.5)`, `calendar_shift("2030-01-01", 999999999999999999999999999999)`,
		`calendar_shift("2030-02-30", 1)`, `calendar_shift("0000-01-01", 1)`,
		`calendar_shift("2030-1-01", 1)`, `calendar_shift(0, 1)`,
		`calendar_shift("2030-01-01", "1")`, `calendar_shift("2030-01-01", true)`,
		`calendar_shift("2030-01-01")`, `calendar_shift("2030-01-01", 1, 2)`,
	} {
		if _, err := mustExpr(t, source).Eval(st.Env()); err == nil {
			t.Errorf("%s accepted", source)
		}
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("date arithmetic changed saved state: %v", err)
	}
}

func TestPlanningCollections(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar: data.Calendar{Start: "2030-01-01", End: "2030-01-03", Phases: []string{"day"}},
		Entities: map[string]data.Entity{"weaver": {Name: "Weaver"}, "potter": {Name: "Potter"}},
		Activities: map[string]data.Activity{
			"shape": {Effort: 3, From: "2030-01-01", To: "2030-01-02", Assignee: "potter"},
			"pack":  {Effort: 2, From: "2030-01-02", To: "2030-01-03", Assignee: "weaver"},
		},
	}
	st := testgame.State(db, 17)
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source string
		want   any
	}{
		{`sum(activities, it.effort)`, 5.0},
		{`count(entities, it.id == "potter")`, 1.0},
		{`sum(filter(activities, it.assignee == "potter"), it.effort)`, 3.0},
		{`activities[0].id`, "pack"},
		{`entities[0].id`, "potter"},
		{`sum(activities, calendar_days(it.from,it.to))`, 2.0},
	} {
		got, err := mustExpr(t, tc.source).Eval(st.Env())
		if err != nil || got != tc.want {
			t.Errorf("%s = %v, %v; want %v", tc.source, got, err, tc.want)
		}
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("collection changed save: %v", err)
	}
}

func TestActivityAuthoredEffortSurvivesResize(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-01-01", End: "2030-01-02", Phases: []string{"day"}}, Activities: map[string]data.Activity{"shape": {Effort: 3}}}
	st := testgame.State(db, 17)
	st.Planning.Activities["shape"].Effort = 7
	got, err := mustExpr(t, `sum(activities, it.effort-it.planned_effort)`).Eval(st.Env())
	if err != nil || got != 4.0 {
		t.Fatalf("authored effort changed: %v, %v", got, err)
	}
}
