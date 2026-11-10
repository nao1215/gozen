package play

import (
	"image"

	lang "github.com/nao1215/gozen/internal/base/text"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/screen/gfx"
)

type confirm struct {
	msg  string
	yes  func()
	onNo func()
	age  int
	no   bool
}

func newConfirm(msg string, yes func()) *confirm { return &confirm{msg: msg, yes: yes} }

func (c *confirm) overlay() bool { return true }

func (c *confirm) layout(g *Game) (lines []string, size, y, h float64, yes, no image.Rectangle) {
	sw, sh := g.screen()
	width := min(720.0, sw-32)
	maxLines := 2
	if width < 720 {
		// Compact viewports need more wrapped lines, while keeping the controls
		// below the message and the whole dialog inside the screen.
		maxLines = max(2, int((sh-32-104)/gfx.LineHeight(sizeM)))
	}
	lines, size = gfx.FitLines(c.msg, sizeM, min(640, width-40), maxLines)
	h = 104 + float64(len(lines))*gfx.LineHeight(size)
	y = sh/2 - h/2
	by := int(y + h - 66)
	buttonWidth := min(150.0, (width-60)/2)
	yes = image.Rect(int(sw/2-20-buttonWidth), by, int(sw/2-20), by+44)
	no = image.Rect(int(sw/2+20), by, int(sw/2+20+buttonWidth), by+44)
	return
}

func (c *confirm) update(g *Game) {
	c.age++
	if c.age < 6 {
		return
	}
	_, _, _, _, yes, no := c.layout(g)
	if g.in.moved {
		if g.in.mouse.In(yes) {
			c.no = false
		} else if g.in.mouse.In(no) {
			c.no = true
		}
	}
	if g.in.left() || g.in.right() {
		c.no = !c.no
		g.sfx("cursor")
	}
	g.in.receivePick(c)
	accept, refuse := false, g.in.cancel()
	if g.in.ok() {
		accept, refuse = !c.no, c.no
	}
	if g.in.inspect(g.in.click) {
		accept, refuse = g.in.mouse.In(yes), g.in.mouse.In(no)
		g.in.query(accept || refuse)
	}
	switch {
	case accept:
		g.sfx("decide")
		g.pop()
		c.yes()
	case refuse:
		g.sfx("cancel")
		g.pop()
		if c.onNo != nil {
			c.onNo()
		}
	}
}

func (c *confirm) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0x70))
	ls, sz, y, h, yes, no := c.layout(g)
	width := min(720.0, sw-32)
	g.window(dst, (sw-width)/2, y, width, h)
	for i, ln := range ls {
		gfx.DrawText(dst, ln, sw/2, y+20+float64(i)*gfx.LineHeight(sz), sz, g.th.text)
	}
	for i, r := range []image.Rectangle{yes, no} {
		label := lang.T("はい")
		if i == 1 {
			label = lang.T("いいえ")
		}
		on := (i == 1) == c.no
		if on {
			gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 10, withAlpha(g.th.accent, 0x70))
		}
		gfx.StrokeRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 10, 2, g.th.border)
		c2 := g.th.dim
		if on {
			c2 = g.th.text
		}
		gfx.DrawText(dst, label, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y)+(float64(r.Dy())-sizeM*1.3)/2, sizeM, c2)
	}
}
