package dungeon_test

import (
	"encoding/json"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

// 条件で 出て、遠くから 追って くる 者（roamers の appear・move: hunt・every・meet）。
const huntGame = `
locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 15
        height: 15
        rooms: 2
        start: S
        place: [{char: ">", far: true}]
      roamers:
        - name: Hunter
          icon: x.png
          mark: "#c0a0ff"
          move: hunt
          appear: "var.pent == 1"
          wait: "4"
          every: "3"
          on_appear: [{say: here}]
          on_leave: [{say: gone}]
          meet: [{say: met}, {battle: one}]
      tiles:
        S: {start: true}
        ">": {name: Stairs, solid: "true", do: [{say: down}]}
`

// distances are the walk from x, y over the cells the party can walk (-1: not reached).
func distances(d *data.Dungeon, st *rpg.State, x0, y0 int) map[[2]int]int {
	dist := map[[2]int]int{{x0, y0}: 0}
	queue := [][2]int{{x0, y0}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, m := range [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
			n := [2]int{p[0] + m[0], p[1] + m[1]}
			if _, ok := dist[n]; ok {
				continue
			}
			if !dungeon.CellAt(d, st, n[0], n[1]).Passable() {
				continue
			}
			dist[n] = dist[p] + 1
			queue = append(queue, n)
		}
	}
	return dist
}

func huntState(t *testing.T) (*data.DB, *rpg.State) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": huntGame})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1, true)
	return db, st
}

// TestHunterComesFromAfar: the hunter comes only when its appear holds and the floor's wait
// is over, onto the floor farthest from the party; it walks a cell every 3 of the party's
// steps over open cells only, is always shown, and touching it runs its meet (it leaves).
func TestHunterComesFromAfar(t *testing.T) {
	db, st := huntState(t)
	d := dungeon.Of(db, st)
	if len(st.Roamers) != 0 || dungeon.Hunters(db, st) != nil {
		t.Fatalf("a hunter before its appear holds: %+v", st.Roamers)
	}
	if got := st.Vars[dungeon.VarHuntNear]; got != float64(dungeon.HuntFar) {
		t.Fatalf("hunt_near with no hunter: %v", got)
	}
	st.Vars["pent"] = 1.0
	if cmds := dungeon.Hunters(db, st); cmds != nil || len(st.Roamers) != 0 {
		t.Fatalf("a hunter within the floor's wait: %+v", st.Roamers)
	}
	st.Steps += 4
	cmds := dungeon.Hunters(db, st)
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "here" || len(st.Roamers) != 1 {
		t.Fatalf("on_appear %+v, roamers %+v", cmds, st.Roamers)
	}
	if dungeon.Hunters(db, st) != nil || len(st.Roamers) != 1 {
		t.Fatal("a second hunter of the same kind")
	}
	// the farthest floor from the party
	dist := distances(d, st, st.Pos.X, st.Pos.Y)
	h := st.Roamers[0]
	most := 0
	for _, v := range dist {
		most = max(most, v)
	}
	if dist[[2]int{h.X, h.Y}] != most {
		t.Fatalf("the hunter came %d cells away, the farthest is %d", dist[[2]int{h.X, h.Y}], most)
	}
	if hs := dungeon.HuntersOn(db, st); len(hs) != 1 {
		t.Fatalf("HuntersOn %+v", hs)
	}
	// hunt_near: the walk to the nearest hunter (データが「近い 間は できない」を 決める)
	dungeon.Hunters(db, st)
	if got := st.Vars[dungeon.VarHuntNear]; got != float64(most) {
		t.Fatalf("hunt_near %v, want %d", got, most)
	}
	// seen in the view: its icon stands where no picture is given
	if c := dungeon.CellAt(d, st, h.X, h.Y); c.Tile == nil || c.Tile.Name != "Hunter" || c.Tile.Picture.File != "x.png" {
		t.Fatalf("the hunter's cell: %+v", c)
	}
	// walk to and fro: it comes a cell every 3 steps, never into a wall, and meets the party
	met := false
	steps, moves := 0, 0
	for range 2000 {
		px, py := st.Pos.X, st.Pos.Y
		was := append([]rpg.Roamer(nil), st.Roamers...)
		moved, cmds := dungeon.Forward(db, st)
		if !moved {
			dungeon.Turn(st, 1)
			continue
		}
		steps++
		if len(cmds) > 0 && cmds[0].Say != nil && cmds[0].Say.Text == "met" {
			met = true
			if len(st.Roamers) != 0 {
				t.Fatalf("the hunter stays after the meet: %+v", st.Roamers)
			}
			if len(cmds) != 2 || cmds[1].Battle != "one" {
				t.Fatalf("meet commands %+v", cmds)
			}
			// 触れる 前から 隣に 見えて いた（遠くから 飛びこまない）か、こちらから 入った
			r := was[0]
			if abs(r.X-px)+abs(r.Y-py) != 1 && (r.X != st.Pos.X || r.Y != st.Pos.Y) {
				t.Fatalf("met a hunter %d,%d not beside %d,%d", r.X, r.Y, px, py)
			}
			break
		}
		if len(cmds) > 0 {
			continue
		}
		r := st.Roamers[0]
		if r != was[0] && (r.X != was[0].X || r.Y != was[0].Y) {
			moves++
			if abs(r.X-was[0].X)+abs(r.Y-was[0].Y) != 1 {
				t.Fatalf("the hunter jumped from %d,%d to %d,%d", was[0].X, was[0].Y, r.X, r.Y)
			}
			if c := d.At(r.X, r.Y); c == data.CellWall || c == data.CellNone {
				t.Fatalf("the hunter walked into a wall at %d,%d", r.X, r.Y)
			}
		}
		// turn away now and then so the walk goes to and fro
		if steps%5 == 0 {
			dungeon.Turn(st, 2)
		}
	}
	if !met {
		t.Fatal("the hunter never met the party")
	}
	if moves*3 > steps+3 {
		t.Errorf("the hunter walked %d cells in %d steps (want a cell every 3 steps)", moves, steps)
	}
	if moves == 0 {
		t.Error("the hunter never walked")
	}
	// right after the meet: not again until the wait is over
	if dungeon.Hunters(db, st) != nil {
		t.Error("the hunter came again right after the meet")
	}
}

// TestHunterLeaves: the hunter leaves when its appear no longer holds (on_leave), and with
// the floor (no on_leave; it waits on the next floor); it stays in a save.
func TestHunterLeaves(t *testing.T) {
	db, st := huntState(t)
	st.Vars["pent"] = 1.0
	st.Steps += 10
	dungeon.Hunters(db, st)
	if len(st.Roamers) != 1 {
		t.Fatal("no hunter")
	}
	// a save keeps where it is
	b, err := json.Marshal(st.Roamers)
	if err != nil {
		t.Fatal(err)
	}
	var back []rpg.Roamer
	if err := json.Unmarshal(b, &back); err != nil || len(back) != 1 || back[0] != st.Roamers[0] {
		t.Fatalf("the save lost the hunter: %s", b)
	}
	st.Vars["pent"] = 0.0
	cmds := dungeon.Hunters(db, st)
	if len(cmds) != 1 || cmds[0].Say == nil || cmds[0].Say.Text != "gone" || len(st.Roamers) != 0 {
		t.Fatalf("on_leave %+v, roamers %+v", cmds, st.Roamers)
	}
	// another floor: gone without a word, and it waits there
	st.Vars["pent"] = 1.0
	st.Steps += 10
	dungeon.Hunters(db, st)
	dungeon.Dive(db, st, "deep", 2, false)
	if len(st.Roamers) != 0 {
		t.Fatalf("the hunter came along to the next floor: %+v", st.Roamers)
	}
	if dungeon.Hunters(db, st) != nil {
		t.Fatal("the hunter came at once on the next floor")
	}
	st.Steps += 4
	if dungeon.Hunters(db, st) == nil || len(st.Roamers) != 1 {
		t.Fatal("the hunter did not come after the wait on the next floor")
	}
}

func TestHunterFasterThanOneCellAStep(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": strings.Replace(huntGame, `every: "3"`, `every: "var.hunt_near > 6 ? 0.5 : 3"`, 1)})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1, true)
	st.Vars["pent"] = 1.0
	st.Steps += 4
	dungeon.Hunters(db, st)
	if len(st.Roamers) != 1 {
		t.Fatalf("no hunter: %+v", st.Roamers)
	}
	fast, slow := 0, 0
	for range 400 {
		if len(st.Roamers) == 0 {
			break
		}
		before := st.Vars[dungeon.VarHuntNear].(float64) //nolint:errcheck // A malformed fixture assertion fails this test; optional counters use zero.
		h0 := st.Roamers[0]
		moved, _ := dungeon.Forward(db, st)
		if !moved {
			dungeon.Turn(st, 1)
			continue
		}
		if len(st.Roamers) == 0 {
			break
		}
		h := st.Roamers[0]
		walked := abs(h.X-h0.X) + abs(h.Y-h0.Y)
		dungeon.Hunters(db, st)
		if before > 6 && walked == 2 {
			fast++
		}
		if before <= 6 && walked >= 2 {
			slow++
		}
	}
	if fast == 0 {
		t.Error("the hunter never came two cells in a step while far")
	}
	if slow > 0 {
		t.Errorf("the hunter came more than a cell a step while near (%d times)", slow)
	}
}
