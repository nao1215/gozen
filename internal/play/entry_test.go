package play

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
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

func TestEntryGameFS(t *testing.T) {
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
}

// TestEntryPreservesReplay checks rejection before the trace destination is opened.
func TestEntryPreservesReplay(t *testing.T) {
	for _, kind := range []string{"same", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "run.jsonl")
			original := []byte("original replay\n")
			if err := os.WriteFile(source, original, 0o600); err != nil {
				t.Fatal(err)
			}
			dest := source
			if kind != "same" {
				dest = filepath.Join(dir, "alias.jsonl")
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(source, dest); err != nil {
					t.Fatal(err)
				}
			}
			err := Main(context.Background(), fstest.MapFS{}, ".", []string{"--dev", "--replay", source, "--trace", dest}, EntryConfig{})
			if err == nil {
				t.Fatal("accepted overlapping replay and trace")
			}
			got, readErr := os.ReadFile(source) //nolint:gosec // Replay fixture belongs to this test.
			if readErr != nil || string(got) != string(original) {
				t.Fatalf("replay changed: %q, %v", got, readErr)
			}
		})
	}
}
