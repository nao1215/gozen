package geom

import (
	"math"
	"testing"
)

func TestPointArithmeticAndLength(t *testing.T) {
	p := Pt{3, 4}
	if p.Len() != 5 {
		t.Errorf("Len = %v", p.Len())
	}
	if got := p.Add(Pt{1, 1}).Sub(Pt{4, 5}); got != (Pt{}) {
		t.Errorf("Add/Sub = %v", got)
	}
	if got := p.Scale(2); got != (Pt{6, 8}) {
		t.Errorf("Scale = %v", got)
	}
	if Dist(Pt{1, 1}, Pt{4, 5}) != 5 {
		t.Error("Dist")
	}
}

func TestEmptyPolygons(t *testing.T) {
	var q Poly
	if q.Center() != (Pt{}) {
		t.Error("the center of nothing is the origin")
	}
	lo, hi := q.Bounds()
	if lo != (Pt{}) || hi != (Pt{}) {
		t.Error("the bounds of nothing are the origin")
	}
	if q.Inside(Pt{}) || q.SelfCrossing() || q.Touches(Poly{{0, 0}, {1, 0}, {0, 1}}) {
		t.Error("an empty polygon contains, crosses and touches nothing")
	}
}

func TestBoundsAndCenter(t *testing.T) {
	q := Poly{{2, 8}, {10, 1}, {6, 12}, {-4, 5}}
	lo, hi := q.Bounds()
	if lo != (Pt{-4, 1}) || hi != (Pt{10, 12}) {
		t.Errorf("bounds %v %v", lo, hi)
	}
	if c := q.Center(); c != (Pt{3.5, 6.5}) {
		t.Errorf("center %v", c)
	}
	one := Poly{{7, 7}}
	if lo, hi := one.Bounds(); lo != hi || lo != (Pt{7, 7}) {
		t.Errorf("one point: %v %v", lo, hi)
	}
}

func TestSegmentDistanceOfADegenerateSegment(t *testing.T) {
	if got := SegmentDistance(Pt{3, 4}, Pt{0, 0}, Pt{0, 0}); got != 5 {
		t.Errorf("a segment of one point is the distance to that point, got %v", got)
	}
	if got := SegmentDistance(Pt{5, 5}, Pt{0, 0}, Pt{10, 0}); got != 5 {
		t.Errorf("above the middle: %v", got)
	}
	if got := SegmentDistance(Pt{-3, 4}, Pt{0, 0}, Pt{10, 0}); got != 5 {
		t.Errorf("past the end: %v", got)
	}
}

// TestPolylineAt: a point along a road clamps to its ends, skips zero-length pieces and gives the direction there.
func TestPolylineAtClampsAndSkipsEmptyPieces(t *testing.T) {
	l := Polyline{{0, 0}, {0, 0}, {10, 0}, {10, 10}}
	if l.Length() != 20 {
		t.Errorf("length %v", l.Length())
	}
	cases := []struct {
		d       float64
		at, dir Pt
	}{
		{-5, Pt{0, 0}, Pt{1, 0}},
		{0, Pt{0, 0}, Pt{1, 0}},
		{4, Pt{4, 0}, Pt{1, 0}},
		{10, Pt{10, 0}, Pt{1, 0}},
		{15, Pt{10, 5}, Pt{0, 1}},
		{20, Pt{10, 10}, Pt{0, 1}},
		{99, Pt{10, 10}, Pt{0, 1}},
	}
	for _, c := range cases {
		at, dir := l.At(c.d)
		if math.Abs(at.X-c.at.X) > 1e-9 || math.Abs(at.Y-c.at.Y) > 1e-9 || dir != c.dir {
			t.Errorf("At(%v) = %v %v, want %v %v", c.d, at, dir, c.at, c.dir)
		}
	}
	if at, dir := (Polyline{}).At(3); at != (Pt{}) || dir != (Pt{}) {
		t.Errorf("an empty line: %v %v", at, dir)
	}
	if at, dir := (Polyline{{4, 4}}).At(3); at != (Pt{}) && at != (Pt{4, 4}) || dir != (Pt{}) {
		t.Errorf("a line of one point has no direction: %v %v", at, dir)
	}
	if at, dir := (Polyline{{4, 4}, {4, 4}}).At(3); at != (Pt{4, 4}) || dir != (Pt{}) {
		t.Errorf("a line of zero length ends at its point with no direction: %v %v", at, dir)
	}
}

// TestTouchesFindsCrossingEdgesWithoutContainedPoints: two long thin rectangles crossing like a plus sign.
func TestTouchesFindsCrossingEdgesWithoutContainedPoints(t *testing.T) {
	h := Poly{{0, 4}, {10, 4}, {10, 6}, {0, 6}}
	v := Poly{{4, 0}, {6, 0}, {6, 10}, {4, 10}}
	if !h.Touches(v) || !v.Touches(h) {
		t.Error("crossing bars touch")
	}
	far := Poly{{20, 20}, {30, 20}, {30, 30}}
	if h.Touches(far) {
		t.Error("distant polygons do not touch")
	}
	inner := Poly{{4.5, 4.5}, {5.5, 4.5}, {5, 5.5}}
	if !h.Touches(inner) || !inner.Touches(h) {
		t.Error("a polygon inside another touches it")
	}
}
