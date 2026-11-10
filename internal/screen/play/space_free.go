package play

import (
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/dungeon"
)

// freeHoldFrom is how many ticks a press is held on a free-move space before it walks.
const freeHoldFrom = 10

func (s *dungeonScene) updateFree(g *Game) {
	held := func(b button) float64 {
		if g.in.heldKey(b) || g.in.pressedKey(b) {
			return 1
		}
		return 0
	}
	in := dungeon.FreeInput{Forward: held(btnUp) - held(btnDown), Turn: held(btnRight) - held(btnLeft)}
	if g.in.query(g.in.shift) {
		in.Strafe, in.Turn = in.Turn, 0
	}
	// A press held on the view walks without a keyboard: its sides turn, its foot steps
	// back, the rest goes forward. A press shorter than freeHoldFrom is a tap (it examines).
	walked := s.freeHold >= freeHoldFrom
	if g.in.inspect(g.in.pheld) {
		s.freeHold++
	} else {
		s.freeHold = 0
	}
	if s.freeHold >= freeHoldFrom && g.in.pointerHeld() {
		sw, sh := g.screen()
		switch mx, my := float64(g.in.mouse.X), float64(g.in.mouse.Y); {
		case mx < sw/3:
			in.Turn = -1
		case mx > sw*2/3:
			in.Turn = 1
		case my > sh*0.8:
			in.Forward = -1
		default:
			in.Forward = 1
		}
	}
	state := g.st
	s.free.Update(g.db, g.st, in, func(cmds []data.Command) bool {
		g.events.Start(cmds)
		blocked := g.pump()
		return !blocked && g.st == state && len(g.stack) == 1 && g.stack[0] == s
	})
	if g.st != state || g.busy() {
		s.free.Reset()
		return
	}
	if g.in.inspect(g.in.pressed[btnOK] || g.in.click && !walked) {
		s.free.Reset()
		g.in.query(s.examine(g))
	}
}

func (g *Game) resetFreeMotion() {
	for _, scene := range g.stack {
		if s, ok := scene.(*dungeonScene); ok {
			s.free.Reset()
		}
	}
}
