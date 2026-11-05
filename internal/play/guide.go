package play

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// Screen guides (ADR 0055): the first time a full-screen surface with a guide
// shows, its lines appear one by one in the notification corner, spoken by
// an actor. A line with focus lights the entries of those marks (or the flow's
// next action), veils the rest of the screen and lets only the lit places be
// used; it goes on when one of them runs its action, or when until holds. A
// line without focus is pressed (anywhere, or OK) to go on. Cancel, save and
// load wait while a guide shows. The finished guide is recorded with the
// gallery's unlocks and never shows again, in this play or a later one.
//
// Scripted, replayed and automatic plays start as a player who has seen every
// guide; the script command guides on shows them as to a new player.

// guideRun is the guide showing now: its screen and the line it is on.
type guideRun struct {
	screen string
	step   int
}

// guidesOn reports whether guides show in this play.
func (g *Game) guidesOn() bool {
	if g.replaying || g.db == nil {
		return false
	}
	if g.testRun() {
		return g.guideScripted
	}
	return true
}

// guideDef is the screen's guide when it still has to show.
func (g *Game) guideDef(name string) *data.ScreenGuide {
	def := g.db.Game.Screens[name]
	if def == nil || def.Guide == nil || len(def.Guide.Steps) == 0 || g.unlocked[data.GuideKey(name)] {
		return nil
	}
	return def.Guide
}

// guideOn reports whether the guide is showing over screen s now.
func (g *Game) guideOn(s *authorScreen) bool {
	return g.guide != nil && g.guide.screen == s.name && !s.running && s.popup == nil && !g.talk.active
}

// guideUnder reports whether screen name is open under the top of the stack.
func (g *Game) guideUnder(name string) bool {
	for _, v := range g.stack[:max(0, len(g.stack)-1)] {
		if s, ok := v.(*authorScreen); ok && s.name == name {
			return true
		}
	}
	return false
}

// finishGuide records the showing guide as seen.
func (g *Game) finishGuide() {
	if g.guide == nil {
		return
	}
	name := g.guide.screen
	g.guide = nil
	g.unlock(data.GuideKey(name))
	g.emit("guide_done", traceLine{"screen": name})
}

// guideStep is the line showing on s, or nil.
func (g *Game) guideStep(s *authorScreen) *data.GuideStep {
	if g.guide == nil || g.guide.screen != s.name {
		return nil
	}
	gd := g.db.Game.Screens[s.name].Guide
	if gd == nil || g.guide.step >= len(gd.Steps) {
		return nil
	}
	return &gd.Steps[g.guide.step]
}

// showGuideStep writes the line to the trace and puts the keyboard focus on
// its first lit place (a carousel member turns to the middle).
func (g *Game) showGuideStep(s *authorScreen) {
	st := g.guideStep(s)
	if st == nil {
		return
	}
	g.emit("guide", traceLine{"screen": s.name, "step": g.guide.step + 1, "who": st.Who, "text": data.Render(g.db.Tr(st.Text), s.env(g)), "focus": st.Focus})
	if stops := s.guideStops(g, st); len(stops) > 0 {
		s.focus = stops[0]
		actions := s.surfaceActions()
		if stops[0] < len(actions) {
			s.syncRing(actions[stops[0]])
		}
	}
}

// advanceGuide moves to the next line, finishing the guide after the last.
func (g *Game) advanceGuide(s *authorScreen) {
	g.guide.step++
	if g.guide.step >= len(g.db.Game.Screens[s.name].Guide.Steps) {
		g.finishGuide()
		return
	}
	g.showGuideStep(s)
}

// guideRects are the places the line lights: the bounds of the visible,
// usable entries of its marks and the flow's next action.
func (s *authorScreen) guideRects(g *Game, st *data.GuideStep) []image.Rectangle {
	var out []image.Rectangle
	for i, e := range s.entries {
		if e.Mark != "" && slices.Contains(st.Focus, e.Mark) && !s.unavailable(g, i) {
			if r := entryBounds(e); !slices.Contains(out, r) {
				out = append(out, r)
			}
		}
	}
	if slices.Contains(st.Focus, data.GuideNext) {
		for _, f := range s.flowStops(g) {
			if f.next {
				out = append(out, f.r)
			}
		}
	}
	return out
}

// guideStops are the keyboard focus stops of the lit places, in ring order.
func (s *authorScreen) guideStops(g *Game, st *data.GuideStep) []int {
	actions := s.surfaceActions()
	ids, _ := s.surfaceTargets(g)
	var out []int
	for j, i := range actions {
		if e := s.entries[i]; e.Mark != "" && slices.Contains(st.Focus, e.Mark) && !s.unavailable(g, i) {
			out = append(out, j)
		}
	}
	if slices.Contains(st.Focus, data.GuideNext) {
		for k, f := range s.flowStops(g) {
			if f.next {
				out = append(out, len(actions)+len(ids)+k)
			}
		}
	}
	return out
}

// guideCarousel reports whether a lit place is a carousel member, whose
// left and right turn it.
func (s *authorScreen) guideCarousel(st *data.GuideStep) bool {
	for _, e := range s.entries {
		if e.Carousel != "" && e.Mark != "" && slices.Contains(st.Focus, e.Mark) {
			return true
		}
	}
	return false
}

// blockInput drops this tick's presses, clicks and wheel.
func (in *input) blockInput() {
	in.pressed = [numButtons]bool{}
	in.click, in.rclick, in.pdown, in.wheel, in.longPress = false, false, false, 0, false
}

// updateGuide starts the screen's guide the first time it can show and
// filters the input while a line shows. It reports whether it used this
// tick, so the screen does not update.
func (g *Game) updateGuide(s *authorScreen) bool {
	if g.guide != nil && g.guide.screen != s.name {
		if g.guideUnder(g.guide.screen) && (g.guideDef(s.name) == nil || !g.guidesOn()) {
			return false // a screen opened over the guided one is free
		}
		g.finishGuide() // left, or a screen over it begins its own guide
	}
	if s.running || s.popup != nil || s.peek != nil || g.talk.active {
		return false
	}
	if g.guide == nil {
		gd := g.guideDef(s.name)
		if gd == nil || !g.guidesOn() || gd.When.Set() && !exprTruth(gd.When, s.env(g)) {
			return false
		}
		g.guide = &guideRun{screen: s.name}
		g.showGuideStep(s)
		g.in.blockInput() // the press that opened the screen does not answer the first line
		return true
	}
	st := g.guideStep(s)
	if st == nil {
		g.finishGuide()
		return false
	}
	if st.Until.Set() && exprTruth(st.Until, s.env(g)) {
		g.advanceGuide(s)
		g.in.blockInput()
		return true
	}
	rects := s.guideRects(g, st)
	if len(rects) == 0 {
		if g.in.ok() || g.in.click {
			g.advanceGuide(s)
		}
		g.in.blockInput()
		return true
	}
	in := &g.in
	lit := false
	for _, r := range rects {
		lit = lit || in.mouse.In(r)
	}
	blocked := false
	if (in.click || in.pdown || in.rclick || in.wheel != 0) && !lit {
		in.click, in.pdown, in.wheel = false, false, 0
		blocked = true
	}
	stops := s.guideStops(g, st)
	if in.ok() && !slices.Contains(stops, s.focus) {
		if len(stops) > 0 {
			s.focus, s.keyed = stops[0], true
		}
		in.pressed[btnOK] = false
		blocked = true
	}
	keep := [numButtons]bool{btnOK: in.pressed[btnOK]}
	if s.guideCarousel(st) {
		keep[btnLeft], keep[btnRight] = in.pressed[btnLeft], in.pressed[btnRight]
	}
	if len(stops) > 1 {
		keep[btnUp], keep[btnDown] = in.pressed[btnUp], in.pressed[btnDown]
	}
	for b := range in.pressed {
		if in.pressed[b] && !keep[b] {
			blocked = true
		}
	}
	in.pressed, in.rclick, in.longPress = keep, false, false
	if blocked {
		g.emit("guide_blocked", traceLine{"screen": s.name, "step": g.guide.step + 1})
	}
	return false
}

// afterGuidedUpdate goes on from a line with focus and no until when the
// screen ran a lit place's action this tick (it is running, or became another
// screen). Only the lit places could be used.
func (g *Game) afterGuidedUpdate(s *authorScreen, name string, running bool) {
	if g.guide == nil || g.guide.screen != name || running {
		return
	}
	if !s.running && s.name == name {
		return
	}
	gd := g.db.Game.Screens[name].Guide
	if gd == nil || g.guide.step >= len(gd.Steps) {
		return
	}
	st := &gd.Steps[g.guide.step]
	if len(st.Focus) == 0 || st.Until.Set() {
		return
	}
	g.guide.step++
	if g.guide.step >= len(gd.Steps) {
		g.finishGuide()
		return
	}
	if s.name == name {
		g.showGuideStep(s)
	}
}

// guideSurface is the guided screen on top, while its line shows.
func (g *Game) guideSurface() *authorScreen {
	if g.guide == nil || len(g.stack) == 0 {
		return nil
	}
	s, ok := g.stack[len(g.stack)-1].(*authorScreen)
	if !ok || !g.guideOn(s) || s.peek != nil {
		return nil
	}
	return s
}

// guideToastRect places the line in the notification corner, or in the
// corner that covers the least of the lit places.
func (g *Game) guideToastRect(rects []image.Rectangle, h int) image.Rectangle {
	sw, sh := g.screen()
	w := 452
	at := []image.Point{{int(sw) - w - 16, 104}, {int(sw) - w - 16, int(sh) - 16 - h}, {16, 104}, {16, int(sh) - 16 - h}}
	best, least := image.Rectangle{}, -1
	for _, p := range at {
		r := image.Rect(p.X, p.Y, p.X+w, p.Y+h)
		covered := 0
		for _, f := range rects {
			o := r.Intersect(f.Inset(-6))
			covered += o.Dx() * o.Dy()
		}
		if least < 0 || covered < least {
			best, least = r, covered
		}
		if covered == 0 {
			break
		}
	}
	return best
}

// drawGuide draws the veil, the rings around the lit places and the line.
func (g *Game) drawGuide(dst *ebiten.Image) {
	s := g.guideSurface()
	if s == nil {
		return
	}
	st := g.guideStep(s)
	if st == nil {
		return
	}
	rects := s.guideRects(g, st)
	pulse := .5 + .5*math.Sin(float64(g.tick)/9)
	accent := g.lightPalette().accent
	if len(rects) > 0 {
		b := dst.Bounds()
		if g.guideVeil == nil || g.guideVeil.Bounds().Size() != b.Size() {
			if g.guideVeil != nil {
				g.guideVeil.Deallocate()
			}
			g.guideVeil = ebiten.NewImage(b.Dx(), b.Dy())
		}
		g.guideVeil.Fill(color.NRGBA{14, 20, 32, 120})
		k := gfx.Scale()
		for _, r := range rects {
			hole := image.Rect(int(float64(r.Min.X-4)*k), int(float64(r.Min.Y-4)*k), int(math.Ceil(float64(r.Max.X+4)*k)), int(math.Ceil(float64(r.Max.Y+4)*k)))
			if cut, ok := g.guideVeil.SubImage(hole.Add(b.Min)).(*ebiten.Image); ok {
				cut.Clear()
			}
		}
		dst.DrawImage(g.guideVeil, nil)
		for _, r := range rects {
			x, y, w, h := float32(r.Min.X-4), float32(r.Min.Y-4), float32(r.Dx()+8), float32(r.Dy()+8)
			glow := withAlpha(accent, uint8(70+80*pulse))
			gfx.StrokeRoundRect(dst, x-3, y-3, w+6, h+6, 16, 5, glow)
			gfx.StrokeRoundRect(dst, x, y, w, h, 14, 2.5, withAlpha(color.NRGBA{255, 255, 255, 255}, uint8(180+60*pulse)))
		}
	}
	text := data.Render(g.db.Tr(st.Text), s.env(g))
	const size, textW = 17.0, 452 - 92
	lines := min(5, max(1, len(gfx.Wrap(text, size, textW))))
	h := 44 + int(float64(lines)*gfx.LineHeight(size)) + 14
	r := g.guideToastRect(rects, h)
	x, y, w, hh := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	light := g.lightPalette()
	if !g.materialSurface(dst, x, y, w, hh, color.NRGBA{250, 252, 254, 246}, "card") {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(hh), 14, color.NRGBA{250, 252, 254, 246})
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(hh), 14, 1.5, withAlpha(accent, 200))
	path, name := g.actorFacePath(st.Who)
	previous := g.faceSquare
	g.faceSquare = true
	g.planningFace(dst, path, x+14+28, y+14+28, 28)
	g.faceSquare = previous
	left := x + 84
	g.drawPlanningText(dst, name, left, y+10, w-(left-x)-16, 15, 1, light.accent)
	g.drawPlanningText(dst, text, left, y+34, textW, size, lines, light.text)
	if len(rects) == 0 {
		// A line pressed to go on: a small mark that breathes at the corner.
		mx, my := float32(x+w-22), float32(y+hh-16+2*pulse)
		gfx.StrokeLine(dst, mx-6, my-4, mx, my+2, 2.2, light.accent, true)
		gfx.StrokeLine(dst, mx, my+2, mx+6, my-4, 2.2, light.accent, true)
	}
}

func (g *Game) releaseGuideVeil() {
	if g.guideVeil != nil {
		g.guideVeil.Deallocate()
		g.guideVeil = nil
	}
}
