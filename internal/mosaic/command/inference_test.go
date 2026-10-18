//go:build cgo

package command

import (
	"image"
	"os"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/mosaic/segment"
	"github.com/nao1215/gozen/internal/mosaic/tagger"
)

// These tests use only generated pixels. A missing environment setting skips;
// a configured but unreadable/incompatible model fails the test.
func TestONNXDetection(t *testing.T) {
	model, lib := inferenceInputs(t, "GOZEN_MOSAIC_MODEL")
	d, err := detect.New(model, lib, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = d.Detect(image.NewRGBA(image.Rect(0, 0, 32, 32)), detect.DefaultPasses[:1], 0.5); err != nil {
		t.Fatal(err)
	}
}
func TestONNXSegmentation(t *testing.T) {
	model, lib := inferenceInputs(t, "GOZEN_MOSAIC_SEG_MODEL")
	d, err := segment.NewYoloSeg(model, lib, 1, 1280, segment.SegLabels)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = d.Find(image.NewRGBA(image.Rect(0, 0, 32, 32)), 0.5, false); err != nil {
		t.Fatal(err)
	}
}
func TestONNXTagging(t *testing.T) {
	dir, lib := inferenceInputs(t, "GOZEN_MOSAIC_TAGGER_DIR")
	d, err := tagger.New(dir, lib, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = d.Tags(image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
}
func inferenceInputs(t *testing.T, key string) (string, string) {
	t.Helper()
	model, lib := os.Getenv(key), os.Getenv("ONNXRUNTIME_LIB")
	if model == "" || lib == "" {
		t.Skipf("ONNX runtime verification requires %s and ONNXRUNTIME_LIB; models and runtime are not bundled", key)
	}
	return model, lib
}
