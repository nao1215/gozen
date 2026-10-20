package geom

import (
	"math"
	"testing"
)

func TestInsideAndDistance(t *testing.T) {
	sq := Poly{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	for _, c := range []struct {
		p    Pt
		in   bool
		dist float64
	}{
		{Pt{5, 5}, true, 0},
		{Pt{15, 5}, false, 5},
		{Pt{13, 14}, false, 5},
		{Pt{-1, 5}, false, 1},
	} {
		if got := sq.Inside(c.p); got != c.in {
			t.Errorf("Inside(%v) = %v", c.p, got)
		}
		if got := sq.Distance(c.p); math.Abs(got-c.dist) > 1e-9 {
			t.Errorf("Distance(%v) = %v, want %v", c.p, got, c.dist)
		}
	}
	if (Poly{{0, 0}, {1, 1}}).Inside(Pt{0.5, 0.5}) {
		t.Error("a polygon of two points has no inside")
	}
}

func TestSelfCrossingAndTouches(t *testing.T) {
	bow := Poly{{0, 0}, {10, 10}, {10, 0}, {0, 10}}
	if !bow.SelfCrossing() {
		t.Error("a bow tie crosses itself")
	}
	sq := Poly{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	if sq.SelfCrossing() {
		t.Error("a square does not cross itself")
	}
	if !sq.Touches(Poly{{5, 5}, {20, 5}, {20, 6}}) || sq.Touches(Poly{{20, 20}, {30, 20}, {30, 30}}) {
		t.Error("Touches")
	}
	// a thin bar across the square: no corner inside either, but the edges cross
	if !sq.Touches(Poly{{-5, 4}, {15, 4}, {15, 6}, {-5, 6}}) {
		t.Error("crossing edges touch")
	}
}

func TestPolylineAt(t *testing.T) {
	l := Polyline{{0, 0}, {10, 0}, {10, 10}}
	if l.Length() != 20 {
		t.Fatalf("Length = %v", l.Length())
	}
	p, dir := l.At(15)
	if p != (Pt{10, 5}) || dir != (Pt{0, 1}) {
		t.Errorf("At(15) = %v %v", p, dir)
	}
	if p, _ := l.At(99); p != (Pt{10, 10}) {
		t.Errorf("At past the end = %v", p)
	}
	if p, _ := l.At(-3); p != (Pt{0, 0}) {
		t.Errorf("At before the start = %v", p)
	}
}
