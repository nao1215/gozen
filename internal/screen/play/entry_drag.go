package play

import (
	"image"
	"image/color"
	"maps"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// Carrying and dropping on a full-screen surface. A drag entry is picked up
// (pressed and moved, or tapped, or confirmed with the keys) and carried to a
// drop entry, whose do runs with the carried value bound to arg.<drop>. A peek
// entry opens a small glass panel of text beside itself and runs nothing.

// pressSwallowed marks a press that closed a peek: its lift does nothing.
const pressSwallowed = -2

// dragSlop is how far (UI units) the pointer moves before a press is a drag.
const dragSlop = 6

// carried is the picked-up entry, kept across refreshes by its source entry
// and row arguments. ok and hints are the drop places' answers for its value.
type carried struct {
	entry int
	index int
	args  map[string]any
	value string
	ok    map[int]bool
	hints map[int]string
}

// pointerPress is a press on a part: on is the entry (-1 for empty space), at
// where it went down, grab the offset inside the card, and dragging is set
// once it moved past dragSlop from a carried entry.
type pointerPress struct {
	on       int
	at, grab image.Point
	dragging bool
}

// peekOpen is the entry whose peek panel is open.
type peekOpen struct {
	entry int
	index int
	args  map[string]any
}

// focusStop reports whether a bounded entry takes the focus: it acts, is
// carried, or opens a peek.
func focusStop(e data.ScreenEntry) bool {
	return len(e.Do) > 0 || e.Destination != "" || e.Drag != "" || e.Peek != ""
}

// pointerPart reports whether the entry is carried, dropped on or peeked:
// pointer presses on them are read by updatePointer, not as an ordinary click.
func pointerPart(e data.ScreenEntry) bool { return e.Drag != "" || e.Drop != "" || e.Peek != "" }

// surfaceActions are the focus stops of the bounded entries in authored order.
func (s *authorScreen) surfaceActions() []int {
	var out []int
	for i, e := range s.entries {
		if e.Target.ID == "" && len(e.Bounds) == 4 && focusStop(e) {
			out = append(out, i)
		}
	}
	return out
}

// partAt is the topmost carried, dropped or peeked entry under p, or -1.
func (s *authorScreen) partAt(p image.Point) int {
	for i, e := range slices.Backward(s.entries) {
		if e.Target.ID == "" && pointerPart(e) && p.In(entryBounds(e)) {
			return i
		}
	}
	return -1
}

// dropEnv binds the carried value for a drop place's guard, hint and action.
func (s *authorScreen) dropEnv(g *Game, i int, value string) expr.Env {
	args := expr.Map{}
	maps.Copy(args, s.rows[i].Args)
	args[s.entries[i].Drop] = value
	return expr.Chain{expr.Map{"arg": args}, s.env(g)}
}

// pickUp carries entry i and asks every drop place whether it takes the value.
func (s *authorScreen) pickUp(g *Game, i int) {
	e := s.entries[i]
	c := &carried{entry: i, index: s.indices[i], args: s.rows[i].Args, value: e.Drag, ok: map[int]bool{}, hints: map[int]string{}}
	for k, d := range s.entries {
		if d.Drop == "" || len(d.Bounds) != 4 {
			continue
		}
		env := s.dropEnv(g, k, c.value)
		c.ok[k] = (!d.Accepts.Set() || exprTruth(d.Accepts, env)) && !s.unavailable(g, k)
		if d.Hint != "" {
			c.hints[k] = data.Render(g.db.Tr(d.Hint), env)
		}
	}
	s.carry = c
	g.emit("screen_carry", traceLine{"screen": s.name, "entry": c.index, "source": c.value})
}

// putDown lets go of what is carried and closes a peek. It reports whether
// there was anything, so the cancel input closes the screen only when not.
func (s *authorScreen) putDown() bool {
	had := s.carry != nil || s.peek != nil || s.press != nil
	s.carry, s.peek, s.press = nil, nil, nil
	return had
}

// dropOn drops the carried value on entry i: a refused place runs nothing,
// an accepting one runs its do through the ordinary activation.
func (s *authorScreen) dropOn(g *Game, i int) bool {
	c := s.carry
	if c == nil || s.entries[i].Drop == "" {
		return false
	}
	if !c.ok[i] {
		g.emit("screen_refused", traceLine{"screen": s.name, "entry": s.indices[i], "source": c.value})
		return true
	}
	s.carry = nil
	g.emit("screen_drop", traceLine{"screen": s.name, "entry": s.indices[i], "source": c.value})
	s.list.sel = i
	s.activateWith(g, map[string]any{s.entries[i].Drop: c.value})
	return true
}

// togglePeek opens the peek of entry i, or closes it when it is open.
func (s *authorScreen) togglePeek(g *Game, i int) {
	if s.peek != nil && s.peek.entry == i {
		s.peek = nil
		g.emit("screen_peek", traceLine{"screen": s.name, "entry": s.indices[i], "open": false})
		return
	}
	s.peek = &peekOpen{entry: i, index: s.indices[i], args: s.rows[i].Args}
	g.emit("screen_peek", traceLine{"screen": s.name, "entry": s.indices[i], "open": true})
}

// confirmPart is the OK input on a focused part. It reports whether entry i
// acted: a carried entry is picked up or put down, a drop place takes what
// is carried, a peek opens or closes.
func (s *authorScreen) confirmPart(g *Game, i int) bool {
	e := s.entries[i]
	switch {
	case e.Drag != "":
		if s.carry != nil && s.carry.entry == i {
			s.carry = nil
		} else {
			s.pickUp(g, i)
		}
	case e.Drop != "":
		return s.dropOn(g, i)
	case e.Peek != "":
		s.togglePeek(g, i)
	default:
		return false
	}
	return true
}

// onControl reports whether p is over another control of the surface, so a
// press there keeps what is carried and reaches that control.
func (s *authorScreen) onControl(g *Game, p image.Point, cells []image.Rectangle) bool {
	in := p.In
	if slices.ContainsFunc(s.surfaceActions(), func(i int) bool { return in(entryBounds(s.entries[i])) }) ||
		slices.ContainsFunc(cells, in) || slices.ContainsFunc(s.tabBounds(g), in) {
		return true
	}
	band, _, _ := s.carouselLayout()
	return p.In(band)
}

// updatePointer reads presses on carried, dropped and peeked entries. A press
// moved past dragSlop from a carried entry drags its copy and drops where it
// lifts; a press lifted in place is a tap: it picks up, drops, opens a peek,
// or, on empty space, puts down. It reports whether it used the input.
func (s *authorScreen) updatePointer(g *Game, actions []int, cells []image.Rectangle) bool {
	in := &g.in
	used := false
	if in.inspect(in.pdown) {
		s.press = nil
		on := s.partAt(in.mouse)
		switch {
		case on >= 0:
			r := entryBounds(s.entries[on])
			s.press = &pointerPress{on: on, at: in.mouse, grab: in.mouse.Sub(r.Min)}
			for j, i := range actions {
				if i == on {
					s.focus = j
				}
			}
		case s.peek != nil:
			// A press outside an open peek closes it and does nothing else; its
			// lift (a finger's click) is swallowed too.
			s.peek = nil
			s.press = &pointerPress{on: pressSwallowed, at: in.mouse}
		case s.carry != nil && !s.onControl(g, in.mouse, cells):
			s.press = &pointerPress{on: -1, at: in.mouse}
		}
		used = s.press != nil
		in.query(used)
	}
	p := s.press
	if p == nil {
		return used
	}
	if !in.pup && in.pointerHeld() {
		d := in.mouse.Sub(p.at)
		if !p.dragging && p.on >= 0 && s.entries[p.on].Drag != "" && d.X*d.X+d.Y*d.Y >= dragSlop*dragSlop {
			p.dragging = true
			if s.carry == nil || s.carry.entry != p.on {
				s.pickUp(g, p.on)
			}
		}
		return true
	}
	if !in.pointerUp() {
		// The press ended unseen (the window lost it): forget it.
		s.press = nil
		return used
	}
	s.press = nil
	if p.dragging {
		target := s.partAt(in.mouse)
		if target >= 0 && s.entries[target].Drop != "" {
			s.dropOn(g, target)
		}
		s.carry = nil
		return true
	}
	// A tap belongs to the part where both press and release occurred. Moving
	// off a drop place or peek cancels it without activating the old target.
	if p.on >= 0 && s.partAt(in.mouse) != p.on {
		return true
	}
	switch e := p.on; {
	case e == pressSwallowed:
	case e < 0:
		s.carry = nil
	case s.entries[e].Drag != "" || s.entries[e].Drop != "" || s.entries[e].Peek != "":
		s.confirmPart(g, e)
	}
	return true
}

// keepCarry carries the picked-up entry, the press and the open peek over a
// refresh by source entry and row arguments; a source that is gone lets go.
func (s *authorScreen) keepCarry(g *Game, old *authorScreen) {
	find := func(index int, args map[string]any) int {
		for i, at := range s.indices {
			if at == index && maps.Equal(s.rows[i].Args, args) {
				return i
			}
		}
		return -1
	}
	if c := old.carry; c != nil {
		if i := find(c.index, c.args); i >= 0 && s.entries[i].Drag != "" {
			s.pickUpQuiet(g, i)
		}
	}
	if p := old.press; p != nil {
		moved := *p
		if p.on >= 0 {
			moved.on = find(old.indices[p.on], old.rows[p.on].Args)
		}
		if p.on < 0 || moved.on >= 0 {
			// An empty or swallowed press keeps its mark.
			s.press = &moved
		}
	}
	if k := old.peek; k != nil {
		if i := find(k.index, k.args); i >= 0 && s.entries[i].Peek != "" {
			s.peek = &peekOpen{entry: i, index: k.index, args: k.args}
		}
	}
}

// pickUpQuiet carries entry i again after a refresh without a trace line.
func (s *authorScreen) pickUpQuiet(g *Game, i int) {
	trace := g.tr
	g.tr = nil
	s.pickUp(g, i)
	g.tr = trace
}

// drawCarried marks the carried entry and every drop place while something is
// carried, draws the hints and the dragged copy under the pointer, and the
// open peek panel.
func (s *authorScreen) drawCarried(g *Game, dst *ebiten.Image) {
	if c := s.carry; c != nil {
		r := entryBounds(s.entries[c.entry]).Inset(-4)
		gfx.FillRoundRect(dst, float32(r.Min.X)+3, float32(r.Max.Y)-2, float32(r.Dx())-6, 6, 3, color.NRGBA{0, 0, 0, 40})
		gfx.StrokeRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 14, 3, g.th.accent)
		for i, e := range s.entries {
			if e.Drop == "" || len(e.Bounds) != 4 {
				continue
			}
			r := entryBounds(e)
			x, y, w, h := float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy())
			if c.ok[i] {
				edge := withAlpha(g.th.accent, 220)
				if g.in.mouse.In(r) {
					edge = g.th.accent
					gfx.FillRoundRect(dst, x, y, w, h, 12, withAlpha(g.th.accent, 28))
				}
				gfx.StrokeRoundRect(dst, x-2, y-2, w+4, h+4, 13, 2.5, edge)
			} else {
				gfx.FillRoundRect(dst, x, y, w, h, 12, color.NRGBA{226, 230, 236, 170})
				cx, cy := x+w-16, y+14
				gfx.StrokeLine(dst, cx-6, cy-6, cx+6, cy+6, 2.5, g.th.dim, true)
				gfx.StrokeLine(dst, cx+6, cy-6, cx-6, cy+6, 2.5, g.th.dim, true)
			}
			if hint := c.hints[i]; hint != "" {
				s.drawDropHint(g, dst, r, hint, c.ok[i])
			}
		}
		if p := s.press; p != nil && p.dragging {
			s.drawGhost(g, dst, c.entry, g.in.mouse.Sub(p.grab))
		}
	}
	if k := s.peek; k != nil && k.entry < len(s.entries) {
		s.drawPeek(g, dst, k.entry)
	}
}

// drawDropHint is a small glass chip on the lower edge of a drop place, or
// above its top when the place is too low to hold it.
func (s *authorScreen) drawDropHint(g *Game, dst *ebiten.Image, r image.Rectangle, hint string, ok bool) {
	const size = 14.0
	text := gfx.Ellipsis(hint, size, float64(r.Dx())+40)
	w := gfx.TextWidth(text, size) + 18
	h := gfx.LineHeight(size) + 6
	x := float64(r.Min.X+r.Max.X)/2 - w/2
	y := float64(r.Max.Y) - h - 4
	if r.Dy() < 64 {
		y = float64(r.Min.Y) - h - 4
	}
	sw, _ := g.screen()
	x = max(4, min(x, sw-w-4))
	// A plain frosted fill: the refracting glass would bend the card under it.
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), float32(h/2), color.NRGBA{250, 252, 255, 235})
	fg := g.lightPalette().text
	if !ok {
		fg = g.lightPalette().dim
	}
	g.text(dst, text, x+9, y+3, size, fg)
}

// drawGhost draws a translucent copy of entry i with its top-left at at.
func (s *authorScreen) drawGhost(g *Game, dst *ebiten.Image, i int, at image.Point) {
	r := entryBounds(s.entries[i])
	const pad = 8
	w, h := r.Dx()+2*pad, r.Dy()+2*pad
	if s.ghostImg == nil || s.ghostImg.Bounds().Dx() < w || s.ghostImg.Bounds().Dy() < h {
		if s.ghostImg != nil {
			s.ghostImg.Deallocate()
		}
		s.ghostImg = ebiten.NewImage(w, h)
	}
	img := s.ghostImg
	img.Clear()
	saved := s.entries[i].Bounds
	s.entries[i].Bounds = []int{pad, pad, r.Dx(), r.Dy()}
	gfx.FillRoundRect(img, pad, pad, float32(r.Dx()), float32(r.Dy()), 12, color.NRGBA{250, 252, 255, 235})
	switch e := s.entries[i]; {
	case e.Ticket != nil:
		s.drawTicket(g, img, i, false)
	default:
		s.drawPlainEntry(g, img, i, false)
	}
	s.entries[i].Bounds = saved
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(at.X-pad), float64(at.Y-pad))
	op.ColorScale.ScaleAlpha(.78)
	if part, ok := img.SubImage(image.Rect(0, 0, w, h)).(*ebiten.Image); ok {
		dst.DrawImage(part, op)
	}
}

// peekRect places the peek panel of entry i beside it, inside the screen, with
// its lines and the size they were wrapped at (smaller when long).
func (s *authorScreen) peekRect(g *Game, i int) (image.Rectangle, []string, float64) {
	r := entryBounds(s.entries[i])
	sw, sh := g.screen()
	width := min(360.0, sw-32)
	lines, size := gfx.FitLines(s.entries[i].Peek, 16, width-28, 10)
	h := float64(len(lines))*gfx.LineHeight(size) + 24
	x := float64(r.Max.X) + 10
	if x+width > sw-12 {
		x = float64(r.Min.X) - width - 10
	}
	if x < 12 {
		x = max(12, min(float64(r.Min.X), sw-width-12))
	}
	y := max(12, min(float64(r.Min.Y), sh-h-12))
	return image.Rect(int(x), int(y), int(x+width), int(y+h)), lines, size
}

// drawPeek draws an open peek: frosted glass without an edge, only text.
func (s *authorScreen) drawPeek(g *Game, dst *ebiten.Image, i int) {
	r, lines, size := s.peekRect(g, i)
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	gfx.FillRoundRect(dst, float32(x)+2, float32(y)+4, float32(w), float32(h), 12, color.NRGBA{0, 0, 0, 28})
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, color.NRGBA{248, 250, 253, 246})
	for n, line := range lines {
		g.text(dst, line, x+14, y+12+float64(n)*gfx.LineHeight(size), size, g.lightPalette().text)
	}
}
