package dungeon

import (
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/rpg"
)

// 自動で進む床（タイルの slide）と回る床（spin）。
//
// slide のタイルに入ると、その向き（facing なら歩いてきた向き）へ 1 マスずつ運ばれる。
// 前が通れない（壁・扉・通れない置き物）か、slide でない別のタイルに着いたら止まる。
// 途中の slide のタイルは、そのタイルの向きへ曲げる。同じマスを同じ向きで 2 度通ったら（輪）止まる。

// SlideEnd is where the party stops when it comes into the cell x, y moving the way moving
// (0..3): the cell itself when it does not slide, else where the slides carry it, and the
// way it faces there. way lists the cells passed after x, y (the last is where it stops).
func SlideEnd(d *data.Dungeon, st *rpg.State, x, y, moving int) (ex, ey, facing int, way [][2]int) {
	c := CellAt(d, st, x, y)
	dir := c.Tile.SlideWay(moving)
	if dir < 0 || c.Tile == nil || c.Tile != d.Tile(c.Char) { // a roamer stands there: no slide
		return x, y, moving & 3, nil
	}
	w, h := d.Size()
	seen := map[[3]int]bool{}
	for range w*h*4 + 1 {
		k := [3]int{x, y, dir}
		if seen[k] {
			break
		}
		seen[k] = true
		nx, ny := x+dx[dir], y+dy[dir]
		n := CellAt(d, st, nx, ny)
		if !n.Passable() || n.Look == "door" {
			break
		}
		x, y = nx, ny
		way = append(way, [2]int{x, y})
		if n.Tile == nil { // plain floor: on it goes
			continue
		}
		if n.Tile != d.Tile(n.Char) || n.Tile.Slide == "" { // another tile (or a roamer): stop on it
			break
		}
		dir = n.Tile.SlideWay(dir)
	}
	return x, y, dir, way
}

// slideOn carries the party on from the slide cell it has come into (moving the way it
// faces), revealing the cells it passes. It reports the cell it stops on and the slide
// tile's own commands (said on the way), taken by fire (which gives a cell's once a step:
// a slide that cannot move stops where it started, and its commands do not run again).
func slideOn(db *data.DB, d *data.Dungeon, st *rpg.State, c Cell, fire func(Cell) []data.Command) (Cell, []data.Command) {
	if c.Tile == nil || c.Tile.Slide == "" || c.Tile != d.Tile(c.Char) {
		return c, nil
	}
	cmds := fire(c)
	x, y, f, way := SlideEnd(d, st, c.X, c.Y, st.Pos.Facing)
	for _, p := range way {
		st.Pos.X, st.Pos.Y = p[0], p[1]
		st.Steps++
		Reveal(db, st)
	}
	st.Pos.X, st.Pos.Y, st.Pos.Facing = x, y, f
	return CellAt(d, st, x, y), cmds
}

// spin turns the party on a spin tile it stopped on.
func spin(st *rpg.State, c Cell) {
	if c.Tile == nil {
		return
	}
	switch c.Tile.Spin {
	case "right":
		Turn(st, 1)
	case "left":
		Turn(st, -1)
	case "back":
		Turn(st, 2)
	case "random":
		Turn(st, 1+st.Rand.IntN(3))
	}
}

// Slides reports whether stepping into the cell carries the party on (a slide tile).
func Slides(c Cell) bool { return c.Tile != nil && c.Tile.Slide != "" }
