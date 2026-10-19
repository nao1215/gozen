package gamedata

import (
	"strings"
	"sync"

	"github.com/nao1215/gozen/expr"
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
		return Expr{e.(*expr.Expr)}, nil //nolint:errcheck // Concrete type is established by the decoder, cache, or preceding reflect.Type check.
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
		e, err := parseExpr(src)
		if err != nil {
			b.WriteString("{" + src + "}")
			continue
		}
		v, err := e.Eval(env)
		if err != nil {
			b.WriteString("{" + src + "}")
			continue
		}
		b.WriteString(expr.Format(v))
	}
}
