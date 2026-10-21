package expr

// Abstract interpretation of expressions: given names as ranges of possible values, compute the range of values an expression can take.
// Used when assembling a trial build (distribution-scope check) to find conditions that can never hold under variable ranges (e.g. floor is 1 to 5).
// Anything unknown falls back to "could be anything" (Top), so a condition is reported as impossible only when it truly cannot hold.

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// maxStrs is the upper bound on the number of strings a Range remembers (beyond it, any string is possible).
// maxSet is the upper bound on remembered numbers (beyond it, only the interval is kept). maxPairs is the upper bound on pairs computed one by one.
const (
	maxStrs  = 64
	maxSet   = 64
	maxPairs = 256
)

// Range is the range of values a value can take: a sum of numbers (interval Lo to Hi), booleans and strings. Any means anything is possible.
type Range struct {
	Any bool // anything is possible (unknown)

	Num    bool      // a number is possible
	Lo, Hi float64   // the interval when Num is set (both ends inclusive; +/-Inf allowed)
	Set    []float64 // when Num is set, the number is exactly one of these (sorted, no duplicates; nil means the whole interval)

	False, True bool // whether the boolean false / true is possible

	Strs   []string // the possible strings (sorted, no duplicates)
	StrAny bool     // any string is possible
	Nil    bool     // nil is possible
}

// Top is the value that can be anything.
func Top() Range { return Range{Any: true} }

// Number is exactly the number v. NaN has no place in an interval (every comparison with it is false), so it is any number.
func Number(v float64) Range {
	if math.IsNaN(v) {
		return anyNumber()
	}
	return Range{Num: true, Lo: v, Hi: v, Set: []float64{v}}
}

// Between is the number interval lo to hi.
func Between(lo, hi float64) Range {
	if math.IsNaN(lo) || math.IsNaN(hi) {
		return anyNumber()
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		return Number(lo)
	}
	return Range{Num: true, Lo: lo, Hi: hi}
}

// numbers is any one of the numbers (nothing at all when vs is empty).
func numbers(vs []float64) Range {
	if len(vs) == 0 {
		return Range{}
	}
	vs = slices.Clone(vs)
	slices.Sort(vs)
	vs = slices.Compact(vs)
	r := Range{Num: true, Lo: vs[0], Hi: vs[len(vs)-1], Set: vs}
	if len(vs) > maxSet {
		r.Set = nil
	}
	return r
}

// Boolean is a boolean (f and t say whether false and true are possible).
func Boolean(f, t bool) Range { return Range{False: f, True: t} }

// stringRange is the singleton string range used by static-analysis clients.
func stringRange(s string) Range { return Range{Strs: []string{s}} }

// Of is exactly the value v (number, boolean, string or nil; anything else is Top).
func Of(v any) Range {
	switch x := v.(type) {
	case nil:
		return Range{Nil: true}
	case bool:
		return Boolean(!x, x)
	case string:
		return stringRange(x)
	}
	if f, ok := ToNumber(v); ok {
		return Number(f)
	}
	return Top()
}

// Union is a value that is either r or o.
func (r Range) Union(o Range) Range {
	if r.Any || o.Any {
		return Top()
	}
	out := Range{False: r.False || o.False, True: r.True || o.True, Nil: r.Nil || o.Nil, StrAny: r.StrAny || o.StrAny}
	switch {
	case r.Num && o.Num:
		if r.Set != nil && o.Set != nil {
			n := numbers(append(slices.Clone(r.Set), o.Set...))
			out.Num, out.Lo, out.Hi, out.Set = true, n.Lo, n.Hi, n.Set
		} else {
			out.Num, out.Lo, out.Hi = true, math.Min(r.Lo, o.Lo), math.Max(r.Hi, o.Hi)
		}
	case r.Num:
		out.Num, out.Lo, out.Hi, out.Set = true, r.Lo, r.Hi, slices.Clone(r.Set)
	case o.Num:
		out.Num, out.Lo, out.Hi, out.Set = true, o.Lo, o.Hi, slices.Clone(o.Set)
	}
	if !out.StrAny {
		out.Strs = append(slices.Clone(r.Strs), o.Strs...)
		slices.Sort(out.Strs)
		out.Strs = slices.Compact(out.Strs)
		if len(out.Strs) > maxStrs {
			out.Strs, out.StrAny = nil, true
		}
	}
	return out
}

// Equal reports whether r and o are the same range of values.
func (r Range) Equal(o Range) bool {
	if r.Any || o.Any {
		return r.Any == o.Any
	}
	if r.Num != o.Num || (r.Num && (r.Lo != o.Lo || r.Hi != o.Hi || !slices.Equal(r.Set, o.Set))) {
		return false
	}
	return r.False == o.False && r.True == o.True && r.Nil == o.Nil && r.StrAny == o.StrAny && slices.Equal(r.Strs, o.Strs)
}

// String formats a range for diagnostics without evaluating expressions.
func (r Range) String() string {
	if r.Any {
		return "?"
	}
	var parts []string
	if r.Num {
		switch {
		case r.Lo == r.Hi:
			parts = append(parts, Format(r.Lo))
		case r.Set != nil:
			var vs []string
			for _, v := range r.Set {
				vs = append(vs, Format(v))
			}
			parts = append(parts, "{"+strings.Join(vs, ", ")+"}")
		default:
			parts = append(parts, "["+fmtBound(r.Lo)+", "+fmtBound(r.Hi)+"]")
		}
	}
	if r.False {
		parts = append(parts, "false")
	}
	if r.True {
		parts = append(parts, "true")
	}
	if r.Nil {
		parts = append(parts, "nil")
	}
	if r.StrAny {
		parts = append(parts, "'*'")
	}
	for _, s := range r.Strs {
		parts = append(parts, strconv.Quote(s))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return strings.Join(parts, " | ")
}

func fmtBound(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return Format(f)
}

// CanBeTrue reports whether the value can be truthy (Truthy).
func (r Range) CanBeTrue() bool {
	if r.Any || r.True || r.StrAny {
		return true
	}
	if r.Num && (r.Lo != 0 || r.Hi != 0) {
		return r.Set == nil || slices.ContainsFunc(r.Set, func(v float64) bool { return v != 0 })
	}
	for _, s := range r.Strs {
		if s != "" {
			return true
		}
	}
	return false
}

// CanBeFalse reports whether the value can be falsy.
func (r Range) CanBeFalse() bool {
	if r.Any || r.False || r.Nil || r.StrAny {
		return true
	}
	if r.Num && r.Lo <= 0 && r.Hi >= 0 && (r.Set == nil || slices.Contains(r.Set, 0)) {
		return true
	}
	return slices.Contains(r.Strs, "")
}

// truth is the truthiness range of the value (a boolean Range).
func (r Range) truth() Range { return Boolean(r.CanBeFalse(), r.CanBeTrue()) }

// numeric is the interval when used as a number (booleans are 0 and 1, nil is 0). ok is false if a string or anything is possible.
func (r Range) numeric() (lo, hi float64, ok bool) {
	if r.Any || r.StrAny || len(r.Strs) > 0 {
		return 0, 0, false
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	add := func(v float64) { lo, hi = math.Min(lo, v), math.Max(hi, v) }
	if r.Num {
		add(r.Lo)
		add(r.Hi)
	}
	if r.False || r.Nil {
		add(0)
	}
	if r.True {
		add(1)
	}
	if lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// exact lists the values when used as a number one by one (booleans are 0 and 1, nil is 0). ok is false if the exact values are unknown.
func (r Range) exact() ([]float64, bool) {
	if r.Any || r.StrAny || len(r.Strs) > 0 {
		return nil, false
	}
	var vs []float64
	if r.Num {
		switch {
		case r.Set != nil:
			vs = append(vs, r.Set...)
		case r.Lo == r.Hi:
			vs = append(vs, r.Lo)
		default:
			return nil, false
		}
	}
	if r.False || r.Nil {
		vs = append(vs, 0)
	}
	if r.True {
		vs = append(vs, 1)
	}
	return vs, len(vs) > 0
}

// anyNumber is any number at all.
func anyNumber() Range { return Between(math.Inf(-1), math.Inf(1)) }

// num turns an interval into a Range (any number if it becomes NaN).
func num(lo, hi float64) Range {
	if math.IsNaN(lo) || math.IsNaN(hi) {
		return anyNumber()
	}
	return Between(lo, hi)
}

// Abstract computes the range of values an expression can take. look returns the range for a name (and the field path after it)
// (Top if unknown). Functions know the built-ins plus rand and chance; any other call is Top.
func (e *Expr) Abstract(look func(Ref) Range) Range {
	if e == nil {
		return Top()
	}
	return abstract(e.root, look)
}

func abstract(n node, look func(Ref) Range) Range {
	if r, _, ok := chain(n); ok {
		return look(r)
	}
	switch x := n.(type) {
	case *lit:
		return Of(x.v)
	case *not:
		v := abstract(x.x, look)
		return Boolean(v.CanBeTrue(), v.CanBeFalse())
	case *neg:
		lo, hi, ok := abstract(x.x, look).numeric()
		if !ok {
			return Top()
		}
		return num(-hi, -lo)
	case *logic:
		l := abstract(x.l, look)
		if x.or {
			if !l.CanBeFalse() {
				return Boolean(false, true)
			}
			r := abstract(x.r, look).truth()
			if !l.CanBeTrue() {
				return r
			}
			return Boolean(r.False, true)
		}
		if !l.CanBeTrue() {
			return Boolean(true, false)
		}
		r := abstract(x.r, look).truth()
		if !l.CanBeFalse() {
			return r
		}
		return Boolean(true, r.True)
	case *cond:
		c := abstract(x.c, look)
		switch {
		case !c.CanBeFalse():
			return abstract(x.a, look)
		case !c.CanBeTrue():
			return abstract(x.b, look)
		}
		return abstract(x.a, look).Union(abstract(x.b, look))
	case *binop:
		return abstractBinop(x.op, abstract(x.l, look), abstract(x.r, look))
	case *call:
		return abstractCall(x, look)
	}
	return Top()
}

func abstractBinop(op string, l, r Range) Range {
	switch op {
	case "==", "!=":
		can, must := equality(l, r)
		if op == "==" {
			return Boolean(!must, can)
		}
		return Boolean(can, !must)
	}
	// if both sides have only a few exact values, really compute each pair
	if xs, ok := l.exact(); ok {
		if ys, ok := r.exact(); ok && len(xs)*len(ys) <= maxPairs {
			out := Range{}
			for _, x := range xs {
				for _, y := range ys {
					v, err := (&binop{op: op, l: &lit{x}, r: &lit{y}}).eval(nil)
					if err != nil {
						return Top()
					}
					out = out.Union(Of(v))
				}
			}
			return out
		}
	}
	a, b, ok1 := l.numeric()
	c, d, ok2 := r.numeric()
	if !ok1 || !ok2 {
		switch op {
		case "<", "<=", ">", ">=":
			return Boolean(true, true)
		}
		return Top()
	}
	switch op {
	case "+":
		return num(a+c, b+d)
	case "-":
		return num(a-d, b-c)
	case "*":
		ps := []float64{a * c, a * d, b * c, b * d}
		if slices.ContainsFunc(ps, math.IsNaN) {
			return anyNumber()
		}
		return num(slices.Min(ps), slices.Max(ps))
	case "/":
		if c <= 0 && d >= 0 {
			return anyNumber() // dividing by 0 gives 0; dividing near 0 gives something large
		}
		ps := []float64{a / c, a / d, b / c, b / d}
		if slices.ContainsFunc(ps, math.IsNaN) {
			return anyNumber()
		}
		return num(slices.Min(ps), slices.Max(ps))
	case "%":
		return remainder(a, b, c, d)
	case "<":
		return Boolean(!(b < c), a < d)
	case "<=":
		return Boolean(!(b <= c), a <= d)
	case ">":
		return Boolean(!(a > d), b > c)
	case ">=":
		return Boolean(!(a >= d), b >= c)
	}
	return Top()
}

// remainder is the remainder of a..b divided by c..d (on integer parts; 0 if the divisor is 0; a negative remainder gets the divisor added).
func remainder(a, b, c, d float64) Range {
	if c != d || math.IsInf(c, 0) {
		return anyNumber()
	}
	bi := int64(c)
	if bi == 0 {
		return Number(0)
	}
	if bi < 0 || math.IsInf(a, 0) || math.IsInf(b, 0) {
		if bi > 0 {
			return Between(0, float64(bi-1))
		}
		return anyNumber()
	}
	lo, hi := int64(a), int64(b) // integer parts (truncated toward 0)
	if lo >= 0 && lo/bi == hi/bi {
		return Between(float64(lo%bi), float64(hi%bi))
	}
	return Between(0, float64(bi-1))
}

// equality reports whether l == r can hold (can) and whether it must hold (must).
func equality(l, r Range) (can, must bool) {
	if l.Any || r.Any {
		return true, false
	}
	// between numbers (a boolean never equals a number)
	if l.Num && r.Num && l.Lo <= r.Hi && r.Lo <= l.Hi {
		switch {
		case l.Set != nil && r.Set != nil:
			for _, v := range l.Set {
				can = can || slices.Contains(r.Set, v)
			}
		case l.Set != nil:
			can = can || slices.ContainsFunc(l.Set, func(v float64) bool { return v >= r.Lo && v <= r.Hi })
		case r.Set != nil:
			can = can || slices.ContainsFunc(r.Set, func(v float64) bool { return v >= l.Lo && v <= l.Hi })
		default:
			can = true
		}
	}
	if (l.False && r.False) || (l.True && r.True) || (l.Nil && r.Nil) {
		can = true
	}
	if (l.StrAny && (r.StrAny || len(r.Strs) > 0)) || (r.StrAny && len(l.Strs) > 0) {
		can = true
	}
	for _, s := range l.Strs {
		if slices.Contains(r.Strs, s) {
			can = true
		}
	}
	lv, lok := single(l)
	rv, rok := single(r)
	return can, lok && rok && equal(lv, rv)
}

// single is the value if the range is exactly one value.
func single(r Range) (any, bool) {
	n := 0
	var v any
	if r.Any || r.StrAny {
		return nil, false
	}
	if r.Num {
		if r.Lo != r.Hi || math.IsInf(r.Lo, 0) {
			return nil, false
		}
		n, v = n+1, r.Lo
	}
	if r.False {
		n, v = n+1, false
	}
	if r.True {
		n, v = n+1, true
	}
	if r.Nil {
		n, v = n+1, nil
	}
	for _, s := range r.Strs {
		n, v = n+1, s
	}
	return v, n == 1
}

func abstractCall(x *call, look func(Ref) Range) Range {
	id, ok := x.fn.(*ident)
	if !ok {
		return Top()
	}
	args := make([]Range, len(x.args))
	for i, a := range x.args {
		args[i] = abstract(a, look)
	}
	nums := func() ([]float64, []float64, bool) {
		var lo, hi []float64
		for _, a := range args {
			l, h, ok := a.numeric()
			if !ok {
				return nil, nil, false
			}
			lo, hi = append(lo, l), append(hi, h)
		}
		return lo, hi, true
	}
	if f, ok := builtins[id.name]; ok && id.name != "has" && id.name != "len" && id.name != "str" && len(args) > 0 {
		// if both sides have only a few exact values, really compute each pair
		combos := [][]any{{}}
		ok := true
		for _, a := range args {
			vs, ok2 := a.exact()
			if !ok2 || len(combos)*len(vs) > maxPairs {
				ok = false
				break
			}
			var next [][]any
			for _, c := range combos {
				for _, v := range vs {
					next = append(next, append(slices.Clone(c), v))
				}
			}
			combos = next
		}
		if ok {
			out := Range{}
			for _, c := range combos {
				v, err := f(c)
				if err != nil {
					return Top()
				}
				out = out.Union(Of(v))
			}
			return out
		}
	}
	switch id.name {
	case "rand":
		lo, hi, ok := nums()
		switch {
		case !ok:
			return anyNumber()
		case len(args) == 0:
			return Between(0, 1)
		case len(args) == 1:
			return num(0, math.Max(0, math.Floor(hi[0])-1))
		case len(args) == 2:
			return num(math.Floor(lo[0]), math.Floor(hi[1]))
		}
		return anyNumber()
	case "chance":
		return Boolean(true, true)
	case "min", "max":
		lo, hi, ok := nums()
		if !ok || len(lo) == 0 {
			return Top()
		}
		if id.name == "min" {
			return num(slices.Min(lo), slices.Min(hi))
		}
		return num(slices.Max(lo), slices.Max(hi))
	case "abs":
		lo, hi, ok := nums()
		if !ok || len(lo) != 1 {
			return Top()
		}
		switch {
		case lo[0] >= 0:
			return num(lo[0], hi[0])
		case hi[0] <= 0:
			return num(-hi[0], -lo[0])
		}
		return num(0, math.Max(-lo[0], hi[0]))
	case "floor", "ceil", "round", "int":
		lo, hi, ok := nums()
		if !ok || len(lo) != 1 {
			return Top()
		}
		f := map[string]func(float64) float64{"floor": math.Floor, "ceil": math.Ceil, "round": math.Round, "int": math.Trunc}[id.name]
		return num(f(lo[0]), f(hi[0]))
	case "clamp":
		lo, hi, ok := nums()
		if !ok || len(lo) != 3 {
			return Top()
		}
		return num(math.Max(lo[1], math.Min(lo[2], lo[0])), math.Max(hi[1], math.Min(hi[2], hi[0])))
	case "sqrt", "pow", "len":
		return anyNumber()
	case "has":
		return Boolean(true, true)
	case "str":
		return Range{StrAny: true}
	}
	return Top()
}
