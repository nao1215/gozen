package expr

import (
	"errors"
	"fmt"
	"math"
	"reflect"
)

// Collection expressions bind it lexically, leaving self and arg untouched.
// The cap applies before evaluating the body, including for filter's output.
const collectionLimit = 4096

type collection struct {
	name         string
	source, body node
}
type element struct{ source node }

// Evaluation scopes are private to one Eval call. Reusing the item scope avoids
// allocating a map and a chain for every row, while nested collections keep
// separate items and share only their visit budget.
type collectionBudget struct {
	Env
	steps int
}

func (e *collectionBudget) Lookup(name string) (any, bool) {
	if name == "\x00collectionBudget" {
		return &e.steps, true
	}
	if e.Env == nil {
		return nil, false
	}
	return e.Env.Lookup(name)
}

type collectionItem struct {
	Env
	value any
}

func (e *collectionItem) Lookup(name string) (any, bool) {
	if name == "\x00item" {
		return e.value, true
	}
	return e.Env.Lookup(name)
}

func collectionName(name string) bool { return name == "sum" || name == "count" || name == "filter" }

func (n *element) eval(env Env) (any, error) {
	v, ok := env.Lookup("\x00item")
	if !ok {
		return nil, errors.New("collection item outside its scope")
	}
	return v, nil
}

func (n *collection) eval(env Env) (any, error) {
	value, err := n.source.eval(env)
	if err != nil {
		return nil, err
	}
	values := reflect.ValueOf(value)
	if values.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%s needs a list", n.name)
	}
	if values.Len() > collectionLimit {
		return nil, fmt.Errorf("%s exceeds %d items", n.name, collectionLimit)
	}
	total := 0.0
	selected := []any{}
	budget, _ := env.Lookup("\x00collectionBudget")
	steps, ok := budget.(*int)
	if !ok {
		return nil, errors.New("missing collection evaluation budget")
	}
	scope := &collectionItem{Env: env}
	for i := range values.Len() {
		*steps++
		if *steps > 65536 {
			return nil, errors.New("collection evaluation exceeds 65536 item visits")
		}
		item := values.Index(i).Interface()
		scope.value = item
		v, err := n.body.eval(scope)
		if err != nil {
			return nil, fmt.Errorf("%s item %d: %w", n.name, i, err)
		}
		if n.name == "sum" {
			if TypeOfValue(v) != NumberType || !IsFiniteScalar(v) {
				return nil, errors.New("sum body must return a finite number")
			}
			number, _ := ToNumber(v)
			total += number
			if math.IsInf(total, 0) || math.IsNaN(total) {
				return nil, errors.New("sum result must be finite")
			}
		} else {
			yes, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("%s predicate must return a boolean", n.name)
			}
			if yes {
				total++
				if n.name == "filter" {
					selected = append(selected, item)
				}
			}
		}
	}
	if n.name == "filter" {
		return selected, nil
	}
	return total, nil
}

func (p *parser) collection(name string) (node, error) {
	p.hasCollections = true
	p.next() // opening parenthesis
	source, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if err = p.expect(","); err != nil {
		return nil, err
	}
	previous := p.itemSource
	p.itemSource = source
	body, err := p.ternary()
	p.itemSource = previous
	if err != nil {
		return nil, err
	}
	if err = p.expect(")"); err != nil {
		return nil, err
	}
	return &collection{name: name, source: source, body: body}, nil
}

// boundedRange supports finite date offsets and other authored integer domains.
func boundedRange(args []any) (any, error) {
	if len(args) != 2 {
		return nil, errors.New("range takes two integer bounds")
	}
	bounds := [2]float64{}
	for i, value := range args {
		n, ok := ToNumber(value)
		if !ok || TypeOfValue(value) != NumberType || !IsFiniteScalar(value) || math.Trunc(n) != n || math.Abs(n) > 9007199254740991 {
			return nil, errors.New("range bounds must be exactly representable integers")
		}
		bounds[i] = n
	}
	size := bounds[1] - bounds[0]
	if size < 0 || size > collectionLimit {
		return nil, errors.New("range must contain 0..4096 items in ascending order")
	}
	values := make([]any, int(size))
	for i := range values {
		values[i] = bounds[0] + float64(i)
	}
	return values, nil
}

func firstOr(args []any) (any, error) {
	if len(args) != 2 {
		return nil, errors.New("first takes a list and a fallback")
	}
	values := reflect.ValueOf(args[0])
	if values.Kind() != reflect.Slice {
		return nil, errors.New("first needs a list")
	}
	if values.Len() > collectionLimit {
		return nil, errors.New("first exceeds 4096 items")
	}
	if values.Len() == 0 {
		return args[1], nil
	}
	return values.Index(0).Interface(), nil
}
