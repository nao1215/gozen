package gamedata

import (
	"strings"
	"sync"

	"github.com/nao1215/gozen/internal/expr"
)

// Interpolations are the expressions embedded in a line of text as {expression}. "{{"
// is a literal brace.
func Interpolations(s string) []string {
	var out []string
	for {
		i := strings.Index(s, "{")
		if i < 0 {
			return out
		}
		if strings.HasPrefix(s[i:], "{{") {
			s = s[i+2:]
			continue
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			return out
		}
		out = append(out, s[i+1:i+j])
		s = s[i+j+1:]
	}
}

var textCache sync.Map // string -> *expr.Expr

func parseExpr(src string) (Expr, error) {
	if e, ok := textCache.Load(src); ok {
		if parsed, ok := e.(*expr.Expr); ok {
			return Expr{parsed}, nil
		}
	}
	e, err := expr.Parse(src)
	if err != nil {
		return Expr{}, err
	}
	textCache.Store(src, e)
	return Expr{e}, nil
}

// Render replaces each {expression} of s by its value in env. An expression that fails
// is left as written, so a mistake shows on the screen instead of stopping the game.
func Render(s string, env expr.Env) string {
	return fill(s, func(src string) (string, bool) { return evalText(src, env) })
}

// evalText is the value of one {expression} as text (false when it fails).
func evalText(src string, env expr.Env) (string, bool) {
	e, err := parseExpr(src)
	if err != nil {
		return "", false
	}
	v, err := e.Eval(env)
	if err != nil {
		return "", false
	}
	return expr.Format(v), true
}

// fill replaces each {expression} of s by value's text, leaving it as written when value
// has none.
func fill(s string, value func(src string) (string, bool)) string {
	if !strings.Contains(s, "{") {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, "{")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		if strings.HasPrefix(s[i:], "{{") {
			b.WriteString("{")
			s = s[i+2:]
			continue
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		src := s[i+1 : i+j]
		s = s[i+j+1:]
		if v, ok := value(src); ok {
			b.WriteString(v)
			continue
		}
		b.WriteString("{" + src + "}")
	}
}

// Phrase is a text as written together with the values its {expressions} had when it was
// shown, so the screen can show it again in another language without evaluating them
// again (ADR 0027: changing the language in the settings changes the text on the screen).
type Phrase struct {
	Source string
	Values map[string]string
}

// Fill is a source text translated into the language shown and filled in from env, with
// the phrase that shows it again (Again). The expressions of every translation are
// evaluated, so a translation that spells an expression differently still finds its value.
func Fill(db *DB, src string, env expr.Env) (string, Phrase) {
	p := Phrase{Source: src}
	keep := func(text string) {
		for _, e := range Interpolations(text) {
			if _, done := p.Values[e]; done {
				continue
			}
			if v, ok := evalText(e, env); ok {
				if p.Values == nil {
					p.Values = map[string]string{}
				}
				p.Values[e] = v
			}
		}
	}
	keep(src)
	for code := range db.Words {
		keep(db.TrIn(code, src))
	}
	return Again(db, p), p
}

// Again is a phrase in the language shown now, filled in with the values it was shown with.
func Again(db *DB, p Phrase) string {
	return fill(db.Tr(p.Source), func(src string) (string, bool) {
		v, ok := p.Values[src]
		return v, ok
	})
}
