// Package geom is the plane geometry of painted scenes and maps: points and polygons in a
// picture's pixels, whether a point is inside, and distances. The rules (checks, the walk
// between points) and the screen share it; it knows nothing of games.
package geom

import (
	"math"
	"slices"
)

// Pt is a point of a picture, in its pixels (y down).
type Pt struct{ X, Y float64 }

// Add is p + q.
func (p Pt) Add(q Pt) Pt { return Pt{p.X + q.X, p.Y + q.Y} }

// Sub is p - q.
func (p Pt) Sub(q Pt) Pt { return Pt{p.X - q.X, p.Y - q.Y} }

// Scale is p times k.
func (p Pt) Scale(k float64) Pt { return Pt{p.X * k, p.Y * k} }

// Len is the distance from the origin.
func (p Pt) Len() float64 { return math.Hypot(p.X, p.Y) }

// Dist is the distance between p and q.
func Dist(p, q Pt) float64 { return math.Hypot(p.X-q.X, p.Y-q.Y) }

// Poly is a closed polygon (the last point joins the first).
type Poly []Pt

// Inside reports whether p is in the polygon (the even-odd rule; a polygon of fewer than
// three points has no inside).
func (q Poly) Inside(p Pt) bool {
	if len(q) < 3 {
		return false
	}
	in := false
	for i, j := 0, len(q)-1; i < len(q); j, i = i, i+1 {
		a, b := q[i], q[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// Center is the mean of the polygon's points.
func (q Poly) Center() Pt {
	var c Pt
	for _, p := range q {
		c = c.Add(p)
	}
	if len(q) == 0 {
		return c
	}
	return c.Scale(1 / float64(len(q)))
}

// Bounds is the smallest box around the polygon: its top-left and bottom-right corners.
func (q Poly) Bounds() (Pt, Pt) {
	if len(q) == 0 {
		return Pt{}, Pt{}
	}
	lo, hi := q[0], q[0]
	for _, p := range q[1:] {
		lo = Pt{math.Min(lo.X, p.X), math.Min(lo.Y, p.Y)}
		hi = Pt{math.Max(hi.X, p.X), math.Max(hi.Y, p.Y)}
	}
	return lo, hi
}

// Distance is how far p is from the polygon: 0 inside, else the distance to its nearest edge.
func (q Poly) Distance(p Pt) float64 {
	if q.Inside(p) {
		return 0
	}
	best := math.Inf(1)
	for i := range q {
		best = math.Min(best, SegmentDistance(p, q[i], q[(i+1)%len(q)]))
	}
	return best
}

// SegmentDistance is the distance from p to the segment from a to b.
func SegmentDistance(p, a, b Pt) float64 {
	d := b.Sub(a)
	l := d.X*d.X + d.Y*d.Y
	if l == 0 {
		return Dist(p, a)
	}
	t := math.Max(0, math.Min(1, ((p.X-a.X)*d.X+(p.Y-a.Y)*d.Y)/l))
	return Dist(p, a.Add(d.Scale(t)))
}

// SelfCrossing reports whether two edges of the polygon that do not share a point cross.
func (q Poly) SelfCrossing() bool {
	n := len(q)
	for i := range n {
		a, b := q[i], q[(i+1)%n]
		for j := i + 2; j < n; j++ {
			if i == 0 && j == n-1 {
				continue // the edges meet at q[0]
			}
			if segmentsCross(a, b, q[j], q[(j+1)%n]) {
				return true
			}
		}
	}
	return false
}

// Touches reports whether two polygons overlap: a point of one is in the other, or edges cross.
func (q Poly) Touches(r Poly) bool {
	if slices.ContainsFunc(q, r.Inside) {
		return true
	}
	if slices.ContainsFunc(r, q.Inside) {
		return true
	}
	for i := range q {
		for j := range r {
			if segmentsCross(q[i], q[(i+1)%len(q)], r[j], r[(j+1)%len(r)]) {
				return true
			}
		}
	}
	return false
}

func cross(o, a, b Pt) float64 { return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X) }

// segmentsCross reports whether the segments ab and cd cross at a point inside both.
func segmentsCross(a, b, c, d Pt) bool {
	d1, d2 := cross(c, d, a), cross(c, d, b)
	d3, d4 := cross(a, b, c), cross(a, b, d)
	return (d1 > 0) != (d2 > 0) && (d3 > 0) != (d4 > 0) && d1 != 0 && d2 != 0 && d3 != 0 && d4 != 0
}

// Polyline is an open line through points (a road).
type Polyline []Pt

// Length is the line's length.
func (l Polyline) Length() float64 {
	s := 0.0
	for i := 1; i < len(l); i++ {
		s += Dist(l[i-1], l[i])
	}
	return s
}

// At is the point at the distance d along the line (clamped to its ends) and the line's
// direction there (a unit vector; zero for a line of one point).
func (l Polyline) At(d float64) (Pt, Pt) {
	if len(l) == 0 {
		return Pt{}, Pt{}
	}
	for i := 1; i < len(l); i++ {
		seg := Dist(l[i-1], l[i])
		if seg == 0 {
			continue
		}
		dir := l[i].Sub(l[i-1]).Scale(1 / seg)
		if d <= seg || i == len(l)-1 {
			d = math.Max(0, math.Min(seg, d))
			return l[i-1].Add(dir.Scale(d)), dir
		}
		d -= seg
	}
	return l[len(l)-1], Pt{}
}
