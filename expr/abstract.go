package expr

// 式の抽象解釈: 名前の値を「取りうる幅」で与えて、式の取りうる値の幅を求める。
// 体験版を組み立てるとき（配布範囲の検査）、変数の幅（階は 1〜5 など）の下で、決して成り立たない条件を見つけるのに使う。
// 分からない所は「何でもありうる」（Top）に倒すので、成り立たないと言えるのは本当に成り立たない時だけ。

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// maxStrs は Range が覚える文字列の数の上限（越えたらどんな文字列もありうる）。
// maxSet は覚える数の数の上限（越えたら区間だけ）。maxPairs は 1 つずつ計算する組の数の上限。
const (
	maxStrs  = 64
	maxSet   = 64
	maxPairs = 256
)

// Range は値の取りうる幅。数（Lo〜Hi の区間）・真偽・文字列の和。Any は何でもありうる。
type Range struct {
	Any bool // 何でもありうる（分からない）

	Num    bool      // 数がありうる
	Lo, Hi float64   // Num のときの区間（両端を含む。±Inf も可）
	Set    []float64 // Num のとき、数がちょうどこのどれか（並べて重ねない。nil なら区間の全部）

	False, True bool // 真偽値の false・true がありうる

	Strs   []string // ありうる文字列（並べて重ねない）
	StrAny bool     // どんな文字列もありうる
	Nil    bool     // nil がありうる
}

// Top は何でもありうる値。
func Top() Range { return Range{Any: true} }

// Number は数 v ちょうど。
func Number(v float64) Range { return Range{Num: true, Lo: v, Hi: v, Set: []float64{v}} }

// Between は数の区間 lo〜hi。
func Between(lo, hi float64) Range {
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		return Number(lo)
	}
	return Range{Num: true, Lo: lo, Hi: hi}
}

// numbers は数のどれか（vs が空なら何も無い）。
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

// Boolean は真偽値（f が false、t が true の ありうる・ありえない）。
func Boolean(f, t bool) Range { return Range{False: f, True: t} }

// String is the singleton string range used by static-analysis clients.
func String(s string) Range { return Range{Strs: []string{s}} }

// Of は値 v ちょうど（数・真偽・文字列・nil。ほかは Top）。
func Of(v any) Range {
	switch x := v.(type) {
	case nil:
		return Range{Nil: true}
	case bool:
		return Boolean(!x, x)
	case string:
		return String(x)
	}
	if f, ok := ToNumber(v); ok {
		return Number(f)
	}
	return Top()
}

// Empty は何もありえないか（どの値も入っていない）。
func (r Range) Empty() bool {
	return !r.Any && !r.Num && !r.False && !r.True && len(r.Strs) == 0 && !r.StrAny && !r.Nil
}

// Union は r と o のどちらかの値。
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

// Equal は r と o が同じ値の幅か。
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

// CanBeTrue は値が真（Truthy）になりうるか。
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

// CanBeFalse は値が偽になりうるか。
func (r Range) CanBeFalse() bool {
	if r.Any || r.False || r.Nil || r.StrAny {
		return true
	}
	if r.Num && r.Lo <= 0 && r.Hi >= 0 && (r.Set == nil || slices.Contains(r.Set, 0)) {
		return true
	}
	return slices.Contains(r.Strs, "")
}

// truth は値の真偽の幅（真偽値の Range）。
func (r Range) truth() Range { return Boolean(r.CanBeFalse(), r.CanBeTrue()) }

// numeric は数として使ったときの区間（真偽は 0・1、nil は 0）。文字列がありうる・何でもありうるなら ok が false。
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

// exact は数として使ったときの値を 1 つずつ（真偽は 0・1、nil は 0）。ちょうどの値が分からなければ ok が false。
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

// anyNumber は数なら何でも。
func anyNumber() Range { return Between(math.Inf(-1), math.Inf(1)) }

// num は区間を Range にする（NaN になったら数なら何でも）。
func num(lo, hi float64) Range {
	if math.IsNaN(lo) || math.IsNaN(hi) {
		return anyNumber()
	}
	return Between(lo, hi)
}

// Abstract は式の取りうる値の幅を求める。look は名前（とその後のフィールドの並び）の幅を返す
// （分からなければ Top）。関数は組み込みの物と rand・chance を知っていて、ほかの呼び出しは Top。
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
	// 両方ともちょうどの値が少しだけなら、1 組ずつ本当に計算する
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
		for _, p := range ps {
			if math.IsNaN(p) {
				return anyNumber()
			}
		}
		return num(slices.Min(ps), slices.Max(ps))
	case "/":
		if c <= 0 && d >= 0 {
			return anyNumber() // 0 で割ると 0、0 の近くで割ると大きい
		}
		ps := []float64{a / c, a / d, b / c, b / d}
		for _, p := range ps {
			if math.IsNaN(p) {
				return anyNumber()
			}
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

// remainder は a〜b を c〜d で割った余り（整数部で。割る数が 0 なら 0、負の余りは割る数を足す）。
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
	lo, hi := int64(a), int64(b) // 整数部（0 へ向けて切り捨て）
	if lo >= 0 && lo/bi == hi/bi {
		return Between(float64(lo%bi), float64(hi%bi))
	}
	return Between(0, float64(bi-1))
}

// equality は l == r が成り立ちうるか（can）と、必ず成り立つか（must）。
func equality(l, r Range) (can, must bool) {
	if l.Any || r.Any {
		return true, false
	}
	// 数どうし（真偽は数と等しくならない）
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

// single は幅がちょうど 1 つの値ならその値。
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
		// ちょうどの値が少しだけなら、1 組ずつ本当に計算する
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
