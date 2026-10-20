package play

import (
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
