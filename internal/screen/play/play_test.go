package play

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/nao1215/gozen/internal/base/savefile"
	data "github.com/nao1215/gozen/internal/model/gamedata"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestStartRenderingLifecycle(t *testing.T) {
	for _, scripted := range []bool{false, true} {
		g := sysInn(t, Options{LiveDir: t.TempDir(), Development: true})
		if scripted {
			g.opts.Script = "press reload"
		}
		g.rendering = true // An offscreen Draw must not suppress loop initialization.
		g.StartRendering()
		if !g.rendering || g.pauseUnfocused == scripted || g.live.stop == nil || !ebiten.IsRunnableOnUnfocused() {
			t.Fatal("render loop initialization missing")
		}
		stop := g.live.stop
		g.StartRendering()
		if g.live.stop != stop {
			t.Fatal("duplicate live watcher")
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		select {
		case <-stop:
		default:
			t.Fatal("Close left live watcher running")
		}
	}
}

func TestNewFailureReleasesRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Options)
		want   error
	}{
		{"regular font missing", func(o *Options) { o.DB.Game.Fonts.Regular = "missing.ttf" }, fs.ErrNotExist},
		{"title font missing", func(o *Options) { o.DB.Game.Fonts.Title = "missing.ttf" }, fs.ErrNotExist},
		{"subtitle font missing", func(o *Options) { o.DB.Game.Fonts.Subtitle = "missing.ttf" }, fs.ErrNotExist},
		{"title font corrupt", func(o *Options) { o.DB.Game.Fonts.Title = "x.png" }, nil},
		{"subtitle font corrupt", func(o *Options) { o.DB.Game.Fonts.Subtitle = "x.png" }, nil},
		{"store factory", func(o *Options) {
			o.Store = nil
			o.StoreFor = func(string) (Store, error) { return nil, fs.ErrPermission }
		}, fs.ErrPermission},
		{"trace writer", func(o *Options) { o.Trace = failingTrace{err: io.ErrClosedPipe} }, io.ErrClosedPipe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := gameFiles(t)
			db, err := data.Load(files)
			if err != nil {
				t.Fatal(err)
			}
			o := Options{FS: files, DB: db, Store: savefile.MemStore(), NoSound: true}
			tc.change(&o)
			g, err := New(o)
			if g != nil {
				if closeErr := g.Close(); closeErr != nil {
					t.Error(closeErr)
				}
				t.Fatal("failed initialization returned a game")
			}
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("missing failure cause: %v", err)
			}
			// A failed constructor must release the exclusive run and leave the
			// shared presentation resources usable by a subsequent game.
			g, err = New(Options{FS: files, Store: savefile.MemStore(), NoSound: true})
			if err != nil {
				t.Fatalf("cannot open after failed initialization: %v", err)
			}
			if err := g.Update(); err != nil {
				t.Error(err)
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
			if err := g.Close(); err != nil {
				t.Fatal("close is not idempotent", err)
			}
		})
	}
}

// Layout must work without a display, including after offscreen drawing. The
// real loop supplies the device scale; scripted layouts deliberately ignore it.
func TestLayoutDeviceScale(t *testing.T) {
	g := sysInn(t, Options{})
	g.db.Game.Screen.Native = true
	g.rendering = true
	if g.deviceScale != nil {
		t.Fatal("New installed a display query before startup")
	}
	w, h := g.Layout(960, 540)
	if w != 960 || h != 540 {
		t.Fatalf("offscreen layout = %dx%d", w, h)
	}
	calls := 0
	g.deviceScale = func() float64 { calls++; return 2 }
	fw, fh := g.LayoutF(960, 540)
	if fw != 1920 || fh != 1080 || calls != 1 {
		t.Fatalf("device layout = %gx%g, queries = %d", fw, fh, calls)
	}
	g.in.script = &script{}
	g.layout(960, 540)
	if calls != 1 {
		t.Fatal("scripted layout queried the display")
	}
}

const sysMusicGame = `music:
  songs:
    town: {file: town.ogg}
    fight: {file: fight.ogg}
    hush: {file: hush.ogg}
  rules:
    field:
      - {when: "flag.alarm", play: fight}
events:
  raise:
    do:
      - set: {flag.alarm: true}
      - say: "Alarm."
  calm:
    do:
      - set: {flag.alarm: false}
      - say: "Calm."
  swap:
    do:
      - music: {song: hush}
      - say: "Hush."
locations:
  hall:
    name: Hall
    music: town
`

// sysMusicGame opens the game with three one-second Ogg songs, recording the trace so that the
// songs are loaded without a sound device.
func sysMusicOpen(t *testing.T, opts Options) *Game {
	t.Helper()
	tone, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	wav := string(tone)
	opts.Trace = &bytes.Buffer{}
	return sysOpen4(t, opts, map[string]string{
		"data/music.yaml":  sysMusicGame,
		"assets/town.ogg":  wav,
		"assets/fight.ogg": wav,
		"assets/hush.ogg":  wav,
	})
}

// sysPlaying runs frames until the module plays song (songs load in the background).
func sysPlaying(t *testing.T, g *Game, song string) {
	t.Helper()
	var got string
	for range 400 {
		sysFrames(t, g, 1)
		if got, _ = g.audioSession().Playing(); got == song {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("playing %q, want %q", got, song)
}

// TestLocationMusicPlaysAndRulesChangeIt: the place's song plays; a rule that holds changes
// to its song and the place's own song comes back when it stops holding.
func TestLocationMusicPlaysAndRulesChangeIt(t *testing.T) {
	covSlow(t)
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall"})
	sysPlaying(t, g, "town")
	if g.baseSong != "town" {
		t.Fatalf("base song %q", g.baseSong)
	}
	g.st.Flags["alarm"] = true
	sysPlaying(t, g, "fight")
	g.st.Flags["alarm"] = false
	sysPlaying(t, g, "town")
}

// TestMusicCommandOverridesThePlace: the event's music command plays that song whatever the
// place sets.
func TestMusicCommandOverridesThePlace(t *testing.T) {
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall", Event: "swap", Line: 1})
	sysPlaying(t, g, "hush")
	if g.songOverride != "hush" || g.baseSong != "hush" {
		t.Fatalf("override %q base %q", g.songOverride, g.baseSong)
	}
	g.Move("camp")
	if g.songOverride != "" {
		t.Fatalf("a move should drop the override: %q", g.songOverride)
	}
}

// TestMoodNeedsASongWithIt: a track has no moods, a request for one is answered false.
func TestMoodNeedsASongWithIt(t *testing.T) {
	g := sysMusicOpen(t, Options{Seed: 1, Location: "hall"})
	if g.hasMood("tense") {
		t.Fatal("a plain track has no mood")
	}
	g.Mood("tense")
	if g.moodOverride != "tense" {
		t.Fatalf("override %q", g.moodOverride)
	}
}

// TestShakenFrameIsKeptAndReleased: a shaking screen draws into one off-screen frame
// while the game keeps its size, a new size replaces it, and closing releases it.
func TestShakenFrameIsKeptAndReleased(t *testing.T) {
	g := openGame(t, Options{Location: "camp"})
	w, h := g.pixels()
	screen := ebiten.NewImage(int(w)+8, int(h)+8)
	defer screen.Deallocate()
	draw := func() *ebiten.Image {
		g.shake = 10
		g.Draw(screen)
		return g.shaken
	}
	first := draw()
	if first == nil || draw() != first {
		t.Fatal("a shaking screen of the same size made a new off-screen frame")
	}
	g.view.w, g.view.h = int(w)+8, int(h)+8
	if resized := draw(); resized == first || resized.Bounds().Dx() != int(w)+8 {
		t.Fatal("a new screen size kept the old off-screen frame")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if g.shaken != nil {
		t.Fatal("closing the game kept the off-screen frame")
	}
}

// BenchmarkShakenDraw measures the CPU side of a frame drawn while the screen shakes
// (a battle hit), which draws the frame into an off-screen image first.
func BenchmarkShakenDraw(b *testing.B) {
	g := openGame(b, Options{Location: "camp"})
	w, h := g.pixels()
	screen := ebiten.NewImage(int(w), int(h))
	defer screen.Deallocate()
	b.ReportAllocs()
	for b.Loop() {
		g.shake = 10
		g.Draw(screen)
	}
}
