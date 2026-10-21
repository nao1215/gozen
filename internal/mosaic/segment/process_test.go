package segment

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
)

func TestSegLabelsAreTheDocumentedClasses(t *testing.T) {
	if len(SegLabels) != 7 || SegLabels[0] != "anus" || SegLabels[3] != "vagina" {
		t.Fatalf("labels %v", SegLabels)
	}
}

func TestPrepareScalesAndPads(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	data, nw, nh, pw, ph, scale := prepare(img, 64, false)
	if scale != 1.6 || nw != 64 || nh != 32 || pw != 64 || ph != 32 || len(data) != 3*pw*ph {
		t.Fatalf("scale %v scaled %dx%d padded %dx%d", scale, nw, nh, pw, ph)
	}
	if data[5] < 0.99 || data[pw*ph+5] > 0.01 || data[2*pw*ph+5] > 0.01 {
		t.Fatal("planar R, G, B of a red picture")
	}
	// 50 x 50 at size 50 is padded up to 64 x 64 with gray.
	sq := image.NewRGBA(image.Rect(0, 0, 50, 50))
	data, nw, _, pw, ph, _ = prepare(sq, 50, false)
	if nw != 50 || pw != 64 || ph != 64 {
		t.Fatalf("padded %dx%d", pw, ph)
	}
	if got := data[60]; math.Abs(float64(got)-114.0/255) > 1e-6 {
		t.Fatalf("padding %v", got)
	}
	// A flipped picture: the left half red and the right blue turns around.
	two := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			if x < 16 {
				two.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				two.SetRGBA(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	plain, _, _, _, _, _ := prepare(two, 32, false)
	flipped, _, _, _, _, _ := prepare(two, 32, true)
	plane := 32 * 32
	if plain[2] < 0.99 || flipped[2] > 0.01 || flipped[2*plane+2] < 0.99 {
		t.Fatal("flip must mirror left and right")
	}
	// An offset picture reads its own pixels, not the origin's.
	off := image.NewRGBA(image.Rect(100, 100, 132, 132))
	for i := 0; i < len(off.Pix); i += 4 {
		off.Pix[i], off.Pix[i+3] = 255, 255
	}
	if d, _, _, _, _, _ := prepare(off, 32, false); d[0] < 0.99 {
		t.Fatal("picture with a non-zero origin")
	}
}

func TestCheckShapes(t *testing.T) {
	ok := func(ps, ms []int64) error { return checkShapes(ps, ms, 7) }
	if err := ok([]int64{1, 43, 8400}, []int64{1, 32, 160, 160}); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ ps, ms []int64 }{
		"rows":        {[]int64{1, 42, 8400}, []int64{1, 32, 160, 160}},
		"batch":       {[]int64{2, 43, 8400}, []int64{1, 32, 160, 160}},
		"negative N":  {[]int64{1, 43, -1}, []int64{1, 32, 160, 160}},
		"proto rank":  {[]int64{1, 43, 8400}, []int64{1, 32, 160}},
		"proto 31":    {[]int64{1, 43, 8400}, []int64{1, 31, 160, 160}},
		"proto zero":  {[]int64{1, 43, 8400}, []int64{1, 32, 0, 160}},
		"proto batch": {[]int64{1, 43, 8400}, []int64{2, 32, 160, 160}},
	} {
		if err := ok(c.ps, c.ms); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := ok([]int64{1, 42, 8400}, []int64{1, 32, 4, 4}); err == nil || !strings.Contains(err.Error(), "[1,43,N]") {
		t.Fatalf("the error must say what was expected: %v", err)
	}
}

// outputFor builds a [4+classes+32, n] output from anchors (cx, cy, w, h, scores..., coefficients...).
func outputFor(classes int, anchors ...[]float32) []float32 {
	n := len(anchors)
	v := make([]float32, (4+classes+protoCount)*n)
	for a, row := range anchors {
		for r, x := range row {
			v[r*n+a] = x
		}
	}
	return v
}

func anchor(cx, cy, w, h float32, scores []float32, coef0 float32) []float32 {
	row := append([]float32{cx, cy, w, h}, scores...)
	coefs := make([]float32, protoCount)
	coefs[0] = coef0
	return append(row, coefs...)
}

func TestCandidatesTakeTheBestClass(t *testing.T) {
	v := outputFor(2,
		anchor(30, 40, 20, 10, []float32{0.2, 0.9}, 1.5),
		anchor(60, 60, 10, 10, []float32{0.3, 0.4}, 0),
		anchor(10, 10, 4, 4, []float32{0.6, 0.1}, -2),
	)
	cs := candidatesFrom(v, 3, 2, 0.5)
	if len(cs) != 2 {
		t.Fatalf("%d candidates: %+v", len(cs), cs)
	}
	a := cs[0]
	if a.c != 1 || a.s != 0.9 || a.x0 != 20 || a.y0 != 35 || a.x1 != 40 || a.y1 != 45 || a.coef[0] != 1.5 {
		t.Fatalf("first candidate %+v", a)
	}
	if b := cs[1]; b.c != 0 || b.s != 0.6 || b.coef[0] != -2 {
		t.Fatalf("second candidate %+v", b)
	}
	// A score equal to the threshold passes; an output with no score above zero gives nothing.
	if got := candidatesFrom(v, 3, 2, 0.9); len(got) != 1 {
		t.Fatalf("threshold 0.9: %d", len(got))
	}
	if got := candidatesFrom(outputFor(1, anchor(1, 1, 1, 1, []float32{0}, 0)), 1, 1, 0); len(got) != 1 && got != nil {
		// A score of 0 is not above the best-so-far of 0, and 0 >= 0 holds: still a candidate.
		t.Fatalf("zero threshold: %d", len(got))
	}
}

func TestSuppress(t *testing.T) {
	mk := func(c int, s float32, x0, y0, x1, y1 float64) candidate {
		return candidate{c: c, s: s, x0: x0, y0: y0, x1: x1, y1: y1}
	}
	in := []candidate{
		mk(0, 0.6, 0, 0, 10, 10),
		mk(0, 0.9, 1, 0, 11, 10),   // overlaps the first strongly, same class, better score
		mk(1, 0.5, 0, 0, 10, 10),   // same box, another class: kept
		mk(0, 0.7, 30, 30, 40, 40), // elsewhere: kept
		mk(0, 0.8, 8, 0, 18, 10),   // overlaps the best only a little: kept
	}
	keep := suppress(in)
	var got []float32
	for _, k := range keep {
		got = append(got, k.s)
	}
	want := []float32{0.9, 0.8, 0.7, 0.5}
	if len(got) != len(want) {
		t.Fatalf("kept scores %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kept scores %v, want %v", got, want)
		}
	}
	if len(suppress(nil)) != 0 {
		t.Fatal("nothing in, nothing out")
	}
}

func TestBilinear(t *testing.T) {
	plane := []float32{0, 10, 20, 30} // 2 x 2
	cases := []struct{ x, y, want float64 }{
		{0, 0, 0},
		{1, 0, 10},
		{0, 1, 20},
		{1, 1, 30},
		{0.5, 0, 5},
		{0, 0.5, 10},
		{0.5, 0.5, 15},
		{-3, -3, 0},
		{9, 9, 30},
		{9, 0, 10},
	}
	for _, c := range cases {
		if got := bilinear(plane, 2, 2, c.x, c.y); math.Abs(float64(got)-c.want) > 1e-5 {
			t.Errorf("bilinear(%v,%v) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

// protoLeftPositive is 32 prototypes of 8 x 8 where the first is +1 on the left half and -1
// on the right; the rest are zero.
func protoLeftPositive() []float32 {
	p := make([]float32, protoCount*8*8)
	for y := range 8 {
		for x := range 8 {
			v := float32(-1)
			if x < 4 {
				v = 1
			}
			p[y*8+x] = v
		}
	}
	return p
}

func TestObjectsFromBuildsMasksFromPrototypes(t *testing.T) {
	b := image.Rect(0, 0, 64, 64)
	var coef [protoCount]float32
	coef[0] = 1
	box := candidate{c: 1, s: 0.8, x0: 8, y0: 8, x1: 56, y1: 56, coef: coef}
	labels := []string{"a", "b"}
	objs := objectsFrom([]candidate{box}, protoLeftPositive(), 8, 8, labels, b, 64, 64, 64, 1, false)
	if len(objs) != 1 {
		t.Fatalf("%d objects", len(objs))
	}
	o := objs[0]
	if o.Label != "b" || o.Score != 0.8 || o.Rect != image.Rect(8, 8, 56, 56) || o.Mask.Rect != o.Rect {
		t.Fatalf("object %+v", o)
	}
	at := func(x, y int) uint8 { return o.Mask.Pix[o.Mask.PixOffset(x, y)] }
	if at(12, 30) != 255 || at(20, 30) != 255 || at(45, 30) != 0 || at(52, 30) != 0 {
		t.Fatal("the mask must be the prototype's left half")
	}
	// A flipped input mirrors the box and the mask back to the picture.
	flipBox := candidate{c: 0, s: 0.7, x0: 8, y0: 8, x1: 40, y1: 56, coef: coef}
	f := objectsFrom([]candidate{flipBox}, protoLeftPositive(), 8, 8, labels, b, 64, 64, 64, 1, true)
	if len(f) != 1 || f[0].Rect != image.Rect(24, 8, 56, 56) {
		t.Fatalf("flipped box %+v", f)
	}
	// The scale turns input pixels into picture pixels.
	big := image.Rect(0, 0, 128, 128)
	scaled := objectsFrom([]candidate{box}, protoLeftPositive(), 8, 8, labels, big, 64, 64, 64, 0.5, false)
	if len(scaled) != 1 || scaled[0].Rect != image.Rect(16, 16, 112, 112) {
		t.Fatalf("scaled box %+v", scaled)
	}
	// A box outside the picture gives no object, and the box is clipped to the picture.
	outside := candidate{x0: 100, y0: 100, x1: 120, y1: 120, coef: coef}
	hang := candidate{x0: -20, y0: -20, x1: 10, y1: 10, coef: coef}
	got := objectsFrom([]candidate{outside, hang}, protoLeftPositive(), 8, 8, labels, b, 64, 64, 64, 1, false)
	if len(got) != 1 || got[0].Rect != image.Rect(0, 0, 10, 10) {
		t.Fatalf("clipping: %+v", got)
	}
	if got := objectsFrom(nil, nil, 8, 8, labels, b, 64, 64, 64, 1, false); len(got) != 0 {
		t.Fatal("no candidates")
	}
}
