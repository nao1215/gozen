package play

import (
	"errors"
	"io/fs"
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
