package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer5/doll"

	"github.com/nao1215/gozen/internal/mosaic/detect"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestFacesWithoutModel: a game with no painted scene has nothing to do; one with a stage
// and no model says how to get it, and writes nothing.
func TestFacesWithoutModel(t *testing.T) {
	out, _ := invoke(t, 0, "faces", fixture(t))
	if !strings.Contains(out, "nothing to do") {
		t.Fatalf("a game without a stage: %s", out)
	}
	game := scenesGame(t)
	_, stderr := invoke(t, 1, "faces", game, "--model", t.TempDir()+"/none.onnx")
	if stderr == "" || strings.Contains(stderr, "no file") {
		t.Fatalf("the reason: %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(game, "assets", "faces.json")); err == nil {
		t.Fatal("a failed run wrote faces.json")
	}
	invoke(t, 2, "art", "faces")
}

// scenesGame is the scenes test game (a stage with a person's picture) in a directory.
func scenesGame(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "scenes")
	fsys := testgame.ScenesFS(nil)
	delete(fsys, "assets/faces.json")
	if err := os.CopyFS(dir, fsys); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBestFace(t *testing.T) {
	b := image.Rect(0, 0, 100, 300)
	boxes := []detect.Box{
		{Rect: image.Rect(40, 200, 60, 220), Score: 0.9}, // low in the picture: a badge
		{Rect: image.Rect(40, 20, 60, 40), Score: 0.6},
	}
	if got, ok := bestFace(boxes, b); !ok || got.Rect.Min.Y != 20 {
		t.Fatalf("best %v %v", got, ok)
	}
	if _, ok := bestFace(nil, b); ok {
		t.Fatal("a face in nothing")
	}
}

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(im.Pix); i += 4 {
		im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return im
}

// The check picture is the top third of the standing picture, 480 high, with the face box
// in red and the circle the balloon face is cut from in green.
func TestFaceCheckDrawsTheBoxAndTheBalloonCircle(t *testing.T) {
	img := solid(300, 900, color.NRGBA{255, 255, 255, 255})
	out := faceCheck(img, &[4]int{100, 50, 200, 150})
	if b := out.Bounds(); b.Dx() != 480 || b.Dy() != 480 {
		t.Fatalf("a 300x300 top third scaled to 480 high is 480x480, got %v", b)
	}
	red := color.RGBA{230, 30, 30, 255}
	// The box (scale 1.6): x 160..320, y 80..240.
	for _, p := range [][2]int{{200, 80}, {200, 240}, {160, 150}, {320, 150}} {
		if got := out.At(p[0], p[1]); got != red {
			t.Errorf("(%d,%d) of the box is %v, want red", p[0], p[1], got)
		}
	}
	if got := out.At(240, 160); got == red {
		t.Error("the inside of the box is not painted")
	}
	// The circle's right-most point.
	r := 160 * doll.FaceMargin / 2
	if got := out.At(int(240+r), 160); got != (color.RGBA{30, 200, 60, 255}) {
		t.Errorf("the balloon circle at (%d,160) is %v", int(240+r), got)
	}
	// Without a face there is only the picture.
	plain := faceCheck(img, nil)
	for _, p := range [][2]int{{200, 80}, {240, 160}} {
		if got := plain.At(p[0], p[1]); got == red {
			t.Errorf("a picture without a face has no red at %v", p)
		}
	}
	// A picture whose top third is 90x30 is scaled up 16 times.
	if b := faceCheck(solid(90, 90, color.NRGBA{9, 9, 9, 255}), nil).Bounds(); b.Dy() != 480 || b.Dx() != 1440 {
		t.Errorf("bounds %v", b)
	}
}

func TestEncodePNGRoundTrips(t *testing.T) {
	b, err := encodePNG(solid(4, 3, color.NRGBA{1, 2, 3, 255}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != 4 || got.Bounds().Dy() != 3 {
		t.Errorf("bounds %v", got.Bounds())
	}
	if r, g, bl, _ := got.At(2, 1).RGBA(); r>>8 != 1 || g>>8 != 2 || bl>>8 != 3 {
		t.Errorf("pixel %v", got.At(2, 1))
	}
}

func TestFaceCheckPreviewBounds(t *testing.T) {
	if got := faceCheck(image.NewNRGBA(image.Rectangle{}), nil); !got.Bounds().Empty() {
		t.Fatalf("empty source produced preview %v", got.Bounds())
	}
	for _, size := range []image.Point{{X: 1, Y: 1}, {X: 1, Y: 2}, {X: 1, Y: 3}, {X: 600, Y: 30}, {X: 1, Y: 10000}} {
		img := solid(size.X, size.Y, color.NRGBA{30, 60, 90, 255})
		preview := faceCheck(img, nil)
		if preview.Bounds().Empty() || preview.Bounds().Dx() > 4096 || preview.Bounds().Dy() > 480 {
			t.Fatalf("size %v produced invalid preview %v", size, preview.Bounds())
		}
		if r, g, b, a := preview.At(0, 0).RGBA(); r>>8 != 30 || g>>8 != 60 || b>>8 != 90 || a != 65535 {
			t.Fatalf("size %v lost source color: %v", size, preview.At(0, 0))
		}
	}
}

func BenchmarkFaceCheck(b *testing.B) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{
		{"portrait", 300, 900}, {"wide", 600, 30},
	} {
		b.Run(tc.name, func(b *testing.B) {
			img := solid(tc.width, tc.height, color.NRGBA{30, 60, 90, 255})
			b.ReportAllocs()
			for b.Loop() {
				_ = faceCheck(img, nil)
			}
		})
	}
}
func TestFaceCheckExtremeAspectRatios(t *testing.T) {
	for _, height := range []int{1, 2, 3, 6000} {
		img := solid(2, height, color.NRGBA{255, 255, 255, 255})
		out := faceCheck(img, nil)
		if out.Bounds().Empty() || out.Bounds().Dy() != 480 {
			t.Fatalf("height %d: %v", height, out.Bounds())
		}
		if _, err := encodePNG(out); err != nil {
			t.Fatal(err)
		}
	}
}
