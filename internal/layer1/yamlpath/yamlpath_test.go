package yamlpath

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestRejectMalformedPaths(t *testing.T) {
	for _, path := range []string{"items[", "items[0][", "items[0]0]", ".items", "items.", "items..name"} {
		if _, err := At([]byte("items: [[one]]\n"), path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: want not-exist, got %v", path, err)
		}
	}
}

func TestSelectOrderedFragments(t *testing.T) {
	src := []byte("people:\n  - id: guide\n    lines: [[Hello, Goodbye]]\n  - 7\nsecond: true\n")
	for path, want := range map[string]string{
		"people.guide.lines[0][1]": "Goodbye",
		"people[0].id":             "guide",
		"second":                   "true",
	} {
		got, err := At(src, path)
		if err != nil || strings.TrimSpace(string(got)) != want {
			t.Errorf("%q: got %q, %v; want %q", path, got, err, want)
		}
	}
	whole, err := At(src, "")
	if err != nil || string(whole) != string(src) {
		t.Fatalf("whole document: %q, %v", whole, err)
	}
	whole[0] = 'X'
	if src[0] != 'p' {
		t.Fatal("returned document shares the source")
	}
	for _, path := range []string{"missing", "people.missing", "second.child", "people[-1]", "people[2]", "people[x]", "people[0", "people[]", "people[1].id"} {
		if _, err := At(src, path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: want not-exist, got %v", path, err)
		}
	}
	if _, err := At([]byte("broken: ["), "broken"); err == nil {
		t.Fatal("invalid YAML accepted")
	}
	got, err := At([]byte("[[one]]"), "[0][0]")
	if err != nil || strings.TrimSpace(string(got)) != "one" {
		t.Fatalf("root sequence: %q, %v", got, err)
	}
}
