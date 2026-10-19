package rpg_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/nao1215/gozen/rpg"
)

// rand() never panics on extreme arguments: NaN is an error, huge counts are capped
// and results stay in range.
func TestRandFuncExtremeArguments(t *testing.T) {
	f := rpg.RandFunc(rand.New(rand.NewPCG(1, 2))) //nolint:gosec // Gameplay randomness under test; no security-sensitive values.
	for _, args := range [][]any{{math.NaN()}, {1.0, math.NaN()}} {
		if _, err := f(args); err == nil {
			t.Errorf("rand(%v) accepted NaN", args)
		}
	}
	for _, c := range []struct {
		args   []any
		lo, hi float64
	}{
		{[]any{1e19}, 0, 1 << 53},
		{[]any{math.Inf(1)}, 0, 1 << 53},
		{[]any{math.Inf(-1)}, 0, 0},
		{[]any{-1e19, 1e19}, -1e19, 1e19},
		{[]any{3.0, 3.0}, 3, 3},
		{[]any{5.0, 1.0}, 5, 5},
	} {
		got, err := f(c.args)
		if err != nil {
			t.Fatalf("rand(%v): %v", c.args, err)
		}
		v, ok := got.(float64)
		if !ok || v < c.lo || v > c.hi {
			t.Errorf("rand(%v) = %v, want within [%v, %v]", c.args, got, c.lo, c.hi)
		}
	}
}
