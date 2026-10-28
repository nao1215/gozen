package gfx

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var roundPath vector.Path

func roundRectPath(x, y, w, h, r float32) *vector.Path {
	p := &roundPath
	p.Reset()
	p.MoveTo(x+r, y)
	p.LineTo(x+w-r, y)
	p.ArcTo(x+w, y, x+w, y+r, r)
	p.LineTo(x+w, y+h-r)
	p.ArcTo(x+w, y+h, x+w-r, y+h, r)
	p.LineTo(x+r, y+h)
	p.ArcTo(x, y+h, x, y+h-r, r)
	p.LineTo(x, y+r)
	p.ArcTo(x, y, x+r, y, r)
	p.Close()
	return p
}

func FillRoundRect(dst *ebiten.Image, x, y, w, h, r float32, clr color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	k := float32(scale)
	drawAt(dst, roundRectImage(w*k, h*k, r*k, 0, clr), x*k, y*k)
}

func StrokeRoundRect(dst *ebiten.Image, x, y, w, h, r, width float32, clr color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	k := float32(scale)
	drawAt(dst, roundRectImage(w*k, h*k, r*k, width*k, clr), x*k, y*k)
}

func DrawImageFit(dst, img *ebiten.Image, x, y, w, h float64, alpha float32) {
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	px, py, s := fitPlacement(iw, ih, x, y, w, h)
	DrawImageScaled(dst, img, px, py, s, alpha)
}

// fitPlacement is where and how large an iw x ih image goes to fit, centered, in the box.
func fitPlacement(iw, ih, x, y, w, h float64) (px, py, s float64) {
	s = math.Min(w/iw, h/ih)
	return x + (w-iw*s)/2, y + (h-ih*s)/2, s
}

func DrawImageScaled(dst, img *ebiten.Image, x, y, s float64, alpha float32) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(s*scale, s*scale)
	op.GeoM.Translate(x*scale, y*scale)
	op.ColorScale.ScaleAlpha(alpha)
	op.Filter = ebiten.FilterLinear
	dst.DrawImage(img, op)
}

func DrawImageCover(dst, img *ebiten.Image, x, y, w, h float64, alpha float32) {
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	crop, s := coverCrop(iw, ih, w, h)
	sub, ok := img.SubImage(crop).(*ebiten.Image)
	if !ok {
		return
	}
	DrawImageScaled(dst, sub, x, y, s, alpha)
}

// coverCrop is the part of an iw x ih image that fills a w x h box, and the scale to draw it
// at, centered in the box.
func coverCrop(iw, ih, w, h float64) (image.Rectangle, float64) {
	s := math.Max(w/iw, h/ih)
	sw, sh := w/s, h/s
	sx, sy := (iw-sw)/2, (ih-sh)/2
	return image.Rect(int(sx), int(sy), int(sx+sw), int(sy+sh)), s
}

var maskCache = map[[3]int]*ebiten.Image{}

func RoundedMask(img *ebiten.Image, r float32) *ebiten.Image {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	key := [3]int{w, h, int(r)}
	mask, ok := maskCache[key]
	if !ok {
		mask = ebiten.NewImage(w, h)
		drawAt(mask, roundRectImage(float32(w), float32(h), r, 0, color.White), 0, 0)
		maskCache[key] = mask
	}
	img.DrawImage(mask, &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn})
	return img
}
