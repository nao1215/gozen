package stage

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func gradient(w, h int) *image.Gray16 {
	g := image.NewGray16(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			g.SetGray16(x, y, color.Gray16{Y: uint16(x * 65535 / (w - 1))}) //nolint:gosec // test values within range
		}
	}
	return g
}

func newRenderer(t *testing.T, bw, bh int, depth image.Image) *Renderer {
	t.Helper()
	r, err := NewRenderer(New(square()), image.NewRGBA(image.Rect(0, 0, bw, bh)), depth)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestGray16(t *testing.T) {
	size := image.Rect(0, 0, 4, 2)
	if g := Gray16(nil, size); g.Bounds() != size || g.Pix[0] != 0 || g.Gray16At(3, 1).Y != 0 {
		t.Fatal("no depth picture means everything is far")
	}
	// A 16-bit gray picture of the size is copied, wherever its own bounds start.
	src := image.NewGray16(image.Rect(10, 20, 14, 22))
	src.SetGray16(10, 20, color.Gray16{Y: 111})
	src.SetGray16(13, 21, color.Gray16{Y: 65535})
	g := Gray16(src, size)
	if g.Gray16At(0, 0).Y != 111 || g.Gray16At(3, 1).Y != 65535 || g.Gray16At(1, 0).Y != 0 {
		t.Fatalf("copy: %v", g.Pix)
	}
	// Another size is stretched by nearest samples: each pixel of a 2x1 picture covers 2x2.
	small := image.NewGray16(image.Rect(0, 0, 2, 1))
	small.SetGray16(0, 0, color.Gray16{Y: 1000})
	small.SetGray16(1, 0, color.Gray16{Y: 60000})
	g = Gray16(small, size)
	for y := range 2 {
		for x := range 4 {
			want := uint16(1000)
			if x >= 2 {
				want = 60000
			}
			if got := g.Gray16At(x, y).Y; got != want {
				t.Fatalf("stretched (%d,%d) = %d, want %d", x, y, got, want)
			}
		}
	}
	// Any other picture type contributes its red channel at 16 bits.
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 2))
	rgba.SetRGBA(2, 0, color.RGBA{R: 255, A: 255})
	if got := Gray16(rgba, size).Gray16At(2, 0).Y; got != 65535 {
		t.Fatalf("red channel %d", got)
	}
}

func TestSceneDefaultsAndHeights(t *testing.T) {
	s := New(&data.Stage{})
	if s.Camera != 30 || s.DepthBias != 0.03 {
		t.Fatalf("defaults %v %v", s.Camera, s.DepthBias)
	}
	if s.Height(0) != 100 || s.Height(999) != 100 {
		t.Fatalf("no marks gives a flat 100, got %v", s.Height(0))
	}
	if math.Abs(s.PPM(0)-100/1.7) > 1e-9 {
		t.Fatalf("PPM %v", s.PPM(0))
	}
	one := New(&data.Stage{Camera: 45, DepthBias: 0.2, Scale: []data.ScaleMark{{At: data.Point{1, 7}, Height: 60}}})
	if one.Camera != 45 || one.DepthBias != 0.2 || one.Height(0) != 60 || one.Height(500) != 60 {
		t.Fatalf("one mark gives a flat height: %v", one.Height(500))
	}
	// Marks at one height cannot define a slope, so the average is used.
	same := New(&data.Stage{Scale: []data.ScaleMark{{At: data.Point{0, 50}, Height: 40}, {At: data.Point{9, 50}, Height: 60}}})
	if same.Height(10) != 50 || same.Height(90) != 50 {
		t.Fatalf("degenerate marks: %v", same.Height(10))
	}
	// A person never shrinks below 8 pixels, whatever the line says.
	steep := New(&data.Stage{Scale: []data.ScaleMark{{At: data.Point{0, 10}, Height: 10}, {At: data.Point{0, 20}, Height: 5}}})
	if steep.Height(1000) != 8 {
		t.Fatalf("minimum height %v", steep.Height(1000))
	}
	// Three marks on a line fit it exactly.
	line := New(&data.Stage{Scale: []data.ScaleMark{{At: data.Point{0, 0}, Height: 10}, {At: data.Point{0, 10}, Height: 20}, {At: data.Point{0, 20}, Height: 30}}})
	if math.Abs(line.Height(15)-25) > 1e-9 {
		t.Fatalf("least squares %v", line.Height(15))
	}
}

func TestFacingWords(t *testing.T) {
	cases := map[string]geom.Pt{
		"north": {Y: -1}, "n": {Y: -1}, "up": {Y: -1}, "": {Y: -1},
		"east": {X: 1}, "e": {X: 1}, "right": {X: 1},
		"south": {Y: 1}, "s": {Y: 1}, "down": {Y: 1},
		"west": {X: -1}, "w": {X: -1}, "left": {X: -1},
		"sideways": {Y: -1},
	}
	for word, want := range cases {
		if got := Facing(word); got != want {
			t.Errorf("Facing(%q) = %v, want %v", word, got, want)
		}
	}
}

func TestTowardAndMoveEdges(t *testing.T) {
	s := New(square())
	p := geom.Pt{X: 20, Y: 80}
	if dx, dy, left := s.Toward(p, p, 1); dx != 0 || dy != 0 || left != 0 {
		t.Fatalf("already there: %v %v %v", dx, dy, left)
	}
	// Close enough: the step is just what is left.
	q := geom.Pt{X: p.X + s.PPM(80)*0.5, Y: p.Y}
	dx, dy, left := s.Toward(p, q, 5)
	if math.Abs(dx-0.5) > 1e-9 || dy != 0 || math.Abs(left-0.5) > 1e-9 {
		t.Fatalf("short step %v %v %v", dx, dy, left)
	}
	// Away from the camera is up the screen (y decreases) and positive dy.
	_, dy, _ = s.Toward(p, geom.Pt{X: p.X, Y: 60}, 1)
	if dy <= 0 {
		t.Fatalf("up the screen should be away from the camera, dy %v", dy)
	}
	if got := s.Move(p, 0, 0); got != p {
		t.Fatalf("no step moved to %v", got)
	}
	// A step into the outside is refused per axis.
	edge := geom.Pt{X: 99, Y: 80}
	if got := s.Move(edge, 10, 0); got != edge {
		t.Fatalf("walked out of the walk area: %v", got)
	}
}

func TestRendererGeometry(t *testing.T) {
	r := newRenderer(t, 200, 100, nil)
	if w, h := r.Size(); w != 200 || h != 100 {
		t.Fatalf("size %d x %d", w, h)
	}
	// The whole picture shows, centered: a wide picture on a square screen has bars above and below.
	k, ox, oy := r.Fit(400, 400)
	if k != 2 || ox != 0 || oy != 100 {
		t.Fatalf("fit %v %v %v", k, ox, oy)
	}
	k, ox, oy = r.Fit(100, 400)
	if k != 0.5 || ox != 0 || oy != 175 {
		t.Fatalf("fit narrow %v %v %v", k, ox, oy)
	}
	x, y, k := r.ScreenOf(geom.Pt{X: 50, Y: 10}, 400, 400)
	if x != 100 || y != 120 || k != 2 {
		t.Fatalf("screen of %v %v %v", x, y, k)
	}
	// PointOf is the inverse of ScreenOf.
	for _, p := range []geom.Pt{{X: 0, Y: 0}, {X: 199, Y: 99}, {X: 33.5, Y: 71}} {
		sx, sy, _ := r.ScreenOf(p, 640, 360)
		if back := r.PointOf(sx, sy, 640, 360); math.Abs(back.X-p.X) > 1e-9 || math.Abs(back.Y-p.Y) > 1e-9 {
			t.Fatalf("round trip %v -> %v", p, back)
		}
	}
	// The head is above the feet by about the doll's height, and taller nearer the camera.
	fx, fy, _ := r.ScreenOf(geom.Pt{X: 100, Y: 90}, 400, 400)
	hx, hy := r.HeadOf(geom.Pt{X: 100, Y: 90}, 400, 400)
	if hx != fx || hy >= fy {
		t.Fatalf("head %v,%v over feet %v,%v", hx, hy, fx, fy)
	}
	nearH := fy - hy
	fx2, fy2, _ := r.ScreenOf(geom.Pt{X: 100, Y: 20}, 400, 400)
	_, hy2 := r.HeadOf(geom.Pt{X: 100, Y: 20}, 400, 400)
	_ = fx2
	if farH := fy2 - hy2; farH >= nearH {
		t.Fatalf("a far doll (%v) is not smaller than a near one (%v)", farH, nearH)
	}
}

func TestRendererDepthAt(t *testing.T) {
	flat := newRenderer(t, 50, 40, nil)
	if flat.DepthAt(geom.Pt{X: 10, Y: 10}) != 0 {
		t.Fatal("without a depth picture everything is far")
	}
	r := newRenderer(t, 101, 10, gradient(101, 10))
	for x, want := range map[float64]float64{0: 0, 50: 0.5, 100: 1, -30: 0, 400: 1} {
		if got := r.DepthAt(geom.Pt{X: x, Y: 3}); math.Abs(got-want) > 1e-3 {
			t.Errorf("depth at x=%v is %v, want %v", x, got, want)
		}
	}
	if got := r.DepthAt(geom.Pt{X: 50, Y: -9}); math.Abs(got-0.5) > 1e-3 {
		t.Errorf("depth above the picture: %v", got)
	}
}

func TestBehindFront(t *testing.T) {
	r := newRenderer(t, 100, 100, nil)
	// square() has a front over x 40..60, y 0..60 standing at y = 60.
	if !r.behindFront(geom.Pt{X: 50, Y: 30}) {
		t.Fatal("feet behind the front's base are hidden")
	}
	if r.behindFront(geom.Pt{X: 50, Y: 70}) {
		t.Fatal("feet before the base are not")
	}
	if r.behindFront(geom.Pt{X: 10, Y: 30}) {
		t.Fatal("feet beside the front are not")
	}
}

func TestDrawSortsFiguresFarFirstAndSetsUniforms(t *testing.T) {
	r := newRenderer(t, 100, 100, gradient(100, 100))
	sheet := ebiten.NewImage(SheetW*8, SheetH*5)
	dst := ebiten.NewImage(200, 200)
	figs := []Figure{
		{At: geom.Pt{X: 70, Y: 90}, Sheet: sheet, Facing: geom.Pt{Y: 1}, Moving: true, Phase: 1},
		{At: geom.Pt{X: 20, Y: 20}, Sheet: sheet, Facing: geom.Pt{X: 1}, Fade: 0.25},
		{At: geom.Pt{X: 45, Y: 50}},
		{At: geom.Pt{X: 50, Y: 55}, Sheet: sheet},
	}
	r.Draw(dst, figs)
	for i := 1; i < len(figs); i++ {
		if figs[i-1].At.Y > figs[i].At.Y {
			t.Fatalf("figures not sorted far first: %v", figs)
		}
	}
	// The last figure drawn is the nearest with a sheet.
	u := r.sop.Uniforms
	if u["FootY"] != float32(90) || u["Alpha"] != float32(1) {
		t.Fatalf("uniforms %v %v", u["FootY"], u["Alpha"])
	}
	if r.sop.Images[0] != sheet || r.sop.Images[1] != r.depth || r.sop.Images[2] != r.fronts {
		t.Fatal("shader images")
	}
	// The depth under the feet at (70,90) of the gradient picture.
	if fd, ok := u["FootDepth"].(float32); !ok || math.Abs(float64(fd)-0.7) > 0.02 {
		t.Fatalf("foot depth %v", u["FootDepth"])
	}
	// The sheet picture is chosen by facing and pose: facing the camera (side 0), walking.
	if fr, ok := u["Frame"].([]float32); !ok || fr[2] != SheetW || fr[3] != SheetH || fr[1] == 0 {
		t.Fatalf("frame %v", u["Frame"])
	}
	// Fading out is reflected in alpha.
	r.Draw(dst, []Figure{{At: geom.Pt{X: 20, Y: 20}, Sheet: sheet, Fade: 0.25}})
	if u["Alpha"] != float32(0.75) {
		t.Fatalf("alpha %v", u["Alpha"])
	}
	r.Look.ScaleWithColor(color.RGBA{R: 200, G: 200, B: 200, A: 255})
	r.Draw(dst, nil)
}
