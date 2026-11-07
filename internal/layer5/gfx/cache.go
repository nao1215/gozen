// Package gfx draws shapes, images, and text on Ebitengine images.
package gfx

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const cacheLimit = 600

type shapeKey struct {
	w, h, r, stroke int
	clr             color.NRGBA
}

var shapeCache = map[shapeKey]*ebiten.Image{}

const shapePad = 4

func roundRectImage(w, h, r, stroke float32, clr color.Color) *ebiten.Image {
	q := func(v float32) int { return int(math.Round(float64(v) * 4)) }
	k := shapeKey{q(w), q(h), q(r), q(stroke), toNRGBA(clr)}
	if img, ok := shapeCache[k]; ok {
		return img
	}
	if len(shapeCache) > cacheLimit {
		for key, img := range shapeCache {
			img.Deallocate()
			delete(shapeCache, key)
		}
	}
	img := ebiten.NewImage(int(math.Ceil(float64(w)))+2*shapePad, int(math.Ceil(float64(h)))+2*shapePad)
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(clr)
	path := roundRectPath(shapePad, shapePad, w, h, r)
	if stroke > 0 {
		vector.StrokePath(img, path, &vector.StrokeOptions{Width: stroke}, op)
	} else {
		vector.FillPath(img, path, nil, op)
	}
	shapeCache[k] = img
	return img
}

func drawAt(dst, img *ebiten.Image, x, y float32) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x)-shapePad, float64(y)-shapePad)
	dst.DrawImage(img, op)
}

func toNRGBA(c color.Color) color.NRGBA {
	n, ok := color.NRGBAModel.Convert(c).(color.NRGBA)
	if !ok {
		return color.NRGBA{}
	}
	return n
}

type textKey struct {
	s             string
	size          float64
	fill, outline color.NRGBA
	pop           bool
}

type textImage struct {
	img    *ebiten.Image
	cx, cy float64
}

var textCache = map[textKey]textImage{}

func outlinedText(s string, size float64, fill, outline color.NRGBA) textImage {
	// ASCII already selects the pop face outside Pop; mixed text changes face.
	k := textKey{s, size, fill, outline, popOn && popSource != nil && !isASCII(s)}
	if t, ok := textCache[k]; ok {
		return t
	}
	if len(textCache) > cacheLimit {
		for key, t := range textCache {
			t.img.Deallocate()
			delete(textCache, key)
		}
	}
	// Labels can be substrings of a much larger dialogue source. Own only the
	// label before either this cache or the font shaper retains its text.
	s = strings.Clone(s)
	k.s = s
	ow := math.Max(2, size/12)
	pad := math.Ceil(ow) + 4
	w := textWidthRaw(s, size)
	_, h := textMeasure(s, size)
	img := ebiten.NewImage(int(math.Ceil(w+2*pad)), int(math.Ceil(h+2*pad)))
	cx, cy := w/2+pad, pad
	for i := range 12 {
		t := float64(i) / 12 * 2 * math.Pi
		drawTextRaw(img, s, cx+math.Cos(t)*ow, cy+math.Sin(t)*ow, size, outline)
	}
	drawTextRaw(img, s, cx, cy, size, fill)
	t := textImage{img: img, cx: cx, cy: cy}
	textCache[k] = t
	return t
}

// Keep glyphs on one fixed texture so ordinary text still batches. One-pixel
// gutters isolate linear filtering. The page and source references are bounded
// and released together on rollover or font replacement.
var (
	glyphCache               = map[*ebiten.Image]*ebiten.Image{}
	glyphPage                *ebiten.Image
	glyphX, glyphY, glyphRow int
)

func resetGlyphs() {
	if glyphPage != nil {
		glyphPage.Deallocate()
		glyphPage = nil
	}
	clear(glyphCache)
	glyphX, glyphY, glyphRow = 0, 0, 0
}

func stableGlyph(src *ebiten.Image) *ebiten.Image {
	if img := glyphCache[src]; img != nil {
		return img
	}
	w, h := src.Bounds().Dx()+2, src.Bounds().Dy()+2
	if glyphPage != nil && glyphX+w > glyphPage.Bounds().Dx() {
		glyphX, glyphY, glyphRow = 0, glyphY+glyphRow, 0
	}
	if glyphPage != nil && (len(glyphCache) >= cacheLimit || glyphY+h > glyphPage.Bounds().Dy() || w > glyphPage.Bounds().Dx()) {
		resetGlyphs()
	}
	if glyphPage == nil {
		glyphPage = ebiten.NewImageWithOptions(image.Rect(0, 0, max(1024, w), max(1024, h)), &ebiten.NewImageOptions{Unmanaged: true})
	}
	r := image.Rect(glyphX+1, glyphY+1, glyphX+w-1, glyphY+h-1)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	glyphPage.DrawImage(src, op)
	img, ok := glyphPage.SubImage(r).(*ebiten.Image)
	if !ok {
		return src
	}
	glyphCache[src] = img
	glyphX += w
	glyphRow = max(glyphRow, h)
	return img
}
