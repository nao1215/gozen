package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A rejected option leaves no --arrival-saves directory behind, so the
// corrected command is not refused with "output already exists".
func TestSimDiveArrivalSavesAppearOnlyAfterTheRun(t *testing.T) {
	dir := fixture(t)
	saves := filepath.Join(t.TempDir(), "saves")
	invoke(t, 2, "sim", "dive", dir, "--location", "grotto", "--floors", "1", "--arrival-saves", saves, "--shops", "nosuch")
	if _, err := os.Stat(saves); err == nil {
		t.Errorf("a rejected command left %s behind", saves)
	}
	invoke(t, 0, "sim", "dive", dir, "--location", "grotto", "--floors", "1", "--battles", "1", "--tries", "1", "--arrival-saves", saves)
	if _, err := os.Stat(saves); err != nil {
		t.Errorf("a successful run publishes the directory: %v", err)
	}
}

// Lists share one grammar: spaces around an element are trimmed for every option.
func TestSimDiveListsTrimSpaces(t *testing.T) {
	dir := fixture(t)
	_, stderr := invoke(t, 2, "sim", "dive", dir, "--location", "grotto", "--shops", "nosuch , other")
	if want := `unknown shop "nosuch"`; !strings.Contains(stderr, want) {
		t.Errorf("stderr %q lacks %q", stderr, want)
	}
}

// A location nothing can be fought at is refused instead of printing a table of zeros.
func TestSimDiveRefusesALocationWithoutEncounters(t *testing.T) {
	dir := fixture(t)
	write(t, filepath.Join(dir, "data", "quiet.yaml"), "locations:\n  quiet: {name: Quiet}\n")
	_, stderr := invoke(t, 2, "sim", "dive", dir, "--location", "quiet", "--floors", "1")
	if want := "has no encounters"; !strings.Contains(stderr, want) {
		t.Errorf("stderr %q lacks %q", stderr, want)
	}
}
