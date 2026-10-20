package puppet

import (
	"math"

	"github.com/nao1215/gozen/internal/layer2/puppetdef"
)

// spring is one resolved swaying part. The hair, when given, is the first one:
// all its shapes share one spring and the defaults below are the hair's.
type spring struct {
	shapes    []puppetdef.Shape
	side      string     // top, bottom, left or right; empty when root is a point
	root      [2]float64 // the point the part hangs from (the middle of the side for an edge)
	head      bool       // follows the head (else the body)
	mask      [2]float64 // 1 for an axis that sways, 0 for one that does not
	k, c      float64    // stiffness and damping
	gain      float64    // lag multiplier
	limit     float64    // largest lag in pixels
	feather   float64    // soft edge of the shapes in pixels
	reachSide float64    // for a point root: the farthest shape point from it
}

// unitOf is the size the default strengths scale with: the head radius, or a
// fifth of the height without a head.
func unitOf(s *puppetdef.Spec, h int) float64 {
	if s.Head != nil {
		return s.Head.Radius
	}
	return float64(h) / 5
}

// resolveSprings lists the swaying parts in drawing order (later ones lie over
// earlier ones). Parts that cannot move (a zero gain) are left out.
func resolveSprings(s *puppetdef.Spec, h int) []spring {
	unit := unitOf(s, h)
	var out []spring
	add := func(sp spring) {
		if sp.gain == 0 || sp.mask == [2]float64{} {
			return
		}
		sp.feather = 0.08 * unit
		top, bottom, left, right := bounds(sp.shapes)
		switch sp.side {
		case "top":
			sp.root = [2]float64{(left + right) / 2, top}
		case "bottom":
			sp.root = [2]float64{(left + right) / 2, bottom}
		case "left":
			sp.root = [2]float64{left, (top + bottom) / 2}
		case "right":
			sp.root = [2]float64{right, (top + bottom) / 2}
		default:
			for _, sh := range sp.shapes {
				for _, q := range corners(sh) {
					sp.reachSide = math.Max(sp.reachSide, math.Hypot(q[0]-sp.root[0], q[1]-sp.root[1]))
				}
			}
		}
		out = append(out, sp)
	}
	if len(s.Hair) > 0 && s.Head != nil {
		sp := spring{shapes: s.Hair, side: "top", head: true, mask: [2]float64{1, 1}, k: 60, c: 7, gain: 1, limit: 0.15 * unit}
		if hs := s.Motion.Hair; hs != nil {
			sp.gain = hs.Gain
			if hs.Stiffness > 0 {
				sp.k = hs.Stiffness
			}
			if hs.Damping > 0 {
				sp.c = hs.Damping
			}
		}
		add(sp)
	}
	for _, p := range s.Springs {
		sp := spring{
			shapes: []puppetdef.Shape{{Rect: p.Rect, Poly: p.Poly}}, side: p.Root.Side, head: p.Follow != "body",
			mask: [2]float64{1, 1}, k: 60, c: 7, gain: 1, limit: 0.15 * unit,
		}
		if p.Root.At != nil {
			sp.side = ""
			sp.root = [2]float64{p.Root.At[0], p.Root.At[1]}
		} else if sp.side == "" {
			sp.side = "top"
		}
		switch p.Axis {
		case "x":
			sp.mask = [2]float64{1, 0}
		case "y":
			sp.mask = [2]float64{0, 1}
		}
		if p.Stiffness > 0 {
			sp.k = p.Stiffness
		}
		if p.Damping > 0 {
			sp.c = p.Damping
		}
		if p.Amount > 0 {
			sp.gain = p.Amount
		}
		if p.Limit > 0 {
			sp.limit = p.Limit
		}
		add(sp)
	}
	return out
}

func corners(sh puppetdef.Shape) [][2]float64 {
	if sh.Rect != nil {
		x, y, w, h := sh.Rect[0], sh.Rect[1], sh.Rect[2], sh.Rect[3]
		return [][2]float64{{x, y}, {x + w, y}, {x, y + h}, {x + w, y + h}}
	}
	out := make([][2]float64, len(sh.Poly))
	for i, q := range sh.Poly {
		out[i] = [2]float64{q[0], q[1]}
	}
	return out
}

// bounds is the box around shapes.
func bounds(shapes []puppetdef.Shape) (top, bottom, left, right float64) {
	top, left = math.Inf(1), math.Inf(1)
	bottom, right = math.Inf(-1), math.Inf(-1)
	for _, sh := range shapes {
		for _, q := range corners(sh) {
			left, right = math.Min(left, q[0]), math.Max(right, q[0])
			top, bottom = math.Min(top, q[1]), math.Max(bottom, q[1])
		}
	}
	return top, bottom, left, right
}

// weight is how much a point follows the spring (inside the shapes with a soft
// edge, more toward the far end from the root) and how much it is covered by the
// part, which decides how it lies over earlier parts.
func (sp *spring) weight(x, y float64) (w, cover float64) {
	for _, s := range sp.shapes {
		var inside, top, bottom, left, right float64
		if s.Rect != nil {
			rx, ry, rw, rh := s.Rect[0], s.Rect[1], s.Rect[2], s.Rect[3]
			dx := math.Max(rx-x, x-(rx+rw))
			dy := math.Max(ry-y, y-(ry+rh))
			inside = 1 - smooth(math.Max(dx, dy)/sp.feather+1)
			top, bottom, left, right = ry, ry+rh, rx, rx+rw
		} else {
			d := polyDist(x, y, s.Poly)
			inside = 1 - smooth(d/sp.feather+1)
			top, bottom, left, right = bounds([]puppetdef.Shape{s})
		}
		var u float64
		switch sp.side {
		case "top":
			u = (y - top) / math.Max(1, bottom-top)
		case "bottom":
			u = (bottom - y) / math.Max(1, bottom-top)
		case "left":
			u = (x - left) / math.Max(1, right-left)
		case "right":
			u = (right - x) / math.Max(1, right-left)
		default:
			u = math.Hypot(x-sp.root[0], y-sp.root[1]) / math.Max(1, sp.reachSide)
		}
		depth := 0.15 + 0.85*smooth(u)
		w = math.Max(w, inside*depth)
		cover = math.Max(cover, inside)
	}
	return w, cover
}

// springState is the moving end of one spring.
type springState struct{ pos, vel [2]float64 }

// target is where the spring's end rests for pose p: the head center's move plus
// the body's shift for a head part, the root's move with the body for a body part.
func (m *Motion) target(sp *spring, p Pose, anchor [2]float64) [2]float64 {
	if sp.head {
		return [2]float64{anchor[0] + p.Body[0], anchor[1] + p.Body[1]}
	}
	q := bodyPoint(sp.root, m.w, m.h, p)
	return [2]float64{q[0] - sp.root[0], q[1] - sp.root[1]}
}

// lag is the spring's offset from where it rests, scaled, capped and masked.
func (m *Motion) lag(sp *spring, st springState, p Pose) [2]float64 {
	var out [2]float64
	var rest [2]float64
	if !sp.head {
		rest = m.target(sp, p, [2]float64{})
	}
	for i := range 2 {
		var l float64
		if sp.head {
			l = st.pos[i] - p.Anchor[i] - p.Body[i]
		} else {
			l = st.pos[i] - rest[i]
		}
		out[i] = clamp(l*sp.gain, -sp.limit, sp.limit) * sp.mask[i]
	}
	return out
}

func (m *Motion) stepSprings(dt float64) {
	if len(m.springs) == 0 {
		return
	}
	p := m.pose(false)
	var a [2]float64
	if m.p.hasHead {
		a = m.anchor(p)
	}
	for i := range m.springs {
		sp := &m.springs[i]
		st := &m.state[i]
		target := m.target(sp, p, a)
		// Semi-implicit Euler is stable while h*sqrt(k) and h*c stay small; split
		// the update for stiff springs or a low TPS. The hair's defaults take one step.
		n := max(1, int(math.Ceil(math.Max(dt*math.Sqrt(sp.k), dt*sp.c)/0.5)))
		h := dt / float64(n)
		for range n {
			for j := range 2 {
				acc := sp.k*(target[j]-st.pos[j]) - sp.c*st.vel[j]
				st.vel[j] += acc * h
				st.pos[j] += st.vel[j] * h
			}
		}
	}
}

// bodyPoint moves a point with the whole body: the breath stretches it up from
// the bottom, then the body scales about the bottom center and shifts.
func bodyPoint(q [2]float64, w, h float64, pose Pose) [2]float64 {
	cx, bottom := w/2, h
	x, y := q[0], q[1]
	y = bottom - (bottom-y)*(1+pose.Breath)
	x = cx + (x-cx)*pose.BodyScale[0] + pose.Body[0]
	y = bottom + (y-bottom)*pose.BodyScale[1] + pose.Body[1]
	return [2]float64{x, y}
}
