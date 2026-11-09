package expr

import (
	"math"
	"reflect"
	"strconv"
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

func BenchmarkCollectionEval(b *testing.B) {
	values := make([]any, 1024)
	for i := range values {
		values[i] = float64(i)
	}
	env := Map{"rows": values}
	for _, src := range []string{"sum(rows, it)", "count(rows, it > 500)", "filter(rows, it > 500)"} {
		b.Run(src, func(b *testing.B) {
			e := mustParse(src)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := e.Eval(env); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Partitioning the input must not change a sum, and filtering must agree with
// counting the same predicate. Each Eval has its own mutable item and budget.
func TestCollectionPartitionAndConcurrentEvaluation(t *testing.T) {
	t.Parallel()
	sum := mustParse("sum(rows, it)")
	count := mustParse("count(rows, it > 0)")
	filter := mustParse("filter(rows, it > 0)")
	for size := range 33 {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()
			values := make([]any, size)
			want := 0.0
			positive := 0
			for i := range values {
				n := float64(i - size/2)
				values[i] = n
				want += n
				if n > 0 {
					positive++
				}
			}
			eval := func(e *Expr, rows []any) any {
				t.Helper()
				v, err := e.Eval(Map{"rows": rows})
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			for split := 0; split <= size; split++ {
				a, _ := ToNumber(eval(sum, values[:split]))
				b, _ := ToNumber(eval(sum, values[split:]))
				if a+b != want {
					t.Fatalf("split %d: sum = %v, want %v", split, a+b, want)
				}
			}
			if n := eval(count, values); n != float64(positive) {
				t.Fatalf("count = %v, want %d", n, positive)
			}
			if rows, ok := eval(filter, values).([]any); !ok || len(rows) != positive {
				t.Fatalf("filtered length = %d, want %d", len(rows), positive)
			}
		})
	}
}
