package gozen

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer3/scope"
)

// Query evaluates an expression on the state and puts its value in out, a pointer: a
// number in *float64 or *int, a string, a bool, a list in a slice, a set of named
// values (menu[0], cell(1, 2), pos) in a struct or a map, anything in *any. The names
// are those of the YAML (flag, var, gold, item, actor, party, location, pos, menu, map,
// cell, result, over, cleared, repairs). An invalid expression is ErrInvalid; a value
// that does not fit out is an error. An object (party,
// actor.ID, party.members[0]) becomes a map or a struct of its fields; the names that
// cannot be listed (party.with_poison) are read one at a time.
func (s *Session) Query(src string, out any) error {
	e, err := expr.Parse(src)
	if err != nil {
		return fmt.Errorf("%w: %q: %w", ErrInvalid, src, err)
	}
	v, err := e.Eval(expr.Chain{scope.Env(s.db, s.st, &s.last), s.st.Env()})
	if err != nil {
		return fmt.Errorf("%w: %q: %w", ErrInvalid, src, err)
	}
	plain, err := toPlain(v)
	if err != nil {
		return fmt.Errorf("%q: %w", src, err)
	}
	switch p := out.(type) {
	case *any:
		*p = plain
		return nil
	case *float64:
		if f, ok := plain.(float64); ok {
			*p = f
			return nil
		}
	case *string:
		if str, ok := plain.(string); ok {
			*p = str
			return nil
		}
	case *bool:
		if b, ok := plain.(bool); ok {
			*p = b
			return nil
		}
	}
	if rv := reflect.ValueOf(out); rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("query %q: out must be a non-nil pointer", src)
	}
	b, err := json.Marshal(plain)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("query %q: %w", src, err)
	}
	return nil
}

// toPlain turns an expression value into numbers, strings, bools, lists and maps.
func toPlain(v any) (any, error) {
	switch x := v.(type) {
	case nil, bool, string, float64:
		return x, nil
	case int:
		return float64(x), nil
	case expr.Map:
		out := make(map[string]any, len(x))
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p, err := toPlain(x[k])
			if err != nil {
				return nil, err
			}
			out[k] = p
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			p, err := toPlain(e)
			if err != nil {
				return nil, err
			}
			out[i] = p
		}
		return out, nil
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice {
		out := make([]any, rv.Len())
		for i := range out {
			p, err := toPlain(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			out[i] = p
		}
		return out, nil
	}
	if o, ok := v.(interface {
		expr.Object
		Keys() []string
	}); ok {
		out := map[string]any{}
		for _, k := range o.Keys() {
			f, ok := o.Field(k)
			if !ok {
				continue
			}
			p, err := toPlain(f)
			if err != nil {
				continue // things inside what cannot list names (copied only when they can be listed)
			}
			out[k] = p
		}
		return out, nil
	}
	return nil, fmt.Errorf("the value is an object: ask for one of its fields")
}
