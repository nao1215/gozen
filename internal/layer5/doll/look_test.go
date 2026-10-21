package doll

import (
	"image"
	"image/color"
	"testing"
)

func TestOpaqueBounds(t *testing.T) {
	r := image.NewRGBA(image.Rect(0, 0, 40, 30))
	r.SetRGBA(10, 5, color.RGBA{255, 0, 0, 255})
	r.SetRGBA(20, 12, color.RGBA{0, 255, 0, 255})
	r.SetRGBA(30, 28, color.RGBA{0, 0, 0, 3}) // nearly clear: not visible
	if got := OpaqueBounds(r); got != image.Rect(10, 5, 21, 13) {
		t.Fatalf("RGBA bounds %v", got)
	}
	// A picture that is clear everywhere is all of its area.
	if got := OpaqueBounds(image.NewRGBA(image.Rect(2, 3, 12, 9))); got != image.Rect(2, 3, 12, 9) {
		t.Fatalf("clear picture bounds %v", got)
	}
	n := image.NewNRGBA(image.Rect(5, 5, 25, 25))
	n.SetNRGBA(8, 9, color.NRGBA{1, 2, 3, 255})
	if got := OpaqueBounds(n); got != image.Rect(8, 9, 9, 10) {
		t.Fatalf("NRGBA bounds %v", got)
	}
	// Any other kind of picture goes through At.
	p := image.NewPaletted(image.Rect(0, 0, 10, 10), color.Palette{color.Transparent, color.Black})
	p.SetColorIndex(4, 6, 1)
	if got := OpaqueBounds(p); got != image.Rect(4, 6, 5, 7) {
		t.Fatalf("paletted bounds %v", got)
	}
	ycc := image.NewNYCbCrA(image.Rect(0, 0, 8, 8), image.YCbCrSubsampleRatio444)
	ycc.A[ycc.AOffset(3, 2)] = 255
	if got := OpaqueBounds(ycc); got != image.Rect(3, 2, 4, 3) {
		t.Fatalf("NYCbCrA bounds %v", got)
	}
}

func TestAlphaOfAgreesWithAt(t *testing.T) {
	r := image.NewRGBA(image.Rect(0, 0, 4, 4))
	r.SetRGBA(1, 1, color.RGBA{10, 10, 10, 128})
	n := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	n.SetNRGBA(2, 2, color.NRGBA{10, 10, 10, 77})
	y := image.NewNYCbCrA(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420)
	y.A[y.AOffset(3, 3)] = 200
	g := image.NewGray(image.Rect(0, 0, 4, 4))
	for _, c := range []struct {
		img  image.Image
		x, y int
	}{{r, 1, 1}, {r, 0, 0}, {n, 2, 2}, {n, 1, 1}, {y, 3, 3}, {y, 0, 0}, {g, 2, 2}} {
		_, _, _, want := c.img.At(c.x, c.y).RGBA()
		if got := alphaOf(c.img)(c.x, c.y); got != want {
			t.Errorf("%T at (%d,%d): alpha %d, At says %d", c.img, c.x, c.y, got, want)
		}
	}
}

func TestSkinLike(t *testing.T) {
	cases := []struct {
		c    color.RGBA
		want bool
	}{
		{color.RGBA{240, 200, 170, 255}, true},
		{color.RGBA{140, 120, 100, 255}, false}, // too dark: r must exceed 140
		{color.RGBA{200, 220, 180, 255}, false}, // green over red
		{color.RGBA{220, 200, 195, 255}, false}, // too gray: r-b below 30
		{color.RGBA{250, 100, 20, 255}, false},  // too saturated: r-b above 140
		{color.RGBA{200, 80, 60, 255}, false},   // green below 90
		{color.RGBA{255, 255, 200, 255}, true},  // r == g is still warm
		{color.RGBA{180, 150, 120, 255}, true},
	}
	for _, c := range cases {
		if got := skinLike(c.c); got != c.want {
			t.Errorf("skinLike(%v) = %v, want %v", c.c, got, c.want)
		}
	}
}

func TestPaletteFallsBackWhenThePictureGivesNothing(t *testing.T) {
	p := PaletteFrom(image.NewRGBA(image.Rect(0, 0, 40, 100)))
	if p.Skin != (color.RGBA{250, 214, 186, 255}) || p.Hair != (color.RGBA{90, 60, 40, 255}) || p.Top != (color.RGBA{90, 120, 180, 255}) {
		t.Fatalf("defaults %+v", p)
	}
	if p.Sleeve != p.Top {
		t.Fatal("without sleeves the top's color is used")
	}
	// A face row that caught something not skin-like is replaced by the usual skin.
	img := person()
	for y := 8; y < 30; y++ {
		for x := 25; x < 55; x++ {
			img.SetRGBA(x, y, color.RGBA{20, 20, 200, 255})
		}
	}
	if got := PaletteFrom(img).Skin; got != (color.RGBA{250, 214, 186, 255}) {
		t.Fatalf("blue face became %v", got)
	}
}

func TestHairStyleFrom(t *testing.T) {
	p := PaletteFrom(person())
	if got := HairStyleFrom(person(), p); got != HairShort {
		t.Fatalf("short hair guessed %q", got)
	}
	bald := p
	bald.Hair = bald.Skin
	if got := HairStyleFrom(person(), bald); got != HairBald {
		t.Fatalf("hair the color of skin guessed %q", got)
	}
	// Hair coming down beside the head to the shoulders is long.
	long := person()
	for y := 20; y < 60; y++ { // below the row the head's width is read from
		for _, x0 := range []int{15, 55} {
			for x := x0; x < x0+10; x++ {
				long.SetRGBA(x, y, color.RGBA{40, 30, 30, 255})
			}
		}
	}
	if got := HairStyleFrom(long, p); got != HairLong {
		t.Fatalf("long hair guessed %q", got)
	}
}

func TestHeadSpan(t *testing.T) {
	mid, w := headSpan(person())
	if mid < 0.45 || mid > 0.55 || w < 0.5 || w > 0.65 {
		t.Fatalf("head at %v, %v wide", mid, w)
	}
	if mid, w := headSpan(image.NewRGBA(image.Rect(0, 0, 10, 10))); mid != 0.5 || w != 0.3 {
		t.Fatalf("clear picture: %v %v", mid, w)
	}
}

func TestFaceCutsAre128Squares(t *testing.T) {
	for name, f := range map[string]image.Image{
		"FaceFrom": FaceFrom(person()),
		"FaceAt":   FaceAt(person(), 0.5, 0.02, 0.4),
		"Square":   Square(person()),
		"FaceIn":   FaceIn(person(), image.Rect(20, 0, 60, 30)),
	} {
		if f.Bounds() != image.Rect(0, 0, 128, 128) {
			t.Errorf("%s is %v", name, f.Bounds())
		}
	}
	// Square takes the middle of a tall picture: a red band at its middle shows.
	tall := image.NewRGBA(image.Rect(10, 20, 50, 140)) // 40 x 120
	for y := 70; y < 90; y++ {
		for x := 10; x < 50; x++ {
			tall.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	sq := asRGBA(t, Square(tall))
	if c := sq.RGBAAt(64, 64); c.R < 200 || c.G > 50 {
		t.Fatalf("the middle of the square is %v", c)
	}
	if c := sq.RGBAAt(64, 2); c.R > 50 && c.A > 200 {
		t.Fatalf("the top of the square is the band's %v", c)
	}
	// A wide picture is cut at its middle too.
	wide := image.NewRGBA(image.Rect(0, 0, 120, 40))
	for y := range 40 {
		for x := 50; x < 70; x++ {
			wide.SetRGBA(x, y, color.RGBA{0, 255, 0, 255})
		}
	}
	if c := asRGBA(t, Square(wide)).RGBAAt(64, 64); c.G < 200 || c.R > 50 {
		t.Fatalf("wide middle %v", c)
	}
}

func TestRoundMasksToACircleAndWhitensClearParts(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := range 40 {
		for x := range 20 {
			src.SetRGBA(x, y, color.RGBA{200, 0, 0, 255}) // left half red, right half clear
		}
	}
	out := Round(src)
	if out.Bounds() != image.Rect(0, 0, 40, 40) {
		t.Fatalf("bounds %v", out.Bounds())
	}
	if out.RGBAAt(0, 0).A != 0 || out.RGBAAt(39, 39).A != 0 {
		t.Fatal("corners must be clear")
	}
	if c := out.RGBAAt(10, 20); c.R < 190 || c.G > 10 || c.A != 255 {
		t.Fatalf("red side %v", c)
	}
	if c := out.RGBAAt(30, 20); c.R != 255 || c.G != 255 || c.B != 255 || c.A != 255 {
		t.Fatalf("a clear part of the picture shows white, got %v", c)
	}
	// A non-square face uses the smaller side.
	if got := Round(image.NewRGBA(image.Rect(0, 0, 30, 50))).Bounds(); got != image.Rect(0, 0, 30, 30) {
		t.Fatalf("rectangular face gives %v", got)
	}
}

func TestPickColorUsesTheMedianOfTheOpaquePixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := range 40 {
		for x := range 40 {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 6), 0, 0, 255})
		}
	}
	def := color.RGBA{1, 2, 3, 255}
	got := pickColor(img, img.Bounds(), 0, 1, 0, 1, def)
	if got.R < 100 || got.R > 140 || got.G != 0 {
		t.Fatalf("median %v", got)
	}
	// Too few opaque pixels: the default.
	if got := pickColor(image.NewRGBA(image.Rect(0, 0, 40, 40)), image.Rect(0, 0, 40, 40), 0, 1, 0, 1, def); got != def {
		t.Fatalf("clear picture gave %v", got)
	}
}

func TestBakerStepsOnePictureAtATime(t *testing.T) {
	const fw, fh = 24, 42
	b := NewBaker(PaletteFrom(person()), HairShort, nil, fw, fh, 30)
	count := func(side, pose int) int {
		n := 0
		for y := range fh {
			for x := range fw {
				if b.Sheet().RGBAAt(side*fw+x, pose*fh+y).A > 0 {
					n++
				}
			}
		}
		return n
	}
	if count(0, 0) != 0 {
		t.Fatal("the sheet starts empty")
	}
	if b.Step() {
		t.Fatal("one picture cannot finish the sheet")
	}
	// Pictures are made side by side: side 0 pose 0, then side 0 pose 1, and so on.
	if count(0, 0) == 0 || count(0, 1) != 0 || count(1, 0) != 0 {
		t.Fatalf("after one step: %d %d %d", count(0, 0), count(0, 1), count(1, 0))
	}
	b.Step()
	if count(0, 1) == 0 {
		t.Fatal("the second step draws the second pose")
	}
	steps := 2
	for !b.Step() {
		steps++
	}
	steps++
	if steps != Sides*Poses {
		t.Fatalf("done after %d steps, want %d", steps, Sides*Poses)
	}
	if !b.Step() {
		t.Fatal("a finished baker stays finished")
	}
	if count(Sides-1, Poses-1) == 0 {
		t.Fatal("the last picture is empty")
	}
}

func TestBakeBaldHasNoHair(t *testing.T) {
	pal := PaletteFrom(person())
	b := NewBaker(pal, HairBald, nil, 24, 42, 30)
	if b.colors[cSkin] != pal.Hair {
		t.Fatalf("a bald head's top takes the hair color as its skin: %v", b.colors[cSkin])
	}
	if got, want := len(shapes(HairBald, nil)), len(shapes(HairShort, nil)); got >= want {
		t.Fatalf("bald has %d solids, short %d", got, want)
	}
	if len(shapes(HairLong, nil)) <= len(shapes(HairShort, nil)) || len(shapes(HairBun, nil)) <= len(shapes(HairShort, nil)) {
		t.Fatal("long and bun hair add shapes")
	}
}

func TestPoseCycle(t *testing.T) {
	if Pose(false, 99) != 0 {
		t.Fatal("standing is row 0 whatever the phase")
	}
	seen := map[int]bool{}
	for phase := 0.0; phase < 4*3.2; phase += 0.1 {
		p := Pose(true, phase)
		if p < 1 || p > 4 {
			t.Fatalf("walk pose %d out of 1..4 at phase %v", p, phase)
		}
		seen[p] = true
	}
	if len(seen) != 4 {
		t.Fatalf("walk uses rows %v", seen)
	}
}

func asRGBA(t *testing.T, img image.Image) *image.RGBA {
	t.Helper()
	r, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("%T is not *image.RGBA", img)
	}
	return r
}
