//go:build cgo

package detect

import (
	"errors"
	"fmt"
	"image"
	"os"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
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
		return nil, fmt.Errorf("missing -model: supply a detection ONNX model")
	}
	if _, err := os.Stat(model); err != nil {
		return nil, fmt.Errorf("detection model: %w", err)
	}
	if lib == "" {
		return nil, fmt.Errorf("missing -ort or ONNXRUNTIME_LIB: supply the ONNX Runtime shared library")
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

func (d *Detector) Close() error { return d.sess.Destroy() }

func (d *Detector) Detect(img image.Image, passes []Pass, min float32) ([]Box, error) {
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
			boxes, err := d.run(img, r, p.Size, p.Flip, min)
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

func (d *Detector) run(img image.Image, region image.Rectangle, size int, flip bool, min float32) (result []Box, err error) {
	rw, rh := region.Dx(), region.Dy()
	if size <= 0 {
		size = max(rw, rh)
	}
	scale := float64(size) / float64(max(rw, rh))
	nw, nh := max(1, int(float64(rw)*scale+0.5)), max(1, int(float64(rh)*scale+0.5))
	pw, ph := (nw+31)/32*32, (nh+31)/32*32
	small := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(small, small.Bounds(), img, region, draw.Src, nil)

	data := make([]float32, 3*pw*ph)
	plane := pw * ph
	for i := range data {
		data[i] = 114.0 / 255
	}
	for y := range nh {
		for x := range nw {
			sx := x
			if flip {
				sx = nw - 1 - x
			}
			o := small.PixOffset(sx, y)
			k := y*pw + x
			data[k] = float32(small.Pix[o]) / 255
			data[plane+k] = float32(small.Pix[o+1]) / 255
			data[2*plane+k] = float32(small.Pix[o+2]) / 255
		}
	}
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
		return nil, fmt.Errorf("the model output is not float32")
	}
	shape := t.GetShape()
	if len(shape) != 3 || shape[0] != 1 || shape[2] < 0 || shape[1] != int64(4+len(d.labels)) {
		return nil, fmt.Errorf("the model output shape %v is not [1,%d,N]", shape, 4+len(d.labels))
	}
	n := int(shape[2])
	v := t.GetData()
	var boxes []Box
	for a := range n {
		for c := range d.labels {
			s := v[(4+c)*n+a]
			if s < min {
				continue
			}
			cx, cy, w, h := float64(v[a]), float64(v[n+a]), float64(v[2*n+a]), float64(v[3*n+a])
			if flip {
				cx = float64(nw) - cx
			}
			x0 := float64(region.Min.X) + (cx-w/2)/scale
			y0 := float64(region.Min.Y) + (cy-h/2)/scale
			x1 := float64(region.Min.X) + (cx+w/2)/scale
			y1 := float64(region.Min.Y) + (cy+h/2)/scale
			r := image.Rect(int(x0), int(y0), int(x1+0.999), int(y1+0.999)).Intersect(region)
			if !r.Empty() {
				boxes = append(boxes, Box{Rect: r, Label: d.labels[c], Score: s})
			}
		}
	}
	return boxes, nil
}
