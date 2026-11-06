// Package planning applies deterministic calendar, assignment and ledger rules.
// It has no presentation dependency and uses the shared saved RPG state.
package planning

import (
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

// ApplyIn performs one atomic operation using the interpreter's argument scope.
// Rule bodies replace that scope with their own parameters. Errors leave state
// unchanged; returned commands are dispatched by the existing event interpreter.
func ApplyIn(s *rpg.State, c data.PlanCommand, args expr.Env) ([]data.Command, error) {
	if s.Planning == nil {
		return nil, errors.New("planning is not configured")
	}
	old := s.Planning
	s.Planning = old.Clone()
	cmds, err := apply(s, c, args)
	if err != nil {
		s.Planning = old
		return nil, err
	}
	return cmds, nil
}

func apply(s *rpg.State, c data.PlanCommand, args expr.Env) ([]data.Command, error) {
	before := map[string]rpg.ActivityState{}
	for id, a := range s.Planning.Activities {
		before[id] = *a
	}
	out, err := applyCommand(s, c, args)
	if err == nil && c.Op != "rule" {
		err = reconcileParts(s, before, c.Op)
	}
	return out, err
}

func applyCommand(s *rpg.State, c data.PlanCommand, args expr.Env) ([]data.Command, error) {
	p, st := s.DB.Game.Planning, s.Planning
	base := expr.Chain{args, s.Env()}
	entity, err := c.Entity.Resolve(s.DB, "entity", base)
	if err != nil {
		return nil, err
	}
	activity, err := c.Activity.Resolve(s.DB, "activity", base)
	if err != nil {
		return nil, err
	}

	switch c.Op {
	case "split":
		a := st.Activities[activity]
		if a == nil || p.Activities[activity].Parent != "" {
			return nil, errors.New("split requires a root activity")
		}
		children := childIDs(s, activity)
		if len(children) < 2 {
			return nil, errors.New("split requires authored children")
		}
		if a.Split {
			return nil, nil
		}
		for _, id := range children {
			ch := st.Activities[id]
			ratio := p.Activities[id].Effort / p.Activities[activity].Effort
			ch.Effort = s.ActivityEffort(activity) * ratio
			ch.Progress, ch.Observed = a.Progress*ratio, a.Observed*ratio
			ch.Cancelled, ch.Deadline, ch.Completed = a.Cancelled, a.Deadline, a.Completed
		}
		a.Split, a.Assignee = true, ""
		return nil, nil

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
			if !expr.IsFiniteScalar(v) {
				return nil, fmt.Errorf("argument %s must be a finite scalar", key)
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
		entityList := make([]any, 0, len(st.Entities))
		for _, id := range slices.Sorted(maps.Keys(st.Entities)) {
			entities[id] = s.EntityValues(id, false)
			entityList = append(entityList, entities[id])
		}
		// Shadow hidden activity progress as well as entity attributes.
		activities := expr.Map{}
		activityList := make([]any, 0, len(st.Activities))
		for _, id := range slices.Sorted(maps.Keys(st.Activities)) {
			a := st.Activities[id]
			activities[id] = expr.Map{"parent": p.Activities[id].Parent, "split": a.Split, "active": s.ActivityValues(id)["active"], "id": id, "deadline": s.ActivityDeadline(id), "progress": a.Observed, "observed": a.Observed, "effort": s.ActivityEffort(id), "assignee": a.Assignee, "cancelled": a.Cancelled}
			activityList = append(activityList, activities[id])
		}
		env := expr.Chain{expr.Map{"entity": entities, "activity": activities, "entities": entityList, "activities": activityList}, base}
		for _, r := range rules {
			yes, err := r.When.Bool(env)
			if err != nil {
				return nil, err
			}
			if yes {
				return []data.Command{{Call: data.EventCall{Event: r.Event}}}, nil
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
			eligible, err := r.When.Bool(s.Env())
			if err != nil {
				return nil, fmt.Errorf("schedule %s: %w", r.ID, err)
			}
			if !eligible {
				continue
			}
			for _, dep := range r.After {
				for _, prerequisite := range p.Schedule {
					if prerequisite.ID == dep && s.Ran[prerequisite.Event][0] == 0 {
						eligible = false
					}
				}
			}
			if r.Exclusive != "" {
				for _, other := range p.Schedule {
					if other.Exclusive == r.Exclusive && st.Dispatched[st.Date+"/"+other.ID] {
						eligible = false
					}
				}
			}
			if !eligible {
				continue
			}
			st.Dispatched[key] = true
			out = append(out, data.Command{Call: data.EventCall{Event: r.Event}})
		}
		return out, nil
	case "assign":
		a := st.Activities[activity]
		if a == nil {
			return nil, fmt.Errorf("unknown activity %q", activity)
		}
		if entity != "" && st.Entities[entity] == nil {
			return nil, fmt.Errorf("unknown entity %q", entity)
		}
		a.Assignee = entity
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
	case "progress", "effort", "cancel", "deadline":
		a := st.Activities[activity]
		if a == nil {
			return nil, fmt.Errorf("unknown activity %q", activity)
		}
		env := expr.Chain{expr.Map{"self": s.EntityValues(a.Assignee, true)}, base}
		v, err := c.Value.Eval(env)
		if err != nil {
			return nil, err
		}
		if c.Op == "deadline" {
			deadline, ok := v.(string)
			date, parseErr := time.Parse(time.DateOnly, deadline)
			if !ok || parseErr != nil || date.Format(time.DateOnly) != deadline || deadline < p.Activities[activity].From {
				return nil, errors.New("deadline requires an ISO date on or after the activity start")
			}
			a.Deadline = deadline
			return nil, nil
		}
		if c.Op == "cancel" {
			cancelled, ok := v.(bool)
			if !ok {
				return nil, errors.New("cancel requires a boolean")
			}
			a.Cancelled = cancelled
			return nil, nil
		}
		n, ok := expr.ToNumber(v)
		if !ok || expr.TypeOfValue(v) != expr.NumberType || !expr.IsFiniteScalar(v) {
			return nil, fmt.Errorf("%s requires a finite number", c.Op)
		}
		if c.Op == "progress" && (a.Cancelled || p.Activities[activity].Parent != "" && !st.Activities[p.Activities[activity].Parent].Split) {
			return nil, nil
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
			for _, dep := range p.Activities[activity].After {
				ratio := 1.0
				if p.Activities[activity].DependencyRatio != nil {
					ratio = *p.Activities[activity].DependencyRatio
				}
				if !st.Activities[dep].Cancelled && st.Activities[dep].Progress < s.ActivityEffort(dep)*ratio {
					return nil, nil
				}
			}
		}
		a.Progress = max(0, min(s.ActivityEffort(activity), a.Progress+n))
		if a.Progress < s.ActivityEffort(activity) {
			a.Completed = ""
		} else if a.Completed == "" {
			a.Completed = st.Date
		}
		if !p.Activities[activity].Observation.Set() {
			a.Observed = a.Progress
		}
		return nil, nil
	case "set", "observe", "record":
	default:
		return nil, fmt.Errorf("unknown planning operation %q", c.Op)
	}
	if c.Op == "set" && activity != "" {
		if st.Activities[activity] == nil {
			return nil, fmt.Errorf("unknown activity %q", activity)
		}
		if entity != "" || c.Tag != "" {
			return nil, errors.New("set activity cannot be combined with entity or tag")
		}
		a, ok := p.ActivityAttributes[c.Attribute]
		if !ok {
			return nil, errors.New("unknown activity attribute")
		}
		env := expr.Chain{expr.Map{"self": s.ActivityValues(activity)}, base}
		yes, err := c.Where.Bool(env)
		if err != nil || !yes {
			return nil, err
		}
		v, err := c.Value.Eval(env)
		if err != nil {
			return nil, err
		}
		v, err = a.Value(v)
		if err != nil {
			return nil, err
		}
		if st.Activities[activity].Attributes == nil {
			st.Activities[activity].Attributes = map[string]any{}
		}
		st.Activities[activity].Attributes[c.Attribute] = v
		return nil, nil
	}
	if c.Op == "observe" && (activity != "" || entity == "" && c.Tag == "" && !c.Where.Set()) {
		if activity != "" && (entity != "" || c.Tag != "") {
			return nil, errors.New("observe activity cannot be combined with entity or tag")
		}
		ids := slices.Sorted(maps.Keys(p.Activities))
		if activity != "" {
			if _, ok := p.Activities[activity]; !ok {
				return nil, fmt.Errorf("unknown activity %q", activity)
			}
			ids = []string{activity}
			if st.Activities[activity].Split {
				ids = childIDs(s, activity)
			}
		}
		for _, id := range ids {
			def := p.Activities[id]
			a := st.Activities[id]
			if a.Split {
				continue
			}
			if def.Parent != "" && !st.Activities[def.Parent].Split {
				continue
			}
			env := expr.Chain{expr.Map{"self": s.ActivityValues(id)}, base}
			yes, err := c.Where.Bool(env)
			if err != nil {
				return nil, err
			}
			if !yes {
				continue
			}
			if !def.Observation.Set() {
				a.Observed = a.Progress
				continue
			}
			v, err := def.Observation.Eval(env)
			if err != nil {
				return nil, err
			}
			n, ok := expr.ToNumber(v)
			if !ok || expr.TypeOfValue(v) != expr.NumberType || !expr.IsFiniteScalar(v) {
				return nil, errors.New("activity observation requires a finite number")
			}
			a.Observed = max(0, min(s.ActivityEffort(id), n))
		}
		s.RecordWorkSample()
	}
	if c.Op == "observe" && activity != "" {
		return nil, nil
	}
	if entity != "" && st.Entities[entity] == nil {
		return nil, fmt.Errorf("unknown entity %q", entity)
	}
	for _, id := range slices.Sorted(maps.Keys(st.Entities)) {
		if entity != "" && id != entity || c.Tag != "" && !slices.Contains(p.Entities[id].Tags, c.Tag) {
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
				if !expr.IsFiniteScalar(v) {
					return nil, fmt.Errorf("observation %s must be a finite scalar", key)
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
		if !ok || expr.TypeOfValue(v) != expr.NumberType || !expr.IsFiniteScalar(v) {
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

// childIDs keeps allocation and reconciliation deterministic across saves.
func childIDs(s *rpg.State, parent string) []string {
	var ids []string
	for id, a := range s.DB.Game.Planning.Activities {
		if a.Parent == parent {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// Parent commands preserve old authored hooks; child work rolls up without adding
// a second budget. Root rework is spread in proportion to existing completed work.
func reconcileParts(s *rpg.State, before map[string]rpg.ActivityState, op string) error {
	for _, id := range slices.Sorted(maps.Keys(s.Planning.Activities)) {
		a := s.Planning.Activities[id]
		old := before[id]
		if !a.Split || !old.Split {
			continue
		}
		children := childIDs(s, id)
		oldEffort := old.Effort
		if oldEffort == 0 {
			oldEffort = s.DB.Game.Planning.Activities[id].Effort
		}
		delta := a.Progress - old.Progress
		for _, child := range children {
			ch := s.Planning.Activities[child]
			childEffort := before[child].Effort
			if childEffort == 0 {
				childEffort = s.DB.Game.Planning.Activities[child].Effort
			}
			ratio := childEffort / oldEffort
			if a.Effort != old.Effort {
				ch.Effort = s.ActivityEffort(id) * ratio
			}
			if a.Cancelled != old.Cancelled {
				ch.Cancelled = a.Cancelled
			}
			if a.Deadline != old.Deadline {
				ch.Deadline = a.Deadline
			}
			if delta != 0 {
				weight := ratio
				if delta < 0 && old.Progress > 0 {
					weight = before[child].Progress / old.Progress
				}
				if delta > 0 && oldEffort > old.Progress {
					weight = (childEffort - before[child].Progress) / (oldEffort - old.Progress)
				}
				ch.Progress = max(0, min(s.ActivityEffort(child), ch.Progress+delta*weight))
			}
			ch.Progress = min(ch.Progress, s.ActivityEffort(child))
			ch.Observed = min(ch.Observed, s.ActivityEffort(child))
			effort := s.ActivityEffort(child)
			if ch.Progress < effort-min(1e-9, effort*1e-9) {
				ch.Completed = ""
			} else {
				// Completion and saved work must agree exactly, even when a
				// proportional budget introduces floating-point roundoff.
				ch.Progress = s.ActivityEffort(child)
				if ch.Completed == "" {
					ch.Completed = s.Planning.Date
				}
			}
			if !s.DB.Game.Planning.Activities[child].Observation.Set() {
				ch.Observed = ch.Progress
			}
		}
		a.Progress, a.Observed = 0, 0
		totalEffort := 0.0
		for _, child := range children {
			totalEffort += s.ActivityEffort(child)
			a.Progress += s.Planning.Activities[child].Progress
			a.Observed += s.Planning.Activities[child].Observed
		}
		if math.IsInf(totalEffort, 0) || math.IsNaN(totalEffort) || math.IsInf(a.Progress, 0) || math.IsNaN(a.Progress) || math.IsInf(a.Observed, 0) || math.IsNaN(a.Observed) {
			return errors.New("activity totals must remain finite")
		}
		if totalEffort != s.ActivityEffort(id) {
			a.Effort = totalEffort
		}
		effort := s.ActivityEffort(id)
		if a.Progress < effort-min(1e-9, effort*1e-9) {
			a.Completed = ""
		} else {
			a.Progress = s.ActivityEffort(id)
			if a.Completed == "" {
				a.Completed = s.Planning.Date
			}
		}
	}
	if op == "observe" {
		s.RecordWorkSample()
	}
	return nil
}
