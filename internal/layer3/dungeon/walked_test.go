package dungeon_test

import (
	"fmt"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/world"
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

// walkWhole は、スタートから行ける全部の通れるマスに立って地図に写す。
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

// どの階・どの種でも、歩きつくせば探索 100% になり、地図に描かれないマス（黒いマス）が残らない。
// 歩く前は、まだ見ていない床の隣の壁を描かない（前方を明かさない）。
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

// 歩きつくした階のスタートに着いたときだけ walked が動き、残りの宝箱の数が var.floor_left に入る。
// 下り階段の前へ飛んで着いたとき（スタートでない所）は動かない。
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
	dungeon.Dive(db, st, "deep", 3, true) // 潜り直して同じ階のスタートへ
	cmds := world.Arrive(db, st, false)
	if len(cmds) != 1 || cmds[0].Prompt == "" {
		t.Fatalf("no walked prompt on arriving at a floor walked whole: %+v", cmds)
	}
	if n := st.Vars[dungeon.VarLeft]; n != 3.0 { // 2 + 3/3 個の宝箱を、まだ開けていない
		t.Errorf("floor_left %v, want 3", n)
	}
	// 下り階段の前へ（at: ">"）: スタートではないので聞かない
	dungeon.DiveAt(db, st, "deep", 3, ">", "")
	if cmds := world.Arrive(db, st, false); len(cmds) != 0 {
		t.Errorf("walked again after jumping to the stairs down: %+v", cmds)
	}
	if c := dungeon.CellAt(dungeon.Of(db, st), st, st.Pos.X, st.Pos.Y); c.Char == '>' {
		t.Errorf("jumped onto the stairs down (it would not go off until stepping off and on)")
	}
}
