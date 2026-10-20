//go:build cgo

// Package tagger tags a picture with a user-supplied ONNX tagging model.
package tagger

import (
	"encoding/csv"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"

	ort "github.com/yalue/onnxruntime_go"
	xdraw "golang.org/x/image/draw"
)

const Keep = 0.05

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

func readNames(path string) (result []string, err error) {
	f, err := os.Open(path) //nolint:gosec // The caller explicitly selects this input file.
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(rows) < 2 || len(rows[0]) < 2 || rows[0][1] != "name" {
		return nil, fmt.Errorf("%s: the second column must be name", path)
	}
	names := make([]string, 0, len(rows)-1)
	for _, r := range rows[1:] {
		names = append(names, r[1])
	}
	return names, nil
}

func (t *Tagger) Close() error { return t.sess.Destroy() }

func (t *Tagger) Tags(img image.Image) (result map[string]float32, err error) {
	b := img.Bounds()
	s := max(b.Dx(), b.Dy())
	sq := image.NewRGBA(image.Rect(0, 0, s, s))
	draw.Draw(sq, sq.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	at := image.Pt((s-b.Dx())/2, (s-b.Dy())/2)
	draw.Draw(sq, image.Rectangle{Min: at, Max: at.Add(b.Size())}, img, b.Min, draw.Over)
	small := image.NewRGBA(image.Rect(0, 0, t.size, t.size))
	xdraw.CatmullRom.Scale(small, small.Bounds(), sq, sq.Bounds(), draw.Src, nil)

	data := make([]float32, t.size*t.size*3)
	for i := range t.size * t.size {
		p := small.Pix[i*4 : i*4+3]
		data[i*3], data[i*3+1], data[i*3+2] = float32(p[2]), float32(p[1]), float32(p[0])
	}
	in, err := ort.NewTensor(ort.NewShape(1, int64(t.size), int64(t.size), 3), data)
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
	v := o.GetData()
	if len(v) != len(t.names) {
		return nil, fmt.Errorf("the tagger gives %d values for %d tags", len(v), len(t.names))
	}
	tags := map[string]float32{}
	for i, p := range v {
		if p >= Keep {
			tags[t.names[i]] = float32(int(p*1000+0.5)) / 1000
		}
	}
	return tags, nil
}
