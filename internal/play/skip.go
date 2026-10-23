package play

import (
	"math"
	"strings"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Skip mode and scene skip (ADR 0026). Skip mode passes the lines read, as Ctrl held does,
// until a line that was not read, a choice, a battle or the end of the events. Scene skip
// replays the running event at once, quietly, to the same places, when all its lines were read.

// skipPasses reports whether skipping goes past the line (read, or the setting skips all).
func (g *Game) skipPasses(s event.Say) bool {
	return s.Text == "" || g.cfg.SkipAll || g.isRead(s)
}

func (g *Game) toggleSkip() {
	if g.skipping {
		g.stopSkip("button")
		return
	}
	if g.talk.choice != nil {
		return // a choice is waiting for the player; skipping would stop here at once
	}
	g.skipping = true
	g.finishCGTransition()
	g.emit("skip", traceLine{"on": true})
}

func (g *Game) stopSkip(why string) {
	if !g.skipping {
		return
	}
	g.skipping = false
	g.emit("skip", traceLine{"on": false, "why": why})
}

// skipBefore stops skip mode and scene skip before the screen shows a request they do not
// pass, so that the trace never has a line not read while skipping.
func (g *Game) skipBefore(r event.Request) {
	why := ""
	switch r := r.(type) {
	case event.Say:
		if !g.skipPasses(r) {
			why = "unread"
		}
	case event.Choice:
		why = "choice"
	case event.Battle:
		why = "battle"
	}
	if why == "" {
		return
	}
	g.stopSkip(why)
	if g.fast != nil && g.fast.scene {
		g.endSceneSkip(why)
	}
}

// eventLines are the keys of the lines an event can show: its say commands, into the branches
// of if, choice and battle and the events it calls, whatever the conditions. Lines with no
// text are left out (skipping passes them anyway), and so are the lines said instead of a
// fallen member in battle.
func (g *Game) eventLines(name string) []uint64 {
	if g.sceneLinesDB != g.db {
		g.sceneLines, g.sceneLinesDB = map[string][]uint64{}, g.db
	}
	if keys, ok := g.sceneLines[name]; ok {
		return keys
	}
	keys := commandLines(g.db, g.commandsNamed(name))
	g.sceneLines[name] = keys
	return keys
}

func commandLines(db *data.DB, cmds []data.Command) []uint64 {
	var keys []uint64
	seen := map[uint64]bool{}
	called := map[string]bool{}
	var walk func(cs []data.Command)
	walk = func(cs []data.Command) {
		for i := range cs {
			c := &cs[i]
			if s := c.Say; s != nil && strings.TrimSpace(s.Text) != "" {
				if k := event.SourceKey(s.Who, s.Text); !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
			if ev := db.Events[c.Call]; ev != nil && !called[c.Call] {
				called[c.Call] = true
				walk(ev.Do)
			}
			walk(c.Then)
			walk(c.Else)
			walk(c.OnWin)
			walk(c.OnLose)
			walk(c.OnRun)
			for _, o := range c.Choice {
				walk(o.Do)
			}
		}
	}
	walk(cmds)
	return keys
}

// readRate is how many of an event's lines were read, of how many it has.
func (g *Game) readRate(name string) (read, all int) {
	for _, k := range g.eventLines(name) {
		all++
		if g.read[k] {
			read++
		}
	}
	return read, all
}

// sceneSkippable reports whether the running event can be skipped: it has a name, it is
// not a recollection, a line is on the screen, and every line it can show was read.
func (g *Game) sceneSkippable() bool {
	if g.evName == "" || g.replaying || g.fast != nil || g.st == nil || !g.talk.active || g.events == nil || !g.events.Running() {
		return false
	}
	read, all := g.readRate(g.evName)
	return all > 0 && read == all
}

// sceneSkip replays the running event at once, quietly, to its next choice, line not read,
// battle or end.
func (g *Game) sceneSkip() {
	if !g.sceneSkippable() {
		return
	}
	g.stopSkip("scene")
	name := g.evName
	read, all := g.readRate(name)
	g.emit("skip", traceLine{"scene": true, "on": true, "event": name, "line": g.evLines, "read": read, "lines": all})
	g.liveCatchUp(&fastTo{lines: math.MaxInt, scene: true}, name, nil)
}

// endSceneSkip stops scene skip (fastStep's stop does the same at the end of the events).
func (g *Game) endSceneSkip(why string) {
	g.fast = nil
	g.emit("skip", traceLine{"scene": true, "on": false, "why": why, "event": g.evName, "line": g.evLines})
}
