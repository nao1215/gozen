package gamedata

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nao1215/gozen/internal/base/diag"
)

// SpeechTerms declares preferred text and explicitly disallowed alternatives.
// Patterns are optional RE2 expressions, for language-specific morphology.
type SpeechTerms struct {
	Preferred string   `yaml:"preferred"`
	Avoid     []string `yaml:"avoid"`
	Patterns  []string `yaml:"patterns"`
}

// SpeechStyle selects literal word or substring matching for one language.
// Endings and addresses are advisory lexical checks, not semantic attribution.
type SpeechStyle struct {
	Match       string                 `yaml:"match"`
	FirstPerson SpeechTerms            `yaml:"first_person"`
	Addresses   map[string]SpeechTerms `yaml:"addresses"`
	Endings     SpeechTerms            `yaml:"endings"`
}

func (s SpeechStyle) terms() []SpeechTerms {
	out := []SpeechTerms{s.FirstPerson, s.Endings}
	for _, k := range slices.Sorted(maps.Keys(s.Addresses)) {
		out = append(out, s.Addresses[k])
	}
	return out
}

func (c *checker) speechDefinitions() {
	for _, id := range slices.Sorted(maps.Keys(c.db.Actors)) {
		for _, language := range slices.Sorted(maps.Keys(c.db.Actors[id].Speech)) {
			style := c.db.Actors[id].Speech[language]
			at := "actors." + id + ".speech." + language
			c.needsVersion(at, "speech checks", SchemaVersion10)
			if style.Match != "" && style.Match != "word" && style.Match != "substring" {
				c.errorf(diag.Shape, at+".match", "want word or substring")
			}
			for _, term := range style.terms() {
				if (len(term.Avoid) > 0 || len(term.Patterns) > 0) && term.Preferred == "" {
					c.errorf(diag.Shape, at, "preferred is required with alternatives")
				}
				for _, token := range term.Avoid {
					if token == "" || token == term.Preferred {
						c.errorf(diag.Shape, at, "avoid must be nonempty and different from preferred")
					}
				}
				for _, pattern := range term.Patterns {
					if _, err := regexp.Compile(pattern); err != nil {
						c.errorf(diag.Shape, at, "invalid speech pattern: %v", err)
					}
				}
			}
		}
	}
}

func speechContains(text, token, mode string) bool {
	if mode == "substring" {
		return strings.Contains(text, token)
	}
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
	offset := 0
	for offset <= len(text) {
		i := strings.Index(text[offset:], token)
		if i < 0 {
			return false
		}
		i += offset
		left, right := []rune(text[:i]), []rune(text[i+len(token):])
		if (len(left) == 0 || !word(left[len(left)-1])) && (len(right) == 0 || !word(right[0])) {
			return true
		}
		// The next match may begin inside this one (!! in a!!!).
		_, size := utf8.DecodeRuneInString(text[i:])
		offset = i + max(size, 1)
	}
	return false
}

func (c *checker) speechLine(w string, line *Say) {
	if line.Who.Expr.Set() || line.TextExpr.Set() {
		return
	}
	actor := c.db.Actors[line.Who.ID]
	if actor == nil {
		return
	}
	style, ok := actor.Speech[c.db.Game.Language]
	if !ok {
		return
	}
	for _, term := range style.terms() {
		found := ""
		for _, token := range term.Avoid {
			if token != "" && speechContains(line.Text, token, style.Match) {
				found = token
				break
			}
		}
		if found == "" {
			for _, pattern := range term.Patterns {
				if re, err := regexp.Compile(pattern); err == nil {
					found = re.FindString(line.Text)
					if found != "" {
						break
					}
				}
			}
		}
		if found != "" {
			c.warnf(diag.SpeechConsistency, w+".text", "%s", fmt.Sprintf("speaker %s uses %q; declared preference is %q", line.Who.ID, found, term.Preferred))
		}
	}
}
