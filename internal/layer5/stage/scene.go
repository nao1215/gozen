// Package stage draws a painted scene (ADR 0022): the location's background seen from a
// fixed camera, and small baked dolls standing in it, hidden where the picture is nearer
// than their feet (its depth) or where a front stands before them. It also holds the
// scene's ground: where feet may go, how tall a person is at a place, and walking.
package stage

import (
	"image"
	"image/color"
	"math"

	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// PersonHeight is a person's height in meters: the meters of the ground are counted in it.
const PersonHeight = 1.7

// Exit is an area of the scene that leads to a location.
type Exit struct {
	To   string
	Area geom.Poly
}

// Front is a part of the picture standing up from the ground at the line y = Base.
type Front struct {
	Area geom.Poly
	Base float64
}

// Scene is the ground of a stage: its polygons in the background's pixels, and a person's
// height in pixels at each feet height.
type Scene struct {
	Walk, Blocks []geom.Poly
	Exits        []Exit
	Fronts       []Front
	Camera       float64 // degrees the camera looks down
	DepthBias    float64
	a, b         float64 // a person's height in pixels is a*y + b at feet y
	g            *grid   // where feet may stand, in cells (made on the first Path)
}

// New is the ground of a stage of the schema.
func New(s *data.Stage) *Scene {
	sc := &Scene{Camera: s.Camera, DepthBias: s.DepthBias}
	if sc.Camera == 0 {
		sc.Camera = 30
	}
	if sc.DepthBias == 0 {
		sc.DepthBias = 0.03
	}
	for _, w := range s.Walk {
		sc.Walk = append(sc.Walk, w.Poly())
	}
	for _, b := range s.Blocks {
		sc.Blocks = append(sc.Blocks, b.Poly())
	}
	for _, e := range s.Exits {
		sc.Exits = append(sc.Exits, Exit{To: e.To, Area: e.Area.Poly()})
	}
	for _, f := range s.Fronts {
		sc.Fronts = append(sc.Fronts, Front{Area: f.Area.Poly(), Base: f.Base})
	}
	// a line through the marks (least squares); one mark gives a flat height
	var sy, sh, syy, syh float64
	for _, m := range s.Scale {
		y := m.At[1]
		sy, sh, syy, syh = sy+y, sh+m.Height, syy+y*y, syh+y*m.Height
	}
	n := float64(len(s.Scale))
	switch den := n*syy - sy*sy; {
	case n > 1 && math.Abs(den) > 1e-9:
		sc.a = (n*syh - sy*sh) / den
		sc.b = (sh - sc.a*sy) / n
	case n > 0:
		sc.b = sh / n
	default:
		sc.b = 100
	}
	return sc
}

// Height is a person's height in pixels with the feet at y.
func (s *Scene) Height(y float64) float64 { return math.Max(8, s.a*y+s.b) }

// PPM is the pixels a meter across the ground at feet y.
func (s *Scene) PPM(y float64) float64 { return s.Height(y) / PersonHeight }

// fore is how much shorter a meter away from the camera looks than one across.
func (s *Scene) fore() float64 { return math.Sin(s.Camera * math.Pi / 180) }

// Meters is the distance on the ground between two points (in meters).
func (s *Scene) Meters(p, q geom.Pt) float64 {
	ppm := s.PPM((p.Y + q.Y) / 2)
	return math.Hypot((p.X-q.X)/ppm, (p.Y-q.Y)/(ppm*s.fore()))
}

// Free reports whether feet may stand at p.
func (s *Scene) Free(p geom.Pt) bool {
	in := false
	for _, w := range s.Walk {
		if w.Inside(p) {
			in = true
			break
		}
	}
	if !in {
		return false
	}
	for _, b := range s.Blocks {
		if b.Inside(p) {
			return false
		}
	}
	return true
}

// ExitAt is the location the feet at p lead to, or "".
func (s *Scene) ExitAt(p geom.Pt) string {
	for _, e := range s.Exits {
		if e.Area.Inside(p) {
			return e.To
		}
	}
	return ""
}

// Move steps someone from p by (dx, dy) meters on the ground (x across, y away from the
// camera), sliding along what blocks; it returns where they end up.
func (s *Scene) Move(p geom.Pt, dx, dy float64) geom.Pt {
	ppm := s.PPM(p.Y)
	if q := (geom.Pt{X: p.X + dx*ppm, Y: p.Y}); s.Free(q) {
		p = q
	}
	if q := (geom.Pt{X: p.X, Y: p.Y - dy*ppm*s.fore()}); s.Free(q) {
		p = q
	}
	return p
}

// Toward is the step (in meters on the ground, as for Move) from p toward q, at most d
// meters long, and the meters left to q.
func (s *Scene) Toward(p, q geom.Pt, d float64) (dx, dy, left float64) {
	ppm := s.PPM(p.Y)
	mx, my := (q.X-p.X)/ppm, -(q.Y-p.Y)/(ppm*s.fore())
	l := math.Hypot(mx, my)
	if l < 1e-9 {
		return 0, 0, 0
	}
	k := math.Min(d, l) / l
	return mx * k, my * k, l
}

// Facing is the screen direction (y down) of a way to face: north is away from the camera.
func Facing(word string) geom.Pt {
	switch data.FacingIndex(word) {
	case 1:
		return geom.Pt{X: 1}
	case 2:
		return geom.Pt{Y: 1}
	case 3:
		return geom.Pt{X: -1}
	}
	return geom.Pt{Y: -1}
}

// Gray16 is a depth picture as 16-bit gray, stretched to size (nil: all far).
func Gray16(img image.Image, size image.Rectangle) *image.Gray16 {
	g := image.NewGray16(size)
	if img == nil {
		return g
	}
	b := img.Bounds()
	for y := size.Min.Y; y < size.Max.Y; y++ {
		sy := b.Min.Y + (y-size.Min.Y)*b.Dy()/size.Dy()
		for x := size.Min.X; x < size.Max.X; x++ {
			sx := b.Min.X + (x-size.Min.X)*b.Dx()/size.Dx()
			r, _, _, _ := img.At(sx, sy).RGBA()
			g.SetGray16(x, y, color.Gray16{Y: uint16(r)}) //nolint:gosec // a 16-bit channel
		}
	}
	return g
}
