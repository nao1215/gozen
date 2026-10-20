// Package doll draws the small box dolls that stand for people in a painted scene (ADR
// 0022): a head a little big, a body, arms and legs of boxes with their corners cut, and
// parts (an apron, a cape, a shield...). The colors are taken from the person's standing
// picture; the doll is baked once from 8 sides in 5 poses onto a sheet of pictures.
package doll

import (
	"image"
	"image/color"
	"math"
	"sort"

	xdraw "golang.org/x/image/draw"
)

// Palette is a person's colors, taken from a standing picture.
type Palette struct {
	Hair, Skin, Top, Sleeve, Bottom, Shoes color.RGBA
}

// The hair styles.
const (
	HairShort = "short"
	HairLong  = "long"
	HairBald  = "bald"
	HairBun   = "bun"
)

func lum(c color.RGBA) int { return int(c.R)*3 + int(c.G)*6 + int(c.B) }

func shade(c color.RGBA, k float64) color.RGBA {
	f := func(v uint8) uint8 { return uint8(min(255, float64(v)*k)) }
	return color.RGBA{f(c.R), f(c.G), f(c.B), 255}
}

func rgba(r, g, b uint32) color.RGBA {
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255} //nolint:gosec // 16-bit to 8-bit
}

func pickColor(img image.Image, b image.Rectangle, y0, y1, x0, x1 float64, def color.RGBA) color.RGBA {
	var cs []color.RGBA
	for y := b.Min.Y + int(y0*float64(b.Dy())); y < b.Min.Y+int(y1*float64(b.Dy())); y += 2 {
		for x := b.Min.X + int(x0*float64(b.Dx())); x < b.Min.X+int(x1*float64(b.Dx())); x += 2 {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0xc000 {
				cs = append(cs, rgba(r, g, bl))
			}
		}
	}
	if len(cs) < 8 {
		return def
	}
	sort.Slice(cs, func(i, j int) bool { return lum(cs[i]) < lum(cs[j]) })
	return cs[len(cs)/2]
}

// skinLike reports whether a color could be skin: warm (red over green over blue), not dark.
func skinLike(c color.RGBA) bool {
	r, g, b := int(c.R), int(c.G), int(c.B)
	return r > 140 && r >= g && g >= b && r-b > 30 && r-b < 140 && g > 90
}

// PaletteFrom takes a person's colors from where they usually are in a full-body picture.
func PaletteFrom(img image.Image) Palette {
	b := OpaqueBounds(img)
	var p Palette
	p.Hair = pickColor(img, b, 0.0, 0.035, 0.35, 0.65, color.RGBA{90, 60, 40, 255})
	p.Skin = pickColor(img, b, 0.075, 0.105, 0.44, 0.56, color.RGBA{250, 214, 186, 255})
	if !skinLike(p.Skin) {
		// the face's row caught hair, goggles or a hood: a usual skin instead
		p.Skin = color.RGBA{250, 214, 186, 255}
	}
	p.Top = pickColor(img, b, 0.24, 0.38, 0.38, 0.62, color.RGBA{90, 120, 180, 255})
	p.Sleeve = pickColor(img, b, 0.26, 0.40, 0.12, 0.26, p.Top)
	p.Bottom = pickColor(img, b, 0.62, 0.80, 0.38, 0.62, color.RGBA{80, 70, 60, 255})
	p.Shoes = pickColor(img, b, 0.94, 0.99, 0.30, 0.70, color.RGBA{70, 50, 40, 255})
	return p
}

func colorNear(a, b color.RGBA, d int) bool {
	ab := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return ab(a.R, b.R)+ab(a.G, b.G)+ab(a.B, b.B) < d
}

// HairStyleFrom guesses the hair: none when the top of the head has the skin's color, long
// when the hair's color comes down beside the shoulders.
func HairStyleFrom(img image.Image, p Palette) string {
	if colorNear(p.Hair, p.Skin, 60) {
		return HairBald
	}
	b := OpaqueBounds(img)
	mid, w := headSpan(img)
	n, hair := 0, 0
	for y := b.Min.Y + int(0.16*float64(b.Dy())); y < b.Min.Y+int(0.3*float64(b.Dy())); y += 3 {
		for _, fx := range []float64{mid - w*0.55, mid + w*0.55} {
			x := b.Min.X + int(fx*float64(b.Dx()))
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0xc000 {
				continue
			}
			n++
			if colorNear(rgba(r, g, bl), p.Hair, 90) {
				hair++
			}
		}
	}
	if n > 4 && hair*2 > n {
		return HairLong
	}
	return HairShort
}

// OpaqueBounds is the part of a picture that is not clear (all of it when it is all clear).
func OpaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	alpha := alphaOf(img)
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if alpha(x, y) > 0x1000 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x+1), max(y1, y+1)
			}
		}
	}
	if x0 >= x1 {
		return b
	}
	return image.Rect(x0, y0, x1, y1)
}

// alphaOf reads a picture's alpha as At(x, y).RGBA() does, straight from the pixels of the
// kinds the decoders make (a browser's build reads WebP, whose At is slow).
func alphaOf(img image.Image) func(x, y int) uint32 {
	switch m := img.(type) {
	case *image.NRGBA:
		return func(x, y int) uint32 { return uint32(m.Pix[m.PixOffset(x, y)+3]) * 0x101 }
	case *image.RGBA:
		return func(x, y int) uint32 { return uint32(m.Pix[m.PixOffset(x, y)+3]) * 0x101 }
	case *image.NYCbCrA:
		return func(x, y int) uint32 { return uint32(m.A[m.AOffset(x, y)]) * 0x101 }
	}
	return func(x, y int) uint32 {
		_, _, _, a := img.At(x, y).RGBA()
		return a
	}
}

// cut copies a rectangle of a picture (in fractions of its visible part) at w x h pixels.
func cut(img image.Image, x0, y0, x1, y1 float64, w, h int) image.Image {
	b := OpaqueBounds(img)
	r := image.Rect(b.Min.X+int(x0*float64(b.Dx())), b.Min.Y+int(y0*float64(b.Dy())),
		b.Min.X+int(x1*float64(b.Dx())), b.Min.Y+int(y1*float64(b.Dy())))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, r, xdraw.Src, nil)
	return dst
}

// headSpan finds the head's middle and width (fractions of the visible part's width) from
// the opaque span of a row a little below the top.
func headSpan(img image.Image) (mid, width float64) {
	b := OpaqueBounds(img)
	y := b.Min.Y + int(0.06*float64(b.Dy()))
	x0, x1 := -1, -1
	alpha := alphaOf(img)
	for x := b.Min.X; x < b.Max.X; x++ {
		if alpha(x, y) > 0x8000 {
			if x0 < 0 {
				x0 = x
			}
			x1 = x
		}
	}
	if x0 < 0 {
		return 0.5, 0.3
	}
	return (float64(x0+x1)/2 - float64(b.Min.X)) / float64(b.Dx()), float64(x1-x0) / float64(b.Dx())
}

// FaceFrom cuts the face (with the bangs) from a standing picture: a square under the top
// of the head, as wide as the head's row at 6% of the height.
func FaceFrom(img image.Image) image.Image {
	mid, w := headSpan(img)
	return FaceAt(img, mid, 0.018, w*0.92)
}

// FaceAt cuts a square face: its middle across and its top (fractions of the visible
// part's width and height), its width (a fraction of the width), at 128 x 128.
func FaceAt(img image.Image, mid, top, width float64) image.Image {
	b := OpaqueBounds(img)
	fy := width * float64(b.Dx()) / float64(b.Dy())
	return cut(img, mid-width/2, top, mid+width/2, top+fy, 128, 128)
}

// FaceMargin is how much larger than a detected face box the balloon's round face is cut
// (a little of the hair around it).
const FaceMargin = 1.4

// FaceIn cuts a square around a face box of a picture (its center, FaceMargin times its
// larger side), at 128 x 128. Outside the picture is clear.
func FaceIn(img image.Image, box image.Rectangle) image.Image {
	cx, cy := float64(box.Min.X+box.Max.X)/2, float64(box.Min.Y+box.Max.Y)/2
	half := float64(max(box.Dx(), box.Dy())) * FaceMargin / 2
	r := image.Rect(int(cx-half), int(cy-half), int(cx+half), int(cy+half))
	dst := image.NewRGBA(image.Rect(0, 0, 128, 128))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, r, xdraw.Src, nil)
	return dst
}

// Square is the middle square of a picture at 128 x 128 (a face drawn for the balloons).
func Square(img image.Image) image.Image {
	b := img.Bounds()
	n := min(b.Dx(), b.Dy())
	r := image.Rect(b.Min.X+(b.Dx()-n)/2, b.Min.Y+(b.Dy()-n)/2, b.Min.X+(b.Dx()+n)/2, b.Min.Y+(b.Dy()+n)/2)
	dst := image.NewRGBA(image.Rect(0, 0, 128, 128))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, r, xdraw.Src, nil)
	return dst
}

// Round masks a face to a circle (soft at the edge).
func Round(face image.Image) *image.RGBA {
	bd := face.Bounds()
	n := min(bd.Dx(), bd.Dy())
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	c := float64(n) / 2
	for y := range n {
		for x := range n {
			a := math.Max(0, math.Min(1, c-1-math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c)))
			if a == 0 {
				continue
			}
			r, g, b, fa := face.At(bd.Min.X+x, bd.Min.Y+y).RGBA()
			// a clear part of the picture (beside the hair) shows white, as the balloon
			w := 255 * (1 - float64(fa)/0xffff) * a
			out.SetRGBA(x, y, color.RGBA{uint8(float64(r>>8)*a + w), uint8(float64(g>>8)*a + w), uint8(float64(b>>8)*a + w), uint8(255 * a)})
		}
	}
	return out
}
