package gfx

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const BaseHeight = 720.0

var scale = 1.0

func SetScale(k float64) {
	if k <= 0 {
		k = 1
	}
	scale = k
}

func Scale() float64 { return scale }

func DrawImage(dst, img *ebiten.Image, op *ebiten.DrawImageOptions) {
	if op == nil {
		op = &ebiten.DrawImageOptions{}
	}
	if scale != 1 {
		copyOp := *op
		op = &copyOp
		op.GeoM.Scale(scale, scale)
	}
	dst.DrawImage(img, op)
}

func ScaleVertices(vs []ebiten.Vertex) {
	if scale == 1 {
		return
	}
	k := float32(scale)
	for i := range vs {
		vs[i].DstX *= k
		vs[i].DstY *= k
	}
}

func FillRect(dst *ebiten.Image, x, y, w, h float32, clr color.Color, aa bool) {
	k := float32(scale)
	vector.FillRect(dst, x*k, y*k, w*k, h*k, clr, aa)
}

func StrokeLine(dst *ebiten.Image, x0, y0, x1, y1, width float32, clr color.Color, aa bool) {
	k := float32(scale)
	vector.StrokeLine(dst, x0*k, y0*k, x1*k, y1*k, width*k, clr, aa)
}

func FillCircle(dst *ebiten.Image, cx, cy, r float32, clr color.Color, aa bool) {
	k := float32(scale)
	vector.FillCircle(dst, cx*k, cy*k, r*k, clr, aa)
}

func StrokeCircle(dst *ebiten.Image, cx, cy, r, width float32, clr color.Color, aa bool) {
	k := float32(scale)
	vector.StrokeCircle(dst, cx*k, cy*k, r*k, width*k, clr, aa)
}
