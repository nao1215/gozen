package play

import (
	"errors"
	"io"
	"io/fs"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/savefile"

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
