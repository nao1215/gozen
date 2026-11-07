package play

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"rsc.io/qr"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// The page of the phone (ADR 0057) is any app or web page: a browser's
// address bar or an app bar over rows of text, pictures, a list and buttons,
// scrolled as the feed is.
const (
	phonePagePad  = 16.0  // the rows' margin at the sides
	phonePageGap  = 12.0  // between two rows
	phonePageCell = 6.0   // between the pictures of a grid
	phonePageItem = 62.0  // a list row's height
	phonePageBtn  = 50.0  // a button's height
	phonePageQR   = 220.0 // a QR code's side at most
)

// pageRow is a row laid out on the page, from the page's top unscrolled: its
// top, its height and the lines of its text or its caption.
type pageRow struct {
	top, h float64
	lines  []string
}

func pageTextSize(r data.PhoneRow) float64 {
	switch r.Size {
	case "large":
		return 22
	case "small":
		return 13
	}
	return 16
}

// pageColumns is how many pictures a grid row puts on a line.
func pageColumns(r data.PhoneRow) int {
	if r.Columns == 2 {
		return 2
	}
	return 3
}

// pageLayout lays the rows out top to bottom on a screen w wide (once per width).
func (p *phoneView) pageLayout(g *Game, w float64) []pageRow {
	if p.rows != nil && p.rowsW == w {
		return p.rows
	}
	out := make([]pageRow, len(p.cue.Rows))
	y, iw := phonePageGap, w-phonePagePad*2
	for i, r := range p.cue.Rows {
		l := pageRow{top: y}
		switch {
		case r.Text != "":
			size := pageTextSize(r)
			l.lines = gfx.Wrap(r.Text, size, iw)
			l.h = float64(len(l.lines)) * gfx.LineHeight(size)
		case r.Picture != "":
			l.h = iw * g.pictureAspect(r.Picture)
			if r.Caption != "" {
				l.lines = gfx.Wrap(r.Caption, 13, iw)
				l.h += 6 + float64(len(l.lines))*gfx.LineHeight(13)
			}
		case len(r.Pictures) > 0:
			cols := pageColumns(r)
			cell := (iw - phonePageCell*float64(cols-1)) / float64(cols)
			lines := (len(r.Pictures) + cols - 1) / cols
			l.h = float64(lines)*cell*1.25 + float64(lines-1)*phonePageCell
		case r.Item != nil:
			l.h = phonePageItem
		case r.QR != "":
			l.h = math.Min(iw, phonePageQR)
			if r.Caption != "" {
				l.lines = gfx.Wrap(r.Caption, 13, iw)
				l.h += 8 + float64(len(l.lines))*gfx.LineHeight(13)
			}
		default:
			l.h = phonePageBtn
		}
		out[i] = l
		y += l.h + phonePageGap
	}
	p.rows, p.rowsW = out, w
	return out
}

// pictureAspect is a picture's height over its width (3:4 when it is not loaded).
func (g *Game) pictureAspect(path string) float64 {
	img := g.pics.get(path)
	if img == nil || img == missing || img.Bounds().Dx() == 0 {
		return 0.75
	}
	return float64(img.Bounds().Dy()) / float64(img.Bounds().Dx())
}

// pageView is the page's screen: the rows laid out and the farthest they scroll.
func (g *Game) pageView(p *phoneView) (rows []pageRow, end float64) {
	_, _, w, h := g.phoneScreenRect(p.side)
	rows = p.pageLayout(g, w)
	if n := len(rows); n > 0 {
		end = math.Max(0, rows[n-1].top+rows[n-1].h+phonePageGap-(h-phoneFeedTop))
	}
	return rows, end
}

func (g *Game) updatePhonePage(p *phoneView) {
	rows, end := g.pageView(p)
	if g.pageKeys(p, rows, end) {
		return
	}
	stops := make([]float64, len(rows))
	for i, l := range rows {
		stops[i] = l.top - phonePageGap
	}
	g.scrollPhone(p, stops, end, func(at image.Point) {
		y, ok := g.phoneColumnAt(p, at)
		for i, l := range rows {
			if ok && p.cue.Rows[i].Button != "" && y >= l.top && y < l.top+l.h {
				p.putDown(i, -1, false)
			}
		}
	})
	if in := &g.in; in.pressedKey(btnUp) || in.pressedKey(btnDown) {
		g.focusInView(p, rows, true)
	}
}

// pageButtons are the rows of the page that are buttons.
func pageButtons(rows []data.PhoneRow) []int {
	var out []int
	for i, r := range rows {
		if r.Button != "" {
			out = append(out, i)
		}
	}
	return out
}

// pageInView tells that a row is all on the screen.
func (g *Game) pageInView(p *phoneView, l pageRow) bool {
	_, _, _, h := g.phoneScreenRect(p.side)
	return l.top >= p.scroll && l.top+l.h <= p.scroll+h-phoneFeedTop
}

// focusInView moves the keys' focus to the first button on the screen when the
// focused one is not (keyed: the keys moved it, so the ring shows).
func (g *Game) focusInView(p *phoneView, rows []pageRow, keyed bool) {
	at := p.scroll
	if p.aim >= 0 {
		at = p.aim
	}
	was := p.scroll
	p.scroll = at
	defer func() { p.scroll = was }()
	if p.focus >= 0 && g.pageInView(p, rows[p.focus]) {
		p.keyed = p.keyed || keyed
		return
	}
	p.focus = -1
	for _, i := range pageButtons(p.cue.Rows) {
		if g.pageInView(p, rows[i]) {
			p.focus, p.keyed = i, p.keyed || keyed
			return
		}
	}
}

// pageKeys moves the keys' focus between the page's buttons (left, right, Tab)
// and presses the focused one with OK, the first OK showing the ring when the
// keys have not moved it yet. It tells whether it took the input.
func (g *Game) pageKeys(p *phoneView, rows []pageRow, end float64) bool {
	in := &g.in
	buttons := pageButtons(p.cue.Rows)
	if len(buttons) == 0 {
		return false
	}
	var d int
	switch {
	case in.pressedKey(btnRight) || in.pressedKey(btnNext):
		d = 1
	case in.pressedKey(btnLeft):
		d = -1
	case in.inspect(in.pressed[btnOK]):
		g.focusInView(p, rows, false)
		switch {
		case p.focus >= 0 && p.keyed:
			p.putDown(p.focus, -1, false)
			return in.query(true)
		case p.focus >= 0:
			p.keyed = true
			return in.query(true)
		}
		return false
	default:
		return false
	}
	k := slices.Index(buttons, p.focus)
	switch {
	case k < 0 && d > 0:
		k = 0
	case k < 0:
		k = len(buttons) - 1
	default:
		k = (k + d + len(buttons)) % len(buttons)
	}
	p.focus, p.keyed = buttons[k], true
	l := rows[p.focus]
	_, _, _, h := g.phoneScreenRect(p.side)
	if !g.pageInView(p, l) {
		p.aim = math.Max(0, math.Min(end, l.top+l.h+phonePageGap-(h-phoneFeedTop)))
		if l.top < p.scroll {
			p.aim = math.Max(0, l.top-phonePageGap)
		}
	}
	return true
}

// pageAccent is the page's color: its app bar's, or the phone's blue.
func (g *Game) pageAccent(p *phoneView) (color.NRGBA, bool) {
	if c, ok := parseDotColor(p.cue.Color); ok {
		c.A = 0xff
		return c, true
	}
	return g.phoneAccent(), false
}

func (g *Game) drawPhonePage(p *phoneView, dst *ebiten.Image, w, h float64) {
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), g.db.Game.UI.PhoneBackground.Or(phoneBubble), false)
	for i, l := range p.pageLayout(g, w) {
		y := phoneFeedTop + l.top - p.scroll
		if y+l.h <= phoneFeedTop || y >= h {
			continue
		}
		g.drawPageRow(dst, p, i, l, y, w)
	}
	// The bar goes over the rows that scroll up under it.
	switch accent, ok := g.pageAccent(p); {
	case p.cue.URL != "":
		gfx.FillRect(dst, 0, 0, float32(w), float32(phoneFeedTop), phoneBubble, false)
		gfx.FillRoundRect(dst, 12, 40, float32(w-24), 34, 17, phoneScreen)
		g.screenText(dst, gfx.Ellipsis(p.cue.URL, 14, w-60), w/2, 48, 14, phoneMuted)
		gfx.FillRect(dst, 0, float32(phoneFeedTop-1), float32(w), 1, phoneLine, false)
	case ok:
		gfx.FillRect(dst, 0, 0, float32(w), float32(phoneFeedTop), accent, false)
		g.screenText(dst, gfx.Ellipsis(p.cue.Title, 19, w-60), w/2, 46, 19, phoneOnVideo)
	default:
		g.drawPhoneHeader(dst, w, p.cue.Title, 34)
	}
}

func (g *Game) drawPageRow(dst *ebiten.Image, p *phoneView, i int, l pageRow, y, w float64) {
	r := p.cue.Rows[i]
	x, iw := phonePagePad, w-phonePagePad*2
	switch {
	case r.Text != "":
		size := pageTextSize(r)
		for k, line := range l.lines {
			ly := y + float64(k)*gfx.LineHeight(size)
			g.text(dst, line, x, ly, size, g.pageInk(p))
			if r.Bold {
				g.text(dst, line, x+0.7, ly, size, g.pageInk(p))
			}
		}
	case r.Picture != "":
		ph := iw * g.pictureAspect(r.Picture)
		if img := g.pics.get(r.Picture); img != nil {
			drawContain(dst, img, x, y, iw, ph)
		}
		if r.Shut {
			g.drawLocked(dst, x, y, iw, ph, r.Label)
		}
		for k, line := range l.lines {
			g.text(dst, line, x, y+ph+6+float64(k)*gfx.LineHeight(13), 13, g.pageMuted(p))
		}
	case len(r.Pictures) > 0:
		g.drawPageGrid(dst, r, x, y, iw)
	case r.Item != nil:
		g.drawPageItem(dst, p, r.Item, x, y, iw)
	case r.QR != "":
		side := math.Min(iw, phonePageQR)
		p.drawQR(dst, r.QR, w/2, y, side)
		for k, line := range l.lines {
			g.screenText(dst, line, w/2, y+side+8+float64(k)*gfx.LineHeight(13), 13, g.pageMuted(p))
		}
	default:
		if g.tr != nil {
			g.traceControl(r.Button, roundRect(x, y, iw, phonePageBtn), p.ready(), p.keyed && p.focus == i, "transition")
		}
		accent, _ := g.pageAccent(p)
		if p.keyed && p.focus == i {
			gfx.StrokeRoundRect(dst, float32(x-4), float32(y-4), float32(iw+8), phonePageBtn+8, 15, 2.5, g.pageInk(p))
		}
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(iw), phonePageBtn, 12, accent)
		g.screenText(dst, gfx.Ellipsis(r.Button, 17, iw-24), w/2, y+13, 17, phoneOnVideo)
	}
}

func (g *Game) drawPageGrid(dst *ebiten.Image, r data.PhoneRow, x, y, iw float64) {
	cols := pageColumns(r)
	cell := (iw - phonePageCell*float64(cols-1)) / float64(cols)
	for k, path := range r.Pictures {
		cx := x + float64(k%cols)*(cell+phonePageCell)
		cy := y + float64(k/cols)*(cell*1.25+phonePageCell)
		gfx.FillRect(dst, float32(cx), float32(cy), float32(cell), float32(cell*1.25), phoneScreen, false)
		if img := g.pics.get(path); img != nil {
			drawCoverTop(dst, img, cx, cy, cell, cell*1.25)
		}
		if r.Shut {
			g.drawLocked(dst, cx, cy, cell, cell*1.25, r.Label)
		}
	}
}

// phoneLockVeil darkens a locked picture.
var phoneLockVeil = color.NRGBA{0x18, 0x1a, 0x22, 0xe0}

// drawLocked covers a locked picture in x, y, w, h: darkened, with a padlock
// and its label in the middle.
func (g *Game) drawLocked(dst *ebiten.Image, x, y, w, h float64, label string) {
	gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(h), phoneLockVeil, false)
	cx, cy := x+w/2, y+h/2
	if label != "" {
		cy -= 10
	}
	k := math.Min(1, w/120)
	gfx.StrokeCircle(dst, float32(cx), float32(cy-8*k), float32(9*k), float32(3*k), phoneOnVideo, true)
	gfx.FillRoundRect(dst, float32(cx-13*k), float32(cy-6*k), float32(26*k), float32(20*k), float32(4*k), phoneOnVideo)
	if label != "" {
		size := math.Max(10, 13*k)
		g.screenText(dst, gfx.Ellipsis(label, size, w-8), cx, cy+20*k, size, phoneOnVideo)
	}
}

func (g *Game) drawPageItem(dst *ebiten.Image, p *phoneView, it *data.PhoneItem, x, y, iw float64) {
	tx := x
	if it.Icon != "" {
		g.drawPhoneIcon(dst, p, it.Icon, it.Title, x, y+10, 42)
		tx += 54
	}
	vw := gfx.TextWidth(it.Value, 16)
	room := x + iw - tx - vw - 10
	g.text(dst, gfx.Ellipsis(it.Title, 16, room), tx, y+10, 16, g.pageInk(p))
	g.text(dst, gfx.Ellipsis(it.Text, 13, room), tx, y+34, 13, g.pageMuted(p))
	g.screenTextRight(dst, it.Value, x+iw, y+20, 16, g.pageInk(p))
	gfx.FillRect(dst, float32(tx), float32(y+phonePageItem+phonePageGap/2), float32(x+iw-tx), 1, phoneLine, false)
}

// qrQuiet is the quiet zone around a QR code, in modules.
const qrQuiet = 4

// qrPixels are the pixels of a QR code of text, one a module, black on white
// with the quiet zone around: n x n RGBA bytes.
func qrPixels(text string) (pix []byte, n int, err error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return nil, 0, err
	}
	n = code.Size + qrQuiet*2
	pix = make([]byte, n*n*4)
	for i := range pix {
		pix[i] = 0xff
	}
	for y := range code.Size {
		for x := range code.Size {
			if code.Black(x, y) {
				o := ((y+qrQuiet)*n + x + qrQuiet) * 4
				pix[o], pix[o+1], pix[o+2] = 0, 0, 0
			}
		}
	}
	return pix, n, nil
}

// qrModule is how many pixels a module takes for a code of n modules in side
// pixels: a whole number, so the code stays crisp.
func qrModule(side float64, n int) int { return max(1, int(side)/n) }

// drawQR draws the QR code of text centered at cx, at y, side UI units at most,
// each module a whole number of pixels (nearest, never smoothed).
func (p *phoneView) drawQR(dst *ebiten.Image, text string, cx, y, side float64) {
	img, ok := p.qrs[text]
	if !ok {
		pix, n, err := qrPixels(text)
		if err != nil {
			p.qrs[text] = nil
			return
		}
		img = ebiten.NewImage(n, n)
		img.WritePixels(pix)
		p.qrs[text] = img
	}
	if img == nil {
		return
	}
	n := img.Bounds().Dx()
	k := gfx.Scale()
	m := qrModule(side*k, n)
	px := float64(m * n)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Scale(float64(m), float64(m))
	op.GeoM.Translate(math.Round(cx*k-px/2), math.Round(y*k+(side*k-px)/2))
	dst.DrawImage(img, op)
}

func (g *Game) phoneAccent() color.NRGBA {
	return g.db.Game.UI.PhoneAccent.Or(phoneMine)
}

func (g *Game) pageInk(_ *phoneView) color.NRGBA {
	return g.db.Game.UI.PhoneText.Or(phoneInk)
}

func (g *Game) pageMuted(_ *phoneView) color.NRGBA {
	return g.db.Game.UI.PhoneMuted.Or(phoneMuted)
}
