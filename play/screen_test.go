package play

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen"
)

func TestSplitMakesLists(t *testing.T) {
	cases := map[string][]string{
		"":         nil,
		"a":        {"a"},
		"a,b,c":    {"a", "b", "c"},
		"a,,b":     {"a", "", "b"},
		",a":       {"", "a"},
		"var.x=3,": {"var.x=3", ""},
	}
	for in, want := range cases {
		if got := split(in); !slices.Equal(got, want) {
			t.Errorf("split(%q) = %q, want %q", in, got, want)
		}
	}
}

// brokenGame is a game directory whose definitions cannot load, so a run that gets as far as
// loading it fails at once, without opening a window.
func brokenGame() fstest.MapFS {
	return fstest.MapFS{"g/game.yaml": {Data: []byte("title: [unclosed")}}
}

func TestMainChecksOptionsBeforeRunning(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"review needs a log", []string{"--dev", "--review"}, "--review requires --review-log"},
		{"script needs shots", []string{"--dev", "--script", "s.txt"}, "--script requires --shots"},
		{"a dev option without dev", []string{"--autoplay", "5"}, "--autoplay requires --dev"},
		{"another dev option", []string{"--trace", "t.jsonl"}, "--trace requires --dev"},
		{"extra operands", []string{"stray"}, "unexpected positional arguments"},
		{"answers are numbers", []string{"--dev", "--answers", "0,x"}, `--answers "0,x"`},
		{"missing script", []string{"--dev", "--script", filepath.Join(t.TempDir(), "none.txt"), "--shots", t.TempDir()}, "none.txt"},
	}
	for _, c := range cases {
		err := Main(ctx, fstest.MapFS{"g/game.yaml": {Data: []byte("x")}}, "g", c.args, Config{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
		if c.name == "missing script" && !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v, want a missing file", c.name, err)
		}
	}
	// The unusable game directory comes first, before the options are read further.
	if err := Main(ctx, nil, "g", []string{"--dev", "--answers", "x"}, Config{}); err == nil || strings.Contains(err.Error(), "--answers") {
		t.Fatalf("a missing game: %v", err)
	}
}

func TestMainReportsAGameThatCannotLoad(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "nested", "dir", "run.jsonl")
	var out bytes.Buffer
	err := Main(context.Background(), brokenGame(), "g", []string{"--dev", "--mute", "--seed", "3", "--trace", trace, "--save-dir", t.TempDir()}, Config{Output: &out})
	if err == nil {
		t.Fatal("a broken game ran")
	}
	// The trace file was opened (and its folders made) before the game was loaded, and closed.
	if _, statErr := os.Stat(trace); statErr != nil {
		t.Fatalf("trace file: %v", statErr)
	}
	// Recording goes the same way.
	err = Main(context.Background(), brokenGame(), "g", []string{"--dev", "--record", filepath.Join(t.TempDir(), "x.mp4")}, Config{})
	if err == nil {
		t.Fatal("a broken game was recorded")
	}
	// The --dir option reads a directory instead.
	err = Main(context.Background(), nil, "", []string{"--dir", t.TempDir()}, Config{})
	if err == nil {
		t.Fatal("an empty game directory ran")
	}
}

func TestMainTraceInAnUnwritablePlace(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Main(context.Background(), brokenGame(), "g", []string{"--dev", "--trace", filepath.Join(blocker, "sub", "t.jsonl")}, Config{})
	if err == nil {
		t.Fatal("a trace under a file was accepted")
	}
}

func TestNewScreenRefusesAnythingButARealSession(t *testing.T) {
	if _, err := New(nil, Config{}); err == nil || !strings.Contains(err.Error(), "nil session") {
		t.Fatalf("nil session: %v", err)
	}
	if _, err := New(&gozen.Session{}, Config{}); err == nil || !strings.Contains(err.Error(), "gozen.New") {
		t.Fatalf("a session not made by gozen.New: %v", err)
	}
}

func TestMainRejectsACancelledContextFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := Main(ctx, nil, "invalid", []string{"--help"}, Config{Output: &out}); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatalf("%v, wrote %d bytes", err, out.Len())
	}
}

func TestScreenFollowsTheSession(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "novel")
	if err := os.CopyFS(game, os.DirFS("testdata/novel")); err != nil {
		t.Fatal(err)
	}
	assets := filepath.Join(game, "assets")
	if err := os.MkdirAll(assets, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "regular.ttf"), goregular.TTF, 0o600); err != nil {
		t.Fatal(err)
	}
	fig := image.NewNRGBA(image.Rect(0, 0, 240, 720))
	var pb bytes.Buffer
	if err := png.Encode(&pb, fig); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "fig.png"), pb.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := gozen.Load(os.DirFS(root), "novel")
	if err != nil {
		t.Fatal(err)
	}
	s, err := gozen.New(g, gozen.Config{Seed: 5})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := New(s, Config{Mute: true})
	if err != nil {
		t.Fatal(err)
	}
	w, h := sc.Layout(1920, 1080)
	if w <= 0 || h <= 0 || w*9 != h*16 {
		t.Fatalf("layout %dx%d is not the game's 16:9 screen", w, h)
	}
	for range 30 {
		if err := sc.Update(); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	sc.Draw(ebiten.NewImage(w, h))
	if err := sc.Close(); err != nil {
		t.Fatal(err)
	}
}
