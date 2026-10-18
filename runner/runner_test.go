package runner

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }
func TestHelpAndErrorsDoNotExit(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), nil, "invalid", Config{Args: []string{"--help"}, Output: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Requirements:") {
		t.Fatal("requirements missing")
	}
	want := errors.New("output unavailable")
	if err := Run(context.Background(), nil, ".", Config{Args: []string{"--help"}, Output: failingWriter{want}}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--unknown"}, {"--start", "camp"}, {"--dev", "--script", "missing"}, {"extra"}} {
		if err := Run(context.Background(), nil, ".", Config{Args: args}); err == nil {
			t.Fatal(args)
		}
	}
}
func TestEmbeddedRootAndCancellation(t *testing.T) {
	source := fstest.MapFS{"game/game.yaml": {Data: []byte("neutral")}}
	sub, err := gameFS(source, "game", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(sub, "game.yaml")
	if err != nil || string(b) != "neutral" {
		t.Fatalf("sub FS: %q %v", b, err)
	}
	if _, err := gameFS(nil, ".", ""); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, nil, ".", Config{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
