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

func TestBoundedRangeAndFirst(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   any
	}{
		{`first(filter(range(0, 7), it > 2 && it != 3), -1)`, 4.0},
		{`first(filter(range(0, 7), false), -1)`, -1.0},
		{`sum(range(-2, 3), it)`, 0.0},
		{`len(range(0, 4096))`, 4096.0},
		{`first(range(3, 3), "empty")`, "empty"},
		{`first(rows, nil).n`, 9.0},
		{`first(range(9007199254740990,9007199254740991),0)`, 9007199254740990.0},
		{`first(range(-9007199254740991,-9007199254740990),0)`, -9007199254740991.0},
	} {
		got, err := mustParse(tc.source).Eval(Map{"rows": []any{Map{"n": 9.0}}})
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %v, %v; want %v", tc.source, got, err, tc.want)
		}
	}
	for _, src := range []string{`range(0,4097)`, `range(3,2)`, `range(0,1.5)`, `range("0",2)`, `range(false,2)`, `range(0)`, `range(0,1,2)`, `range(0,huge)`, `range(huge,huge)`, `range(nan,1)`, `range(inf,inf)`, `first(1,0)`, `first(range(0,1))`, `first(large,0)`, `range(9007199254740992,9007199254740992)`} {
		_, err := mustParse(src).Eval(Map{"huge": math.MaxFloat64, "nan": math.NaN(), "inf": math.Inf(1), "large": make([]any, 4097)})
		if err == nil {
			t.Errorf("accepted %s", src)
		}
	}
}
