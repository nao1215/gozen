package gamedata

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

// IDRef is a literal ID or a pure expression consumed by planning operations.
// Literal IDs retain their existing JSON representation for replay fingerprints.
// Resolve checks dynamic type and existence before an operation changes state.
type IDRef struct {
	ID   string `yaml:"-"`
	Expr Expr   `yaml:"expr"`
}

func (v *IDRef) UnmarshalYAML(unmarshal func(any) error) error {
	var id string
	if err := unmarshal(&id); err == nil {
		*v = IDRef{ID: id}
		return nil
	}
	type plain IDRef
	var out plain
	if err := unmarshal(&out); err != nil {
		return err
	}
	if !out.Expr.Set() {
		return errors.New("ID expression requires expr")
	}
	*v = IDRef(out)
	return nil
}

func (v IDRef) MarshalJSON() ([]byte, error) {
	if !v.Expr.Set() {
		return json.Marshal(v.ID)
	}
	return json.Marshal(struct {
		Expr Expr `json:"expr"`
	}{v.Expr})
}

// set distinguishes an omitted target from an expression awaiting evaluation.
func (v IDRef) set() bool { return v.ID != "" || v.Expr.Set() }

// Resolve evaluates a target in the caller's scope. Literal empty IDs preserve
// operations such as clearing an assignment; dynamic empty or missing IDs fail.
// The planning interpreter uses this before the operation mutates planning state.
func (v IDRef) Resolve(db *DB, kind string, env expr.Env) (string, error) {
	if !v.Expr.Set() {
		return v.ID, nil
	}
	if v.ID != "" {
		return "", errors.New("target has both a literal and an expression")
	}
	args, err := db.bindArguments(map[string]string{"target": kind}, map[string]Expr{"target": v.Expr}, env)
	if err != nil {
		return "", err
	}
	id, ok := args["target"].(string)
	if !ok {
		return "", errors.New("target must resolve to an ID")
	}
	return id, nil
}

// EventCall is a named invocation consumed by the event interpreter. Args are pure
// expressions evaluated in the caller's scope before any callee command executes.
type EventCall struct {
	Event  string          `yaml:"event"`
	Args   map[string]Expr `yaml:"args"`
	mapped bool
}

func (v *EventCall) UnmarshalYAML(unmarshal func(any) error) error {
	var name string
	if err := unmarshal(&name); err == nil {
		*v = EventCall{Event: name}
		return nil
	}
	type plain EventCall
	var out plain
	if err := unmarshal(&out); err != nil {
		return err
	}
	*v = EventCall(out)
	v.mapped = true
	return nil
}

// ScreenCall opens an authored screen with pure expressions evaluated by event.
// The scalar form keeps existing parameterless authored and built-in screens.
type ScreenCall struct {
	Screen string          `yaml:"screen"`
	Args   map[string]Expr `yaml:"args"`
	mapped bool
}

func (v *ScreenCall) UnmarshalYAML(unmarshal func(any) error) error {
	var name string
	if err := unmarshal(&name); err == nil {
		*v = ScreenCall{Screen: name}
		return nil
	}
	type plain ScreenCall
	var out plain
	if err := unmarshal(&out); err != nil {
		return err
	}
	*v = ScreenCall(out)
	v.mapped = true
	return nil
}

// MarshalJSON preserves the fingerprint of existing literal open commands.
func (v ScreenCall) MarshalJSON() ([]byte, error) {
	if len(v.Args) == 0 {
		return json.Marshal(v.Screen)
	}
	type plain ScreenCall
	return json.Marshal(plain(v))
}

// BindScreen freezes an invocation before event emits an Open request. Built-in
// screens accept only the scalar, parameterless form.
func (db *DB) BindScreen(call ScreenCall, env expr.Env) (expr.Map, error) {
	screen := db.Game.Screens[call.Screen]
	if screen == nil {
		if !slices.Contains(OpenScreens, call.Screen) || call.mapped || len(call.Args) > 0 {
			return nil, fmt.Errorf("screen %q does not accept this invocation", call.Screen)
		}
		return nil, nil
	}
	return db.bindArguments(screen.Params, call.Args, env)
}

// argumentValue validates a declared parameter for the interpreter and saved
// cursors. Reference values remain scalar IDs; this never changes game state.
func (db *DB) argumentValue(kind string, value any) (any, error) {
	if slices.Contains([]string{"number", "bool", "string"}, kind) {
		return (Attribute{Type: kind}).Value(value)
	}
	id, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("expected %s ID or date string", kind)
	}
	valid := false
	switch kind {
	case "entity":
		if p := db.Game.Planning; p != nil {
			_, valid = p.Entities[id]
		}
	case "activity":
		if p := db.Game.Planning; p != nil {
			_, valid = p.Activities[id]
		}
	case "actor":
		valid = db.Actors[id] != nil
	case "date":
		_, err := time.Parse(time.DateOnly, id)
		valid = err == nil
	}
	if !valid {
		return nil, fmt.Errorf("invalid %s %q", kind, id)
	}
	return id, nil
}

// Bind evaluates and validates all required arguments in sorted name order. The
// returned map is detached and must be treated as immutable by runner frames.
func (db *DB) Bind(call EventCall, env expr.Env) (expr.Map, error) {
	ev := db.Events[call.Event]
	if ev == nil {
		return nil, fmt.Errorf("unknown event %q", call.Event)
	}
	return db.bindArguments(ev.Params, call.Args, env)
}

func (db *DB) bindArguments(params map[string]string, args map[string]Expr, env expr.Env) (expr.Map, error) {
	if len(params) != len(args) {
		return nil, errors.New("arguments must match parameters")
	}
	if len(params) == 0 {
		return nil, nil
	}
	out := expr.Map{}
	for _, name := range slices.Sorted(maps.Keys(params)) {
		e, ok := args[name]
		if !ok || !e.Set() {
			return nil, fmt.Errorf("missing argument %s", name)
		}
		for _, n := range e.Names() {
			if n == "rand" || n == "chance" {
				return nil, fmt.Errorf("argument %s must be pure", name)
			}
		}
		v, err := e.Eval(env)
		if err == nil {
			v, err = db.argumentValue(params[name], v)
		}
		if err != nil {
			return nil, fmt.Errorf("argument %s: %w", name, err)
		}
		out[name] = v
	}
	return out, nil
}

func (c *checker) argumentType() *vtype {
	fields := map[string]*vtype{}
	for name, kind := range c.params {
		t := tStr
		switch kind {
		case "number":
			t = tNum
		case "bool":
			t = tBool
		}
		fields[name] = t
	}
	return objType("event arguments", fields, nil)
}

func (c *checker) eventCall(w string, call EventCall, names []string) {
	ev := c.db.Events[call.Event]
	if ev == nil {
		c.errorf(diag.UnknownReference, w, "unknown event %q", call.Event)
		return
	}
	if call.mapped || len(call.Args) > 0 {
		c.needsVersion(w, "event arguments", SchemaVersion10)
	}
	c.callArguments(w, ev.Params, call.Args, names)
}

func (c *checker) screenCall(w string, call ScreenCall, names []string) {
	if call.mapped || len(call.Args) > 0 {
		c.needsVersion(w, "screen arguments", SchemaVersion10)
	}
	if screen := c.db.Game.Screens[call.Screen]; screen != nil {
		c.callArguments(w, screen.Params, call.Args, names)
	} else if call.mapped || len(call.Args) > 0 {
		c.errorf(diag.Shape, w, "arguments require an authored screen; use a scalar built-in name")
	}
}

func (c *checker) parameters(w string, params map[string]string) {
	if params == nil {
		return
	}
	c.needsVersion(w, "parameters", SchemaVersion10)
	for name, kind := range params {
		if !varName.MatchString(name) || !slices.Contains([]string{"number", "bool", "string", "entity", "activity", "actor", "date"}, kind) {
			c.errorf(diag.Shape, w+"."+name, "invalid parameter name or type %q", kind)
		}
	}
}

func (c *checker) callArguments(w string, params map[string]string, args map[string]Expr, names []string) {
	if len(args) != len(params) {
		c.errorf(diag.Shape, w, "arguments must match parameters; supply every declared parameter")
	}
	for _, name := range slices.Sorted(maps.Keys(params)) {
		if _, ok := args[name]; !ok {
			c.errorf(diag.Shape, w, "missing argument %q", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(args)) {
		e := args[name]
		kind, ok := params[name]
		if !ok {
			c.errorf(diag.Shape, w, "unknown argument %q", name)
			continue
		}
		at := w + ".args." + name
		c.argumentExpression(at, e, kind, names, nil)
	}
}

func (c *checker) argumentExpression(at string, e Expr, kind string, names []string, over map[string]*vtype) {
	c.exprIn(at, e, names, over)
	c.planningPure(at, e)
	expected := expr.StringType
	switch kind {
	case "number":
		expected = expr.NumberType
	case "bool":
		expected = expr.BoolType
	}
	if got := c.resultType(e, names); got != expr.UnknownType && got != expected {
		c.errorf(diag.Expression, at, "expected %s; pass a matching typed value", kind)
	}
	if slices.Contains([]string{"entity", "activity", "actor", "date"}, kind) {
		for _, ref := range e.Refs() {
			if ref.Name != "arg" || len(ref.Path) == 0 {
				continue
			}
			source := c.params[ref.Path[0].Name]
			if slices.Contains([]string{"entity", "activity", "actor", "date"}, source) && source != kind {
				c.errorf(diag.Expression, at, "expected %s, got %s parameter; pass a matching typed value", kind, source)
			}
		}
	}
	if len(e.Names()) == 0 {
		v, err := e.Eval(nil)
		if err == nil {
			_, err = c.db.argumentValue(kind, v)
		}
		if err != nil {
			c.errorf(diag.Expression, at, "expected %s: %v", kind, err)
		}
	}
}

// ValidateArguments copies saved/root bindings after checking their names and
// declared types for an event or named screen entry. Replay consumers use it
// before replacing execution context.
func (db *DB) ValidateArguments(event string, values map[string]any) (expr.Map, error) {
	ev := db.Events[event]
	if ev != nil {
		return db.validateArguments(ev.Params, values)
	}
	parts := strings.Split(event, ":")
	if len(parts) == 3 && parts[0] == "screen" {
		index, err := strconv.Atoi(parts[2])
		screen := db.Game.Screens[parts[1]]
		if err == nil && strconv.Itoa(index) == parts[2] && screen != nil && index >= 0 && index < len(screen.Entries) {
			return db.validateArguments(screen.Params, values)
		}
	}
	return nil, fmt.Errorf("unknown bound root %q", event)
}

func (db *DB) validateArguments(params map[string]string, values map[string]any) (expr.Map, error) {
	if len(params) != len(values) {
		return nil, errors.New("arguments must match parameters")
	}
	if len(params) == 0 {
		return nil, nil
	}
	out := expr.Map{}
	for _, name := range slices.Sorted(maps.Keys(params)) {
		v, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("missing argument %s", name)
		}
		value, err := db.argumentValue(params[name], v)
		if err != nil {
			return nil, fmt.Errorf("argument %s: %w", name, err)
		}
		out[name] = value
	}
	return out, nil
}
