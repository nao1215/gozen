package distribute

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopRejectsInvalidRuntimeWithoutPublishing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		runtime Runtime
		want    string
	}{
		{"version", Runtime{Version: "old", Target: "linux-amd64"}, "need devel"},
		{"target", Runtime{Version: "devel", Target: "windows-amd64"}, "need devel for linux-amd64"},
		{"path", Runtime{Version: "devel", Target: "linux-amd64", Player: "../escape"}, "must be gozen-player"},
		{"checksum", Runtime{Version: "devel", Target: "linux-amd64", Player: "gozen-player", SHA256: "bad"}, "checksum mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle, src := t.TempDir(), t.TempDir()
			b, err := json.Marshal(tc.runtime)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bundle, "runtime.json"), b, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bundle, "gozen-player"), []byte("not a player"), 0o600); err != nil {
				t.Fatal(err)
			}
			parent := t.TempDir()
			dst := filepath.Join(parent, "out")
			err = Desktop(src, dst, bundle, "linux-amd64", "devel")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			entries, err := os.ReadDir(parent)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("partial export: %v", entries)
			}
		})
	}
}

func TestDesktopRejectsOverlapAndPreservesExistingDestination(t *testing.T) {
	src, bundle := t.TempDir(), t.TempDir()
	for _, dst := range []string{filepath.Join(src, "output"), filepath.Join(bundle, "output")} {
		if err := Desktop(src, dst, bundle, "linux-amd64", "devel"); err == nil || !strings.Contains(err.Error(), "separate") {
			t.Fatalf("overlap: %v", err)
		}
	}
	dst := t.TempDir()
	name := filepath.Join(dst, "keep.txt")
	if err := os.WriteFile(name, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Desktop(src, dst, bundle, "linux-amd64", "devel"); err == nil {
		t.Fatal("existing destination accepted")
	}
	b, err := os.ReadFile(name) //nolint:gosec // Test-owned temporary file.
	if err != nil || string(b) != "keep" {
		t.Fatalf("destination changed: %q, %v", b, err)
	}
}
