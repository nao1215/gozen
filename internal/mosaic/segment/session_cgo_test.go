//go:build cgo

package segment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewYoloSegRefusals(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "seg.onnx")
	if err := os.WriteFile(model, []byte("not a model"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "libonnxruntime-missing.so")
	labels := []string{"a"}
	for name, c := range map[string]struct {
		model, lib string
		size       int
		labels     []string
		want       string
	}{
		"no model":      {"", lib, 640, labels, "explicit model/runtime"},
		"no runtime":    {model, "", 640, labels, "explicit model/runtime"},
		"size zero":     {model, lib, 0, labels, "input size in 1..4096"},
		"size too big":  {model, lib, 4097, labels, "input size in 1..4096"},
		"no labels":     {model, lib, 640, nil, "labels"},
		"runtime fails": {model, lib, 640, labels, "cannot load onnxruntime"},
	} {
		y, err := NewYoloSeg(c.model, c.lib, 1, c.size, c.labels)
		if err == nil || y != nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v %v, want an error with %q", name, y, err, c.want)
		}
	}
}
