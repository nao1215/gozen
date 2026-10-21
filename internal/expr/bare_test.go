package expr

import (
	"slices"
	"testing"
)

// TestBare: the names written as a whole value are listed, built-in functions included; a
// called name and the start of a field or an index are not.
func TestBare(t *testing.T) {
	for src, want := range map[string][]string{
		"floor + 1":                    {"floor"},
		"floor(gold / 2)":              {"gold"},
		"rand(3) > 1":                  nil,
		"rand > 1 ? rand : b":          {"rand", "b"},
		"party.members[i].hp + max":    {"i", "max"},
		"has(self.states, \"poison\")": nil,
		"-abs":                         {"abs"},
		"(min)":                        {"min"},
		"!chance && x[floor]":          {"chance", "floor"},
		"str(len)":                     {"len"},
	} {
		e, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.Bare(); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", src, got, want)
		}
	}
	if (*Expr)(nil).Bare() != nil {
		t.Error("a nil expression has bare names")
	}
	if !IsBuiltin("floor") || IsBuiltin("rand") || IsBuiltin("gold") {
		t.Error("IsBuiltin is wrong")
	}
}
