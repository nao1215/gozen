//go:build cgo

// Package tagger tags a picture with a user-supplied ONNX tagging model.
package tagger

import (
	"errors"
	"fmt"
	"image"
	"path/filepath"

	ort "github.com/yalue/onnxruntime_go"
)

type Tagger struct {
	sess  *ort.DynamicAdvancedSession
	names []string
	size  int
}

func New(dir, lib string, threads int) (result *Tagger, err error) {
	if dir == "" || lib == "" {
		return nil, errors.New("tagging needs an explicit model directory and ONNX Runtime shared library")
	}
	names, err := readNames(filepath.Join(dir, "selected_tags.csv"))
	if err != nil {
		return nil, err
	}
	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(lib)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("cannot load onnxruntime (%s): %w", lib, err)
		}
	}
	model := filepath.Join(dir, "model.onnx")
	ins, outs, err := ort.GetInputOutputInfo(model)
	if err != nil {
		return nil, fmt.Errorf("cannot read the model %s: %w", model, err)
	}
	if len(ins) != 1 || len(outs) != 1 || len(ins[0].Dimensions) != 4 {
		return nil, fmt.Errorf("the model %s is not a WD tagger (one NHWC input and one output)", model)
	}
	size := int(ins[0].Dimensions[1])
	if size < 1 || size > 4096 || ins[0].Dimensions[0] != 1 || ins[0].Dimensions[2] != int64(size) || ins[0].Dimensions[3] != 3 {
		return nil, errors.New("tagger input must be fixed [1,H,H,3], H in 1..4096")
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
	s, err := ort.NewDynamicAdvancedSession(model, []string{ins[0].Name}, []string{outs[0].Name}, opts)
	if err != nil {
		return nil, fmt.Errorf("cannot load the model %s: %w", model, err)
	}
	return &Tagger{sess: s, names: names, size: size}, nil
}

func (t *Tagger) Close() error { return t.sess.Destroy() }

func (t *Tagger) Tags(img image.Image) (result map[string]float32, err error) {
	in, err := ort.NewTensor(ort.NewShape(1, int64(t.size), int64(t.size), 3), prepare(img, t.size))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, in.Destroy()) }()
	outs := []ort.Value{nil}
	if err := t.sess.Run([]ort.Value{in}, outs); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, outs[0].Destroy()) }()
	o, ok := outs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("the tagger output is not float32")
	}
	return tagsFrom(o.GetData(), t.names)
}
