package expr

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

type node interface {
	eval(env Env) (any, error)
}

type (
	lit   struct{ v any }
	ident struct{ name string }
	field struct {
		x    node
		name string
	}
	index struct{ x, i node }
	call  struct {
		fn   node
		args []node
	}
	not   struct{ x node }
	neg   struct{ x node }
	logic struct {
		or   bool
		l, r node
	}
	binop struct {
		op   string
		l, r node
	}
	cond struct{ c, a, b node }
)

// walk calls f on n and every node under it.
func walk(n node, f func(node)) {
	f(n)
	switch x := n.(type) {
	case *field:
		walk(x.x, f)
	case *index:
		walk(x.x, f)
		walk(x.i, f)
	case *call:
		walk(x.fn, f)
		for _, a := range x.args {
			walk(a, f)
		}
	case *not:
		walk(x.x, f)
	case *neg:
		walk(x.x, f)
	case *logic:
		walk(x.l, f)
		walk(x.r, f)
	case *binop:
		walk(x.l, f)
		walk(x.r, f)
	case *cond:
		walk(x.c, f)
		walk(x.a, f)
		walk(x.b, f)
	}
}

func (n *lit) eval(Env) (any, error) { return n.v, nil }

func (n *ident) eval(env Env) (any, error) {
	if env != nil {
		if v, ok := env.Lookup(n.name); ok {
			return v, nil
		}
	}
	if f, ok := builtins[n.name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("unknown name %q", n.name)
}

func (n *field) eval(env Env) (any, error) {
	x, err := n.x.eval(env)
	if err != nil {
		return nil, err
	}
	o, ok := x.(Object)
	if !ok {
		if x == nil {
			return nil, fmt.Errorf("field %q of nil", n.name)
		}
		return nil, fmt.Errorf("%s has no field %q", describe(x), n.name)
	}
	v, ok := o.Field(n.name)
	if !ok {
		return nil, fmt.Errorf("unknown field %q", n.name)
	}
	return v, nil
}

func (n *index) eval(env Env) (any, error) {
	x, err := n.x.eval(env)
	if err != nil {
		return nil, err
	}
	i, err := n.i.eval(env)
	if err != nil {
		return nil, err
	}
	if s, ok := i.(string); ok {
		if o, ok := x.(Object); ok {
			v, _ := o.Field(s)
			return v, nil
		}
	}
	f, ok := ToNumber(i)
	if !ok {
		return nil, fmt.Errorf("index %s is not a number", describe(i))
	}
	rv := reflect.ValueOf(x)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%s cannot be indexed", describe(x))
	}
	k := int(f)
	if k < 0 || k >= rv.Len() {
		return nil, nil // out of range is nil: "enemies[3].hp" fails, "enemies[3] == nil" holds
	}
	return rv.Index(k).Interface(), nil
}

func (n *call) eval(env Env) (any, error) {
	fv, err := n.fn.eval(env)
	if err != nil {
		return nil, err
	}
	f, ok := fv.(Func)
	if !ok {
		if g, ok2 := fv.(func([]any) (any, error)); ok2 {
			f = g
		} else {
			return nil, fmt.Errorf("%s is not a function", describe(fv))
		}
	}
	args := make([]any, len(n.args))
	for i, a := range n.args {
		if args[i], err = a.eval(env); err != nil {
			return nil, err
		}
	}
	return f(args)
}

func (n *not) eval(env Env) (any, error) {
	x, err := n.x.eval(env)
	if err != nil {
		return nil, err
	}
	return !Truthy(x), nil
}

func (n *neg) eval(env Env) (any, error) {
	x, err := n.x.eval(env)
	if err != nil {
		return nil, err
	}
	f, ok := ToNumber(x)
	if !ok {
		return nil, fmt.Errorf("cannot negate %s", describe(x))
	}
	return -f, nil
}

func (n *logic) eval(env Env) (any, error) {
	l, err := n.l.eval(env)
	if err != nil {
		return nil, err
	}
	if Truthy(l) == n.or {
		return n.or, nil
	}
	r, err := n.r.eval(env)
	if err != nil {
		return nil, err
	}
	return Truthy(r), nil
}

func (n *cond) eval(env Env) (any, error) {
	c, err := n.c.eval(env)
	if err != nil {
		return nil, err
	}
	if Truthy(c) {
		return n.a.eval(env)
	}
	return n.b.eval(env)
}

func (n *binop) eval(env Env) (any, error) {
	l, err := n.l.eval(env)
	if err != nil {
		return nil, err
	}
	r, err := n.r.eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "==":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	}
	if ls, ok := l.(string); ok {
		if rs, ok := r.(string); ok {
			switch n.op {
			case "+":
				return ls + rs, nil
			case "<":
				return ls < rs, nil
			case "<=":
				return ls <= rs, nil
			case ">":
				return ls > rs, nil
			case ">=":
				return ls >= rs, nil
			}
		}
		if n.op == "+" {
			return ls + Format(r), nil
		}
	}
	if rs, ok := r.(string); ok && n.op == "+" {
		return Format(l) + rs, nil
	}
	a, ok1 := ToNumber(l)
	b, ok2 := ToNumber(r)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("%s %s %s: not numbers", describe(l), n.op, describe(r))
	}
	switch n.op {
	case "+":
		return a + b, nil
	case "-":
		return a - b, nil
	case "*":
		return a * b, nil
	case "/":
		if b == 0 {
			return 0.0, nil // a division by zero in a formula is 0, not a crash mid-battle
		}
		return a / b, nil
	case "%":
		bi := int64(b)
		if bi == 0 {
			return 0.0, nil
		}
		m := int64(a) % bi
		if m < 0 {
			m += bi
		}
		return float64(m), nil
	case "<":
		return a < b, nil
	case "<=":
		return a <= b, nil
	case ">":
		return a > b, nil
	case ">=":
		return a >= b, nil
	}
	return nil, fmt.Errorf("unknown operator %s", n.op)
}

func equal(l, r any) bool {
	if l == nil || r == nil {
		return l == nil && r == nil
	}
	if a, ok := ToNumber(l); ok {
		if _, isBool := l.(bool); !isBool {
			if b, ok := ToNumber(r); ok {
				if _, isBool := r.(bool); !isBool {
					return a == b
				}
			}
		}
	}
	switch x := l.(type) {
	case string, bool:
		return l == r
	default:
		rl, rr := reflect.ValueOf(x), reflect.ValueOf(r)
		if rl.Type() != rr.Type() || !rl.Type().Comparable() {
			return false
		}
		return l == r
	}
}

// Format is a value as text (whole numbers without a decimal point).
func Format(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return fmt.Sprintf("%g", x)
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func nums(name string, args []any, n int) ([]float64, error) {
	if n >= 0 && len(args) != n {
		return nil, fmt.Errorf("%s takes %d arguments, got %d", name, n, len(args))
	}
	out := make([]float64, len(args))
	for i, a := range args {
		f, ok := ToNumber(a)
		if !ok {
			return nil, fmt.Errorf("%s: %s is not a number", name, describe(a))
		}
		out[i] = f
	}
	return out, nil
}

func unary(name string, f func(float64) float64) Func {
	return func(args []any) (any, error) {
		x, err := nums(name, args, 1)
		if err != nil {
			return nil, err
		}
		return f(x[0]), nil
	}
}

// builtins are the functions every expression can call.
var builtins = map[string]Func{
	"abs":   unary("abs", math.Abs),
	"floor": unary("floor", math.Floor),
	"ceil":  unary("ceil", math.Ceil),
	"round": unary("round", math.Round),
	"int":   unary("int", math.Trunc),
	"sqrt":  unary("sqrt", math.Sqrt),
	"pow": func(args []any) (any, error) {
		x, err := nums("pow", args, 2)
		if err != nil {
			return nil, err
		}
		return math.Pow(x[0], x[1]), nil
	},
	"min": func(args []any) (any, error) {
		x, err := nums("min", args, -1)
		if err != nil || len(x) == 0 {
			return nil, errors.Join(err, errEmpty(len(x)))
		}
		m := x[0]
		for _, v := range x[1:] {
			m = math.Min(m, v)
		}
		return m, nil
	},
	"max": func(args []any) (any, error) {
		x, err := nums("max", args, -1)
		if err != nil || len(x) == 0 {
			return nil, errors.Join(err, errEmpty(len(x)))
		}
		m := x[0]
		for _, v := range x[1:] {
			m = math.Max(m, v)
		}
		return m, nil
	},
	"clamp": func(args []any) (any, error) {
		x, err := nums("clamp", args, 3)
		if err != nil {
			return nil, err
		}
		return math.Max(x[1], math.Min(x[2], x[0])), nil
	},
	"len": func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("len takes 1 argument")
		}
		if s, ok := args[0].(string); ok {
			return float64(len([]rune(s))), nil
		}
		rv := reflect.ValueOf(args[0])
		if rv.Kind() == reflect.Slice {
			return float64(rv.Len()), nil
		}
		return nil, fmt.Errorf("len of %s", describe(args[0]))
	},
	// has(obj, "name") reports whether an object has a truthy field (has(self.states, "poison")).
	"has": func(args []any) (any, error) {
		if len(args) != 2 {
			return nil, errors.New("has takes 2 arguments")
		}
		o, ok := args[0].(Object)
		name, ok2 := args[1].(string)
		if !ok || !ok2 {
			return false, nil
		}
		v, ok := o.Field(name)
		return ok && Truthy(v), nil
	},
	"str": func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("str takes 1 argument")
		}
		return Format(args[0]), nil
	},
}

func errEmpty(n int) error {
	if n == 0 {
		return errors.New("needs at least one argument")
	}
	return nil
}

// builtin reports whether name is a built-in function.
func builtin(name string) bool { _, ok := builtins[name]; return ok }

// IsBuiltin reports whether name is a built-in function (floor, max, len, ...), which a
// name of the environment hides.
func IsBuiltin(name string) bool { return builtin(name) }
