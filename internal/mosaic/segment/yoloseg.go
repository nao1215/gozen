//go:build cgo

package segment

import (
	"errors"
	"fmt"
	"image"
	"os"

	ort "github.com/yalue/onnxruntime_go"
)

type YoloSeg struct {
	sess   *ort.DynamicAdvancedSession
	labels []string
	size   int
}

func NewYoloSeg(model, lib string, threads, size int, labels []string) (result *YoloSeg, err error) {
	if model == "" || lib == "" || size < 1 || size > 4096 || len(labels) == 0 {
		return nil, errors.New("segmentation needs explicit model/runtime paths, labels, and input size in 1..4096")
	}
	if !ort.IsInitialized() {
		ort.SetSharedLibraryPath(lib)
		if err := ort.InitializeEnvironment(); err != nil {
			return nil, fmt.Errorf("cannot load onnxruntime (%s): %w", lib, err)
		}
	}
	if _, err := os.Stat(model); err != nil {
		return nil, err
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
	s, err := ort.NewDynamicAdvancedSession(model, []string{"images"}, []string{"output0", "output1"}, opts)
	if err != nil {
		return nil, fmt.Errorf("cannot load the model %s: %w", model, err)
	}
	return &YoloSeg{sess: s, labels: labels, size: size}, nil
}

func (y *YoloSeg) Close() error { return y.sess.Destroy() }

func (y *YoloSeg) Find(img image.Image, minScore float32, flip bool) (result []Object, err error) {
	data, nw, _, pw, ph, scale := prepare(img, y.size, flip)
	in, err := ort.NewTensor(ort.NewShape(1, 3, int64(ph), int64(pw)), data)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, in.Destroy()) }()
	outs := []ort.Value{nil, nil}
	if err := y.sess.Run([]ort.Value{in}, outs); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, outs[0].Destroy()) }()
	defer func() { err = errors.Join(err, outs[1].Destroy()) }()
	pt, ok1 := outs[0].(*ort.Tensor[float32])
	mt, ok2 := outs[1].(*ort.Tensor[float32])
	if !ok1 || !ok2 {
		return nil, errors.New("the segmentation output is not float32")
	}
	nc := len(y.labels)
	ps, msh := pt.GetShape(), mt.GetShape()
	if err := checkShapes(ps, msh, nc); err != nil {
		return nil, err
	}
	keep := suppress(candidatesFrom(pt.GetData(), int(ps[2]), nc, minScore))
	return objectsFrom(keep, mt.GetData(), int(msh[2]), int(msh[3]), y.labels, img.Bounds(), nw, pw, ph, scale, flip), nil
}
