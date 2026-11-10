package gamedata_test

import (
	"errors"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

const twoLanguages = "schema_version: 4\nid: b\nwritten_in: en\nlanguages: [en, ja]\n"

// TestLanguageWords: written_in and languages need version 4 and built-in languages, and
// the language shown is one of them (ADR 0027).
func TestLanguageWords(t *testing.T) {
	for _, c := range []struct{ head, want string }{
		{twoLanguages, ""},
		{"schema_version: 3\nid: b\nlanguages: [en, ja]\n", "languages needs schema_version: 4"},
		{"schema_version: 3\nid: b\nwritten_in: en\n", "written_in needs schema_version: 4"},
		{"schema_version: 4\nid: b\nlanguages: [en, fr]\n", `"fr" is not a built-in language`},
		{"schema_version: 4\nid: b\nlanguages: [en, en]\n", `"en" is written twice`},
		{"schema_version: 4\nid: b\nlanguage: ja\nlanguages: [en]\n", "the language shown (ja) is not one of languages"},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": c.head + testgame.Game})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%q: %v", c.head, err)
		}
	}
}

// TestTranslations: lang/<code>.yaml translates the names in place when the language is
// shown and the lines through Tr; check reports a translation whose expressions differ,
// and warns of keys that are no text of the game and of texts not translated.
func TestTranslations(t *testing.T) {
	files := map[string]string{
		"game.yaml":   twoLanguages + "language: ja\n" + testgame.Game,
		"data/z.yaml": `events: {hello: {do: [{say: {who: Courier, text: "Gold {gold}."}}, {choice: [{label: Stay}]}]}}`,
		"lang/ja.yaml": `text:
  "Camp": "野営地"
  "Gold {gold}.": "所持金 {gold}。"
  "Courier": "飛脚"
  "はい": "うん"
  "Old line": "古い台詞"
`,
	}
	db := testgame.Load(t, files)
	if db.Shown() != "ja" || db.Locations["camp"].Name != "野営地" || db.Tr("Gold {gold}.") != "所持金 {gold}。" || db.Tr("Stay") != "Stay" {
		t.Fatalf("shown %s: camp %q", db.Shown(), db.Locations["camp"].Name)
	}
	db.Show("en")
	if db.Locations["camp"].Name != "Camp" || db.Tr("Gold {gold}.") != "Gold {gold}." {
		t.Fatalf("back to English: %q", db.Locations["camp"].Name)
	}
	w := errors.Join(db.SceneWarnings()...).Error()
	for _, want := range []string{`"Old line" is not a text of the game`, `events.hello.do[1].choice[0].label: not translated to ja: "Stay"`} {
		if !strings.Contains(w, want) {
			t.Errorf("no warning %q in\n%s", want, w)
		}
	}
	if strings.Contains(w, `"はい" is not a text`) {
		t.Error("a word of the engine was taken for a stray key")
	}
	files["lang/ja.yaml"] = "text:\n  \"Gold {gold}.\": \"所持金。\"\n"
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), `the translation of "Gold {gold}." has the expressions {}, not {gold}`) {
		t.Errorf("a lost expression: %v", err)
	}
	files["lang/ja.yaml"] = "words: {}\n"
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "lang/ja.yaml") {
		t.Errorf("an unknown field: %v", err)
	}
}

func TestMapAdviceSourceTexts(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	db.Locations["town"].Map.Points["square"].Advice = []data.MapAdvice{{Heading: "Guide", Text: "Supplies {item.potion}"}}
	found := map[string]string{}
	for _, text := range db.SourceTexts() {
		found[text.Path] = text.Text
	}
	for suffix, want := range map[string]string{"heading": "Guide", "text": "Supplies {item.potion}"} {
		if got := found["locations.town.map.points.square.advice[0]."+suffix]; got != want {
			t.Fatalf("%s: %q", suffix, got)
		}
	}
}

// TestCopiesTranslateTheirOwnTitle: a copy of the definitions (Fork, and the view a
// state owns, ForState) shows its title in the language it is shown in, and shares the
// names of the definitions; and a view takes a game's instances without the loaded
// definitions or another view seeing them.
func TestCopiesTranslateTheirOwnTitle(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":    twoLanguages + testgame.Game,
		"lang/ja.yaml": "text:\n  \"Camp expedition\": \"野営の旅\"\n  \"Camp\": \"野営地\"\n",
	})
	for name, copyOf := range map[string]func(*data.DB) *data.DB{"Fork": data.Fork, "ForState": (*data.DB).ForState} {
		c := copyOf(db)
		c.Show("ja")
		if c.Game.Title != "野営の旅" || c.Locations["camp"].Name != "野営地" || c.Shown() != "ja" {
			t.Errorf("%s shown in ja: title %q, camp %q", name, c.Game.Title, c.Locations["camp"].Name)
		}
		c.Show("en")
		if c.Game.Title != "Camp expedition" || c.Locations["camp"].Name != "Camp" {
			t.Errorf("%s back in en: title %q, camp %q", name, c.Game.Title, c.Locations["camp"].Name)
		}
	}
	if db.AddInstance("monster_1", &data.Actor{Name: "Mote"}) {
		t.Error("the loaded definitions took an instance")
	}
	first, second := db.ForState(), db.ForState()
	if !first.AddInstance("monster_1", &data.Actor{Name: "Mote"}) || first.AddInstance("scout", &data.Actor{Name: "Mote"}) {
		t.Fatal("a view takes an instance under a new id, never under an authored actor's")
	}
	if db.Actors["monster_1"] != nil || second.Actors["monster_1"] != nil || first.ForState().Actors["monster_1"] != nil {
		t.Error("an instance left its view")
	}
	if data.Fork(first).Actors["monster_1"] == nil {
		t.Error("a fork of a view lost the view's instance")
	}
	first.RemoveInstance("scout")
	first.RemoveInstance("monster_1")
	if first.Actors["scout"] == nil || first.Actors["monster_1"] != nil {
		t.Error("RemoveInstance drops instances only")
	}
}
