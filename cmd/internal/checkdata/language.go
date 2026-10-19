package checkdata

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/image/font/sfnt"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/internal/text/catalog"
)

// languageWarnings reports a game whose own words are Japanese while game.yaml leaves the
// engine's words at their English default, and a regular font that lacks characters of the
// engine's words in the chosen language or of game.yaml's text (they would show as boxes).
// Only letters and digits count: a font for another script is the usual cause.
func languageWarnings(fsys fs.FS, g *data.Game) []string {
	var out []string
	if g.Language == "" && japanese(g) {
		out = append(out, "game.yaml language: not set, so the engine's own words (menus, battle messages) are English while the game's text is Japanese; add language: ja for Japanese words")
	}
	if g.Fonts.Regular == "" {
		return out
	}
	b, err := fs.ReadFile(fsys, "assets/"+g.Fonts.Regular)
	if err != nil {
		return out // the data check reports a missing font
	}
	f, err := sfnt.Parse(b)
	if err != nil {
		return out // not a single font (a collection, or broken); the game reports what it cannot load
	}
	words, err := catalog.Words(g.Language)
	if err != nil {
		return out // the data check reports the language
	}
	seen := map[rune]bool{}
	var missing []rune
	var buf sfnt.Buffer
	check := func(s string) {
		for _, r := range s {
			if seen[r] || !unicode.IsLetter(r) && !unicode.IsNumber(r) { // symbols are the font's own choice
				continue
			}
			seen[r] = true
			if i, err := f.GlyphIndex(&buf, r); err == nil && i == 0 {
				missing = append(missing, r)
			}
		}
	}
	for _, w := range words {
		check(w)
	}
	for _, w := range g.Text {
		check(w)
	}
	if len(missing) == 0 {
		return out
	}
	slices.Sort(missing)
	shown := missing[:min(len(missing), 12)]
	more := ""
	if len(missing) > len(shown) {
		more = fmt.Sprintf(" and %d more", len(missing)-len(shown))
	}
	return append(out, fmt.Sprintf("game.yaml fonts.regular: %s has no glyph for %d letters of the engine's words in language %q or of text (shown as boxes): %s%s",
		g.Fonts.Regular, len(missing), cmpLanguage(g.Language), strings.Join(strings.Split(string(shown), ""), " "), more))
}

func cmpLanguage(l string) string {
	if l == "" {
		return catalog.English
	}
	return l
}

// japanese tells whether the game's title, subtitle or text replacements are written in Japanese.
func japanese(g *data.Game) bool {
	has := func(s string) bool {
		for _, r := range s {
			if unicode.In(r, unicode.Hiragana, unicode.Katakana, unicode.Han) {
				return true
			}
		}
		return false
	}
	if has(g.Title) || has(g.Subtitle) {
		return true
	}
	for _, w := range g.Text {
		if has(w) {
			return true
		}
	}
	return false
}
