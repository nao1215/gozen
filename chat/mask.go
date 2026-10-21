package chat

import (
	"strings"
	"unicode"

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

// foldRunes normalizes s with NFKC cluster by cluster (a base character with the marks
// that follow it, so a voiced mark written apart and a half-width kana compose), folds
// each resulting rune and drops the ignored ones. at and end map each folded rune to the
// first and last rune index in s of the cluster it came from, so the whole original
// range is hidden even when normalizing changed the length.
func foldRunes(s string) (out []rune, at, end []int) {
	src := []rune(s)
	for i := 0; i < len(src); {
		j := i + 1
		for j < len(src) && isMark(src[j]) {
			j++
		}
		for _, r := range norm.NFKC.String(string(src[i:j])) {
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
	}
	return out, at, end
}

// isMark reports whether r attaches to the character before it: a combining mark or a
// half-width voiced or semi-voiced sound mark.
func isMark(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me) || r == 'ﾞ' || r == 'ﾟ'
}
