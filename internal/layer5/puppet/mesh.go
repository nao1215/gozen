package puppet

import (
	"math"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// Mesh is a grid over the base picture. Each vertex keeps its source position and
// how much it belongs to the head and to each swaying part, so deforming is a cheap loop.
type Mesh struct {
	Cols, Rows int // vertices across and down
	Src        [][2]float32
	Index      []uint16
	head       []float64
	parts      []meshPart
	w, h       float64
	center     [2]float64
	neck       [2]float64
}

// meshPart is one swaying part's weight and cover at every vertex, and whether
// it follows the head.
type meshPart struct {
	w, cover []float64
	head     bool
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
	if hd := r.Spec.Head; hd != nil {
		m.center = [2]float64{hd.Center[0], hd.Center[1]}
		m.neck = [2]float64{hd.Neck[0], hd.Neck[1]}
		for k, s := range m.Src {
			m.head[k] = headWeight(float64(s[0]), float64(s[1]), hd)
		}
	}
	for _, sp := range resolveSprings(r.Spec, hi) {
		part := meshPart{w: make([]float64, len(m.Src)), cover: make([]float64, len(m.Src)), head: sp.head}
		for k, s := range m.Src {
			part.w[k], part.cover[k] = sp.weight(float64(s[0]), float64(s[1]))
		}
		m.parts = append(m.parts, part)
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
	for k, s := range m.Src {
		p := [2]float64{float64(s[0]), float64(s[1])}
		if w := m.head[k]; w > 0 {
			q := headPoint(p, m.neck, pose)
			p[0] += w * (q[0] - p[0])
			p[1] += w * (q[1] - p[1])
		}
		// Each part lies over the earlier ones by its cover, so where parts overlap
		// the later one decides; a lone part adds weight times its lag.
		var d [2]float64
		moved := false
		for i, part := range m.parts {
			c := part.cover[k]
			if c == 0 {
				continue
			}
			moved = true
			w := part.w[k]
			v := pose.Lag[i]
			if part.head {
				// A part outside the head circle follows the head through the spring too.
				f := 1 - m.head[k]
				v[0] += f * pose.Anchor[0]
				v[1] += f * pose.Anchor[1]
			}
			d[0] = d[0]*(1-c) + w*v[0]
			d[1] = d[1]*(1-c) + w*v[1]
		}
		if moved {
			p[0] += d[0]
			p[1] += d[1]
		}
		p = bodyPoint(p, m.w, m.h, pose)
		dst[k] = [2]float32{float32(p[0]), float32(p[1])}
	}
}
