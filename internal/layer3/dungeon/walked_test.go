package dungeon_test

import (
	"fmt"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/internal/world"
)

const madeWalked = `
locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 21
        height: 21
        rooms: 4
        size: "floor <= 2 ? 13 : 17"
        start: "@"
        place:
          - {char: ">", far: true}
          - {char: U, wall: true, steps: 2}
          - {char: R, wall: true, steps: 9}
          - {char: T, count: "2 + floor / 3"}
      tiles:
        "@": {}
        U: {name: Up, look: door, solid: "true", mark: "#9fd3ff", symbol: "↑", do: [{say: up}]}
        R: {name: Hut, look: door, solid: "true", do: [{say: hut}]}
        T: {name: Chest, solid: "true", once: true, after: ".", mark: "#ffd060", do: [{say: chest}]}
        ">": {name: Down, trigger: step, mark: "#ffd0e8", symbol: "↓", do: [{dive: deep, floor: "var.floor + 1"}]}
      walked:
        - prompt: "walked {var.floor_left}"
          choice:
            - label: jump
              do: [{dive: deep, floor: "var.floor", at: ">"}]
            - label: walk
`

// walkWhole stands on every walkable tile reachable from the start and copies it onto the map.
func walkWhole(db *data.DB, st *rpg.State) {
	d := dungeon.Of(db, st)
	w, h := d.Size()
	from := st.Pos
	seen := map[[2]int]bool{{from.X, from.Y}: true}
	q := [][2]int{{from.X, from.Y}}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		st.Pos.X, st.Pos.Y = p[0], p[1]
		dungeon.Reveal(db, st)
		for _, s := range [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
			n := [2]int{p[0] + s[0], p[1] + s[1]}
			if n[0] < 0 || n[1] < 0 || n[0] >= w || n[1] >= h || seen[n] || !dungeon.CellAt(d, st, n[0], n[1]).Passable() {
				continue
			}
			seen[n] = true
			q = append(q, n)
		}
	}
	st.Pos = from
	dungeon.Reveal(db, st)
}

func mapWithShown(d *data.Dungeon, shown []bool) string {
	w, h := d.Size()
	out := ""
	for y := range h {
		for x := range w {
			if shown[y*w+x] {
				out += string(d.At(x, y))
			} else {
				out += "?"
			}
		}
		out += "\n"
	}
	return out
}

// For any floor and seed, walking everything gives 100% exploration, and no tile (black tile) is left undrawn on the map.
// Before walking, walls next to floor not yet seen are not drawn (the area ahead is not revealed).
func TestAWalkedFloorShowsEveryCell(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": madeWalked})
	for floor := 1; floor <= 6; floor++ {
		for seed := range 30 {
			st := testgame.State(db, uint64(seed))
			st.Vars[dungeon.VarSeed] = float64(seed*7919 + 13)
			st.Location = "deep"
			dungeon.Dive(db, st, "deep", floor, true)
			d := dungeon.Of(db, st)
			name := fmt.Sprintf("B%dF seed %d", floor, seed)
			before := dungeon.Shown(db, st)
			w, h := d.Size()
			for y := range h {
				for x := range w {
					if before[y*w+x] && !dungeon.Seen(db, st, "deep", x, y) && d.At(x, y) != data.CellWall {
						t.Fatalf("%s: an open cell %d,%d not seen is drawn", name, x, y)
					}
				}
			}
			walkWhole(db, st)
			if e := st.Vars[dungeon.VarExplored]; e != 100.0 {
				t.Fatalf("%s: explored %v after walking every cell", name, e)
			}
			shown := dungeon.Shown(db, st)
			for i, ok := range shown {
				if !ok {
					t.Fatalf("%s: %d,%d is not drawn on a floor walked whole\n%s", name, i%w, i/w, mapWithShown(d, shown))
				}
			}
		}
	}
}

// walked moves only on arriving at the start of a fully walked floor, and the number of remaining chests goes into var.floor_left.
// Jumping to in front of the down stairs (not the start) does not move it.
func TestWalkedOnArrivingAtTheStart(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": madeWalked})
	st := testgame.State(db, 1)
	st.Vars[dungeon.VarSeed] = 77.0
	st.Location = "deep"
	dungeon.Dive(db, st, "deep", 3, true)
	if cmds := world.Arrive(db, st, false); len(cmds) != 0 {
		t.Fatalf("walked on a floor not walked yet: %+v", cmds)
	}
	walkWhole(db, st)
	dungeon.Dive(db, st, "deep", 3, true) // dive again and return to the start of the same floor
	cmds := world.Arrive(db, st, false)
	if len(cmds) != 1 || cmds[0].Prompt == "" {
		t.Fatalf("no walked prompt on arriving at a floor walked whole: %+v", cmds)
	}
	if n := st.Vars[dungeon.VarLeft]; n != 3.0 { // 2 + 3/3 chests not yet opened
		t.Errorf("floor_left %v, want 3", n)
	}
	// In front of the down stairs (at: ">"): not the start, so it does not ask
	dungeon.DiveAt(db, st, "deep", 3, ">", "")
	if cmds := world.Arrive(db, st, false); len(cmds) != 0 {
		t.Errorf("walked again after jumping to the stairs down: %+v", cmds)
	}
	if c := dungeon.CellAt(dungeon.Of(db, st), st, st.Pos.X, st.Pos.Y); c.Char == '>' {
		t.Errorf("jumped onto the stairs down (it would not go off until stepping off and on)")
	}
}
