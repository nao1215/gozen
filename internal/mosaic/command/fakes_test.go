package command

import (
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/mosaic/segment"
)

// fakeDetector stands in for an ONNX detection model: it answers every picture with the
// same boxes, and counts its calls.
type fakeDetector struct {
	boxes    []detect.Box
	err      error
	calls    int
	closed   int
	closeErr error
	passes   []detect.Pass
}

func (f *fakeDetector) Detect(_ image.Image, passes []detect.Pass, _ float32) ([]detect.Box, error) {
	f.calls++
	f.passes = passes
	if f.err != nil {
		return nil, f.err
	}
	return slices.Clone(f.boxes), nil
}

func (f *fakeDetector) Close() error { f.closed++; return f.closeErr }

// fakeSegmenter stands in for a segmentation model.
type fakeSegmenter struct {
	objects []segment.Object
	// sequence, when given, answers the first calls one by one; later calls get objects.
	sequence [][]segment.Object
	err      error
	calls    int
	closed   int
}

func (f *fakeSegmenter) Find(_ image.Image, _ float32, _ bool) ([]segment.Object, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.calls <= len(f.sequence) {
		return slices.Clone(f.sequence[f.calls-1]), nil
	}
	return slices.Clone(f.objects), nil
}

func (f *fakeSegmenter) Close() error { f.closed++; return nil }

// fakeTagger stands in for a tagger model.
type fakeTagger struct {
	tags   map[string]float32
	err    error
	calls  int
	closed int
}

func (f *fakeTagger) Tags(image.Image) (map[string]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.tags, nil
}

func (f *fakeTagger) Close() error { f.closed++; return nil }

// models puts fake models behind the file names the command is given; the hooks are
// restored when the test ends.
type fakeModels struct {
	detectors map[string]*fakeDetector
	seg       *fakeSegmenter
	tagger    *fakeTagger
	opened    []string
}

func useModels(t *testing.T) *fakeModels {
	t.Helper()
	m := &fakeModels{detectors: map[string]*fakeDetector{}}
	oldD, oldS, oldT := newDetector, newSegmenter, newTagger
	t.Cleanup(func() { newDetector, newSegmenter, newTagger = oldD, oldS, oldT })
	newDetector = func(model, _ string, _ int, _ []string) (boxDetector, error) {
		m.opened = append(m.opened, model)
		d, ok := m.detectors[filepath.Base(model)]
		if !ok {
			return nil, errors.New("fake: no such model " + model)
		}
		return d, nil
	}
	newSegmenter = func(model, _ string, _, _ int, _ []string) (segmenter, error) {
		m.opened = append(m.opened, model)
		if m.seg == nil {
			return nil, errors.New("fake: no segmentation model")
		}
		return m.seg, nil
	}
	newTagger = func(dir, _ string, _ int) (tagModel, error) {
		m.opened = append(m.opened, dir)
		if m.tagger == nil {
			return nil, errors.New("fake: no tagger")
		}
		return m.tagger, nil
	}
	return m
}

// detector registers a fake detection model under a file name and returns the path to give
// --model (the file exists, so the command finds it).
func (m *fakeModels) detector(t *testing.T, name string, boxes ...detect.Box) (string, *fakeDetector) {
	t.Helper()
	d := &fakeDetector{boxes: boxes}
	m.detectors[name] = d
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("not a model"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p, d
}

func penis(x0, y0, x1, y1 int, score float32) detect.Box {
	return detect.Box{Rect: image.Rect(x0, y0, x1, y1), Label: "penis", Score: score, Pass: "p"}
}

func pussy(x0, y0, x1, y1 int, score float32) detect.Box {
	return detect.Box{Rect: image.Rect(x0, y0, x1, y1), Label: "pussy", Score: score, Pass: "p"}
}

// readEntries reads the report the command wrote.
func readEntries(t *testing.T, out string) []entry {
	t.Helper()
	var es []entry
	if err := json.Unmarshal(readFile(t, filepath.Join(out, "report.json")), &es); err != nil {
		t.Fatal(err)
	}
	return es
}

func loadPNG(t *testing.T, path string) image.Image {
	t.Helper()
	img, err := decode(path)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// samePixels reports whether two pictures have the same pixels over r.
func samePixels(a, b image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if color.RGBAModel.Convert(a.At(x, y)) != color.RGBAModel.Convert(b.At(x, y)) {
				return false
			}
		}
	}
	return true
}

// readFile reads a file the test knows about.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // The tests read files they made in t.TempDir.
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// readDir lists a directory the test knows about.
func readDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	es, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	return es
}
