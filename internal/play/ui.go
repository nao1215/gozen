package play

import (
	"image"
	"image/color"
	"math"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

type theme struct {
	window, border, text, accent, cursor, shadow, nameTag, dim color.NRGBA
}

func (g *Game) loadTheme() {
	u := g.db.Game.UI
	g.th = theme{
		window:  u.Window.Or(color.NRGBA{0x18, 0x24, 0x30, 0xe6}),
		border:  u.Border.Or(color.NRGBA{0xa0, 0xb8, 0xc8, 0xff}),
		text:    u.Text.Or(color.NRGBA{0xf0, 0xf4, 0xf8, 0xff}),
		accent:  u.Accent.Or(color.NRGBA{0x60, 0xb8, 0xd0, 0xff}),
		cursor:  u.Cursor.Or(color.NRGBA{0xb0, 0xe0, 0xf0, 0xff}),
		shadow:  u.Shadow.Or(color.NRGBA{0, 0, 0, 0x90}),
		nameTag: u.NameTag.Or(color.NRGBA{0x40, 0x90, 0xb0, 0xff}),
	}
	g.th.dim = color.NRGBA{g.th.text.R, g.th.text.G, g.th.text.B, 0x70}
}

const (
	sizeS  = 20.0
	sizeM  = 24.0
	sizeL  = 30.0
	sizeXL = 44.0
)

func (g *Game) window(dst *ebiten.Image, x, y, w, h float64) {
	gfx.FillRoundRect(dst, float32(x+4), float32(y+6), float32(w), float32(h), 16, g.th.shadow)
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 16, g.th.window)
	gfx.StrokeRoundRect(dst, float32(x+3), float32(y+3), float32(w-6), float32(h-6), 13, 2, withAlpha(g.th.border, 0xc0))
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

func (g *Game) text(dst *ebiten.Image, s string, x, y, size float64, c color.Color) {
	gfx.DrawTextLeft(dst, s, x, y, size, c)
}

func bar(dst *ebiten.Image, x, y, w, h, rate float64, fill color.NRGBA) {
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), float32(h/2), color.NRGBA{0, 0, 0, 0x90})
	rate = math.Max(0, math.Min(1, rate))
	if rate > 0 {
		gfx.FillRoundRect(dst, float32(x+1), float32(y+1), float32(math.Max(h-2, (w-2)*rate)), float32(h-2), float32(h/2-1), fill)
	}
}

func hpColor(rate float64) color.NRGBA {
	switch {
	case rate < 0.25:
		return color.NRGBA{0xff, 0x52, 0x52, 0xff}
	case rate < 0.5:
		return color.NRGBA{0xff, 0xc1, 0x45, 0xff}
	}
	return color.NRGBA{0x7c, 0xe5, 0x8e, 0xff}
}

var mpColor = color.NRGBA{0x6f, 0xb8, 0xff, 0xff}

type listItem struct {
	label    string
	right    string
	disabled bool
	color    color.Color
	id       string

	note      string
	noteColor color.Color
}

type list struct {
	items      []listItem
	sel, top   int
	rows       int
	x, y, w    float64
	rowH, size float64
}

func newList(items []listItem, x, y, w float64, rows int) *list {
	return &list{items: items, x: x, y: y, w: w, rows: rows, rowH: 44, size: sizeM}
}

func (l *list) height() float64 { return float64(min(l.rows, max(1, len(l.items))))*l.rowH + 24 }

func (l *list) rowRect(i int) image.Rectangle {
	ry := l.y + 12 + float64(i-l.top)*l.rowH
	return image.Rect(int(l.x), int(ry), int(l.x+l.w), int(ry+l.rowH))
}

func (l *list) update(g *Game) (chose, cancel bool) {
	in := &g.in
	n := len(l.items)
	if in.cancel() {
		g.sfx("cancel")
		return false, true
	}
	if n == 0 {
		return false, false
	}
	old := l.sel
	if in.up() {
		l.sel = (l.sel - 1 + n) % n
	}
	if in.down() {
		l.sel = (l.sel + 1) % n
	}
	hit := -1
	for i := l.top; i < min(n, l.top+l.rows); i++ {
		if in.mouse.In(l.rowRect(i)) {
			hit = i
		}
	}
	if hit >= 0 && (in.moved || in.click) {
		l.sel = hit
	}
	if l.sel != old {
		g.sfx("cursor")
	}
	l.scroll()
	if in.ok() || (in.click && hit >= 0) {
		if l.items[l.sel].disabled {
			g.sfx("buzzer")
			return false, false
		}
		g.sfx("decide")
		return true, false
	}
	return false, false
}

func (l *list) scroll() {
	if l.sel < l.top {
		l.top = l.sel
	}
	if l.sel >= l.top+l.rows {
		l.top = l.sel - l.rows + 1
	}
}

func (l *list) draw(g *Game, dst *ebiten.Image, active bool) {
	if active {
		g.traceList(l)
	}
	g.window(dst, l.x, l.y, l.w, l.height())
	l.drawRows(g, dst, active)
}

func (l *list) drawRows(g *Game, dst *ebiten.Image, active bool) {
	for i := l.top; i < min(len(l.items), l.top+l.rows); i++ {
		it := l.items[i]
		r := l.rowRect(i)
		ty := float64(r.Min.Y) + (l.rowH-l.size*1.3)/2
		if i == l.sel && active {
			pulse := uint8(0x50 + 0x20*math.Sin(float64(g.tick)/8))
			gfx.FillRoundRect(dst, float32(r.Min.X+10), float32(r.Min.Y+2), float32(l.w-20), float32(l.rowH-4), 10, withAlpha(g.th.accent, pulse))
			g.text(dst, ">", float64(r.Min.X)+16, ty, l.size*0.8, g.th.cursor)
		}
		var c color.Color = g.th.text
		if it.color != nil {
			c = it.color
		}
		if it.disabled {
			c = g.th.dim
		}

		room := float64(l.w) - 42 - 22
		if it.right != "" {
			room -= gfx.TextWidth(it.right, l.size) + 16
		}
		if it.note != "" {
			nc := it.noteColor
			if nc == nil {
				nc = g.th.border
			}
			nx := float64(r.Max.X) - 22
			if it.right != "" {
				nx -= gfx.TextWidth(it.right, l.size) + 16
			}
			ns := l.size * 0.78
			if utf8.RuneCountInString(it.note) == 1 {
				ns = l.size
			}
			gfx.DrawTextRight(dst, it.note, nx, ty+(l.size-ns)*0.55, ns, nc)
			room -= gfx.TextWidth(it.note, ns) + 12
		}
		label, size := it.label, l.size
		if tw := gfx.TextWidth(label, size); tw > room && room > 0 {
			size = math.Max(l.size*0.8, l.size*room/tw)
			label = gfx.Ellipsis(label, size, room)
		}
		g.text(dst, label, float64(r.Min.X)+42, ty+(l.size-size)*0.65, size, c)
		if it.right != "" {
			gfx.DrawTextRight(dst, it.right, float64(r.Max.X)-22, ty, l.size, c)
		}
	}
	if l.top > 0 {
		gfx.DrawText(dst, "▲", l.x+l.w/2, l.y-4, sizeS*0.8, g.th.cursor)
	}
	if l.top+l.rows < len(l.items) {
		gfx.DrawText(dst, "▼", l.x+l.w/2, l.y+l.height()-22, sizeS*0.8, g.th.cursor)
	}
}

func (g *Game) drawLines(dst *ebiten.Image, s string, x, y, w, size float64, c color.Color) float64 {
	lines := gfx.Wrap(s, size, w)
	for i, ln := range lines {
		g.text(dst, ln, x, y+float64(i)*gfx.LineHeight(size), size, c)
	}
	return float64(len(lines)) * gfx.LineHeight(size)
}

func (g *Game) drawLinesFit(dst *ebiten.Image, s string, x, y, w, size float64, lines int, c color.Color) float64 {
	ls, sz := gfx.FitLines(s, size, w, lines)
	for i, ln := range ls {
		g.text(dst, ln, x, y+float64(i)*gfx.LineHeight(sz), sz, c)
	}
	return float64(len(ls)) * gfx.LineHeight(sz)
}

var pixel = func() *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(color.White)
	return img
}()

func rect(dst *ebiten.Image, x, y, w, h float64, c color.Color) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(w, h)
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(c)
	gfx.DrawImage(dst, pixel, op)
}

func fill(dst *ebiten.Image, c color.Color) {
	b := dst.Bounds()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(b.Dx()), float64(b.Dy()))
	op.ColorScale.ScaleWithColor(c)
	dst.DrawImage(pixel, op)
}
