package play

import (
	"strings"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

func (g *Game) walkStep(char string) (button, bool) {
	face := strings.HasPrefix(char, "face:")
	char = strings.TrimPrefix(char, "face:")
	if g.busy() {
		return 0, true
	}
	ds, ok := g.stack[0].(*dungeonScene)
	if !ok || len(g.stack) != 1 {
		return 0, true
	}
	if ds.transT > 0 {
		return btnNone, false
	}
	d := dungeon.Of(g.db, g.st)
	if d == nil {
		return 0, true
	}
	st := g.st
	w, h := d.Size()
	type node struct{ x, y int }
	start := node{st.Pos.X, st.Pos.Y}
	isGoal := func(n node) bool {
		if char == "roamer" {
			for _, r := range st.Roamers {
				if r.X == n.x && r.Y == n.y {
					return true
				}
			}
			return false
		}
		c := dungeon.CellAt(d, st, n.x, n.y)
		return string(c.Char) == char && !dungeon.Spent(st, st.Location, n.x, n.y)
	}

	for _, avoid := range []bool{true, false} {
		prev := map[node]node{start: start}
		q := []node{start}
		var goal *node
		for len(q) > 0 && goal == nil {
			c := q[0]
			q = q[1:]
			for _, dd := range [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
				n := node{c.x + dd[0], c.y + dd[1]}
				if n.x < 0 || n.y < 0 || n.x >= w || n.y >= h {
					continue
				}
				if _, seen := prev[n]; seen {
					continue
				}
				if isGoal(n) {
					prev[n] = c
					goal = &n
					break
				}
				cell := dungeon.CellAt(d, st, n.x, n.y)
				if !cell.Passable() || dungeon.Slides(cell) {
					continue
				}
				if avoid && cell.Tile != nil && len(cell.Tile.Do) > 0 && !dungeon.Spent(st, st.Location, n.x, n.y) {
					continue
				}
				prev[n] = c
				q = append(q, n)
			}
		}
		if goal == nil {
			continue
		}
		g.walkGoal = [3]int{goal.x, goal.y, 1}

		next := *goal
		for prev[next] != start {
			next = prev[next]
		}
		want := facingTo(st.Pos.X, st.Pos.Y, next.x, next.y)
		switch (want - st.Pos.Facing + 4) % 4 {
		case 0:
			if face && next == *goal {
				g.walkGoal = [3]int{}
				return 0, true
			}
			cell := dungeon.CellAt(d, st, next.x, next.y)
			if next == *goal && !cell.Passable() {
				return btnUp, false
			}
			return btnUp, false
		case 1:
			return btnRight, false
		case 3:
			return btnLeft, false
		default:
			return btnRight, false
		}
	}
	return 0, true
}

func (g *Game) walkArrived() bool {
	if g.walkGoal[2] == 0 || g.st == nil {

		return false
	}
	p := g.st.Pos
	if p.X == g.walkGoal[0] && p.Y == g.walkGoal[1] {
		return true
	}
	x, y := dungeon.Rel(p, 1, 0)
	return x == g.walkGoal[0] && y == g.walkGoal[1]
}
