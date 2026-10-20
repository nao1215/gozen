package dungeon

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// The moves Next returns.
const (
	MoveForward = "forward"
	MoveLeft    = "left"
	MoveRight   = "right"
	MoveLook    = "look"
	MoveHere    = "here" // the party is on the goal
)

// Next is the first move on the shortest way to the nearest cell whose character (as it
// is now) is goal: walking onto it, or, for a solid one, up to it and looking at it. The
// way does not step on other tiles that do something when stepped on (stairs...), unless
// there is no other way: then it steps on those that keep the party where it is (a steam
// vent that hurts, a roamer in the way to fight), never on those that take it elsewhere. ok is false when there is no way.
func Next(db *data.DB, st *rpg.State, goal rune) (string, bool) {
	d := Of(db, st)
	if d == nil {
		return "", false
	}
	if m, ok := next(d, st, goal, false); ok {
		return m, true
	}
	return next(d, st, goal, true)
}

func next(d *data.Dungeon, st *rpg.State, goal rune, through bool) (string, bool) {
	p := st.Pos
	if CellAt(d, st, p.X, p.Y).Char == goal {
		return MoveHere, true
	}
	w, h := d.Size()
	type node struct{ x, y, first int } // first: the facing of the first step (-1: none)
	seen := make([]bool, w*h)
	seen[p.Y*w+p.X] = true
	queue := []node{{p.X, p.Y, -1}}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for f := range 4 {
			x, y := n.x+dx[f], n.y+dy[f]
			if x < 0 || y < 0 || x >= w || y >= h || seen[y*w+x] {
				continue
			}
			c := CellAt(d, st, x, y)
			first := n.first
			if first < 0 {
				first = f
			}
			if Slides(c) && c.Passable() { // carried on: the way goes on from where it stops
				ex, ey, _, _ := SlideEnd(d, st, x, y, f)
				e := CellAt(d, st, ex, ey)
				if e.Char == goal {
					return face(p.Facing, first, MoveForward), true
				}
				if !seen[ey*w+ex] && !avoid(st, e, through) {
					seen[ey*w+ex] = true
					queue = append(queue, node{ex, ey, first})
				}
				continue // a slide may be come into from another side
			}
			seen[y*w+x] = true
			if c.Char == goal {
				if n.first < 0 && c.Solid { // next to it: face it and look
					return face(p.Facing, f, MoveLook), true
				}
				if !c.Solid || n.first >= 0 {
					return face(p.Facing, first, MoveForward), true
				}
			}
			if c.Passable() && !avoid(st, c, through) {
				queue = append(queue, node{x, y, first})
			}
		}
	}
	return "", false
}

// face is the turn toward facing want, or then the move.
func face(have, want int, then string) string {
	switch (want - have + 4) & 3 {
	case 0:
		return then
	case 1:
		return MoveRight
	}
	return MoveLeft
}

// avoid reports whether a way keeps off the cell: one that sets its tile off when stepped
// on (a tile with once that has gone off no longer does), or with through only one that
// takes the party elsewhere.
func avoid(st *rpg.State, c Cell, through bool) bool {
	if !stepsOff(c) || c.Tile.Once && Spent(st, st.Location, c.X, c.Y) {
		return false
	}
	return !through || movesAway(c.Tile.Do)
}

// movesAway reports whether the commands may take the party to another place (an event
// or a choice may).
func movesAway(cmds []data.Command) bool {
	for i := range cmds {
		c := &cmds[i]
		if c.Move != "" || c.Dive != "" || c.Call != "" || len(c.Then) > 0 || len(c.Else) > 0 || len(c.Choice) > 0 {
			return true
		}
	}
	return false
}

// stepsOff reports whether stepping on the cell sets its tile off.
func stepsOff(c Cell) bool {
	return c.Tile != nil && c.Tile.Slide == "" && len(c.Tile.Do) > 0 && Trigger(c.Tile, c.Solid) == "step"
}
