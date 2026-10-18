package play

import (
	"image/color"
	"math"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

const bubbleMax = 24

const bubbleDotR = 16

var (
	bubbleDot *ebiten.Image
	bubbleOp  ebiten.DrawImageOptions
)

func bubbleImage() *ebiten.Image {
	if bubbleDot != nil {
		return bubbleDot
	}
	n := bubbleDotR * 2
	pix := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)+0.5-bubbleDotR, float64(y)+0.5-bubbleDotR
			d := math.Sqrt(dx*dx+dy*dy) / bubbleDotR
			a := 0.0
			switch {
			case d < 0.35:
				a = 1
			case d < 1:
				k := (1 - d) / 0.65
				a = k * k * 0.85
			}
			v := byte(a * 255)
			i := (y*n + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = v, v, v, v
		}
	}
	bubbleDot = ebiten.NewImage(n, n)
	bubbleDot.WritePixels(pix)
	return bubbleDot
}

func bubbleHash(a, b, c int) float64 {
	//nolint:gosec // Visual hash intentionally wraps modulo 2^32.
	h := uint32(a)*0x9e3779b1 ^ uint32(b)*0x85ebca77 ^ uint32(c)*0xc2b2ae3d
	h ^= h >> 15
	h *= 0x2c1b3c6d
	h ^= h >> 12
	h *= 0x297a2d39
	h ^= h >> 15
	return float64(h&0xffffff) / float64(0x1000000)
}

func (g *Game) drawCardBubbles(dst *ebiten.Image, actor string, states map[string]int, slot int, x, y, w, h float64) {
	rules := g.db.Game.CardBubbles
	if len(rules) == 0 {
		return
	}
	for ri := range rules {
		r := &rules[ri]
		if r.Actor != "" && r.Actor != actor {
			continue
		}
		if r.State != "" && states[r.State] <= 0 {
			continue
		}
		if r.When.Set() {
			if ok, err := r.When.Bool(g.st.Env()); err != nil || !ok {
				continue
			}
		}
		g.drawBubbleRule(dst, ri, slot, x, y, w)
	}
}

func (g *Game) drawBubbleRule(dst *ebiten.Image, ri, slot int, x, y, w float64) {
	r := &g.db.Game.CardBubbles[ri]
	n := r.Count
	if n == 0 && g.db.Game.SchemaVersion == data.LegacySchemaVersion {
		n = 4
	}
	n = min(n, bubbleMax)
	size, speed, rise := r.Size, r.Speed, r.Rise
	if size == 0 {
		size = 3
	}
	if speed == 0 {
		speed = 30
	}
	if rise == 0 {
		rise = 50
	}
	c := r.Color.Or(color.NRGBA{0xff, 0xff, 0xff, 0xff})
	img := bubbleImage()
	life := rise / speed * 60
	t := float64(g.tick)
	for i := 0; i < n; i++ {
		off := bubbleHash(slot, ri, i)
		ph := t/life + off + float64(i)/float64(n)
		cycle := int(math.Floor(ph))
		p := ph - float64(cycle)
		hx := bubbleHash(slot*31+i, ri, cycle)
		hs := bubbleHash(i, cycle, slot*7+ri)
		bx := x + w*(0.12+0.76*hx) + math.Sin(p*math.Pi*2+hx*6)*4
		by := y + 6 - p*rise*(0.7+0.3*hs)
		a := math.Sin(p * math.Pi)
		a *= 0.75 + 0.25*hs
		rr := size * (0.7 + 0.5*hs) * (1 - 0.3*p)
		k := rr / bubbleDotR
		bubbleOp = ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		bubbleOp.GeoM.Scale(k, k)
		bubbleOp.GeoM.Translate(bx-rr, by-rr)
		bubbleOp.ColorScale.ScaleWithColor(c)
		bubbleOp.ColorScale.ScaleAlpha(float32(a))
		gfx.DrawImage(dst, img, &bubbleOp)
	}
}
