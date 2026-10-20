package chat

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// Script.YAML and Parse: read a script built by the game and return part of it as YAML.
func TestScriptYAML(t *testing.T) {
	s, err := Parse([]byte(script))
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.YAML("lines.hype")
	if err != nil || !strings.Contains(string(got), "888") {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := s.YAML("lines.none"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := Parse([]byte("names: []\nrate: 1\n")); err == nil {
		t.Fatal("an invalid script was accepted")
	}
}
