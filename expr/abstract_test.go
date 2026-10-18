package expr

import (
	"math"
	"testing"
)

// TestAbstract: 名前の幅の下で、式が成り立ちうるか・必ず成り立つかを正しく言う。
// 幅から選んだ値で実際に評価した結果が、いつも求めた幅に入ることも確かめる（抽象解釈が健全か）。
func TestAbstract(t *testing.T) {
	look := func(r Ref) Range {
		switch {
		case r.Name == "floor" && len(r.Path) == 0:
			return Between(1, 5)
		case r.Name == "var" && len(r.Path) == 1 && r.Path[0].Name == "floor":
			return Between(0, 5)
		case r.Name == "var" && len(r.Path) == 1 && r.Path[0].Name == "who":
			return String("scout").Union(String("guide")).Union(Number(0))
		case r.Name == "flag" && len(r.Path) == 1 && r.Path[0].Name == "never":
			return Boolean(true, false)
		case r.Name == "flag" && len(r.Path) == 1 && r.Path[0].Name == "maybe":
			return Boolean(true, true)
		}
		return Top()
	}
	cases := []struct {
		src       string
		can, must bool // 成り立ちうるか、必ず成り立つか
	}{
		{"var.floor >= 6", false, false},
		{"var.floor <= 5", true, true},
		{"var.floor >= 6 && var.floor <= 10", false, false},
		{"var.floor % 5 == 0", true, false},
		{"var.floor == 8 && !flag.x", false, false},
		{"var.floor >= 21 || (var.floor >= 11 && rand() < 0.35)", false, false},
		{"floor >= 4 && floor <= 10 ? 1 : 0", true, false},
		{"floor >= 6 ? 1 : 0", false, false},
		{"flag.never", false, false},
		{"!flag.never", true, true},
		{"flag.maybe", true, false},
		{"flag.never && var.unknown", false, false},
		{"var.unknown || !flag.never", true, true},
		{"var.who == 'scout'", true, false},
		{"var.who == 'smith'", false, false},
		{"var.who != 'smith'", true, true},
		{"var.unknown >= 100", true, false},
		{"(var.floor + 1) > 5", true, false},
		{"(var.floor - 1) > 5", false, false},
		{"max(var.floor, 3) >= 3", true, true},
		{"min(var.floor, 3) > 3", false, false},
		{"var.floor * 10 + 1 > 51", false, false},
		{"rand(3) == 3", false, false},
		{"chance(0.5)", true, false},
		{"flag.never == true", false, false},
		{"var.floor == true", false, false}, // 数と真偽は等しくならない
	}
	for _, c := range cases {
		e := mustParse(c.src)
		r := e.Abstract(look)
		if r.CanBeTrue() != c.can || !r.CanBeFalse() != c.must {
			t.Errorf("%s: 幅 %s（成り立ちうる %v・必ず %v）、want %v・%v", c.src, r, r.CanBeTrue(), !r.CanBeFalse(), c.can, c.must)
		}
	}
}

// TestAbstractSound: 幅の中の値で実際に評価した値は、いつも求めた幅に入る。
func TestAbstractSound(t *testing.T) {
	srcs := []string{
		"var.a + var.b * 2", "var.a % 5", "var.a % 5 == 0", "var.a / (var.b + 1)", "max(var.a, var.b) - min(var.a, 2)",
		"var.a > 2 ? var.a : -var.b", "abs(var.a - 3)", "floor(var.a / 2)", "clamp(var.a, 1, 3)", "var.a >= 2 && var.b < 1",
		"var.a == var.b", "!(var.a < var.b) || var.b == 0", "var.a <= 5 ? 1 : var.a <= 10 ? 2 : 3",
	}
	for _, src := range srcs {
		e := mustParse(src)
		r := e.Abstract(func(ref Ref) Range {
			if ref.Name == "var" && len(ref.Path) == 1 {
				return Between(-2, 7)
			}
			return Top()
		})
		for a := -2.0; a <= 7; a++ {
			for b := -2.0; b <= 7; b++ {
				v, err := e.Eval(Map{"var": Map{"a": a, "b": b}})
				if err != nil {
					t.Fatal(src, err)
				}
				if !within(r, v) {
					t.Errorf("%s（a=%v b=%v）= %v が幅 %s の外", src, a, b, v, r)
				}
			}
		}
	}
}

func within(r Range, v any) bool {
	if r.Any {
		return true
	}
	switch x := v.(type) {
	case bool:
		return (x && r.True) || (!x && r.False)
	case float64:
		return r.Num && x >= r.Lo && x <= r.Hi && !math.IsNaN(x)
	case string:
		if r.StrAny {
			return true
		}
		for _, s := range r.Strs {
			if s == x {
				return true
			}
		}
	case nil:
		return r.Nil
	}
	return false
}
