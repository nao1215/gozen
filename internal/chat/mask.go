package chat

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Mask is a Filter that hides listed words. Matching normalizes the text with NFKC
// first, so width and a voiced mark written apart ("ｶﾞ", "か" and U+3099) do not
// matter, and ignores katakana versus hiragana, letter case, spaces, punctuation and
// long-vowel marks, so "ば-か" and "ＢＡＫＡ" are caught. The characters of the original
// text that formed the word are replaced with '*'.
type Mask struct {
	words [][]rune
}

// NewMask makes a Mask for the words. Words that fold to nothing are ignored.
func NewMask(words []string) *Mask {
	m := &Mask{}
	for _, w := range words {
		if f, _, _ := foldRunes(w); len(f) > 0 {
			m.words = append(m.words, f)
		}
	}
	return m
}

// Filter implements Filter. It never drops a message.
func (m *Mask) Filter(msg Message) (Message, bool) {
	orig := []rune(msg.Text)
	f, at, end := foldRunes(msg.Text)
	hide := make([]bool, len(orig))
	found := false
	for _, w := range m.words {
		for i := 0; i+len(w) <= len(f); i++ {
			if equal(f[i:i+len(w)], w) {
				for k := at[i]; k <= end[i+len(w)-1]; k++ {
					if !unicode.IsSpace(orig[k]) {
						hide[k] = true
					}
				}
				found = true
			}
		}
	}
	if !found {
		return msg, true
	}
	var b strings.Builder
	for i, r := range orig {
		if hide[i] {
			r = '*'
		}
		b.WriteRune(r)
	}
	msg.Text = b.String()
	return msg, true
}

func equal(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// foldRunes normalizes complete NFKC segments, folds each resulting rune and
// drops ignored ones. at and end map normalized runes to their original segment,
// including Hangul sequences whose components are letters rather than marks.
func foldRunes(s string) (out []rune, at, end []int) {
	i := 0
	for s != "" {
		n := norm.NFKC.NextBoundaryInString(s, true)
		segment := s[:n]
		j := i + utf8.RuneCountInString(segment)
		for _, r := range norm.NFKC.String(segment) {
			switch {
			case r >= 'ァ' && r <= 'ヶ':
				r = r - 'ァ' + 'ぁ'
			case r == 'ー' || r == '〜' || r == '~' || r == '・' || r == '-' ||
				unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
				continue
			}
			out = append(out, unicode.ToLower(r))
			at = append(at, i)
			end = append(end, j-1)
		}
		i = j
		s = s[n:]
	}
	return out, at, end
}
