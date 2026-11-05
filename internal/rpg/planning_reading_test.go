package rpg_test

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// While the state is only read (Reading), expressions get the same values as
// without it; a change made after the read is seen by the next one.
func TestPlanningReadingSameValues(t *testing.T) {
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
	sources := []string{`sum(activities, it.effort)`, `count(entities, it.id == "potter")`, `entity.potter.name + activity.shape.assignee`, `sum(activities, it.effort) + count(activities, it.assignee == "weaver")`}
	plain := make([]any, len(sources))
	for i, source := range sources {
		v, err := mustExpr(t, source).Eval(st.Env())
		if err != nil {
			t.Fatal(err)
		}
		plain[i] = v
	}
	st.Reading(func() {
		for i, source := range sources {
			for range 2 {
				v, err := mustExpr(t, source).Eval(st.Env())
				if err != nil || v != plain[i] {
					t.Errorf("%s while reading = %v, %v; want %v", source, v, err, plain[i])
				}
			}
		}
	})
	st.Planning.Activities["pack"].Assignee = "potter"
	st.Reading(func() {
		v, err := mustExpr(t, `count(activities, it.assignee == "potter")`).Eval(st.Env())
		if err != nil || v != 2.0 {
			t.Errorf("a read after a change = %v, %v; want 2", v, err)
		}
	})
}
