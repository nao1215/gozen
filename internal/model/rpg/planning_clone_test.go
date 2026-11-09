package rpg

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestPlanningCloneMatchesJSONRoundTrip: Clone gives what the JSON round trip
// ApplyIn used to make, including number types and empty omitempty fields.
func TestPlanningCloneMatchesJSONRoundTrip(t *testing.T) {
	p := &PlanningState{
		WorkHistory: map[string]WorkSample{},
		Date:        "2026-01-05", Phase: 2,
		Entities: map[string]*EntityState{
			"a": {Attributes: map[string]any{"n": 3, "s": "x", "b": true, "l": []any{1.5, "y"}, "m": map[string]any{"k": 2}}, Observations: map[string]any{}},
			"b": {},
		},
		Activities: map[string]*ActivityState{"t": {Attributes: map[string]any{}, Observed: 1, Assignee: "a"}, "u": {Attributes: map[string]any{"q": 4.0}}},
		Ledgers:    map[string]map[string]map[string]float64{"h": {"a": {"2026-01": 2}}, "e": nil},
		Closed:     map[string]bool{"2026-01-01": true},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var want PlanningState
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	got := p.Clone()
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("clone differs:\n got %#v\nwant %#v", *got, want)
	}
	got.Entities["a"].Attributes["n"] = 9.0
	if p.Entities["a"].Attributes["n"] != 3 {
		t.Fatal("clone shares a map with the original")
	}
}
