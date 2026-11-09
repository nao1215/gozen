package play

import (
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/dungeon"
)

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
	if g.in.inspect(g.in.pressed[btnOK] || g.in.click) {
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
