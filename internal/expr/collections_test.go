package expr

import (
	"math"
	"reflect"
	"testing"
)

func TestCollections(t *testing.T) {
	env := Map{"rows": []any{Map{"n": 2.0}, Map{"n": 3.0}}, "empty": []any{}, "large": make([]any, 4097), "many": make([]any, 257), "limit": make([]any, 4096), "overflow": []any{math.MaxFloat64, math.MaxFloat64}, "nan": []any{math.NaN()}}
	for _, tc := range []struct {
		src  string
		want any
	}{
		{`sum(rows, it.n)`, 5.0},
		{`sum(limit, 1)`, 4096.0},
		{`filter(rows, true)[0].n`, 2.0},
		{`count(rows, it.n > 2)`, 1.0},
		{`sum(filter(rows, it.n > 2), it.n)`, 3.0},
		{`sum(rows, sum(rows, it.n) + it.n)`, 15.0},
		{`sum(empty, it.missing)`, 0.0},
		{`count(empty, true)`, 0.0},
		{`filter(empty, true)`, []any{}},
	} {
		e, err := Parse(tc.src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.Eval(env)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %v, %v; want %v", tc.src, got, err, tc.want)
		}
	}
	for _, src := range []string{`sum(large, 1)`, `sum(overflow, it)`, `sum(nan, it)`, `sum(many, sum(many, 1))`, `sum(rows, "x")`, `count(rows, 1)`, `filter(rows, nil)`, `sum(1, it)`, `sum(rows, it.missing)`, `sum(rows)`, `sum(rows, it.n, 1)`, `it.n`} {
		e, err := Parse(src)
		if err == nil {
			_, err = e.Eval(env)
		}
		if err == nil {
			t.Errorf("accepted %s", src)
		}
	}
	e, err := Parse(`sum(filter(rows, it.n > threshold), it.n * factor)`)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Names(); !reflect.DeepEqual(got, []string{"rows", "threshold", "factor"}) {
		t.Fatalf("names: %v", got)
	}
	if got := e.ResultType(func(Ref) ValueType { return UnknownType }); got != NumberType {
		t.Fatalf("type: %v", got)
	}
}
