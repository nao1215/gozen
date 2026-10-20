package play

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

const (
	dreamZoom  = 0.07
	dreamTicks = 60 * 9
	dreamMaskW = 320
	dreamMaskH = 180
)

var dreamHaze = color.NRGBA{0xff, 0xc4, 0xd8, 0xff}

type dreamState struct {
	at   map[string]int
	buf  *ebiten.Image
	mask *ebiten.Image
	haze map[color.NRGBA]*ebiten.Image
}

func (g *Game) dreamShown(c data.CG) {
	if c.Look != "dream" || c.File == "" {
		return
	}
	if g.dream.at == nil {
		g.dream.at = map[string]int{}
	}
	if len(g.dream.at) > 64 {
		clear(g.dream.at)
	}
	g.dream.at[c.File] = g.tick
}

func (g *Game) dreamScale(file string) float64 {
	at, ok := g.dream.at[file]
	if !ok {
		return 1
	}
	t := math.Min(1, float64(g.tick-at)/dreamTicks)
	t = t * t * (3 - 2*t)
	return 1 + dreamZoom*t
}

func (g *Game) drawDream(dst *ebiten.Image, c data.CG, a float32) {
	sw, sh := g.screen()
	pic := c.Picture()
	k := g.dreamScale(c.File)

	w, h := sw*k*1.02, sh*k*1.02
	x, y := (sw-w)/2, (sh-h)/2

	back := pic
	back.Blur = true
	g.drawPicture(dst, back, x, y, w, h, a)

	b := dst.Bounds()
	if g.dream.buf == nil || g.dream.buf.Bounds().Size() != b.Size() {
		if g.dream.buf != nil {
			g.dream.buf.Deallocate()
		}
		g.dream.buf = ebiten.NewImage(b.Dx(), b.Dy())
	}
	buf := g.dream.buf
	buf.Clear()
	g.drawPicture(buf, pic, x, y, w, h, 1)
	px := float64(b.Dx()) / sw
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear, Blend: ebiten.BlendDestinationIn}
	op.GeoM.Scale(w*px/dreamMaskW, h*px/dreamMaskH)
	op.GeoM.Translate(x*px, y*px)
	buf.DrawImage(g.dreamMask(), op)
	op2 := &ebiten.DrawImageOptions{}
	op2.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	op2.ColorScale.ScaleAlpha(a)
	dst.DrawImage(buf, op2)

	hc := dreamHaze
	if c.Haze.Valid {
		hc = c.Haze.NRGBA
	}
	pulse := 0.88 + 0.12*math.Sin(float64(g.tick)*2*math.Pi/240)
	op3 := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op3.GeoM.Scale(float64(b.Dx())/dreamMaskW, float64(b.Dy())/dreamMaskH)
	op3.GeoM.Translate(float64(b.Min.X), float64(b.Min.Y))
	op3.ColorScale.ScaleAlpha(a * float32(pulse))
	dst.DrawImage(g.dreamHazeImage(hc), op3)
}

func dreamEdge(px, py int) float64 {
	u := (float64(px)+0.5)/dreamMaskW*2 - 1
	v := (float64(py)+0.5)/dreamMaskH*2 - 1
	return math.Sqrt(u*u*0.9 + v*v*1.15)
}

func smooth(e0, e1, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-e0)/(e1-e0)))
	return t * t * (3 - 2*t)
}

func (g *Game) dreamMask() *ebiten.Image {
	if g.dream.mask != nil {
		return g.dream.mask
	}
	m := image.NewNRGBA(image.Rect(0, 0, dreamMaskW, dreamMaskH))
	for py := range dreamMaskH {
		for px := range dreamMaskW {
			al := 1 - smooth(0.72, 1.0, dreamEdge(px, py))
			m.SetNRGBA(px, py, color.NRGBA{0xff, 0xff, 0xff, uint8(al * 255)})
		}
	}
	g.dream.mask = ebiten.NewImageFromImage(m)
	return g.dream.mask
}

func (g *Game) dreamHazeImage(c color.NRGBA) *ebiten.Image {
	if img, ok := g.dream.haze[c]; ok {
		return img
	}
	m := image.NewNRGBA(image.Rect(0, 0, dreamMaskW, dreamMaskH))
	for py := range dreamMaskH {
		for px := range dreamMaskW {
			al := 0.6 * smooth(0.75, 1.3, dreamEdge(px, py))
			m.SetNRGBA(px, py, color.NRGBA{c.R, c.G, c.B, uint8(al * 255)})
		}
	}
	img := ebiten.NewImageFromImage(m)
	if g.dream.haze == nil {
		g.dream.haze = map[color.NRGBA]*ebiten.Image{}
	}
	g.dream.haze[c] = img
	return img
}

func (g *Game) AsideBlocked() bool {
	return g.replaying || (g.cg.File != "" && !g.cg.Faces) || g.currentBattle() != nil
}
