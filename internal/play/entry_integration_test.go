package play_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/play"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
func TestHelpAndErrorsDoNotExit(t *testing.T) {
	var out bytes.Buffer
	if err := play.Main(context.Background(), nil, "invalid", []string{"--help"}, play.EntryConfig{Output: &out}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Requirements:", "[Usage]", "[Examples]", "[Options]", "  Test play (--dev):", "-h, --help",
		"Report an issue: https://github.com/nao1215/gozen/issues",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help lacks %q", want)
		}
	}
	if strings.Contains(out.String(), "  Other:") {
		t.Fatal("an option is missing from the help groups")
	}
	var short bytes.Buffer
	if err := play.Main(context.Background(), nil, "invalid", []string{"-h"}, play.EntryConfig{Output: &short}); err != nil || short.String() != out.String() {
		t.Fatalf("-h differs from --help: %v", err)
	}
	want := errors.New("output unavailable")
	if err := play.Main(context.Background(), nil, ".", []string{"--help"}, play.EntryConfig{Output: failingWriter{want}}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-dev"}, {"--dev", "-seed", "7"}, {"-mute"}} {
		err := play.Main(context.Background(), nil, ".", args, play.EntryConfig{})
		if err == nil || !strings.Contains(err.Error(), "single-dash long option") {
			t.Fatalf("%v: %v", args, err)
		}
	}
	// Both GNU value forms reach the same check after parsing.
	for _, args := range [][]string{{"--seed=7", "--start=camp"}, {"--seed", "7", "--start", "camp"}} {
		err := play.Main(context.Background(), nil, ".", args, play.EntryConfig{})
		if err == nil || err.Error() != "--start requires --dev" {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"--unknown"}, {"--start", "camp"}, {"--dev", "--script", "missing"}, {"extra"}} {
		if err := play.Main(context.Background(), nil, ".", args, play.EntryConfig{}); err == nil {
			t.Fatal(args)
		}
	}
}

func TestEmbeddedRootAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := play.Main(ctx, nil, ".", nil, play.EntryConfig{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
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
		err := play.Main(ctx, fstest.MapFS{"g/game.yaml": {Data: []byte("x")}}, "g", c.args, play.EntryConfig{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
		if c.name == "missing script" && !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s: %v, want a missing file", c.name, err)
		}
	}
	// The unusable game directory comes first, before the options are read further.
	if err := play.Main(ctx, nil, "g", []string{"--dev", "--answers", "x"}, play.EntryConfig{}); err == nil || strings.Contains(err.Error(), "--answers") {
		t.Fatalf("a missing game: %v", err)
	}
}

func TestMainReportsAGameThatCannotLoad(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "nested", "dir", "run.jsonl")
	var out bytes.Buffer
	err := play.Main(context.Background(), brokenGame(), "g", []string{"--dev", "--mute", "--seed", "3", "--trace", trace, "--save-dir", t.TempDir()}, play.EntryConfig{Output: &out})
	if err == nil {
		t.Fatal("a broken game ran")
	}
	// The trace file was opened (and its folders made) before the game was loaded, and closed.
	if _, statErr := os.Stat(trace); statErr != nil {
		t.Fatalf("trace file: %v", statErr)
	}
	// Recording goes the same way.
	err = play.Main(context.Background(), brokenGame(), "g", []string{"--dev", "--record", filepath.Join(t.TempDir(), "x.mp4")}, play.EntryConfig{})
	if err == nil {
		t.Fatal("a broken game was recorded")
	}
	// The --dir option reads a directory instead.
	err = play.Main(context.Background(), nil, "", []string{"--dir", t.TempDir()}, play.EntryConfig{})
	if err == nil {
		t.Fatal("an empty game directory ran")
	}
}

func TestMainTraceInAnUnwritablePlace(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := play.Main(context.Background(), brokenGame(), "g", []string{"--dev", "--trace", filepath.Join(blocker, "sub", "t.jsonl")}, play.EntryConfig{})
	if err == nil {
		t.Fatal("a trace under a file was accepted")
	}
}

func TestMainRejectsACancelledContextFirst(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if err := play.Main(ctx, nil, "invalid", []string{"--help"}, play.EntryConfig{Output: &out}); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatalf("%v, wrote %d bytes", err, out.Len())
	}
}
