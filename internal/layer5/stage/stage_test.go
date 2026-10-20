package stage

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func square() *data.Stage {
	return &data.Stage{
		Camera: 30,
		Scale:  []data.ScaleMark{{At: data.Point{50, 90}, Height: 40}, {At: data.Point{50, 10}, Height: 10}},
		Walk:   []data.Polygon{{{0, 0}, {100, 0}, {100, 100}, {0, 100}}},
		Blocks: []data.Polygon{{{40, 40}, {60, 40}, {60, 60}, {40, 60}}},
		Fronts: []data.Front{{Area: data.Polygon{{40, 0}, {60, 0}, {60, 60}, {40, 60}}, Base: 60}},
		Exits:  []data.StageExit{{To: "north", Area: data.Polygon{{0, 0}, {100, 0}, {100, 5}, {0, 5}}}},
	}
}

func TestGround(t *testing.T) {
	s := New(square())
	if h := s.Height(90); math.Abs(h-40) > 1e-9 {
		t.Errorf("height at 90: %v", h)
	}
	if h := s.Height(50); math.Abs(h-25) > 1e-9 {
		t.Errorf("height at 50: %v (between the marks)", h)
	}
	if !s.Free(geom.Pt{X: 20, Y: 20}) || s.Free(geom.Pt{X: 50, Y: 50}) || s.Free(geom.Pt{X: 150, Y: 50}) {
		t.Error("free: the open, the block, outside")
	}
	if s.ExitAt(geom.Pt{X: 50, Y: 2}) != "north" || s.ExitAt(geom.Pt{X: 50, Y: 50}) != "" {
		t.Error("exits")
	}
	// walking into the block stops at it; going a little away from the camera slides along
	// it and past its corner
	p := geom.Pt{X: 30, Y: 50}
	for range 200 {
		p = s.Move(p, 0.05, 0)
	}
	if p.X >= 40 || p.Y != 50 {
		t.Errorf("walked into the block: %v", p)
	}
	for range 400 {
		p = s.Move(p, 0.05, 0.01)
	}
	if p.X <= 60 || p.Y >= 40 {
		t.Errorf("did not slide past the block: %v", p)
	}
	// a step toward a point is at most as long as asked, and the meters left are the ground's
	dx, dy, left := s.Toward(geom.Pt{X: 20, Y: 80}, geom.Pt{X: 80, Y: 80}, 0.1)
	if math.Abs(math.Hypot(dx, dy)-0.1) > 1e-9 || dy != 0 || left <= 0.1 {
		t.Errorf("toward: %v %v %v", dx, dy, left)
	}
	if got := s.Meters(geom.Pt{X: 20, Y: 80}, geom.Pt{X: 20 + s.PPM(80), Y: 80}); math.Abs(got-1) > 1e-9 {
		t.Errorf("a meter across: %v", got)
	}
	if f := Facing("east"); f != (geom.Pt{X: 1}) || Facing("") != (geom.Pt{Y: -1}) {
		t.Errorf("facing %v", f)
	}
}

func TestShaderCompiles(t *testing.T) {
	sh, err := ebiten.NewShader(dollShader)
	if err != nil {
		t.Fatal(err)
	}
	sh.Deallocate()
}

// TestPath finds a way around the block and cuts its corners: every leg is clear, the way
// ends at the goal, and it is not much longer than the way around.
func TestPath(t *testing.T) {
	s := New(square())
	from, to := geom.Pt{X: 50, Y: 70}, geom.Pt{X: 50, Y: 30} // below and above the block
	way := s.Path(from, to)
	if len(way) < 2 || way[len(way)-1] != to {
		t.Fatalf("way %v", way)
	}
	at, length := from, 0.0
	for _, p := range way {
		if !s.Clear(at, p) {
			t.Fatalf("the leg %v -> %v is blocked", at, p)
		}
		length += geom.Dist(at, p)
		at = p
	}
	if length > 60 {
		t.Fatalf("the way is %v long (around the block is about 50)", length)
	}
	if way := s.Path(geom.Pt{X: 20, Y: 80}, geom.Pt{X: 30, Y: 80}); len(way) != 1 {
		t.Fatalf("a straight way %v", way)
	}
	// a place outside walk: the nearest place feet may stand
	if way := s.Path(from, geom.Pt{X: 150, Y: 70}); len(way) == 0 || !s.Free(way[len(way)-1]) {
		t.Fatalf("to outside: %v", way)
	}
}

// town is a 1920 x 1080 scene with 40 boxes in rows (stalls), for the way's cost.
func town() *Scene {
	st := &data.Stage{
		Camera: 30,
		Scale:  []data.ScaleMark{{At: data.Point{960, 1060}, Height: 190}, {At: data.Point{960, 300}, Height: 70}},
		Walk:   []data.Polygon{{{40, 200}, {1880, 200}, {1880, 1070}, {40, 1070}}},
	}
	for i := range 40 {
		x, y := 150+float64(i%8)*220, 280+float64(i/8)*160
		st.Blocks = append(st.Blocks, data.Polygon{{x, y}, {x + 120, y}, {x + 120, y + 60}, {x, y + 60}})
	}
	return New(st)
}

// TestPathAround: across the town every leg of the way is clear, and a click outside walk
// ends at the nearest place one may stand.
func TestPathAround(t *testing.T) {
	s := town()
	pt := func(x, y float64) geom.Pt { return geom.Pt{X: x, Y: y} }
	for _, c := range [][2]geom.Pt{{pt(60, 220), pt(1860, 1050)}, {pt(1860, 220), pt(60, 1050)}, {pt(100, 360), pt(1800, 760)}, {pt(60, 220), pt(5000, 3000)}} {
		way := s.Path(c[0], c[1])
		if len(way) == 0 {
			t.Fatalf("no way %v -> %v", c[0], c[1])
		}
		at := c[0]
		for _, p := range way {
			if !s.Clear(at, p) {
				t.Errorf("%v -> %v: the leg %v -> %v is blocked", c[0], c[1], at, p)
			}
			at = p
		}
		if !s.Free(way[len(way)-1]) {
			t.Errorf("%v -> %v ends where feet may not stand: %v", c[0], c[1], way[len(way)-1])
		}
	}
}

// BenchmarkPath is a click across the town (ADR 0022 records the time).
func BenchmarkPath(b *testing.B) {
	s := town()
	s.Path(geom.Pt{X: 60, Y: 220}, geom.Pt{X: 61, Y: 221}) // make the grid
	b.ResetTimer()
	for i := range b.N {
		if i%2 == 0 {
			s.Path(geom.Pt{X: 60, Y: 220}, geom.Pt{X: 1860, Y: 1050})
		} else {
			s.Path(geom.Pt{X: 60, Y: 220}, geom.Pt{X: 5000, Y: 3000}) // outside, far away
		}
	}
}
