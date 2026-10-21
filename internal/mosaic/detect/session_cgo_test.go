//go:build cgo

package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These check what New refuses before any model is read. They need no ONNX Runtime: the
// library given is one that does not exist.

func TestNewWithLabelsRefusals(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(model, []byte("not a model"), 0o600); err != nil {
		t.Fatal(err)
	}
	missingLib := filepath.Join(dir, "libonnxruntime-missing.so")
	cases := []struct {
		name, model, lib, want string
	}{
		{"no model", "", missingLib, "--model"},
		{"model not there", filepath.Join(dir, "none.onnx"), missingLib, "detection model"},
		{"no runtime named", model, "", "--ort"},
		{"runtime not there", model, missingLib, "cannot load onnxruntime"},
	}
	for _, c := range cases {
		d, err := NewWithLabels(c.model, c.lib, 1, Labels)
		if err == nil || d != nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v %v, want an error with %q", c.name, d, err, c.want)
		}
	}
	if d, err := New("", "", 0); err == nil || d != nil {
		t.Fatalf("New without a model: %v %v", d, err)
	}
	if names := metadataClasses(model, missingLib); names != nil {
		t.Fatalf("metadata read without a runtime: %v", names)
	}
	// With a runtime that cannot load, class names fall back instead of failing.
	names, src, err := ResolveClasses(model, missingLib, []string{"z"})
	if err != nil || src != SourceDefault || len(names) != 1 || names[0] != "z" {
		t.Fatalf("fallback: %v %s %v", names, src, err)
	}
}
