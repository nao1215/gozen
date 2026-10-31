package rpg

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/nao1215/gozen/internal/expr"
)

// PlanningState is saved within the existing state envelope. Rules own mutations;
// presentation consumes observations instead of Attributes.
type PlanningState struct {
	Date       string                                   `json:"date"`
	Phase      int                                      `json:"phase"`
	Finished   bool                                     `json:"finished"`
	Entities   map[string]*EntityState                  `json:"entities"`
	Activities map[string]*ActivityState                `json:"activities"`
	Ledgers    map[string]map[string]map[string]float64 `json:"ledgers"`
	Closed     map[string]bool                          `json:"closed"`
	Dispatched map[string]bool                          `json:"dispatched"`
}

// EntityState retains true attributes and explicitly refreshed observations.
type EntityState struct {
	Attributes   map[string]any `json:"attributes"`
	Observations map[string]any `json:"observations"`
}

// ActivityState tracks assignment and reversible completion independently of actors.
type ActivityState struct {
	Deadline  string  `json:"deadline,omitempty"`  // Empty retains the authored end date.
	Cancelled bool    `json:"cancelled,omitempty"` // Excluded work retains its budget and progress.
	Observed  float64 `json:"observed"`
	Effort    float64 `json:"effort,omitempty"` // Zero retains the authored effort for older saves.

	Assignee  string  `json:"assignee"`
	Progress  float64 `json:"progress"`
	Completed string  `json:"completed"`
}

// ActivityEffort supplies the current work budget to rules and presentation.
// A zero saved override uses the authored budget, preserving older saves.
// Unknown IDs return zero; callers must validate activity references first.
func (s *State) ActivityEffort(id string) float64 {
	if s.Planning != nil {
		if a := s.Planning.Activities[id]; a != nil && a.Effort != 0 {
			return a.Effort
		}
	}
	if s.DB.Game.Planning != nil {
		return s.DB.Game.Planning.Activities[id].Effort
	}
	return 0
}

// ActivityDeadline supplies the saved end date to planning expressions and timelines.
// An empty override retains the authored date; unknown activities return empty.
func (s *State) ActivityDeadline(id string) string {
	if s.Planning != nil {
		if a := s.Planning.Activities[id]; a != nil && a.Deadline != "" {
			return a.Deadline
		}
	}
	if s.DB.Game.Planning != nil {
		return s.DB.Game.Planning.Activities[id].To
	}
	return ""
}

func (s *State) initPlanning() {
	p := s.DB.Game.Planning
	if p == nil || s.Planning != nil {
		return
	}
	st := &PlanningState{Date: p.Calendar.Start, Entities: map[string]*EntityState{}, Activities: map[string]*ActivityState{}, Ledgers: map[string]map[string]map[string]float64{}, Closed: map[string]bool{}, Dispatched: map[string]bool{}}
	for id, e := range p.Entities {
		values := map[string]any{}
		for key, a := range p.Attributes {
			v := a.Initial
			if x, ok := e.Attributes[key]; ok {
				v = x
			}
			normalized, err := a.Value(v)
			if err != nil {
				values[key] = v
			} else {
				values[key] = normalized
			}
		}
		st.Entities[id] = &EntityState{Attributes: values, Observations: map[string]any{}}
	}
	for id, a := range p.Activities {
		st.Activities[id] = &ActivityState{Assignee: a.Assignee}
	}
	for id := range p.Ledgers {
		st.Ledgers[id] = map[string]map[string]float64{}
	}
	s.Planning = st
}

// EntityValues returns a detached expression object. full is reserved for rules
// and explicit diagnostic consumers; ordinary views must pass false.
func (s *State) EntityValues(id string, full bool) expr.Map {
	out := expr.Map{"id": id}
	p := s.DB.Game.Planning
	if p == nil || s.Planning == nil {
		return out
	}
	def, ok := p.Entities[id]
	if !ok {
		return out
	}
	st := s.Planning.Entities[id]
	if st == nil {
		return out
	}
	out["name"] = def.Name
	for key, value := range st.Attributes {
		if full || !p.Attributes[key].Hidden {
			out[key] = value
		}
	}
	maps.Copy(out, st.Observations)
	return out
}

func (s *State) planningLookup(name string) (any, bool) {
	p := s.DB.Game.Planning
	st := s.Planning
	if p == nil || st == nil {
		return nil, false
	}
	out := expr.Map{}
	switch name {
	case "period_value":
		return expr.Func(s.periodValue), true
	case "planning":
		d, err := time.Parse(time.DateOnly, st.Date)
		if err != nil {
			return nil, false
		}
		return expr.Map{"date": st.Date, "period": d.Format("2006-01"), "weekday": float64(d.Weekday()), "phase": p.Calendar.Phases[st.Phase], "finished": st.Finished, "closed": s.PlanningClosedDay()}, true
	case "entity":
		for id := range st.Entities {
			out[id] = s.EntityValues(id, true)
		}
	case "activity":
		for id, a := range st.Activities {
			out[id] = expr.Map{"deadline": s.ActivityDeadline(id), "progress": a.Progress, "effort": s.ActivityEffort(id), "assignee": a.Assignee, "completed": a.Completed != "", "cancelled": a.Cancelled}
		}
	}
	return out, true
}

// PlanningClosedDay is shared by phase advancement and calendar presentation.
func (s *State) PlanningClosedDay() bool {
	if s.Planning == nil {
		return false
	}
	d, err := time.Parse(time.DateOnly, s.Planning.Date)
	if err != nil {
		return false
	}
	c := s.DB.Game.Planning.Calendar
	return slices.Contains(c.WeekdaysOff, int(d.Weekday())) || slices.Contains(c.Closed, s.Planning.Date)
}

func (s *State) validatePlanning() error {
	st := s.Planning
	if st == nil {
		return nil
	}
	p := s.DB.Game.Planning
	if p == nil {
		return errors.New("planning: definitions missing")
	}
	d, err := time.Parse(time.DateOnly, st.Date)
	if err != nil || d.Format(time.DateOnly) != st.Date || st.Date < p.Calendar.Start || st.Date > p.Calendar.End || st.Phase < 0 || st.Phase >= len(p.Calendar.Phases) {
		return errors.New("planning: invalid date or phase")
	}
	if len(st.Entities) != len(p.Entities) || len(st.Activities) != len(p.Activities) || len(st.Ledgers) != len(p.Ledgers) || st.Closed == nil || st.Dispatched == nil {
		return errors.New("planning: incomplete state")
	}
	for _, id := range slices.Sorted(maps.Keys(st.Entities)) {
		e := st.Entities[id]
		_, ok := p.Entities[id]
		if !ok || e == nil || len(e.Attributes) != len(p.Attributes) || e.Observations == nil {
			return fmt.Errorf("planning.entities.%s: invalid state", id)
		}
		for k, a := range p.Attributes {
			if _, err := a.Value(e.Attributes[k]); err != nil {
				return fmt.Errorf("planning.entities.%s.%s: %w", id, k, err)
			}
		}
		for k, v := range e.Observations {
			if _, ok := p.Entities[id].Observations[k]; !ok || expr.TypeOfValue(v) == expr.UnknownType {
				return fmt.Errorf("planning.entities.%s.observations.%s: invalid observation", id, k)
			}
		}
	}
	for id, a := range st.Activities {
		_, ok := p.Activities[id]
		effort := s.ActivityEffort(id)
		if !ok || a == nil || math.IsNaN(a.Effort) || math.IsInf(a.Effort, 0) || a.Effort < 0 || math.IsNaN(a.Progress) || math.IsInf(a.Progress, 0) || a.Progress < 0 || a.Progress > effort || math.IsNaN(a.Observed) || math.IsInf(a.Observed, 0) || a.Observed < 0 || a.Observed > effort {
			return fmt.Errorf("planning.activities.%s: invalid progress", id)
		}
		if a.Deadline != "" {
			d, err := time.Parse(time.DateOnly, a.Deadline)
			if err != nil || d.Format(time.DateOnly) != a.Deadline || a.Deadline < p.Activities[id].From {
				return fmt.Errorf("planning.activities.%s: invalid deadline", id)
			}
		}
		if a.Assignee != "" {
			if _, ok := st.Entities[a.Assignee]; !ok {
				return fmt.Errorf("planning.activities.%s: unknown assignee", id)
			}
		}
		if (a.Completed != "") != (a.Progress == effort) {
			return fmt.Errorf("planning.activities.%s: invalid completion", id)
		}
		if a.Completed != "" {
			if _, err := time.Parse(time.DateOnly, a.Completed); err != nil {
				return fmt.Errorf("planning.activities.%s: invalid completion date", id)
			}
		}
	}
	for id, periods := range st.Ledgers {
		if _, ok := p.Ledgers[id]; !ok || periods == nil {
			return fmt.Errorf("planning.ledgers.%s: unknown ledger", id)
		}
		for period, owners := range periods {
			if _, err := time.Parse("2006-01", period); err != nil || owners == nil {
				return fmt.Errorf("planning.ledgers.%s: invalid period", id)
			}
			for owner, n := range owners {
				if _, ok := st.Entities[owner]; !ok || math.IsNaN(n) || math.IsInf(n, 0) {
					return fmt.Errorf("planning.ledgers.%s: invalid owner or total", id)
				}
			}
		}
	}
	return nil
}

// periodValue exposes retained monthly history to authored constraints.
func (s *State) periodValue(args []any) (any, error) {
	if len(args) != 3 {
		return nil, errors.New("period_value needs ledger, entity and month offset")
	}
	ledger, ok := args[0].(string)
	if !ok {
		return nil, errors.New("period_value ledger must be a string")
	}
	owner, ok := args[1].(string)
	if !ok {
		return nil, errors.New("period_value entity must be a string")
	}
	offset, ok := expr.ToNumber(args[2])
	if !ok || expr.TypeOfValue(args[2]) != expr.NumberType || math.IsNaN(offset) || offset != math.Trunc(offset) || offset < -120 || offset > 0 {
		return nil, errors.New("period_value offset must be an integer from -120 to 0")
	}
	if _, ok := s.Planning.Ledgers[ledger]; !ok {
		return nil, errors.New("period_value unknown ledger")
	}
	if s.Planning.Entities[owner] == nil {
		return nil, errors.New("period_value unknown entity")
	}
	date, err := time.Parse("2006-01", s.Planning.Date[:7])
	if err != nil {
		return nil, err
	}
	return s.Planning.Ledgers[ledger][date.AddDate(0, int(offset), 0).Format("2006-01")][owner], nil
}
