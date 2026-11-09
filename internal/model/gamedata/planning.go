package gamedata

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/base/diag"
	"github.com/nao1215/gozen/internal/base/expr"
)

// Planning defines optional non-combat simulation data consumed by state and rules.
// Maps have stable IDs; operation ordering uses sorted IDs, never Go map order.
type Planning struct {
	// SeedXor optionally derives the second PCG seed by XOR for imported deterministic simulations.
	SeedXor  *uint64                 `yaml:"seed_xor"`
	Rules    map[string]PlanRule     `yaml:"rules"`
	Policies map[string][]PolicyRule `yaml:"policies"`

	// ActivityAttributes supplies typed task-local defaults to state, save validation and rules.
	ActivityAttributes map[string]Attribute `yaml:"activity_attributes"`
	Attributes         map[string]Attribute `yaml:"attributes"`
	Entities           map[string]Entity    `yaml:"entities"`
	Calendar           Calendar             `yaml:"calendar"`
	Activities         map[string]Activity  `yaml:"activities"`
	Ledgers            map[string]Ledger    `yaml:"ledgers"`
	Schedule           []Reservation        `yaml:"schedule"`
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
		if !ok || expr.TypeOfValue(v) != expr.NumberType || !expr.IsFiniteScalar(v) {
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

// DefinitionData is immutable scalar/list metadata exposed by RPG entity expressions.
// The loader validates finite values and homogeneous lists; rules cannot assign it.
type DefinitionData map[string]any

// UnmarshalYAML normalizes unsigned YAML integers to expression numbers.
func (d *DefinitionData) UnmarshalYAML(unmarshal func(any) error) error {
	var values map[string]any
	if err := unmarshal(&values); err != nil {
		return err
	}
	number := func(v any) any {
		rv := reflect.ValueOf(v)
		if rv.IsValid() && (rv.Kind() == reflect.Uint || rv.Kind() == reflect.Uint64) {
			return float64(rv.Uint())
		}
		return v
	}
	for key, value := range values {
		if list, ok := value.([]any); ok {
			for i, item := range list {
				list[i] = number(item)
			}
		} else {
			values[key] = number(value)
		}
	}
	*d = values
	return nil
}

// Entity joins optional actor presentation with independently typed state.
// Observations are evaluated on explicit refresh, not during drawing.
type Entity struct {
	// Data holds read-only definition values for rule and presentation expressions.
	// Values are finite scalars or homogeneous scalar lists (at most 4096 items);
	// they are not saved or changed by plan operations.
	Data         DefinitionData  `yaml:"data"`
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
	// Looks dresses each phase (and "closed" for a closed day) so the player
	// sees the time pass: the clock beside the date, the color of its light
	// washed over workspace surfaces, and a short turn card when play moves
	// into it. Omitted phases keep the plain look and show no card.
	Looks map[string]PhaseLook `yaml:"looks"`
}

// PhaseLook is how one phase looks. Time is a short clock text shown after
// the date ("18:30"); Color is the light of the phase, Wash how strongly
// workspace surfaces are covered with it (0..0.8). Card false shows no turn
// card for the phase; Note is an interpolated line under the card's clock.
// Label names the phase on the card (default: a view's phase label).
type PhaseLook struct {
	Label string  `yaml:"label"`
	Time  string  `yaml:"time"`
	Color Color   `yaml:"color"`
	Wash  float64 `yaml:"wash"`
	Card  *bool   `yaml:"card"`
	Note  string  `yaml:"note"`
}

// MaxPhaseWash keeps a washed surface readable.
const MaxPhaseWash = 0.8

// Activity is a bounded amount of work with strict completion dependencies.
type Activity struct {
	// Parent identifies a one-level decomposition; children remain dormant until split.
	Parent          string   `yaml:"parent"`
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
	// After lists reservation IDs whose events must have run before selection.
	After []string `yaml:"after"`
	// Exclusive admits at most one reservation in this group on a calendar date.
	Exclusive string `yaml:"exclusive"`
	// When is a pure eligibility condition, for example public presence attributes.
	When Expr `yaml:"when"`

	// Label is an optional public calendar annotation; it never changes dispatch.
	Label string `yaml:"label"`
	// HideClosed omits this annotation on closed dates, without changing dispatch.
	HideClosed bool   `yaml:"hide_closed"`
	ID         string `yaml:"id"`
	Date       string `yaml:"date"`
	Weekdays   []int  `yaml:"weekdays"`
	Phase      string `yaml:"phase"`
	Event      string `yaml:"event"`
}

// PlanCommand is an immediate operation. Where and Value bind self to each
// selected entity in ID order. A missing Where selects all tagged entities.
type PlanCommand struct {
	Rule   string          `yaml:"rule"`
	Args   map[string]Expr `yaml:"args"`
	Policy string          `yaml:"policy"`

	Op        string `yaml:"op"`
	Entity    IDRef  `yaml:"entity"`
	Tag       string `yaml:"tag"`
	Where     Expr   `yaml:"where"`
	Attribute string `yaml:"attribute"`
	Value     Expr   `yaml:"value"`
	Activity  IDRef  `yaml:"activity"`
	Ledger    string `yaml:"ledger"`
	Period    string `yaml:"period"`
}

func (c *checker) planning() {
	c.planningVersions()
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
	for _, name := range slices.Sorted(maps.Keys(p.Calendar.Looks)) {
		look := p.Calendar.Looks[name]
		lw := w + ".calendar.looks." + name
		if !seen[name] && name != "closed" {
			c.errorf(diag.Shape, lw, "a look names a phase of the calendar or closed")
		}
		if !finiteNumber(look.Wash) || look.Wash < 0 || look.Wash > MaxPhaseWash {
			c.errorf(diag.OutOfRange, lw+".wash", "wash must be 0 through %g", MaxPhaseWash)
		}
		if look.Wash > 0 && !look.Color.Valid {
			c.errorf(diag.Shape, lw+".color", "a wash needs a color")
		}
		if len([]rune(look.Time)) > 12 {
			c.errorf(diag.OutOfRange, lw+".time", "a clock text is at most 12 characters")
		}
		c.text(lw+".note", look.Note, FieldNames)
		c.text(lw+".label", look.Label, FieldNames)
	}
	for _, d := range p.Calendar.Closed {
		checkDate(w+".calendar.closed", d)
	}
	for _, d := range p.Calendar.WeekdaysOff {
		if d < 0 || d > 6 {
			c.errorf(diag.OutOfRange, w+".calendar.weekdays_off", "weekday must be 0 (Sunday) through 6")
		}
	}
	c.planningAttributes(w+".activity_attributes", p.ActivityAttributes, []string{"id", "from", "to", "planned_effort", "deadline", "progress", "observed", "effort", "assignee", "completed", "cancelled", "parent", "split", "active"})
	c.planningAttributes(w+".attributes", p.Attributes, []string{"id", "name", "data"})
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
		for _, k := range slices.Sorted(maps.Keys(e.Data)) {
			at := at + ".data." + k
			if !declaredName.MatchString(k) {
				c.errorf(diag.Shape, at, "invalid definition data key")
			}
			value := e.Data[k]
			if list, ok := value.([]any); ok {
				if len(list) > 4096 {
					c.errorf(diag.OutOfRange, at, "definition data list exceeds 4096 items")
				}
				kind := expr.UnknownType
				for _, item := range list {
					if item == nil || !expr.IsFiniteScalar(item) || (kind != expr.UnknownType && expr.TypeOfValue(item) != kind) {
						c.errorf(diag.Shape, at, "definition data requires homogeneous finite scalar lists")
						break
					}
					kind = expr.TypeOfValue(item)
				}
			} else if value == nil || !expr.IsFiniteScalar(value) {
				c.errorf(diag.Shape, at, "definition data requires finite scalars or scalar lists")
			}
		}
		for _, k := range slices.Sorted(maps.Keys(e.Observations)) {
			if _, ok := p.Attributes[k]; ok || k == "id" || k == "name" || k == "data" {
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
			c.planCommand(fmt.Sprintf("%s.do[%d]", at, i), &cmd, append([]string{}, r.Params...))
		}
	}
	for id, rules := range p.Policies {
		for i, r := range rules {
			at := fmt.Sprintf("%s.policies.%s[%d]", w, id, i)
			c.planningPure(at+".when", r.When)
			c.expr(at+".when", r.When, FieldNames)
			if c.db.Events[r.Event] == nil {
				c.errorf(diag.UnknownReference, at+".event", "unknown policy event")
			} else {
				c.ref(at+".event", "event", r.Event, true)
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
	activityIDs := slices.Sorted(maps.Keys(p.Activities))
	for _, id := range activityIDs {
		a := p.Activities[id]
		at := w + ".activities." + id
		if !declaredName.MatchString(id) {
			c.errorf(diag.Shape, at, "invalid activity ID")
		}
		c.planningPure(at+".observation", a.Observation)
		c.planningNumber(at+".observation", a.Observation)
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

		if a.Parent != "" {
			parent, ok := p.Activities[a.Parent]
			if !ok || a.Parent == id || parent.Parent != "" {
				c.errorf(diag.Shape, at+".parent", "parent must reference a root activity")
			}
			if a.Assignee != "" {
				c.errorf(diag.Shape, at+".assignee", "dormant children must be unassigned")
			}
		}
		visit(id)
	}

	// Accumulate each split once, in stable order, instead of scanning every
	// activity again for every possible parent.
	type splitTotal struct {
		effort float64
		count  int
	}
	splits := make(map[string]splitTotal)
	for _, id := range activityIDs {
		child := p.Activities[id]
		if child.Parent != "" {
			total := splits[child.Parent]
			total.effort += child.Effort
			total.count++
			splits[child.Parent] = total
		}
	}
	for _, id := range activityIDs {
		total := splits[id]
		if total.count > 0 && (total.count < 2 || math.Abs(total.effort-p.Activities[id].Effort) > 1e-8) {
			c.errorf(diag.Shape, w+".activities."+id, "children must conserve parent effort and number at least two")
		}
	}
	for id, l := range p.Ledgers {
		if l.Retain < 1 || l.Retain > 120 {
			c.errorf(diag.OutOfRange, w+".ledgers."+id+".retain", "retain must be 1 through 120")
		}
	}
	c.scheduleDependencies(w, p)
	seen = map[string]bool{}
	for i, r := range p.Schedule {
		at := fmt.Sprintf("%s.schedule[%d]", w, i)
		if !declaredName.MatchString(r.ID) || seen[r.ID] {
			c.errorf(diag.Shape, at+".id", "reservation ID must be unique and nonempty")
		}
		seen[r.ID] = true
		c.expr(at+".when", r.When, FieldNames)
		c.planningPure(at+".when", r.When)
		if r.Date != "" {
			checkDate(at+".date", r.Date)
		}
		if !slices.Contains(p.Calendar.Phases, r.Phase) {
			c.errorf(diag.UnknownReference, at+".phase", "unknown phase")
		}
		if c.db.Events[r.Event] == nil {
			c.errorf(diag.UnknownReference, at+".event", "unknown event")
		} else {
			c.ref(at+".event", "event", r.Event, true)
		}
		for _, d := range r.Weekdays {
			if d < 0 || d > 6 {
				c.errorf(diag.OutOfRange, at+".weekdays", "weekday must be 0 through 6")
			}
		}
	}
}

// A nil params slice denotes an event command; a nonnil slice scopes a rule body.
func (c *checker) planCommand(w string, cmd *PlanCommand, params []string) {
	if cmd == nil {
		return
	}
	allowed := map[string][]string{
		"set": {"entity", "tag", "where", "attribute", "value", "activity"}, "observe": {"entity", "tag", "where", "activity"},
		"split": {"activity"}, "deadline": {"activity", "value"}, "cancel": {"activity", "value"}, "effort": {"activity", "value"}, "progress": {"activity", "value"}, "assign": {"activity", "entity"}, "advance": {}, "dispatch": {},
		"record": {"entity", "tag", "where", "ledger", "period", "value"}, "close": {"ledger", "period"},
		"rule": {"rule", "args"}, "policy": {"policy"},
	}
	fields := map[string]bool{"entity": cmd.Entity.set(), "tag": cmd.Tag != "", "where": cmd.Where.Set(), "attribute": cmd.Attribute != "", "value": cmd.Value.Set(), "activity": cmd.Activity.set(), "ledger": cmd.Ledger != "", "period": cmd.Period != "", "rule": cmd.Rule != "", "args": len(cmd.Args) > 0, "policy": cmd.Policy != ""}
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
	if !slices.Contains([]string{"split", "set", "observe", "assign", "progress", "effort", "cancel", "deadline", "advance", "dispatch", "record", "close", "rule", "policy"}, cmd.Op) {
		c.errorf(diag.NotAWord, w+".op", "unknown planning operation %q", cmd.Op)
	}
	if cmd.Op == "observe" && cmd.Activity.set() && (cmd.Entity.set() || cmd.Tag != "") {
		c.errorf(diag.Shape, w, "observe activity cannot be combined with entity or tag")
	}
	if cmd.Entity.ID != "" {
		if _, ok := p.Entities[cmd.Entity.ID]; !ok {
			c.errorf(diag.UnknownReference, w+".entity", "unknown entity")
		}
	}
	if cmd.Op == "set" {
		attrs := p.Attributes
		if cmd.Activity.set() {
			attrs = p.ActivityAttributes
			if cmd.Entity.set() || cmd.Tag != "" {
				c.errorf(diag.Shape, w, "set activity cannot be combined with entity or tag")
			}
		}
		if _, ok := attrs[cmd.Attribute]; !ok {
			c.errorf(diag.UnknownReference, w+".attribute", "unknown attribute")
		}
	}
	if cmd.Op == "split" || cmd.Op == "deadline" || cmd.Op == "cancel" || cmd.Op == "effort" || cmd.Op == "progress" || cmd.Op == "assign" || (cmd.Op == "observe" || cmd.Op == "set") && cmd.Activity.set() {
		if _, ok := p.Activities[cmd.Activity.ID]; !ok && !cmd.Activity.Expr.Set() {
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
	if cmd.Op == "progress" || cmd.Op == "effort" || cmd.Op == "record" {
		c.planningNumber(w+".value", cmd.Value)
	}
	names := append(slices.Clone(FieldNames), "self")
	over := map[string]*vtype{"self": tAny}
	if params != nil {
		names = append(names, "arg")
		fields := make(map[string]*vtype, len(params))
		for _, param := range params {
			fields[param] = tAny
		}
		over["arg"] = objType("rule arguments", fields, nil)
	}
	for kind, target := range map[string]IDRef{"entity": cmd.Entity, "activity": cmd.Activity} {
		if !target.Expr.Set() {
			continue
		}
		at := w + "." + kind + ".expr"
		c.needsVersion(at, "expression targets", SchemaVersion10)
		targetNames := slices.DeleteFunc(slices.Clone(names), func(name string) bool { return name == "self" })
		targetOver := maps.Clone(over)
		delete(targetOver, "self")
		c.argumentExpression(at, target.Expr, kind, targetNames, targetOver)
	}
	for key, e := range cmd.Args {
		c.exprIn(w+".args."+key, e, names, over)
	}
	for _, e := range append([]Expr{cmd.Where, cmd.Value}, slices.Collect(maps.Values(cmd.Args))...) {
		c.planningPure(w, e)
	}
	c.exprIn(w+".where", cmd.Where, names, over)
	c.exprIn(w+".value", cmd.Value, names, over)
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

// EnsembleChat configures a read-only meeting chat. All labels are authored;
// Reaction names a public entity observation, never a hidden attribute.
type EnsembleChat struct {
	Title     string `yaml:"title"`
	Joined    string `yaml:"joined"`
	Left      string `yaml:"left"`
	CameraOff string `yaml:"camera_off"`
	CameraOn  string `yaml:"camera_on"`
	Reaction  string `yaml:"reaction"`
}

// EnsembleCall presents a desktop call. Variant maps use the base portrait path
// as key and a same-canvas replacement image as value; omission uses motion only.
type EnsembleCall struct {
	Address string `yaml:"address"`
	// Controls are decorative round buttons, each a label or {label, icon, color}.
	Controls []CallControl     `yaml:"controls"`
	Mouth    map[string]string `yaml:"mouth"`
	Blink    map[string]string `yaml:"blink"`
	// Self shows a small self view in the corner of the speaker view: a planning
	// entity, a guest actor or a picture path. Empty shows none.
	Self string `yaml:"self"`
	// SelfLabel names the self view; empty uses the entity's or actor's name.
	SelfLabel string `yaml:"self_label"`
	// SelfBackdrop is a picture behind the self view when self is an actor who is not
	// a guest, or a picture, as an entity's backdrop (view.backdrop) is behind it.
	// Empty keeps the plain tile. An entity's self view takes its own backdrop, and a
	// guest's takes guest_backdrops, instead.
	SelfBackdrop string `yaml:"self_backdrop"`
	// SelfStage puts the self view on the stage, large, while the caller speaks (a
	// line of the call whose speaker is not one of its participants); every
	// participant then stands in the strip. Off, the stage keeps whoever spoke last
	// and only the small self view shows that the caller speaks. Needs self.
	SelfStage bool `yaml:"self_stage"`
	// Colors recolors the call; unset colors keep the built-in look.
	Colors *CallColors `yaml:"colors"`
	// Still keeps the people on camera from swaying and bobbing, so the speaker's
	// face stays where it is framed; blinks and mouths still move.
	Still bool `yaml:"still"`
	// Captions is where the lines of the call show: empty, in captions over the
	// speaker view; window, in the novel message window under the call, the call
	// fitted above it (its controls move under the messages); overlay, in the novel
	// message window laid over the lower part of a full call (layout: full), the
	// window reaching the screen's lower edge.
	Captions string `yaml:"captions"`
	// Layout full fills the screen with the call, the browser maximized: the stage
	// runs to the screen's lower edge under the message window (captions: overlay)
	// and the people on camera are framed in the part above it. Empty keeps the
	// browser window on the wallpaper.
	Layout string `yaml:"layout"`
	// Others is where the participants not on the stage stand: empty, in a strip
	// under the stage; side, in a list of round faces and names in the side column,
	// in place of the chat transcript. A join or a leave shows as a face coming or
	// going there, an authored reaction (chat.reaction) under the name.
	Others string `yaml:"others"`
	// OthersTitle heads the side list (others: side); empty shows no heading.
	OthersTitle string `yaml:"others_title"`
	// WindowOpacity deepens the message window laid over the call (captions:
	// overlay), 0 to 1; zero keeps ui.material.opacities.message.
	WindowOpacity float64 `yaml:"window_opacity"`
}

// PlanningView selects public values and geometry for an authored screen kind.
// Fields without a consumer for that kind are rejected by the loader.
type PlanningView struct {
	// Window adds a desktop title bar to fullscreen islands. Controls are decorative;
	// authored actions and tabs remain the navigation within the tool.
	Window string `yaml:"window"`

	Call *EnsembleCall `yaml:"call"`

	// Background supplies a decorative image, independent of simulation state.
	Background string `yaml:"background"`
	// Burnup adds a dated work chart to an islands surface and records observed
	// totals on observe operations. Labels belong to the author, not the engine.
	Burnup *BurnupView `yaml:"burnup"`
	// Light uses a quiet paper surface for cards and authored controls.
	Light bool `yaml:"light"`
	// FaceShape overrides ui.face_shape on this screen: round or square.
	FaceShape string `yaml:"face_shape"`

	// Chat enables a side message column and camera tiles with a bounded aspect ratio.
	Chat *EnsembleChat `yaml:"chat"`

	Guests []string `yaml:"guests"`
	// GuestBackdrops supplies optional picture paths for non-resident participants.
	GuestBackdrops map[string]string `yaml:"guest_backdrops"`
	// Tabs names two to five fullscreen peers, including this screen. Play replaces
	// the current surface when switching, preserving its original close callback.
	Tabs []string `yaml:"tabs"`
	// Rows groups timeline activities in authored order; omission keeps the task list.
	Rows []TimelineRow `yaml:"rows"`
	// FilterLabel enables the grouped timeline assignee selector and labels its
	// unfiltered state. Order, Visible and Portrait supply its public residents.
	FilterLabel string `yaml:"filter_label"`
	// Markers annotates dates without changing scheduling or simulation rules.
	Markers []TimelineMarker `yaml:"markers"`
	// DateLink opens one screen for any selected date inside the planning range.
	// Arguments can read selected.date, offset, weekday, closed and summary.
	// Explicit marker destinations take precedence.
	DateLink *ScreenCall `yaml:"date_link"`

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
	// Band "none" leaves out the full-screen bottom strip (a result screen whose
	// buttons stand on their own); omitted keeps the strip.
	Band string `yaml:"band"`
	// HeaderFace is an interpolated picture path of a small square face crop
	// drawn at the start of the header text, a state shown by a face instead
	// of a word; HeaderTint is an interpolated "#rrggbb" ring around it.
	HeaderFace string `yaml:"header_face"`
	HeaderTint string `yaml:"header_tint"`
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
}

// BurnupView labels public work totals. Planned optionally evaluates the cumulative
// budget with selected.date; omission uses working-day interpolation per activity.
type BurnupView struct {
	// Bounds positions an optional compact chart on workspace surfaces.
	Bounds       []int  `yaml:"bounds"`
	Compact      bool   `yaml:"compact"`
	Unit         string `yaml:"unit"`
	Title        string `yaml:"title"`
	Total        string `yaml:"total"`
	PlannedLabel string `yaml:"planned_label"`
	Observed     string `yaml:"observed"`
	Today        string `yaml:"today"`
	Deadline     string `yaml:"deadline"`
	Planned      Expr   `yaml:"planned"`
	// Colors recolors the lines; unset colors keep the built-in look.
	Colors *BurnupColors `yaml:"colors"`
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
	// Args bind the lane destination when the player opens it.
	Args map[string]Expr `yaml:"args"`
	// Text adds one authored, interpolated line to an expanded activity.
	// The renderer also includes the full text in the activity description.
	Text string `yaml:"text"`
	// Sample is a saved observed ratio (0..1), negative or absent when unknown.
	// SampleLabel gives its authored provenance, such as the observation date.
	Sample      Expr   `yaml:"sample"`
	SampleLabel string `yaml:"sample_label"`
	// SampleFreshness lets the renderer fade a saved sample from fresh (1) to
	// old (0). Omission preserves full brightness; invalid values use old.
	// Authors define the aging interval and explain age in SampleLabel.
	SampleFreshness Expr `yaml:"sample_freshness"`
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
	// Screen opens an authored date detail from calendar and timeline input.
	// Omission keeps the annotation read-only. One destination is allowed per date.
	Screen string `yaml:"screen"`
	// Args bind the date destination in the current screen scope before opening.
	Args map[string]Expr `yaml:"args"`
}

// PlanningField describes a public value column and optional numeric gauge.
// Marks and Maximum use the same units as the observed value.
type PlanningField struct {
	Key     string  `yaml:"key"`
	Label   string  `yaml:"label"`
	Maximum float64 `yaml:"maximum"`
	// MaximumKey and WarningKey bind public per-entity gauge limits; zero
	// maximum disables the gauge. CriticalRatio colors proximity to the limit.
	MaximumKey    string  `yaml:"maximum_key"`
	WarningKey    string  `yaml:"warning_key"`
	CriticalRatio float64 `yaml:"critical_ratio"`
	// TextKey supplies a public formatted value while Key remains numeric.
	TextKey string    `yaml:"text_key"`
	Marks   []float64 `yaml:"marks"`
}

func (c *checker) planningView(w string, v PlanningView, plain bool) {
	if v.FaceShape != "" && v.FaceShape != "round" && v.FaceShape != "square" {
		c.errorf(diag.Shape, w+".face_shape", "face_shape must be round, square or empty")
	}
	if v.Call != nil {
		if v.Chat == nil {
			c.errorf(diag.OutOfRange, w+".call", "requires chat")
		}
		if len(v.Call.Controls) > 6 {
			c.errorf(diag.OutOfRange, w+".call.controls", "at most six controls")
		}
		c.callControls(w+".call.controls", v.Call)
		if v.Call.Self != "" && !c.callSelf(v.Call.Self) {
			c.file(w+".call.self", v.Call.Self)
		}
		c.text(w+".call.self_label", v.Call.SelfLabel, FieldNames)
		if b := v.Call.SelfBackdrop; b != "" {
			switch {
			case v.Call.Self == "":
				c.errorf(diag.Shape, w+".call.self_backdrop", "needs self, the view it is behind")
			case c.isEntity(v.Call.Self):
				c.errorf(diag.Shape, w+".call.self_backdrop", "self %q is an entity: its view takes the entity's backdrop (view.backdrop)", v.Call.Self)
			case slices.Contains(v.Guests, v.Call.Self):
				c.errorf(diag.Shape, w+".call.self_backdrop", "self %q is a guest: its view takes guest_backdrops", v.Call.Self)
			default:
				c.file(w+".call.self_backdrop", b)
			}
		}
		if v.Call.SelfStage && v.Call.Self == "" {
			c.errorf(diag.Shape, w+".call.self_stage", "needs self, the view put on the stage")
		}
		if v.Call.Captions != "" && v.Call.Captions != "window" && v.Call.Captions != "overlay" {
			c.errorf(diag.OutOfRange, w+".call.captions", "want window or overlay, got %q", v.Call.Captions)
		}
		if v.Call.Layout != "" && v.Call.Layout != "full" {
			c.errorf(diag.OutOfRange, w+".call.layout", "want full, got %q", v.Call.Layout)
		}
		if (v.Call.Layout == "full") != (v.Call.Captions == "overlay") {
			c.errorf(diag.Shape, w+".call.layout", "layout: full and captions: overlay go together: a full call shows its lines in the window over it")
		}
		if v.Call.Others != "" && v.Call.Others != "side" {
			c.errorf(diag.OutOfRange, w+".call.others", "want side, got %q", v.Call.Others)
		}
		c.text(w+".call.others_title", v.Call.OthersTitle, FieldNames)
		if !finiteNumber(v.Call.WindowOpacity) || v.Call.WindowOpacity < 0 || v.Call.WindowOpacity > 1 {
			c.errorf(diag.OutOfRange, w+".call.window_opacity", "want 0 to 1, got %v", v.Call.WindowOpacity)
		}
		if v.Call.WindowOpacity != 0 && v.Call.Captions != "overlay" {
			c.errorf(diag.Shape, w+".call.window_opacity", "needs captions: overlay, the window over the call")
		}
	}

	if v.Burnup != nil {
		if b := v.Burnup.Bounds; len(b) != 0 && (len(b) != 4 || b[0] < 0 || b[1] < 0 || b[2] < 120 || b[3] < 48) {
			c.errorf(diag.OutOfRange, w+".burnup.bounds", "requires x, y and a size of at least 120 by 48")
		}
		c.planningPure(w+".burnup.planned", v.Burnup.Planned)
		c.planningNumber(w+".burnup.planned", v.Burnup.Planned)
		c.exprIn(w+".burnup.planned", v.Burnup.Planned, append(slices.Clone(FieldNames), "selected"), nil)
	}

	for _, actor := range v.Guests {
		if c.db.Actors[actor] == nil {
			c.errorf(diag.UnknownReference, w+".guests", "unknown actor %q", actor)
		}
	}
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
			c.useDefinition("screen", id)
			screen := c.db.Game.Screens[id]
			if screen == nil || !screen.View.Fullscreen || seenTabs[id] {
				c.errorf(diag.UnknownReference, w+".tabs", "unknown, duplicate or non-fullscreen tab %q", id)
			} else if len(screen.Params) > 0 {
				c.errorf(diag.Shape, w+".tabs", "tab screens cannot require arguments")
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
			c.text(at+".text", item.Text, FieldNames)
			c.text(at+".sample_label", item.SampleLabel, FieldNames)
			c.planningPure(at+".sample_freshness", item.SampleFreshness)
			c.exprIn(at+".sample_freshness", item.SampleFreshness, FieldNames, nil)
			c.text(at+".forecast_label", item.ForecastLabel, FieldNames)
			if item.Screen != "" && c.db.Game.Screens[item.Screen] == nil {
				c.errorf(diag.UnknownReference, at, "unknown timeline screen %q", item.Screen)
			} else if item.Screen != "" {
				c.screenCall(at, ScreenCall{Screen: item.Screen, Args: item.Args}, FieldNames)
			} else if len(item.Args) > 0 {
				c.errorf(diag.Shape, at, "arguments require a destination screen")
			}
		}
	}
	if v.DateLink != nil {
		at := w + ".date_link"
		c.needsVersion(at, "selected calendar dates", SchemaVersion10)
		if c.db.Game.Screens[v.DateLink.Screen] == nil {
			c.errorf(diag.UnknownReference, at, "date link requires an authored screen")
		} else {
			c.screenCall(at, *v.DateLink, append(slices.Clone(FieldNames), "selected"))
		}
	}
	destinations := map[string]TimelineMarker{}
	for i, marker := range v.Markers {
		if _, err := time.Parse(time.DateOnly, marker.Date); err != nil || (marker.Label == "" && marker.Screen == "") {
			c.errorf(diag.Shape, fmt.Sprintf("%s.markers[%d]", w, i), "timeline marker requires an ISO date and a label or screen")
		}
		if marker.Screen == "" && len(marker.Args) > 0 {
			c.errorf(diag.Shape, fmt.Sprintf("%s.markers[%d].args", w, i), "arguments require a destination screen")
		}
		if marker.Screen != "" {
			at := fmt.Sprintf("%s.markers[%d].screen", w, i)
			if c.db.Game.Screens[marker.Screen] == nil {
				c.errorf(diag.UnknownReference, at, "unknown calendar screen %q", marker.Screen)
			} else {
				c.screenCall(fmt.Sprintf("%s.markers[%d]", w, i), ScreenCall{Screen: marker.Screen, Args: marker.Args}, FieldNames)
			}
			if previous, ok := destinations[marker.Date]; ok && (previous.Screen != marker.Screen || !reflect.DeepEqual(previous.Args, marker.Args)) {
				c.errorf(diag.Shape, at, "calendar date has conflicting screens")
			}
			destinations[marker.Date] = marker
		}
	}
	c.text(w+".window", v.Window, FieldNames)
	c.text(w+".header", v.Header, FieldNames)
	c.text(w+".header_face", v.HeaderFace, FieldNames)
	if v.HeaderFace != "" && !strings.Contains(v.HeaderFace, "{") {
		c.file(w+".header_face", v.HeaderFace)
	}
	c.text(w+".header_tint", v.HeaderTint, FieldNames)
	if v.HeaderTint != "" && !strings.Contains(v.HeaderTint, "{") && !dotColor.MatchString(v.HeaderTint) {
		c.errorf(diag.Shape, w+".header_tint", "a header tint is a #rrggbb color")
	}
	if (v.HeaderFace != "" || v.HeaderTint != "") && !v.Fullscreen {
		c.errorf(diag.Shape, w+".header_face", "a header face belongs to a full-screen view")
	}
	c.text(w+".footer", v.Footer, FieldNames)
	if v.Band != "" && v.Band != "none" {
		c.errorf(diag.OutOfRange, w+".band", "want none, got %q", v.Band)
	}
	seen := map[string]bool{}
	for _, id := range v.Order {
		if _, ok := p.Entities[id]; !ok || seen[id] {
			c.errorf(diag.Shape, w+".order", "unknown or duplicate entity %q", id)
		}
		seen[id] = true
	}
	keys := []string{v.Visible, v.Highlight, v.Detail, v.Icon, v.Portrait, v.Camera, v.Badge, v.Backdrop, v.Mask}
	for i, f := range v.Fields {
		at := fmt.Sprintf("%s.fields[%d]", w, i)
		if strings.Contains(f.Key, "{") && plain {
			c.needsVersion(at+".key", "field key interpolation", SchemaVersion10)
			c.text(at+".key", f.Key, FieldNames)
		} else {
			keys = append(keys, f.Key, f.MaximumKey, f.WarningKey, f.TextKey)
		}
		if plain {
			if strings.Contains(f.Label, "{") {
				c.needsVersion(at+".label", "field label interpolation", SchemaVersion10)
			}
			c.text(at+".label", f.Label, FieldNames)
		}
		if f.CriticalRatio < 0 || f.CriticalRatio > 1 || math.IsNaN(f.CriticalRatio) {
			c.errorf(diag.Shape, w, "critical ratio must be between zero and one")
		}
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

// planningNumber rejects known nonnumeric results without evaluating formulas.
// Dynamic references are checked by the executor when their values are available.
func (c *checker) planningNumber(w string, e Expr) {
	if !e.Set() {
		return
	}
	t := c.resultType(e, append(slices.Clone(FieldNames), "self", "arg"))
	if t & ^(expr.NumberType|expr.UnknownType) != 0 {
		c.errorf(diag.Expression, w, "planning value requires a number; use a numeric expression or an explicit conversion such as int(value)")
	}
}

func (c *checker) planningAttributes(w string, attrs map[string]Attribute, reserved []string) {
	for _, id := range slices.Sorted(maps.Keys(attrs)) {
		a := attrs[id]
		at := w + "." + id
		if !declaredName.MatchString(id) || slices.Contains(reserved, id) {
			c.errorf(diag.Shape, at, "invalid or reserved attribute ID")
		}
		if _, err := a.Value(a.Initial); err != nil {
			c.errorf(diag.Shape, at+".initial", "%v", err)
		}
		if a.Type != "number" && (a.Min != nil || a.Max != nil) {
			c.errorf(diag.Shape, at, "bounds require number type")
		}
	}
}

// planningVersions keeps the planning vocabulary, the planning views of
// screens and their bounded entries behind schema 10, so a game that still
// says 9 is known to load in v0.15.
func (c *checker) planningVersions() {
	g := &c.db.Game
	if g.Planning != nil {
		c.needsVersion("game.yaml planning", "planning", SchemaVersion10)
	}
	for _, name := range slices.Sorted(maps.Keys(g.Screens)) {
		screen := g.Screens[name]
		if screen == nil {
			continue
		}
		w := "game.yaml.screens." + name
		if screen.Kind != "" {
			c.needsVersion(w+".kind", "planning views", SchemaVersion10)
		}
		if !reflect.ValueOf(screen.View).IsZero() {
			c.needsVersion(w+".view", "screen views", SchemaVersion10)
		}
		for i, e := range screen.Entries {
			if e.Destination != "" || e.Muted.Set() || e.Icon != "" || e.Portrait || e.Tone != "" || len(e.Bounds) > 0 ||
				e.Style != "" || e.Ticket != nil || e.Carousel != "" || e.Cost != "" || e.Unavailable.Set() || e.Reason != "" ||
				e.Drag != "" || e.Drop != "" || e.Accepts.Set() || e.Hint != "" || e.Peek != "" || e.Dot != "" || e.Sign != "" || e.Bust != nil || e.Meter != nil || e.Mark != "" {
				c.needsVersion(fmt.Sprintf("%s.entries[%d]", w, i), "workspace entries", SchemaVersion10)
			}
		}
	}
}
