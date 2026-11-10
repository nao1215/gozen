package expr

import (
	"math"
	"testing"
)

// TestRemainderIsNeverNegative: the remainder lies in [0, |divisor|) whichever signs
// the operands have, and a divisor of 0 gives 0.
func TestRemainderIsNeverNegative(t *testing.T) {
	for src, want := range map[string]float64{
		"7 % 3":             1,
		"(0 - 7) % 3":       2,
		"7 % (0 - 3)":       1,
		"(0 - 7) % (0 - 3)": 2,
		"(0 - 1) % (0 - 5)": 4,
		"(0 - 6) % (0 - 3)": 0,
		"7 % 0":             0,
	} {
		got, err := mustParse(src).Number(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
}

// TestIndexThatIsNotAnIntIsTheSameEverywhere: converting NaN or a number beyond an
// int to an int follows the platform (the lowest int on amd64, 0 or the highest on
// arm64 and wasm), so NaN is an error and a huge index is out of range.
func TestIndexThatIsNotAnIntIsTheSameEverywhere(t *testing.T) {
	env := Map{"xs": []any{"first"}, "nan": math.NaN(), "huge": 1e300, "low": -1e300}
	if v, err := mustParse("xs[nan]").Eval(env); err == nil {
		t.Errorf("xs[NaN] = %v, want an error", v)
	}
	for _, src := range []string{"xs[huge]", "xs[low]"} {
		if v, err := mustParse(src).Eval(env); err != nil || v != nil {
			t.Errorf("%s = %v, %v; want nil (out of range)", src, v, err)
		}
	}
	if v, err := mustParse("xs[0.9]").Eval(env); err != nil || v != "first" {
		t.Errorf("xs[0.9] = %v, %v", v, err)
	}
}
