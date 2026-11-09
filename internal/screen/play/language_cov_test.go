package play

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sysTwoLanguages(t *testing.T, opts Options) *Game {
	t.Helper()
	return sysOpen4(t, opts, map[string]string{
		"game.yaml+":     "written_in: en\nlanguages: [en, ja]\n",
		"lang/ja.yaml":   twoLanguageWords + "  \"Camp\": \"野営地\"\n",
		"data/talk.yaml": twoLanguageTalk,
	})
}

// TestLanguageChoiceIsKeptForTheNextRun: the language chosen in the settings is written with
// the settings and is the language of the next play.
func TestLanguageChoiceIsKeptForTheNextRun(t *testing.T) {
	dir := t.TempDir()
	g := sysTwoLanguages(t, Options{SaveDir: dir, Location: "camp"})
	if g.language() != "en" || g.db.Locations["camp"].Name != "Camp" {
		t.Fatalf("starts in %s with %q", g.language(), g.db.Locations["camp"].Name)
	}
	g.push(newConfigScreen(g))
	s := sysAs[*configScreen](t, g.stack[len(g.stack)-1])
	s.list.sel = s.langRow
	s.change(g, 1, false)
	s.list.sel = len(s.list.items) - 1
	sysKey(g, btnOK)
	s.update(g)
	b, err := os.ReadFile(filepath.Join(dir, "config.json")) //nolint:gosec // reads a file this test wrote
	if err != nil || !strings.Contains(string(b), `"language": "ja"`) {
		t.Fatalf("config %s (%v)", b, err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g2 := sysTwoLanguages(t, Options{SaveDir: dir, Location: "camp"})
	if g2.language() != "ja" || g2.db.Locations["camp"].Name != "野営地" {
		t.Fatalf("restarted in %s with %q", g2.language(), g2.db.Locations["camp"].Name)
	}
}

// TestLanguageNotOfferedIsIgnored: a config.json that names a language the game does not offer
// does not change the language.
func TestLanguageNotOfferedIsIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"language": "fr"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	g := sysTwoLanguages(t, Options{SaveDir: dir})
	if g.language() != "en" {
		t.Fatalf("language %s", g.language())
	}
}

// TestSaveHoldsNoTextSoItLoadsInEitherLanguage: a save made in English loads after a switch to
// Japanese, with the place named in the language shown now.
func TestSaveHoldsNoTextSoItLoadsInEitherLanguage(t *testing.T) {
	g := sysTwoLanguages(t, Options{Seed: 2, Location: "camp"})
	g.st.Gold = 31
	if err := g.save(0); err != nil {
		t.Fatal(err)
	}
	label, ok := g.slotLabel(0)
	if !ok || !strings.Contains(label, "Camp") || !strings.Contains(label, "Scout") {
		t.Fatalf("label %q", label)
	}
	g.cfg.Language = "ja"
	if err := g.useLanguage(); err != nil {
		t.Fatal(err)
	}
	label, ok = g.slotLabel(0)
	if !ok || !strings.Contains(label, "野営地") || !strings.Contains(label, "斥候") {
		t.Fatalf("label after the switch %q", label)
	}
	st, err := g.load(0)
	if err != nil {
		t.Fatal(err)
	}
	g.loadSavedGame(st)
	if g.st.Gold != 31 || g.st.Location != "camp" {
		t.Fatalf("loaded %d gold at %s", g.st.Gold, g.st.Location)
	}
}

// TestRollBackAfterASwitchShowsTheLineInTheNewLanguage: a line rolled back to is shown in the
// language chosen after it was first read.
func TestRollBackAfterASwitchShowsTheLineInTheNewLanguage(t *testing.T) {
	g := sysTwoLanguages(t, Options{Seed: 2, Location: "camp", Event: "talk", Line: 2})
	sysRunFast(t, g, 3000)
	if g.talk.say.Text != "Welcome." {
		t.Fatalf("at %q", g.talk.say.Text)
	}
	g.cfg.Language = "ja"
	if err := g.useLanguage(); err != nil {
		t.Fatal(err)
	}
	g.talk.reword(g)
	if g.talk.say.Text != "いらっしゃい。" {
		t.Fatalf("after the switch %q", g.talk.say.Text)
	}
	g.rollBack()
	sysFrames(t, g, 2)
	if g.talk.say.Text != "100 枚 持っている。" {
		t.Fatalf("rolled back to %q", g.talk.say.Text)
	}
}

// TestSwitchOfLanguageKeepsReadLines: the read record is by line, not by text, so a switch of
// language does not make read lines unread.
func TestSwitchOfLanguageKeepsReadLines(t *testing.T) {
	g := sysTwoLanguages(t, Options{Seed: 2, Location: "camp", Event: "talk", Line: 2})
	sysRunFast(t, g, 3000)
	line := g.talk.say
	g.markRead(line)
	if !g.isRead(line) {
		t.Fatal("not read after it was marked")
	}
	g.cfg.Language = "ja"
	if err := g.useLanguage(); err != nil {
		t.Fatal(err)
	}
	g.talk.reword(g)
	if !g.isRead(line) {
		t.Fatal("a line read in English is unread after the switch to Japanese")
	}
}
