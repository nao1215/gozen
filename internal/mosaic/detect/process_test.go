package detect

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
)

// halves is a w x h picture whose left half is red and right half blue.
func halves(w, h int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if x < w/2 {
				m.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				m.SetRGBA(x, y, color.RGBA{0, 0, 255, 255})
			}
		}
	}
	return m
}

func TestPrepareLetterboxesToMultiplesOf32(t *testing.T) {
	img := halves(20, 10)
	data, nw, nh, pw, ph, scale := prepare(img, img.Bounds(), 64, false)
	if scale != 3.2 || nw != 64 || nh != 32 || pw != 64 || ph != 32 {
		t.Fatalf("scale %v, scaled %dx%d, padded %dx%d", scale, nw, nh, pw, ph)
	}
	if len(data) != 3*pw*ph {
		t.Fatalf("data %d", len(data))
	}
	plane := pw * ph
	at := func(c, x, y int) float32 { return data[c*plane+y*pw+x] }
	if at(0, 2, 2) < 0.99 || at(1, 2, 2) > 0.01 || at(2, 2, 2) > 0.01 {
		t.Fatal("left half must be red (planar R, G, B)")
	}
	if at(2, 60, 2) < 0.99 || at(0, 60, 2) > 0.01 {
		t.Fatal("right half must be blue")
	}
	// A tall picture is padded on the right and below with the YOLO gray.
	tall := halves(10, 20)
	data, nw, nh, pw, ph, _ = prepare(tall, tall.Bounds(), 64, false)
	if nw != 32 || nh != 64 || pw != 32 || ph != 64 {
		t.Fatalf("tall: scaled %dx%d padded %dx%d", nw, nh, pw, ph)
	}
	odd := halves(30, 30)
	data, nw, nh, pw, ph, _ = prepare(odd, odd.Bounds(), 40, false)
	if nw != 40 || nh != 40 || pw != 64 || ph != 64 {
		t.Fatalf("padding to a multiple of 32: scaled %dx%d padded %dx%d", nw, nh, pw, ph)
	}
	plane = pw * ph
	for c := range 3 {
		if got := data[c*plane+50*pw+50]; math.Abs(float64(got)-114.0/255) > 1e-6 {
			t.Fatalf("channel %d padding %v", c, got)
		}
		if got := data[c*plane+0*pw+60]; math.Abs(float64(got)-114.0/255) > 1e-6 {
			t.Fatalf("channel %d right padding %v", c, got)
		}
	}
}

func TestPrepareFlipRegionAndNativeSize(t *testing.T) {
	img := halves(20, 10)
	data, _, _, pw, ph, _ := prepare(img, img.Bounds(), 64, true)
	plane := pw * ph
	if data[2*plane+2*pw+2] < 0.99 || data[2] > 0.01 {
		t.Fatal("flipping puts the blue half on the left")
	}
	// size 0 keeps the region's own size; only the region is read.
	region := image.Rect(10, 0, 20, 10) // the blue half
	data, nw, nh, pw, ph, scale := prepare(img, region, 0, false)
	if scale != 1 || nw != 10 || nh != 10 || pw != 32 || ph != 32 {
		t.Fatalf("native: %v %dx%d %dx%d", scale, nw, nh, pw, ph)
	}
	plane = pw * ph
	if data[2*plane+3*pw+3] < 0.99 || data[3*pw+3] > 0.01 {
		t.Fatal("the region is not the blue half")
	}
	// A region too small to matter still gives one pixel.
	if _, nw, nh, _, _, _ = prepare(img, image.Rect(0, 0, 1, 1), 0, false); nw != 1 || nh != 1 {
		t.Fatalf("1x1 region scaled to %dx%d", nw, nh)
	}
}

func TestCheckShape(t *testing.T) {
	labels := []string{"a", "b"}
	if err := checkShape([]int64{1, 6, 100}, labels); err != nil {
		t.Fatal(err)
	}
	if err := checkShape([]int64{1, 6, 0}, labels); err != nil {
		t.Fatalf("no candidates is a valid output: %v", err)
	}
	for _, shape := range [][]int64{{1, 5, 10}, {1, 7, 10}, {2, 6, 10}, {1, 6}, {1, 6, 10, 1}, {1, 6, -1}, nil} {
		if err := checkShape(shape, labels); err == nil {
			t.Errorf("%v accepted", shape)
		}
	}
	err := checkShape([]int64{1, 9, 10}, labels)
	if err == nil || !strings.Contains(err.Error(), "[1,6,N]") || !strings.Contains(err.Error(), "a,b") {
		t.Fatalf("error does not say what was expected: %v", err)
	}
}

// output builds a [4+classes, n] output from anchors given as cx, cy, w, h, scores...
func output(labels int, anchors ...[]float32) []float32 {
	n := len(anchors)
	v := make([]float32, (4+labels)*n)
	for a, row := range anchors {
		for r, x := range row {
			v[r*n+a] = x
		}
	}
	return v
}

func TestBoxesFrom(t *testing.T) {
	labels := []string{"a", "b"}
	region := image.Rect(100, 200, 300, 400)
	v := output(2,
		[]float32{40, 30, 20, 10, 0.9, 0.6},
		[]float32{80, 80, 10, 10, 0.2, 0.5},
	)
	got := boxesFrom(v, 2, labels, region, 100, 2, false, 0.5)
	// Anchor 0 is (30..50, 25..35) in the input, so (115..125, 212..217.5) in the picture; the far
	// edge is rounded up. Both classes of anchor 0 pass, and anchor 1 only its second class
	// (a score equal to the threshold counts).
	want := []Box{
		{Rect: image.Rect(115, 212, 125, 218), Label: "a", Score: 0.9},
		{Rect: image.Rect(115, 212, 125, 218), Label: "b", Score: 0.6},
		{Rect: image.Rect(137, 237, 143, 243), Label: "b", Score: 0.5},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d boxes: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("box %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := boxesFrom(v, 2, labels, region, 100, 2, false, 0.7); len(got) != 1 || got[0].Label != "a" {
		t.Fatalf("threshold 0.7: %+v", got)
	}
	if got := boxesFrom(v, 2, labels, region, 100, 2, false, 0.95); len(got) != 0 {
		t.Fatalf("threshold 0.95: %+v", got)
	}
	// Flipped input: x is mirrored inside the scaled width (100): center 40 becomes 60.
	flipped := boxesFrom(v, 2, labels, region, 100, 2, true, 0.8)
	if len(flipped) != 1 || flipped[0].Rect != image.Rect(125, 212, 135, 218) {
		t.Fatalf("flipped: %+v", flipped)
	}
}

func TestBoxesFromClipsToTheRegion(t *testing.T) {
	region := image.Rect(10, 10, 60, 60)
	// A box hanging out of the region is cut; one wholly outside is dropped.
	v := output(1,
		[]float32{0, 0, 40, 40, 0.9},
		[]float32{500, 500, 10, 10, 0.9},
		[]float32{30, 30, 0, 0, 0.9},
	)
	got := boxesFrom(v, 3, []string{"x"}, region, 50, 1, false, 0.5)
	if len(got) != 1 || got[0].Rect != image.Rect(10, 10, 30, 30) {
		t.Fatalf("clipping: %+v", got)
	}
	if len(boxesFrom(nil, 0, []string{"x"}, region, 50, 1, false, 0.5)) != 0 {
		t.Fatal("no anchors, no boxes")
	}
}
