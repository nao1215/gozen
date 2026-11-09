package play

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

func TestEntryDotColor(t *testing.T) {
	if c, ok := parseDotColor("#7fc8a9"); !ok || c != (color.NRGBA{0x7f, 0xc8, 0xa9, 235}) {
		t.Fatal(c, ok)
	}
	for _, bad := range []string{"", "green", "#12345", "#zzzzzz"} {
		if _, ok := parseDotColor(bad); ok {
			t.Errorf("%q parsed", bad)
		}
	}
	dst := ebiten.NewImage(64, 64)
	// Pixels cannot be read before the game loop starts; drawing must not fail.
	drawEntryDot(dst, image.Rect(0, 0, 64, 64), "#ff0000")
	drawEntryDot(dst, image.Rect(0, 0, 10, 10), "not a color")
}

func TestBustLayout(t *testing.T) {
	r := image.Rect(100, 50, 400, 250)
	area, top, face := bustLayout(r, &data.EntryBust{})
	if area != image.Rect(106, 56, 220, 244) || top != .12 || face != .3 {
		t.Fatal(area, top, face)
	}
	area, top, face = bustLayout(r, &data.EntryBust{Width: .5, Top: .2, Face: .4})
	if area.Max.X != 250 || top != .2 || face != .4 {
		t.Fatal(area, top, face)
	}
}

// Faces are round badges unless face_shape is square; a rounded square keeps
// its corners inside the half size and its sides on it.
func TestFaceOutline(t *testing.T) {
	g := &Game{}
	round := g.faceOutline(10)
	if len(round) != 33 {
		t.Fatal(len(round))
	}
	g.faceSquare = true
	square := g.faceOutline(10)
	maxX := 0.0
	for _, p := range square {
		if p[0] > 10.0001 || p[1] > 10.0001 || p[0] < -10.0001 || p[1] < -10.0001 {
			t.Fatal("outside the badge", p)
		}
		maxX = max(maxX, p[0])
	}
	if maxX < 9.999 || square[0] != square[len(square)-1] {
		t.Fatal("the square does not reach its side or is not closed", maxX)
	}
}
