package doll

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Part is a piece added to a doll.
type Part struct {
	Kind  string
	Color color.RGBA
}

// ValidPart reports an error for a part kind that is not one of the schema's.
func ValidPart(k string) error {
	if slices.Contains(data.DollPartKinds, k) {
		return nil
	}
	return fmt.Errorf("part %q (want one of %v)", k, data.DollPartKinds)
}

// Sides and Poses are the columns (sides, 45 degrees apart: the camera at 0, 45, ... 315
// degrees around from the doll's front) and the rows (standing, then four of a walk) of a
// baked sheet.
const (
	Sides = 8
	Poses = 5
)

// unitsH is the picture's height in the doll's heights (a little room over the head).
const unitsH = 1.12

// The color indexes of a doll's shapes; the parts' own colors come after.
const (
	cHair = iota
	cSkin
	cTop
	cSleeve
	cBottom
	cShoes
	cEye
	cWhite
	cMouth
	cPart
)

func paletteColors(pal Palette) []color.RGBA {
	return []color.RGBA{pal.Hair, pal.Skin, pal.Top, pal.Sleeve, pal.Bottom, pal.Shoes, {46, 36, 44, 255}, {250, 250, 248, 255}, {150, 80, 70, 255}}
}

type vec [3]float64

func sub(a, b vec) vec           { return vec{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func dot(a, b vec) float64       { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func length(a vec) float64       { return math.Sqrt(dot(a, a)) }
func scale(a vec, k float64) vec { return vec{a[0] * k, a[1] * k, a[2] * k} }

// solid is a convex solid of planes around a (unit normals and distances), in units of the
// doll's height (feet at 0, the top of the head about 1). A limb turns about its pivot.
type solid struct {
	a      vec
	r      float64 // a sphere around a holding the solid (to skip the rays that miss it)
	planes [][4]float64
	col    int
	limb   int // 0 none, 1 the right leg, 2 the left leg, 3 the right arm, 4 the left arm
	pivot  vec
}

func polytope(c vec, r float64, ns []vec, hs []float64, col int) solid {
	s := solid{a: c, r: r, col: col}
	for i, n := range ns {
		l := length(n)
		s.planes = append(s.planes, [4]float64{n[0] / l, n[1] / l, n[2] / l, hs[i]})
	}
	return s
}

// box is an upright box from lo to hi.
func box(lo, hi vec, col int) solid {
	c := scale(vec{lo[0] + hi[0], lo[1] + hi[1], lo[2] + hi[2]}, 0.5)
	h := scale(sub(hi, lo), 0.5)
	return polytope(c, length(h)+1e-6, []vec{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}},
		[]float64{h[0], h[0], h[1], h[1], h[2], h[2]}, col)
}

// cbox is a box from lo to hi with its corners and edges cut by cut (0 none .. 1 much).
func cbox(lo, hi vec, cut float64, col int) solid {
	if cut <= 0 {
		return box(lo, hi, col)
	}
	c := scale(vec{lo[0] + hi[0], lo[1] + hi[1], lo[2] + hi[2]}, 0.5)
	h := scale(sub(hi, lo), 0.5)
	var ns []vec
	var hs []float64
	for _, n := range []vec{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
		ns = append(ns, n)
		hs = append(hs, math.Abs(n[0])*h[0]+math.Abs(n[1])*h[1]+math.Abs(n[2])*h[2])
	}
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			for _, sz := range []float64{-1, 1} {
				n := vec{sx / h[0], sy / h[1], sz / h[2]}
				ns = append(ns, n)
				hs = append(hs, (3-cut)/length(n))
			}
		}
	}
	for _, e := range []vec{{1, 1, 0}, {1, -1, 0}, {-1, 1, 0}, {-1, -1, 0}, {0, 1, 1}, {0, 1, -1}, {0, -1, 1}, {0, -1, -1}, {1, 0, 1}, {1, 0, -1}, {-1, 0, 1}, {-1, 0, -1}} {
		n := vec{e[0] / h[0], e[1] / h[1], e[2] / h[2]}
		ns = append(ns, n)
		hs = append(hs, (2-cut*0.5)/length(n))
	}
	return polytope(c, length(h)+1e-6, ns, hs, col)
}

func limbOf(s solid, limb int, pivot vec) solid {
	s.limb, s.pivot = limb, pivot
	return s
}

// frustum is a box narrower at the top (y1, half sizes a) than at the bottom (y0, half sizes b).
func frustum(y0, y1 float64, a, b [2]float64, col int) solid {
	hy := (y1 - y0) / 2
	c := vec{0, (y0 + y1) / 2, 0}
	var ns []vec
	var hs []float64
	for _, side := range []struct{ axis, sign float64 }{{0, 1}, {0, -1}, {2, 1}, {2, -1}} {
		i := int(side.axis)
		ai, bi := a[0], b[0]
		if i == 2 {
			ai, bi = a[1], b[1]
		}
		n := vec{}
		n[i] = side.sign * (y1 - y0)
		n[1] = bi - ai
		n = scale(n, 1/length(n))
		p := vec{}
		p[i] = side.sign * ai
		p[1] = hy
		ns = append(ns, n)
		hs = append(hs, dot(n, p))
	}
	ns = append(ns, vec{0, 1, 0}, vec{0, -1, 0})
	hs = append(hs, hy, hy)
	return polytope(c, math.Sqrt(max(a[0], b[0])*max(a[0], b[0])+max(a[1], b[1])*max(a[1], b[1])+hy*hy)+1e-6, ns, hs, col)
}

// shapes builds a doll: hair is the style, parts the pieces (their colors come after the
// palette's, from cPart on).
func shapes(hair string, parts []Part) []solid {
	hipL, hipR := vec{-0.058, 0.37, 0}, vec{0.058, 0.37, 0}
	shL, shR := vec{-0.165, 0.625, 0}, vec{0.165, 0.625, 0}
	s := []solid{
		cbox(vec{-0.13, 0.675, -0.125}, vec{0.13, 0.975, 0.125}, 0.45, cSkin), // the head
		cbox(vec{-0.03, 0.64, -0.03}, vec{0.03, 0.69, 0.03}, 0, cSkin),        // the neck
		cbox(vec{-0.115, 0.405, -0.07}, vec{0.115, 0.655, 0.07}, 0.35, cTop),  // the body
		cbox(vec{-0.11, 0.33, -0.066}, vec{0.11, 0.405, 0.066}, 0.3, cBottom), // the hips
		limbOf(cbox(vec{-0.102, 0.075, -0.044}, vec{-0.014, 0.34, 0.044}, 0.3, cBottom), 1, hipL),
		limbOf(cbox(vec{0.014, 0.075, -0.044}, vec{0.102, 0.34, 0.044}, 0.3, cBottom), 2, hipR),
		limbOf(cbox(vec{-0.11, 0, -0.055}, vec{-0.006, 0.08, 0.1}, 0.4, cShoes), 1, hipL),
		limbOf(cbox(vec{0.006, 0, -0.055}, vec{0.11, 0.08, 0.1}, 0.4, cShoes), 2, hipR),
		limbOf(cbox(vec{-0.2, 0.42, -0.036}, vec{-0.128, 0.64, 0.036}, 0.35, cSleeve), 3, shL),
		limbOf(cbox(vec{0.128, 0.42, -0.036}, vec{0.2, 0.64, 0.036}, 0.35, cSleeve), 4, shR),
		limbOf(cbox(vec{-0.214, 0.33, -0.048}, vec{-0.12, 0.425, 0.048}, 0.5, cSkin), 3, shL), // hands
		limbOf(cbox(vec{0.12, 0.33, -0.048}, vec{0.214, 0.425, 0.048}, 0.5, cSkin), 4, shR),
	}
	span := func(sg, a, b float64) (float64, float64) {
		if sg < 0 {
			return sg * b, sg * a
		}
		return sg * a, sg * b
	}
	// the face: whites, pupils looking a little inward, brows and a mouth, just out of the face
	for _, sg := range []float64{-1, 1} {
		x0, x1 := span(sg, 0.028, 0.08)
		s = append(s, box(vec{x0, 0.79, 0.112}, vec{x1, 0.838, 0.132}, cWhite))
		p0, p1 := span(sg, 0.03, 0.062)
		s = append(s, box(vec{p0, 0.79, 0.112}, vec{p1, 0.834, 0.136}, cEye))
		b0, b1 := span(sg, 0.024, 0.084)
		s = append(s, box(vec{b0, 0.856, 0.112}, vec{b1, 0.868, 0.134}, cHair))
	}
	s = append(s, box(vec{-0.026, 0.735, 0.112}, vec{0.026, 0.745, 0.13}, cMouth))
	if hair != HairBald {
		s = append(s,
			cbox(vec{-0.143, 0.885, -0.142}, vec{0.143, 0.995, 0.135}, 0.4, cHair), // the cap
			cbox(vec{-0.135, 0.875, 0.09}, vec{0.135, 0.93, 0.142}, 0.3, cHair),    // the bangs
			cbox(vec{-0.15, 0.74, -0.14}, vec{-0.12, 0.93, 0.07}, 0.2, cHair),      // the sides
			cbox(vec{0.12, 0.74, -0.14}, vec{0.15, 0.93, 0.07}, 0.2, cHair),
			cbox(vec{-0.07, 0.97, -0.05}, vec{0.01, 1.02, 0.06}, 0.4, cHair), // locks on top
			cbox(vec{0.02, 0.97, -0.09}, vec{0.09, 1.01, 0.02}, 0.4, cHair),
			cbox(vec{-0.13, 0.84, -0.155}, vec{0.13, 0.98, -0.12}, 0.2, cHair), // the back of the head
		)
		if hair == HairLong {
			s = append(s, cbox(vec{-0.13, 0.5, -0.165}, vec{0.13, 0.9, -0.115}, 0.25, cHair))
		}
		if hair == HairBun {
			s = append(s, cbox(vec{-0.06, 0.98, -0.12}, vec{0.06, 1.08, 0.0}, 0.6, cHair))
		}
	}
	for k, p := range parts {
		col := cPart + k
		switch p.Kind {
		case "apron":
			s = append(s, box(vec{-0.1, 0.2, 0.068}, vec{0.1, 0.62, 0.082}, col))
		case "cape":
			s = append(s, cbox(vec{-0.15, 0.08, -0.12}, vec{0.15, 0.65, -0.075}, 0.2, col))
		case "robe":
			s = append(s, frustum(0.06, 0.42, [2]float64{0.12, 0.075}, [2]float64{0.17, 0.11}, col),
				cbox(vec{-0.15, 0.08, -0.12}, vec{0.15, 0.65, -0.075}, 0.2, col))
		case "skirt":
			s = append(s, frustum(0.14, 0.42, [2]float64{0.12, 0.075}, [2]float64{0.155, 0.1}, col))
		case "dress":
			s = append(s, frustum(0.07, 0.42, [2]float64{0.12, 0.075}, [2]float64{0.16, 0.105}, col))
		case "coat":
			s = append(s, frustum(0.2, 0.42, [2]float64{0.125, 0.078}, [2]float64{0.15, 0.095}, col))
		case "shield":
			s = append(s, limbOf(box(vec{0.205, 0.3, -0.11}, vec{0.232, 0.64, 0.11}, col), 4, shR),
				limbOf(box(vec{0.232, 0.43, -0.03}, vec{0.245, 0.51, 0.03}, col), 4, shR))
		case "staff":
			s = append(s, limbOf(box(vec{-0.18, 0.0, 0.02}, vec{-0.155, 0.98, 0.045}, col), 3, shL))
		case "glasses":
			for _, sg := range []float64{-1, 1} {
				a, b := span(sg, 0.018, 0.09)
				s = append(s, box(vec{a, 0.838, 0.13}, vec{b, 0.848, 0.142}, col),
					box(vec{a, 0.778, 0.13}, vec{b, 0.786, 0.142}, col),
					box(vec{sg*0.09 - 0.005, 0.778, 0.13}, vec{sg*0.09 + 0.005, 0.848, 0.142}, col))
			}
		case "hat":
			s = append(s, box(vec{-0.2, 0.93, -0.2}, vec{0.2, 0.95, 0.2}, col),
				cbox(vec{-0.12, 0.95, -0.12}, vec{0.12, 1.06, 0.12}, 0.3, col))
		case "hood":
			s = append(s, box(vec{-0.165, 0.97, -0.17}, vec{0.165, 1.01, 0.13}, col),
				box(vec{-0.165, 0.64, -0.17}, vec{0.165, 1.01, -0.13}, col),
				box(vec{-0.165, 0.66, -0.17}, vec{-0.138, 1.01, 0.13}, col),
				box(vec{0.138, 0.66, -0.17}, vec{0.165, 1.01, 0.13}, col))
		case "bag":
			s = append(s, cbox(vec{0.12, 0.33, -0.04}, vec{0.19, 0.45, 0.07}, 0.3, col),
				box(vec{-0.12, 0.45, 0.07}, vec{0.13, 0.47, 0.08}, col))
		case "goggles":
			s = append(s, box(vec{-0.146, 0.9, -0.146}, vec{0.146, 0.935, 0.146}, cEye))
			for _, sg := range []float64{-1, 1} {
				a, b := span(sg, 0.02, 0.09)
				s = append(s, cbox(vec{a, 0.89, 0.12}, vec{b, 0.948, 0.16}, 0.5, col))
			}
		case "muffler":
			s = append(s, cbox(vec{-0.11, 0.62, -0.08}, vec{0.11, 0.7, 0.09}, 0.4, col),
				box(vec{0.03, 0.48, 0.07}, vec{0.08, 0.63, 0.095}, col))
		case "shawl":
			s = append(s, cbox(vec{-0.21, 0.57, -0.09}, vec{0.21, 0.67, 0.09}, 0.4, col),
				box(vec{-0.21, 0.4, -0.08}, vec{-0.19, 0.6, 0.06}, col),
				box(vec{0.19, 0.4, -0.08}, vec{0.21, 0.6, 0.06}, col))
		case "ponytail":
			s = append(s, cbox(vec{-0.035, 0.6, -0.21}, vec{0.035, 0.94, -0.14}, 0.4, cHair))
		case "lashes":
			for _, sg := range []float64{-1, 1} {
				a, b := span(sg, 0.024, 0.088)
				s = append(s, box(vec{a, 0.834, 0.112}, vec{b, 0.846, 0.138}, cEye))
			}
		}
	}
	return s
}

// swingCS turns v about the x axis through the pivot by the angle of cosine c and sine s.
func swingCS(v, pivot vec, c, s float64) vec {
	y, z := v[1]-pivot[1], v[2]-pivot[2]
	return vec{v[0], pivot[1] + c*y - s*z, pivot[2] + s*y + c*z}
}

// limbAngle is how far a shape turns for the walk's leg angle.
func limbAngle(s solid, leg float64) float64 {
	switch s.limb {
	case 1:
		return leg
	case 2:
		return -leg
	case 3:
		return -0.8 * leg
	case 4:
		return 0.8 * leg
	}
	return 0
}

// posed is a solid in one picture of a sheet, with what all the picture's rays share (they
// are parallel) worked out once. A turned limb meets the rays turned back the other way.
type posed struct {
	s      *solid
	turned bool
	cb, sb float64   // the cosine and sine that turn a ray into the limb's own frame
	cf, sf float64   // and those that turn a normal back
	ld     vec       // the rays' direction in the solid's frame
	den    []float64 // each plane's normal along ld
	x0, x1 int       // the pixels whose rays may meet the solid's sphere
	y0, y1 int
}

func posePart(s *solid, d vec, leg float64, den []float64) posed {
	p := posed{s: s, ld: d, den: den[:0]}
	if a := limbAngle(*s, leg); a != 0 {
		p.turned = true
		p.cb, p.sb = math.Cos(-a), math.Sin(-a)
		p.cf, p.sf = math.Cos(a), math.Sin(a)
		p.ld = swingCS(d, vec{}, p.cb, p.sb)
	}
	for _, pl := range s.planes {
		p.den = append(p.den, dot(vec{pl[0], pl[1], pl[2]}, p.ld))
	}
	return p
}

// hit is where the ray from o (along the picture's direction) enters the solid, and the
// normal there; t < 0 when it misses.
func (p *posed) hit(o vec) (float64, vec) {
	s := p.s
	lo := o
	if p.turned {
		lo = swingCS(o, s.pivot, p.cb, p.sb)
	}
	q := sub(lo, s.a)
	if bq := dot(q, p.ld); bq*bq-(dot(q, q)-s.r*s.r) < 0 {
		return -1, vec{} // the ray misses the solid's sphere
	}
	tin, tout := -math.MaxFloat64, math.MaxFloat64
	var nin vec
	for i, pl := range s.planes {
		den := p.den[i]
		num := pl[3] - (pl[0]*q[0] + pl[1]*q[1] + pl[2]*q[2])
		if math.Abs(den) < 1e-12 {
			if num < 0 {
				return -1, nin
			}
			continue
		}
		t := num / den
		if den < 0 {
			if t > tin {
				tin, nin = t, vec{pl[0], pl[1], pl[2]}
			}
		} else if t < tout {
			tout = t
		}
		if tin > tout {
			return -1, nin
		}
	}
	if tin < 0 {
		return -1, nin
	}
	if p.turned {
		nin = swingCS(nin, vec{}, p.cf, p.sf)
	}
	return tin, nin
}

// Bake draws a doll from Sides sides in Poses poses, fw x fh pixels each, seen from elev
// degrees above (a fixed camera looking down on a painted background). Its feet are at the
// bottom of each picture, the top of the head at about 0.9 of the height.
func Bake(pal Palette, hair string, parts []Part, fw, fh int, elev float64) *image.RGBA {
	b := NewBaker(pal, hair, parts, fw, fh, elev)
	for !b.Step() {
	}
	return b.Sheet()
}

// Baker bakes a doll's sheet a picture at a time (Step), so that a program with one
// thread (a browser's) can draw its frames between the pictures.
type Baker struct {
	colors []color.RGBA
	solids []solid
	sheet  *image.RGBA
	fw, fh int
	el     float64
	next   int // the next picture: side*Poses + pose

	depth []float64
	frame *image.RGBA
	posed []posed
	dens  [][]float64
	rows  [][]int // the solids each row of pixels may meet
}

// NewBaker starts baking a doll; see Bake.
func NewBaker(pal Palette, hair string, parts []Part, fw, fh int, elev float64) *Baker {
	if hair == HairBald {
		pal.Skin = pal.Hair // the top of a bald head is the skin (the face may be a beard)
	}
	colors := paletteColors(pal)
	for _, p := range parts {
		colors = append(colors, p.Color)
	}
	solids := shapes(hair, parts)
	return &Baker{
		colors: colors, solids: solids, sheet: image.NewRGBA(image.Rect(0, 0, fw*Sides, fh*Poses)),
		fw: fw, fh: fh, el: elev * math.Pi / 180,
		depth: make([]float64, fw*fh), frame: image.NewRGBA(image.Rect(0, 0, fw, fh)),
		posed: make([]posed, len(solids)), dens: make([][]float64, len(solids)), rows: make([][]int, fh),
	}
}

// Sheet is the sheet baked so far (all of it once Step has said it is done).
func (b *Baker) Sheet() *image.RGBA { return b.sheet }

// Step bakes the next picture of the sheet and reports whether the sheet is done.
func (b *Baker) Step() bool {
	if b.next >= Sides*Poses {
		return true
	}
	side, pose := b.next/Poses, b.next%Poses
	b.next++
	b.picture(side, pose)
	return b.next >= Sides*Poses
}

func (b *Baker) picture(side, pose int) {
	fw, fh, el := b.fw, b.fh, b.el
	dc, dr := vec{0, 0.55, 0}, 0.7 // the whole doll lies in this sphere
	k := unitsH / float64(fh)
	light := vec{-0.45, 0.75, 0.5}
	light = scale(light, 1/length(light))
	th := float64(side) * math.Pi / 4
	cam := vec{math.Sin(th) * math.Cos(el), math.Sin(el), math.Cos(th) * math.Cos(el)}
	right := vec{math.Cos(th), 0, -math.Sin(th)}
	up := vec{cam[1]*right[2] - cam[2]*right[1], cam[2]*right[0] - cam[0]*right[2], cam[0]*right[1] - cam[1]*right[0]}
	d := scale(cam, -1)
	// the light turns with the camera: always from the viewer's upper left
	lw := vec{
		right[0]*light[0] + up[0]*light[1] + cam[0]*light[2],
		right[1]*light[0] + up[1]*light[1] + cam[1]*light[2],
		right[2]*light[0] + up[2]*light[1] + cam[2]*light[2],
	}
	leg := 0.0
	if pose > 0 {
		leg = 0.5 * math.Sin(float64(pose-1)*math.Pi/2+math.Pi/2)
	}
	depth, frame, rows := b.depth, b.frame, b.rows
	for i := range depth {
		depth[i] = math.Inf(1)
	}
	clear(frame.Pix)
	for i := range rows {
		rows[i] = rows[i][:0]
	}
	// The rays are parallel, so a solid's sphere covers a disc of the picture: only the
	// pixels around it (with a pixel to spare) try the solid.
	half := float64(fw)/2 - 0.5
	for i := range b.solids {
		s := &b.solids[i]
		p := posePart(s, d, leg, b.dens[i])
		b.dens[i] = p.den
		c := s.a
		if p.turned {
			c = swingCS(c, s.pivot, p.cf, p.sf)
		}
		cu, cv, r := dot(c, right)/k, dot(c, up)/k, s.r/k
		p.x0, p.x1 = int(math.Floor(cu+half-r))-1, int(math.Ceil(cu+half+r))+1
		p.y0, p.y1 = int(math.Floor(float64(fh)-0.5-cv-r))-1, int(math.Ceil(float64(fh)-0.5-cv+r))+1
		b.posed[i] = p
		for y := max(0, p.y0); y <= min(fh-1, p.y1); y++ {
			rows[y] = append(rows[y], i)
		}
	}
	for py := range fh {
		for px := range fw {
			u := (float64(px) + 0.5 - float64(fw)/2) * k
			v := (float64(fh) - float64(py) - 0.5) * k
			o := vec{right[0]*u + up[0]*v + cam[0]*2, right[1]*u + up[1]*v + cam[1]*2, right[2]*u + up[2]*v + cam[2]*2}
			oc := sub(o, dc)
			bq := dot(oc, d)
			if bq*bq-(dot(oc, oc)-dr*dr) < 0 {
				continue // the ray misses the doll's sphere
			}
			best, ci := math.MaxFloat64, 0
			var bn vec
			for _, i := range rows[py] {
				p := &b.posed[i]
				if px < p.x0 || px > p.x1 {
					continue
				}
				if t, n := p.hit(o); t >= 0 && t < best {
					best, ci, bn = t, p.s.col, n
				}
			}
			if best < math.MaxFloat64 {
				frame.SetRGBA(px, py, shade(b.colors[ci], 0.62+0.38*math.Max(0, dot(bn, lw))))
				depth[py*fw+px] = best
			}
		}
	}
	// a thin outline: at the silhouette and where the depth jumps (an arm before the body)
	for py := range fh {
		for px := range fw {
			c := frame.RGBAAt(px, py)
			if c.A == 0 {
				continue
			}
			z := depth[py*fw+px]
			for _, o := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				x, y := px+o[0], py+o[1]
				if x < 0 || y < 0 || x >= fw || y >= fh || frame.RGBAAt(x, y).A == 0 || depth[y*fw+x]-z > 0.06 {
					c = shade(c, 0.8)
					break
				}
			}
			b.sheet.SetRGBA(side*fw+px, pose*fh+py, c)
		}
	}
}

// Side is which side of a baked sheet the camera sees of someone facing (fx, fy) on the
// screen (y down: toward the camera).
func Side(fx, fy float64) int {
	heading := math.Atan2(fx, fy)
	rel := math.Remainder(-heading, 2*math.Pi)
	return (int(math.Round(rel/(math.Pi/4))) + Sides) % Sides
}

// Pose is the row of a sheet: 0 standing, 1..4 the walk at its phase (radians).
func Pose(moving bool, phase float64) int {
	if !moving {
		return 0
	}
	return 1 + int(phase/(math.Pi/2))%4
}
