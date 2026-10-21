package detect

import (
	"fmt"
	"image"
	"strings"

	"golang.org/x/image/draw"
)

// The pre- and post-processing of a detection model, apart from the ONNX session: the
// picture is scaled and padded to a network input, and the network output is read back as
// boxes. They need no ONNX Runtime, so they are checked without a model.

// padValue is the gray the input is padded with (the letterbox color of YOLO models).
const padValue = 114.0 / 255

// prepare scales a region of img so that its long side is size pixels (the region's own
// long side when size is not above 0), and lays it in the top left of a gray NCHW float
// plane set whose sides are rounded up to multiples of 32. flip mirrors the picture. It
// returns the data, the scaled size (nw, nh), the padded size (pw, ph) and the scale.
func prepare(img image.Image, region image.Rectangle, size int, flip bool) (data []float32, nw, nh, pw, ph int, scale float64) {
	rw, rh := region.Dx(), region.Dy()
	if size <= 0 {
		size = max(rw, rh)
	}
	scale = float64(size) / float64(max(rw, rh))
	nw, nh = max(1, int(float64(rw)*scale+0.5)), max(1, int(float64(rh)*scale+0.5))
	pw, ph = (nw+31)/32*32, (nh+31)/32*32
	small := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(small, small.Bounds(), img, region, draw.Src, nil)

	data = make([]float32, 3*pw*ph)
	plane := pw * ph
	for i := range data {
		data[i] = padValue
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
	return data, nw, nh, pw, ph, scale
}

// checkShape accepts the output shape [1, 4+classes, N] and rejects any other.
func checkShape(shape []int64, labels []string) error {
	if len(shape) != 3 || shape[0] != 1 || shape[2] < 0 || shape[1] != int64(4+len(labels)) {
		return fmt.Errorf("the model output shape %v is not [1,%d,N]: the model must emit %d classes to match --classes (%s)", shape, 4+len(labels), len(labels), strings.Join(labels, ","))
	}
	return nil
}

// boxesFrom reads the boxes of an output v shaped [4+classes, n] (center x, center y,
// width, height, then one score row per class) that score at least minScore. The boxes
// are in the picture's pixels: region is where the network input came from, scale its
// scaling, and nw the scaled width (needed to undo flip).
func boxesFrom(v []float32, n int, labels []string, region image.Rectangle, nw int, scale float64, flip bool, minScore float32) []Box {
	var boxes []Box
	for a := range n {
		for c := range labels {
			s := v[(4+c)*n+a]
			if s < minScore {
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
				boxes = append(boxes, Box{Rect: r, Label: labels[c], Score: s})
			}
		}
	}
	return boxes
}
