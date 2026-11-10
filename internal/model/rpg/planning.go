package rpg

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/nao1215/gozen/internal/base/expr"
	"github.com/nao1215/gozen/internal/base/safeint"
)

// WorkSample is a saved public observation for presentation charts, never true progress.
type WorkSample struct {
	Total    float64 `json:"total"`
	Observed float64 `json:"observed"`
}

// planningBasis records the authored limits at save time, separate from overrides.
type planningBasis struct {
	Start   string             `json:"start"`
	End     string             `json:"end"`
	Efforts map[string]float64 `json:"efforts"`
	From    map[string]string  `json:"from"`
}

// PlanningState is saved in the state envelope. Rules own mutations; presentation
// consumes observations instead of hidden attributes.
type PlanningState struct {
	Basis       *planningBasis                           `json:"basis,omitempty"`
	WorkHistory map[string]WorkSample                    `json:"work_history,omitempty"`
	Date        string                                   `json:"date"`
	Phase       int                                      `json:"phase"`
	Finished    bool                                     `json:"finished"`
	Entities    map[string]*EntityState                  `json:"entities"`
	Activities  map[string]*ActivityState                `json:"activities"`
	Ledgers     map[string]map[string]map[string]float64 `json:"ledgers"`
	Closed      map[string]bool                          `json:"closed"`
	Dispatched  map[string]bool                          `json:"dispatched"`
}

// EntityState retains true attributes and explicitly refreshed observations.
type EntityState struct {
	Attributes   map[string]any `json:"attributes"`
	Observations map[string]any `json:"observations"`
}

// ActivityState tracks assignment and reversible completion independently of actors.
type ActivityState struct {
	Split bool `json:"split,omitempty"`
	// Attributes stores typed rule overrides; absent keys read the authored defaults.
	Attributes map[string]any `json:"attributes,omitempty"`
	Deadline   string         `json:"deadline,omitempty"`  // Empty retains the authored end date.
	Cancelled  bool           `json:"cancelled,omitempty"` // Excluded work retains its budget and progress.
	Observed   float64        `json:"observed"`
	Effort     float64        `json:"effort,omitempty"` // Zero retains the authored effort for older saves.

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
	// Keep the expression object detached, including authored lists.
	values := expr.Map{}
	for key, value := range def.Data {
		if list, ok := value.([]any); ok {
			value = slices.Clone(list)
		}
		values[key] = value
	}
	out["data"] = values
	for key, value := range st.Attributes {
		if full || !p.Attributes[key].Hidden {
			out[key] = value
		}
	}
	maps.Copy(out, st.Observations)
	return out
}

// EntityValue reads one public field for views without building the complete object.
// Composite metadata retains EntityValues' detached-copy contract.
func (s *State) EntityValue(id, key string) any {
	p := s.DB.Game.Planning
	if p == nil || s.Planning == nil {
		return s.EntityValues(id, false)[key]
	}
	st := s.Planning.Entities[id]
	if st == nil {
		return s.EntityValues(id, false)[key]
	}
	if value, ok := st.Observations[key]; ok {
		return value
	}
	if value, ok := st.Attributes[key]; ok && !p.Attributes[key].Hidden {
		return value
	}
	if key == "id" {
		return id
	}
	if key == "name" {
		return p.Entities[id].Name
	}
	if key == "data" {
		return s.EntityValues(id, false)[key]
	}
	return nil
}

func (s *State) planningLookup(name string) (any, bool) {
	p := s.DB.Game.Planning
	st := s.Planning
	if p == nil || st == nil {
		return nil, false
	}
	out := expr.Map{}
	switch name {
	case "calendar_days", "calendar_workdays":
		return expr.Func(func(args []any) (any, error) { return s.calendarCount(name, args) }), true
	case "calendar_shift":
		return expr.Func(calendarShift), true
	case "period_value":
		return expr.Func(s.periodValue), true
	case "planning":
		d, err := time.Parse(time.DateOnly, st.Date)
		if err != nil {
			return nil, false
		}
		return expr.Map{"date": st.Date, "period": d.Format("2006-01"), "weekday": float64(d.Weekday()), "phase": p.Calendar.Phases[st.Phase], "finished": st.Finished, "closed": s.PlanningClosedDay()}, true
	case "entities":
		if r := s.reading; r != nil && r.entities != nil {
			return r.entities, true
		}
		values := make([]any, 0, len(st.Entities))
		for _, id := range slices.Sorted(maps.Keys(st.Entities)) {
			values = append(values, s.entityRead(id))
		}
		if s.reading != nil {
			s.reading.entities = values
		}
		return values, true
	case "activities":
		if r := s.reading; r != nil && r.activities != nil {
			return r.activities, true
		}
		values := make([]any, 0, len(st.Activities))
		for _, id := range slices.Sorted(maps.Keys(st.Activities)) {
			values = append(values, s.activityRead(id))
		}
		if s.reading != nil {
			s.reading.activities = values
		}
		return values, true
	case "entity":
		return entityView{s}, true
	case "activity":
		return activityView{s}, true
	}
	return out, true
}

// entityView and activityView are entity.<id> and activity.<id>: the values of
// the one named, made when it is read (an expression rarely reads more than a
// few, and building all of them for each read made screens slow).
type entityView struct{ s *State }

func (v entityView) Field(id string) (any, bool) {
	if _, ok := v.s.Planning.Entities[id]; !ok {
		return nil, false
	}
	return v.s.entityRead(id), true
}

type activityView struct{ s *State }

func (v activityView) Field(id string) (any, bool) {
	if _, ok := v.s.Planning.Activities[id]; !ok {
		return nil, false
	}
	return v.s.activityRead(id), true
}

// planningRead holds the values expressions read while the state is only read:
// a screen evaluates hundreds of expressions a frame, each reading the same few
// entities and the whole activity list, and making those maps again for every
// read was most of a frame.
type planningRead struct {
	entity, activity     map[string]expr.Map
	entities, activities []any
}

// Reading runs fn with the planning values made for expressions memoized: fn
// must not change the state (drawing a screen, building its entries). The
// values are shared between the expressions fn evaluates, which never change
// them. Nested calls share the outer memo.
func (s *State) Reading(fn func()) {
	if s.reading != nil {
		fn()
		return
	}
	s.reading = &planningRead{entity: map[string]expr.Map{}, activity: map[string]expr.Map{}}
	defer func() { s.reading = nil }()
	fn()
}

func (s *State) entityRead(id string) expr.Map {
	r := s.reading
	if r == nil {
		return s.EntityValues(id, true)
	}
	v, ok := r.entity[id]
	if !ok {
		v = s.EntityValues(id, true)
		r.entity[id] = v
	}
	return v
}

func (s *State) activityRead(id string) expr.Map {
	r := s.reading
	if r == nil {
		return s.ActivityValues(id)
	}
	v, ok := r.activity[id]
	if !ok {
		v = s.ActivityValues(id)
		r.activity[id] = v
	}
	return v
}

// ActivityValues returns detached full activity values for rules and their self scope.
// Presentation must use explicit observations instead. Unconfigured or unknown IDs return nil.
func (s *State) ActivityValues(id string) expr.Map {
	if s.Planning == nil {
		return nil
	}
	a := s.Planning.Activities[id]
	if a == nil {
		return nil
	}
	def := s.DB.Game.Planning.Activities[id]
	active := !a.Split
	if def.Parent != "" {
		active = s.Planning.Activities[def.Parent].Split
	}
	out := expr.Map{"name": def.Name, "parent": def.Parent, "split": a.Split, "active": active, "id": id, "from": def.From, "to": def.To, "planned_effort": def.Effort, "deadline": s.ActivityDeadline(id), "progress": a.Progress, "observed": a.Observed, "effort": s.ActivityEffort(id), "assignee": a.Assignee, "completed": a.Completed != "", "cancelled": a.Cancelled}
	for key, attr := range s.DB.Game.Planning.ActivityAttributes {
		v, err := attr.Value(attr.Initial)
		if err == nil {
			out[key] = v
		}
	}
	maps.Copy(out, a.Attributes)
	return out
}

// calendarCount reads the configured closure rules without advancing state.
// Whole weeks plus at most six days avoid scanning long authored ranges.
func (s *State) calendarCount(name string, args []any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%s needs two ISO dates", name)
	}
	var dates [2]time.Time
	for i, value := range args {
		date, err := calendarDate(value)
		if err != nil {
			return nil, fmt.Errorf("%s date %d: %w", name, i+1, err)
		}
		dates[i] = date
	}
	// Sub uses a duration and saturates beyond about 290 years.
	days := (dates[1].Unix() - dates[0].Unix()) / 86400
	if name == "calendar_days" {
		return float64(days), nil
	}
	if days < 0 {
		return float64(0), nil
	}
	days++ // Both endpoints are included in working-day counts.
	calendar := s.DB.Game.Planning.Calendar
	count := int64(0)
	for weekday := range 7 {
		if !slices.Contains(calendar.WeekdaysOff, weekday) {
			count += days / 7
		}
	}
	for offset := range int(days % 7) {
		weekday := (int(dates[0].Weekday()) + offset) % 7
		if !slices.Contains(calendar.WeekdaysOff, weekday) {
			count++
		}
	}
	seen := make(map[string]bool, len(calendar.Closed))
	for _, closed := range calendar.Closed {
		if seen[closed] {
			continue
		}
		seen[closed] = true
		date, err := time.Parse(time.DateOnly, closed)
		if err == nil && !date.Before(dates[0]) && !date.After(dates[1]) && !slices.Contains(calendar.WeekdaysOff, int(date.Weekday())) {
			count--
		}
	}
	return float64(count), nil
}

// calendarDate keeps the same ISO range for counting and shifting dates.
func calendarDate(value any) (time.Time, error) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, errors.New("date must be a string")
	}
	date, err := time.Parse(time.DateOnly, text)
	if err != nil || date.Year() < 1 || date.Format(time.DateOnly) != text {
		return time.Time{}, errors.New("date must be YYYY-MM-DD in years 0001 through 9999")
	}
	return date, nil
}

// calendarShift computes a civil date without advancing the planning clock.
func calendarShift(args []any) (any, error) {
	if len(args) != 2 {
		return nil, errors.New("calendar_shift needs an ISO date and an integer day offset")
	}
	date, err := calendarDate(args[0])
	if err != nil {
		return nil, fmt.Errorf("calendar_shift: %w", err)
	}
	days, ok := expr.ToNumber(args[1])
	// This bound also prevents overflow in conversion and date arithmetic.
	if !ok || expr.TypeOfValue(args[1]) != expr.NumberType || math.IsNaN(days) || math.Abs(days) > 3652058 || math.Trunc(days) != days {
		return nil, errors.New("calendar_shift offset must be an integer within -3652058..3652058")
	}
	result := date.AddDate(0, 0, safeint.From(days))
	if result.Year() < 1 || result.Year() > 9999 {
		return nil, errors.New("calendar_shift result must be in years 0001 through 9999")
	}
	return result.Format(time.DateOnly), nil
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
	for date, sample := range st.WorkHistory {
		parsed, err := time.Parse(time.DateOnly, date)
		if err != nil || parsed.Format(time.DateOnly) != date || date < p.Calendar.Start || date > st.Date || sample.Total < 0 || sample.Observed < 0 || sample.Observed > sample.Total || math.IsNaN(sample.Total) || math.IsNaN(sample.Observed) || math.IsInf(sample.Total, 0) || math.IsInf(sample.Observed, 0) {
			return errors.New("planning: invalid work history")
		}
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
		if a == nil {
			return fmt.Errorf("planning.activities.%s: invalid state", id)
		}
		for key, v := range a.Attributes {
			attr, ok := p.ActivityAttributes[key]
			if !ok {
				return fmt.Errorf("planning.activities.%s.%s: unknown attribute", id, key)
			}
			if _, err := attr.Value(v); err != nil {
				return fmt.Errorf("planning.activities.%s.%s: %w", id, key, err)
			}
		}

		_, ok := p.Activities[id]
		effort := s.ActivityEffort(id)
		if !ok || math.IsNaN(a.Effort) || math.IsInf(a.Effort, 0) || a.Effort < 0 || math.IsNaN(a.Progress) || math.IsInf(a.Progress, 0) || a.Progress < 0 || a.Progress > effort || math.IsNaN(a.Observed) || math.IsInf(a.Observed, 0) || a.Observed < 0 || a.Observed > effort {
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
	return s.Planning.Ledgers[ledger][date.AddDate(0, safeint.From(offset), 0).Format("2006-01")][owner], nil
}

// RecordWorkSample is called by the planning interpreter after successful observe
// operations. A date has one latest sample; loading old saves leaves earlier dates unknown.
func (s *State) RecordWorkSample() {
	if s.Planning == nil {
		return
	}
	enabled := false
	for _, screen := range s.DB.Game.Screens {
		if screen.View.Burnup != nil {
			enabled = true
			break
		}
	}
	if !enabled {
		return
	}
	var sample WorkSample
	for _, id := range slices.Sorted(maps.Keys(s.Planning.Activities)) {
		a := s.Planning.Activities[id]
		if s.DB.Game.Planning.Activities[id].Parent != "" {
			continue
		}
		if a.Cancelled {
			continue
		}
		sample.Total += s.ActivityEffort(id)
		sample.Observed += a.Observed
	}
	if s.Planning.WorkHistory == nil {
		s.Planning.WorkHistory = map[string]WorkSample{}
	}
	s.Planning.WorkHistory[s.Planning.Date] = sample
}

// PlannedWork supplies the default chart budget using public effort and deadlines.
// It interpolates working days, with zero at start and completion at the deadline.
func (s *State) PlannedWork(date string) float64 {
	total := 0.0
	for _, id := range slices.Sorted(maps.Keys(s.Planning.Activities)) {
		a := s.Planning.Activities[id]
		if s.DB.Game.Planning.Activities[id].Parent != "" {
			continue
		}
		if a.Cancelled {
			continue
		}
		from, to := s.DB.Game.Planning.Activities[id].From, s.ActivityDeadline(id)
		if from == "" {
			from = s.DB.Game.Planning.Calendar.Start
		}
		if to == "" {
			to = s.DB.Game.Planning.Calendar.End
		}
		ratio := 0.0
		if date >= to {
			ratio = 1
		} else if date > from {
			n, err := s.calendarCount("calendar_workdays", []any{from, date})
			d, derr := s.calendarCount("calendar_workdays", []any{from, to})
			if err == nil && derr == nil {
				nn, _ := expr.ToNumber(n)
				dd, _ := expr.ToNumber(d)
				ratio = min(1, nn/max(1, dd))
			}
		}
		total += ratio * s.ActivityEffort(id)
	}
	return total
}
