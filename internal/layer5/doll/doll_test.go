package doll

import (
	"image"
	"image/color"
	"testing"
	"time"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// person is a plain standing figure: dark hair, a skin-colored face, a blue top, brown legs.
func person() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 80, 200))
	fill := func(x0, y0, x1, y1 int, c color.RGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	fill(25, 0, 55, 8, color.RGBA{40, 30, 30, 255})      // hair
	fill(25, 8, 55, 30, color.RGBA{240, 200, 170, 255})  // face
	fill(15, 30, 65, 110, color.RGBA{40, 80, 180, 255})  // top
	fill(25, 110, 55, 190, color.RGBA{110, 80, 50, 255}) // legs
	fill(25, 190, 55, 200, color.RGBA{50, 40, 30, 255})  // shoes
	return img
}

func TestPaletteFrom(t *testing.T) {
	p := PaletteFrom(person())
	if p.Top != (color.RGBA{40, 80, 180, 255}) || p.Skin != (color.RGBA{240, 200, 170, 255}) || p.Hair != (color.RGBA{40, 30, 30, 255}) {
		t.Fatalf("palette %+v", p)
	}
	if h := HairStyleFrom(person(), p); h != HairShort {
		t.Fatalf("hair %s", h)
	}
	if f := FaceFrom(person()); f.Bounds().Dx() != 128 {
		t.Fatalf("face %v", f.Bounds())
	}
}

// TestBakeFillsEveryFrame: 8 sides by 5 poses, each with the doll in it, its feet at the
// bottom; the steps of the walk differ from the standing one.
func TestBakeFillsEveryFrame(t *testing.T) {
	const fw, fh = 48, 84
	start := time.Now()
	sheet := Bake(PaletteFrom(person()), HairLong, []Part{{Kind: "apron", Color: color.RGBA{200, 40, 40, 255}}}, fw, fh, 30)
	t.Logf("baked a doll (%dx%d x 40) in %v", fw, fh, time.Since(start))
	if sheet.Bounds().Dx() != fw*Sides || sheet.Bounds().Dy() != fh*Poses {
		t.Fatalf("sheet %v", sheet.Bounds())
	}
	frame := func(side, pose int) (n, bottom int, sum uint64) {
		for y := range fh {
			for x := range fw {
				c := sheet.RGBAAt(side*fw+x, pose*fh+y)
				if c.A > 0 {
					n++
					bottom = max(bottom, y)
					sum += uint64(c.R)*uint64(x+1) + uint64(c.G)*uint64(y+1)
				}
			}
		}
		return n, bottom, sum
	}
	for side := range Sides {
		_, _, still := frame(side, 0)
		for pose := range Poses {
			n, bottom, sum := frame(side, pose)
			if n < fw*fh/12 {
				t.Errorf("side %d pose %d: %d pixels", side, pose, n)
			}
			if bottom < fh-6 {
				t.Errorf("side %d pose %d: the feet end at %d of %d", side, pose, bottom, fh)
			}
			if (pose == 1 || pose == 3) && sum == still { // 2 and 4 pass the legs by each other
				t.Errorf("side %d pose %d: the walk does not move", side, pose)
			}
		}
	}
}

func TestPartsMatchTheSchema(t *testing.T) {
	for _, k := range data.DollPartKinds {
		if err := ValidPart(k); err != nil {
			t.Error(err)
		}
		// each kind adds something to the doll
		if len(shapes(HairShort, []Part{{Kind: k}})) <= len(shapes(HairShort, nil)) {
			t.Errorf("part %s adds nothing", k)
		}
	}
	if ValidPart("wings") == nil {
		t.Error("an unknown part is valid")
	}
}

func TestSide(t *testing.T) {
	for _, c := range []struct {
		fx, fy float64
		want   int
	}{{0, 1, 0}, {0, -1, 4}, {1, 0, 6}, {-1, 0, 2}} {
		if got := Side(c.fx, c.fy); got != c.want {
			t.Errorf("Side(%v, %v) = %d, want %d", c.fx, c.fy, got, c.want)
		}
	}
	if Pose(false, 3) != 0 || Pose(true, 0) != 1 || Pose(true, 3.2) != 3 {
		t.Error("Pose")
	}
}

// TestFaceIn cuts around the box's center: a red square in a picture lands in the middle
// of the round face, a little smaller than it (the margin around the face).
func TestFaceIn(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 300))
	for y := 40; y < 80; y++ {
		for x := 120; x < 160; x++ {
			img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	face := Round(FaceIn(img, image.Rect(120, 40, 160, 80)))
	n := face.Bounds().Dx()
	if c := face.RGBAAt(n/2, n/2); c.R < 200 || c.G > 40 {
		t.Fatalf("the middle is %v, not the face", c)
	}
	red := 0
	for x := range n {
		if c := face.RGBAAt(x, n/2); c.R > 200 && c.G < 40 {
			red++
		}
	}
	if want := int(float64(n) / FaceMargin); red < want-4 || red > want+4 {
		t.Fatalf("the face is %d of %d pixels across, want about %d", red, n, want)
	}
	if face.RGBAAt(0, 0).A != 0 {
		t.Fatal("the corner is not clear (round)")
	}
}
