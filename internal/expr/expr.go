package expr

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Object is a value with named fields (an actor, the party, the flags).
type Object interface {
	// Field is the value of the field name; ok is false when there is no such field.
	Field(name string) (v any, ok bool)
}

// Env resolves the names an expression uses.
type Env interface {
	// Lookup is the value of the name; ok is false when the name is unknown.
	Lookup(name string) (v any, ok bool)
}

// Func is a function an expression can call.
type Func func(args []any) (any, error)

// Map is an Env (and an Object) made of a map.
type Map map[string]any

// Lookup implements Env.
func (m Map) Lookup(name string) (any, bool) { v, ok := m[name]; return v, ok }

// Field implements Object.
func (m Map) Field(name string) (any, bool) { v, ok := m[name]; return v, ok }

// Chain is an Env that looks names up in each Env in turn.
type Chain []Env

// Lookup implements Env.
func (c Chain) Lookup(name string) (any, bool) {
	for _, e := range c {
		if e == nil {
			continue
		}
		if v, ok := e.Lookup(name); ok {
			return v, true
		}
	}
	return nil, false
}

// Expr is a parsed expression.
type Expr struct {
	src  string
	root node
}

// Parse parses src.
func Parse(src string) (*Expr, error) {
	p := &parser{lex: lexer{src: src}}
	p.next()
	n, err := p.ternary()
	if err == nil {
		err = p.err
	}
	if err != nil {
		return nil, fmt.Errorf("%q: %w", src, err)
	}
	if p.tok.kind != tEOF {
		return nil, fmt.Errorf("%q: unexpected %q at %d", src, p.tok.text, p.tok.pos)
	}
	return &Expr{src: src, root: n}, nil
}

// String returns the parsed source; a nil expression returns an empty string.
func (e *Expr) String() string {
	if e == nil {
		return ""
	}
	return e.src
}

// Names are the top-level names the expression refers to (not the built-in functions),
// in the order they first appear. The data checker uses them to report unknown names.
func (e *Expr) Names() []string {
	if e == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	walk(e.root, func(n node) {
		if id, ok := n.(*ident); ok && !seen[id.name] {
			if _, builtin := builtins[id.name]; builtin {
				return
			}
			seen[id.name] = true
			out = append(out, id.name)
		}
	})
	return out
}

// Bare are the names written as a whole value: not called, and not followed by a field or an
// index (built-in functions included), in the order they first appear. The data checker
// uses them to find a function's name used as a value, such as floor + 1.
func (e *Expr) Bare() []string {
	if e == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	var visit func(n node)
	visit = func(n node) {
		switch x := n.(type) {
		case *ident:
			if !seen[x.name] {
				seen[x.name] = true
				out = append(out, x.name)
			}
		case *field:
			if _, ok := x.x.(*ident); !ok {
				visit(x.x)
			}
		case *index:
			if _, ok := x.x.(*ident); !ok {
				visit(x.x)
			}
			visit(x.i)
		case *call:
			if _, ok := x.fn.(*ident); !ok {
				visit(x.fn)
			}
			for _, a := range x.args {
				visit(a)
			}
		case *not:
			visit(x.x)
		case *neg:
			visit(x.x)
		case *logic:
			visit(x.l)
			visit(x.r)
		case *binop:
			visit(x.l)
			visit(x.r)
		case *cond:
			visit(x.c)
			visit(x.a)
			visit(x.b)
		}
	}
	visit(e.root)
	return out
}

// Step is one step of traversal after a name: a field (a.b, or a string index a["b"]) or an
// expression index
// (a[i]; Name is empty).
type Step struct {
	Name  string
	Index bool // written as a[...] (if Name is empty, a numeric or expression index)
}

// Ref is a name an expression refers to, followed by the sequence of fields and indexes (for
// party.members[0].hp,
// Name is party and Path is members, [], hp). The sequence ends at a function call.
type Ref struct {
	Name string
	Path []Step
}

// Refs returns the names an expression refers to, with the sequence of fields that follows, in
// order of appearance (built-in functions excluded).
// Data checks use it to find mistyped fields on fixed-type items (such as party).
func (e *Expr) Refs() []Ref {
	if e == nil {
		return nil
	}
	var out []Ref
	var visit func(n node)
	visit = func(n node) {
		if r, inner, ok := chain(n); ok {
			if _, builtin := builtins[r.Name]; !builtin || len(r.Path) > 0 {
				out = append(out, r)
			}
			for _, i := range inner {
				visit(i)
			}
			return
		}
		switch x := n.(type) {
		case *field:
			visit(x.x)
		case *index:
			visit(x.x)
			visit(x.i)
		case *call:
			if id, ok := x.fn.(*ident); !ok || !builtin(id.name) {
				visit(x.fn)
			}
			for _, a := range x.args {
				visit(a)
			}
		case *not:
			visit(x.x)
		case *neg:
			visit(x.x)
		case *logic:
			visit(x.l)
			visit(x.r)
		case *binop:
			visit(x.l)
			visit(x.r)
		case *cond:
			visit(x.c)
			visit(x.a)
			visit(x.b)
		}
	}
	visit(e.root)
	return out
}

// chain unravels a sequence of fields and indexes that starts from a name (inner is the index
// expression).
func chain(n node) (Ref, []node, bool) {
	switch x := n.(type) {
	case *ident:
		return Ref{Name: x.name}, nil, true
	case *field:
		r, inner, ok := chain(x.x)
		if !ok {
			return Ref{}, nil, false
		}
		r.Path = append(r.Path, Step{Name: x.name})
		return r, inner, true
	case *index:
		r, inner, ok := chain(x.x)
		if !ok {
			return Ref{}, nil, false
		}
		st := Step{Index: true}
		if l, ok := x.i.(*lit); ok {
			if s, ok := l.v.(string); ok {
				st.Name = s
			}
		}
		r.Path = append(r.Path, st)
		return r, append(inner, x.i), true
	}
	return Ref{}, nil, false
}

// Eval evaluates the expression in env.
func (e *Expr) Eval(env Env) (any, error) {
	if e == nil {
		return nil, errors.New("empty expression")
	}
	v, err := e.root.eval(env)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", e.src, err)
	}
	return v, nil
}

// Bool evaluates the expression and reports whether the result is truthy. A nil
// expression is true (an absent condition always holds).
func (e *Expr) Bool(env Env) (bool, error) {
	if e == nil {
		return true, nil
	}
	v, err := e.Eval(env)
	if err != nil {
		return false, err
	}
	return Truthy(v), nil
}

// Number evaluates the expression to a number. A nil expression is 0.
func (e *Expr) Number(env Env) (float64, error) {
	if e == nil {
		return 0, nil
	}
	v, err := e.Eval(env)
	if err != nil {
		return 0, err
	}
	f, ok := ToNumber(v)
	if !ok {
		return 0, fmt.Errorf("%s: %v is not a number", e.src, describe(v))
	}
	return f, nil
}

// UnmarshalText parses the expression from YAML or JSON text. A bare number or boolean
// in YAML arrives here as its text, which parses as the literal.
func (e *Expr) UnmarshalText(b []byte) error {
	p, err := Parse(string(b))
	if err != nil {
		return err
	}
	*e = *p
	return nil
}

// MarshalText writes the expression's source.
func (e Expr) MarshalText() ([]byte, error) { return []byte(e.src), nil }

// Truthy is the truth of a value: false, nil, 0 and "" are false, the rest true.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case string:
		return x != ""
	}
	return true
}

// ToNumber converts a value to a number (booleans are 0 and 1).
func ToNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case nil:
		return 0, true
	}
	return 0, false
}

func describe(v any) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case string:
		return strconv.Quote(x)
	case Func:
		return "a function"
	case Object:
		return "an object"
	}
	return fmt.Sprint(v)
}

// ---- lexer

type tokKind int

const (
	tEOF tokKind = iota
	tNum
	tStr
	tIdent
	tOp
)

type token struct {
	kind tokKind
	text string
	num  float64
	pos  int
}

type lexer struct {
	src string
	pos int
}

func isIdentStart(r byte) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r >= 0x80
}

func isDigit(r byte) bool { return r >= '0' && r <= '9' }

func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) && strings.ContainsRune(" \t\r\n", rune(l.src[l.pos])) {
		l.pos++
	}
	start := l.pos
	if l.pos >= len(l.src) {
		return token{kind: tEOF, pos: start}, nil
	}
	c := l.src[l.pos]
	switch {
	case isDigit(c) || (c == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1])):
		for l.pos < len(l.src) && (isDigit(l.src[l.pos]) || l.src[l.pos] == '.') {
			l.pos++
		}
		f, err := strconv.ParseFloat(l.src[start:l.pos], 64)
		if err != nil {
			return token{}, fmt.Errorf("bad number %q at %d", l.src[start:l.pos], start)
		}
		return token{kind: tNum, num: f, text: l.src[start:l.pos], pos: start}, nil
	case c == '"' || c == '\'':
		l.pos++
		var b strings.Builder
		for l.pos < len(l.src) && l.src[l.pos] != c {
			if l.src[l.pos] == '\\' && l.pos+1 < len(l.src) {
				l.pos++
			}
			b.WriteByte(l.src[l.pos])
			l.pos++
		}
		if l.pos >= len(l.src) {
			return token{}, fmt.Errorf("unterminated string at %d", start)
		}
		l.pos++
		return token{kind: tStr, text: b.String(), pos: start}, nil
	case isIdentStart(c):
		for l.pos < len(l.src) && (isIdentStart(l.src[l.pos]) || isDigit(l.src[l.pos])) {
			l.pos++
		}
		return token{kind: tIdent, text: l.src[start:l.pos], pos: start}, nil
	}
	for _, op := range []string{"==", "!=", "<=", ">=", "&&", "||"} {
		if strings.HasPrefix(l.src[l.pos:], op) {
			l.pos += 2
			return token{kind: tOp, text: op, pos: start}, nil
		}
	}
	if strings.ContainsRune("+-*/%()<>!,.[]?:", rune(c)) {
		l.pos++
		return token{kind: tOp, text: string(c), pos: start}, nil
	}
	return token{}, fmt.Errorf("unexpected character %q at %d", c, start)
}

// ---- parser

// maxDepth bounds the parser's recursion: a parenthesis, bracket, call argument or
// conditional branch costs two levels and a prefix operator one, so about a hundred
// nested parentheses are allowed. A hostile expression is then an error instead of
// exhausting the stack in the recursive parser and evaluator.
const maxDepth = 256

type parser struct {
	lex   lexer
	tok   token
	err   error
	depth int
}

// enter counts one level of nesting; the caller defers p.leave.
func (p *parser) enter() error {
	p.depth++
	if p.depth > maxDepth {
		return fmt.Errorf("expression nests too deeply at %d", p.tok.pos)
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

func (p *parser) next() {
	if p.err != nil {
		return
	}
	t, err := p.lex.next()
	if err != nil {
		p.err = err
		p.tok = token{kind: tEOF}
		return
	}
	p.tok = t
}

func (p *parser) is(op string) bool {
	return (p.tok.kind == tOp && p.tok.text == op) || (p.tok.kind == tIdent && p.tok.text == op && (op == "and" || op == "or" || op == "not"))
}

func (p *parser) expect(op string) error {
	if !p.is(op) {
		if p.err != nil {
			return p.err
		}
		return fmt.Errorf("expected %q at %d, found %q", op, p.tok.pos, p.tok.text)
	}
	p.next()
	return nil
}

func (p *parser) ternary() (node, error) {
	defer p.leave()
	if err := p.enter(); err != nil {
		return nil, err
	}
	c, err := p.or()
	if err != nil {
		return nil, err
	}
	if !p.is("?") {
		return c, nil
	}
	p.next()
	a, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if err := p.expect(":"); err != nil {
		return nil, err
	}
	b, err := p.ternary()
	if err != nil {
		return nil, err
	}
	return &cond{c, a, b}, nil
}

func (p *parser) or() (node, error) {
	l, err := p.and()
	for err == nil && (p.is("||") || p.is("or")) {
		p.next()
		var r node
		if r, err = p.and(); err == nil {
			l = &logic{or: true, l: l, r: r}
		}
	}
	return l, err
}

func (p *parser) and() (node, error) {
	l, err := p.equality()
	for err == nil && (p.is("&&") || p.is("and")) {
		p.next()
		var r node
		if r, err = p.equality(); err == nil {
			l = &logic{l: l, r: r}
		}
	}
	return l, err
}

func (p *parser) binary(sub func() (node, error), ops ...string) (node, error) {
	l, err := sub()
	for err == nil {
		op := ""
		for _, o := range ops {
			if p.is(o) {
				op = o
			}
		}
		if op == "" {
			break
		}
		p.next()
		var r node
		if r, err = sub(); err == nil {
			l = &binop{op: op, l: l, r: r}
		}
	}
	return l, err
}

func (p *parser) equality() (node, error) { return p.binary(p.relational, "==", "!=") }
func (p *parser) relational() (node, error) {
	return p.binary(p.additive, "<", "<=", ">", ">=")
}
func (p *parser) additive() (node, error) { return p.binary(p.multiplicative, "+", "-") }
func (p *parser) multiplicative() (node, error) {
	return p.binary(p.unary, "*", "/", "%")
}

func (p *parser) unary() (node, error) {
	defer p.leave()
	if err := p.enter(); err != nil {
		return nil, err
	}
	if p.is("!") || p.is("not") {
		p.next()
		x, err := p.unary()
		return &not{x}, err
	}
	if p.is("-") {
		p.next()
		x, err := p.unary()
		return &neg{x}, err
	}
	return p.postfix()
}

func (p *parser) postfix() (node, error) {
	x, err := p.primary()
	for err == nil {
		switch {
		case p.is("."):
			p.next()
			if p.tok.kind != tIdent {
				return nil, fmt.Errorf("expected a field name at %d", p.tok.pos)
			}
			x = &field{x: x, name: p.tok.text}
			p.next()
		case p.is("["):
			p.next()
			var i node
			if i, err = p.ternary(); err != nil {
				return nil, err
			}
			if err = p.expect("]"); err != nil {
				return nil, err
			}
			x = &index{x: x, i: i}
		case p.is("("):
			p.next()
			var args []node
			for !p.is(")") {
				var a node
				if a, err = p.ternary(); err != nil {
					return nil, err
				}
				args = append(args, a)
				if !p.is(",") {
					break
				}
				p.next()
			}
			if err = p.expect(")"); err != nil {
				return nil, err
			}
			x = &call{fn: x, args: args}
		default:
			return x, nil
		}
	}
	return x, err
}

func (p *parser) primary() (node, error) {
	if p.err != nil {
		return nil, p.err
	}
	t := p.tok
	switch t.kind {
	case tNum:
		p.next()
		return &lit{t.num}, nil
	case tStr:
		p.next()
		return &lit{t.text}, nil
	case tIdent:
		p.next()
		switch t.text {
		case "true":
			return &lit{true}, nil
		case "false":
			return &lit{false}, nil
		case "nil":
			return &lit{nil}, nil
		}
		return &ident{t.text}, nil
	case tOp:
		if t.text == "(" {
			p.next()
			x, err := p.ternary()
			if err != nil {
				return nil, err
			}
			return x, p.expect(")")
		}
	case tEOF:
		return nil, errors.New("unexpected end of expression")
	}
	return nil, fmt.Errorf("unexpected %q at %d", t.text, t.pos)
}

// Comparison is a name compared with a string written in the expression (a == 'x', 'x' !=
// a.b): the name with its path, the operator, and the string.
type Comparison struct {
	Ref  Ref
	Op   string
	Text string
}

// Comparisons are the comparisons (== and !=) of a name with a string written in the
// expression, in order: checks use them to find a word that is not one (a misspelt id).
func (e *Expr) Comparisons() []Comparison {
	if e == nil {
		return nil
	}
	var out []Comparison
	walk(e.root, func(n node) {
		b, ok := n.(*binop)
		if !ok || (b.op != "==" && b.op != "!=") {
			return
		}
		for _, side := range [][2]node{{b.l, b.r}, {b.r, b.l}} {
			l, ok := side[1].(*lit)
			if !ok {
				continue
			}
			s, ok := l.v.(string)
			if !ok {
				continue
			}
			if r, _, ok := chain(side[0]); ok {
				out = append(out, Comparison{Ref: r, Op: b.op, Text: s})
			}
		}
	})
	return out
}

// StringArgs are, for each call of the function fn, its arguments written as strings (""
// for an argument that is not one).
func (e *Expr) StringArgs(fn string) [][]string {
	if e == nil {
		return nil
	}
	var out [][]string
	walk(e.root, func(n node) {
		c, ok := n.(*call)
		if !ok {
			return
		}
		if id, ok := c.fn.(*ident); !ok || id.name != fn {
			return
		}
		args := make([]string, len(c.args))
		for i, a := range c.args {
			if l, ok := a.(*lit); ok {
				if v, ok := l.v.(string); ok {
					args[i] = v
				}
			}
		}
		out = append(out, args)
	})
	return out
}
