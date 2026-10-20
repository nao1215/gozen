package chat

import (
	"strings"
	"unicode"

	"golang.org/x/text/width"
)

// Mask is a Filter that hides listed words. Matching ignores width, katakana versus
// hiragana, letter case, spaces, punctuation and long-vowel marks, so "ば-か" and
// "ＢＡＫＡ" are caught; the characters of the original text that formed the word are
// replaced with '*'.
type Mask struct {
	words [][]rune
}

// NewMask makes a Mask for the words. Words that fold to nothing are ignored.
func NewMask(words []string) *Mask {
	m := &Mask{}
	for _, w := range words {
		if f, _ := foldRunes(w); len(f) > 0 {
			m.words = append(m.words, f)
		}
	}
	return m
}

// Filter implements Filter. It never drops a message.
func (m *Mask) Filter(msg Message) (Message, bool) {
	orig := []rune(msg.Text)
	f, at := foldRunes(msg.Text)
	hide := make([]bool, len(orig))
	found := false
	for _, w := range m.words {
		for i := 0; i+len(w) <= len(f); i++ {
			if equal(f[i:i+len(w)], w) {
				for k := at[i]; k <= at[i+len(w)-1]; k++ {
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

// foldRunes folds each rune of s and drops the ignored ones; at maps each folded rune
// to its rune index in s.
func foldRunes(s string) (out []rune, at []int) {
	for i, r := range []rune(s) {
		r = []rune(width.Fold.String(string(r)))[0]
		switch {
		case r >= 'ァ' && r <= 'ヶ':
			r = r - 'ァ' + 'ぁ'
		case r == 'ー' || r == '〜' || r == '~' || r == '・' || r == '-' ||
			unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			continue
		}
		out = append(out, unicode.ToLower(r))
		at = append(at, i)
	}
	return out, at
}
