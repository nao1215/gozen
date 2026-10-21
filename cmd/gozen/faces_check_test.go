package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/nao1215/gozen/internal/layer5/doll"
)

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
