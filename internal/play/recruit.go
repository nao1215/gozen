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

const recruitLeaveTicks = 36

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
	if joined {
		g.sfx("heart")
	}
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
	var alpha float32
	x, bottom, h := sw*0.5, sh*0.66, sh*0.46
	if v.leave == 0 {
		p := min(1, max(0, float64(g.tick-v.start-12)/24))
		alpha = float32(p)
		bottom += (1 - p) * sh * 0.035
	} else {
		p := math.Min(1, float64(g.tick-v.leave)/recruitLeaveTicks)
		if v.joined {
			if p < 1.0/3.0 {
				bottom -= math.Sin(p*3*math.Pi) * sh * 0.008
				alpha = 1
			} else {
				q := (p - 1.0/3.0) * 1.5
				x -= q * sw * 0.06
				alpha = float32(1 - q)
			}
		} else {
			q := max(0, (p-1.0/3.0)*1.5)
			x += q * sw * 0.05
			alpha = float32(1 - q)
		}
	}
	dim := uint8(51)
	if v.leave == 0 {
		dim = uint8(51 * min(1, float64(g.tick-v.start)/12))
	}
	fill(dst, withAlpha(colorBlack, dim))
	if v.offer.Image != "" {
		gfx.FillRoundRect(dst, float32(x-h*0.38), float32(bottom-sh*0.008), float32(h*0.76), float32(sh*0.025), float32(sh*0.012), color.NRGBA{0, 0, 0, uint8(float32(95) * alpha)})
		g.drawBirthMonster(dst, v.offer.Image, x, bottom, h, sw*0.48, alpha, false)
	}
	if v.leave != 0 && v.joined {
		p := float64(g.tick-v.leave) / recruitLeaveTicks
		for i := range 4 {
			rad := h * 0.25
			sx, sy := x+float64(i-2)*rad*0.35, bottom-h*0.3-p*h*0.2
			c := color.NRGBA{255, 235, 140, uint8(220 * (1 - p))}
			gfx.StrokeLine(dst, float32(sx-7), float32(sy), float32(sx+7), float32(sy), 2, c, true)
			gfx.StrokeLine(dst, float32(sx), float32(sy-7), float32(sx), float32(sy+7), 2, c, true)
		}
	}
}
