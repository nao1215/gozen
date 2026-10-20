package checkdata

import (
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	data "github.com/nao1215/gozen/schema"
)

func TestLanguageWarnings(t *testing.T) {
	fsys := fstest.MapFS{"assets/latin.ttf": {Data: goregular.TTF}}
	latin := data.Fonts{Regular: "latin.ttf"}
	cases := []struct {
		name string
		game data.Game
		want []string // parts of the warnings, in order; nil: none
	}{
		{"an English game with a Latin font", data.Game{Title: "Trail", Fonts: latin, Text: map[string]string{"はい": "Sure"}}, nil},
		{"Japanese words in a Latin font", data.Game{Title: "Trail", Language: "ja", Fonts: latin}, []string{`fonts.regular: latin.ttf has no glyph for`, `language "ja"`}},
		{"a Japanese game without language", data.Game{Title: "小径", Fonts: data.Fonts{}}, []string{"add language: ja"}},
		{"a Japanese replacement in a Latin font", data.Game{Title: "Trail", Language: "en", Fonts: latin, Text: map[string]string{"はい": "うん"}}, []string{"letters of the engine's words in language \"en\"", "う ん"}},
		{"a missing font file is the data check's business", data.Game{Title: "Trail", Language: "ja", Fonts: data.Fonts{Regular: "none.ttf"}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(languageWarnings(fsys, &c.game), "\n")
			if c.want == nil && got != "" {
				t.Fatalf("unexpected warnings:\n%s", got)
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Fatalf("want %q in:\n%s", w, got)
				}
			}
		})
	}
}
