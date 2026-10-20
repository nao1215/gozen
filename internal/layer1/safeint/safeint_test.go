package safeint

import (
	"math"
	"testing"
)

func TestFrom(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want int
	}{
		{0, 0},
		{2.9, 2},
		{-2.9, -2},
		{math.NaN(), 0},
		{math.Inf(1), Limit},
		{math.Inf(-1), -Limit},
		{1e19, Limit},
		{-1e19, -Limit},
		{Limit - 1, Limit - 1},
	} {
		if got := From(c.in); got != c.want {
			t.Errorf("From(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
