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
	for i := range values.Len() {
		*steps++
		if *steps > 65536 {
			return nil, errors.New("collection evaluation exceeds 65536 item visits")
		}
		item := values.Index(i).Interface()
		v, err := n.body.eval(Chain{Map{"\x00item": item}, env})
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
