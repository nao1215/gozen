package play

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// recruitView keeps the offer picture on screen through its line and choice.
type recruitView struct {
	offer  *event.RecruitOffer
	start  int
	leave  int
	joined bool
}

const recruitLeaveTicks = 28

func recruitName(o *event.RecruitOffer) string {
	if o == nil {
		return ""
	}
	if o.Sex == "male" {
		return o.Name + " ♂"
	}
	return o.Name + " ♀"
}

func (g *Game) beginRecruit(o *event.RecruitOffer) {
	if o == nil {
		return
	}
	if g.recruit != nil && g.recruit.offer.Species == o.Species && g.recruit.leave == 0 {
		return
	}
	g.recruit = &recruitView{offer: o, start: g.tick}
	if o.Image != "" {
		g.pics.get(o.Image)
	}
}

func (g *Game) endRecruit(joined bool) {
	if g.recruit == nil {
		return
	}
	g.recruit.leave, g.recruit.joined = g.tick, joined
}

func (g *Game) drawRecruit(dst *ebiten.Image) {
	v := g.recruit
	if v == nil {
		return
	}
	if v.leave != 0 && g.tick-v.leave >= recruitLeaveTicks {
		g.recruit = nil
		return
	}
	sw, sh := g.screen()
	alpha, x, bottom, h := float32(1), sw*0.5, sh*0.67, sh*0.47
	if v.leave == 0 {
		p := math.Min(1, float64(g.tick-v.start)/24)
		rise := 1 - math.Pow(1-p, 3)
		alpha = float32(p)
		bottom += (1-rise)*sh*0.14 - math.Sin(p*math.Pi)*sh*0.018
		h *= 0.62 + 0.38*rise
	} else {
		p := math.Min(1, float64(g.tick-v.leave)/recruitLeaveTicks)
		alpha = float32(1 - p)
		if v.joined {
			x -= p * sw * 0.32
			bottom += p * sh * 0.18
			h *= 1 - 0.8*p
		} else {
			x += p * sw * 0.27
			bottom += p * sh * 0.07
		}
	}
	fill(dst, withAlpha(colorBlack, uint8(float32(85)*alpha)))
	if v.offer.Image != "" {
		gfx.FillRoundRect(dst, float32(x-h*0.38), float32(sh*0.64), float32(h*0.76), float32(sh*0.045), float32(sh*0.02), color.NRGBA{0, 0, 0, uint8(float32(95) * alpha)})
		g.drawStanding(dst, v.offer.Image, x, bottom, h, alpha, false, nil)
	}
	if v.leave != 0 && v.joined {
		p := float64(g.tick-v.leave) / recruitLeaveTicks
		for i := range 8 {
			a := float64(i)*math.Pi/4 + p*2
			rad := h * (0.3 + 0.35*p)
			sx, sy := x+math.Cos(a)*rad, bottom-h*0.5+math.Sin(a)*rad*0.7
			c := color.NRGBA{255, 235, 140, uint8(220 * (1 - p))}
			gfx.StrokeLine(dst, float32(sx-7), float32(sy), float32(sx+7), float32(sy), 2, c, true)
			gfx.StrokeLine(dst, float32(sx), float32(sy-7), float32(sx), float32(sy+7), 2, c, true)
		}
	}
}
