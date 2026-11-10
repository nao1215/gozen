package dungeon_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/dungeon"
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

func TestLoadedRoamersOutsideGridDoNotPanic(t *testing.T) {
	for _, pos := range [][2]int{{-1, 0}, {0, -1}, {15, 1}, {1, 15}, {math.MaxInt, math.MaxInt}} {
		t.Run(fmt.Sprint(pos), func(t *testing.T) {
			db := testgame.Load(t, map[string]string{"data/r.yaml": roamGame})
			st := testgame.State(db, 3)
			st.Location, st.Calm = "deep", true
			dungeon.Dive(db, st, "deep", 1)
			st.Roamers = []rpg.Roamer{{Kind: 0, Troop: "one", X: pos[0], Y: pos[1]}}
			saved, err := st.Save()
			if err != nil {
				t.Fatal(err)
			}
			restored, err := rpg.Parse(db, saved, nil)
			if err != nil {
				t.Fatal(err)
			}
			moved := false
			for range 4 {
				if ok, _ := dungeon.Forward(db, restored); ok {
					moved = true
					break
				}
				dungeon.Turn(restored, 1)
			}
			if !moved {
				t.Fatal("fixture never moved the player")
			}
			if r := restored.Roamers[0]; r.X != pos[0] || r.Y != pos[1] {
				t.Fatalf("invalid roamer moved: %+v", r)
			}
		})
	}
}

const brokenRoamers = `
locations:
  deep:
    name: Deep
    dungeon:
      generate: {width: 15, height: 15, rooms: 2, start: S, place: [{char: ">", far: true}]}
      roamers:
        - {name: Broken, picture: x.png, count: "len(floor)", troops: [{troop: one}]}
        - {name: Fickle, icon: x.png, appear: "len(floor) > 0", troops: [{troop: one}]}
        - {name: Picky, picture: x.png, troops: [{troop: one, when: "len(floor) > 0"}]}
        - {name: Ghost, picture: x.png, count: "1", move: guard, troops: [{troop: one}]}
      tiles:
        S: {start: true}
        ">": {name: Stairs, solid: "true", do: [{say: down}]}
`

// TestRoamersWhoseConditionsFailAreLeftOut: a count, an appear or a troop condition that fails when evaluated places or brings nobody,
// and never stops the dive.
func TestRoamersWhoseConditionsFailAreLeftOut(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": brokenRoamers})
	st := testgame.State(db, 2)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	for _, r := range st.Roamers {
		if r.Kind != 3 {
			t.Errorf("only the guard with a working count is placed, found kind %d", r.Kind)
		}
	}
	if len(st.Roamers) != 1 {
		t.Fatalf("roamers %+v", st.Roamers)
	}
	if cmds := dungeon.Hunters(db, st); len(cmds) != 0 {
		t.Errorf("an appear that fails brings nobody: %+v", cmds)
	}
	// a roamer that stands still stays where it was put through many steps
	at := st.Roamers[0]
	for range 60 {
		if moved, _ := dungeon.Forward(db, st); !moved {
			dungeon.Turn(st, 1)
		}
	}
	found := false
	for _, r := range st.Roamers {
		found = found || r.X == at.X && r.Y == at.Y
	}
	if !found {
		t.Errorf("a guard walked off: was at %d,%d, now %+v", at.X, at.Y, st.Roamers)
	}
}

// TestStaleRoamersAreHarmless: roamers of a kind the dungeon no longer has (an older save), or of kinds out of range,
// are not drawn as tiles and do not stop the party.
func TestStaleRoamersAreHarmless(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": brokenRoamers})
	st := testgame.State(db, 2)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	st.Roamers = []rpg.Roamer{{Kind: 99, Troop: "one", X: st.Pos.X + 1, Y: st.Pos.Y}, {Kind: -1, Troop: "one", X: st.Pos.X, Y: st.Pos.Y + 1}}
	for range 30 {
		if moved, _ := dungeon.Forward(db, st); !moved {
			dungeon.Turn(st, 1)
		}
	}
	if cmds := dungeon.Hunters(db, st); len(cmds) != 0 {
		t.Errorf("%+v", cmds)
	}
}

// A roamer that appears by condition draws its battle from its troops as one placed on the
// dive does (without meet, the troop is its battle); with no troop to draw it stays away.
func TestAppearRoamerDrawsItsTroop(t *testing.T) {
	appear := func(troops string) (*data.DB, *rpg.State) {
		db := testgame.Load(t, map[string]string{"data/r.yaml": strings.Replace(huntGame, "meet: [{say: met}, {battle: one}]", "troops: "+troops, 1)})
		st := testgame.State(db, 3)
		st.Location = "deep"
		st.Calm = true
		dungeon.Dive(db, st, "deep", 1)
		st.Vars["pent"] = 1.0
		st.Steps += 4
		dungeon.Hunters(db, st)
		return db, st
	}
	db, st := appear("[{troop: one}]")
	if len(st.Roamers) != 1 {
		t.Fatalf("roamers %+v", st.Roamers)
	}
	h := st.Roamers[0]
	c := dungeon.CellAt(dungeon.Of(db, st), st, h.X, h.Y)
	if h.Troop != "one" || c.Tile == nil || len(c.Tile.Do) != 1 || c.Tile.Do[0].Battle != "one" {
		t.Errorf("the hunter's troop is %q and meeting it gives %+v, want the battle one", h.Troop, c.Tile)
	}
	if _, st := appear(`[{troop: one, when: "false"}]`); len(st.Roamers) != 0 {
		t.Errorf("a hunter with no troop to draw came out: %+v", st.Roamers)
	}
}

// A far beyond the floor's farthest cell is the farthest cell: the search for a cell does
// not walk down from the number given (unchecked data may give any).
func TestRoamerFarBeyondTheFloor(t *testing.T) {
	for _, appear := range []bool{false, true} {
		game := roamGame
		if appear {
			game = strings.Replace(huntGame, "meet: [{say: met}, {battle: one}]", "far: 3\n          troops: [{troop: one}]", 1)
		}
		places := func(far int) string {
			db := testgame.Load(t, map[string]string{"data/r.yaml": game})
			db.Locations["deep"].Dungeon.Roamers[0].Far = far
			st := testgame.State(db, 3)
			st.Location = "deep"
			st.Calm = true
			dungeon.Dive(db, st, "deep", 1)
			st.Vars["pent"] = 1.0
			st.Steps += 4
			dungeon.Hunters(db, st)
			if len(st.Roamers) == 0 {
				t.Fatalf("appear %v, far %d: no roamer", appear, far)
			}
			return fmt.Sprint(st.Roamers)
		}
		// 15x15 cells: no walk is 500 steps, and the steps down from far keep their parity
		if near, beyond := places(500), places(math.MaxInt-1); near != beyond {
			t.Errorf("appear %v: far beyond the floor placed %s, want %s as for the farthest cell", appear, beyond, near)
		}
	}
}

// roamer_fled and roamer_beaten are variables a game's commands may set: a number that is
// no roamer's (between 0 and 1, or past the last) settles nothing.
func TestSettleRoamersIgnoresANumberOfNoRoamer(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/r.yaml": roamGame})
	st := testgame.State(db, 3)
	st.Location = "deep"
	st.Calm = true
	dungeon.Dive(db, st, "deep", 1)
	before := fmt.Sprint(st.Roamers)
	for _, name := range []string{dungeon.VarRoamerFled, dungeon.VarRoamerBeaten} {
		for _, n := range []float64{0.5, 99} {
			st.Vars[name] = n
			dungeon.SettleRoamers(st)
			if _, kept := st.Vars[name]; kept || fmt.Sprint(st.Roamers) != before {
				t.Errorf("%s = %g: roamers %v (were %s), variable kept %v", name, n, st.Roamers, before, kept)
			}
		}
	}
}
