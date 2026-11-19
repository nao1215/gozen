package gfx

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

// pageText draws a line of the kind a game writes: kana, latin words, spaces, sentence
// ends of both scripts and newlines.
func pageText(t *rapid.T) string {
	parts := rapid.SliceOfN(rapid.SampledFrom([]string{"あ", "漢", "A", "word", " ", "、", "。", ".", "!", "\n"}), 0, 80).Draw(t, "parts")
	return strings.Join(parts, "")
}

// visible is the text without its spaces and newlines, which wrapping may move or drop.
func visible(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// TestPaginateProperties: whatever the text, pages lose and repeat nothing, no page has
// more lines than the window, and the size stays between the floor and the asked size.
func TestPaginateProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := pageText(t)
		size := rapid.Float64Range(8, 40).Draw(t, "size")
		least := size * rapid.Float64Range(pageShrink, 1).Draw(t, "floor")
		width := rapid.Float64Range(40, 400).Draw(t, "width")
		lines := rapid.IntRange(1, 4).Draw(t, "lines")
		pages, sz := paginateFrom(s, size, least, lines, widthWrap(width))
		if got := visible(strings.Join(slices.Concat(pages...), "")); got != visible(s) {
			t.Fatalf("pages %q keep %q, want %q", pages, got, visible(s))
		}
		for i, p := range pages {
			if len(p) > lines {
				t.Fatalf("page %d has %d lines, window %d: %q", i, len(p), lines, p)
			}
		}
		if sz > size || sz < least-1e-9 {
			t.Fatalf("size %v out of [%v, %v]", sz, least, size)
		}
	})
}

// TestPaginateMetamorphic: one sentence in a window with more lines, or at a fixed size in a
// wider window, never takes more pages. Several sentences may: pages begin at a sentence
// where they can, so "!" and a sentence of four lines take three pages of four lines and
// two of three.
func TestPaginateMetamorphic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		parts := rapid.SliceOfN(rapid.SampledFrom([]string{"あ", "漢", "A", "word", " ", "、"}), 0, 80).Draw(t, "parts")
		s := strings.Join(parts, "")
		size := rapid.Float64Range(8, 40).Draw(t, "size")
		width := rapid.Float64Range(40, 400).Draw(t, "width")
		lines := rapid.IntRange(1, 4).Draw(t, "lines")
		base, _ := paginateFrom(s, size, size, lines, widthWrap(width))
		taller, _ := paginateFrom(s, size, size, lines+1, widthWrap(width))
		if len(taller) > len(base) {
			t.Fatalf("%d lines take %d pages, %d lines %d", lines+1, len(taller), lines, len(base))
		}
		wider, _ := paginateFrom(s, size, size, lines, widthWrap(width*rapid.Float64Range(1, 3).Draw(t, "grow")))
		if len(wider) > len(base) {
			t.Fatalf("a wider window takes %d pages, %d before", len(wider), len(base))
		}
	})
}
