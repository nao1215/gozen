package play

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

func (s *dungeonScene) updateFree(g *Game) {
	held := func(b button) float64 {
		if g.in.held[b] || g.in.pressed[b] {
			return 1
		}
		return 0
	}
	in := dungeon.FreeInput{Forward: held(btnUp) - held(btnDown), Turn: held(btnRight) - held(btnLeft)}
	if g.in.shift {
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
	if g.in.ok() || g.in.click {
		s.free.Reset()
		if cmds := dungeon.Look(g.db, g.st); len(cmds) > 0 {
			g.sfx("decide")
			g.events.Start(cmds)
		}
	}
}

func (g *Game) resetFreeMotion() {
	for _, scene := range g.stack {
		if s, ok := scene.(*dungeonScene); ok {
			s.free.Reset()
		}
	}
}
