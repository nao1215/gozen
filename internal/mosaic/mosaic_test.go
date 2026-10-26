package mosaic

import (
	"image"
	"math/rand/v2"
	"testing"
)

// bestOffsetByPixels is the plain search BestOffset must agree with: every covered pixel
// listed, and for each offset the cells those pixels fall in counted.
func bestOffsetByPixels(b image.Rectangle, shapes []Shape, block int) image.Point {
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

// randomShapes are a few boxes and polygons inside b.
func randomShapes(rng *rand.Rand, b image.Rectangle) []Shape {
	var ss []Shape
	for range 1 + rng.IntN(4) {
		x, y := b.Min.X+rng.IntN(b.Dx()), b.Min.Y+rng.IntN(b.Dy())
		if rng.IntN(2) == 0 {
			ss = append(ss, Box(image.Rect(x, y, x+1+rng.IntN(b.Dx()/2), y+1+rng.IntN(b.Dy()/2))))
			continue
		}
		ss = append(ss, Polygon{{x, y}, {x + 5 + rng.IntN(b.Dx()/2), y + rng.IntN(9)}, {x + rng.IntN(9), y + 5 + rng.IntN(b.Dy()/2)}})
	}
	return ss
}

// BestOffset picks the same offset as the plain search, for shapes of every kind, images
// that do not start at zero, and blocks small and large.
func TestBestOffsetMatchesThePlainSearch(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5)) //nolint:gosec // A fixed seed for a reproducible test.
	for i := range 300 {
		b := image.Rect(rng.IntN(20)-10, rng.IntN(20)-10, 0, 0)
		b.Max = b.Min.Add(image.Pt(8+rng.IntN(120), 8+rng.IntN(120)))
		ss := randomShapes(rng, b)
		block := rng.IntN(24)
		if got, want := BestOffset(b, ss, block), bestOffsetByPixels(b, ss, block); got != want {
			t.Fatalf("case %d (b %v, block %d, shapes %v): offset %v, want %v", i, b, block, ss, got, want)
		}
	}
}

// BenchmarkBestOffset is the search on a large picture: a region over most of it, a thin
// sparse polygon, and several regions apart.
func BenchmarkBestOffset(b *testing.B) {
	pic := image.Rect(0, 0, 1024, 1024)
	for _, c := range []struct {
		name   string
		shapes []Shape
	}{
		{"large", []Shape{Box(image.Rect(40, 40, 1000, 1000))}},
		{"sparse", []Shape{Polygon{{0, 0}, {1023, 20}, {30, 1023}}}},
		{"several", []Shape{Box(image.Rect(10, 10, 200, 200)), Box(image.Rect(700, 100, 900, 400)), Polygon{{300, 600}, {700, 650}, {400, 1000}}}},
	} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				BestOffset(pic, c.shapes, 0)
			}
		})
	}
}
