// Package optional reads values that game state keeps loosely typed (State.Vars,
// state name defaults) where an absent or differently typed value means zero.
package optional

// Value returns v as a T, or T's zero value when v is nil or holds another type.
func Value[T any](v any) T {
	t, ok := v.(T)
	if !ok {
		var zero T
		return zero
	}
	return t
}
