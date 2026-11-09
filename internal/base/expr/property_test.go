package expr

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// genExpr builds a random expression over the names a, b (numbers), s (string) and t (boolean).
func genExpr(r *rand.Rand, depth int) string {
	if depth <= 0 || r.Intn(5) == 0 {
		switch r.Intn(8) {
		case 0:
			return "a"
		case 1:
			return "b"
		case 2:
			return "t"
		case 3:
			return "s"
		case 4:
			return strconv.Itoa(r.Intn(9) - 3)
		case 5:
			return fmt.Sprintf("%d.5", r.Intn(4))
		case 6:
			return []string{"true", "false", "nil", "'x'", "''", "'scout'"}[r.Intn(6)]
		}
		return "a"
	}
	sub := func() string { return genExpr(r, depth-1) }
	switch r.Intn(12) {
	case 0:
		return "(" + sub() + " " + []string{"+", "-", "*", "/", "%"}[r.Intn(5)] + " " + sub() + ")"
	case 1:
		return "(" + sub() + " " + []string{"<", "<=", ">", ">="}[r.Intn(4)] + " " + sub() + ")"
	case 2:
		return "(" + sub() + " " + []string{"==", "!="}[r.Intn(2)] + " " + sub() + ")"
	case 3:
		return "(" + sub() + " " + []string{"&&", "||"}[r.Intn(2)] + " " + sub() + ")"
	case 4:
		return "!" + sub()
	case 5:
		return "-" + sub()
	case 6:
		return "(" + sub() + " ? " + sub() + " : " + sub() + ")"
	case 7:
		return []string{"abs", "floor", "ceil", "round", "int"}[r.Intn(5)] + "(" + sub() + ")"
	case 8:
		return []string{"min", "max"}[r.Intn(2)] + "(" + sub() + ", " + sub() + ")"
	case 9:
		return "clamp(" + sub() + ", " + sub() + ", " + sub() + ")"
	case 10:
		return []string{"sqrt", "str", "len"}[r.Intn(3)] + "(" + sub() + ")"
	}
	return "pow(" + sub() + ", 2)"
}

// TestAbstractSoundRandom: for random expressions, every concrete evaluation whose names lie in the given
// ranges falls in the computed range (the contract of Abstract: a condition is called impossible only when it truly is).
func TestAbstractSoundRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1)) //nolint:gosec // A fixed seed for a reproducible test.
	look := func(ref Ref) Range {
		switch ref.Name {
		case "a":
			return Between(-3, 6)
		case "b":
			return Number(2).Union(Number(-1)).Union(Number(0))
		case "s":
			return stringRange("scout").Union(stringRange(""))
		case "t":
			return Boolean(true, true)
		}
		return Top()
	}
	avals := []float64{-3, -2.5, -1, 0, 0.5, 1, 2, 3.5, 6}
	bvals := []float64{2, -1, 0}
	svals := []string{"scout", ""}
	checked := 0
	for range 4000 {
		src := genExpr(r, 4)
		e, err := Parse(src)
		if err != nil {
			t.Fatalf("generated expression %q does not parse: %v", src, err)
		}
		rg := e.Abstract(look)
		for _, a := range avals {
			for _, b := range bvals {
				for _, s := range svals {
					for _, tv := range []bool{false, true} {
						v, err := e.Eval(Map{"a": a, "b": b, "s": s, "t": tv})
						if err != nil {
							continue // type errors such as "x" - 1 are reported by Eval, not by Abstract
						}
						if f, ok := v.(float64); ok && math.IsNaN(f) {
							continue
						}
						if !within(rg, v) {
							t.Fatalf("%s (a=%v b=%v s=%q t=%v) = %#v is outside the range %s", src, a, b, s, tv, v, rg)
						}
						checked++
					}
				}
			}
		}
	}
	if checked < 10000 {
		t.Fatalf("only %d evaluations succeeded; the generator is broken", checked)
	}
}

// TestParseRoundTripRandom: the source of any accepted expression parses again to the same source,
// and evaluation is deterministic (the same environment gives the same value or the same error).
func TestParseRoundTripRandom(t *testing.T) {
	r := rand.New(rand.NewSource(2)) //nolint:gosec // A fixed seed for a reproducible test.
	for range 1000 {
		src := genExpr(r, 4)
		e := mustParse(src)
		text, err := e.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var back Expr
		if err := back.UnmarshalText(text); err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if back.String() != src {
			t.Fatalf("round trip changed %q into %q", src, back.String())
		}
		env := Map{"a": 1.5, "b": 2.0, "s": "scout", "t": true}
		v1, err1 := e.Eval(env)
		v2, err2 := back.Eval(env)
		if (err1 == nil) != (err2 == nil) || (err1 == nil && fmt.Sprint(v1) != fmt.Sprint(v2)) {
			t.Fatalf("%q evaluates differently after a round trip: %v/%v vs %v/%v", src, v1, err1, v2, err2)
		}
	}
}

func TestRangeEqualAndString(t *testing.T) {
	cases := []struct {
		r    Range
		want string
	}{
		{Top(), "?"},
		{Range{}, "{}"},
		{Number(3), "3"},
		{Between(1, 4), "[1, 4]"},
		{Between(math.Inf(-1), math.Inf(1)), "[-inf, inf]"},
		{Number(1).Union(Number(5)), "{1, 5}"},
		{Boolean(true, true), "false | true"},
		{Of(nil), "nil"},
		{Of("a"), `"a"`},
		{Range{StrAny: true}, "'*'"},
		{Of(2).Union(Of("a")).Union(Of(true)), `2 | true | "a"`},
	}
	for _, c := range cases {
		if got := c.r.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
		same := c.r
		if !c.r.Equal(same) {
			t.Errorf("%s is not equal to itself", c.want)
		}
	}
	if Top().Equal(Number(1)) || Number(1).Equal(Top()) || !Top().Equal(Top()) {
		t.Error("Top equals only Top")
	}
	if Between(1, 2).Equal(Between(1, 3)) || Number(1).Equal(Of("1")) || Boolean(true, false).Equal(Boolean(false, true)) {
		t.Error("different ranges compare equal")
	}
	if Of(1).Equal(Of(nil)) || Of("a").Equal(Of("b")) {
		t.Error("different values compare equal")
	}
	if got := Of(struct{}{}); !got.Any {
		t.Errorf("Of(unknown type) = %s, want Top", got)
	}
	if got := Of(int64(4)); !got.Equal(Number(4)) {
		t.Errorf("Of(int64) = %s", got)
	}
}

func TestBetweenSwapsAndCollapses(t *testing.T) {
	if got := Between(5, 1); !got.Equal(Between(1, 5)) {
		t.Errorf("Between(5, 1) = %s", got)
	}
	if got := Between(2, 2); !got.Equal(Number(2)) {
		t.Errorf("Between(2, 2) = %s", got)
	}
	vs := make([]float64, 0, 100)
	for i := range 100 {
		vs = append(vs, float64(i))
	}
	big := numbers(vs)
	if big.Set != nil || big.Lo != 0 || big.Hi != 99 {
		t.Errorf("more than %d numbers must fall back to the interval, got %+v", maxSet, big)
	}
	if got := numbers(nil); got.Num || got.Any {
		t.Errorf("no numbers must be the empty range, got %+v", got)
	}
}

func TestUnionStringsOverflowBecomeAny(t *testing.T) {
	var out Range
	for i := range maxStrs + 5 {
		out = out.Union(Of(fmt.Sprint("s", i)))
	}
	if !out.StrAny || out.Strs != nil {
		t.Errorf("more than %d strings must become StrAny, got %d strings, StrAny=%v", maxStrs, len(out.Strs), out.StrAny)
	}
	if !out.CanBeTrue() || !out.CanBeFalse() {
		t.Error("any string can be truthy and falsy")
	}
}

func TestTruthinessOfRanges(t *testing.T) {
	cases := []struct {
		r             Range
		canTrue, canF bool
	}{
		{Number(0), false, true},
		{Number(1), true, false},
		{Between(0, 0), false, true},
		{Between(-1, 1), true, true},
		{Between(1, 2), true, false},
		{Number(0).Union(Number(3)), true, true},
		{Of(""), false, true},
		{Of("a"), true, false},
		{Of(nil), false, true},
		{Boolean(false, true), true, false},
		{Boolean(true, false), false, true},
		{Top(), true, true},
		{Range{}, false, false},
	}
	for _, c := range cases {
		if c.r.CanBeTrue() != c.canTrue || c.r.CanBeFalse() != c.canF {
			t.Errorf("%s: CanBeTrue=%v CanBeFalse=%v, want %v %v", c.r, c.r.CanBeTrue(), c.r.CanBeFalse(), c.canTrue, c.canF)
		}
	}
}

func TestAbstractBuiltinsAndOperators(t *testing.T) {
	look := func(ref Ref) Range {
		switch ref.Name {
		case "n":
			return Between(-4, 9)
		case "z":
			return Between(-2, 2)
		case "pos":
			return Between(3, 4)
		case "str":
			return Range{StrAny: true}
		}
		return Top()
	}
	cases := []struct {
		src  string
		want string
	}{
		{"-n", "[-9, 4]"},
		{"-str", "?"},
		{"abs(n)", "[0, 9]"},
		{"abs(pos)", "[3, 4]"},
		{"abs(-pos)", "[3, 4]"},
		{"floor(n)", "[-4, 9]"},
		{"clamp(n, 0, 5)", "[0, 5]"},
		{"min(n, 3)", "[-4, 3]"},
		{"max(n, pos)", "[3, 9]"},
		{"pos * pos", "[9, 16]"},
		{"n * z", "[-18, 18]"},
		{"pos / pos", "[0.75, 1.3333333333333333]"},
		{"n / z", "[-inf, inf]"},
		{"n % 5", "[0, 4]"},
		{"n % 0", "0"},
		{"pos % 10", "[3, 4]"},
		{"n % z", "[-inf, inf]"},
		{"n < 100", "true"},
		{"n < 5", "false | true"},
		{"str < 'a'", "false | true"},
		{"str - 1", "?"},
		{"rand()", "[0, 1]"},
		{"rand(10)", "[0, 9]"},
		{"rand(3, 8)", "[3, 8]"},
		{"rand(str)", "[-inf, inf]"},
		{"chance(0.1)", "false | true"},
		{"sqrt(n)", "[-inf, inf]"},
		{"len('abc')", "[-inf, inf]"},
		{"has(self, 'x')", "false | true"},
		{"str(n)", "'*'"},
		{"mystery(n)", "?"},
		{"n['k'](1)", "?"},
		{"1 + 2", "3"},
		{"'a' + 'b'", "?"},
		{"true == true", "true"},
		{"nil == nil", "true"},
		{"'a' != 'b'", "true"},
		{"str == 'a'", "false | true"},
		{"'a' == str", "false | true"},
		{"n == 100", "false"},
		{"z == 0", "false | true"},
		{"z != 0", "false | true"},
		{"true ? 1 : 2", "1"},
		{"false ? 1 : 2", "2"},
		{"n > 0 ? 1 : 2", "{1, 2}"},
		{"nil || 1", "true"},
		{"0 || n", "false | true"},
		{"1 && n", "false | true"},
		{"0 && n", "false"},
	}
	for _, c := range cases {
		got := mustParse(c.src).Abstract(look).String()
		if got != c.want {
			t.Errorf("Abstract(%s) = %s, want %s", c.src, got, c.want)
		}
	}
	var nilExpr *Expr
	if !nilExpr.Abstract(look).Any {
		t.Error("a nil expression has no known range")
	}
}

func TestAbstractRemainderMatchesEval(t *testing.T) {
	look := func(ref Ref) Range {
		if ref.Name == "n" {
			return Between(-9, 9)
		}
		return Top()
	}
	for _, d := range []string{"1", "2", "3", "7", "-3", "0", "2.5"} {
		src := "n % " + d
		e := mustParse(src)
		rg := e.Abstract(look)
		for n := -9.0; n <= 9; n += 0.5 {
			v, err := e.Eval(Map{"n": n})
			if err != nil {
				t.Fatal(err)
			}
			if !within(rg, v) {
				t.Errorf("%s with n=%v = %v is outside %s", src, n, v, rg)
			}
		}
	}
}

func TestEvalErrorsAndEdges(t *testing.T) {
	env := Map{"obj": Map{"k": 1.0}, "list": []any{1.0, 2.0}, "s": "x", "n": 3.0, "f": Func(func([]any) (any, error) { return nil, errors.New("boom") })}
	errCases := []struct{ src, want string }{
		{"missing", "unknown name"},
		{"obj.nope", "unknown field"},
		{"n.x", "no field"},
		{"nil.x", "of nil"},
		{"list['a']", "not a number"},
		{"s[0]", "cannot be indexed"},
		{"n[0]", "cannot be indexed"},
		{"n()", "not a function"},
		{"f()", "boom"},
		{"-s", "cannot negate"},
		{"s - 1", "not numbers"},
		{"[]", ""},
		{"abs()", "takes 1 arguments"},
		{"abs('a')", "not a number"},
		{"min()", "needs at least one"},
		{"max('a')", "not a number"},
		{"clamp(1)", "takes 3"},
		{"pow(1)", "takes 2"},
		{"len()", "takes 1"},
		{"len(n)", "len of"},
		{"has(obj)", "takes 2"},
		{"str()", "takes 1"},
		{"missing + 1", "unknown name"},
		{"1 + missing", "unknown name"},
		{"missing && 1", "unknown name"},
		{"1 && missing", "unknown name"},
		{"missing ? 1 : 2", "unknown name"},
		{"!missing", "unknown name"},
		{"-missing", "unknown name"},
		{"missing.x", "unknown name"},
		{"missing[0]", "unknown name"},
		{"list[missing]", "unknown name"},
		{"f(missing)", "unknown name"},
		{"missing(1)", "unknown name"},
	}
	for _, c := range errCases {
		e, err := Parse(c.src)
		if err != nil {
			if c.want == "" {
				continue
			}
			t.Errorf("Parse(%q): %v", c.src, err)
			continue
		}
		_, err = e.Eval(env)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("Eval(%q) error = %v, want it to contain %q", c.src, err, c.want)
		}
	}
	okCases := []struct {
		src  string
		want any
	}{
		{"obj['k']", 1.0},
		{"obj['zz']", nil},
		{"list[-1]", nil},
		{"list[1]", 2.0},
		{"list[1.9]", 2.0},
		{"len('ミアあ')", 3.0},
		{"len(list)", 2.0},
		{"str(2.5)", "2.5"},
		{"str(nil)", ""},
		{"s + 1", "x1"},
		{"1 + s", "1x"},
		{"s + true", "xtrue"},
		{"'a' <= 'a' && 'b' >= 'a' && 'b' > 'a'", true},
		{"5 % 0", 0.0},
		{"-7 % 3", 2.0},
		{"7.9 % 3", 1.0},
		{"1 / 4", 0.25},
		{"true + 1", 2.0},
		{"nil + 1", 1.0},
		{"2 == 2.0", true},
		{"'1' == 1", false},
		{"obj == list", false},
		{"pow(2, 10)", 1024.0},
		{"sqrt(16)", 4.0},
		{"round(2.5)", 3.0},
		{"int(-2.7)", -2.0},
		{"ceil(-0.5)", math.Copysign(0, -1)},
	}
	for _, c := range okCases {
		e, err := Parse(c.src)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.src, err)
			continue
		}
		got, err := e.Eval(env)
		if err != nil {
			t.Errorf("Eval(%q): %v", c.src, err)
			continue
		}
		if fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", c.want) {
			t.Errorf("Eval(%q) = %#v, want %#v", c.src, got, c.want)
		}
	}
}

func TestNilExpressionContracts(t *testing.T) {
	var e *Expr
	if _, err := e.Eval(nil); err == nil {
		t.Error("evaluating a nil expression must fail")
	}
	if b, err := e.Bool(nil); err != nil || !b {
		t.Errorf("a nil condition always holds, got %v %v", b, err)
	}
	if n, err := e.Number(nil); err != nil || n != 0 {
		t.Errorf("a nil number is 0, got %v %v", n, err)
	}
	if e.String() != "" || e.Names() != nil || e.Refs() != nil {
		t.Error("a nil expression has no source, names or refs")
	}
	if got := e.ResultType(nil); got != NilType {
		t.Errorf("ResultType of nil = %v", got)
	}
	if _, err := mustParse("'a'").Number(nil); err == nil {
		t.Error("a string is not a number")
	}
	if _, err := mustParse("nope").Bool(nil); err == nil {
		t.Error("an unknown name must fail Bool")
	}
	if _, err := mustParse("nope").Number(nil); err == nil {
		t.Error("an unknown name must fail Number")
	}
	var bad Expr
	if err := bad.UnmarshalText([]byte("1 +")); err == nil {
		t.Error("UnmarshalText must reject a broken expression")
	}
}

func TestChainLookup(t *testing.T) {
	c := Chain{nil, Map{"a": 1.0}, Map{"a": 2.0, "b": 3.0}}
	if v, ok := c.Lookup("a"); !ok || v != 1.0 {
		t.Errorf("first env wins, got %v %v", v, ok)
	}
	if v, ok := c.Lookup("b"); !ok || v != 3.0 {
		t.Errorf("later env found, got %v %v", v, ok)
	}
	if _, ok := c.Lookup("c"); ok {
		t.Error("unknown name must not be found")
	}
}

func TestNamesAndRefsOrder(t *testing.T) {
	e := mustParse("party.members[0].hp + x + party.gold + max(y, x['k']) + abs(1)")
	if got := e.Names(); !slices.Equal(got, []string{"party", "x", "y"}) {
		t.Errorf("Names = %v", got)
	}
	refs := e.Refs()
	if len(refs) != 5 || refs[0].Name != "party" || len(refs[0].Path) != 3 || !refs[0].Path[1].Index {
		t.Errorf("Refs = %+v", refs)
	}
}

// TestNaNIsNotAnIntervalEnd: sqrt(-1) is NaN at run time; a range holding NaN as an end would make every
// comparison on it false and wrongly call reachable values impossible.
func TestNaNIsNotAnIntervalEnd(t *testing.T) {
	e := mustParse("max(1, sqrt(-1)) >= 0 || (sqrt(-1) < 0 ? 0 : 5) >= 0")
	rg := e.Abstract(func(Ref) Range { return Top() })
	if !rg.CanBeTrue() {
		t.Errorf("expression can hold, got range %s", rg)
	}
	if !Number(math.NaN()).Equal(Between(math.Inf(-1), math.Inf(1))) || !Between(math.NaN(), 1).Num {
		t.Error("NaN must widen to any number")
	}
	u := Number(math.NaN()).Union(Number(0))
	if !within(u, 0.0) {
		t.Errorf("0 must be in %s", u)
	}
}

func TestParseRejectsBrokenInput(t *testing.T) {
	for _, src := range []string{"", "1 +", "(", ")", "'open", "1 2", "a..b", "a[", "a[]", "f(,)", "?", "1 ? 2", "&& 1", "1 = 1", "@", "a.", "a.1", "\"open", "1 +* 2", strings.Repeat("(", 5000) + "1" + strings.Repeat(")", 5000), strings.Repeat("-", 5000) + "1", strings.Repeat("!", 5000) + "1"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%.20q) must fail", src)
		}
	}
}
