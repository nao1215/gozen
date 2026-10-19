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
	numButtons

	btnNone button = -1
)

var keys = [numButtons][]ebiten.Key{
	btnUp:     {ebiten.KeyArrowUp, ebiten.KeyW, ebiten.KeyK},
	btnDown:   {ebiten.KeyArrowDown, ebiten.KeyS, ebiten.KeyJ},
	btnLeft:   {ebiten.KeyArrowLeft, ebiten.KeyA, ebiten.KeyH},
	btnRight:  {ebiten.KeyArrowRight, ebiten.KeyD, ebiten.KeyL},
	btnOK:     {ebiten.KeyZ, ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyNumpadEnter},
	btnCancel: {ebiten.KeyX, ebiten.KeyEscape, ebiten.KeyBackspace},
	btnMenu:   {ebiten.KeyX, ebiten.KeyEscape, ebiten.KeyC},
	btnLog:    {ebiten.KeyPageUp, ebiten.KeyB},
	btnAuto:   {ebiten.KeyF, ebiten.KeyA},
	btnHide:   {ebiten.KeyV, ebiten.KeyDelete},
	btnConfig: {ebiten.KeyO, ebiten.KeyF2},
	btnNext:   {ebiten.KeyTab},
	btnMemo:   {ebiten.KeyF9},
	btnReload: {ebiten.KeyF5},
	btnTag:    {ebiten.KeyF10},
	btnGraphs: {ebiten.KeyF7},
}

var (
	digitKeys  = [10]ebiten.Key{ebiten.KeyDigit0, ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3, ebiten.KeyDigit4, ebiten.KeyDigit5, ebiten.KeyDigit6, ebiten.KeyDigit7, ebiten.KeyDigit8, ebiten.KeyDigit9}
	numpadKeys = [10]ebiten.Key{ebiten.KeyNumpad0, ebiten.KeyNumpad1, ebiten.KeyNumpad2, ebiten.KeyNumpad3, ebiten.KeyNumpad4, ebiten.KeyNumpad5, ebiten.KeyNumpad6, ebiten.KeyNumpad7, ebiten.KeyNumpad8, ebiten.KeyNumpad9}
)

var pads = [numButtons][]ebiten.StandardGamepadButton{
	btnUp:     {ebiten.StandardGamepadButtonLeftTop},
	btnDown:   {ebiten.StandardGamepadButtonLeftBottom},
	btnLeft:   {ebiten.StandardGamepadButtonLeftLeft},
	btnRight:  {ebiten.StandardGamepadButtonLeftRight},
	btnOK:     {ebiten.StandardGamepadButtonRightBottom},
	btnCancel: {ebiten.StandardGamepadButtonRightRight},
	btnMenu:   {ebiten.StandardGamepadButtonRightRight, ebiten.StandardGamepadButtonRightTop, ebiten.StandardGamepadButtonCenterRight},
	btnLog:    {ebiten.StandardGamepadButtonFrontTopLeft},
	btnAuto:   {ebiten.StandardGamepadButtonFrontTopRight},
	btnHide:   {ebiten.StandardGamepadButtonCenterLeft},
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
	cmds  []scriptCmd
	pc    int
	wait  int
	shot  string
	quit  bool
	idle  func() bool
	limit int
	gap   int
	tps   int

	waiting func() (what string, ticks int)

	walk    func(char string) (button, bool)
	arrived func() bool
	walking bool

	lineN    map[string]int
	lineShot bool
	lineDone int

	battleAuto func()
	inBattle   func() bool

	typeText func(string)
	memoNote func(kind, text string)
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
			if len(f) < 2 {
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
		case "wait", "read", "gap":
			n, err := strconv.Atoi(c.arg)
			if err != nil {
				return nil, fmt.Errorf("script line %d: %w", line, err)
			}
			c.n = n
		case "click":
			x, err := strconv.Atoi(c.arg)
			if err != nil {
				return nil, fmt.Errorf("script line %d: %w", line, err)
			}
			c.x, c.y = x, c.n
			c.n = 1
		case "press", "shot", "until", "quit", "walk", "face", "play":
		case "lines":
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
		if c.verb == "shot" && (c.arg == "" || filepath.Base(c.arg) != c.arg || strings.ContainsAny(c.arg, "/\\")) {
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
	"z": btnOK, "ok": btnOK, "enter": btnOK, "x": btnCancel, "cancel": btnCancel, "menu": btnMenu,
	"tab": btnNext, "up": btnUp, "down": btnDown, "left": btnLeft, "right": btnRight,
	"log": btnLog, "auto": btnAuto, "hide": btnHide, "config": btnConfig, "memo": btnMemo,
	"reload": btnReload, "tag": btnTag, "graphs": btnGraphs,
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
	for s.pc < len(s.cmds) {
		c := &s.cmds[s.pc]
		switch c.verb {
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
				if s.limit%20 == 0 {
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
		case "quit":
			s.quit = true
			return
		}
	}
	s.quit = true
}
