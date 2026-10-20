// Package mosaic implements non-destructive pixelation and fix documents for
// the mosaic CLI. Input images remain owned by the caller and are never changed.
package mosaic

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/nao1215/gozen/internal/mosaic/settings"
)

var MinBlock = settings.Defaults.MinBlock

// DefaultBlock chooses max(4, ceil(long edge / 100)), independent of model data.
func DefaultBlock(w, h int) int {
	long := max(w, h)
	return max(MinBlock, long/settings.Defaults.BlockDivisor+min(1, long%settings.Defaults.BlockDivisor))
}

func Apply(img image.Image, rects []image.Rectangle, block int) *image.RGBA {
	shapes := make([]Shape, len(rects))
	for i, r := range rects {
		shapes[i] = Box(r)
	}
	return ApplyShapes(img, shapes, block)
}

type Shape interface {
	Bounds() image.Rectangle
	Hits(cell image.Rectangle) bool
}

type Box image.Rectangle

func (r Box) Bounds() image.Rectangle        { return image.Rectangle(r) }
func (r Box) Hits(cell image.Rectangle) bool { return image.Rectangle(r).Overlaps(cell) }

type Polygon []image.Point

func (p Polygon) Bounds() image.Rectangle {
	if len(p) == 0 {
		return image.Rectangle{}
	}
	r := image.Rectangle{Min: p[0], Max: p[0]}
	for _, q := range p[1:] {
		r.Min.X, r.Min.Y = min(r.Min.X, q.X), min(r.Min.Y, q.Y)
		r.Max.X, r.Max.Y = max(r.Max.X, q.X), max(r.Max.Y, q.Y)
	}
	r.Max = r.Max.Add(image.Pt(1, 1))
	return r
}

func (p Polygon) Hits(cell image.Rectangle) bool {
	if !p.Bounds().Overlaps(cell) {
		return false
	}
	for _, q := range p {
		if q.In(cell) {
			return true
		}
	}
	for y := cell.Min.Y; y < cell.Max.Y; y++ {
		for x := cell.Min.X; x < cell.Max.X; x++ {
			if p.Contains(float64(x)+0.5, float64(y)+0.5) {
				return true
			}
		}
	}
	return false
}

func (p Polygon) Contains(x, y float64) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		xi, yi, xj, yj := float64(p[i].X), float64(p[i].Y), float64(p[j].X), float64(p[j].Y)
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

func ApplyShapes(img image.Image, shapes []Shape, block int) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	if block < 1 {
		block = DefaultBlock(b.Dx(), b.Dy())
	}
	for _, c := range Cells(b, shapes, block) {
		fill(out, c, average(img, c))
	}
	return out
}

// ApplyShapesAt returns a fresh RGBA image, averaging each selected cell once
// from the original pixels. Block values below 1 select the default.
func ApplyShapesAt(img image.Image, shapes []Shape, block int, off image.Point) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	if block < 1 {
		block = DefaultBlock(b.Dx(), b.Dy())
	}
	for _, c := range CellsAt(b, shapes, block, off) {
		fill(out, c, average(img, c))
	}
	return out
}

func Cells(b image.Rectangle, shapes []Shape, block int) []image.Rectangle {
	return CellsAt(b, shapes, block, image.Point{})
}

// CellsAt clips and deduplicates cells. Block values below 1 use DefaultBlock;
// offsets may be negative and the caller owns the returned slice.
func CellsAt(b image.Rectangle, shapes []Shape, block int, off image.Point) []image.Rectangle {
	if block < 1 {
		block = DefaultBlock(b.Dx(), b.Dy())
	}
	block = min(block, max(1, max(b.Dx(), b.Dy())))
	done := map[image.Point]bool{}
	var out []image.Rectangle
	ox, oy := b.Min.X+off.X%block-block, b.Min.Y+off.Y%block-block
	for _, s := range shapes {
		r := s.Bounds().Intersect(b)
		if r.Empty() {
			continue
		}
		x0 := (r.Min.X - ox) / block
		y0 := (r.Min.Y - oy) / block
		x1 := (r.Max.X - ox - 1) / block
		y1 := (r.Max.Y - oy - 1) / block
		for by := y0; by <= y1; by++ {
			for bx := x0; bx <= x1; bx++ {
				p := image.Pt(bx, by)
				if done[p] {
					continue
				}
				cell := image.Rect(ox+bx*block, oy+by*block, ox+(bx+1)*block, oy+(by+1)*block).Intersect(b)
				if s.Hits(cell) {
					done[p] = true
					out = append(out, cell)
				}
			}
		}
	}
	return out
}

func average(img image.Image, r image.Rectangle) color.RGBA {
	var sr, sg, sb, sa, n uint64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			sr += uint64(cr)
			sg += uint64(cg)
			sb += uint64(cb)
			sa += uint64(ca)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{}
	}
	return color.RGBA{uint8(sr / n >> 8), uint8(sg / n >> 8), uint8(sb / n >> 8), uint8(sa / n >> 8)} //nolint:gosec // Channel averages remain within 0..255.
}

func fill(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

func Pad(r image.Rectangle, mx, my int, bounds image.Rectangle) image.Rectangle {
	return image.Rect(r.Min.X-mx, r.Min.Y-my, r.Max.X+mx, r.Max.Y+my).Intersect(bounds)
}

func Grow(r image.Rectangle, w, h int, bounds image.Rectangle) image.Rectangle {
	w, h = min(w, bounds.Dx()), min(h, bounds.Dy())
	grow := func(lo, hi, size, blo, bhi int) (int, int) {
		if hi-lo >= size {
			return lo, hi
		}
		c := (lo + hi) / 2
		lo = c - size/2
		hi = lo + size
		if lo < blo {
			lo, hi = blo, blo+size
		}
		if hi > bhi {
			lo, hi = bhi-size, bhi
		}
		return lo, hi
	}
	r.Min.X, r.Max.X = grow(r.Min.X, r.Max.X, w, bounds.Min.X, bounds.Max.X)
	r.Min.Y, r.Max.Y = grow(r.Min.Y, r.Max.Y, h, bounds.Min.Y, bounds.Max.Y)
	return r
}

func Merge(rects []image.Rectangle, gap int) []image.Rectangle {
	out := append([]image.Rectangle(nil), rects...)
	for changed := true; changed; {
		changed = false
		for i := 0; i < len(out) && !changed; i++ {
			for j := i + 1; j < len(out); j++ {
				if out[i].Inset(-gap - 1).Overlaps(out[j]) {
					out[i] = out[i].Union(out[j])
					out = append(out[:j], out[j+1:]...)
					changed = true
					break
				}
			}
		}
	}
	return out
}

func BestOffset(b image.Rectangle, shapes []Shape, block int) image.Point {
	if block < 1 {
		block = DefaultBlock(b.Dx(), b.Dy())
	}
	block = min(block, max(1, max(b.Dx(), b.Dy())))
	if block <= 1 || len(shapes) == 0 {
		return image.Point{}
	}
	var u image.Rectangle
	for _, s := range shapes {
		u = u.Union(s.Bounds().Intersect(b))
	}
	if u.Empty() {
		return image.Point{}
	}

	w, h := u.Dx(), u.Dy()
	var xs, ys []int
	for y := u.Min.Y; y < u.Max.Y; y++ {
		for x := u.Min.X; x < u.Max.X; x++ {
			px := image.Rect(x, y, x+1, y+1)
			for _, s := range shapes {
				if s.Bounds().Overlaps(px) && s.Hits(px) {
					xs, ys = append(xs, x-b.Min.X), append(ys, y-b.Min.Y)
					break
				}
			}
		}
	}

	ux, uy := u.Min.X-b.Min.X, u.Min.Y-b.Min.Y
	cw, ch := w/block+3, h/block+3
	seen := make([]bool, cw*ch)
	best, bestArea := image.Point{}, -1

	thin := func(o, size int) bool {
		l, r := o%block, (size-o%block)%block
		return l > 0 && l < MinBlock || r > 0 && r < MinBlock
	}
	for oy := range block {
		if oy > 0 && thin(oy, b.Dy()) {
			continue
		}
		for ox := range block {
			if ox > 0 && thin(ox, b.Dx()) {
				continue
			}
			clear(seen)
			bx0, by0 := (ux-ox+block)/block, (uy-oy+block)/block
			area := 0
			for i := range xs {
				cx, cy := (xs[i]-ox+block)/block, (ys[i]-oy+block)/block
				k := (cy-by0)*cw + cx - bx0
				if !seen[k] {
					seen[k] = true
					r := image.Rect(b.Min.X+ox+(cx-1)*block, b.Min.Y+oy+(cy-1)*block, b.Min.X+ox+cx*block, b.Min.Y+oy+cy*block).Intersect(b)
					area += r.Dx() * r.Dy()
				}
			}
			if bestArea < 0 || area < bestArea {
				best, bestArea = image.Pt(ox, oy), area
			}
		}
	}
	return best
}
