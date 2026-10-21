package gamedata_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestInterpolationsAndRender(t *testing.T) {
	cases := []struct {
		in    string
		exprs []string
		out   string
	}{
		{"plain", nil, "plain"},
		{"{gold}G", []string{"gold"}, "100G"},
		{"{gold} and {n + 1}", []string{"gold", "n + 1"}, "100 and 3"},
		{"{{gold}", nil, "{gold}"},
		{"{{{gold}}}", []string{"gold"}, "{100}}"},
		{"half {open", nil, "half {open"},
		{"{}", []string{""}, "{}"},
		{"{unknown}", []string{"unknown"}, "{unknown}"},
		{"{1 +}", []string{"1 +"}, "{1 +}"},
		{"{name}", []string{"name"}, "ミア"},
		{"{gold}{gold}", []string{"gold", "gold"}, "100100"},
		{"日本語 {gold} 日本語", []string{"gold"}, "日本語 100 日本語"},
		{"{ gold }", []string{" gold "}, "100"},
	}
	env := expr.Map{"gold": 100.0, "n": 2.0, "name": "ミア"}
	for _, c := range cases {
		if got := data.Interpolations(c.in); !slices.Equal(got, c.exprs) {
			t.Errorf("Interpolations(%q) = %q, want %q", c.in, got, c.exprs)
		}
		if got := data.Render(c.in, env); got != c.out {
			t.Errorf("Render(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

// TestFillKeepsTheValuesItWasShownWith (ADR 0027): a phrase shown again in another language does not evaluate its expressions again.
func TestFillKeepsTheValuesItWasShownWith(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":    "schema_version: 4\nid: p\nwritten_in: en\nlanguages: [en, ja]\n" + testgame.Game,
		"lang/ja.yaml": "text:\n  \"I have {gold}G, {item.potion} potions.\": \"{item.potion}個の薬と{gold}G。\"\n",
	})
	env := expr.Map{"gold": 100.0, "item": expr.Map{"potion": 2.0}}
	text, phrase := data.Fill(db, "I have {gold}G, {item.potion} potions.", env)
	if text != "I have 100G, 2 potions." {
		t.Errorf("source language: %q", text)
	}
	db.Show("ja")
	if got := data.Again(db, phrase); got != "2個の薬と100G。" {
		t.Errorf("shown again in Japanese with the values kept: %q", got)
	}
	// the values were taken from every translation's expressions, so a translation that spells them differently still finds them
	if phrase.Values["gold"] != "100" || phrase.Values["item.potion"] != "2" {
		t.Errorf("values %v", phrase.Values)
	}
	db.Show("en")
	if got := data.Again(db, phrase); got != text {
		t.Errorf("back: %q", got)
	}
	// an expression that fails stays as it was written
	bad, p := data.Fill(db, "{missing}", env)
	if bad != "{missing}" || len(p.Values) != 0 {
		t.Errorf("%q %v", bad, p)
	}
}

func TestWrittenInMustBeABuiltInLanguage(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 4\nid: p\nwritten_in: xx\nlanguages: [xx]\n" + testgame.Game})
	if err == nil {
		t.Fatal("an unknown written_in was accepted")
	}
	_, err = testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 4\nid: p\nlanguage: en\nlanguages: [ja]\n" + testgame.Game})
	if err == nil || !strings.Contains(err.Error(), "languages") {
		t.Errorf("languages must hold the language shown: %v", err)
	}
	if _, err := testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 3\nid: p\nlanguages: [en, ja]\n" + testgame.Game}); err == nil {
		t.Error("languages need schema_version 4")
	}
}

func TestEngineWordsFollowTheLanguage(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":    "schema_version: 4\nid: p\nlanguage: en\nwritten_in: en\nlanguages: [en, ja]\ntext:\n  \"Strike\": \"Hit\"\n" + testgame.Game,
		"lang/ja.yaml": "text:\n  \"Strike\": \"打つ\"\n",
	})
	if got := db.EngineWords("en")["Strike"]; got != "Hit" {
		t.Errorf("the shown language takes game.yaml's text: %q", got)
	}
	if got := db.EngineWords("ja")["Strike"]; got != "打つ" {
		t.Errorf("another language takes its file: %q", got)
	}
	if got := db.EngineWords("fr"); len(got) != 0 {
		t.Errorf("an unknown language has no words: %v", got)
	}
}

// TestCustomVerbsAreReadWithTheirValues: a verb declared in game.yaml is carried with its value; a typo is an error that says so.
func TestCustomVerbsAreReadWithTheirValues(t *testing.T) {
	v2 := "schema_version: 2\nid: v\n" + testgame.Game + "verbs: [dance, wave]\n"
	db := testgame.Load(t, map[string]string{"game.yaml": v2, "data/z.yaml": "events: {e: {do: [{dance: samba}, {wave: {hand: left}}]}}\n"})
	verb, arg := data.CustomVerb(&db.Events["e"].Do[0])
	if verb != "dance" || arg != "samba" {
		t.Errorf("%q %v", verb, arg)
	}
	verb, arg = data.CustomVerb(&db.Events["e"].Do[1])
	if verb != "wave" || arg == nil {
		t.Errorf("%q %v", verb, arg)
	}
	if verb, arg := data.CustomVerb(&data.Command{}); verb != "" || arg != nil {
		t.Errorf("a command that is not custom: %q %v", verb, arg)
	}
	for name, y := range map[string]string{
		"a builtin verb with an unknown field": "events: {e: {do: [{gold: 1, dance: samba}]}}",
		"two undeclared verbs":                 "events: {e: {do: [{dance: samba, wave: left}]}}",
		"a companion field with a custom verb": "events: {e: {do: [{dance: samba, count: 3}]}}",
		"no verb at all":                       "events: {e: {do: [{}]}}",
		"two builtin verbs":                    "events: {e: {do: [{gold: 1, heal: all}]}}",
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": v2, "data/z.yaml": y})
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestStandAndStagingMistakes(t *testing.T) {
	v4 := "schema_version: 4\nid: s\n" + testgame.Game
	faced := "actors: {helper: {name: Helper, class: ranger, faces: chars/helper}}\n"
	files := func(y string) map[string]string {
		return map[string]string{"game.yaml": v4, "data/z.yaml": faced + y, "assets/chars/helper/normal.png": "x"}
	}
	for name, c := range map[string]struct{ y, want string }{
		"stand with a word":         {"events: {e: {do: [{stand: sit}]}}", "stand is none or a map"},
		"stand with a leave":        {"events: {e: {do: [{stand: {who: helper, leave: spin}}]}}", "want fade or slide"},
		"stand of one with no face": {"events: {e: {do: [{stand: {who: scout}}]}}", "has no standing picture"},
		"effect with negative time": {"events: {e: {do: [{effect: {name: flash, seconds: -1}}]}}", "0 or more"},
	} {
		_, err := testgame.TryLoad(files(c.y))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want an error with %q, got %v", name, c.want, err)
		}
	}
	if _, err := testgame.TryLoad(files("events: {e: {do: [{stand: none}, {stand: {who: helper, at: 0.3, enter: slide}}]}}")); err != nil {
		t.Errorf("correct staging: %v", err)
	}
}
