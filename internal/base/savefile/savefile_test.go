package savefile

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReservedName: the device names of Windows in any case, and nothing longer or shorter.
func TestReservedName(t *testing.T) {
	for name, want := range map[string]bool{
		"CON": true, "con": true, "Prn": true, "aux": true, "NUL": true, "com1": true, "LPT9": true,
		"": false, "com0": false, "com10": false, "lpt": false, "console": false, "nul1": false,
	} {
		if got := ReservedName(name); got != want {
			t.Errorf("ReservedName(%q) = %v", name, got)
		}
	}
}

// TestStage: a staged file holds the bytes under a private mode until it is published or
// unstaged, and a failed stage leaves no file behind.
func TestStage(t *testing.T) {
	dir := t.TempDir()
	name, err := Stage(dir, ".stage-*.tmp", []byte("bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(name); err != nil || string(b) != "bytes" || filepath.Dir(name) != dir { //nolint:gosec // A file of this test.
		t.Fatalf("staged %q: %q %v", name, b, err)
	}
	if err := Unstage(name); err != nil {
		t.Fatal(err)
	}
	if err := Unstage(name); err != nil {
		t.Fatalf("unstaging a file already gone: %v", err)
	}
	if _, err := Stage(filepath.Join(dir, "missing"), ".stage-*.tmp", nil); err == nil {
		t.Fatal("a stage in a missing directory succeeded")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("files left behind: %v %v", entries, err)
	}
}
