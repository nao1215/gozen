package rpg_test

import (
	"bytes"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
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

func TestEntityDefinitionData(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Calendar: data.Calendar{Start: "2030-01-01", End: "2030-01-02", Phases: []string{"day"}}, Entities: map[string]data.Entity{
		"potter": {Data: map[string]any{"threshold": 40, "greetings": []any{"Hello", "Welcome"}}},
	}}
	st := testgame.State(db, 17)
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for source, want := range map[string]any{`entity.potter.data.threshold`: 40, `entity.potter.data.greetings[1]`: "Welcome", `count(entity.potter.data.greetings, true)`: 2.0} {
		got, err := mustExpr(t, source).Eval(st.Env())
		if err != nil || got != want {
			t.Fatalf("%s = %v, %v; want %v", source, got, err, want)
		}
	}
	// Consumers may edit a detached result without modifying authored definitions.
	values, ok := st.EntityValues("potter", true)["data"].(expr.Map)
	if !ok {
		t.Fatal("definition data is not an expression map")
	}
	values["threshold"] = 99
	greetings, ok := values["greetings"].([]any)
	if !ok {
		t.Fatal("greetings is not a list")
	}
	greetings[0] = "Changed"
	got, err := mustExpr(t, `entity.potter.data.greetings[0]`).Eval(st.Env())
	if err != nil || got != "Hello" {
		t.Fatalf("definition mutated through returned map: %v, %v", got, err)
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("definition read changed save: %v", err)
	}
	if bytes.Contains(after, []byte("Welcome")) {
		t.Fatal("definition data leaked into save")
	}
}

func TestEntityValueMatchesPublicObject(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar: data.Calendar{Start: "2030-01-01", End: "2030-01-02", Phases: []string{"day"}},
		Attributes: map[string]data.Attribute{
			"visible": {Type: "bool", Initial: true},
			"secret":  {Type: "number", Initial: 7, Hidden: true},
		},
		Entities: map[string]data.Entity{"potter": {Name: "Potter", Data: map[string]any{"items": []any{"bowl"}}}},
	}
	st := testgame.State(db, 17)
	st.Planning.Entities["potter"].Observations["visible"] = false
	for _, id := range []string{"potter", "missing"} {
		for _, key := range []string{"id", "name", "unknown", "visible", "secret"} {
			if got, want := st.EntityValue(id, key), st.EntityValues(id, false)[key]; got != want {
				t.Fatalf("%s.%s: %v != %v", id, key, got, want)
			}
		}
	}
	values, ok := st.EntityValue("potter", "data").(expr.Map)
	if !ok {
		t.Fatal("missing metadata")
	}
	items, ok := values["items"].([]any)
	if !ok {
		t.Fatal("missing list")
	}
	items[0] = "changed"
	original, ok := db.Game.Planning.Entities["potter"].Data["items"].([]any)
	if !ok || original[0] != "bowl" {
		t.Fatal("metadata aliased")
	}
}

func BenchmarkEntityPublicRead(b *testing.B) {
	db := testgame.Load(b, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"potter": {Name: "Potter"}}}
	st := testgame.State(db, 17)
	b.Run("object", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = st.EntityValues("potter", false)["name"]
		}
	})
	b.Run("field", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = st.EntityValue("potter", "name")
		}
	})
}
