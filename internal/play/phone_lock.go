package play

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The lock screen of the phone (ADR 0057): a large clock and its notices,
// arriving one by one on a press (or by themselves); a notice that is a call
// fills the screen until it is answered or declined.
const (
	phoneLockAuto = 90 // ticks between notices arriving by themselves
	phoneLockIn   = 14 // ticks a notice takes to slide in
)

var (
	phoneLockSky    = color.NRGBA{0x2a, 0x3a, 0x5c, 0xff}
	phoneLockGround = color.NRGBA{0x10, 0x15, 0x22, 0xff}
	phoneCard       = color.NRGBA{0xf4, 0xf5, 0xf8, 0xe8}
	phoneAnswer     = color.NRGBA{0x34, 0xc7, 0x59, 0xff}
	phoneDecline    = color.NRGBA{0xff, 0x3b, 0x30, 0xff}
)

// calling tells that the newest notice on the lock screen is a call.
func (p *phoneView) calling() bool {
	return p.cue.App == "lock" && p.shown > 0 && p.cue.Notices[p.shown-1].Call
}

// declineCall declines the call on the screen: the next notice comes (another
// call rings again) and true is returned, or, at the last one, false lets
// Cancel put the phone down.
func (p *phoneView) declineCall() bool {
	if !p.calling() || p.shown >= len(p.cue.Notices) {
		return false
	}
	p.shown, p.since = p.shown+1, 0
	return true
}

func (g *Game) updatePhoneLock(p *phoneView) {
	in := &g.in
	if p.calling() {
		answer, decline := g.callButtons(p)
		switch {
		case in.ok() || in.click && answer.Dist(in.mouse) <= 40:
			p.putDown(-1, p.shown-1)
		case in.click && decline.Dist(in.mouse) <= 40:
			if !p.declineCall() {
				p.putDown(-1, -1)
			}
		}
		return
	}
	n := len(p.cue.Notices)
	if p.cue.Auto && p.shown < n && p.since >= phoneLockAuto {
		p.shown, p.since = p.shown+1, 0
		return
	}
	if !in.advance() && !g.phoneAuto(p) {
		return
	}
	if p.shown < n {
		p.shown, p.since = p.shown+1, 0
		return
	}
	p.closing = phoneRise
}

// phonePoint is a point on the game's screen.
type phonePoint struct{ x, y float64 }

// Dist is how far the point is from at, in UI units.
func (c phonePoint) Dist(at image.Point) float64 {
	return math.Hypot(float64(at.X)-c.x, float64(at.Y)-c.y)
}

// callButtons are the centers of a call's answer and decline buttons on the game's screen.
func (g *Game) callButtons(p *phoneView) (answer, decline phonePoint) {
	sx, sy, w, h := g.phoneScreenRect(p.side)
	ax, dx, y := callButtonsAt(w, h)
	return phonePoint{sx + ax, sy + y}, phonePoint{sx + dx, sy + y}
}

// callButtonsAt are the buttons' places on a screen w x h: the answer's and
// the decline's x and their y.
func callButtonsAt(w, h float64) (answer, decline, y float64) {
	return w * 0.73, w * 0.27, h - 120
}

func (g *Game) drawPhoneLock(p *phoneView, dst *ebiten.Image, w, h float64) {
	// The sky darkens toward the bottom, in bands.
	for i := range 12 {
		t := float64(i) / 11
		c := phoneLockSky
		c.R = uint8(float64(c.R)*(1-t) + float64(phoneLockGround.R)*t)
		c.G = uint8(float64(c.G)*(1-t) + float64(phoneLockGround.G)*t)
		c.B = uint8(float64(c.B)*(1-t) + float64(phoneLockGround.B)*t)
		gfx.FillRect(dst, 0, float32(h*t*11/12), float32(w), float32(h/12+1), c, false)
	}
	if p.calling() {
		g.drawPhoneCall(p, dst, w, h)
		return
	}
	clock := p.cue.Time
	if clock == "" {
		clock = g.phoneClock()
	}
	gfx.DrawText(dst, p.cue.Date, w/2, 62, 17, phoneOnVideo)
	gfx.DrawText(dst, clock, w/2, 84, 72, phoneOnVideo)
	y := 200.0
	for i := p.shown - 1; i >= 0 && y < h; i-- {
		in := 1.0
		if i == p.shown-1 {
			in = easeOut(math.Min(1, float64(p.since)/phoneLockIn))
		}
		y += g.drawLockNotice(p, dst, i, 10, y-(1-in)*40, w-20, in) + 10
	}
	gfx.FillRoundRect(dst, float32(w/2-60), float32(h-14), 120, 5, 2.5, phoneOnVideo)
}

// drawLockNotice draws a notice's card at x, y, w wide, faded in by alpha, and
// tells its height.
func (g *Game) drawLockNotice(p *phoneView, dst *ebiten.Image, i int, x, y, w, alpha float64) float64 {
	n := p.cue.Notices[i]
	lines, _ := gfx.FitLines(n.Text, 14, w-28, 2)
	ch := 58 + float64(len(lines))*gfx.LineHeight(14)
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(ch), 18, alphaColor(phoneCard, alpha))
	ink, muted := alphaColor(phoneInk, alpha), alphaColor(phoneMuted, alpha)
	g.drawPhoneIcon(dst, p, n.Icon, x+12, y+11, 24)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(n.App, 12, w-120), x+44, y+14, 12, muted)
	gfx.DrawTextRight(dst, lang.T("今"), x+w-14, y+14, 12, muted)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(n.Title, 15, w-28), x+14, y+36, 15, ink)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(n.Title, 15, w-28), x+14.6, y+36, 15, ink)
	for k, line := range lines {
		gfx.DrawTextLeft(dst, line, x+14, y+56+float64(k)*gfx.LineHeight(14), 14, ink)
	}
	return ch
}

// drawPhoneCall draws an incoming call over the whole screen: the app, the
// caller's icon ringing, their name and text, and the two round buttons.
func (g *Game) drawPhoneCall(p *phoneView, dst *ebiten.Image, w, h float64) {
	n := p.cue.Notices[p.shown-1]
	gfx.DrawText(dst, n.App, w/2, 80, 15, alphaColor(phoneOnVideo, 0.75))
	cy := 190.0
	for k := range 2 {
		t := math.Mod(float64(p.since)/50+float64(k)*0.5, 1)
		gfx.StrokeCircle(dst, float32(w/2), float32(cy), float32(56+t*40), 2, alphaColor(phoneOnVideo, 0.5*(1-t)), true)
	}
	g.drawPhoneIcon(dst, p, n.Icon, w/2-56, cy-56, 112)
	gfx.DrawText(dst, gfx.Ellipsis(n.Title, 26, w-32), w/2, cy+80, 26, phoneOnVideo)
	lines, _ := gfx.FitLines(n.Text, 15, w-48, 2)
	for k, line := range lines {
		gfx.DrawText(dst, line, w/2, cy+124+float64(k)*gfx.LineHeight(15), 15, alphaColor(phoneOnVideo, 0.75))
	}
	ax, dx, by := callButtonsAt(w, h)
	for _, b := range []struct {
		x     float64
		fill  color.NRGBA
		label string
	}{{ax, phoneAnswer, lang.T("応答")}, {dx, phoneDecline, lang.T("拒否")}} {
		gfx.FillCircle(dst, float32(b.x), float32(by), 34, b.fill, true)
		gfx.FillRoundRect(dst, float32(b.x-15), float32(by-5), 30, 10, 5, phoneOnVideo)
		gfx.DrawText(dst, b.label, b.x, by+44, 14, phoneOnVideo)
	}
}
