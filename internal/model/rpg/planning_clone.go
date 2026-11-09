package rpg

import (
	"encoding/json"
	"maps"
)

// Clone copies the planning state as a JSON round trip would (ApplyIn's
// transaction), without encoding: maps and slices are copied, numbers become
// float64, and empty omitempty collections read back as nil.
func (p *PlanningState) Clone() *PlanningState {
	next := &PlanningState{Date: p.Date, Phase: p.Phase, Finished: p.Finished}
	if len(p.WorkHistory) > 0 {
		next.WorkHistory = maps.Clone(p.WorkHistory)
	}
	if p.Entities != nil {
		next.Entities = make(map[string]*EntityState, len(p.Entities))
		for id, e := range p.Entities {
			if e == nil {
				next.Entities[id] = nil
				continue
			}
			next.Entities[id] = &EntityState{Attributes: cloneJSONMap(e.Attributes), Observations: cloneJSONMap(e.Observations)}
		}
	}
	if p.Activities != nil {
		next.Activities = make(map[string]*ActivityState, len(p.Activities))
		for id, a := range p.Activities {
			if a == nil {
				next.Activities[id] = nil
				continue
			}
			c := *a
			c.Attributes = nil
			if len(a.Attributes) > 0 {
				c.Attributes = cloneJSONMap(a.Attributes)
			}
			next.Activities[id] = &c
		}
	}
	if p.Ledgers != nil {
		next.Ledgers = make(map[string]map[string]map[string]float64, len(p.Ledgers))
		for k, owners := range p.Ledgers {
			if owners == nil {
				next.Ledgers[k] = nil
				continue
			}
			o := make(map[string]map[string]float64, len(owners))
			for owner, periods := range owners {
				o[owner] = maps.Clone(periods)
			}
			next.Ledgers[k] = o
		}
	}
	next.Closed = maps.Clone(p.Closed)
	next.Dispatched = maps.Clone(p.Dispatched)
	return next
}

func cloneJSONMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneJSONValue(v)
	}
	return out
}

// cloneJSONValue is v after encoding/json would encode and decode it into any.
func cloneJSONValue(v any) any {
	switch x := v.(type) {
	case nil, string, bool, float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float32:
		return float64(x)
	case map[string]any:
		return cloneJSONMap(x)
	case []any:
		if x == nil {
			return nil
		}
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneJSONValue(e)
		}
		return out
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}
