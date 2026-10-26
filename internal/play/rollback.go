package play

import (
	"log"
	"math/rand/v2"
	"slices"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
)

// Rolling back and the cursor of a save made in the middle of an event (ADR 0025). Both
// begin an event again from the state saved when it started and replay it, quietly, to a
// line with the answers given before it: the replay of a note of F9 and of F5.

// backEvent is an earlier event the player can roll back into.
type backEvent struct {
	snap, rnd []byte
	cmds      []data.Command
	name      string
	lines     int
	answers   []int
	log       []logLine
}

// rollbackSize is how many earlier events are kept (ui.rollback; 0 is off).
func (g *Game) rollbackSize() int {
	if r := g.db.Game.UI.Rollback; r != nil {
		return *r
	}
	return data.DefaultRollback
}

// keepEvent puts the event that just ended on the ring of earlier events, when it can be
// replayed (it showed a line and passed no boundary).
func (g *Game) keepEvent() {
	n := g.rollbackSize()
	if g.rollingBack || g.evCmds == nil || g.evLines == 0 || g.evNoBack || g.evSnap == nil || n == 0 {
		return
	}
	g.backRing = append(g.backRing, backEvent{
		snap: g.evSnap, rnd: g.evRand, cmds: g.evCmds, name: g.evName,
		lines: g.evLines, answers: slices.Clone(g.evAnswers), log: g.evLog,
	})
	if len(g.backRing) > n {
		g.backRing = slices.Delete(g.backRing, 0, len(g.backRing)-n)
	}
}

// boundary marks a request that cannot be replayed (a battle, a shop, a save, a screen, a
// move, the end): the running event is not rolled back nor saved with a cursor, and the
// earlier events are dropped.
func (g *Game) boundary() {
	g.evNoBack = true
	g.backRing = nil
}

// isBoundary reports whether a request is a boundary.
func isBoundary(r event.Request) bool {
	switch r.(type) {
	case event.Battle, event.Shop, event.Save, event.Open, event.Clear, event.GameOver:
		return true
	}
	return false
}

// canRollBack reports whether the line on the screen has a line before it to go back to.
func (g *Game) canRollBack() bool {
	if g.rollbackSize() == 0 || !g.talk.active || len(g.stack) != 1 || g.replaying || g.fast != nil || g.st == nil ||
		g.events == nil || g.currentBattle() != nil || g.evLines == 0 {
		return false
	}
	return g.evLines > 1 && !g.evNoBack && g.evSnap != nil && g.evCmds != nil || len(g.backRing) > 0
}

// rollBack goes back one line: to the line before in the running event, or to the last line
// of the event before it.
func (g *Game) rollBack() {
	if !g.canRollBack() {
		return
	}
	from, name := g.evLines, g.evName
	var to backEvent
	into := false
	if from > 1 && !g.evNoBack && g.evSnap != nil && g.evCmds != nil && len(g.evHist) >= from-1 {
		to = backEvent{snap: g.evSnap, rnd: g.evRand, cmds: g.evCmds, name: g.evName, lines: from - 1, answers: slices.Clone(g.evAnswers[:g.evHist[from-2]]), log: g.evLog}
	} else {
		last := len(g.backRing) - 1
		to, into = g.backRing[last], true
		g.backRing = g.backRing[:last]
	}
	if !g.replayEvent(to) {
		return
	}
	l := traceLine{"from": from, "to": g.evLines, "event": g.evName}
	if into {
		l["back_into"], l["left"] = true, name
	}
	g.emit("rollback", l)
}

// replayEvent begins an event again from its saved start and reads on to its line.
func (g *Game) replayEvent(e backEvent) bool {
	p := rand.NewPCG(1, 2)
	if p.UnmarshalBinary(e.rnd) != nil {
		p = g.newPCG()
	}
	st, err := rpg.Parse(g.db, e.snap, rand.New(p)) //nolint:gosec // Restores deterministic gameplay randomness, not secrets.
	if err != nil {
		log.Printf("roll back: %v", err)
		return false
	}
	g.stopSkip("rollback")
	ring := g.backRing
	g.rollingBack = true
	g.pcg = p
	g.loadGame(st)
	g.enterAt = ""
	g.backlog = slices.Clip(e.log)
	g.events.Start(e.cmds)
	g.rollingBack = false
	g.backRing = ring
	g.liveCatchUp(&fastTo{lines: e.lines, answers: e.answers}, g.evName, nil)
	return true
}

// startArrival makes and starts the commands of arriving at the location, keeping the state
// before them for the cursor of a save made in the arrival's lines.
func (g *Game) startArrival(battles bool) []data.Command {
	snap, err := g.st.Save()
	if err != nil {
		g.failure = err
		return nil
	}
	var rnd []byte
	if g.pcg != nil {
		if rnd, err = g.pcg.MarshalBinary(); err != nil {
			g.failure = err
			return nil
		}
	}
	cmds := world.Arrive(g.db, g.st, battles)
	g.arriveSnap, g.arriveRand, g.arriving = snap, rnd, true
	g.events.Start(cmds)
	g.arriving = false
	return cmds
}

// cursor is where a save made now goes on, with the state and random bytes to save
// (false: the save holds the state as it is).
func (g *Game) cursor() (rpg.Cursor, []byte, []byte, bool) {
	if g.events == nil || !g.events.Running() || !g.talk.active || g.evNoBack || g.evLines == 0 || g.replaying || g.fast != nil || g.evSnap == nil {
		return rpg.Cursor{}, nil, nil, false
	}
	c := rpg.Cursor{Event: g.evName, Line: g.evLines, Answers: slices.Clone(g.evAnswers)}
	switch {
	case g.evName != "":
		return c, g.evSnap, g.evRand, true
	case g.evArrival && g.arriveSnap != nil:
		c.Arrival = true
		return c, g.arriveSnap, g.arriveRand, true
	}
	return rpg.Cursor{}, nil, nil, false
}

// saveBytes is the save of now: with a cursor in the middle of an event.
func (g *Game) saveBytes() ([]byte, error) {
	c, snap, rnd, ok := g.cursor()
	if !ok {
		b, err := g.st.Save()
		if err != nil || g.pcg == nil {
			return b, err
		}
		if rnd, err = g.pcg.MarshalBinary(); err != nil {
			return nil, err
		}
		return rpg.WithRand(b, rnd)
	}
	return rpg.WithCursor(snap, c, rnd)
}

// resume goes on from the cursor of a loaded save: the event begins again and is read on to
// the line (an arrival happens again on the place's first update).
func (g *Game) resume(r *rpg.Resume) {
	c := r.Cursor
	name := c.Event
	if !c.Arrival {
		cmds := g.commandsNamed(c.Event)
		if cmds == nil {
			log.Printf("load: no event %q (the data changed?): starting from where it began", c.Event)
			return
		}
		g.enterAt = ""
		g.events.Start(cmds)
	}
	g.liveCatchUp(&fastTo{lines: c.Line, answers: c.Answers}, name, nil)
	g.emit("resume", traceLine{"event": c.Event, "arrival": c.Arrival, "line": c.Line, "at": g.evLines})
}

// saveBlocked is why a save now is refused, "" when it is not: in the lines of an event
// after a battle, a shop, a screen or a move, a save cannot hold where the event goes on
// (the event cannot be read again up to that line), so it would lose the event's rest.
func (g *Game) saveBlocked() string {
	if g.events == nil || !g.events.Running() || !g.talk.active || !g.evNoBack || g.replaying {
		return ""
	}
	return lang.T("この 会話の 途中では セーブ できない。会話が 終わってから セーブ しよう。")
}
