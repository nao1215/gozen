package rpg

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"time"
)

// reconcilePlanning fits a saved planning state to the current definitions
// before validation, so adding or removing an entity, activity, ledger or
// attribute in a newer release does not lock players out of their saves.
// Missing records take the same initial values as a new game (initPlanning);
// records whose definition is gone are dropped. Every repair is listed in
// LoadReport. Values that are invalid for definitions that did not change
// (NaN, out of range, unknown references) are left for validatePlanning.
func (s *State) reconcilePlanning() error {
	st := s.Planning
	if st == nil {
		return nil
	}
	p := s.DB.Game.Planning
	if p == nil {
		s.Planning = nil
		s.changed("planning", "", "planning definitions removed")
		return nil
	}
	if st.Entities == nil {
		st.Entities = map[string]*EntityState{}
	}
	if st.Activities == nil {
		st.Activities = map[string]*ActivityState{}
	}
	if st.Ledgers == nil {
		st.Ledgers = map[string]map[string]map[string]float64{}
	}
	if st.Closed == nil {
		st.Closed = map[string]bool{}
	}
	if st.Dispatched == nil {
		st.Dispatched = map[string]bool{}
	}
	// Validate limits from the original definition before any migration.
	if st.Basis != nil {
		for id, a := range st.Activities {
			old, ok := st.Basis.Efforts[id]
			if !ok || a == nil {
				continue
			}
			if a.Effort != 0 {
				old = a.Effort
			}
			if old <= 0 || math.IsNaN(old) || math.IsInf(old, 0) || a.Progress < 0 || a.Progress > old || a.Observed < 0 || a.Observed > old || (a.Completed != "") != (a.Progress == old) {
				return fmt.Errorf("planning.activities.%s: invalid saved progress or completion", id)
			}
		}
	}
	if st.Basis != nil && (st.Basis.Start != p.Calendar.Start || st.Basis.End != p.Calendar.End) {
		s.reconcileCalendar()
	}

	for _, id := range slices.Sorted(maps.Keys(st.Entities)) {
		if _, ok := p.Entities[id]; !ok {
			delete(st.Entities, id)
			s.changed("planning/entities", id, "entity definition removed")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Entities)) {
		def := p.Entities[id]
		e := st.Entities[id]
		if e == nil {
			e = &EntityState{Attributes: map[string]any{}, Observations: map[string]any{}}
			st.Entities[id] = e
			s.changed("planning/entities", id, "entity added with its initial values")
		}
		if e.Attributes == nil {
			e.Attributes = map[string]any{}
		}
		if e.Observations == nil {
			e.Observations = map[string]any{}
		}
		for _, key := range slices.Sorted(maps.Keys(e.Attributes)) {
			if _, ok := p.Attributes[key]; !ok {
				delete(e.Attributes, key)
				s.changed("planning/entities/"+id+"/attributes", key, "attribute definition removed")
			}
		}
		for _, key := range slices.Sorted(maps.Keys(p.Attributes)) {
			if _, ok := e.Attributes[key]; ok {
				continue
			}
			a := p.Attributes[key]
			v := a.Initial
			if x, ok := def.Attributes[key]; ok {
				v = x
			}
			if normalized, err := a.Value(v); err == nil {
				v = normalized
			}
			e.Attributes[key] = v
			s.changed("planning/entities/"+id+"/attributes", key, "attribute added with its initial value")
		}
		for _, key := range slices.Sorted(maps.Keys(e.Observations)) {
			if _, ok := def.Observations[key]; !ok {
				delete(e.Observations, key)
				s.changed("planning/entities/"+id+"/observations", key, "observation definition removed")
			}
		}
	}

	for _, id := range slices.Sorted(maps.Keys(st.Activities)) {
		if _, ok := p.Activities[id]; !ok {
			delete(st.Activities, id)
			s.changed("planning/activities", id, "activity definition removed")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Activities)) {
		def := p.Activities[id]
		a := st.Activities[id]
		if a == nil {
			st.Activities[id] = &ActivityState{Assignee: def.Assignee}
			s.changed("planning/activities", id, "activity added with its initial values")
			continue
		}
		for _, key := range slices.Sorted(maps.Keys(a.Attributes)) {
			if _, ok := p.ActivityAttributes[key]; !ok {
				delete(a.Attributes, key)
				s.changed("planning/activities/"+id+"/attributes", key, "activity attribute definition removed")
			}
		}
		if a.Assignee != "" {
			if _, ok := p.Entities[a.Assignee]; !ok {
				s.changed("planning/activities/"+id+"/assignee", a.Assignee, "assignee definition removed")
				a.Assignee = ""
			}
		}
		if st.Basis != nil && st.Basis.From[id] != def.From && a.Deadline != "" && a.Deadline < def.From {
			s.changed("planning/activities/"+id+"/deadline", a.Deadline, "deadline precedes the authored start; the authored end is used")
			a.Deadline = ""
		}
		// Only an authored budget can change under a save; a saved override is kept.
		if old, known := savedEffort(st.Basis, id); a.Effort == 0 && known && old != def.Effort {
			effort := def.Effort
			if a.Progress > effort {
				s.changed("planning/activities/"+id+"/progress", "", "progress exceeds the revised effort")
				a.Progress = effort
			}
			if a.Observed > effort {
				a.Observed = effort
			}
			switch {
			case a.Completed == "" && a.Progress == effort:
				a.Completed = st.Date
				s.changed("planning/activities/"+id+"/completed", st.Date, "completed under the revised effort")
			case a.Completed != "" && a.Progress != effort:
				s.changed("planning/activities/"+id+"/completed", a.Completed, "reopened under the revised effort")
				a.Completed = ""
			}
		}
	}

	for _, id := range slices.Sorted(maps.Keys(st.Ledgers)) {
		if _, ok := p.Ledgers[id]; !ok {
			delete(st.Ledgers, id)
			s.changed("planning/ledgers", id, "ledger definition removed")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Ledgers)) {
		periods := st.Ledgers[id]
		if periods == nil {
			if _, saved := st.Ledgers[id]; !saved {
				s.changed("planning/ledgers", id, "ledger added")
			}
			st.Ledgers[id] = map[string]map[string]float64{}
			continue
		}
		for _, period := range slices.Sorted(maps.Keys(periods)) {
			for _, owner := range slices.Sorted(maps.Keys(periods[period])) {
				if _, ok := p.Entities[owner]; !ok {
					delete(periods[period], owner)
					s.changed("planning/ledgers/"+id+"/"+period, owner, "ledger owner definition removed")
				}
			}
		}
	}
	st.Basis = nil // migration metadata is not live gameplay state
	return nil
}

func savedEffort(b *planningBasis, id string) (float64, bool) {
	if b == nil {
		return 0, false
	}
	v, ok := b.Efforts[id]
	return v, ok
}

// reconcileCalendar moves a saved date into a calendar whose start or end
// was revised, and drops work samples that now fall outside it. A phase out of
// range on an unchanged date is damage, left for validatePlanning.
func (s *State) reconcileCalendar() {
	st := s.Planning
	c := s.DB.Game.Planning.Calendar
	if _, err := time.Parse(time.DateOnly, st.Date); err == nil {
		switch {
		case c.End != "" && st.Date > c.End:
			s.changed("planning/date", st.Date, "date after the revised calendar end")
			st.Date = c.End
			st.Phase = max(0, len(c.Phases)-1)
		case c.Start != "" && st.Date < c.Start:
			s.changed("planning/date", st.Date, "date before the revised calendar start")
			st.Date, st.Phase = c.Start, 0
		}
	}
	for _, date := range slices.Sorted(maps.Keys(st.WorkHistory)) {
		if _, err := time.Parse(time.DateOnly, date); err == nil && (date < c.Start || c.End != "" && date > c.End) {
			delete(st.WorkHistory, date)
			s.changed("planning/work_history", date, "work sample outside the revised calendar")
		}
	}
}
