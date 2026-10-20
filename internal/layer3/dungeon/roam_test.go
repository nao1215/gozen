package dungeon_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const roamGame = `
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
        - {name: Brute, picture: x.png, count: "2", move: chase, sight: 99, far: 3, troops: [{troop: one}]}
      tiles:
        S: {start: true}
        ">": {name: Stairs, solid: "true", do: [{say: down}]}
`

// TestRoamersComeAndFight: a dive places the roamers away from the start; a chaser comes
// to the party a cell a step, meeting it starts its battle, and won it is gone.
func TestRoamersComeAndFight(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": roamGame})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	if len(st.Roamers) != 2 {
		t.Fatalf("placed %d roamers, want 2", len(st.Roamers))
	}
	for _, r := range st.Roamers {
		if abs(r.X-st.Pos.X)+abs(r.Y-st.Pos.Y) < 3 {
			t.Fatalf("a roamer at %d,%d is too near the start %d,%d", r.X, r.Y, st.Pos.X, st.Pos.Y)
		}
	}
	d := dungeon.Of(db, st)
	// a roamer shows as a tile with its name
	r := st.Roamers[0]
	if c := dungeon.CellAt(d, st, r.X, r.Y); c.Tile == nil || c.Tile.Name != "Brute" {
		t.Fatalf("the roamer's cell: %+v", c)
	}
	// stand still facing a wall and step into it: the chasers come
	var met bool
	for range 200 {
		px, py := st.Pos.X, st.Pos.Y
		was := append([]rpg.Roamer(nil), st.Roamers...)
		moved, cmds := dungeon.Forward(db, st)
		if !moved {
			dungeon.Turn(st, 1)
		}
		if len(cmds) > 0 && cmds[0].Battle == "one" {
			met = true
			beside := false
			for _, r := range was {
				if abs(r.X-px)+abs(r.Y-py) == 1 {
					beside = true
				}
			}
			if !beside {
				t.Fatalf("a roamer not next to the party (%d,%d) before the step started its battle: %+v", px, py, was)
			}
			break
		}
	}
	if !met {
		t.Fatal("no roamer was met")
	}
	// the battle won: its on_win marks it beaten, and it is gone at the next step
	st.Vars[dungeon.VarRoamerBeaten] = 1.0
	dungeon.SettleRoamers(st)
	if len(st.Roamers) != 1 {
		t.Fatalf("after the win %d roamers, want 1", len(st.Roamers))
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// TestFloorsKeepTheirMaps: a floor is the same floor on each dive, and what was walked on
// it stays on its map after going to another floor and to camp.
func TestFloorsKeepTheirMaps(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": roamGame})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	start := st.Pos
	grid := dungeon.Of(db, st).Map
	dungeon.Dive(db, st, "deep", 2)
	if !dungeon.Seen(db, st, "deep", st.Pos.X, st.Pos.Y) {
		t.Fatal("the start of floor 2 is not on its map")
	}
	dungeon.Dive(db, st, "deep", 1) // a new dive from camp
	if dungeon.Of(db, st).Map != grid || st.Pos != start {
		t.Fatal("floor 1 is another floor on the next dive")
	}
	if !dungeon.Seen(db, st, "deep", start.X, start.Y) {
		t.Error("floor 1 forgot its map")
	}
}

// After escaping a chasing strong enemy, it does not chase for a while (it must not catch up on the next step and force a rematch).
func TestFledRoamerLetsThePartyGo(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": strings.Replace(roamGame, `count: "2"`, `count: "1"`, 1)})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	st.Vars[dungeon.VarRoamCalm] = 0.0
	var met []data.Command
	for range 400 {
		moved, cmds := dungeon.Forward(db, st)
		if !moved {
			dungeon.Turn(st, 1)
		}
		if len(cmds) > 0 && cmds[0].Battle == "one" {
			met = cmds
			break
		}
	}
	if met == nil {
		t.Fatal("no roamer was met")
	}
	if len(met[0].OnRun) == 0 {
		t.Fatal("the roamer's battle sets nothing on_escape")
	}
	st.Vars[dungeon.VarRoamerFled] = 1.0 // what its on_escape sets
	for step := range 4 {
		r := st.Roamers[0]
		walked := false
		for range 4 {
			f := dungeon.Front(db, st)
			if f.Passable() && f.Tile == nil && f.Look != "door" && (f.X != r.X || f.Y != r.Y) {
				_, cmds := dungeon.Forward(db, st)
				if len(cmds) > 0 && cmds[0].Battle == "one" {
					t.Fatalf("%d 歩目で逃げた強敵とまた戦った", step+1)
				}
				walked = true
				break
			}
			dungeon.Turn(st, 1)
		}
		if !walked {
			t.Fatal("fixture must supply an escape route")
		}
	}
}

// TestRoamersStayOnTheirFloor: moving from a floor with roamers to another dungeon (a move,
// not a dive) leaves them behind; walking there does not meet the last floor's roamers.
func TestRoamersStayOnTheirFloor(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": roamGame + `  hall:
    name: Hall
    dungeon:
      map: |
        #####
        #S..#
        #####
      tiles:
        S: {start: true, facing: east}
`})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	if len(st.Roamers) == 0 {
		t.Fatal("no roamers on the dive")
	}
	st.Location = "hall"
	dungeon.Place(db, st, "hall", "", "")
	if len(st.Roamers) != 0 {
		t.Fatalf("the last floor's roamers came along: %+v", st.Roamers)
	}
	if moved, _ := dungeon.Forward(db, st); !moved {
		t.Fatal("could not walk the hall")
	}
}
