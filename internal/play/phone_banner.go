package play

import (
	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// A phone banner (the phone command's app banner, ADR 0057) slides down from
// the top of the screen as a phone's push notification does, over any scene,
// while the event goes on; it goes up again after a few seconds. A click on it
// opens its event (after the event that is running, if one is).
const (
	bannerSlide = 18  // ticks to slide down or up
	bannerStay  = 240 // ticks it stays down
	bannerW     = 440.0
	bannerH     = 88.0
)

// phoneBanner is a banner waiting or on the screen: the notice and its age in ticks.
type phoneBanner struct {
	n   data.PhoneNotice
	age int
}

// bannerTop is how far down the banner on the screen is (its top), sliding.
func (b phoneBanner) top() float64 {
	t := 1.0
	switch end := bannerSlide*2 + bannerStay; {
	case b.age < bannerSlide:
		t = easeOut(float64(b.age) / bannerSlide)
	case b.age > end-bannerSlide:
		t = easeOut(float64(end-b.age) / bannerSlide)
	}
	return -bannerH + (bannerH+14)*t
}

// updateBanners ages the banner on the screen, drops it at its end, and opens
// its event on a click on it (taking the click from the scene).
func (g *Game) updateBanners() {
	if len(g.banners) == 0 {
		return
	}
	b := &g.banners[0]
	if g.skipping {
		b.age = max(b.age, bannerSlide+bannerStay)
	}
	b.age++
	if b.age >= bannerSlide*2+bannerStay {
		g.banners = g.banners[1:]
		return
	}
	sw, _ := g.screen()
	x, y := (sw-bannerW)/2, b.top()
	m := g.in.mouse
	on := float64(m.X) >= x && float64(m.X) < x+bannerW && float64(m.Y) >= y && float64(m.Y) < y+bannerH
	if !g.in.click || !on || b.n.Event == "" {
		return
	}
	g.in.click, g.in.pdown, g.in.pup = false, false, false
	if err := g.events.StartBound(b.n.Event, nil); err != nil {
		g.events.Log(err)
	}
	b.n.Event = ""
	b.age = max(b.age, bannerSlide+bannerStay)
}

func (g *Game) drawBanners(dst *ebiten.Image) {
	if len(g.banners) == 0 {
		return
	}
	b := g.banners[0]
	sw, _ := g.screen()
	x, y := (sw-bannerW)/2, b.top()
	gfx.FillRoundRect(dst, float32(x), float32(y+3), bannerW, bannerH, 20, phoneVeil)
	gfx.FillRoundRect(dst, float32(x), float32(y), bannerW, bannerH, 20, phoneCard)
	if b.n.Icon != "" {
		g.drawPicture(dst, data.Picture{File: b.n.Icon}, x+14, y+14, 22, 22, 1)
	}
	gfx.DrawTextLeft(dst, gfx.Ellipsis(b.n.App, 13, bannerW-140), x+44, y+16, 13, phoneMuted)
	gfx.DrawTextRight(dst, lang.T("今"), x+bannerW-16, y+16, 13, phoneMuted)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(b.n.Title, 15, bannerW-32), x+16, y+38, 15, phoneInk)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(b.n.Title, 15, bannerW-32), x+16.6, y+38, 15, phoneInk)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(b.n.Text, 14, bannerW-32), x+16, y+60, 14, phoneInk)
}
