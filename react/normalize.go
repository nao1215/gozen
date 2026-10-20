package react

import (
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// Normalize folds text so that casual spellings match the lexicon: full-width and
// half-width forms are unified, katakana becomes hiragana, Latin letters become lower
// case, and spaces, punctuation and long-vowel marks ("ー", "〜") are dropped. A run of
// three or more of the same character becomes two, so "かわいいいい" matches "かわいい".
// The lexicon and the input go through the same folding.
func Normalize(s string) string {
	var b strings.Builder
	var last rune
	run := 0
	for _, r := range width.Fold.String(s) {
		r = fold(r)
		if r == 0 {
			continue
		}
		if r == last {
			run++
		} else {
			last, run = r, 1
		}
		if run > 2 {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// fold maps one rune; 0 drops it.
func fold(r rune) rune {
	switch {
	case r >= 'ァ' && r <= 'ヶ':
		return r - 'ァ' + 'ぁ'
	case r == 'ー' || r == '〜' || r == '~' || r == '・':
		return 0
	case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
		return 0
	}
	return unicode.ToLower(r)
}

// lines splits text into non-empty normalized lines (for acrostics).
func lines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if n := Normalize(l); n != "" {
			out = append(out, n)
		}
	}
	return out
}
