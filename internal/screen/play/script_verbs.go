package play

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// scriptVerb is what step needs to know of one script command: how a tick of it
// runs, how long it may wait for the game, and whether a screen must take it.
type scriptVerb struct {
	// run plays one tick of the command. It reports whether the tick is over;
	// false lets step go on to the next command in the same tick.
	run func(s *script, in *input, c *scriptCmd) bool
	// deadline is the ticks the command may wait before the script fails (0: no limit).
	deadline int
	// offered commands are input a screen has to accept when the script asks
	// for acknowledgement.
	offered bool
}

// scriptVerbOf is the table of the script commands, one row a verb.
func scriptVerbOf(verb string) (scriptVerb, bool) {
	switch verb {
	case "hold":
		return scriptVerb{run: (*script).runHold, offered: true}, true
	case "wait":
		return scriptVerb{run: (*script).runWait}, true
	case "click":
		return scriptVerb{run: (*script).runClick, offered: true}, true
	case "drag":
		return scriptVerb{run: (*script).runDrag, offered: true}, true
	case "press":
		return scriptVerb{run: (*script).runPress, offered: true}, true
	case "gap":
		return scriptVerb{run: (*script).runGap}, true
	case "read":
		return scriptVerb{run: (*script).runRead, deadline: 6000, offered: true}, true
	case "shot":
		return scriptVerb{run: (*script).runShot}, true
	case "skip":
		return scriptVerb{run: (*script).runSkip}, true
	case "wait-graphs":
		return scriptVerb{run: (*script).runWaitGraphs}, true
	case "until":
		return scriptVerb{run: (*script).runUntil, deadline: 3000, offered: true}, true
	case "walk", "face":
		return scriptVerb{run: (*script).runWalk, deadline: 6000, offered: true}, true
	case "lines":
		return scriptVerb{run: (*script).runLines, deadline: 20000, offered: true}, true
	case "play":
		return scriptVerb{run: (*script).runPlay, deadline: 40000, offered: true}, true
	case "pick":
		return scriptVerb{run: (*script).runPick, offered: true}, true
	case "window":
		return scriptVerb{run: (*script).runWindow}, true
	case "quit":
		return scriptVerb{run: (*script).runQuit}, true
	case "type":
		v := scriptHook(2, func(s *script, c *scriptCmd) {
			if !s.acknowledge && s.typeText != nil {
				s.typeText(c.arg)
			}
		})
		v.offered = true
		return v, true
	case "memo":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.memoNote != nil {
				s.memoNote(c.arg, c.name)
			}
		}), true
	case "jump":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.jump != nil {
				s.jump(c.arg, c.n)
			}
		}), true
	case "do":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.do != nil {
				s.do(c.arg)
			}
		}), true
	case "battle-log":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.battleLog != nil {
				s.battleLog(c.arg == "on")
			}
		}), true
	case "battle-hp":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.battleHP != nil {
				s.battleHP(c.arg, c.x, c.n)
			}
		}), true
	case "recruit":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.recruit != nil {
				s.recruit(c.arg, c.n)
			}
		}), true
	case "guarantee-recruit":
		return scriptHook(2, func(s *script, c *scriptCmd) {
			if s.guaranteeRecruit != nil {
				s.guaranteeRecruit(c.arg)
			}
		}), true
	case "mark":
		return scriptHook(0, func(s *script, c *scriptCmd) {
			if s.mark != nil {
				s.mark(c.arg)
			}
		}), true
	case "snapshot":
		return scriptHook(0, func(s *script, _ *scriptCmd) {
			if s.snapshot != nil {
				s.snapshot()
			}
		}), true
	case "look":
		return scriptHook(0, func(s *script, _ *scriptCmd) {
			if s.look != nil {
				s.look()
			}
		}), true
	case "guides":
		return scriptHook(0, func(s *script, c *scriptCmd) {
			if s.guides != nil {
				s.guides(c.arg == "on")
			}
		}), true
	}
	return scriptVerb{}, false
}

// scriptHook is a command that only calls one of the game's hooks. With rest it
// ends the tick and waits that many ticks; with none the next command runs in
// the same tick.
func scriptHook(rest int, hook func(s *script, c *scriptCmd)) scriptVerb {
	return scriptVerb{run: func(s *script, _ *input, c *scriptCmd) bool {
		hook(s, c)
		s.pc++
		if rest == 0 {
			return false
		}
		s.wait = rest
		return true
	}}
}

func (s *script) runHold(in *input, c *scriptCmd) bool {
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
	return true
}

func (s *script) runWait(_ *input, c *scriptCmd) bool {
	s.wait = c.n
	s.pc++
	return true
}

func (s *script) runClick(in *input, c *scriptCmd) bool {
	in.click, in.mouse = true, image.Pt(c.x, c.y)
	in.pdown, in.pup = true, true
	s.pc++
	s.wait = 8
	return true
}

// runDrag presses at the start, moves in even steps while held, and lifts at the end.
func (s *script) runDrag(in *input, c *scriptCmd) bool {
	k := dragTicks - c.n
	in.mouse, in.moved = image.Pt(c.x+(c.toX-c.x)*k/dragTicks, c.y+(c.toY-c.y)*k/dragTicks), true
	switch c.n {
	case dragTicks:
		in.click, in.pdown, in.pheld = true, true, true
	case 0:
		in.pup = true
		c.n = dragTicks
		s.pc++
		s.wait = 8
		return true
	default:
		in.pheld = true
	}
	c.n--
	return true
}

func (s *script) runPress(in *input, c *scriptCmd) bool {
	b, ok := scriptButtons[c.arg]
	if d, err := strconv.Atoi(c.arg); !ok && err == nil && len(c.arg) == 1 {
		in.digit = d
		if d == 0 {
			in.digit = 10
		}
	} else if !ok {
		s.pc++
		return false
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
	return true
}

func (s *script) runGap(_ *input, c *scriptCmd) bool {
	s.gap = c.n
	s.pc++
	return false
}

func (s *script) runRead(in *input, c *scriptCmd) bool {
	what, t := "", 0
	if s.waiting != nil {
		what, t = s.waiting()
	}
	s.limit++
	if what == "choice" || what == "command" || (s.idle != nil && s.idle()) {
		s.limit = 0
		s.pc++
		return true
	}
	in.pressed[btnOK] = (what == "text" || what == "phone") && t >= c.n
	return true
}

func (s *script) runShot(_ *input, c *scriptCmd) bool {
	s.shot = c.arg
	s.pc++
	s.wait = 1
	return true
}

func (s *script) runSkip(in *input, c *scriptCmd) bool {
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
	return true
}

func (s *script) runWaitGraphs(*input, *scriptCmd) bool {
	if s.graphsReady != nil && s.graphsReady() {
		s.pc++
	}
	return true
}

func (s *script) runUntil(in *input, _ *scriptCmd) bool {
	if s.idle != nil && !s.idle() {
		s.limit++
		in.pressed[btnOK] = s.limit%6 == 0
		return true
	}
	s.limit = 0
	s.pc++
	return true
}

// runWalk is walk and face: it steers toward the target, and reads on through
// an event the walk sets off.
func (s *script) runWalk(in *input, c *scriptCmd) bool {
	if s.walk == nil {
		s.pc++
		return false
	}
	if !s.walking && s.idle != nil && !s.idle() && s.arrived != nil && !s.arrived() {
		s.walking = true
	}
	if s.walking {
		if s.idle != nil && s.idle() {
			s.walking = false
			s.wait = 10
			return true
		}
		name := c.name
		if name == "" {
			name = "walk_" + c.arg
		}
		s.midEvent(in, name+"_w")
		return true
	}
	arg := c.arg
	if c.verb == "face" {
		arg = "face:" + arg
	}
	b, done := s.walk(arg)
	s.limit++
	if done {
		s.limit = 0
		s.pc++
		return true
	}
	if b >= 0 {
		in.pressed[b] = true
		in.held[b] = true
		s.wait = 2
	}
	return true
}

func (s *script) runLines(in *input, c *scriptCmd) bool {
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
			return true
		}
		end()
		return true
	case (what == "text" || what == "phone") && t >= 4:
		if !s.lineShot {
			s.lineShot = true
			shoot()
			s.wait = 1
			return true
		}
		if c.n > 0 && s.lineDone >= c.n {
			end()
			return true
		}
		in.pressed[btnOK] = true
		s.lineShot = false
		s.wait = 2
		return true
	case what == "" && s.idle != nil && s.idle() && s.limit > 30:
		end()
		return true
	}
	s.lineShot = false
	return true
}

func (s *script) runPlay(in *input, c *scriptCmd) bool {
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
			return true
		}
		s.limit, s.lineShot = 0, false
		s.pc++
		return true
	case what == "command":
		shoot("_b")
		s.wait = 1
		if s.battleAuto != nil {
			s.battleAuto()
		}
		return true
	case (what == "text" || what == "phone") && t >= 4:
		if !s.lineShot {
			s.lineShot = true
			shoot("_")
			s.wait = 1
			return true
		}
		in.pressed[btnOK] = true
		s.lineShot = false
		s.wait = 2
		return true
	case s.inBattle != nil && s.inBattle():
		// `play NAME EVERY` films a battle every EVERY ticks (20 when not given).
		every := 20
		if c.n > 0 {
			every = c.n
		}
		if s.limit%every == 0 {
			shoot("_b")
		}
		return true
	case what == "" && s.idle != nil && s.idle() && s.limit > 30:
		s.limit, s.lineShot = 0, false
		s.pc++
		return true
	}
	s.lineShot = false
	return true
}

func (s *script) runPick(in *input, c *scriptCmd) bool {
	if !s.acknowledge && s.pick != nil {
		s.pick(in, c.name) // a pick that finds nothing is written to the trace
		if s.err != nil {
			return true
		}
	}
	s.pc++
	s.wait = 8
	return true
}

func (s *script) runWindow(_ *input, c *scriptCmd) bool {
	s.size = [2]int{c.x, c.y}
	if s.window != nil {
		s.window(c.x, c.y)
	}
	s.pc++
	s.wait = 10
	return true
}

func (s *script) runQuit(*input, *scriptCmd) bool {
	s.quit = true
	return true
}
