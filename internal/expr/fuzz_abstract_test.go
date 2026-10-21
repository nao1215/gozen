package expr

import "testing"

// FuzzAbstractSound checks the contract of Abstract on arbitrary accepted expressions: a value that Eval actually
// produces (for names inside the ranges) always lies inside the computed range, and Eval never panics.
func FuzzAbstractSound(f *testing.F) {
	for _, src := range []string{"a % b", "a / (b - 1)", "max(a, b) < 3 ? a : -b", "!a || t", "a == s", "clamp(a, b, 3)", "sqrt(a) > 1", "pow(a, b)", "int(a) % 3", "s + a"} {
		f.Add(src)
	}
	look := func(ref Ref) Range {
		switch ref.Name {
		case "a":
			return Between(-3, 6)
		case "b":
			return Number(2).Union(Number(-1)).Union(Number(0))
		case "s":
			return stringRange("x").Union(stringRange(""))
		case "t":
			return Boolean(true, true)
		}
		return Top()
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 2048 {
			return
		}
		e, err := Parse(src)
		if err != nil {
			return
		}
		rg := e.Abstract(look)
		for _, a := range []float64{-3, -0.5, 0, 1, 6} {
			for _, b := range []float64{2, -1, 0} {
				v, err := e.Eval(Map{"a": a, "b": b, "s": "x", "t": true})
				if err != nil {
					continue
				}
				if x, ok := v.(float64); ok && x != x {
					continue
				}
				if !within(rg, v) {
					t.Fatalf("%s (a=%v b=%v) = %#v is outside %s", src, a, b, v, rg)
				}
			}
		}
	})
}
