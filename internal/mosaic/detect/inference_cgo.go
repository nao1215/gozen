//go:build cgo

package detect

import (
	"errors"
	"fmt"
	"image"
	"os"

	ort "github.com/yalue/onnxruntime_go"
)

type Detector struct {
	sess   *ort.DynamicAdvancedSession
	labels []string
}

func New(model, lib string, threads int) (*Detector, error) {
	return NewWithLabels(model, lib, threads, Labels)
}

func NewWithLabels(model, lib string, threads int, labels []string) (result *Detector, err error) {
	if model == "" {
		return nil, errors.New("missing --model: supply a detection ONNX model")
	}
	if _, err := os.Stat(model); err != nil {
		return nil, fmt.Errorf("detection model: %w", err)
	}
	if lib == "" {
		return nil, errors.New("missing --ort or ONNXRUNTIME_LIB: supply the ONNX Runtime shared library")
	}
	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(lib)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("cannot load onnxruntime (%s): %w", lib, err)
		}
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, opts.Destroy()) }()
	if threads > 0 {
		if err := opts.SetIntraOpNumThreads(threads); err != nil {
			return nil, err
		}
	}
	s, err := ort.NewDynamicAdvancedSession(model, []string{"images"}, []string{"output0"}, opts)
	if err != nil {
		return nil, fmt.Errorf("cannot load the model %s: %w", model, err)
	}
	return &Detector{sess: s, labels: labels}, nil
}

// metadataClasses reads the names entry of the ONNX metadata; nil means there
// is none or it cannot be read.
func metadataClasses(model, lib string) []string {
	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(lib)
		if ort.InitializeEnvironment() != nil {
			return nil
		}
	}
	s, err := ort.NewDynamicAdvancedSession(model, []string{"images"}, []string{"output0"}, nil)
	if err != nil {
		return nil
	}
	defer s.Destroy() //nolint:errcheck // read-only probe
	m, err := s.GetModelMetadata()
	if err != nil {
		return nil
	}
	defer m.Destroy() //nolint:errcheck // read-only probe
	v, ok, err := m.LookupCustomMetadataMap("names")
	if err != nil || !ok {
		return nil
	}
	names, err := ParseNamesMetadata(v)
	if err != nil {
		return nil
	}
	return names
}

func (d *Detector) Close() error { return d.sess.Destroy() }

func (d *Detector) Detect(img image.Image, passes []Pass, minScore float32) ([]Box, error) {
	var out []Box
	b := img.Bounds()
	for _, p := range passes {
		var regions []image.Rectangle
		if p.Tiles < 2 {
			regions = []image.Rectangle{b}
		} else {
			regions = tiles(b, p.Tiles, p.Overlap)
		}
		for _, r := range regions {
			boxes, err := d.run(img, r, p.Size, p.Flip, minScore)
			if err != nil {
				return nil, err
			}
			for i := range boxes {
				boxes[i].Pass = p.Name
			}
			out = append(out, boxes...)
		}
	}
	return out, nil
}

func (d *Detector) run(img image.Image, region image.Rectangle, size int, flip bool, minScore float32) (result []Box, err error) {
	data, nw, _, pw, ph, scale := prepare(img, region, size, flip)
	in, err := ort.NewTensor(ort.NewShape(1, 3, int64(ph), int64(pw)), data)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, in.Destroy()) }()
	outs := []ort.Value{nil}
	if err := d.sess.Run([]ort.Value{in}, outs); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, outs[0].Destroy()) }()
	t, ok := outs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("the model output is not float32")
	}
	shape := t.GetShape()
	if err := checkShape(shape, d.labels); err != nil {
		return nil, err
	}
	return boxesFrom(t.GetData(), int(shape[2]), d.labels, region, nw, scale, flip, minScore), nil
}
