package play

import (
	"fmt"
	"strconv"
)

func (g *Game) startAt() error {
	o := g.opts
	switch {
	case o.Scene != "":
		scenes := g.db.Game.Gallery.Scenes
		i := -1
		for k, sc := range scenes {
			if sc.Title == o.Scene {
				i = k
				break
			}
		}
		if n, err := strconv.Atoi(o.Scene); i < 0 && err == nil && n >= 1 && n <= len(scenes) {
			i = n - 1
		}
		if i < 0 {
			return fmt.Errorf("unknown gallery scene %q", o.Scene)
		}
		if g.db.Events[scenes[i].Event] == nil {
			return fmt.Errorf("gallery scene %q: unknown event %q", o.Scene, scenes[i].Event)
		}
		g.push(newSceneReplay(g, scenes[i]))
	default:
		if g.st == nil {
			if err := g.testStart(); err != nil {
				return err
			}
		}
		cmds := g.commandsNamed(o.Event)
		if cmds == nil {
			return fmt.Errorf("unknown event %q", o.Event)
		}
		g.enterAt = ""
		g.events.Start(cmds)
	}
	g.fast = &fastTo{lines: max(1, o.Line), answers: o.Answers, picture: o.Picture}
	return nil
}
