package gfx

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestCoverCropAndFitPlacement(t *testing.T) {
	// A wide 200x100 image covering a square 50x50 box: scale 0.5, the middle 100 columns.
	r, s := coverCrop(200, 100, 50, 50)
	if s != 0.5 || r != image.Rect(50, 0, 150, 100) {
		t.Fatalf("cover %v %v", r, s)
	}
	// A tall 100x200 image: centered vertically.
	r, s = coverCrop(100, 200, 50, 50)
	if s != 0.5 || r != image.Rect(0, 50, 100, 150) {
		t.Fatalf("cover tall %v %v", r, s)
	}
	px, py, k := fitPlacement(200, 100, 10, 20, 100, 100)
	if k != 0.5 || px != 10 || py != 45 {
		t.Fatalf("fit %v %v %v", px, py, k)
	}
	px, py, k = fitPlacement(100, 200, 0, 0, 100, 100)
	if k != 0.5 || px != 25 || py != 0 {
		t.Fatalf("fit tall %v %v %v", px, py, k)
	}
}

func TestImageDrawHelpersAcceptAnyAspect(t *testing.T) {
	t.Cleanup(Reset)
	dst, img := ebiten.NewImage(100, 100), ebiten.NewImage(40, 20)
	DrawImageFit(dst, img, 0, 0, 100, 100, 1)
	DrawImageCover(dst, img, 0, 0, 100, 100, 0.5)
	DrawImageScaled(dst, img, 5, 5, 2, 1)
	if dst.Bounds() != image.Rect(0, 0, 100, 100) {
		t.Fatal("destination bounds changed")
	}
}
