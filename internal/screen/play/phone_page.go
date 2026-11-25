package play

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"rsc.io/qr"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
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
	x, w   float64
	lines  []string
	// inset is how far a row on a card sits in from the page's margins.
	inset float64
	// pin is, for a row at the bottom, how far its top is above the screen's
	// bottom edge; it does not scroll.
	pin float64
	// corner tells a button in the app bar's right end: top and x are on the
	// screen, and it does not scroll.
	corner bool
}

// pageCard is a card behind some of a page's rows, from the page's top unscrolled.
type pageCard struct{ top, h float64 }

const (
	pageCardPad    = 12.0 // a card's padding around its rows
	pageCardRadius = 18.0
	pageBottomGap  = 16.0 // under the last row at the bottom
	pageCornerH    = 32.0 // a button in the app bar's corner
	pageCornerText = 14.0
)

func pageTextSize(r data.PhoneRow) float64 {
	switch r.Size {
	case "large":
		return 22
	case "small":
		return 13
	case "display":
		return 30
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
	if p.cue.App == "home" {
		return p.homeLayout(w)
	}
	out := make([]pageRow, len(p.cue.Rows))
	y := phonePageGap
	var line pageLine
	var bottom, corner []int
	p.cards = p.cards[:0]
	card, inset := "", 0.0
	for i, r := range p.cue.Rows {
		if r.Hidden {
			continue
		}
		if r.Bottom {
			bottom = append(bottom, i)
			continue
		}
		if r.Corner {
			corner = append(corner, i)
			continue
		}
		if r.Card != card {
			line.end(&y)
			if card != "" {
				p.closeCard(&y)
			}
			card, inset = r.Card, 0
			if card != "" {
				p.cards = append(p.cards, pageCard{top: y})
				y += pageCardPad
				inset = pageCardPad
			}
		}
		iw := w - phonePagePad*2 - inset*2
		if pageCell(r) {
			out[i] = line.place(r.Columns, &y, phonePagePad+inset, iw)
			if r.Text != "" {
				out[i].lines = []string{gfx.Ellipsis(r.Text, pageCellText, out[i].w-8)}
			}
			continue
		}
		line.end(&y)
		l := g.pageBlock(r, iw)
		l.top, l.inset = y, inset
		out[i] = l
		y += l.h + phonePageGap
	}
	line.end(&y)
	if card != "" {
		p.closeCard(&y)
	}
	// The rows at the bottom stack up from the screen's bottom edge, in order.
	pin := pageBottomGap
	for _, i := range slices.Backward(bottom) {
		l := g.pageBlock(p.cue.Rows[i], w-phonePagePad*2)
		pin += l.h
		l.pin = pin
		out[i] = l
		pin += phonePageGap
	}
	if len(bottom) > 0 {
		y += pin
	}
	// The buttons in the corner line up from the app bar's right end, the
	// first at the edge.
	right := w - phonePagePad
	for _, i := range corner {
		bw := gfx.TextWidth(p.cue.Rows[i].Button, pageCornerText) + 28
		right -= bw
		out[i] = pageRow{top: phoneFeedTop - pageCornerH - 12, h: pageCornerH, x: right, w: bw, corner: true}
		right -= 8
	}
	p.rows, p.rowsW, p.rowsEnd = out, w, y
	return out
}

// closeCard ends the card being laid out at y, past its padding.
func (p *phoneView) closeCard(y *float64) {
	*y += pageCardPad - phonePageGap
	c := &p.cards[len(p.cards)-1]
	c.h = *y - c.top
	*y += phonePageGap
}

// pageBlock lays out a row that takes a line of its own iw wide: its height
// and its lines (top and inset are the caller's).
func (g *Game) pageBlock(r data.PhoneRow, iw float64) pageRow {
	l := pageRow{}
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
	return l
}

// pageCellText is the text size of a cell (a key or a number ball).
const pageCellText = 17.0

// pageCell tells that a row is a cell sharing its line with the next ones: a
// page's button or text with columns (a keypad's key, a drawn number).
func pageCell(r data.PhoneRow) bool {
	return r.Columns >= 2 && r.Item == nil && r.Picture == "" && (r.Button != "" || r.Text != "")
}

// pageLine is the line of cells being laid out: how many it holds so far and
// to how many columns.
type pageLine struct {
	used, cols int
	h          float64 // the line's cells' height
}

// place lays a cell of cols columns on the line, starting a new line when this
// one is full or has another width, and moves y down past a line it fills.
func (n *pageLine) place(cols int, y *float64, left, iw float64) pageRow {
	if n.used > 0 && n.cols != cols {
		n.end(y)
	}
	n.cols = cols
	cw := (iw - phonePageCell*float64(cols-1)) / float64(cols)
	x0 := left
	if cols >= pageDenseColumns && cw > pageDenseCellMax {
		// On a wide page (a phone held sideways) a grid's cells keep about their
		// height in width, centered, rather than stretching into long bars.
		cw = pageDenseCellMax
		x0 += (iw - cw*float64(cols) - phonePageCell*float64(cols-1)) / 2
	}
	n.h = pageCellHeight(cols)
	if cols >= pageDenseColumns {
		// A dense grid narrower than its cells are tall (on a card) keeps its
		// cells square, so its lines do not stand apart.
		n.h = math.Min(n.h, cw)
	}
	l := pageRow{top: *y, h: n.h, x: x0 + float64(n.used)*(cw+phonePageCell), w: cw}
	n.used++
	if n.used == cols {
		n.end(y)
	}
	return l
}

// end closes a line holding cells, moving y down past it. Lines of narrow
// cells (a keypad, a grid of numbers) sit as close as their cells do, so a
// grid of 43 numbers leaves the page's buttons in view.
func (n *pageLine) end(y *float64) {
	if n.used > 0 {
		gap := phonePageGap
		if n.cols >= pageDenseColumns {
			gap = phonePageCell
		}
		*y += n.h + gap
	}
	n.used = 0
}

// pageDenseColumns is the number of columns from which cells are a dense grid.
const pageDenseColumns = 6

// pageDenseCellMax is the widest a dense grid's cell grows.
const pageDenseCellMax = 64.0

// pageCellHeight is the height of a cell of a line of cols columns: a
// button's, or less on a dense grid whose cells are about as wide.
func pageCellHeight(cols int) float64 {
	if cols >= pageDenseColumns {
		return 40
	}
	return phonePageBtn
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
	end = math.Max(0, p.rowsEnd-(h-phoneFeedTop))
	return rows, end
}

func (g *Game) updatePhonePage(p *phoneView) {
	rows, end := g.pageView(p)
	g.in.receivePick(p)
	if g.pageKeys(p, rows, end) {
		return
	}
	stops := make([]float64, 0, len(rows))
	for _, l := range rows {
		if l.h > 0 && l.pin == 0 && !l.corner {
			stops = append(stops, l.top-phonePageGap)
		}
	}
	g.scrollPhone(p, stops, end, func(at image.Point) {
		if i := g.pageCornerAt(p, rows, at); i >= 0 {
			p.putDown(i, -1, false)
			return
		}
		y, ok := g.phoneColumnAt(p, at)
		sx, _, _, _ := g.phoneScreenRect(p.side)
		x := float64(at.X) - sx
		// The rows at the bottom lie over the rows that scroll: they are hit first.
		for _, pinned := range []bool{true, false} {
			for i, l := range rows {
				top := g.pageRowTop(p, l)
				if ok && !l.corner && (l.pin > 0) == pinned && p.cue.Rows[i].Button != "" && y >= top && y < top+l.h && (l.w == 0 || x >= l.x && x < l.x+l.w) {
					p.putDown(i, -1, false)
					return
				}
			}
		}
	})
	if in := &g.in; in.pressedKey(btnUp) || in.pressedKey(btnDown) {
		g.focusInView(p, rows, true)
	}
}

// pageCornerAt is the button in the app bar's corner under at (a point on the
// game's screen), or -1.
func (g *Game) pageCornerAt(p *phoneView, rows []pageRow, at image.Point) int {
	sx, sy, _, _ := g.phoneScreenRect(p.side)
	x, y := float64(at.X)-sx, float64(at.Y)-sy
	for i, l := range rows {
		// A finger is wider than the small button: a few points around it count.
		if l.corner && l.h > 0 && x >= l.x-6 && x < l.x+l.w+6 && y >= l.top-8 && y < l.top+l.h+8 {
			return i
		}
	}
	return -1
}

// pageButtons are the rows of the page that are buttons.
func pageButtons(rows []data.PhoneRow) []int {
	var out []int
	for i, r := range rows {
		if r.Button != "" && !r.Hidden {
			out = append(out, i)
		}
	}
	return out
}

// pickables are the page's buttons on the screen, for a script's pick and
// look: a pick rings one as the keys would and the OK it sends presses it, so
// a script reaches a phone app by what it shows, as a player does.
func (g *Game) pagePickables(p *phoneView) []pickable {
	rows, _ := g.pageView(p)
	var out []pickable
	for _, i := range pageButtons(p.cue.Rows) {
		if g.pageInView(p, rows[i]) {
			out = append(out, pickable{target: p, label: p.cue.Rows[i].Button, selectIt: func() { p.focus, p.keyed = i, true }})
		}
	}
	return out
}

// pageInView tells that a row is all on the screen.
func (g *Game) pageInView(p *phoneView, l pageRow) bool {
	if l.pin > 0 || l.corner {
		return true
	}
	_, _, _, h := g.phoneScreenRect(p.side)
	return l.top >= p.scroll && l.top+l.h <= p.scroll+h-phoneFeedTop
}

// pageRowTop is a row's top from the page's top unscrolled: a row at the
// bottom is where the screen's bottom edge puts it at the current scroll.
func (g *Game) pageRowTop(p *phoneView, l pageRow) float64 {
	if l.pin > 0 {
		_, _, _, h := g.phoneScreenRect(p.side)
		return p.scroll + h - phoneFeedTop - l.pin
	}
	if l.corner {
		return p.scroll + l.top - phoneFeedTop
	}
	return l.top
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
	textStart, controlStart := g.screenOffsets()
	ground := g.db.Game.UI.Phone.Background.Or(phoneBubble)
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), ground, false)
	rows := p.pageLayout(g, w)
	for _, c := range p.cards {
		y := phoneFeedTop + c.top - p.scroll
		if y+c.h > phoneFeedTop && y < h {
			gfx.FillRoundRect(dst, float32(phonePagePad), float32(y), float32(w-phonePagePad*2), float32(c.h), pageCardRadius, g.db.Game.UI.Phone.Card.Or(phoneBubble))
		}
	}
	pinTop := h
	for i, l := range rows {
		if l.h == 0 {
			continue
		}
		if l.pin > 0 {
			pinTop = math.Min(pinTop, h-l.pin)
			continue
		}
		if l.corner {
			continue
		}
		y := phoneFeedTop + l.top - p.scroll
		if y+l.h <= phoneFeedTop || y >= h {
			continue
		}
		g.drawPageRow(dst, p, i, l, y, w)
	}
	if pinTop < h {
		// The rows at the bottom sit on the page's ground over what scrolls.
		gfx.FillRect(dst, 0, float32(pinTop-phonePageGap), float32(w), float32(h-pinTop+phonePageGap), ground, false)
		for i, l := range rows {
			if l.pin > 0 && l.h > 0 {
				g.drawPageRow(dst, p, i, l, h-l.pin, w)
			}
		}
	}
	// The bar goes over the rows that scroll up under it.
	g.clipScreenRecords(textStart, controlStart, phoneFeedTop, w, h)
	switch accent, ok := g.pageAccent(p); {
	case p.cue.Bar == "large":
		gfx.FillRect(dst, 0, 0, float32(w), float32(phoneFeedTop), ground, false)
		title := gfx.Ellipsis(p.cue.Title, pageTextSize(data.PhoneRow{Size: "display"}), pageCornerLeft(rows, w)-phonePagePad*2)
		g.text(dst, title, phonePagePad+4, 38, 30, g.pageInk(p))
		g.text(dst, title, phonePagePad+4.8, 38, 30, g.pageInk(p))
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
	for i, l := range rows {
		if l.corner && l.h > 0 {
			g.drawPageCorner(dst, p, i, l)
		}
	}
}

// pageCornerLeft is where the app bar's buttons in the corner begin, or the
// page's width when it has none: the title stops short of them.
func pageCornerLeft(rows []pageRow, w float64) float64 {
	left := w
	for _, l := range rows {
		if l.corner && l.h > 0 {
			left = math.Min(left, l.x-8+phonePagePad)
		}
	}
	return left
}

// drawPageCorner draws a button in the app bar's corner: a small pill in the
// page's soft fill with the accent's text, as the secondary buttons are.
func (g *Game) drawPageCorner(dst *ebiten.Image, p *phoneView, i int, l pageRow) {
	label := p.cue.Rows[i].Button
	if g.controlFrame() != nil {
		g.traceControl(label, roundRect(l.x, l.top, l.w, l.h), p.ready(), p.keyed && p.focus == i, "transition")
	}
	accent := g.pageSelection(p)
	if p.keyed && p.focus == i {
		gfx.StrokeRoundRect(dst, float32(l.x-3), float32(l.top-3), float32(l.w+6), float32(l.h+6), float32(l.h/2+3), 2, g.pageInk(p))
	}
	gfx.FillRoundRect(dst, float32(l.x), float32(l.top), float32(l.w), float32(l.h), float32(l.h/2), g.pageSoft(p))
	edge := accent
	edge.A = 0x30
	gfx.StrokeRoundRect(dst, float32(l.x), float32(l.top), float32(l.w), float32(l.h), float32(l.h/2), 1, edge)
	ty := l.top + (l.h-gfx.LineHeight(pageCornerText))/2 + 1
	g.screenText(dst, label, l.x+l.w/2, ty, pageCornerText, accent)
}

func (g *Game) drawPageRow(dst *ebiten.Image, p *phoneView, i int, l pageRow, y, w float64) {
	r := p.cue.Rows[i]
	x, iw := phonePagePad+l.inset, w-(phonePagePad+l.inset)*2
	if l.w > 0 {
		g.drawPageCell(dst, p, i, l, y)
		return
	}
	switch {
	case r.Text != "":
		size := pageTextSize(r)
		for k, line := range l.lines {
			ly := y + float64(k)*gfx.LineHeight(size)
			g.text(dst, line, x, ly, size, g.pageInk(p))
			if r.Bold || r.Size == "display" {
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
		if r.Button != "" {
			g.drawPageTapRing(dst, p, i, r.Button, x, y, iw, l.h)
		}
		for k, line := range l.lines {
			g.text(dst, line, x, y+ph+6+float64(k)*gfx.LineHeight(13), 13, g.pageMuted(p))
		}
	case len(r.Pictures) > 0:
		g.drawPageGrid(dst, r, x, y, iw)
	case r.Item != nil:
		if r.Button != "" {
			g.drawPageTapRing(dst, p, i, r.Button, x, y, iw, l.h)
		}
		g.drawPageItem(dst, p, r.Item, x, y, iw)
	case r.QR != "":
		side := math.Min(iw, phonePageQR)
		p.drawQR(dst, r.QR, w/2, y, side)
		for k, line := range l.lines {
			g.screenText(dst, line, w/2, y+side+8+float64(k)*gfx.LineHeight(13), 13, g.pageMuted(p))
		}
	default:
		if g.controlFrame() != nil {
			g.traceControl(r.Button, roundRect(x, y, iw, phonePageBtn), p.ready(), p.keyed && p.focus == i, "transition")
		}
		accent := g.pageSelection(p)
		if p.keyed && p.focus == i {
			gfx.StrokeRoundRect(dst, float32(x-4), float32(y-4), float32(iw+8), phonePageBtn+8, 18, 2.5, g.pageInk(p))
		}
		fill, ink := accent, pageInkOn(accent)
		if r.Style == "secondary" {
			fill, ink = g.pageSoft(p), accent
		}
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(iw), phonePageBtn, 14, fill)
		if r.Style == "secondary" {
			edge := accent
			edge.A = 0x30
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(iw), phonePageBtn, 14, 1, edge)
		}
		g.screenText(dst, gfx.Ellipsis(r.Button, 17, iw-24), w/2, y+13, 17, ink)
		g.screenText(dst, gfx.Ellipsis(r.Button, 17, iw-24), w/2+0.6, y+13, 17, ink)
	}
}

// drawPageCell draws a cell: a key (a button, a rounded square as a keypad's or
// a mark sheet's) or a ball (a text, round as a drawn number). An unselected
// cell is a faint tile of the page's ink, so a grid of 43 stays readable at a
// glance; a selected one is filled with the selection color, so a page can show
// which numbers are picked and which of the drawn ones match.
func (g *Game) drawPageCell(dst *ebiten.Image, p *phoneView, i int, l pageRow, y float64) {
	r := p.cue.Rows[i]
	pick := g.pageSelection(p)
	x, top, w, h := float32(l.x), float32(y), float32(l.w), float32(l.h)
	radius := float32(8)
	label, ink := r.Button, g.pageInk(p)
	if r.Button == "" {
		label = ""
		if len(l.lines) > 0 {
			label = l.lines[0]
		}
	}
	if r.Button == "" || r.Round {
		// A ball: a circle as wide as the cell is tall, in the cell's middle.
		d := float32(math.Min(l.w, l.h))
		x, top, w, h = x+(w-d)/2, top+(h-d)/2, d, d
		radius = d / 2
	}
	switch {
	case r.Mark:
		mark := g.db.Game.UI.Phone.Mark.Or(pick)
		gfx.FillRoundRect(dst, x, top, w, h, radius, mark)
		ink = pageInkOn(mark)
	case r.Lit:
		gfx.FillRoundRect(dst, x, top, w, h, radius, pick)
		ink = pageInkOn(pick)
	default:
		g.drawPageTile(dst, x, top, w, h, radius)
	}
	if r.Button != "" {
		focused := p.keyed && p.focus == i
		if focused {
			gfx.StrokeRoundRect(dst, x-3, top-3, w+6, h+6, radius+3, 2.5, pick)
		}
		if g.controlFrame() != nil {
			g.traceControl(r.Button, roundRect(l.x, y, l.w, l.h), p.ready(), focused, "transition")
		}
	}
	g.screenText(dst, gfx.Ellipsis(label, pageCellText, l.w-8), l.x+l.w/2, y+(l.h-gfx.LineHeight(pageCellText))/2+2, pageCellText, ink)
	if r.Bold || r.Lit || r.Mark {
		g.screenText(dst, gfx.Ellipsis(label, pageCellText, l.w-8), l.x+l.w/2+0.7, y+(l.h-gfx.LineHeight(pageCellText))/2+2, pageCellText, ink)
	}
}

// drawPageTile draws an unselected cell: a white card edged with a hairline of
// the ink on a light page (as an app's cards sit on its pale ground), a faint
// tile of the ink on a dark one.
func (g *Game) drawPageTile(dst *ebiten.Image, x, y, w, h, radius float32) {
	if soft := g.db.Game.UI.Phone.Soft; soft.Valid {
		gfx.FillRoundRect(dst, x, y, w, h, radius, g.pageSoft(nil))
		return
	}
	ink := g.pageInk(nil)
	bg := g.db.Game.UI.Phone.Background.Or(phoneBubble)
	if 0.299*float64(bg.R)+0.587*float64(bg.G)+0.114*float64(bg.B) > 200 {
		gfx.FillRoundRect(dst, x, y, w, h, radius, phoneBubble)
		ink.A = 0x24
		gfx.StrokeRoundRect(dst, x, y, w, h, radius, 1, ink)
		return
	}
	ink.A = 0x1c
	gfx.FillRoundRect(dst, x, y, w, h, radius, ink)
}

// pageSelection is the color a selected cell and a page's own buttons are
// filled with: game.yaml's ui.phone accent when the game gives one (a page
// whose bar is dark can still mark its picks in a bright color), else the
// page's color.
func (g *Game) pageSelection(p *phoneView) color.NRGBA {
	if a := g.db.Game.UI.Phone.Accent; a.Valid {
		c := a.NRGBA
		c.A = 0xff
		return c
	}
	c, _ := g.pageAccent(p)
	return c
}

// pageSoft is ui.phone's soft fill (an unselected cell, a secondary
// button), or a faint wash of the selection color.
func (g *Game) pageSoft(p *phoneView) color.NRGBA {
	if s := g.db.Game.UI.Phone.Soft; s.Valid {
		c := s.NRGBA
		c.A = 0xff
		return c
	}
	c := g.pageSelection(p)
	c.A = 0x1a
	return c
}

// pageInkOn is the text color on a fill: black on a light one (a gold or a
// pale key), white on a dark one. inkOn gives the same as a color.Color.
func pageInkOn(c color.NRGBA) color.NRGBA {
	if 0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B) < 140 {
		return colorWhite
	}
	return colorBlack
}

// drawPageTapRing marks an item or a picture that is itself the row's button:
// the keys' focus ring when it holds the focus, and the control a trace records.
func (g *Game) drawPageTapRing(dst *ebiten.Image, p *phoneView, i int, label string, x, y, w, h float64) {
	focused := p.keyed && p.focus == i
	if focused {
		gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 10, 2, g.phoneAccent())
	}
	g.traceControl(label, roundRect(x, y, w, h), p.ready(), focused, "transition")
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
	return g.db.Game.UI.Phone.Accent.Or(phoneMine)
}

func (g *Game) pageInk(_ *phoneView) color.NRGBA {
	return g.db.Game.UI.Phone.Text.Or(phoneInk)
}

func (g *Game) pageMuted(_ *phoneView) color.NRGBA {
	return g.db.Game.UI.Phone.Muted.Or(phoneMuted)
}

// homeLayout shares authored row identities with page outcomes while arranging
// only unlocked apps into three columns. No navigation state is saved.
func (p *phoneView) homeLayout(w float64) []pageRow {
	out := make([]pageRow, len(p.cue.Rows))
	cell := (w - phonePagePad*2) / 3
	visible := 0
	p.rowsEnd = 0
	for i, row := range p.cue.Rows {
		if row.Hidden {
			continue
		}
		out[i] = pageRow{top: phonePageGap + float64(visible/3)*116, h: 104, x: phonePagePad + float64(visible%3)*cell, w: cell}
		p.rowsEnd = out[i].top + out[i].h + phonePageGap
		visible++
	}
	p.rows, p.rowsW = out, w
	return out
}

func (g *Game) updatePhoneHome(p *phoneView) {
	g.preparePhoneRows(p)
	defer func() {
		if p.selectedApp != "" {
			id := p.selectedApp
			p.selectedApp = ""
			if err := g.startPhoneGame(p, id); err != nil {
				p.gameError = err.Error()
			}
		}
	}()
	rows, end := g.pageView(p)
	g.in.receivePick(p)
	if g.pageKeys(p, rows, end) {
		return
	}
	buttons := pageButtons(p.cue.Rows)
	if len(buttons) > 0 && (g.in.pressedKey(btnDown) || g.in.pressedKey(btnUp)) {
		step := 3
		if g.in.pressedKey(btnUp) {
			step = -3
		}
		at := slices.Index(buttons, p.focus)
		if at < 0 {
			at = 0
		} else {
			at = max(0, min(len(buttons)-1, at+step))
		}
		p.focus, p.keyed = buttons[at], true
		p.aim = math.Max(0, math.Min(end, rows[p.focus].top-phonePageGap))
	}
	g.dragPhoneAlong(p, false, func(at image.Point) {
		y, ok := g.phoneColumnAt(p, at)
		sx, _, _, _ := g.phoneScreenRect(p.side)
		x := float64(at.X) - sx
		for i, row := range rows {
			if ok && row.h > 0 && x >= row.x && x < row.x+row.w && y >= row.top && y < row.top+row.h {
				p.putDown(i, -1, false)
				return
			}
		}
	})
	if g.in.wheel != 0 {
		p.aim, p.glide = -1, 0
		p.scroll -= g.in.wheel * phoneFeedWheel
	}
	p.settle(end)
}

func (g *Game) drawPhoneHome(p *phoneView, dst *ebiten.Image, w, h float64) {
	textStart, controlStart := g.screenOffsets()
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), g.db.Game.UI.Phone.Background.Or(phoneBubble), false)
	for i, row := range p.pageLayout(g, w) {
		y := phoneFeedTop + row.top - p.scroll
		if row.h == 0 || y+row.h <= phoneFeedTop || y >= h {
			continue
		}
		entry := p.cue.Rows[i]
		icon := ""
		if entry.Item != nil {
			icon = entry.Item.Icon
		}
		cx := row.x + row.w/2
		if p.keyed && p.focus == i {
			gfx.StrokeRoundRect(dst, float32(row.x+3), float32(y), float32(row.w-6), float32(row.h), 12, 2, g.phoneAccent())
		}
		g.drawPhoneIcon(dst, p, icon, entry.Button, cx-27, y+6, 54)
		lines, size := gfx.FitLines(entry.Button, 14, row.w-8, 2)
		for k, line := range lines {
			g.screenText(dst, line, cx, y+66+float64(k)*gfx.LineHeight(size), size, g.pageInk(p))
		}
		g.traceControl(entry.Button, roundRect(row.x, y, row.w, row.h), p.ready(), p.keyed && p.focus == i, "transition")
	}
	g.clipScreenRecords(textStart, controlStart, phoneFeedTop, w, h)
	g.drawPhoneHeader(dst, w, p.cue.Title, 34)
}
