//go:build cgo

package tagger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRefusals(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "libonnxruntime-missing.so")
	if tg, err := New("", lib, 1); err == nil || tg != nil || !strings.Contains(err.Error(), "explicit model directory") {
		t.Fatalf("no directory: %v %v", tg, err)
	}
	if tg, err := New(dir, "", 1); err == nil || tg != nil || !strings.Contains(err.Error(), "explicit model directory") {
		t.Fatalf("no runtime: %v %v", tg, err)
	}
	// The tag list is read before the runtime is loaded.
	if _, err := New(dir, lib, 1); !os.IsNotExist(err) {
		t.Fatalf("missing selected_tags.csv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "selected_tags.csv"), []byte("tag_id,name\n1,a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if tg, err := New(dir, lib, 1); err == nil || tg != nil || !strings.Contains(err.Error(), "cannot load onnxruntime") {
		t.Fatalf("runtime that cannot load: %v %v", tg, err)
	}
}
