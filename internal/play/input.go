package play

import (
	"bufio"
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type button int

const (
	btnUp button = iota
	btnDown
	btnLeft
	btnRight
	btnOK
	btnCancel
	btnMenu
	btnLog
	btnAuto
	btnHide
	btnConfig
	btnNext
	btnMemo
	btnReload
	btnTag
	btnGraphs
	btnSkip
	btnSceneSkip
	btnBack
	btnSave
	btnLoad
	btnPanel
	btnBook
	btnBestiary
	btnMapZoomIn
	btnMapZoomOut
	btnMapOverview
	btnMapFocus
	btnMapPan
	btnEndTurn

	numButtons

	btnNone button = -1
)

var keys = [numButtons][]ebiten.Key{
	btnMapZoomIn:   {ebiten.KeyEqual, ebiten.KeyNumpadAdd},
	btnMapZoomOut:  {ebiten.KeyMinus, ebiten.KeyNumpadSubtract},
	btnMapOverview: {ebiten.KeyHome},
	btnMapFocus:    {ebiten.KeyEnd},
	btnMapPan:      {ebiten.KeyP},
	btnEndTurn:     {ebiten.KeyE},
	btnUp:          {ebiten.KeyArrowUp, ebiten.KeyW, ebiten.KeyK},
	btnDown:        {ebiten.KeyArrowDown, ebiten.KeyS, ebiten.KeyJ},
	btnLeft:        {ebiten.KeyArrowLeft, ebiten.KeyA, ebiten.KeyH},
	btnRight:       {ebiten.KeyArrowRight, ebiten.KeyD, ebiten.KeyL},
	btnOK:          {ebiten.KeyZ, ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyNumpadEnter},
	btnCancel:      {ebiten.KeyX, ebiten.KeyEscape, ebiten.KeyBackspace},
	btnMenu:        {ebiten.KeyX, ebiten.KeyEscape, ebiten.KeyC},
	btnLog:         {ebiten.KeyPageUp, ebiten.KeyB},
	btnAuto:        {ebiten.KeyF, ebiten.KeyA},
	btnHide:        {ebiten.KeyV, ebiten.KeyDelete},
	btnConfig:      {ebiten.KeyO, ebiten.KeyF2},
	btnNext:        {ebiten.KeyTab},
	btnMemo:        {ebiten.KeyF9},
	btnReload:      {ebiten.KeyF5},
	btnTag:         {ebiten.KeyF10},
	btnGraphs:      {ebiten.KeyF7},
	// S is down (WASD), so skip mode is N, and scene skip Page Down (Page Up opens the log).
	btnSkip:      {ebiten.KeyN},
	btnSceneSkip: {ebiten.KeyPageDown},
	// Q rolls back a line (Backspace is cancel); saving and loading are buttons of the novel band.
	btnBack: {ebiten.KeyQ},
	// F8 opens the development panel under --dev (ADR 0029).
	btnPanel: {ebiten.KeyF8},
	// F6 opens and closes the character book under --dev (ADR 0031), and for players
	// when game.yaml sets ui.profiles.
	btnBook:     {ebiten.KeyF6},
	btnBestiary: {ebiten.KeyF4},
}

var (
	digitKeys  = [10]ebiten.Key{ebiten.KeyDigit0, ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3, ebiten.KeyDigit4, ebiten.KeyDigit5, ebiten.KeyDigit6, ebiten.KeyDigit7, ebiten.KeyDigit8, ebiten.KeyDigit9}
	numpadKeys = [10]ebiten.Key{ebiten.KeyNumpad0, ebiten.KeyNumpad1, ebiten.KeyNumpad2, ebiten.KeyNumpad3, ebiten.KeyNumpad4, ebiten.KeyNumpad5, ebiten.KeyNumpad6, ebiten.KeyNumpad7, ebiten.KeyNumpad8, ebiten.KeyNumpad9}
)

var pads = [numButtons][]ebiten.StandardGamepadButton{
	btnMapZoomIn:   {ebiten.StandardGamepadButtonFrontTopRight},
	btnMapZoomOut:  {ebiten.StandardGamepadButtonFrontTopLeft},
	btnMapOverview: {ebiten.StandardGamepadButtonCenterLeft},
	btnMapFocus:    {ebiten.StandardGamepadButtonLeftStick},
	btnMapPan:      {ebiten.StandardGamepadButtonRightStick},
	btnEndTurn:     {ebiten.StandardGamepadButtonRightLeft},
	btnUp:          {ebiten.StandardGamepadButtonLeftTop},
	btnDown:        {ebiten.StandardGamepadButtonLeftBottom},
	btnLeft:        {ebiten.StandardGamepadButtonLeftLeft},
	btnRight:       {ebiten.StandardGamepadButtonLeftRight},
	btnOK:          {ebiten.StandardGamepadButtonRightBottom},
	btnCancel:      {ebiten.StandardGamepadButtonRightRight},
	btnMenu:        {ebiten.StandardGamepadButtonRightRight, ebiten.StandardGamepadButtonRightTop, ebiten.StandardGamepadButtonCenterRight},
	btnLog:         {ebiten.StandardGamepadButtonFrontTopLeft},
	btnAuto:        {ebiten.StandardGamepadButtonFrontTopRight},
	btnHide:        {ebiten.StandardGamepadButtonCenterLeft},
}

type input struct {
	pressed [numButtons]bool
	held    [numButtons]bool
	fast    bool
	esc     bool
	shift   bool
	digit   int
	mouse   image.Point
	ui      float64
	moved   bool
	click   bool
	rclick  bool
	wheel   float64
	script  *script
	replay  *replay
	tick    int
	lastXY  image.Point

	stick [4]int
	pads  []ebiten.GamepadID

	// the touch being followed (a tap is a click, a long press or two fingers the right click)
	touches, justTouched []ebiten.TouchID
	touchID              ebiten.TouchID
	touching, touchDone  bool
	touchAt              image.Point
	longPress, longUsed  bool
}

func (in *input) update() {
	in.pressed = [numButtons]bool{}
	in.click, in.rclick, in.moved, in.wheel = false, false, false, 0
	in.esc, in.shift = false, false
	in.digit = 0
	if in.script != nil {
		in.script.step(in)
		return
	}
	if in.replay != nil {
		in.replay.feed(in, in.tick)
		return
	}
	alt := ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
	for b, ks := range keys {
		held := false
		for _, k := range ks {
			if alt && k == ebiten.KeyEnter {
				continue
			}
			d := inpututil.KeyPressDuration(k)
			if d > 0 {
				held = true
			}
			if d == 1 || (d > 18 && d%5 == 0 && button(b) <= btnRight) {
				in.pressed[b] = true
			}
		}
		in.held[b] = held
	}
	in.fast = ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyControlLeft)

	for _, k := range keys[btnOK] {
		if inpututil.KeyPressDuration(k) > 24 {
			in.fast = true
		}
	}
	in.esc = inpututil.IsKeyJustPressed(ebiten.KeyEscape)
	in.shift = ebiten.IsKeyPressed(ebiten.KeyShift)
	for i := range 10 {
		if inpututil.IsKeyJustPressed(digitKeys[i]) || inpututil.IsKeyJustPressed(numpadKeys[i]) {
			in.digit = i
			if i == 0 {
				in.digit = 10
			}
		}
	}
	in.gamepads()
	x, y := ebiten.CursorPosition()
	if in.ui > 0 && in.ui != 1 {
		x, y = int(float64(x)/in.ui), int(float64(y)/in.ui)
	}
	in.mouse = image.Pt(x, y)
	if in.mouse != in.lastXY {
		in.moved = true
		in.lastXY = in.mouse
	}
	in.click = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	in.rclick = inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	_, in.wheel = ebiten.Wheel()
	in.touch()
}

// longPress is how many ticks a touch is held to be a long press (half a second).
const longPress = 30

// touch turns the touches into the mouse. A tap is a click where the finger lifts; a finger
// held moves the pointer (the lists follow it). A long press is its own signal (longPress):
// a map and a painted scene open the list of the place's entries with it and mark it used;
// anywhere else nothing takes it, and the finger lifting still clicks (a list's entry is
// chosen, not closed). Two fingers are the right click (back, or the list).
func (in *input) touch() {
	in.longPress = false
	in.touches = ebiten.AppendTouchIDs(in.touches[:0])
	if len(in.touches) >= 2 && !in.touchDone {
		in.rclick, in.touchDone = true, true
		return
	}
	if in.touching {
		if inpututil.IsTouchJustReleased(in.touchID) {
			in.touching = false
			if !in.touchDone && !in.longUsed {
				in.click, in.mouse, in.moved = true, in.touchAt, true
			}
			in.longUsed = false
			return
		}
		x, y := ebiten.TouchPosition(in.touchID)
		in.touchAt = in.toUI(x, y)
		in.mouse, in.moved = in.touchAt, true
		if inpututil.TouchPressDuration(in.touchID) == longPress {
			in.longPress = true
		}
		return
	}
	if len(in.touches) == 0 {
		in.touchDone = false
		return
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(in.justTouched[:0]) {
		x, y := ebiten.TouchPosition(id)
		in.touchID, in.touching, in.touchDone, in.longUsed, in.touchAt = id, true, false, false, in.toUI(x, y)
		in.mouse, in.moved = in.touchAt, true
		return
	}
}

// openList reports whether a map or a painted scene opens its list now: the menu key, the
// right click, or a long press (then used: the finger lifting does not click).
func (in *input) openList() bool {
	if in.longPress {
		in.longUsed = true
		return true
	}
	if in.rclick {
		in.rclick = false
		return true
	}
	return in.pressed[btnMenu] && !in.held[btnOK]
}

// toUI is a screen position in the UI's units.
func (in *input) toUI(x, y int) image.Point {
	if in.ui > 0 && in.ui != 1 {
		return image.Pt(int(float64(x)/in.ui), int(float64(y)/in.ui))
	}
	return image.Pt(x, y)
}

func (in *input) gamepads() {
	in.pads = ebiten.AppendGamepadIDs(in.pads[:0])
	var stick [4]bool
	for _, id := range in.pads {
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue
		}
		for b, bs := range pads {
			for _, pb := range bs {
				d := inpututil.StandardGamepadButtonPressDuration(id, pb)
				if d > 0 {
					in.held[b] = true
				}
				if d == 1 || (d > 18 && d%5 == 0 && button(b) <= btnRight) {
					in.pressed[b] = true
				}
			}
		}
		if ebiten.StandardGamepadButtonValue(id, ebiten.StandardGamepadButtonFrontBottomRight) > 0.5 ||
			inpututil.StandardGamepadButtonPressDuration(id, ebiten.StandardGamepadButtonRightBottom) > 24 {
			in.fast = true
		}
		x := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
		y := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
		stick[0] = stick[0] || y < -0.6
		stick[1] = stick[1] || y > 0.6
		stick[2] = stick[2] || x < -0.6
		stick[3] = stick[3] || x > 0.6
	}
	for i, on := range stick {
		if !on {
			in.stick[i] = 0
			continue
		}
		in.stick[i]++
		b := btnUp + button(i)
		in.held[b] = true
		if d := in.stick[i]; d == 1 || (d > 18 && d%5 == 0) {
			in.pressed[b] = true
		}
	}
}

func (in *input) ok() bool     { return in.pressed[btnOK] }
func (in *input) cancel() bool { return in.pressed[btnCancel] || in.rclick }
func (in *input) up() bool     { return in.pressed[btnUp] || in.wheel > 0 }
func (in *input) down() bool   { return in.pressed[btnDown] || in.wheel < 0 }

func (in *input) tab() int {
	if !in.pressed[btnNext] {
		return 0
	}
	if in.shift {
		return -1
	}
	return 1
}

func (in *input) left() bool  { return in.pressed[btnLeft] }
func (in *input) right() bool { return in.pressed[btnRight] }

func (in *input) advance() bool { return in.ok() || in.click || in.wheel < 0 }

type script struct {
	cmds        []scriptCmd
	pc          int
	wait        int
	shot        string
	quit        bool
	idle        func() bool
	graphsReady func() bool
	limit       int
	gap         int
	tps         int

	waiting func() (what string, ticks int)

	walk    func(char string) (button, bool)
	arrived func() bool
	walking bool

	lineN    map[string]int
	lineShot bool
	lineDone int

	battleAuto func()
	inBattle   func() bool
	battleHP   func(team string, index, hp int)
	battleLog  func(enabled bool)

	typeText func(string)
	memoNote func(kind, text string)
	// jump starts an event at a line and do runs a line of YAML (the panel's paths).
	jump             func(name string, line int)
	do               func(src string)
	recruit          func(species string, level int)
	guaranteeRecruit func(species string)

	// snapshot writes the whole game state to the trace (the snapshot command).
	snapshot func()
	// next supplies further lines after the commands run out (Simulate's interactive
	// input); false ends the lines. rejected hears of a line that does not parse.
	next     func() (string, bool)
	rejected func(line string, err error)
}

type scriptCmd struct {
	name string
	x, y int
	verb string
	arg  string
	n    int
}

func parseScript(src string) (*script, error) {
	s := &script{}
	sc := bufio.NewScanner(strings.NewReader(src))
	line := 0
	for sc.Scan() {
		line++
		f := strings.Fields(sc.Text())
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		c := scriptCmd{verb: f[0], n: 1}
		if c.verb == "battle-log" {
			if len(f) != 2 || (f[1] != "off" && f[1] != "on") {
				return nil, fmt.Errorf("script line %d: battle-log off|on", line)
			}
			c.arg = f[1]
			s.cmds = append(s.cmds, c)
			continue
		}
		if c.verb == "battle-hp" {
			if len(f) != 4 || (f[1] != "enemy" && f[1] != "party") {
				return nil, fmt.Errorf("script line %d: battle-hp enemy|party INDEX HP", line)
			}
			index, err := strconv.Atoi(f[2])
			if err != nil || index < 0 {
				return nil, fmt.Errorf("script line %d: battle-hp index must be nonnegative", line)
			}
			hp, err := strconv.Atoi(f[3])
			if err != nil || hp < 1 {
				return nil, fmt.Errorf("script line %d: battle-hp HP must be positive", line)
			}
			c.arg, c.x, c.n = f[1], index, hp
			s.cmds = append(s.cmds, c)
			continue
		}
		if c.verb == "do" {
			c.arg = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "do"))
			if c.arg == "" {
				return nil, fmt.Errorf("script line %d: do YAML", line)
			}
			s.cmds = append(s.cmds, c)
			continue
		}
		// A campaign's order and the end of its turn, as the commands they are (ADR 0030):
		// order ID [GENERAL[:TACTIC][,GENERAL[:TACTIC]...]|-] [TARGET] (the first leads).
		if c.verb == "order" || c.verb == "end_turn" {
			c.verb, c.arg = "do", "- end_turn: true"
			if f[0] == "order" {
				if len(f) < 2 || len(f) > 4 {
					return nil, fmt.Errorf("script line %d: order ID [GENERAL|-] [TARGET]", line)
				}
				general, target := "", ""
				var with, tactics []string
				if len(f) > 2 && f[2] != "-" {
					for i, g := range strings.Split(f[2], ",") {
						id, tactic, ok := strings.Cut(g, ":")
						if ok {
							tactics = append(tactics, fmt.Sprintf("%q: %q", id, tactic))
						}
						if i == 0 {
							general = id
						} else {
							with = append(with, fmt.Sprintf("%q", id))
						}
					}
				}
				if len(f) > 3 {
					target = f[3]
				}
				c.arg = fmt.Sprintf("- order: {id: %s, general: %q, with: [%s], tactics: {%s}, target: %q}", f[1], general, strings.Join(with, ", "), strings.Join(tactics, ", "), target)
			}
			s.cmds = append(s.cmds, c)
			continue
		}
		if c.verb == "type" {
			c.arg = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "type"))
			s.cmds = append(s.cmds, c)
			continue
		}
		if c.verb == "skip" {
			if len(f) < 2 {
				return nil, fmt.Errorf("script line %d: skip TICKS [NAME]", line)
			}
			n, err := strconv.Atoi(f[1])
			if err != nil {
				return nil, fmt.Errorf("script line %d: %w", line, err)
			}
			c.n = n
			if n < 0 {
				return nil, fmt.Errorf("script line %d: negative count", line)
			}
			if len(f) > 2 {
				c.name = f[2]
			}
			s.cmds = append(s.cmds, c)
			continue
		}
		if c.verb == "memo" {
			if len(f) < 3 {
				return nil, fmt.Errorf("script line %d: memo KIND TEXT", line)
			}
			c.arg = f[1]
			rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "memo"))
			c.name = strings.TrimSpace(strings.TrimPrefix(rest, f[1]))
			s.cmds = append(s.cmds, c)
			continue
		}
		if len(f) > 1 {
			c.arg = f[1]
		}
		if len(f) > 2 && (c.verb == "walk" || c.verb == "face") {
			c.name = f[2]
		} else if len(f) > 2 {
			n, err := strconv.Atoi(f[2])
			if err != nil {
				return nil, fmt.Errorf("script line %d: %w", line, err)
			}
			c.n = n
		}
		switch c.verb {
		case "hold":
			if len(f) != 3 || c.n < 1 || c.n > 36000 {
				return nil, fmt.Errorf("script line %d: hold BUTTON[+BUTTON...] TICKS (1..36000)", line)
			}
			for name := range strings.SplitSeq(c.arg, "+") {
				if _, ok := scriptButtons[name]; !ok && name != "shift" {
					return nil, fmt.Errorf("script line %d: unknown held button %q", line, name)
				}
			}
			c.x = c.n
		case "wait", "read", "gap":
			n, err := strconv.Atoi(c.arg)
			if err != nil {
				return nil, fmt.Errorf("script line %d: %w", line, err)
			}
			c.n = n
		case "click":
			if len(f) < 3 {
				return nil, fmt.Errorf("script line %d: click X Y", line)
			}
			x, err := strconv.Atoi(c.arg)
			if err != nil {
				return nil, fmt.Errorf("script line %d: %w", line, err)
			}
			c.x, c.y = x, c.n
			c.n = 1
		case "wait-graphs":
			if len(f) != 1 {
				return nil, fmt.Errorf("script line %d: wait-graphs takes no arguments", line)
			}
		case "press", "shot", "until", "quit":
		case "snapshot":
			if len(f) != 1 {
				return nil, fmt.Errorf("script line %d: snapshot takes no arguments", line)
			}
		case "recruit":
			if c.arg == "" || len(f) != 3 {
				return nil, fmt.Errorf("script line %d: recruit SPECIES LEVEL", line)
			}
			if c.n < 1 {
				return nil, fmt.Errorf("script line %d: recruit level must be positive", line)
			}
		case "guarantee-recruit":
			if c.arg == "" || len(f) != 2 {
				return nil, fmt.Errorf("script line %d: guarantee-recruit SPECIES", line)
			}
		case "walk", "face":
			if c.arg == "" {
				return nil, fmt.Errorf("script line %d: %s TARGET", line, c.verb)
			}
		case "jump":
			if c.arg == "" {
				return nil, fmt.Errorf("script line %d: jump EVENT [LINE]", line)
			}
		case "lines", "play":
			if len(f) < 3 {
				c.n = 0
			}
		default:
			return nil, fmt.Errorf("script line %d: unknown command %q", line, c.verb)
		}

		if c.n < 0 {
			return nil, fmt.Errorf("script line %d: negative count", line)
		}
		if c.verb == "press" {
			if _, ok := scriptButtons[c.arg]; !ok && (len(c.arg) != 1 || c.arg[0] < '0' || c.arg[0] > '9') {
				return nil, fmt.Errorf("script line %d: unknown button %q", line, c.arg)
			}
		}
		if c.verb == "shot" && (c.arg == "" || c.arg == "." || c.arg == ".." || filepath.Base(c.arg) != c.arg || strings.ContainsAny(c.arg, "/\\")) {
			return nil, fmt.Errorf("script line %d: invalid screenshot name", line)
		}
		s.cmds = append(s.cmds, c)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("script: %w", err)
	}
	return s, nil
}

var scriptButtons = map[string]button{
	"map_zoom_in": btnMapZoomIn, "map_zoom_out": btnMapZoomOut, "map_overview": btnMapOverview, "map_focus": btnMapFocus, "map_pan": btnMapPan, "end_turn_key": btnEndTurn,
	"z": btnOK, "ok": btnOK, "enter": btnOK, "x": btnCancel, "cancel": btnCancel, "menu": btnMenu,
	"tab": btnNext, "up": btnUp, "down": btnDown, "left": btnLeft, "right": btnRight,
	"log": btnLog, "auto": btnAuto, "hide": btnHide, "config": btnConfig, "memo": btnMemo,
	"reload": btnReload, "tag": btnTag, "graphs": btnGraphs, "skip": btnSkip, "scene_skip": btnSceneSkip,
	"bestiary": btnBestiary,
	"back":     btnBack, "save": btnSave, "load": btnLoad, "panel": btnPanel, "characters": btnBook,
}

func (s *script) midEvent(in *input, name string) {
	what, t := "", 0
	if s.waiting != nil {
		what, t = s.waiting()
	}
	if s.lineN == nil {
		s.lineN = map[string]int{}
	}
	s.limit++
	shoot := func(kind string) {
		s.lineN[name+kind]++
		s.shot = fmt.Sprintf("%s%s%03d", name, kind, s.lineN[name+kind])
	}
	switch {
	case what == "command":
		shoot("_b")
		s.wait = 1
		if s.battleAuto != nil {
			s.battleAuto()
		}
	case what == "text" || what == "choice":
		if t < 4 && what == "text" {
			return
		}
		if !s.lineShot {
			s.lineShot = true
			shoot("_")
			s.wait = 1
			return
		}
		in.pressed[btnOK] = true
		s.lineShot = false
		s.wait = 2
	case s.inBattle != nil && s.inBattle():
		if s.limit%20 == 0 {
			shoot("_b")
		}
	}
}

func (s *script) step(in *input) {
	in.held = [numButtons]bool{}
	in.fast = false
	if s.wait > 0 {
		s.wait--
		return
	}
	for s.pc < len(s.cmds) || s.more() {
		c := &s.cmds[s.pc]
		switch c.verb {
		case "hold":
			for name := range strings.SplitSeq(c.arg, "+") {
				if name == "shift" {
					in.shift = true
					continue
				}
				b := scriptButtons[name]
				in.held[b] = true
				in.pressed[b] = c.n == c.x
			}
			c.n--
			if c.n == 0 {
				s.pc++
			}
			return
		case "wait":
			s.wait = c.n
			s.pc++
			return
		case "click":
			in.click, in.mouse = true, image.Pt(c.x, c.y)
			s.pc++
			s.wait = 8
			return
		case "press":
			b, ok := scriptButtons[c.arg]
			if d, err := strconv.Atoi(c.arg); !ok && err == nil && len(c.arg) == 1 {
				in.digit = d
				if d == 0 {
					in.digit = 10
				}
			} else if !ok {
				s.pc++
				continue
			} else {
				in.pressed[b] = true
			}
			if ok && b == btnCancel {
				in.pressed[btnMenu] = true
			}
			c.n--
			if c.n <= 0 {
				s.pc++
			}
			s.wait = s.gap
			if s.wait == 0 {
				s.wait = 8
			}
			return
		case "gap":
			s.gap = c.n
			s.pc++
			continue
		case "read":
			what, t := "", 0
			if s.waiting != nil {
				what, t = s.waiting()
			}
			s.limit++
			if what == "choice" || what == "command" || (s.idle != nil && s.idle()) || s.limit > 6000 {
				s.limit = 0
				s.pc++
				return
			}
			in.pressed[btnOK] = what == "text" && t >= c.n
			return
		case "shot":
			s.shot = c.arg
			s.pc++
			s.wait = 1
			return
		case "skip":
			in.fast = true
			s.limit++
			if c.name != "" {
				if s.limit == 1 {
					s.tps = ebiten.TPS()
					ebiten.SetTPS(ebiten.SyncWithFPS)
				}
				s.shot = fmt.Sprintf("%s_%03d", c.name, s.limit)
			}
			if s.limit >= c.n {
				if c.name != "" && s.tps > 0 {
					ebiten.SetTPS(s.tps)
				}
				s.limit = 0
				s.pc++
			}
			return
		case "wait-graphs":
			if s.graphsReady != nil && s.graphsReady() {
				s.pc++
			}
			return
		case "until":
			if s.idle != nil && !s.idle() {
				s.limit++
				if s.limit < 3000 {
					in.pressed[btnOK] = s.limit%6 == 0
					return
				}
			}
			s.limit = 0
			s.pc++
			return
		case "walk", "face":
			if s.walk == nil {
				s.pc++
				continue
			}
			if !s.walking && s.idle != nil && !s.idle() && s.arrived != nil && !s.arrived() {
				s.walking = true
			}
			if s.walking {
				if s.idle != nil && s.idle() {
					s.walking = false
					s.wait = 10
					return
				}
				name := c.name
				if name == "" {
					name = "walk_" + c.arg
				}
				s.midEvent(in, name+"_w")
				return
			}
			arg := c.arg
			if c.verb == "face" {
				arg = "face:" + arg
			}
			b, done := s.walk(arg)
			s.limit++
			if done || s.limit > 6000 {
				s.limit = 0
				s.pc++
				return
			}
			if b >= 0 {
				in.pressed[b] = true
				in.held[b] = true
				s.wait = 2
			}
			return
		case "lines":
			what, t := "", 0
			if s.waiting != nil {
				what, t = s.waiting()
			}
			if s.lineN == nil {
				s.lineN = map[string]int{}
			}
			s.limit++
			shoot := func() {
				s.lineN[c.arg]++
				s.lineDone++
				s.shot = fmt.Sprintf("%s_%03d", c.arg, s.lineN[c.arg])
			}
			end := func() {
				s.limit, s.lineDone, s.lineShot = 0, 0, false
				s.pc++
			}
			switch {
			case what == "choice" || what == "command":
				if !s.lineShot {
					s.lineShot = true
					shoot()
					s.wait = 1
					return
				}
				end()
				return
			case what == "text" && t >= 4:
				if !s.lineShot {
					s.lineShot = true
					shoot()
					s.wait = 1
					return
				}
				if c.n > 0 && s.lineDone >= c.n {
					end()
					return
				}
				in.pressed[btnOK] = true
				s.lineShot = false
				s.wait = 2
				return
			case what == "" && s.idle != nil && s.idle() && s.limit > 30, s.limit > 20000:
				end()
				return
			}
			s.lineShot = false
			return
		case "play":
			what, t := "", 0
			if s.waiting != nil {
				what, t = s.waiting()
			}
			if s.lineN == nil {
				s.lineN = map[string]int{}
			}
			s.limit++
			shoot := func(kind string) {
				key := c.arg + kind
				s.lineN[key]++
				s.shot = fmt.Sprintf("%s%s%03d", c.arg, kind, s.lineN[key])
			}
			switch {
			case what == "choice":
				if !s.lineShot {
					s.lineShot = true
					shoot("_")
					s.wait = 1
					return
				}
				s.limit, s.lineShot = 0, false
				s.pc++
				return
			case what == "command":
				shoot("_b")
				s.wait = 1
				if s.battleAuto != nil {
					s.battleAuto()
				}
				return
			case what == "text" && t >= 4:
				if !s.lineShot {
					s.lineShot = true
					shoot("_")
					s.wait = 1
					return
				}
				in.pressed[btnOK] = true
				s.lineShot = false
				s.wait = 2
				return
			case s.inBattle != nil && s.inBattle():
				// `play NAME EVERY` films a battle every EVERY ticks (20 when not given).
				every := 20
				if c.n > 0 {
					every = c.n
				}
				if s.limit%every == 0 {
					shoot("_b")
				}
				return
			case what == "" && s.idle != nil && s.idle() && s.limit > 30, s.limit > 40000:
				s.limit, s.lineShot = 0, false
				s.pc++
				return
			}
			s.lineShot = false
			return
		case "memo":
			if s.memoNote != nil {
				s.memoNote(c.arg, c.name)
			}
			s.pc++
			s.wait = 2
			return
		case "type":
			if s.typeText != nil {
				s.typeText(c.arg)
			}
			s.pc++
			s.wait = 2
			return
		case "jump":
			if s.jump != nil {
				s.jump(c.arg, c.n)
			}
			s.pc++
			s.wait = 2
			return
		case "do":
			if s.do != nil {
				s.do(c.arg)
			}
			s.pc++
			s.wait = 2
			return
		case "battle-log":
			if s.battleLog != nil {
				s.battleLog(c.arg == "on")
			}
			s.pc++
			s.wait = 2
			return
		case "battle-hp":
			if s.battleHP != nil {
				s.battleHP(c.arg, c.x, c.n)
			}
			s.pc++
			s.wait = 2
			return
		case "recruit":
			if s.recruit != nil {
				s.recruit(c.arg, c.n)
			}
			s.pc++
			s.wait = 2
			return
		case "guarantee-recruit":
			if s.guaranteeRecruit != nil {
				s.guaranteeRecruit(c.arg)
			}
			s.pc++
			s.wait = 2
			return
		case "snapshot":
			if s.snapshot != nil {
				s.snapshot()
			}
			s.pc++
			continue
		case "quit":
			s.quit = true
			return
		}
	}
	s.quit = true
}

// more appends the commands of the next line that parses from next, and reports whether
// there were any. A line that does not parse goes to rejected and the next is read.
func (s *script) more() bool {
	for s.next != nil {
		line, ok := s.next()
		if !ok {
			s.next = nil
			return false
		}
		more, err := parseScript(line)
		if err != nil {
			if s.rejected != nil {
				s.rejected(line, err)
			}
			continue
		}
		if len(more.cmds) > 0 {
			s.cmds = append(s.cmds, more.cmds...)
			return true
		}
	}
	return false
}
