package mosaic

import "image"

type Found struct {
	Mosaic bool
	Block  int
	Flat   float64
}

// minFindBlock is the smallest cell Find takes for a mosaic.
const minFindBlock = 4

// Find recognizes exact repeated square cells. Lossy encodings may not match;
// inspection exposes this as a hint, never a verification of image suitability.
//
// Every pixel of the region has to lie in a flat cell of one square grid, and the grid
// needs at least nine whole cells inside the region. Block is the largest such cell, so a
// 16 pixel mosaic is not reported as its divisors 4 or 8; only a region too small to hold
// nine whole cells of the real block reports a divisor. Each pixel is read once.
func Find(img image.Image, r image.Rectangle) Found {
	r = r.Intersect(img.Bounds())
	w, h := r.Dx(), r.Dy()
	limit := min(w, h) / 3
	if limit < minFindBlock {
		return Found{}
	}
	// A grid line is a column (or a row) where a pixel differs from its left (upper)
	// neighbor. The gaps between the lines of an axis are multiples of the block, so the
	// block divides their greatest common divisor.
	columns, rows := gridLines{first: -1}, gridLines{first: -1}
	above := make([]uint64, w)
	for y := range h {
		var left uint64
		for x := range w {
			rr, gg, bb, aa := img.At(r.Min.X+x, r.Min.Y+y).RGBA()
			v := uint64(rr)<<48 | uint64(gg)<<32 | uint64(bb)<<16 | uint64(aa)
			if x > 0 && v != left && !columns.add(x) || y > 0 && v != above[x] && !rows.add(y) {
				return Found{}
			}
			left, above[x] = v, v
		}
	}
	if columns.first < 0 && rows.first < 0 {
		// One flat color has no cells to tell apart.
		return Found{}
	}
	step := gcd(columns.step, rows.step)
	for block := limit; block >= minFindBlock; block-- {
		if step%block != 0 {
			continue
		}
		if columns.whole(w, block)*rows.whole(h, block) >= 9 {
			return Found{true, block, 1}
		}
	}
	return Found{}
}

// gridLines collects the grid lines of one axis: the position of the first, and the
// greatest common divisor of the distances of the others from it (0 with fewer than two).
type gridLines struct{ first, step int }

// add records a line and reports whether the lines can still be a grid of mosaic cells.
func (g *gridLines) add(at int) bool {
	if g.first < 0 {
		g.first = at
		return true
	}
	g.step = gcd(g.step, max(at-g.first, g.first-at))
	return g.step == 0 || g.step >= minFindBlock
}

// whole counts the whole cells of the block size along an axis of the given length.
func (g gridLines) whole(length, block int) int {
	return (length - max(g.first, 0)%block) / block
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
