// Package planning applies deterministic calendar, assignment and ledger rules.
// It has no presentation dependency and uses the shared saved RPG state.
package planning

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// Apply performs one atomic operation and returns scheduled event calls for the
// existing interpreter. Invalid expressions or targets leave progress unchanged.
// Selection visits stable entity IDs, with updates visible to subsequent entities.
func Apply(s *rpg.State, c data.PlanCommand) ([]data.Command, error) {
	if s.Planning == nil {
		return nil, errors.New("planning is not configured")
	}
	b, err := json.Marshal(s.Planning)
	if err != nil {
		return nil, err
	}
	var next rpg.PlanningState
	if err = json.Unmarshal(b, &next); err != nil {
		return nil, err
	}
	old := s.Planning
	s.Planning = &next
	cmds, err := apply(s, c, nil)
	if err != nil {
		s.Planning = old
		return nil, err
	}
	return cmds, nil
}

func apply(s *rpg.State, c data.PlanCommand, args expr.Env) ([]data.Command, error) {
	p, st := s.DB.Game.Planning, s.Planning
	base := expr.Chain{args, s.Env()}
	switch c.Op {
	case "rule":
		r, ok := p.Rules[c.Rule]
		if !ok || len(r.Do) > 256 || len(c.Args) != len(r.Params) {
			return nil, errors.New("invalid rule invocation")
		}
		params := expr.Map{}
		for _, key := range r.Params {
			e, ok := c.Args[key]
			if !ok {
				return nil, fmt.Errorf("missing argument %s", key)
			}
			v, err := e.Eval(base)
			if err != nil {
				return nil, err
			}
			if expr.TypeOfValue(v) == expr.UnknownType {
				return nil, errors.New("argument must be scalar")
			}
			params[key] = v
		}
		var out []data.Command
		for _, step := range r.Do {
			if step.Op == "rule" || step.Op == "policy" {
				return nil, errors.New("nested rule invocation")
			}
			cmds, err := apply(s, step, expr.Map{"arg": params})
			if err != nil {
				return nil, err
			}
			out = append(out, cmds...)
		}
		return out, nil
	case "policy":
		rules, ok := p.Policies[c.Policy]
		if !ok {
			return nil, errors.New("unknown policy")
		}
		entities := expr.Map{}
		for id := range st.Entities {
			entities[id] = s.EntityValues(id, false)
		}
		// Shadow hidden activity progress as well as entity attributes.
		activities := expr.Map{}
		for id, a := range st.Activities {
			activities[id] = expr.Map{"progress": a.Observed, "effort": s.ActivityEffort(id), "assignee": a.Assignee}
		}
		env := expr.Chain{expr.Map{"entity": entities, "activity": activities}, base}
		for _, r := range rules {
			yes, err := r.When.Bool(env)
			if err != nil {
				return nil, err
			}
			if yes {
				return []data.Command{{Call: r.Event}}, nil
			}
		}
		return nil, errors.New("policy has no matching rule")

	case "advance":
		if st.Finished {
			return nil, errors.New("calendar has finished")
		}
		if !s.PlanningClosedDay() && st.Phase+1 < len(p.Calendar.Phases) {
			st.Phase++
		} else if st.Date == p.Calendar.End {
			st.Finished = true
		} else {
			d, err := time.Parse(time.DateOnly, st.Date)
			if err != nil {
				return nil, err
			}
			st.Date = d.AddDate(0, 0, 1).Format(time.DateOnly)
			st.Phase = 0
		}
		return nil, nil
	case "dispatch":
		if st.Finished {
			return nil, nil
		}
		d, err := time.Parse(time.DateOnly, st.Date)
		if err != nil {
			return nil, err
		}
		var out []data.Command
		for _, r := range p.Schedule {
			key := st.Date + "/" + r.ID
			if st.Dispatched[key] || r.Phase != p.Calendar.Phases[st.Phase] || r.Date != "" && r.Date != st.Date || len(r.Weekdays) > 0 && !slices.Contains(r.Weekdays, int(d.Weekday())) {
				continue
			}
			st.Dispatched[key] = true
			out = append(out, data.Command{Call: r.Event})
		}
		return out, nil
	case "assign":
		a := st.Activities[c.Activity]
		if a == nil {
			return nil, fmt.Errorf("unknown activity %q", c.Activity)
		}
		if c.Entity != "" && st.Entities[c.Entity] == nil {
			return nil, fmt.Errorf("unknown entity %q", c.Entity)
		}
		a.Assignee = c.Entity
		return nil, nil
	case "close":
		if _, ok := p.Ledgers[c.Ledger]; !ok {
			return nil, errors.New("unknown ledger")
		}
		period := c.Period
		if period == "" {
			period = st.Date[:7]
		}
		if _, err := time.Parse("2006-01", period); err != nil {
			return nil, err
		}
		key := c.Ledger + "/" + period
		if st.Closed[key] {
			return nil, errors.New("period already closed")
		}
		st.Closed[key] = true
		totals := st.Ledgers[c.Ledger]
		if totals[period] == nil {
			totals[period] = map[string]float64{}
		}
		keys := slices.Sorted(maps.Keys(totals))
		for len(keys) > p.Ledgers[c.Ledger].Retain {
			delete(totals, keys[0])
			keys = keys[1:]
		}
		return nil, nil
	case "progress", "effort":
		a := st.Activities[c.Activity]
		if a == nil {
			return nil, fmt.Errorf("unknown activity %q", c.Activity)
		}
		env := expr.Chain{expr.Map{"self": s.EntityValues(a.Assignee, true)}, base}
		v, err := c.Value.Eval(env)
		if err != nil {
			return nil, err
		}
		n, ok := expr.ToNumber(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("%s requires a finite number", c.Op)
		}
		if c.Op == "effort" {
			if n <= 0 {
				return nil, errors.New("effort requires a positive number")
			}
			a.Effort = n
			a.Observed = min(a.Observed, n)
			n = 0
		}
		if n > 0 {
			for _, dep := range p.Activities[c.Activity].After {
				ratio := 1.0
				if p.Activities[c.Activity].DependencyRatio != nil {
					ratio = *p.Activities[c.Activity].DependencyRatio
				}
				if st.Activities[dep].Progress < s.ActivityEffort(dep)*ratio {
					return nil, nil
				}
			}
		}
		a.Progress = max(0, min(s.ActivityEffort(c.Activity), a.Progress+n))
		if a.Progress < s.ActivityEffort(c.Activity) {
			a.Completed = ""
		} else if a.Completed == "" {
			a.Completed = st.Date
		}
		if !p.Activities[c.Activity].Observation.Set() {
			a.Observed = a.Progress
		}
		return nil, nil
	case "set", "observe", "record":
	default:
		return nil, fmt.Errorf("unknown planning operation %q", c.Op)
	}
	if c.Op == "observe" {
		for _, id := range slices.Sorted(maps.Keys(p.Activities)) {
			def := p.Activities[id]
			a := st.Activities[id]
			if !def.Observation.Set() {
				a.Observed = a.Progress
				continue
			}
			env := expr.Chain{expr.Map{"self": expr.Map{"progress": a.Progress, "effort": s.ActivityEffort(id), "assignee": a.Assignee}}, base}
			v, err := def.Observation.Eval(env)
			if err != nil {
				return nil, err
			}
			n, ok := expr.ToNumber(v)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, errors.New("activity observation must be finite")
			}
			a.Observed = max(0, min(s.ActivityEffort(id), n))
		}
	}
	if c.Entity != "" && st.Entities[c.Entity] == nil {
		return nil, fmt.Errorf("unknown entity %q", c.Entity)
	}
	for _, id := range slices.Sorted(maps.Keys(st.Entities)) {
		if c.Entity != "" && id != c.Entity || c.Tag != "" && !slices.Contains(p.Entities[id].Tags, c.Tag) {
			continue
		}
		env := expr.Chain{expr.Map{"self": s.EntityValues(id, true)}, base}
		yes, err := c.Where.Bool(env)
		if err != nil {
			return nil, err
		}
		if !yes {
			continue
		}
		if c.Op == "observe" {
			values := map[string]any{}
			for _, key := range slices.Sorted(maps.Keys(p.Entities[id].Observations)) {
				v, err := p.Entities[id].Observations[key].Eval(env)
				if err != nil {
					return nil, err
				}
				if expr.TypeOfValue(v) == expr.UnknownType {
					return nil, fmt.Errorf("observation %s must be scalar", key)
				}
				values[key] = v
			}
			st.Entities[id].Observations = values
			continue
		}
		v, err := c.Value.Eval(env)
		if err != nil {
			return nil, err
		}
		if c.Op == "set" {
			a, ok := p.Attributes[c.Attribute]
			if !ok {
				return nil, errors.New("unknown attribute")
			}
			v, err = a.Value(v)
			if err != nil {
				return nil, err
			}
			st.Entities[id].Attributes[c.Attribute] = v
			continue
		}
		if _, ok := p.Ledgers[c.Ledger]; !ok {
			return nil, errors.New("unknown ledger")
		}
		period := c.Period
		if period == "" {
			period = st.Date[:7]
		}
		if _, err := time.Parse("2006-01", period); err != nil {
			return nil, err
		}
		if st.Closed[c.Ledger+"/"+period] {
			return nil, errors.New("period already closed")
		}
		n, ok := expr.ToNumber(v)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, errors.New("ledger requires finite number")
		}
		totals := st.Ledgers[c.Ledger]
		if totals[period] == nil {
			totals[period] = map[string]float64{}
		}
		n += totals[period][id]
		if math.IsInf(n, 0) {
			return nil, errors.New("ledger overflow")
		}
		totals[period][id] = n
	}
	return nil, nil
}
