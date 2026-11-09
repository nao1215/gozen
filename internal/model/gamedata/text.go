package gamedata

import (
	"iter"
	"strings"

	"github.com/nao1215/gozen/internal/base/expr"
)

// Interpolations are the expressions embedded in a line of text as {expression}. "{{"
// is a literal brace.
func Interpolations(s string) []string {
	var out []string
	for part := range textParts(s) {
		if part.expression {
			out = append(out, part.text)
		}
	}
	return out
}

// textPart is either literal text (with escaped braces resolved) or an expression.
// Both validation and rendering consume the same delimiter rules.
type textPart struct {
	text       string
	expression bool
}

func textParts(s string) iter.Seq[textPart] {
	return func(yield func(textPart) bool) {
		for {
			i := strings.IndexByte(s, '{')
			if i < 0 {
				yield(textPart{text: s})
				return
			}
			if !yield(textPart{text: s[:i]}) {
				return
			}
			s = s[i:]
			if strings.HasPrefix(s, "{{") {
				if !yield(textPart{text: "{"}) {
					return
				}
				s = s[2:]
				continue
			}
			j := interpolationEnd(s)
			if j < 0 {
				yield(textPart{text: s})
				return
			}
			if !yield(textPart{text: s[1:j], expression: true}) {
				return
			}
			s = s[j+1:]
		}
	}
}

// interpolationEnd follows the expression lexer's quote and escape rules.
func interpolationEnd(s string) int {
	var quote byte
	for i := 1; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		} else if c == '\'' || c == '"' {
			quote = c
		} else if c == '}' {
			return i
		}
	}
	return -1
}

// quoteExprString protects a value interpolated into an expression. The lexer
// accepts literal newlines, and uses backslash to escape its delimiter or itself.
func quoteExprString(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
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
	b.Grow(len(s))
	for part := range textParts(s) {
		if !part.expression {
			b.WriteString(part.text)
		} else if v, ok := value(part.text); ok {
			b.WriteString(v)
		} else {
			b.WriteByte('{')
			b.WriteString(part.text)
			b.WriteByte('}')
		}
	}
	return b.String()
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
