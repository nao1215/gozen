// Package safeint converts numbers from game data expressions to Go integers without
// relying on Go's implementation-specific conversion of NaN, infinities or values
// outside the int range.
package safeint

import "math"

// Limit is the largest magnitude From returns: 2^53, the last integer a float64 holds
// exactly.
const Limit = 1 << 53

// From truncates f toward zero and saturates it to [-Limit, Limit]. NaN is 0.
func From(f float64) int {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= Limit:
		return Limit
	case f <= -Limit:
		return -Limit
	}
	return int(f)
}
