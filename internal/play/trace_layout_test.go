package play

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestContrastRatio(t *testing.T) {
	t.Parallel()
	black, white := color.NRGBA{A: 255}, color.NRGBA{255, 255, 255, 255}
	if r := contrastRatio(white, black); math.Abs(r-21) > 0.01 {
		t.Fatalf("white on black %g, want 21", r)
	}
	if r := contrastRatio(black, white); math.Abs(r-21) > 0.01 {
		t.Fatalf("the order of the colors changed the ratio: %g", r)
	}
	// #767676 is the lightest gray that keeps 4.5:1 on white.
	if r := contrastRatio(color.NRGBA{0x76, 0x76, 0x76, 255}, white); r < 4.5 || r > 4.6 {
		t.Fatalf("#767676 on white %g, want just over 4.5", r)
	}
	// A half transparent white text on black shows as a middle gray.
	if got := blend(color.NRGBA{255, 255, 255, 128}, black); got.R < 127 || got.R > 129 {
		t.Fatalf("blend %v, want a middle gray", got)
	}
}

// pixels makes a w×h screen of back with the text's color on the left fifth of it.
func pixels(w, h int, back, text color.NRGBA, textCols int) []byte {
	pix := make([]byte, 4*w*h)
	for y := range h {
		for x := range w {
			c := back
			if x < textCols {
				c = text
			}
			i := 4 * (y*w + x)
			pix[i], pix[i+1], pix[i+2], pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return pix
}

func TestBackColor(t *testing.T) {
	t.Parallel()
	b := image.Rect(0, 0, 50, 10)
	gray, white := color.NRGBA{90, 90, 90, 255}, color.NRGBA{250, 250, 250, 255}
	back, ok := backColor(pixels(50, 10, gray, white, 10), b, b, white)
	if !ok || back != gray {
		t.Fatalf("back %v %v, want the gray behind the glyphs", back, ok)
	}
	// Nothing of the text's color shows: something drawn later covers it.
	if _, ok := backColor(pixels(50, 10, gray, white, 0), b, b, white); ok {
		t.Fatal("a covered text was measured")
	}
	// Text that is nearly the color behind it: everything is near it, and the median
	// of all the pixels stands for the back, so the ratio comes out low, not missing.
	near := color.NRGBA{100, 100, 100, 255}
	back, ok = backColor(pixels(50, 10, gray, near, 10), b, b, near)
	if !ok || contrastRatio(near, back) > 1.3 {
		t.Fatalf("back %v %v of a text close to it, want a ratio near 1", back, ok)
	}
	// A white text at alpha 200 on a dark back shows gray, farther than the threshold
	// from its own color: it is still found and measured, not taken for covered.
	dark, half := color.NRGBA{20, 20, 20, 255}, color.NRGBA{255, 255, 255, 200}
	back, ok = backColor(pixels(50, 10, dark, blend(half, dark), 10), b, b, half)
	if !ok || back != dark {
		t.Fatalf("back %v %v of a translucent text, want the dark back", back, ok)
	}
}
