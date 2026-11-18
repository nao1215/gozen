package play

import (
	"slices"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/screen/gfx"
)

// Ruby: a line marks the reading of a word with the notation Japanese web novels use,
// `|base《reading》`, and the bar may be left out when the base is a run of kanji
// (`漢字《かんじ》`). The braces are taken by expressions, so ruby has its own marks. The
// window shows the base and the reading small above it; the backlog shows the base.

// rubySpan is a reading over the runes [start, end) of the text without its marks.
type rubySpan struct {
	start, end int
	text       string
}

// parseRuby takes the marks out of s: the text to show and the readings over it. A `《`
// with no base before it, or with no `》` after it, is kept as written.
func parseRuby(s string) (string, []rubySpan) {
	if !strings.ContainsRune(s, '《') {
		return s, nil
	}
	var out []rune
	var spans []rubySpan
	bar, barRune := -1, '|' // where in out the base after a bar starts, and the bar
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '|' || r == '｜' {
			if bar >= 0 { // a bar after a bar: the first one was text
				out = slices.Insert(out, bar, barRune)
			}
			bar, barRune = len(out), r
			continue
		}
		if r == '《' {
			end := slices.Index(rs[i+1:], '》')
			start := bar
			if start < 0 {
				start = len(out)
				for start > 0 && isKanji(out[start-1]) {
					start--
				}
			}
			if end > 0 && start < len(out) {
				spans = append(spans, rubySpan{start, len(out), string(rs[i+1 : i+1+end])})
				i, bar = i+1+end, -1
				continue
			}
		}
		out = append(out, r)
	}
	if bar >= 0 { // a bar with no reading after it is text
		out = slices.Insert(out, bar, barRune)
	}
	return string(out), spans
}

func isKanji(r rune) bool { return unicode.Is(unicode.Han, r) || r == '々' || r == '〆' }

// rubyText is s without its ruby marks: the backlog, the trace of the screen and any text
// measured for the window.
func rubyText(s string) string {
	plain, _ := parseRuby(s)
	return plain
}

// drawRuby draws the readings over the lines of a page as far as shown runes of it are
// typed out. lines are the page's lines, at offset runes into the text; a reading shows
// once its whole base is typed. x and y place the first line, and step is a line's
// height.
func (g *Game) drawRuby(dst *ebiten.Image, spans []rubySpan, text string, lines []string, offset, shown int, x, y, step, size float64) {
	if len(spans) == 0 {
		return
	}
	rs := size * 0.5
	plain := []rune(text)
	at := offset
	for i, ln := range lines {
		lr := []rune(ln)
		// A wrapped line may have lost the space it was broken at: find it in the text.
		from := at
		for from+len(lr) <= len(plain) && string(plain[from:from+len(lr)]) != ln {
			from++
		}
		if from+len(lr) > len(plain) {
			return
		}
		for _, sp := range spans {
			if sp.start < from || sp.end > from+len(lr) || sp.end-offset > shown {
				continue
			}
			base := string(plain[sp.start:sp.end])
			left := gfx.TextWidth(string(plain[from:sp.start]), size)
			bw, rw := gfx.TextWidth(base, size), gfx.TextWidth(sp.text, rs)
			g.text(dst, sp.text, x+left+(bw-rw)/2, y+float64(i)*step-rs*1.05, rs, g.th.text)
		}
		at = from + len(lr)
	}
}
