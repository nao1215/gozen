//go:build cgo

package segment

import (
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"sort"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
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
	b := img.Bounds()
	scale := float64(y.size) / float64(max(b.Dx(), b.Dy()))
	nw, nh := max(1, int(float64(b.Dx())*scale+0.5)), max(1, int(float64(b.Dy())*scale+0.5))
	pw, ph := (nw+31)/32*32, (nh+31)/32*32
	small := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.BiLinear.Scale(small, small.Bounds(), img, b, draw.Src, nil)
	plane := pw * ph
	data := make([]float32, 3*plane)
	for i := range data {
		data[i] = 114.0 / 255
	}
	for py := range nh {
		for px := range nw {
			sx := px
			if flip {
				sx = nw - 1 - px
			}
			o := small.PixOffset(sx, py)
			k := py*pw + px
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
	if len(ps) != 3 || ps[0] != 1 || ps[2] < 0 || ps[1] != int64(4+nc+32) || len(msh) != 4 || msh[0] != 1 || msh[1] != 32 || msh[2] < 1 || msh[3] < 1 {
		return nil, fmt.Errorf("the segmentation output shapes %v %v are not [1,%d,N] and [1,32,h,w]", ps, msh, 4+nc+32)
	}
	n := int(ps[2])
	p := pt.GetData()
	type cand struct {
		c      int
		s      float32
		x0, y0 float64
		x1, y1 float64
		coef   [32]float32
	}
	var cs []cand
	for a := range n {
		best, bc := float32(0), 0
		for c := range nc {
			if v := p[(4+c)*n+a]; v > best {
				best, bc = v, c
			}
		}
		if best < minScore {
			continue
		}
		cx, cy, w, h := float64(p[a]), float64(p[n+a]), float64(p[2*n+a]), float64(p[3*n+a])
		k := cand{c: bc, s: best, x0: cx - w/2, y0: cy - h/2, x1: cx + w/2, y1: cy + h/2}
		for i := range 32 {
			k.coef[i] = p[(4+nc+i)*n+a]
		}
		cs = append(cs, k)
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].s > cs[j].s })
	var keep []cand
	for _, k := range cs {
		dup := false
		for _, q := range keep {
			if q.c != k.c {
				continue
			}
			iw := math.Max(0, math.Min(k.x1, q.x1)-math.Max(k.x0, q.x0))
			ih := math.Max(0, math.Min(k.y1, q.y1)-math.Max(k.y0, q.y0))
			inter := iw * ih
			if inter/((k.x1-k.x0)*(k.y1-k.y0)+(q.x1-q.x0)*(q.y1-q.y0)-inter) > 0.5 {
				dup = true
				break
			}
		}
		if !dup {
			keep = append(keep, k)
		}
	}
	mh, mw := int(msh[2]), int(msh[3])
	proto := mt.GetData()
	var objs []Object
	for _, k := range keep {

		low := make([]float32, mh*mw)
		for i := range 32 {
			c := k.coef[i]
			pr := proto[i*mh*mw : (i+1)*mh*mw]
			for j := range low {
				low[j] += c * pr[j]
			}
		}
		x0, y0, x1, y1 := k.x0, k.y0, k.x1, k.y1
		if flip {
			x0, x1 = float64(nw)-k.x1, float64(nw)-k.x0
		}
		r := image.Rect(b.Min.X+int(math.Floor(x0/scale)), b.Min.Y+int(math.Floor(y0/scale)),
			b.Min.X+int(math.Ceil(x1/scale)), b.Min.Y+int(math.Ceil(y1/scale))).Intersect(b)
		if r.Empty() {
			continue
		}
		m := image.NewAlpha(r)

		sx := float64(mw) / float64(pw)
		sy := float64(mh) / float64(ph)
		for oy := r.Min.Y; oy < r.Max.Y; oy++ {
			iy := (float64(oy-b.Min.Y) + 0.5) * scale
			for ox := r.Min.X; ox < r.Max.X; ox++ {
				ix := (float64(ox-b.Min.X) + 0.5) * scale
				if flip {
					ix = float64(nw) - ix
				}
				if bilinear(low, mw, mh, ix*sx-0.5, iy*sy-0.5) > 0 {
					m.Pix[m.PixOffset(ox, oy)] = 255
				}
			}
		}
		objs = append(objs, Object{Label: y.labels[k.c], Score: k.s, Rect: r, Mask: m})
	}
	return objs, nil
}

func bilinear(v []float32, w, h int, x, y float64) float32 {
	x = math.Max(0, math.Min(float64(w-1), x))
	y = math.Max(0, math.Min(float64(h-1), y))
	x0, y0 := int(x), int(y)
	x1, y1 := min(x0+1, w-1), min(y0+1, h-1)
	fx, fy := float32(x-float64(x0)), float32(y-float64(y0))
	a := v[y0*w+x0]*(1-fx) + v[y0*w+x1]*fx
	b := v[y1*w+x0]*(1-fx) + v[y1*w+x1]*fx
	return a*(1-fy) + b*fy
}
