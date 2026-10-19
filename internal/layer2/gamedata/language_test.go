package gamedata

import (
	"testing"
	"testing/fstest"

	lang "github.com/nao1215/gozen/text"
)

func TestUseWordsFollowsGameYAML(t *testing.T) {
	defer func() {
		if err := lang.Use(lang.Japanese, nil); err != nil {
			t.Error(err)
		}
	}()
	for _, c := range []struct{ yaml, key, want string }{
		{"title: T\n", "はい", "Yes"},
		{"language: ja\ntext: {はい: うん}\n", "はい", "うん"},
		{"language: ja\n", "いいえ", "いいえ"},
		{"language: en\ntext: {はい: Sure}\n", "はい", "Sure"},
		{"language: xx\n", "はい", "Yes"},
		{"text: [broken\n", "はい", "Yes"},
	} {
		if err := UseWords(fstest.MapFS{"game.yaml": {Data: []byte(c.yaml)}}); err != nil {
			t.Fatal(err)
		}
		if got := lang.T(c.key); got != c.want {
			t.Errorf("%q: T(%q) = %q, want %q", c.yaml, c.key, got, c.want)
		}
	}
	if err := UseWords(fstest.MapFS{}); err != nil || lang.T("はい") != "Yes" {
		t.Errorf("without game.yaml: %v %q", err, lang.T("はい"))
	}
}

func TestDefaultSlotNamesFollowTheLanguage(t *testing.T) {
	for _, c := range []struct {
		g    Game
		want string
	}{{Game{}, "Weapon"}, {Game{Language: "ja"}, "武器"}, {Game{Language: "en", Text: map[string]string{"武器": "Blade"}}, "Blade"}} {
		db := &DB{Game: c.g}
		db.defaults()
		if got := db.Game.Slots[0].Name; got != c.want {
			t.Errorf("%+v: %q, want %q", c.g, got, c.want)
		}
	}
}
