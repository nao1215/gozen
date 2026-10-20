package play

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	lang "github.com/nao1215/gozen/internal/text"
)

// TestLanguageRow: a game that offers two languages has a language row in the settings;
// choosing the other shows the game's names and the engine's words in it, and the choice
// is kept in the settings (ADR 0027).
func TestLanguageRow(t *testing.T) {
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 4\nwritten_in: en\nlanguages: [en, ja]\n"), fsys["game.yaml"].Data...)}
	fsys["lang/ja.yaml"] = &fstest.MapFile{Data: []byte("text:\n  \"Camp\": \"野営地\"\n")}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	if g.language() != "en" || g.db.Locations["camp"].Name != "Camp" {
		t.Fatalf("starts in %s: %q", g.language(), g.db.Locations["camp"].Name)
	}
	s := newConfigScreen(g)
	if s.langRow < 0 {
		t.Fatal("no language row")
	}
	s.list.sel = s.langRow
	s.change(g, 1, false)
	if g.cfg.Language != "ja" || g.db.Locations["camp"].Name != "野営地" || lang.T("設定") != "設定" {
		t.Fatalf("after the change: %s, %q, %q", g.cfg.Language, g.db.Locations["camp"].Name, lang.T("設定"))
	}
	s.change(g, 1, false)
	if g.cfg.Language != "en" || g.db.Locations["camp"].Name != "Camp" || lang.T("設定") != "Settings" {
		t.Fatalf("back: %s, %q, %q", g.cfg.Language, g.db.Locations["camp"].Name, lang.T("設定"))
	}
}

const twoLanguageTalk = `events:
  talk:
    do:
      - say: {who: scout, text: "You have {gold} coins."}
      - say: {who: Keeper, text: "Welcome."}
      - choice:
          - {label: "Stay", do: []}
          - {label: "Leave", do: []}
        prompt: "Stay the night?"
`

const twoLanguageWords = `text:
  "Scout": "斥候"
  "You have {gold} coins.": "{gold} 枚 持っている。"
  "Keeper": "主人"
  "Welcome.": "いらっしゃい。"
  "Stay the night?": "泊まる？"
  "Stay": "泊まる"
  "Leave": "出る"
`

// twoLanguageGame is a game written in English that offers Japanese, at a talk with a line
// that fills in an expression, a speaker who is no actor and a question.
func twoLanguageGame(t *testing.T, script string) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 4\nwritten_in: en\nlanguages: [en, ja]\n"), fsys["game.yaml"].Data...)}
	fsys["lang/ja.yaml"] = &fstest.MapFile{Data: []byte(twoLanguageWords)}
	fsys["data/talk.yaml"] = &fstest.MapFile{Data: []byte(twoLanguageTalk)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), ShotDir: t.TempDir(), Location: "camp", Event: "talk", Script: script})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	return g
}

// switchLanguage chooses the next language in the settings, as the player does.
func switchLanguage(t *testing.T, g *Game) {
	t.Helper()
	s := newConfigScreen(g)
	if s.langRow < 0 {
		t.Fatal("no language row")
	}
	s.list.sel = s.langRow
	s.change(g, 1, false)
}

// TestLanguageChangeRewordsTheScreen: changing the language in the settings shows the open
// line, its speaker, the question, its options and the backlog in the new language at once,
// with the values the line was shown with, and keeps the option selected (ADR 0027).
func TestLanguageChangeRewordsTheScreen(t *testing.T) {
	g := twoLanguageGame(t, "wait 10\nquit\n")
	runToEnd(t, g)
	if g.talk.say.Text != "You have 100 coins." || g.talk.say.Name != "Scout" {
		t.Fatalf("the line before: %q by %q", g.talk.say.Text, g.talk.say.Name)
	}
	g.st.Gold = 7 // the line keeps the value it was shown with
	switchLanguage(t, g)
	if got := g.talk.say; got.Text != "100 枚 持っている。" || got.Name != "斥候" {
		t.Fatalf("the line after: %q by %q", got.Text, got.Name)
	}
	if got := strings.Join(g.talk.pages[g.talk.page], ""); got != "100 枚 持っている。" {
		t.Fatalf("the window shows %q", got)
	}
	if name, text := g.backlog[len(g.backlog)-1].shown(g.db); name != "斥候" || text != "100 枚 持っている。" {
		t.Fatalf("the backlog shows %q: %q", name, text)
	}
	switchLanguage(t, g)
	if got := g.talk.say; got.Text != "You have 100 coins." || got.Name != "Scout" {
		t.Fatalf("the line back: %q by %q", got.Text, got.Name)
	}
}

// TestLanguageChangeRewordsTheQuestion: the open question, its options and the backlog
// change language at once, and the option selected stays selected.
func TestLanguageChangeRewordsTheQuestion(t *testing.T) {
	g := twoLanguageGame(t, "wait 10\npress ok\nwait 10\npress ok\nwait 10\npress down\nwait 5\nquit\n")
	runToEnd(t, g)
	if g.talk.choice == nil || g.talk.choice.sel != 1 {
		t.Fatal("the question is not open with the second option selected")
	}
	switchLanguage(t, g)
	c := g.talk.choice
	if c == nil || c.items[0].label != "泊まる" || c.items[1].label != "出る" || c.sel != 1 {
		t.Fatalf("the options after: %+v", c)
	}
	if got := strings.Join(g.talk.pages[g.talk.page], ""); got != "泊まる？" {
		t.Fatalf("the prompt after: %q", got)
	}
	var log []string
	for _, l := range g.backlog {
		name, text := l.shown(g.db)
		log = append(log, name+":"+text)
	}
	if want := []string{"斥候:100 枚 持っている。", "主人:いらっしゃい。", ":泊まる？"}; !slices.Equal(log, want) {
		t.Fatalf("the backlog %q, want %q", log, want)
	}
}

// TestLanguageChangeRewordsTheAnswer: the backlog shows an option taken in the language
// shown now.
func TestLanguageChangeRewordsTheAnswer(t *testing.T) {
	g := twoLanguageGame(t, "wait 10\npress ok\nwait 10\npress ok\nwait 10\npress ok\nwait 10\nquit\n")
	runToEnd(t, g)
	last := g.backlog[len(g.backlog)-1]
	if _, text := last.shown(g.db); !last.answer || text != "→ Stay" {
		t.Fatalf("the answer before: %+v", last)
	}
	switchLanguage(t, g)
	if _, text := last.shown(g.db); text != "→ 泊まる" {
		t.Fatalf("the answer after: %q", text)
	}
}

// TestConfigRowsFollowTheGame: a game that cannot fight has no battle rows in the
// settings; a game with troops has them, and each row still sets its own value.
func TestConfigRowsFollowTheGame(t *testing.T) {
	g := openGame(t, Options{})
	labels := func(s *configScreen) []string {
		var out []string
		for _, it := range s.list.items {
			out = append(out, it.label)
		}
		return out
	}
	s := newConfigScreen(g)
	if !slices.Contains(labels(s), lang.T("戦闘の速さ")) || !slices.Contains(labels(s), lang.T("戦闘をオートではじめる")) {
		t.Fatalf("a game with troops lacks the battle rows: %q", labels(s))
	}
	g.db.Troops = nil
	s = newConfigScreen(g)
	for _, l := range labels(s) {
		if l == lang.T("戦闘の速さ") || l == lang.T("戦闘をオートではじめる") {
			t.Fatalf("a game without battles shows %q: %q", l, labels(s))
		}
	}
	skip := slices.Index(labels(s), lang.T("早送りで飛ばす文"))
	if skip < 0 || s.row(skip) != rowSkipAll {
		t.Fatalf("the skip row at %d: %q", skip, labels(s))
	}
	s.list.sel = skip
	before := g.cfg.BattleSpeed
	s.change(g, 1, false)
	if !g.cfg.SkipAll || g.cfg.BattleSpeed != before {
		t.Fatalf("the skip row set skip_all=%v, battle_speed=%d", g.cfg.SkipAll, g.cfg.BattleSpeed)
	}
}
