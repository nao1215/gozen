package segment

import (
	"fmt"
	"image"
	"math"
	"sort"

	"golang.org/x/image/draw"
)

// The pre- and post-processing of a segmentation model, apart from the ONNX session.
// They need no ONNX Runtime, so they are checked without a model.

// padValue is the gray the input is padded with (the letterbox color of YOLO models).
const padValue = 114.0 / 255

// protoCount is how many mask prototypes a YOLO segmentation model emits.
const protoCount = 32

// prepare scales img so that its long side is size pixels and lays it in the top left of a
// gray NCHW float plane set whose sides are rounded up to multiples of 32. flip mirrors
// the picture. It returns the data, the scaled size (nw, nh), the padded size (pw, ph)
// and the scale.
func prepare(img image.Image, size int, flip bool) (data []float32, nw, nh, pw, ph int, scale float64) {
	b := img.Bounds()
	scale = float64(size) / float64(max(b.Dx(), b.Dy()))
	nw, nh = max(1, int(float64(b.Dx())*scale+0.5)), max(1, int(float64(b.Dy())*scale+0.5))
	pw, ph = (nw+31)/32*32, (nh+31)/32*32
	small := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.BiLinear.Scale(small, small.Bounds(), img, b, draw.Src, nil)
	plane := pw * ph
	data = make([]float32, 3*plane)
	for i := range data {
		data[i] = padValue
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
	return data, nw, nh, pw, ph, scale
}

// checkShapes accepts the outputs [1, 4+classes+32, N] and [1, 32, h, w], and rejects any other.
func checkShapes(ps, msh []int64, classes int) error {
	if len(ps) != 3 || ps[0] != 1 || ps[2] < 0 || ps[1] != int64(4+classes+protoCount) || len(msh) != 4 || msh[0] != 1 || msh[1] != protoCount || msh[2] < 1 || msh[3] < 1 {
		return fmt.Errorf("the segmentation output shapes %v %v are not [1,%d,N] and [1,32,h,w]", ps, msh, 4+classes+protoCount)
	}
	return nil
}

// candidate is one detection of the model before suppression: its best class and score,
// its box in the network input's pixels, and its mask coefficients.
type candidate struct {
	c      int
	s      float32
	x0, y0 float64
	x1, y1 float64
	coef   [protoCount]float32
}

// candidatesFrom reads the detections scoring at least minScore from an output p shaped
// [4+classes+32, n] (center x, center y, width, height, one score row per class, then the
// mask coefficients). A detection takes its best class.
func candidatesFrom(p []float32, n, classes int, minScore float32) []candidate {
	var cs []candidate
	for a := range n {
		best, bc := float32(0), 0
		for c := range classes {
			if v := p[(4+c)*n+a]; v > best {
				best, bc = v, c
			}
		}
		if best < minScore {
			continue
		}
		cx, cy, w, h := float64(p[a]), float64(p[n+a]), float64(p[2*n+a]), float64(p[3*n+a])
		k := candidate{c: bc, s: best, x0: cx - w/2, y0: cy - h/2, x1: cx + w/2, y1: cy + h/2}
		for i := range protoCount {
			k.coef[i] = p[(4+classes+i)*n+a]
		}
		cs = append(cs, k)
	}
	return cs
}

// suppress keeps, best score first, each candidate that does not overlap a kept one of
// its class by more than 0.5 (intersection over union).
func suppress(cs []candidate) []candidate {
	sort.Slice(cs, func(i, j int) bool { return cs[i].s > cs[j].s })
	var keep []candidate
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
	return keep
}

// objectsFrom turns the kept candidates into objects in the picture's pixels. proto holds
// the 32 mask prototypes of mh x mw; a pixel is in an object's mask when the coefficients'
// combination of the prototypes, read at the pixel, is above 0. b is the picture's bounds,
// nw the scaled width, pw x ph the padded input size and scale the scaling (see prepare).
func objectsFrom(keep []candidate, proto []float32, mh, mw int, labels []string, b image.Rectangle, nw, pw, ph int, scale float64, flip bool) []Object {
	var objs []Object
	for _, k := range keep {
		low := make([]float32, mh*mw)
		for i := range protoCount {
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
		objs = append(objs, Object{Label: labels[k.c], Score: k.s, Rect: r, Mask: m})
	}
	return objs
}

// bilinear reads the w x h plane v at the fractional position (x, y), clamped to the plane.
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
