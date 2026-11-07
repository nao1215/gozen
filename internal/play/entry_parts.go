package play

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// The reading and pressing parts of a workspace: a plate is a quiet frosted
// board that never takes focus, a button a pressable pill with an edge, a ticket
// card the face of one piece of work, and a carousel a row of square boxes that
// rotates under the left and right keys with the chosen box in the middle.

func exprNumber(x data.Expr, env expr.Env) float64 {
	if !x.Set() {
		return 0
	}
	n, err := x.Number(env)
	if err != nil || math.IsNaN(n) {
		return 0
	}
	return n
}

func exprTruth(x data.Expr, env expr.Env) bool {
	if !x.Set() {
		return false
	}
	b, err := x.Bool(env)
	return err == nil && b
}

// renderEntryParts interpolates the text of a row's parts once per refresh.
// The ticket is copied so the definition shared by every row stays untouched.
func renderEntryParts(g *Game, e *data.ScreenEntry, env expr.Env) {
	e.Cost = data.Render(g.db.Tr(e.Cost), env)
	e.Reason = data.Render(g.db.Tr(e.Reason), env)
	e.Drag = data.Render(e.Drag, env)
	e.Peek = data.Render(g.db.Tr(e.Peek), env)
	e.Dot = data.Render(e.Dot, env)
	e.Sign = data.Render(e.Sign, env)
	if e.Bust != nil {
		b := *e.Bust
		b.Picture = data.Render(b.Picture, env)
		e.Bust = &b
	}
	if e.Ticket == nil {
		return
	}
	t := *e.Ticket
	t.Due = data.Render(g.db.Tr(t.Due), env)
	t.State = data.Render(g.db.Tr(t.State), env)
	t.After = nil
	for _, name := range e.Ticket.After {
		if name = strings.TrimSpace(data.Render(g.db.Tr(name), env)); name != "" {
			t.After = append(t.After, name)
		}
	}
	e.Ticket = &t
}

// unavailable reports whether the row's action is shown but refused now.
func (s *authorScreen) unavailable(g *Game, i int) bool {
	if i < 0 || i >= len(s.entries) {
		return false
	}
	available, err := s.rows[i].Available(g.db, s.entries[i], s.env(g))
	return err != nil || !available
}

// ringMembers are the visible carousel members in authored order.
func (s *authorScreen) ringMembers() []int {
	var out []int
	for i, e := range s.entries {
		if e.Carousel != "" && len(e.Bounds) == 4 && e.Target.ID == "" && (len(e.Do) > 0 || e.Destination != "") {
			out = append(out, i)
		}
	}
	return out
}

// ringCenter is the entry index in the middle of the carousel, or -1.
func (s *authorScreen) ringCenter() int {
	members := s.ringMembers()
	if len(members) == 0 {
		return -1
	}
	return members[max(0, min(s.ringAt, len(members)-1))]
}

// ringKey identifies the middle member across refreshes by entry and row.
func (s *authorScreen) ringKey() (int, map[string]any, bool) {
	c := s.ringCenter()
	if c < 0 {
		return 0, nil, false
	}
	return s.indices[c], s.rows[c].Args, true
}

// keepRing restores the middle member after the rows were rebuilt.
func (s *authorScreen) keepRing(index int, args map[string]any, ok bool) {
	if !ok {
		return
	}
	for k, i := range s.ringMembers() {
		if s.indices[i] == index && maps.Equal(s.rows[i].Args, args) {
			s.ringAt = k
			return
		}
	}
	s.ringAt = max(0, min(s.ringAt, len(s.ringMembers())-1))
}

// syncRing puts entry i in the middle when it is a carousel member.
func (s *authorScreen) syncRing(i int) {
	for k, m := range s.ringMembers() {
		if m == i {
			s.ringAt = k
		}
	}
}

// rotateRing turns the carousel by d and moves the focus onto its middle.
func (s *authorScreen) rotateRing(d int, actions []int) bool {
	members := s.ringMembers()
	if len(members) == 0 || d == 0 {
		return false
	}
	s.ringAt = ((s.ringAt+d)%len(members) + len(members)) % len(members)
	s.ringAnim = float64(d)
	for j, i := range actions {
		if i == members[s.ringAt] {
			s.focus = j
		}
	}
	return true
}

// ringSkipped reports whether a focus stop is a carousel member off the middle:
// up and down pass over them, so the whole carousel is a single stop.
func (s *authorScreen) ringSkipped(focus int, actions []int) bool {
	if focus < 0 || focus >= len(actions) {
		return false
	}
	i := actions[focus]
	return s.entries[i].Carousel != "" && i != s.ringCenter()
}

// carouselBox0 is the size of the middle box: the band's height less the
// caption under it, from 48 up to 180 logical pixels.
func carouselBox0(r image.Rectangle) float64 { return max(48, min(float64(r.Dy())-86, 180)) }

// carouselBox is one drawn box: its member, square and how far from the middle.
type carouselBox struct {
	member int
	x, y   float64
	size   float64
	alpha  float32
	step   int
}

func (b carouselBox) rect() image.Rectangle {
	return image.Rect(int(b.x), int(b.y), int(b.x+b.size), int(b.y+b.size))
}

// carouselLayout is shared by drawing and hit testing. It returns the band,
// the boxes from the outside in (the middle last) and the two arrow buttons.
func (s *authorScreen) carouselLayout() (image.Rectangle, []carouselBox, [2]image.Rectangle) {
	members := s.ringMembers()
	if len(members) == 0 {
		return image.Rectangle{}, nil, [2]image.Rectangle{}
	}
	r := entryBounds(s.entries[members[0]])
	n := len(members)
	center := max(0, min(s.ringAt, n-1))
	big := carouselBox0(r)
	cx := float64(r.Min.X+r.Max.X) / 2
	cy := float64(r.Min.Y) + 8 + big/2
	scales := []float64{1, .72, .56, .44}
	alphas := []float32{1, .55, .32, .18}
	const gap = 18.0
	pos := func(k int) float64 {
		x := 0.0
		for a := 1; a <= abs(k); a++ {
			x += big*scales[a-1]/2 + gap + big*scales[a]/2
		}
		if k < 0 {
			return -x
		}
		return x
	}
	left := min(3, (n-1)/2)
	right := min(3, n-1-left)
	shift := s.ringAnim * (big/2 + gap + big*scales[1]/2)
	lo, hi := float64(r.Min.X)+44, float64(r.Max.X)-44
	var boxes []carouselBox
	add := func(k int) {
		d := abs(k)
		size := big * scales[d]
		x := cx + pos(k) + shift - size/2
		if x < lo || x+size > hi {
			return
		}
		boxes = append(boxes, carouselBox{member: members[((center+k)%n+n)%n], x: x, y: cy - size/2, size: size, alpha: alphas[d], step: k})
	}
	// From the outside in, so the middle box is drawn over its neighbors.
	for d := 3; d >= 1; d-- {
		if d <= left {
			add(-d)
		}
		if d <= right {
			add(d)
		}
	}
	add(0)
	arrow := 32
	ay := int(cy) - arrow/2
	arrows := [2]image.Rectangle{
		image.Rect(r.Min.X+8, ay, r.Min.X+8+arrow, ay+arrow),
		image.Rect(r.Max.X-8-arrow, ay, r.Max.X-8, ay+arrow),
	}
	return r, boxes, arrows
}

// updateCarousel turns or confirms the carousel. It reports whether the input
// was used; activate is called for a click on the middle box.
func (s *authorScreen) updateCarousel(g *Game, actions []int) bool {
	members := s.ringMembers()
	if len(members) == 0 {
		return false
	}
	s.ringAnim *= .6
	if math.Abs(s.ringAnim) < .02 {
		s.ringAnim = 0
	}
	if g.in.inspect(g.in.pressed[btnLeft]) {
		g.in.query(s.rotateRing(-1, actions))
		s.keyed = true
		return true
	}
	if g.in.inspect(g.in.pressed[btnRight]) {
		g.in.query(s.rotateRing(1, actions))
		s.keyed = true
		return true
	}
	if !g.in.inspect(g.in.click) {
		return false
	}
	band, boxes, arrows := s.carouselLayout()
	for k, a := range arrows {
		if g.in.mouse.In(a) {
			g.in.query(s.rotateRing(2*k-1, actions))
			return true
		}
	}
	for _, box := range slices.Backward(boxes) {
		if !g.in.mouse.In(box.rect()) {
			continue
		}
		if box.step != 0 {
			g.in.query(s.rotateRing(box.step, actions))
			return true
		}
		for j, i := range actions {
			if i == box.member {
				s.focus = j
			}
		}
		s.list.sel = box.member
		g.in.query(s.activate(g))
		return true
	}
	// The band itself is not a control: a click on its empty part does nothing.
	return g.in.mouse.In(band)
}

// drawCarousel draws the band, the boxes and the name, cost and detail of the
// middle member. A refused member is faded and shows its reason.
func (s *authorScreen) drawCarousel(g *Game, dst *ebiten.Image, focused bool) {
	band, boxes, arrows := s.carouselLayout()
	if band.Empty() {
		return
	}
	g.surfaceWindow(dst, float64(band.Min.X), float64(band.Min.Y), float64(band.Dx()), float64(band.Dy()), "control")
	for _, box := range boxes {
		e := s.entries[box.member]
		refused := s.unavailable(g, box.member)
		if g.controlFrame() != nil {
			g.traceControl(s.entries[box.member].Label, roundRect(box.x, box.y, box.size, box.size), !refused, focused && box.member == s.ringCenter(), s.entries[box.member].Reason)
		}
		tint := g.th.window
		if box.step == 0 && g.in.mouse.In(box.rect()) {
			tint = g.th.window
		}
		if !g.materialSurface(dst, box.x, box.y, box.size, box.size, tint, "card") {
			gfx.FillRoundRect(dst, float32(box.x), float32(box.y), float32(box.size), float32(box.size), 14, withAlpha(tint, uint8(255*box.alpha)))
		}
		alpha := box.alpha
		if refused {
			alpha *= .35
		}
		pad := box.size * .12
		if e.Bust != nil && e.Bust.Picture != "" {
			// A bust fills the whole box: a close crop of a standing picture,
			// placed by its face box, where an icon would leave a margin.
			_, top, face := bustLayout(image.Rect(0, 0, 1, 1), e.Bust)
			if e.Bust.Fit == "contain" {
				g.drawPictureContain(dst, e.Bust.Picture, box.x+4, box.y+4, box.size-8, box.size-8, 1)
			} else {
				g.planningBust(dst, e.Bust.Picture, box.x+4, box.y+4, box.size-8, box.size-8, top, face)
			}
			if alpha < 1 {
				gfx.FillRoundRect(dst, float32(box.x), float32(box.y), float32(box.size), float32(box.size), 14, color.NRGBA{235, 240, 246, uint8(255 * (1 - alpha))})
			}
		} else if e.Picture != "" {
			if e.PictureFit == "contain" {
				g.drawPictureContain(dst, e.Picture, box.x+pad, box.y+pad, box.size-2*pad, box.size-2*pad, float32(alpha))
			} else {
				g.drawPicture(dst, data.Picture{File: e.Picture}, box.x+pad, box.y+pad, box.size-2*pad, box.size-2*pad, alpha)
			}
		} else {
			mark := []rune(strings.TrimSpace(e.Label))
			if len(mark) > 0 {
				size := box.size * .42
				t := withAlpha(g.th.text, uint8(255*alpha))
				g.text(dst, string(mark[0]), box.x+box.size/2-gfx.TextWidth(string(mark[0]), size)/2, box.y+box.size/2-size*.7, size, t)
			}
		}
		if box.step != 0 {
			// Off-middle boxes recede: a veil of the band keeps them quieter.
			gfx.FillRoundRect(dst, float32(box.x), float32(box.y), float32(box.size), float32(box.size), 14, color.NRGBA{235, 240, 246, uint8(150 * (1 - box.alpha))})
			continue
		}
		edge := color.NRGBA{255, 255, 255, 220}
		width := float32(1.5)
		if focused {
			edge, width = g.th.accent, 3
		}
		gfx.StrokeRoundRect(dst, float32(box.x), float32(box.y), float32(box.size), float32(box.size), 14, width, edge)
		if refused {
			drawLock(dst, box.x+box.size-22, box.y+10, g.th.dim)
		}
	}
	for k, a := range arrows {
		r, tint := g.controlMaterial(a)
		if !g.materialSurface(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), tint, "control") {
			gfx.FillCircle(dst, float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2), float32(r.Dx()/2), tint, true)
		}
		cx, cy := float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2)
		d := float32(5 * (2*k - 1))
		gfx.StrokeLine(dst, cx-d/2, cy-7, cx+d/2, cy, 2.5, g.th.accent, true)
		gfx.StrokeLine(dst, cx+d/2, cy, cx-d/2, cy+7, 2.5, g.th.accent, true)
	}
	middle := s.ringCenter()
	if middle < 0 {
		return
	}
	e := s.entries[middle]
	refused := s.unavailable(g, middle)
	var big float64
	for _, b := range boxes {
		if b.step == 0 {
			big = b.size
		}
	}
	if big == 0 {
		big = carouselBox0(band)
	}
	cx := float64(band.Min.X+band.Max.X) / 2
	ty := float64(band.Min.Y) + 8 + big + 8
	labelTint := g.th.text
	if focused {
		labelTint = g.th.accent
	}
	if refused {
		labelTint = g.th.dim
	}
	const nameSize, costSize = 19.0, 15.0
	width := float64(band.Dx()) - 120
	label := gfx.Ellipsis(e.Label, nameSize, width*.7)
	lw := gfx.TextWidth(label, nameSize)
	cw := 0.0
	if e.Cost != "" {
		cw = gfx.TextWidth(e.Cost, costSize) + 20
	}
	spacing := 0.0
	if cw > 0 {
		spacing = 10
	}
	x := cx - (lw+cw+spacing)/2
	g.text(dst, label, x, ty, nameSize, labelTint)
	if cw > 0 {
		chipX := x + lw + 10
		gfx.FillRoundRect(dst, float32(chipX), float32(ty+2), float32(cw), 22, 11, withAlpha(g.th.accent, 36))
		g.text(dst, e.Cost, chipX+10, ty+3, costSize, g.th.accent)
	}
	detail, tint := e.Text, g.th.dim
	if refused && e.Reason != "" {
		detail, tint = e.Reason, g.th.accent
	}
	lines := max(1, int((float64(band.Max.Y)-(ty+30)-4)/gfx.LineHeight(15)))
	if detail != "" {
		wrapped, size := gfx.FitLines(detail, 15, width, lines)
		for n, line := range wrapped {
			g.text(dst, line, cx-gfx.TextWidth(line, size)/2, ty+30+float64(n)*gfx.LineHeight(size), size, tint)
		}
	}
	// Small dots show how many there are and where the middle is.
	members := s.ringMembers()
	if len(members) > 1 {
		dot := 7.0
		total := float64(len(members))*dot + float64(len(members)-1)*6
		dx := float64(band.Max.X) - 24 - total
		dy := float64(band.Max.Y) - 16
		for k := range members {
			c := withAlpha(g.th.dim, 90)
			if k == s.ringAt {
				c = g.th.accent
			}
			gfx.FillCircle(dst, float32(dx+float64(k)*(dot+6)+dot/2), float32(dy), float32(dot/2), c, true)
		}
	}
}

// drawLock is a small padlock: a body and a shackle.
func drawLock(dst *ebiten.Image, x, y float64, c color.NRGBA) {
	gfx.StrokeCircle(dst, float32(x+6), float32(y+5), 4, 2, c, true)
	gfx.FillRoundRect(dst, float32(x), float32(y+5), 12, 9, 2, c)
}

// drawLink is a small chain of two links.
func drawLink(dst *ebiten.Image, x, y float64, c color.NRGBA) {
	gfx.StrokeRoundRect(dst, float32(x), float32(y+2), 10, 7, 3.5, 1.8, c)
	gfx.StrokeRoundRect(dst, float32(x+6), float32(y+2), 10, 7, 3.5, 1.8, c)
}

// drawPlate draws a read-only board: frosted, without an edge or a shadow.
func (g *Game) drawPlate(dst *ebiten.Image, r image.Rectangle) {
	tint := color.NRGBA{246, 249, 252, 255}
	if !g.materialSurface(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), tint, "plate") {
		gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 10, withAlpha(tint, 150))
	}
}

// drawButton draws a pressable control: a pill with a bright edge, its label
// centered and an accent ring on focus. Marks such as a chevron belong to the label.
func (s *authorScreen) drawButton(g *Game, dst *ebiten.Image, i int, focused bool) {
	e := s.entries[i]
	r, tint := g.controlMaterial(entryBounds(e))
	text := g.th.text
	tones := g.tonePalette()
	switch e.Tone {
	case "primary":
		tint, text = tones.primaryCard, tones.primaryText
	case "note":
		tint = tones.note
	}
	refused := s.unavailable(g, i)
	if refused {
		text = g.th.dim
	}
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	radius := float32(min(18, h/2))
	if !g.materialSurface(dst, x, y, w, h, tint, "control") || e.Tone == "primary" {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), radius, tint)
	}
	edge, width := g.th.border, float32(1.2)
	if focused {
		edge, width = g.th.accent, 2.5
		if e.Tone == "primary" {
			edge = tones.primaryText
		}
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), radius, width, edge)
	size := 17.0
	label := gfx.Ellipsis(e.Label, size, w-16)
	lw := gfx.TextWidth(label, size)
	ly := y + h/2 - gfx.LineHeight(size)/2 + 1
	if e.Text != "" && h >= 56 {
		ly = y + 8
	}
	g.text(dst, label, x+(w-lw)/2, ly, size, text)
	if e.Text != "" && h >= 56 {
		g.drawPlanningText(dst, e.Text, x+14, ly+gfx.LineHeight(size), w-28, 15, max(1, int((h-gfx.LineHeight(size)-14)/21)), g.th.dim)
	}
}

// drawTicket draws a ticket card: a colored stripe, the title and state, the
// dependency mark, a bar of the planned and reported fractions, and a stub with
// the holder's face and the due date.
func (s *authorScreen) drawTicket(g *Game, dst *ebiten.Image, i int, focused bool) {
	e := s.entries[i]
	t := e.Ticket
	env := s.rows[i].Env(s.env(g))
	planned := clamp01(exprNumber(t.Planned, env))
	progress := clamp01(exprNumber(t.Progress, env))
	behind, blocked := exprTruth(t.Behind, env), exprTruth(t.Blocked, env)
	stripe := g.th.accent
	switch {
	case progress >= 1:
		stripe = g.th.cursor
	case blocked:
		stripe = g.th.dim
	case behind:
		stripe = g.th.accent
	}
	r, _ := g.controlMaterial(entryBounds(e))
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	tint := g.th.window
	if !g.materialSurface(dst, x, y, w, h, tint, "card") {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 10, tint)
	}
	edge, width := g.th.border, float32(1)
	if focused {
		edge, width = g.th.accent, 2.5
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, width, edge)
	gfx.FillRoundRect(dst, float32(x+5), float32(y+10), 5, float32(h-20), 2.5, stripe)

	stub := 0.0
	if e.Icon != "" || t.Due != "" {
		stub = min(92, w*.3)
		sx := x + w - stub
		for yy := y + 10; yy < y+h-10; yy += 9 {
			rect(dst, sx, yy, 1, 5, withAlpha(g.th.dim, 120))
		}
		radius := max(10, min(28, (stub-20)/2, (h-54)/2))
		faceY := y + 12 + radius
		if e.Icon != "" {
			g.planningFace(dst, e.Icon, sx+stub/2, faceY, radius)
			if muted := exprTruth(e.Muted, env); muted {
				g.fillFace(dst, sx+stub/2, faceY, radius, color.NRGBA{166, 174, 181, 180})
			}
		}
		if t.Due != "" {
			dueTint := g.th.text
			if behind && progress < 1 {
				dueTint = g.th.accent
			}
			g.drawPlanningTextCentered(dst, t.Due, sx+stub/2, faceY+radius+6, stub-8, 15, dueTint)
		}
	}
	left := x + 20
	mw := w - stub - 30
	titleLines := 1
	if h >= 130 {
		titleLines = 2
	}
	g.drawPlanningText(dst, e.Label, left, y+10, mw, 17, titleLines, g.th.text)

	meta := y + h - 58
	cx := left
	if t.State != "" {
		size := 15.0
		room := mw * .5
		if len(t.After) == 0 {
			room = mw - 8
		}
		label := gfx.Ellipsis(t.State, size, room)
		cw := gfx.TextWidth(label, size) + 16
		gfx.FillRoundRect(dst, float32(cx), float32(meta), float32(cw), 22, 11, withAlpha(stripe, 44))
		g.text(dst, label, cx+8, meta+1, size, stripe)
		cx += cw + 8
	}
	if len(t.After) > 0 {
		c := g.th.accent
		if blocked {
			drawLock(dst, cx, meta+3, g.th.dim)
			c = g.th.dim
		} else {
			drawLink(dst, cx-2, meta+5, c)
		}
		cx += 20
		// A name may span lines on its own card; the mark reads it on one line.
		g.drawPlanningText(dst, strings.ReplaceAll(strings.Join(t.After, " / "), "\n", " "), cx, meta, max(1, left+mw-cx), 15, 1, c)
	}

	g.drawProgressBar(dst, left, y+h-26, mw-48, progress, planned, t.Planned.Set(), stripe)
}

// drawProgressBar is the progress bar of ticket cards and meters: a track, the
// fill, an optional planned mark and the percentage to the right.
func (g *Game) drawProgressBar(dst *ebiten.Image, left, by, bw, progress, planned float64, marked bool, fill color.NRGBA) {
	gfx.FillRoundRect(dst, float32(left), float32(by), float32(bw), 8, 4, g.th.shadow)
	if progress > 0 {
		gfx.FillRoundRect(dst, float32(left), float32(by), float32(max(8, bw*progress)), 8, 4, fill)
	}
	if marked {
		px := left + bw*planned
		gfx.StrokeLine(dst, float32(px), float32(by-5), float32(px), float32(by+13), 2, g.th.text, true)
		gfx.StrokeLine(dst, float32(px-4), float32(by-7), float32(px+4), float32(by-7), 2, g.th.text, true)
	}
	pct := fmt.Sprintf("%d%%", int(math.Round(progress*100)))
	g.text(dst, pct, left+bw+8, by-8, 15, fill)
}

// drawEntryMeter draws a bounded entry's meter along its bottom, right of left.
func (s *authorScreen) drawEntryMeter(g *Game, dst *ebiten.Image, i int, r image.Rectangle, left float64) {
	m := s.entries[i].Meter
	env := s.rows[i].Env(s.env(g))
	progress := clamp01(exprNumber(m.Progress, env))
	planned := clamp01(exprNumber(m.Planned, env))
	fill := g.th.accent
	switch {
	case progress >= 1:
		fill = g.th.cursor
	case exprTruth(m.Behind, env):
		fill = g.th.accent
	}
	g.drawProgressBar(dst, left, float64(r.Max.Y)-24, max(16, float64(r.Max.X)-left-60), progress, planned, m.Planned.Set(), fill)
}

// bustLayout is the left part of r a bust takes, and its crop fractions.
func bustLayout(r image.Rectangle, b *data.EntryBust) (area image.Rectangle, top, face float64) {
	width, top, face := b.Width, b.Top, b.Face
	if width == 0 {
		width = .4
	}
	if top == 0 {
		top = .12
	}
	if face == 0 {
		face = .3
	}
	if b.Height > 0 {
		// Across the top of a tall card, the label and text under it.
		h := int(float64(r.Dy()) * min(1, b.Height))
		return image.Rect(r.Min.X+6, r.Min.Y+6, r.Max.X-6, r.Min.Y+max(8, h)), top, face
	}
	w := int(float64(r.Dx()) * width)
	if b.Bare && width >= 1 {
		return r, top, face
	}
	return image.Rect(r.Min.X+6, r.Min.Y+6, r.Min.X+max(8, w), r.Max.Y-6), top, face
}

// drawEntryBust draws the head-to-chest crop of a bounded entry and veils it
// when muted. It returns the left edge for the entry's text.
func (s *authorScreen) drawEntryBust(g *Game, dst *ebiten.Image, i int, r image.Rectangle) int {
	e := s.entries[i]
	area, top, face := bustLayout(r, e.Bust)
	x, y, w, h := float64(area.Min.X), float64(area.Min.Y), float64(area.Dx()), float64(area.Dy())
	if !e.Bust.Bare {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 8, color.NRGBA{236, 241, 247, 200})
	}
	if e.Bust.Picture != "" && e.Bust.Fit == "contain" {
		g.drawPictureContain(dst, e.Bust.Picture, x, y, w, h, 1)
	} else if e.Bust.Picture != "" {
		g.planningBust(dst, e.Bust.Picture, x, y, w, h, top, face)
	}
	if muted, err := e.Muted.Bool(s.rows[i].Env(s.env(g))); e.Muted.Set() && err == nil && muted {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 8, color.NRGBA{150, 158, 168, 150})
	}
	if e.Bust.Height > 0 {
		return 10
	}
	return area.Max.X - r.Min.X + 12
}

// drawTicketLinks joins each visible ticket card to the visible cards it names in
// after. A neighbor on the same row gets a short arrow; others go over the top.
func (s *authorScreen) drawTicketLinks(g *Game, dst *ebiten.Image) {
	cards := map[string]image.Rectangle{}
	var order []int
	for i, e := range s.entries {
		if e.Ticket != nil && len(e.Bounds) == 4 {
			cards[e.Label] = entryBounds(e)
			order = append(order, i)
		}
	}
	lane := 0
	for _, i := range order {
		e := s.entries[i]
		to := entryBounds(e)
		for _, name := range e.Ticket.After {
			from, ok := cards[name]
			if !ok || from == to {
				continue
			}
			sameRow := from.Min.Y == to.Min.Y
			between := false
			for _, j := range order {
				o := entryBounds(s.entries[j])
				if o != from && o != to && o.Min.Y == to.Min.Y && o.Min.X > min(from.Max.X, to.Max.X)-1 && o.Max.X < max(from.Min.X, to.Min.X)+1 {
					between = true
				}
			}
			if sameRow && !between {
				y := float32(from.Min.Y + from.Dy()/2)
				x0, x1 := float32(from.Max.X), float32(to.Min.X)
				if from.Min.X > to.Min.X {
					x0, x1 = float32(from.Min.X), float32(to.Max.X)
				}
				gfx.StrokeLine(dst, x0, y, x1, y, 2, g.th.accent, true)
				g.arrowHead(dst, x1, y, x1-x0)
				continue
			}
			top := float32(min(from.Min.Y, to.Min.Y) - 6 - 5*lane)
			lane = (lane + 1) % 3
			ax, bx := float32(from.Min.X+from.Dx()/2), float32(to.Min.X+to.Dx()/2)
			gfx.StrokeLine(dst, ax, float32(from.Min.Y), ax, top, 2, g.th.accent, true)
			gfx.StrokeLine(dst, ax, top, bx, top, 2, g.th.accent, true)
			gfx.StrokeLine(dst, bx, top, bx, float32(to.Min.Y), 2, g.th.accent, true)
			gfx.StrokeLine(dst, bx-4, float32(to.Min.Y)-6, bx, float32(to.Min.Y), 2, g.th.accent, true)
			gfx.StrokeLine(dst, bx+4, float32(to.Min.Y)-6, bx, float32(to.Min.Y), 2, g.th.accent, true)
		}
	}
}

func (g *Game) arrowHead(dst *ebiten.Image, x, y, dir float32) {
	d := float32(6)
	if dir < 0 {
		d = -6
	}
	gfx.StrokeLine(dst, x-d, y-4, x, y, 2, g.th.accent, true)
	gfx.StrokeLine(dst, x-d, y+4, x, y, 2, g.th.accent, true)
}

// drawEntryDot draws a bounded entry's status dot: a small filled circle with
// a soft light ring in the top-right corner. An unreadable color draws nothing.
func drawEntryDot(dst *ebiten.Image, r image.Rectangle, hex string) {
	c, ok := parseDotColor(hex)
	if !ok || r.Dx() < 24 || r.Dy() < 24 {
		return
	}
	x, y := float32(r.Max.X-16), float32(r.Min.Y+16)
	gfx.FillCircle(dst, x, y, 8, color.NRGBA{255, 255, 255, 170}, true)
	gfx.FillCircle(dst, x, y, 6, c, true)
}

func parseDotColor(hex string) (color.NRGBA, bool) {
	var r, g, b uint8
	if len(hex) != 7 || hex[0] != '#' {
		return color.NRGBA{}, false
	}
	if _, err := fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b); err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{r, g, b, 235}, true
}

// drawEntrySign draws a bounded entry's small square sign in the top-right
// corner, left of the dot when there is one.
func (g *Game) drawEntrySign(dst *ebiten.Image, r image.Rectangle, path string, dot bool) {
	if path == "" || r.Dx() < 48 || r.Dy() < 32 {
		return
	}
	size := 26.0
	x := float64(r.Max.X) - size - 8
	if dot {
		x -= 22
	}
	g.drawPicture(dst, data.Picture{File: path}, x, float64(r.Min.Y)+4, size, size, 1)
}
