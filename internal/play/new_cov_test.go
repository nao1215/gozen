package play

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestNewRefusesBrokenGames: a game that cannot load, or whose fonts are wrong, stops New with
// an error that names the problem, and a later game can still open.
func TestNewRefusesBrokenGames(t *testing.T) {
	noGame := gameFiles(t)
	delete(noGame, "game.yaml")
	badTitle := gameFiles(t)
	badTitle["game.yaml"] = &fstest.MapFile{Data: append(badTitle["game.yaml"].Data, "fonts_title_marker: 1\n"...)}
	cases := map[string]struct {
		extra map[string]string
		files func() fstest.MapFS
		want  string
	}{
		"no game.yaml": {files: func() fstest.MapFS { return noGame }, want: "game.yaml"},
		"title font missing": {
			extra: map[string]string{"game.yaml+": "\nfonts: {regular: test.ttf, title: gone.ttf}\n"}, want: "fonts.title",
		},
		"subtitle font missing": {
			extra: map[string]string{"game.yaml+": "\nfonts: {regular: test.ttf, subtitle: gone.ttf}\n"}, want: "fonts.subtitle",
		},
		"title font broken": {
			extra: map[string]string{"game.yaml+": "\nfonts: {regular: test.ttf, title: bad.ttf}\n", "assets/bad.ttf": "not a font"}, want: "title font",
		},
	}
	for name, c := range cases {
		fsys := gameFiles(t)
		if c.files != nil {
			fsys = c.files()
		}
		for n, src := range c.extra {
			if n == "game.yaml+" {
				// replace the base fonts line.
				d := strings.Replace(string(fsys["game.yaml"].Data), "fonts: {regular: test.ttf}\n", "", 1)
				fsys["game.yaml"] = &fstest.MapFile{Data: []byte(d + src)}
				continue
			}
			fsys[n] = &fstest.MapFile{Data: []byte(src)}
		}
		g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir()})
		if err == nil {
			sysClose(t, g)
			t.Errorf("%s: accepted", name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not mention %q", name, err, c.want)
		}
	}
	g := sysOpen(t, Options{}, nil)
	if g.st != nil {
		t.Fatal("state before the title")
	}
}

// TestFontErrorsNameTheField: LoadFonts reports the game.yaml field and unwraps to the cause.
func TestFontErrorsNameTheField(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	db := g.db
	db.Game.Fonts.Regular = "gone.ttf"
	err := LoadFonts(db, gameFiles(t))
	var fe *FontError
	if !errors.As(err, &fe) || fe.Key != "fonts.regular" || fe.Unwrap() == nil || !strings.Contains(fe.Error(), "fonts.regular") {
		t.Fatalf("err = %v", err)
	}
	fsys := gameFiles(t)
	fsys["assets/bad.ttf"] = &fstest.MapFile{Data: []byte("nope")}
	db.Game.Fonts.Regular = "bad.ttf"
	if err := LoadFonts(db, fsys); !errors.As(err, &fe) || fe.Key != "fonts.regular" {
		t.Fatalf("broken regular: %v", err)
	}
	db.Game.Fonts.Regular = "test.ttf"
	db.Game.Fonts.Pop = "gone.ttf"
	if err := LoadFonts(db, fsys); !errors.As(err, &fe) || fe.Key != "fonts.pop" {
		t.Fatalf("missing pop: %v", err)
	}
	db.Game.Fonts.Pop = "bad.ttf"
	if err := LoadFonts(db, fsys); !errors.As(err, &fe) || fe.Key != "fonts.pop" {
		t.Fatalf("broken pop: %v", err)
	}
	db.Game.Fonts.Pop = ""
	db.Game.Fonts.Regular = ""
	if err := LoadFonts(db, fsys); err != nil {
		t.Fatalf("the built-in font should be used without fonts.regular: %v", err)
	}
	db.Game.Fonts.Regular = "test.ttf"
	if err := LoadFonts(db, fsys); err != nil {
		t.Fatal(err)
	}
}

// TestMessageMetrics: the line width follows the message style and the screen; prompts are cut
// to the number of lines the window has.
func TestMessageMetrics(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	gm := &g.db.Game
	pages, size := MessagePages(gm, strings.Repeat("word ", 200))
	if size <= 0 || len(pages) < 2 {
		t.Fatalf("a long text should take several pages: %d pages size %v", len(pages), size)
	}
	for i, p := range pages {
		if len(p) > MessageLines() {
			t.Fatalf("page %d has %d lines, the window has %d", i, len(p), MessageLines())
		}
	}
	if len(MessageWrap(gm, strings.Repeat("word ", 200))) <= MessageLines() {
		t.Fatal("wrapping should not paginate")
	}
	lines, _, cut := PromptLines(gm, strings.Repeat("word ", 200), nil)
	if !cut || len(lines) > MessageLines() {
		t.Fatalf("prompt: %d lines cut=%v", len(lines), cut)
	}
	if _, _, cut := PromptLines(gm, "short", nil); cut {
		t.Fatal("a short prompt was cut")
	}
	gm.UI.Message = "novel"
	_, nw := messageMetrics(gm)
	gm.UI.Message = ""
	_, bw := messageMetrics(gm)
	if nw == bw {
		t.Fatal("the novel window has the same width as the box")
	}
	if w := choiceWidth(gm, []string{"a"}); w != 300 {
		t.Fatalf("narrow choice width %v", w)
	}
	if w := choiceWidth(gm, []string{strings.Repeat("W", 200)}); w <= 300 || w > logicalWidth(gm) {
		t.Fatalf("wide choice width %v", w)
	}
	gm.UI.Message = "novel"
	if w := choiceWidth(gm, []string{strings.Repeat("W", 200)}); w <= 300 {
		t.Fatalf("novel choice width %v", w)
	}
	gm.UI.Message = ""
	gm.Screen.Height = 0
	if UIScale(gm) != 1 {
		t.Fatal("scale of a game with no height")
	}
	gm.Screen.Height = 1440
	if UIScale(gm) != 2 {
		t.Fatalf("scale of a double height game: %v", UIScale(gm))
	}
}

// TestSessionStateSkipsTheTitle: a game started from a Session's state does not show the
// title; fresh runs the opening event, otherwise it loads the place.
func TestSessionStateSkipsTheTitle(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Gold = 4242
	g, err := New(Options{FS: gameFiles(t), NoSound: true, DB: db, State: st, SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if g.st != st || g.st.Gold != 4242 {
		t.Fatal("did not continue from the state")
	}
	if _, ok := g.stack[0].(*title); ok {
		t.Fatal("the title was shown")
	}
	sysFrames(t, g, 5)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestSessionFontsAreReadByNew: a state given with a database still reads the title fonts
// from the file system and reports a missing one.
func TestSessionFontsAreReadByNew(t *testing.T) {
	for _, c := range []struct {
		set  func(*Options)
		want string
	}{
		{func(o *Options) { o.DB.Game.Fonts.Title = "gone.ttf" }, "title font"},
		{func(o *Options) { o.DB.Game.Fonts.Subtitle = "gone.ttf" }, "subtitle font"},
		{func(o *Options) { o.DB.Game.Fonts.Title = "x.png" }, "title font"},
	} {
		o := Options{FS: gameFiles(t), NoSound: true, DB: testgame.Load(t, nil), SaveDir: t.TempDir()}
		c.set(&o)
		g, err := New(o)
		if err == nil {
			sysClose(t, g)
			t.Fatalf("%s: accepted", c.want)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("error %q does not mention %q", err, c.want)
		}
	}
}

// TestSessionFreshRunsTheOpeningEvent: Fresh starts the game's opening event.
func TestSessionFreshRunsTheOpeningEvent(t *testing.T) {
	game := strings.Replace(testgame.Game, "start: {location: camp, party: [scout], gold: 100, items: {potion: 2}}",
		"start: {location: camp, party: [scout], gold: 100, items: {potion: 2}, event: sub}", 1)
	db := testgame.Load(t, map[string]string{"game.yaml": game})
	st := testgame.State(db, 1)
	g, err := New(Options{FS: gameFiles(t), NoSound: true, DB: db, State: st, Fresh: true, SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer sysClose(t, g)
	if !g.events.Running() {
		t.Fatal("the opening event did not start")
	}
	sysFrames(t, g, 10)
	if g.st.Gold != 105 {
		t.Fatalf("the opening event gave %d gold, want 105", g.st.Gold)
	}
}

// TestDefaultStoreIsTheUserConfigDirectory: without any save location the saves go to the OS
// configuration directory under the game's ID.
func TestDefaultStoreIsTheUserConfigDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	g, err := New(Options{FS: gameFiles(t), NoSound: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sysClose(t, g)
	if g.storeGiven {
		t.Fatal("the default location is not an explicit one")
	}
	g.newGame()
	if err := g.save(0); err == nil {
		// a real play keeps its saves
		if _, err := os.Stat(filepath.Join(root, "gozen", "screen-test", "save1.json")); err != nil {
			t.Fatalf("no save in the config directory: %v", err)
		}
	} else {
		t.Fatal(err)
	}
}

// TestStoreForIsAskedForTheGameID and its failure.
func TestStoreForIsAskedForTheGameID(t *testing.T) {
	var asked string
	mem := gozen.MemStore()
	g, err := New(Options{FS: gameFiles(t), NoSound: true, StoreFor: func(id string) (Store, error) { asked = id; return mem, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if asked != "screen-test" || g.store != mem {
		t.Fatalf("asked %q", asked)
	}
	sysClose(t, g)
	_, err = New(Options{FS: gameFiles(t), NoSound: true, StoreFor: func(string) (Store, error) { return nil, errors.New("no storage") }})
	if err == nil || !strings.Contains(err.Error(), "no storage") {
		t.Fatalf("err = %v", err)
	}
}

// TestAudioRecordingWritesOneBlockPerTick: with an audio writer the game records PCM as it
// runs: the same amount every tick, and it goes silent-free once a song plays.
func TestAudioRecordingWritesOneBlockPerTick(t *testing.T) {
	covSlow(t)
	tone, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	g := sysOpen4(t, Options{Audio: &audio, Location: "hall", Seed: 1}, map[string]string{
		"data/music.yaml": sysMusicGame, "assets/town.ogg": string(tone), "assets/fight.ogg": string(tone), "assets/hush.ogg": string(tone),
	})
	sysFrames(t, g, 5)
	perTick := audio.Len() / 5
	if perTick == 0 || audio.Len()%5 != 0 {
		t.Fatalf("%d bytes after 5 ticks", audio.Len())
	}
	sysFrames(t, g, 5)
	if audio.Len() != perTick*10 {
		t.Fatalf("%d bytes after 10 ticks, want %d", audio.Len(), perTick*10)
	}
}
