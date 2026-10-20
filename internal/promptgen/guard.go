package promptgen

import (
	"strings"
	"unicode"
)

// These guards cover adult-only person selections. Catalogs cannot replace,
// weaken, or count them against the configurable negative term limit.
const mature = "adult"
const negativeFixed = "child, underage"

func hasPerson(sel []selected) bool {
	for _, s := range sel {
		if s.l.Person {
			return true
		}
	}
	return false
}
func guardWords(side string) []string {
	if side == "positive" {
		return strings.Split(mature, ", ")
	}
	return strings.Split(negativeFixed, ", ")
}
func protected(word string) bool {
	for _, side := range []string{"positive", "negative"} {
		for _, w := range guardWords(side) {
			if core(word) == w {
				return true
			}
		}
	}
	return false
}
func phrase(s string) string {
	return " " + strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ") + " "
}
func checkGuard(side, word string) error {
	if side == "positive" {
		for _, w := range guardWords("negative") {
			if strings.Contains(phrase(word), phrase(w)) {
				return problem("positive contains an age guard exclusion: %s", word)
			}
		}
	}
	if protected(word) {
		return problem("catalog cannot override an age guard: %s", word)
	}
	return nil
}
