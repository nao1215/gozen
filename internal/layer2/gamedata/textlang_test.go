package gamedata_test

import (
	"errors"
	"strings"
	"testing"

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
