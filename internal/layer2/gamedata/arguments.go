package gamedata

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

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
	if len(ev.Params) != len(call.Args) {
		return nil, errors.New("arguments must match event parameters")
	}
	if len(ev.Params) == 0 {
		return nil, nil
	}
	out := expr.Map{}
	for _, name := range slices.Sorted(maps.Keys(ev.Params)) {
		e, ok := call.Args[name]
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
			v, err = db.argumentValue(ev.Params[name], v)
		}
		if err != nil {
			return nil, fmt.Errorf("event %s argument %s: %w", call.Event, name, err)
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
	if len(call.Args) != len(ev.Params) {
		c.errorf(diag.Shape, w, "arguments must match event parameters; supply every declared parameter")
	}
	for _, name := range slices.Sorted(maps.Keys(ev.Params)) {
		if _, ok := call.Args[name]; !ok {
			c.errorf(diag.Shape, w, "missing argument %q", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(call.Args)) {
		e := call.Args[name]
		kind, ok := ev.Params[name]
		if !ok {
			c.errorf(diag.Shape, w, "unknown argument %q", name)
			continue
		}
		at := w + ".args." + name
		c.expr(at, e, names)
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
}

// ValidateArguments copies saved/root bindings after checking their names and
// declared types. Session and event use it before replacing execution context.
func (db *DB) ValidateArguments(event string, values map[string]any) (expr.Map, error) {
	ev := db.Events[event]
	if ev == nil {
		return nil, fmt.Errorf("unknown event %q", event)
	}
	if len(ev.Params) != len(values) {
		return nil, errors.New("arguments must match event parameters")
	}
	if len(ev.Params) == 0 {
		return nil, nil
	}
	out := expr.Map{}
	for _, name := range slices.Sorted(maps.Keys(ev.Params)) {
		v, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("missing argument %s", name)
		}
		value, err := db.argumentValue(ev.Params[name], v)
		if err != nil {
			return nil, fmt.Errorf("argument %s: %w", name, err)
		}
		out[name] = value
	}
	return out, nil
}
