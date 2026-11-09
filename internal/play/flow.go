package play

import (
	"fmt"
	"image"
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/screen/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// Flows: the screens of a flow are gone through in order. Each step but the
// last has a next action at the bottom right that finishes it and replaces the
// screen with the next step, as a tab does; a step band at the top shows the
// steps, finished ones with a check (pressed to go back), the ones not reached
// faded and locked. Opening a step not reached yet opens the current step.

// flowNextSize is the next action's size; it sits 16 from the right and 24
// from the bottom, left of the tabs when there are tabs.
const flowNextW, flowNextH = 176, 48

// flowKey is the flow's restart value now.
func (g *Game) flowKey(f *data.Flow) string {
	if !f.Restart.Set() {
		return ""
	}
	v, err := f.Restart.Eval(g.st.Env())
	if err != nil {
		return ""
	}
	return fmt.Sprint(v)
}

// flowProgress is flow name's progress under the current restart value:
// progress made under another value counts as none.
func (g *Game) flowProgress(name string) rpg.FlowProgress {
	f := g.db.Game.Flows[name]
	if f == nil {
		return rpg.FlowProgress{}
	}
	key := g.flowKey(f)
	p, ok := g.st.Flows[name]
	if !ok || p.Key != key {
		// No progress under this value yet: start from the first step, still
		// passing over steps already finished by done or left out by when.
		p = rpg.FlowProgress{Key: key}
	}
	p.Reached = max(0, min(p.Reached, len(f.Steps)-1))
	// A step whose done condition holds is finished by itself, as if its next
	// action were pressed (a choice made on a screen outside the flow).
	// A step left out by its when is passed over the same way.
	for p.Reached < len(f.Steps)-1 && (g.flowLeftOut(f.Steps[p.Reached]) || f.Steps[p.Reached].Done.Set() && exprTruth(f.Steps[p.Reached].Done, g.st.Env())) {
		p.Reached++
	}
	p.Entered = slices.Clone(p.Entered)
	return p
}

// flowLeftOut reports whether a step's when leaves it out of the flow now.
func (g *Game) flowLeftOut(st data.FlowStep) bool {
	return st.When.Set() && !exprTruth(st.When, g.st.Env())
}

// setFlow keeps a flow's progress in the saved state.
func (g *Game) setFlow(name string, p rpg.FlowProgress) {
	if g.st.Flows == nil {
		g.st.Flows = map[string]rpg.FlowProgress{}
	}
	g.st.Flows[name] = p
}

// flowReached is the first step of flow name not finished.
func (g *Game) flowReached(name string) int { return g.flowProgress(name).Reached }

// flowSkip reports whether the flow lets every step open now.
func (g *Game) flowSkip(f *data.Flow) bool { return exprTruth(f.Skip, g.st.Env()) }

// flowHeld is the last held step of the flow finished under the current
// restart value, or -1: it and the steps before it do not open again.
func flowHeld(f *data.Flow, reached int) int {
	for k := reached - 1; k >= 0; k-- {
		if f.Steps[k].Hold {
			return k
		}
	}
	return -1
}

// flowOpen is the screen to open for name: a step not reached yet, or one at
// or before a finished held step, becomes the current step unless the flow
// lets every step open. Navigation commits automatic progress here; Draw only
// reads flowProgress and never mutates saved state.
func (g *Game) flowOpen(name string) string {
	flow, step := g.db.FlowOf(name)
	if flow == "" {
		return name
	}
	f := g.db.Game.Flows[flow]
	g.setFlow(flow, g.flowProgress(flow))
	if reached := g.flowReached(flow); (step > reached || step <= flowHeld(f, reached) || g.flowLeftOut(f.Steps[step])) && !g.flowSkip(f) {
		return f.Steps[reached].Screen
	}
	return name
}

// flowGivesWay reports whether this step screen would no longer open as
// itself (a held step finished, a new restart value): after an action it is
// replaced by the step that opens.
func (s *authorScreen) flowGivesWay(g *Game) bool {
	if len(s.args) > 0 {
		return false
	}
	flow, _ := g.db.FlowOf(s.name)
	return flow != "" && g.flowOpen(s.name) != s.name
}

// flowFinish marks step of flow finished under the current restart value.
func (g *Game) flowFinish(flow string, step int) {
	f := g.db.Game.Flows[flow]
	p := g.flowProgress(flow)
	p.Reached = max(p.Reached, min(step+1, len(f.Steps)-1))
	g.setFlow(flow, p)
	g.emit("flow_next", traceLine{"flow": flow, "step": step + 1})
}

// enterFlow runs the enter event of this step over the screen the first time
// it shows under the flow's restart value, as an entry's action runs. It
// reports whether it began one.
func (s *authorScreen) enterFlow(g *Game) bool {
	flow, step := g.db.FlowOf(s.name)
	if flow == "" {
		return false
	}
	enter := g.db.Game.Flows[flow].Steps[step].Enter
	p := g.flowProgress(flow)
	g.setFlow(flow, p)
	if enter == "" || g.db.Events[enter] == nil || slices.Contains(p.Entered, step) {
		return false
	}
	p.Entered = append(p.Entered, step)
	g.setFlow(flow, p)
	g.emit("flow_enter", traceLine{"flow": flow, "step": step + 1, "event": enter})
	s.destination, s.homed = "", false
	s.startEvent(g, enter, nil)
	return true
}

// flowStop is a control of the step band or the next action.
type flowStop struct {
	label   string
	step    int // the step it goes to
	next    bool
	enabled bool
	current bool
	done    bool
	r       image.Rectangle
}

// flowStops lays out the step band and the next action of this screen.
func (s *authorScreen) flowStops(g *Game) []flowStop {
	flow, step := g.db.FlowOf(s.name)
	if flow == "" {
		return nil
	}
	f := g.db.Game.Flows[flow]
	reached, skip := g.flowReached(flow), g.flowSkip(f)
	held := flowHeld(f, reached)
	sw, sh := g.screen()
	window := g.db.Game.Screens[s.name].View.Window != ""
	y, h, right := 10, 32, int(sw)-24
	if window {
		y, h, right = 62, 32, int(sw)-44
	}
	const size = 15.0
	var stops []flowStop
	x := right
	shown := 0
	for _, def := range f.Steps {
		if !g.flowLeftOut(def) {
			shown++
		}
	}
	for k, def := range slices.Backward(f.Steps) {
		if g.flowLeftOut(def) && k != step {
			continue
		}
		label := fmt.Sprintf("%d %s", shown, data.Render(g.db.Tr(def.Label), s.env(g)))
		shown--
		w := int(gfx.TextWidth(label, size)) + 40
		x -= w
		stops = append(stops, flowStop{
			label: label, step: k, current: k == step, done: k < reached,
			enabled: k != step && ((k <= reached && k > held) || skip),
			r:       image.Rect(x, y, x+w, y+h),
		})
		x -= 18
	}
	for i, j := 0, len(stops)-1; i < j; i, j = i+1, j-1 {
		stops[i], stops[j] = stops[j], stops[i]
	}
	if step < len(f.Steps)-1 && !f.Steps[step].Hold {
		label := lang.T("次へ ›")
		if f.Next != "" {
			label = data.Render(g.db.Tr(f.Next), s.env(g))
		}
		nx := int(sw) - 16 - flowNextW
		if tabs := s.tabBounds(g); len(tabs) > 0 {
			nx = tabs[0].Min.X - 12 - flowNextW
		}
		ny := int(sh) - 24 - flowNextH
		if window {
			nx, ny = nx-16, ny-8
		}
		stops = append(stops, flowStop{label: label, step: step + 1, next: true, enabled: true, r: image.Rect(nx, ny, nx+flowNextW, ny+flowNextH)})
	}
	return stops
}

// flowBandLeft is where the step band begins, so the header text stops there.
func (s *authorScreen) flowBandLeft(g *Game) (float64, bool) {
	for _, st := range s.flowStops(g) {
		if !st.next {
			return float64(st.r.Min.X), true
		}
	}
	return 0, false
}

// goFlow runs a flow stop: the next action finishes this step; either one
// replaces the screen with the step it goes to, keeping the opener.
func (s *authorScreen) goFlow(g *Game, st flowStop) bool {
	if !st.enabled {
		return false
	}
	flow, step := g.db.FlowOf(s.name)
	if st.next {
		g.flowFinish(flow, step)
	}
	*s = *newAuthorScreen(g, g.db.Game.Flows[flow].Steps[st.step].Screen, s.done, nil)
	return true
}

// updateFlowClick runs the flow stop under a click. It reports whether one was.
func (s *authorScreen) updateFlowClick(g *Game) bool {
	if !g.in.inspect(g.in.click) {
		return false
	}
	for _, st := range s.flowStops(g) {
		if g.in.mouse.In(st.r) {
			g.in.query(s.goFlow(g, st))
			return true
		}
	}
	return false
}

// drawFlow draws the step band and the next action. focus is the focused
// flow stop, or -1.
func (s *authorScreen) drawFlow(g *Game, dst *ebiten.Image, focus int) {
	light := g.lightPalette()
	for i, st := range s.flowStops(g) {
		x, y, w, h := float64(st.r.Min.X), float64(st.r.Min.Y), float64(st.r.Dx()), float64(st.r.Dy())
		focused := i == focus && s.keyed
		if st.next {
			tones := g.tonePalette()
			r, _ := g.controlMaterial(st.r)
			x, y = float64(r.Min.X), float64(r.Min.Y)
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), float32(h/2), tones.primaryCard)
			edge, width := color.NRGBA{255, 255, 255, 230}, float32(1.2)
			if focused {
				edge, width = tones.primaryText, 2.5
			}
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), float32(h/2), width, edge)
			label := gfx.Ellipsis(st.label, 18, w-20)
			g.text(dst, label, x+(w-gfx.TextWidth(label, 18))/2, y+h/2-gfx.LineHeight(18)/2+1, 18, tones.primaryText)
			continue
		}
		if i > 0 {
			g.text(dst, "›", x-14, y+6, 15, light.dim)
		}
		fg, tint := light.text, color.NRGBA{250, 252, 255, 230}
		switch {
		case st.current:
			fg, tint = color.NRGBA{255, 255, 255, 255}, light.accent
		case !st.enabled && !st.done:
			fg, tint = withAlpha(light.dim, 150), color.NRGBA{236, 239, 243, 170}
		}
		if g.in.mouse.In(st.r) && st.enabled {
			tint = color.NRGBA{255, 255, 255, 255}
		}
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), float32(h/2), tint)
		if focused {
			gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), float32(h/2), 2.5, light.accent)
		}
		g.text(dst, st.label, x+14, y+h/2-gfx.LineHeight(15)/2, 15, fg)
		mx := x + w - 20
		switch {
		case st.done && !st.current:
			gfx.StrokeLine(dst, float32(mx), float32(y+h/2), float32(mx+4), float32(y+h/2+4), 2.2, g.th.cursor, true)
			gfx.StrokeLine(dst, float32(mx+4), float32(y+h/2+4), float32(mx+11), float32(y+h/2-5), 2.2, g.th.cursor, true)
		case !st.enabled && !st.current:
			drawLock(dst, mx, y+h/2-8, withAlpha(g.th.dim, 170))
		}
	}
}
