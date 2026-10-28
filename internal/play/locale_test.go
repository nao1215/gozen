package play

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	lang "github.com/nao1215/gozen/internal/text"
)

// TestFirstLanguage: the first of the device's languages the game offers, a region or a
// script matching its language ("en-US" is "en"), none when the game offers none of them.
func TestFirstLanguage(t *testing.T) {
	offered := []string{"ja", "en"}
	for _, c := range []struct {
		prefs []string
		want  string
	}{
		{[]string{"en-US", "ja"}, "en"},
		{[]string{"EN_gb"}, "en"},
		{[]string{"fr-FR", "ja-JP", "en"}, "ja"},
		{[]string{"zh-Hant-TW"}, ""},
		{[]string{"", "fr"}, ""},
		{nil, ""},
	} {
		if got := firstLanguage(c.prefs, offered); got != c.want {
			t.Errorf("firstLanguage(%q) = %q, want %q", c.prefs, got, c.want)
		}
	}
	if got := firstLanguage([]string{"pt-BR", "pt"}, []string{"pt", "pt-BR"}); got != "pt-BR" {
		t.Errorf("the language named exactly comes before its first part: %q", got)
	}
}

// TestLocaleTag: a POSIX locale as a language tag; C and POSIX name no language.
func TestLocaleTag(t *testing.T) {
	for in, want := range map[string]string{
		"en_US.UTF-8": "en-US", "ja_JP.eucJP@euro": "ja-JP", "de": "de", "C": "", "POSIX": "", "C.UTF-8": "", "": "",
	} {
		if got := localeTag(in); got != want {
			t.Errorf("localeTag(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEnvLocales: LC_ALL, then LC_MESSAGES, then LANG, the empty ones and C left out.
func TestEnvLocales(t *testing.T) {
	env := map[string]string{"LC_ALL": "", "LC_MESSAGES": "C", "LANG": "en_US.UTF-8"}
	if got := envLocales(func(k string) string { return env[k] }); len(got) != 1 || got[0] != "en-US" {
		t.Fatalf("locales %q", got)
	}
	env["LC_ALL"] = "fr_FR.UTF-8"
	if got := envLocales(func(k string) string { return env[k] }); len(got) != 2 || got[0] != "fr-FR" || got[1] != "en-US" {
		t.Fatalf("locales %q", got)
	}
}

// TestAppleLanguages: the languages of `defaults read -g AppleLanguages`.
func TestAppleLanguages(t *testing.T) {
	got := appleLanguages("(\n    \"en-US\",\n    \"ja-JP\"\n)\n")
	if len(got) != 2 || got[0] != "en-US" || got[1] != "ja-JP" {
		t.Fatalf("languages %q", got)
	}
	if got := appleLanguages(""); len(got) != 0 {
		t.Fatalf("languages %q", got)
	}
}

// TestFirstRunTakesTheDeviceLanguage: a game offering two languages, opened for the first
// time, is shown in the first of the device's languages it offers; the language kept in
// the settings comes before it, and a game of one language keeps game.yaml's.
func TestFirstRunTakesTheDeviceLanguage(t *testing.T) {
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	shown := func(opts Options, files map[string]string) string {
		t.Helper()
		g := sysOpen4(t, opts, files)
		defer func() {
			if err := g.Close(); err != nil {
				t.Error(err)
			}
		}()
		return g.language()
	}
	// two is a game offering English and Japanese (a map a run may change, so new each time)
	two := func() map[string]string {
		return map[string]string{"game.yaml+": "written_in: en\nlanguages: [en, ja]\n", "lang/ja.yaml": twoLanguageWords}
	}
	if l := shown(Options{SaveDir: t.TempDir(), Locales: []string{"fr-FR", "ja-JP"}}, two()); l != "ja" {
		t.Fatalf("a first run in %s", l)
	}
	if l := shown(Options{SaveDir: t.TempDir(), Locales: []string{"fr-FR"}}, two()); l != "en" {
		t.Fatalf("a device language not offered: %s", l)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"language": "en"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if l := shown(Options{SaveDir: dir, Locales: []string{"ja-JP"}}, two()); l != "en" {
		t.Fatalf("the language kept in the settings gives way: %s", l)
	}
	if l := shown(Options{SaveDir: t.TempDir(), Locales: []string{"ja"}}, nil); l != "en" {
		t.Fatalf("a game of one language in %s", l)
	}
}

// TestStreamsNameBoxSwitchesLanguage: while the name is asked, the bar's languages are
// pressed: the box is shown in the language at once, the name being typed stays, and the
// box stays open.
func TestStreamsNameBoxSwitchesLanguage(t *testing.T) {
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	g := wldOpenWith(t, Options{FS: streamsFS(t, "0"), Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	l, ok := g.stack[0].(*streamLobby)
	if !ok || l.name == nil {
		t.Fatalf("scene %T without the name box", g.stack[0])
	}
	wldPlay(t, g, "type Pa\nwait 5\n", 100)
	if lang.T("あなたの名前") != "Your name" {
		t.Fatalf("the box in %s: %q", g.language(), lang.T("あなたの名前"))
	}
	bs := g.barButtons(l.entries(g))
	ja := -1
	for i, b := range bs {
		if b.lang == "ja" {
			ja = i
		}
	}
	g.in.click, g.in.mouse = true, image.Pt(int(bs[ja].x+5), int(bs[ja].y+10))
	l.update(g)
	g.in.click = false
	if g.language() != "ja" || lang.T("あなたの名前") != "あなたの名前" {
		t.Fatalf("the bar's language not taken under the name box: %s", g.language())
	}
	if l.name == nil || l.name.text != "Pa" || g.st.Streams.Viewer != "" {
		t.Fatalf("the name box after the switch: open %v, typed %q, viewer %q", l.name != nil, l.name.text, g.st.Streams.Viewer)
	}
}
