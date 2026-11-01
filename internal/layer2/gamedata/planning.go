package gamedata

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"time"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

// Planning defines optional non-combat simulation data consumed by state and rules.
// Maps have stable IDs; operation ordering uses sorted IDs, never Go map order.
type Planning struct {
	// SeedXor optionally derives the second PCG seed by XOR for imported deterministic simulations.
	SeedXor  *uint64                 `yaml:"seed_xor"`
	Rules    map[string]PlanRule     `yaml:"rules"`
	Policies map[string][]PolicyRule `yaml:"policies"`

	Attributes map[string]Attribute `yaml:"attributes"`
	Entities   map[string]Entity    `yaml:"entities"`
	Calendar   Calendar             `yaml:"calendar"`
	Activities map[string]Activity  `yaml:"activities"`
	Ledgers    map[string]Ledger    `yaml:"ledgers"`
	Schedule   []Reservation        `yaml:"schedule"`
}

// Attribute is a typed scalar declaration. Hidden values are omitted from observations.
type Attribute struct {
	Type    string   `yaml:"type"`
	Initial any      `yaml:"initial"`
	Min     *float64 `yaml:"min"`
	Max     *float64 `yaml:"max"`
	Hidden  bool     `yaml:"hidden"`
}

// Value validates and normalizes a scalar for initial state and rule assignments.
// It never clamps: authors must explicitly clamp formulas where that is intended.
func (a Attribute) Value(v any) (any, error) {
	if a.Min != nil && (math.IsNaN(*a.Min) || math.IsInf(*a.Min, 0)) || a.Max != nil && (math.IsNaN(*a.Max) || math.IsInf(*a.Max, 0)) {
		return nil, errors.New("attribute bounds must be finite")
	}
	if a.Type == "number" {
		rv := reflect.ValueOf(v)
		if rv.IsValid() && (rv.Kind() == reflect.Uint || rv.Kind() == reflect.Uint64) {
			v = float64(rv.Uint())
		}
		n, ok := expr.ToNumber(v)
		if !ok || expr.TypeOfValue(v) != expr.NumberType || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, errors.New("expected a finite number")
		}
		if a.Min != nil && n < *a.Min || a.Max != nil && n > *a.Max {
			return nil, errors.New("number outside declared range")
		}
		return n, nil
	}
	if a.Type == "string" {
		if _, ok := v.(string); ok {
			return v, nil
		}
	}
	if a.Type == "bool" {
		if _, ok := v.(bool); ok {
			return v, nil
		}
	}
	return nil, fmt.Errorf("expected %s", a.Type)
}

// Entity joins optional actor presentation with independently typed state.
// Observations are evaluated on explicit refresh, not during drawing.
type Entity struct {
	Name         string          `yaml:"name"`
	Actor        string          `yaml:"actor"`
	Tags         []string        `yaml:"tags"`
	Attributes   map[string]any  `yaml:"attributes"`
	Observations map[string]Expr `yaml:"observations"`
}

// Calendar is an inclusive date range; closed days use one phase.
type Calendar struct {
	Start       string   `yaml:"start"`
	End         string   `yaml:"end"`
	Phases      []string `yaml:"phases"`
	WeekdaysOff []int    `yaml:"weekdays_off"`
	Closed      []string `yaml:"closed"`
}

// Activity is a bounded amount of work with strict completion dependencies.
type Activity struct {
	Observation     Expr     `yaml:"observation"`
	DependencyRatio *float64 `yaml:"dependency_ratio"`

	Name     string   `yaml:"name"`
	Effort   float64  `yaml:"effort"`
	Assignee string   `yaml:"assignee"`
	After    []string `yaml:"after"`
	From     string   `yaml:"from"`
	To       string   `yaml:"to"`
}

// Ledger defines retained monthly totals, with explicit closing by a rule.
type Ledger struct {
	Retain int `yaml:"retain"`
}

// Reservation is an event on a date or recurring weekday and phase.
// Its stable ID prevents duplicate dispatch after saving and loading.
type Reservation struct {
	ID       string `yaml:"id"`
	Date     string `yaml:"date"`
	Weekdays []int  `yaml:"weekdays"`
	Phase    string `yaml:"phase"`
	Event    string `yaml:"event"`
}

// PlanCommand is an immediate operation. Where and Value bind self to each
// selected entity in ID order. A missing Where selects all tagged entities.
type PlanCommand struct {
	Rule   string          `yaml:"rule"`
	Args   map[string]Expr `yaml:"args"`
	Policy string          `yaml:"policy"`

	Op        string `yaml:"op"`
	Entity    string `yaml:"entity"`
	Tag       string `yaml:"tag"`
	Where     Expr   `yaml:"where"`
	Attribute string `yaml:"attribute"`
	Value     Expr   `yaml:"value"`
	Activity  string `yaml:"activity"`
	Ledger    string `yaml:"ledger"`
	Period    string `yaml:"period"`
}

func (c *checker) planning() {
	p := c.db.Game.Planning
	if p == nil {
		return
	}
	w := "game.yaml.planning"
	checkDate := func(at, v string) {
		if _, err := time.Parse(time.DateOnly, v); err != nil {
			c.errorf(diag.Shape, at, "expected an ISO date, got %q", v)
		}
	}
	checkDate(w+".calendar.start", p.Calendar.Start)
	checkDate(w+".calendar.end", p.Calendar.End)
	if p.Calendar.End < p.Calendar.Start {
		c.errorf(diag.OutOfRange, w+".calendar.end", "end precedes start")
	}
	if len(p.Calendar.Phases) == 0 {
		c.errorf(diag.Shape, w+".calendar.phases", "at least one phase is required")
	}
	seen := map[string]bool{}
	for _, v := range p.Calendar.Phases {
		if !declaredName.MatchString(v) || seen[v] {
			c.errorf(diag.Shape, w+".calendar.phases", "invalid or duplicate phase %q", v)
		}
		seen[v] = true
	}
	for _, d := range p.Calendar.Closed {
		checkDate(w+".calendar.closed", d)
	}
	for _, d := range p.Calendar.WeekdaysOff {
		if d < 0 || d > 6 {
			c.errorf(diag.OutOfRange, w+".calendar.weekdays_off", "weekday must be 0 (Sunday) through 6")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Attributes)) {
		a := p.Attributes[id]
		at := w + ".attributes." + id
		if !declaredName.MatchString(id) || id == "id" || id == "name" {
			c.errorf(diag.Shape, at, "invalid or reserved attribute ID")
		}
		if _, err := a.Value(a.Initial); err != nil {
			c.errorf(diag.Shape, at+".initial", "%v", err)
		}
		if a.Type != "number" && (a.Min != nil || a.Max != nil) {
			c.errorf(diag.Shape, at, "bounds require number type")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Entities)) {
		e := p.Entities[id]
		at := w + ".entities." + id
		if !declaredName.MatchString(id) {
			c.errorf(diag.Shape, at, "invalid entity ID")
		}
		if e.Actor != "" && c.db.Actors[e.Actor] == nil {
			c.errorf(diag.UnknownReference, at+".actor", "unknown actor %q", e.Actor)
		}
		for _, k := range slices.Sorted(maps.Keys(e.Attributes)) {
			a, ok := p.Attributes[k]
			if !ok {
				c.errorf(diag.UnknownReference, at+".attributes."+k, "unknown attribute")
				continue
			}
			if _, err := a.Value(e.Attributes[k]); err != nil {
				c.errorf(diag.Shape, at+".attributes."+k, "%v", err)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(e.Observations)) {
			if _, ok := p.Attributes[k]; ok || k == "id" || k == "name" {
				c.errorf(diag.Shape, at+".observations."+k, "observation must not shadow an attribute")
			}
			c.planningPure(at+".observations."+k, e.Observations[k])
			c.exprIn(at+".observations."+k, e.Observations[k], append(slices.Clone(FieldNames), "self"), map[string]*vtype{"self": tAny})
		}
	}
	for id, r := range p.Rules {
		at := w + ".rules." + id
		if len(r.Do) > 256 {
			c.errorf(diag.OutOfRange, at, "a rule may contain at most 256 operations")
		}
		seen := map[string]bool{}
		for _, param := range r.Params {
			if !declaredName.MatchString(param) || seen[param] {
				c.errorf(diag.Shape, at+".params", "invalid or duplicate parameter")
			}
			seen[param] = true
		}
		for i, cmd := range r.Do {
			if cmd.Op == "rule" || cmd.Op == "policy" {
				c.errorf(diag.Shape, at, "nested rule and policy calls are not allowed")
			}
			c.planCommand(fmt.Sprintf("%s.do[%d]", at, i), &cmd)
		}
	}
	for id, rules := range p.Policies {
		for i, r := range rules {
			at := fmt.Sprintf("%s.policies.%s[%d]", w, id, i)
			c.planningPure(at+".when", r.When)
			c.expr(at+".when", r.When, FieldNames)
			if c.db.Events[r.Event] == nil {
				c.errorf(diag.UnknownReference, at+".event", "unknown policy event")
			}
		}
	}
	colors := map[string]int{}
	var visit func(string)
	visit = func(id string) {
		if colors[id] == 1 {
			c.errorf(diag.Shape, w+".activities."+id+".after", "cyclic dependency")
			return
		}
		if colors[id] == 2 {
			return
		}
		colors[id] = 1
		for _, dep := range p.Activities[id].After {
			if _, ok := p.Activities[dep]; !ok {
				c.errorf(diag.UnknownReference, w+".activities."+id+".after", "unknown activity %q", dep)
			} else {
				visit(dep)
			}
		}
		colors[id] = 2
	}
	for _, id := range slices.Sorted(maps.Keys(p.Activities)) {
		a := p.Activities[id]
		at := w + ".activities." + id
		if !declaredName.MatchString(id) {
			c.errorf(diag.Shape, at, "invalid activity ID")
		}
		c.planningPure(at+".observation", a.Observation)
		c.exprIn(at+".observation", a.Observation, append(slices.Clone(FieldNames), "self"), map[string]*vtype{"self": tAny})
		if a.DependencyRatio != nil && (math.IsNaN(*a.DependencyRatio) || *a.DependencyRatio < 0 || *a.DependencyRatio > 1) {
			c.errorf(diag.OutOfRange, at+".dependency_ratio", "ratio must be between 0 and 1")
		}
		if !(a.Effort > 0) || math.IsInf(a.Effort, 0) {
			c.errorf(diag.OutOfRange, at+".effort", "effort must be finite and positive")
		}
		if a.Assignee != "" {
			if _, ok := p.Entities[a.Assignee]; !ok {
				c.errorf(diag.UnknownReference, at+".assignee", "unknown entity")
			}
		}
		if a.From != "" {
			checkDate(at+".from", a.From)
		}
		if a.To != "" {
			checkDate(at+".to", a.To)
		}
		if a.To != "" && a.From > a.To {
			c.errorf(diag.OutOfRange, at+".to", "end precedes start")
		}
		visit(id)
	}
	for id, l := range p.Ledgers {
		if l.Retain < 1 || l.Retain > 120 {
			c.errorf(diag.OutOfRange, w+".ledgers."+id+".retain", "retain must be 1 through 120")
		}
	}
	seen = map[string]bool{}
	for i, r := range p.Schedule {
		at := fmt.Sprintf("%s.schedule[%d]", w, i)
		if !declaredName.MatchString(r.ID) || seen[r.ID] {
			c.errorf(diag.Shape, at+".id", "reservation ID must be unique and nonempty")
		}
		seen[r.ID] = true
		if r.Date != "" {
			checkDate(at+".date", r.Date)
		}
		if !slices.Contains(p.Calendar.Phases, r.Phase) {
			c.errorf(diag.UnknownReference, at+".phase", "unknown phase")
		}
		if c.db.Events[r.Event] == nil {
			c.errorf(diag.UnknownReference, at+".event", "unknown event")
		}
		for _, d := range r.Weekdays {
			if d < 0 || d > 6 {
				c.errorf(diag.OutOfRange, at+".weekdays", "weekday must be 0 through 6")
			}
		}
	}
}

func (c *checker) planCommand(w string, cmd *PlanCommand) {
	if cmd == nil {
		return
	}
	allowed := map[string][]string{
		"set": {"entity", "tag", "where", "attribute", "value"}, "observe": {"entity", "tag", "where"},
		"deadline": {"activity", "value"}, "cancel": {"activity", "value"}, "effort": {"activity", "value"}, "progress": {"activity", "value"}, "assign": {"activity", "entity"}, "advance": {}, "dispatch": {},
		"record": {"entity", "tag", "where", "ledger", "period", "value"}, "close": {"ledger", "period"},
		"rule": {"rule", "args"}, "policy": {"policy"},
	}
	fields := map[string]bool{"entity": cmd.Entity != "", "tag": cmd.Tag != "", "where": cmd.Where.Set(), "attribute": cmd.Attribute != "", "value": cmd.Value.Set(), "activity": cmd.Activity != "", "ledger": cmd.Ledger != "", "period": cmd.Period != "", "rule": cmd.Rule != "", "args": len(cmd.Args) > 0, "policy": cmd.Policy != ""}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if fields[key] && !slices.Contains(allowed[cmd.Op], key) {
			c.errorf(diag.Shape, w+"."+key, "field is not used by %s", cmd.Op)
		}
	}
	p := c.db.Game.Planning
	if p == nil {
		c.errorf(diag.Shape, w, "plan requires game.yaml planning")
		return
	}
	if !slices.Contains([]string{"set", "observe", "assign", "progress", "effort", "cancel", "deadline", "advance", "dispatch", "record", "close", "rule", "policy"}, cmd.Op) {
		c.errorf(diag.NotAWord, w+".op", "unknown planning operation %q", cmd.Op)
	}
	if cmd.Entity != "" {
		if _, ok := p.Entities[cmd.Entity]; !ok {
			c.errorf(diag.UnknownReference, w+".entity", "unknown entity")
		}
	}
	if cmd.Op == "set" {
		if _, ok := p.Attributes[cmd.Attribute]; !ok {
			c.errorf(diag.UnknownReference, w+".attribute", "unknown attribute")
		}
	}
	if cmd.Op == "deadline" || cmd.Op == "cancel" || cmd.Op == "effort" || cmd.Op == "progress" || cmd.Op == "assign" {
		if _, ok := p.Activities[cmd.Activity]; !ok {
			c.errorf(diag.UnknownReference, w+".activity", "unknown activity")
		}
	}
	if cmd.Op == "record" || cmd.Op == "close" {
		if _, ok := p.Ledgers[cmd.Ledger]; !ok {
			c.errorf(diag.UnknownReference, w+".ledger", "unknown ledger")
		}
	}
	if cmd.Op == "set" || cmd.Op == "deadline" || cmd.Op == "cancel" || cmd.Op == "effort" || cmd.Op == "progress" || cmd.Op == "record" {
		if !cmd.Value.Set() {
			c.errorf(diag.Shape, w+".value", "operation requires a value")
		}
	}
	if cmd.Op == "rule" {
		r, ok := p.Rules[cmd.Rule]
		if !ok {
			c.errorf(diag.UnknownReference, w+".rule", "unknown rule")
		}
		if len(cmd.Args) != len(r.Params) {
			c.errorf(diag.Shape, w+".args", "arguments must match rule parameters")
		}
		for _, param := range r.Params {
			if _, ok := cmd.Args[param]; !ok {
				c.errorf(diag.Shape, w+".args", "missing parameter %q", param)
			}
		}
	}
	if cmd.Op == "policy" {
		if _, ok := p.Policies[cmd.Policy]; !ok {
			c.errorf(diag.UnknownReference, w+".policy", "unknown policy")
		}
	}
	names := append(slices.Clone(FieldNames), "self", "arg")
	for key, e := range cmd.Args {
		c.exprIn(w+".args."+key, e, names, map[string]*vtype{"self": tAny, "arg": tAny})
	}
	for _, e := range append([]Expr{cmd.Where, cmd.Value}, slices.Collect(maps.Values(cmd.Args))...) {
		c.planningPure(w, e)
	}
	c.exprIn(w+".where", cmd.Where, names, map[string]*vtype{"self": tAny, "arg": tAny})
	c.exprIn(w+".value", cmd.Value, names, map[string]*vtype{"self": tAny, "arg": tAny})
}

// PlanRule is a bounded reusable sequence with scalar arguments, consumed by plan rule.
type PlanRule struct {
	Params []string      `yaml:"params"`
	Do     []PlanCommand `yaml:"do"`
}

// PolicyRule selects the first matching event using player observations only.
type PolicyRule struct {
	When  Expr   `yaml:"when"`
	Event string `yaml:"event"`
}

func (c *checker) planningPure(w string, e Expr) {
	for _, n := range e.Names() {
		if n == "rand" || n == "chance" {
			c.errorf(diag.Expression, w, "planning expressions must be pure; draw randomness in a preceding event command")
		}
	}
}

// PlanningView selects only explicitly named player values for presentation.
// Order controls seating; fields and portrait bindings never read hidden state.
type PlanningView struct {
	// Tabs names two to five fullscreen peers, including this screen. Play replaces
	// the current surface when switching, preserving its original close callback.
	Tabs []string `yaml:"tabs"`
	// Rows groups timeline activities in authored order; omission keeps the task list.
	Rows []TimelineRow `yaml:"rows"`
	// Markers annotates dates without changing scheduling or simulation rules.
	Markers []TimelineMarker `yaml:"markers"`

	// Visible binds an optional public boolean; only true values show a resident.
	Visible string `yaml:"visible"`
	// Highlight binds a public boolean used by play to outline ensemble tiles.
	// Missing, false or non-boolean values leave the tile unaccented.
	Highlight string `yaml:"highlight"`
	// Fullscreen uses the play surface, with authored hotspots and entity actions.
	Fullscreen bool `yaml:"fullscreen"`
	// Header and Footer are rendered through the ordinary text interpolator.
	Header string `yaml:"header"`
	Footer string `yaml:"footer"`
	// DateFormat uses Go time layout tokens. Weekdays and Phases replace display labels.
	DateFormat string `yaml:"date_format"`
	// WeekStart chooses the first calendar column: 0 (default Sunday) through 6.
	WeekStart  int               `yaml:"week_start"`
	Weekdays   []string          `yaml:"weekdays"`
	Phases     map[string]string `yaml:"phases"`
	EventLabel string            `yaml:"event_label"`

	// Detail binds the selected entity pane to an explicitly public text value.
	Detail   string          `yaml:"detail"`
	Icon     string          `yaml:"icon"`
	Order    []string        `yaml:"order"`
	Fields   []PlanningField `yaml:"fields"`
	Portrait string          `yaml:"portrait"`
	Camera   string          `yaml:"camera"`
	Badge    string          `yaml:"badge"`
	Backdrop string          `yaml:"backdrop"`
	Mask     string          `yaml:"mask"`
	Door     string          `yaml:"door"`
}

// TimelineRow is an authored group rendered by the timeline view.
// Items appear in separate compact lanes; omitted activities remain in fallback rows.
type TimelineRow struct {
	Label string `yaml:"label"`
	// Text replaces lane labels with an authored, interpolated row summary.
	Text  string         `yaml:"text"`
	Items []TimelineItem `yaml:"items"`
}

// TimelineItem binds a lane to an activity and optionally opens an authored screen.
// Label is a short lane identifier. Screen omission makes the lane read-only.
type TimelineItem struct {
	Activity string `yaml:"activity"`
	Label    string `yaml:"label"`
	Screen   string `yaml:"screen"`
	// Sample is a saved observed ratio (0..1), negative or absent when unknown.
	// SampleLabel gives its authored provenance, such as the observation date.
	Sample      Expr   `yaml:"sample"`
	SampleLabel string `yaml:"sample_label"`
	// ForecastLabel enables a calendar-day linear forecast from reported work.
	// The author supplies the explanatory label; empty disables the forecast.
	ForecastLabel string `yaml:"forecast_label"`
}

// TimelineMarker is a named calendar annotation consumed by the timeline renderer.
// Milestones extend a dashed line through all rows; ordinary markers shade one day.
type TimelineMarker struct {
	Date      string `yaml:"date"`
	Label     string `yaml:"label"`
	Milestone bool   `yaml:"milestone"`
}

// PlanningField describes a public value column and optional numeric gauge.
// Marks and Maximum use the same units as the observed value.
type PlanningField struct {
	Key     string    `yaml:"key"`
	Label   string    `yaml:"label"`
	Maximum float64   `yaml:"maximum"`
	Marks   []float64 `yaml:"marks"`
}

func (c *checker) planningView(w string, v PlanningView) {
	p := c.db.Game.Planning
	if p == nil {
		return
	}
	if len(v.Tabs) > 0 {
		if !v.Fullscreen || len(v.Tabs) < 2 || len(v.Tabs) > 5 {
			c.errorf(diag.Shape, w+".tabs", "tabs require a fullscreen view and two to five screens")
		}
		seenTabs := map[string]bool{}
		for _, id := range v.Tabs {
			screen := c.db.Game.Screens[id]
			if screen == nil || !screen.View.Fullscreen || seenTabs[id] {
				c.errorf(diag.UnknownReference, w+".tabs", "unknown, duplicate or non-fullscreen tab %q", id)
			} else if !slices.Equal(screen.View.Tabs, v.Tabs) {
				c.errorf(diag.Shape, w+".tabs", "tab screens must declare the same ordered tabs")
			}
			seenTabs[id] = true
		}
	}
	if v.WeekStart < 0 || v.WeekStart > 6 {
		c.errorf(diag.OutOfRange, w+".week_start", "weekday must be 0 through 6")
	}
	if len(v.Weekdays) != 0 && len(v.Weekdays) != 7 {
		c.errorf(diag.Shape, w+".weekdays", "provide seven weekday labels")
	}
	used := map[string]bool{}
	for i, row := range v.Rows {
		at := fmt.Sprintf("%s.rows[%d]", w, i)
		c.text(at+".text", row.Text, FieldNames)
		if row.Label == "" || len(row.Items) == 0 {
			c.errorf(diag.Shape, at, "timeline row requires a label and items")
		}
		for _, item := range row.Items {
			if _, ok := p.Activities[item.Activity]; !ok || used[item.Activity] {
				c.errorf(diag.UnknownReference, at, "unknown or duplicate timeline activity %q", item.Activity)
			}
			used[item.Activity] = true
			c.planningPure(at+".sample", item.Sample)
			c.exprIn(at+".sample", item.Sample, FieldNames, nil)
			c.text(at+".sample_label", item.SampleLabel, FieldNames)
			c.text(at+".forecast_label", item.ForecastLabel, FieldNames)
			if item.Screen != "" && c.db.Game.Screens[item.Screen] == nil {
				c.errorf(diag.UnknownReference, at, "unknown timeline screen %q", item.Screen)
			}
		}
	}
	for i, marker := range v.Markers {
		if _, err := time.Parse(time.DateOnly, marker.Date); err != nil || marker.Label == "" {
			c.errorf(diag.Shape, fmt.Sprintf("%s.markers[%d]", w, i), "timeline marker requires an ISO date and label")
		}
	}
	c.text(w+".header", v.Header, FieldNames)
	c.text(w+".footer", v.Footer, FieldNames)
	seen := map[string]bool{}
	for _, id := range v.Order {
		if _, ok := p.Entities[id]; !ok || seen[id] {
			c.errorf(diag.Shape, w+".order", "unknown or duplicate entity %q", id)
		}
		seen[id] = true
	}
	keys := []string{v.Visible, v.Highlight, v.Detail, v.Icon, v.Portrait, v.Camera, v.Badge, v.Backdrop, v.Mask}
	for _, f := range v.Fields {
		keys = append(keys, f.Key)
		if f.Maximum < 0 || math.IsNaN(f.Maximum) || math.IsInf(f.Maximum, 0) {
			c.errorf(diag.Shape, w, "gauge maximum must be finite and nonnegative")
		}
		for _, n := range f.Marks {
			if n < 0 || n > f.Maximum || math.IsNaN(n) {
				c.errorf(diag.Shape, w, "gauge mark outside range")
			}
		}
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if a, ok := p.Attributes[key]; ok {
			if a.Hidden {
				c.errorf(diag.Shape, w, "view cannot expose hidden attribute %q", key)
			}
			continue
		}
		found := key == "name" || key == "id"
		for _, e := range p.Entities {
			if _, ok := e.Observations[key]; ok {
				found = true
			}
		}
		if !found {
			c.errorf(diag.UnknownReference, w, "unknown player value %q", key)
		}
	}
}
