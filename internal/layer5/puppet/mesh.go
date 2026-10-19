package puppet

import (
	"math"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// Mesh is a grid over the base picture. Each vertex keeps its source position and
// how much it belongs to the head and to the hair, so deforming is a cheap loop.
type Mesh struct {
	Cols, Rows int // vertices across and down
	Src        [][2]float32
	Index      []uint16
	head, hair []float64
	w, h       float64
	center     [2]float64
	neck       [2]float64
}

// maxCells keeps the vertex count far below the uint16 index limit.
const maxCells = 48

// NewMesh builds the grid for a rig. Cells are about square, at most 48 across
// the longer side and at least 8 pixels.
func NewMesh(r *Rig) *Mesh {
	wi, hi := r.Size()
	w, h := float64(wi), float64(hi)
	step := math.Max(8, math.Max(w, h)/maxCells)
	cols := int(math.Ceil(w/step)) + 1
	rows := int(math.Ceil(h/step)) + 1
	m := &Mesh{Cols: cols, Rows: rows, w: w, h: h}
	for j := range rows {
		for i := range cols {
			x := math.Min(w, float64(i)*w/float64(cols-1))
			y := math.Min(h, float64(j)*h/float64(rows-1))
			m.Src = append(m.Src, [2]float32{float32(x), float32(y)})
		}
	}
	for j := range rows - 1 {
		for i := range cols - 1 {
			a := uint16(j*cols + i)                          //nolint:gosec // At most 49x49 vertices (maxCells), far below 65536.
			b, c, d := a+1, a+uint16(cols), a+uint16(cols)+1 //nolint:gosec // cols is at most 49.
			m.Index = append(m.Index, a, b, c, b, d, c)
		}
	}
	m.head = make([]float64, len(m.Src))
	m.hair = make([]float64, len(m.Src))
	if hd := r.Spec.Head; hd != nil {
		m.center = [2]float64{hd.Center[0], hd.Center[1]}
		m.neck = [2]float64{hd.Neck[0], hd.Neck[1]}
		for k, s := range m.Src {
			m.head[k] = headWeight(float64(s[0]), float64(s[1]), hd)
			m.hair[k] = hairWeight(float64(s[0]), float64(s[1]), r.Spec.Hair, hd.Radius)
		}
	}
	return m
}

// headWeight is 1 inside the head circle, falls to 0 at 1.35 radius, and fades
// out toward the neck root so the neck bends instead of breaking.
func headWeight(x, y float64, hd *puppetdef.Head) float64 {
	r := hd.Radius
	d := math.Hypot(x-hd.Center[0], y-hd.Center[1])
	w := 1 - smooth((d-r)/(0.35*r))
	below := smooth((hd.Neck[1] - y) / (0.4 * r))
	return w * below
}

// hairWeight is how much a point follows the hair spring: inside a hair shape
// (with a soft edge), more toward the bottom of the shape where the hair tips are.
func hairWeight(x, y float64, shapes []puppetdef.Shape, radius float64) float64 {
	feather := 0.08 * radius
	best := 0.0
	for _, s := range shapes {
		var inside, top, bottom float64
		if s.Rect != nil {
			rx, ry, rw, rh := s.Rect[0], s.Rect[1], s.Rect[2], s.Rect[3]
			dx := math.Max(rx-x, x-(rx+rw))
			dy := math.Max(ry-y, y-(ry+rh))
			inside = 1 - smooth(math.Max(dx, dy)/feather+1)
			top, bottom = ry, ry+rh
		} else {
			d := polyDist(x, y, s.Poly)
			inside = 1 - smooth(d/feather+1)
			top, bottom = s.Poly[0][1], s.Poly[0][1]
			for _, p := range s.Poly {
				top, bottom = math.Min(top, p[1]), math.Max(bottom, p[1])
			}
		}
		depth := 0.15 + 0.85*smooth((y-top)/math.Max(1, bottom-top))
		best = math.Max(best, inside*depth)
	}
	return best
}

// polyDist is the signed distance to a polygon edge, negative inside.
func polyDist(x, y float64, poly [][]float64) float64 {
	d := math.Inf(1)
	in := false
	for i := range poly {
		a, b := poly[i], poly[(i+1)%len(poly)]
		if (a[1] > y) != (b[1] > y) && x < (b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
		ex, ey := b[0]-a[0], b[1]-a[1]
		t := clamp(((x-a[0])*ex+(y-a[1])*ey)/math.Max(1e-9, ex*ex+ey*ey), 0, 1)
		d = math.Min(d, math.Hypot(x-a[0]-t*ex, y-a[1]-t*ey))
	}
	if in {
		return -d
	}
	return d
}

// headPoint moves a point fully with the head: squash toward the neck, turn about
// the neck, then shift.
func headPoint(p, neck [2]float64, pose Pose) [2]float64 {
	qx, qy := p[0]-neck[0], (p[1]-neck[1])*(1-pose.HeadSquash)
	sin, cos := math.Sincos(pose.HeadAngle)
	return [2]float64{neck[0] + qx*cos - qy*sin + pose.HeadShift[0], neck[1] + qx*sin + qy*cos + pose.HeadShift[1]}
}

// Deform writes the deformed position of every vertex into dst (len(Src)).
func (m *Mesh) Deform(pose Pose, dst [][2]float32) {
	cx, bottom := m.w/2, m.h
	for k, s := range m.Src {
		p := [2]float64{float64(s[0]), float64(s[1])}
		if w := m.head[k]; w > 0 {
			q := headPoint(p, m.neck, pose)
			p[0] += w * (q[0] - p[0])
			p[1] += w * (q[1] - p[1])
		}
		if w := m.hair[k]; w > 0 {
			// Hair outside the head circle follows the head through the spring too.
			f := 1 - m.head[k]
			p[0] += w * (pose.Hair[0] + f*pose.Anchor[0])
			p[1] += w * (pose.Hair[1] + f*pose.Anchor[1])
		}
		p[1] = bottom - (bottom-p[1])*(1+pose.Breath)
		p[0] = cx + (p[0]-cx)*pose.BodyScale[0] + pose.Body[0]
		p[1] = bottom + (p[1]-bottom)*pose.BodyScale[1] + pose.Body[1]
		dst[k] = [2]float32{float32(p[0]), float32(p[1])}
	}
}
