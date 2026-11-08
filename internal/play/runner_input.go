package play

import (
	"errors"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer3/casual"
)

const (
	runnerPointerLimit = 4
	runnerMouseID      = -2147483648
)

// runnerPointerSample is separate from the menu's followed finger. Coordinates
// and Scale are logical UI units, including each phone viewport transform.
type runnerPointerSample struct {
	ID int     `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type runnerGesture struct {
	id                                     int
	x, y, anchorX, anchorY, startX, startY float64
	age, began                             int
	moved, button                          bool
}

type runnerControls struct {
	pointers      []runnerGesture
	tick, lastTap int
	lastX, lastY  float64
	pause         bool
	left, right   bool
	lastSide      float64
}

func (c *runnerControls) reset() { *c = runnerControls{} }

func (g *Game) captureRunnerInput() {
	g.in.runnerCapture = g.runnerOwnsInput()
	if p := g.phone; p != nil && p.child != nil && p.child.game.runnerOwnsInput() {
		g.in.runnerCapture = true
	}
}

// quietRunnerMouse rejects browser mouse events synthesized from a touch. The
// ordinary followed-finger adapter runs afterwards and can still produce taps.
func (in *input) quietRunnerMouse(touching bool) {
	if !in.runnerCapture {
		return
	}
	if touching {
		in.runnerMouseQuiet = 60
	} else if in.runnerMouseQuiet > 0 {
		in.runnerMouseQuiet--
	}
	if in.runnerMouseQuiet > 0 {
		in.click, in.rclick, in.pdown, in.pheld, in.pup = false, false, false, false, false
	}
}

func (g *Game) runnerOwnsInput() bool {
	if len(g.stack) == 0 {
		return false
	}
	_, ok := g.stack[len(g.stack)-1].(*runnerScene)
	return ok
}

func (in *input) readRunnerPointers() {
	in.runnerPointers = in.runnerPointers[:0]
	if !in.runnerCapture {
		return
	}
	if len(in.touches) > runnerPointerLimit {
		in.runnerCancel = true
		return
	}
	scale := in.ui
	if scale <= 0 {
		scale = 1
	}
	for _, id := range in.touches {
		x, y := ebiten.TouchPosition(id)
		in.runnerPointers = append(in.runnerPointers, runnerPointerSample{ID: int(id), X: float64(x) / scale, Y: float64(y) / scale})
	}
	if in.runnerMouseQuiet == 0 && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		in.runnerPointers = append(in.runnerPointers, runnerPointerSample{ID: runnerMouseID, X: float64(in.mouse.X), Y: float64(in.mouse.Y)})
	}
}

func (c *runnerControls) update(g *Game) casual.Input {
	in := &g.in
	sw, sh := g.screen()
	return c.advance(in, sw, sh)
}

// advance is device-independent so tests and replay use the same gesture rules.
// Missing captured pointers are releases; explicit cancellation never taps.
func (c *runnerControls) advance(in *input, width, height float64) casual.Input {
	if in.runnerCancel {
		in.query(in.pdown || in.pheld || in.pup || len(in.runnerPointers) > 0)
		c.reset()
		return casual.Input{}
	}
	out := casual.Input{Boost: in.heldKey(btnUp), Action: in.pressedKey(btnOK)}
	left, right := in.heldKey(btnLeft), in.heldKey(btnRight)
	c.tick++
	c.pause = false
	scale := in.runnerScale
	if !finiteRunnerValue(scale) || scale <= 0 {
		scale = 1
	}
	now := in.runnerPointers
	// Scripted pointer input remains usable without physical polling.
	if len(now) == 0 && in.pheld && !in.runnerCapture {
		now = []runnerPointerSample{{ID: runnerMouseID, X: float64(in.mouse.X), Y: float64(in.mouse.Y)}}
	}
	if len(now) > runnerPointerLimit {
		c.reset()
		return casual.Input{}
	}
	for _, p := range now {
		if !finiteRunnerValue(p.X) || !finiteRunnerValue(p.Y) || p.X < 0 || p.X >= width || p.Y < 0 || p.Y >= height {
			c.reset()
			return casual.Input{}
		}
	}
	in.query(in.pdown || in.pheld || in.pup || in.moved || in.click)
	out.Action = c.release(now, width, scale) || out.Action
	for _, sample := range now {
		i := slices.IndexFunc(c.pointers, func(p runnerGesture) bool { return p.id == sample.ID })
		if i < 0 {
			c.pointers = append(c.pointers, runnerGesture{id: sample.ID, anchorX: sample.X, anchorY: sample.Y, startX: sample.X, startY: sample.Y, began: c.tick, button: sample.X >= width-64 && sample.Y < 44})
			i = len(c.pointers) - 1
		}
		p := &c.pointers[i]
		p.x, p.y = sample.X, sample.Y
		p.age++
		p.moved = p.moved || math.Hypot(p.x-p.startX, p.y-p.startY) > 10*scale
	}
	for i := range c.pointers {
		p := &c.pointers[i]
		if p.button {
			continue
		}
		p.anchorX = max(p.x-48*scale, min(p.x+48*scale, p.anchorX))
		p.anchorY = max(p.y-48*scale, min(p.y+48*scale, p.anchorY))
		dx, dy := p.x-p.anchorX, p.y-p.anchorY
		if math.Abs(dx) >= math.Abs(dy) && math.Abs(dx) >= 12*scale {
			left, right = left || dx < 0, right || dx > 0
		} else if math.Abs(dy) > math.Abs(dx) && dy <= -12*scale {
			out.Boost = true
		}
		break
	}
	out.Steer = c.side(left, right)
	return out
}

func (c *runnerControls) side(left, right bool) float64 {
	pressedLeft, pressedRight := left && !c.left, right && !c.right
	if pressedLeft != pressedRight {
		if pressedLeft {
			c.lastSide = -1
		} else {
			c.lastSide = 1
		}
	}
	c.left, c.right = left, right
	if left && right {
		return c.lastSide
	}
	if left {
		return -1
	}
	if right {
		return 1
	}
	return 0
}

func (c *runnerControls) release(now []runnerPointerSample, width, scale float64) bool {
	action := false
	for i := 0; i < len(c.pointers); {
		p := c.pointers[i]
		if slices.ContainsFunc(now, func(n runnerPointerSample) bool { return n.ID == p.id }) {
			i++
			continue
		}
		if p.button {
			c.pause = c.pause || !p.moved || p.x >= width-64 && p.y < 44
		} else if !p.moved && p.age <= 24 {
			if c.lastTap > 0 && p.began-c.lastTap <= 18 && math.Hypot(p.x-c.lastX, p.y-c.lastY) <= 100*scale {
				action, c.lastTap = true, 0
			} else {
				c.lastTap, c.lastX, c.lastY = c.tick, p.x, p.y
			}
		}
		c.pointers = slices.Delete(c.pointers, i, i+1)
	}
	return action
}

func finiteRunnerValue(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// transformRunnerPointers carries every pointer through the phone's viewport.
// One out-of-bounds captured pointer cancels the gesture rather than releasing it.
func transformRunnerPointers(dst, src *input, left, top, width, height, screenWidth, screenHeight float64) {
	dst.runnerPointers = nil
	dst.runnerCancel = src.runnerCancel
	if !src.runnerCapture {
		return
	}
	if width <= 0 || height <= 0 {
		dst.runnerCancel = true
		return
	}
	scale := src.runnerScale
	if scale <= 0 {
		scale = 1
	}
	dst.runnerScale = scale * screenWidth / width
	for _, p := range src.runnerPointers {
		x, y := p.X-left, p.Y-top
		if x < 0 || x >= width || y < 0 || y >= height {
			dst.runnerCancel = true
			continue
		}
		dst.runnerPointers = append(dst.runnerPointers, runnerPointerSample{ID: p.ID, X: x * screenWidth / width, Y: y * screenHeight / height})
	}
}

// runnerPointerHeld includes secondary touches that the menu pointer does not follow.
func (in *input) runnerPointerHeld() bool { return in.pheld || len(in.runnerPointers) > 0 }

// runnerInputRecord stores the complete captured pointer set, including empty
// frames, so release order and phone transforms are reproducible.
type runnerInputRecord struct {
	Scale    float64               `json:"scale"`
	Cancel   bool                  `json:"cancel"`
	Pointers []runnerPointerSample `json:"pointers"`
}

func parseRunnerInput(raw any) (runnerInputRecord, error) {
	var result runnerInputRecord
	m, ok := raw.(map[string]any)
	if !ok {
		return result, errors.New("runner input must be an object")
	}
	scale, ok := m["scale"].(float64)
	if !ok || !finiteRunnerValue(scale) || scale < .0001 || scale > 10000 {
		return result, errors.New("runner input scale is out of range")
	}
	cancel, ok := m["cancel"].(bool)
	if !ok {
		return result, errors.New("runner input cancel must be a boolean")
	}
	result.Scale, result.Cancel = scale, cancel
	rows, ok := m["pointers"].([]any)
	if m["pointers"] != nil && !ok {
		return result, errors.New("runner pointers must be an array")
	}
	if len(rows) > runnerPointerLimit {
		return result, errors.New("too many runner pointers")
	}
	for _, row := range rows {
		p, ok := row.(map[string]any)
		if !ok {
			return result, errors.New("runner pointer must be an object")
		}
		id, idOK := p["id"].(float64)
		x, xOK := p["x"].(float64)
		y, yOK := p["y"].(float64)
		if !idOK || !xOK || !yOK || !finiteRunnerValue(id) || !finiteRunnerValue(x) || !finiteRunnerValue(y) || id != math.Trunc(id) || id < -2147483648 || id > 2147483647 || math.Abs(x) > 1000000 || math.Abs(y) > 1000000 {
			return result, errors.New("invalid runner pointer coordinates or identity")
		}
		if slices.ContainsFunc(result.Pointers, func(p runnerPointerSample) bool { return p.ID == int(id) }) {
			return result, errors.New("duplicate runner pointer identity")
		}
		result.Pointers = append(result.Pointers, runnerPointerSample{ID: int(id), X: x, Y: y})
	}
	return result, nil
}

var (
	runnerConfirmKeys = []ebiten.Key{ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyX}
	runnerCancelKeys  = []ebiten.Key{ebiten.KeyEscape, ebiten.KeyBackspace, ebiten.KeyZ, ebiten.KeyF1, ebiten.KeyP}
)

func (in *input) runnerKeys(b button, ordinary []ebiten.Key) []ebiten.Key {
	if in.runnerCapture {
		if b == btnOK {
			return runnerConfirmKeys
		}
		if b == btnCancel {
			return runnerCancelKeys
		}
	}
	return ordinary
}

// runnerPadExtras retains the runner's pause/select buttons and raw USB pads
// without changing the ordinary UI's bindings.
func (in *input) runnerPadExtras() {
	if !in.runnerCapture {
		return
	}
	for _, id := range in.pads {
		if ebiten.IsStandardGamepadLayoutAvailable(id) {
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonCenterRight) {
				in.held[btnOK], in.held[btnCancel] = true, true
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonCenterLeft) {
				in.held[btnCancel] = true
			}
			continue
		}
		if ebiten.IsGamepadButtonPressed(id, 0) || ebiten.IsGamepadButtonPressed(id, 7) {
			in.held[btnOK] = true
		}
		if ebiten.IsGamepadButtonPressed(id, 1) || ebiten.IsGamepadButtonPressed(id, 6) || ebiten.IsGamepadButtonPressed(id, 7) {
			in.held[btnCancel] = true
		}
		if ebiten.GamepadAxisCount(id) >= 2 {
			x, y := ebiten.GamepadAxisValue(id, 0), ebiten.GamepadAxisValue(id, 1)
			in.held[btnLeft] = in.held[btnLeft] || x < -0.5
			in.held[btnRight] = in.held[btnRight] || x > 0.5
			in.held[btnUp] = in.held[btnUp] || y < -0.5
			in.held[btnDown] = in.held[btnDown] || y > 0.5
		}
	}
}

func (in *input) runnerButtonEdges(previous [numButtons]bool) {
	if !in.runnerCapture {
		return
	}
	for _, b := range []button{btnOK, btnCancel} {
		in.pressed[b] = in.held[b] && !previous[b]
	}
}
