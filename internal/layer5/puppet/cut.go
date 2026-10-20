package puppet

import (
	"image"
	"image/draw"
)

// Patch is a small picture placed at At on the base.
type Patch struct {
	Img *image.NRGBA
	At  image.Point
}

// Rect is the patch area in base coordinates.
func (p *Patch) Rect() image.Rectangle { return p.Img.Bounds().Sub(p.Img.Bounds().Min).Add(p.At) }

const (
	cutBlock = 8 // pixels per block when looking for the differing part
	cutPad   = 2 // blocks of margin, faded out so the edge of the patch blends in
	// cutLevel is the mean difference per channel (0 to 255) that marks a block as changed.
	// Whole pictures redrawn at a low strength differ a little everywhere; the eyes and
	// mouth differ much more.
	cutLevel = 28
)

// Cut finds where variant differs from base inside area (the whole picture when
// empty) and returns that part as a patch with a soft edge. ok is false when
// nothing differs. Both pictures must have the same bounds.
//
// The search marks 8x8 blocks whose mean difference is high, drops small specks
// (connected groups below a quarter of the largest group), and pads the box by two
// blocks whose alpha fades to zero toward the edge.
func Cut(base, variant image.Image, area image.Rectangle) (Patch, bool) {
	b := base.Bounds()
	if area.Empty() {
		area = b
	}
	area = area.Intersect(b)
	if area.Empty() || variant.Bounds() != b {
		return Patch{}, false
	}
	bw := (area.Dx() + cutBlock - 1) / cutBlock
	bh := (area.Dy() + cutBlock - 1) / cutBlock
	marked := make([]bool, bw*bh)
	for by := range bh {
		for bx := range bw {
			r := image.Rect(area.Min.X+bx*cutBlock, area.Min.Y+by*cutBlock, area.Min.X+(bx+1)*cutBlock, area.Min.Y+(by+1)*cutBlock).Intersect(area)
			marked[by*bw+bx] = blockDiff(base, variant, r) >= cutLevel
		}
	}
	groups := components(marked, bw, bh)
	if len(groups) == 0 {
		return Patch{}, false
	}
	largest := 0
	for _, g := range groups {
		largest = max(largest, len(g))
	}
	var box image.Rectangle
	for _, g := range groups {
		if len(g)*4 < largest {
			continue
		}
		for _, i := range g {
			x, y := i%bw, i/bw
			box = box.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	box = image.Rect(box.Min.X-cutPad, box.Min.Y-cutPad, box.Max.X+cutPad, box.Max.Y+cutPad)
	px := image.Rect(area.Min.X+box.Min.X*cutBlock, area.Min.Y+box.Min.Y*cutBlock, area.Min.X+box.Max.X*cutBlock, area.Min.Y+box.Max.Y*cutBlock).Intersect(area)
	return Patch{Img: soften(variant, px, cutPad*cutBlock), At: px.Min}, true
}

// blockDiff is the mean per-channel difference of premultiplied colors in r.
func blockDiff(a, b image.Image, r image.Rectangle) int {
	var sum, n uint64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			sum += uint64(absDiff(r1, r2) + absDiff(g1, g2) + absDiff(b1, b2) + absDiff(a1, a2))
			n += 4
		}
	}
	if n == 0 {
		return 0
	}
	return int(sum / n >> 8)
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// components groups marked cells that touch (8 neighbours).
func components(marked []bool, w, h int) [][]int {
	seen := make([]bool, len(marked))
	var groups [][]int
	for start := range marked {
		if !marked[start] || seen[start] {
			continue
		}
		var g []int
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			g = append(g, i)
			x, y := i%w, i/w
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					j := ny*w + nx
					if marked[j] && !seen[j] {
						seen[j] = true
						stack = append(stack, j)
					}
				}
			}
		}
		groups = append(groups, g)
	}
	return groups
}

// soften copies r of src and fades the alpha to zero over fade pixels at the edges.
func soften(src image.Image, r image.Rectangle, fade int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), src, r.Min, draw.Src)
	for y := range r.Dy() {
		for x := range r.Dx() {
			d := min(x+1, y+1, r.Dx()-x, r.Dy()-y)
			if d >= fade {
				continue
			}
			k := float64(d) / float64(fade)
			k = k * k * (3 - 2*k)
			c := out.NRGBAAt(x, y)
			c.A = uint8(float64(c.A)*k + 0.5)
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// toNRGBA returns src as an NRGBA picture whose bounds start at the origin.
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok && n.Bounds().Min == (image.Point{}) {
		return n
	}
	out := image.NewNRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
	return out
}
