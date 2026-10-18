package dungeon_test

import (
	"context"
	"strings"
	"testing"

	"github.com/nao1215/gozen/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

const maze = `
locations:
  maze:
    name: Maze
    encounters: {rate: "var.danger", troops: [{troop: one}]}
    dungeon:
      map: |
        #######
        #S.T..#
        #.###L#
        #..>#K#
        #######
      tiles:
        S: {start: true, facing: east}
        T: {name: Chest, picture: x.png, when: "!flag.chest", do: [{give: {item: potion}}, {set: {flag.chest: true}}]}
        K: {name: Key, when: "!flag.key", do: [{set: {flag.key: true}}]}
        L: {name: Locked, look: door, solid: "!flag.key", do: [{say: locked}]}
        ">": {name: Stairs, solid: "true", do: [{move: maze2, at: a, facing: west}]}
  maze2:
    name: Maze 2
    dungeon:
      map: |
        ####
        #<a#
        ####
      tiles:
        a: {start: true}
        "<": {name: Up, do: [{move: camp}]}
`

func setup(t *testing.T) (*data.DB, *rpg.State) {
	db := testgame.Load(t, map[string]string{"data/maze.yaml": maze})
	st := testgame.State(db, 1)
	st.Location = "maze"
	dungeon.Ensure(db, st)
	return db, st
}

func TestWalkAndTurn(t *testing.T) {
	db, st := setup(t)
	if p := st.Pos; p.X != 1 || p.Y != 1 || p.Facing != 1 || p.In != "maze" {
		t.Fatalf("start = %+v, want 1,1 facing east", p)
	}
	if moved, _ := dungeon.Forward(db, st); !moved || st.Pos.X != 2 {
		t.Fatalf("forward: %+v", st.Pos)
	}
	// the chest is stepped on
	_, cmds := dungeon.Forward(db, st)
	if len(cmds) != 2 || cmds[0].Give == nil {
		t.Fatalf("stepping on the chest: %+v", cmds)
	}
	st.Flags["chest"] = true
	if c := dungeon.CellAt(dungeon.Of(db, st), st, 3, 1); c.Char != '.' || c.Tile != nil {
		t.Errorf("the opened chest is %q, want floor", c.Char)
	}
	// a wall stops the party; turning
	dungeon.Turn(st, 1) // south: a wall
	if moved, _ := dungeon.Forward(db, st); moved {
		t.Error("walked into a wall")
	}
	dungeon.Turn(st, -2)
	if st.Pos.Facing != 0 {
		t.Errorf("facing %d, want north", st.Pos.Facing)
	}
	if !dungeon.Seen(db, st, "maze", 4, 2) || dungeon.Seen(db, st, "maze", 5, 3) {
		t.Error("the map of the cells seen is wrong")
	}
}

func TestLockedDoorAndLook(t *testing.T) {
	db, st := setup(t)
	st.Pos = rpg.Pos{In: "maze", X: 5, Y: 1, Facing: 2}
	if !dungeon.Front(db, st).Solid {
		t.Fatal("the locked door lets the party through")
	}
	if cmds := dungeon.Look(db, st); len(cmds) != 1 || cmds[0].Say == nil {
		t.Fatalf("looking at the locked door: %+v", cmds)
	}
	st.Flags["key"] = true
	if c := dungeon.Front(db, st); c.Solid || c.Look != "door" {
		t.Errorf("the unlocked door: %+v", c)
	}
}

func TestEncounterOnStep(t *testing.T) {
	db, st := setup(t)
	st.Vars["danger"] = 1.0
	_, cmds := dungeon.Forward(db, st)
	if len(cmds) != 1 || cmds[0].Battle != "one" {
		t.Fatalf("a step with rate 1 met %+v", cmds)
	}
}

// TestStairs walks with the headless driver: the shortest way to the stairs; looking
// at them moves to the next floor (at "a", facing west), where stepping on "<" goes up.
func TestStairs(t *testing.T) {
	db, st := setup(t)
	d := headless.New(db, st)
	if err := d.Walk('>'); err != nil {
		t.Fatal(err)
	}
	if p := st.Pos; st.Location != "maze2" || p.X != 2 || p.Y != 1 || p.Facing != 3 {
		t.Fatalf("after the stairs: %s %+v", st.Location, p)
	}
	if err := d.Walk('<'); err != nil || st.Location != "camp" {
		t.Fatalf("up the stairs: %v, at %s", err, st.Location)
	}
}

func TestCheckDungeon(t *testing.T) {
	bad := map[string]string{
		"unknown char": "locations: {m: {name: M, dungeon: {map: \"#S?#\", tiles: {S: {start: true}}}}}",
		"no start":     "locations: {m: {name: M, dungeon: {map: \"#..#\"}}}",
		"bad at":       "locations: {m: {name: M, dungeon: {map: \"#S#\", tiles: {S: {start: true}}}}}\nevents: {e: {do: [{move: m, at: Z}]}}",
		"people":       "locations: {m: {name: M, people: [{name: P}], dungeon: {map: \"#S#\", tiles: {S: {start: true}}}}}",
	}
	want := map[string]string{
		"unknown char": `"?" is neither`,
		"no start":     "no tile with start",
		"bad at":       `"Z" is not on the map`,
		"people":       "a dungeon has no people",
	}
	for name, src := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/z.yaml": src})
			if err == nil || !strings.Contains(err.Error(), want[name]) {
				t.Fatalf("want %q, got %v", want[name], err)
			}
		})
	}
}

// TestSaveKeepsPlaceAndMap saves in a dungeon and loads: the place, the facing and the
// map of the cells seen come back.
func TestSaveKeepsPlaceAndMap(t *testing.T) {
	db, st := setup(t)
	dungeon.Forward(db, st)
	dungeon.Turn(st, 1)
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pos != st.Pos {
		t.Errorf("pos %+v, want %+v", got.Pos, st.Pos)
	}
	dungeon.Ensure(db, got) // a loaded game keeps its place
	if got.Pos != st.Pos || !dungeon.Seen(db, got, "maze", 2, 1) || dungeon.Seen(db, got, "maze", 3, 1) {
		t.Errorf("after Ensure: %+v; the cell walked on is kept, the open one ahead is not seen", got.Pos)
	}
}

func TestCompleteOnce(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/maze.yaml": `
locations:
  hall:
    name: Hall
    dungeon:
      map: |
        ######
        #S...#
        ######
      tiles:
        S: {start: true, facing: east}
      complete: [{say: all seen}]
`})
	st := testgame.State(db, 1)
	st.Location = "hall"
	dungeon.Ensure(db, st)
	var says int
	for range 3 {
		_, cmds := dungeon.Forward(db, st)
		for _, c := range cmds {
			if c.Say != nil {
				says++
			}
		}
	}
	dungeon.Turn(st, 2)
	for range 3 {
		dungeon.Forward(db, st)
	}
	if says != 1 {
		t.Errorf("the reward came %d times, want once", says)
	}
}

func TestThroughADoor(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/maze.yaml": `
locations:
  hall:
    name: Hall
    dungeon:
      map: |
        ######
        #SD..#
        ######
      tiles:
        S: {start: true, facing: east}
`})
	st := testgame.State(db, 1)
	st.Location = "hall"
	dungeon.Ensure(db, st)
	if moved, _ := dungeon.Forward(db, st); !moved || st.Pos.X != 3 {
		t.Errorf("through the door the party is at x %d, want 3 (the cell beyond)", st.Pos.X)
	}
}

func TestCompleteAfterWarp(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/maze.yaml": `
locations:
  hall:
    name: Hall
    dungeon:
      map: |
        #####
        #S..#
        ###T#
        #####
      tiles:
        S: {start: true, facing: east}
        T: {facing: north}
      complete: [{say: all seen}]
`})
	st := testgame.State(db, 1)
	st.Location = "hall"
	dungeon.Ensure(db, st)
	says := 0
	count := func(cmds []data.Command) {
		for _, c := range cmds {
			if c.Say != nil {
				says++
			}
		}
	}
	for range 2 {
		_, cmds := dungeon.Forward(db, st)
		count(cmds)
	}
	dungeon.Place(db, st, "hall", "T", "") // the warp: the last cell is seen by being put there
	if moved, cmds := dungeon.Forward(db, st); !moved {
		t.Fatal("could not step off the warp")
	} else {
		count(cmds)
	}
	for range 2 {
		dungeon.Turn(st, 2)
		_, cmds := dungeon.Forward(db, st)
		count(cmds)
	}
	if says != 1 || !dungeon.Completed(st, "hall") {
		t.Errorf("the reward came %d times, want once", says)
	}
}

// TestBump: walking into a solid tile with do sets it off (trigger bump, the default of
// solid tiles), but only once until the party moves to another cell; the button still
// does it. A wall's tile (a sign) and trigger: look do nothing when walked into.
func TestBump(t *testing.T) {
	db, st := setup(t)
	st.Pos = rpg.Pos{In: "maze", X: 5, Y: 1, Facing: 2} // the locked door below
	moved, cmds := dungeon.Forward(db, st)
	if moved || len(cmds) != 1 || cmds[0].Say == nil {
		t.Fatalf("walking into the locked door: moved %v, %+v", moved, cmds)
	}
	if _, cmds := dungeon.Forward(db, st); cmds != nil {
		t.Errorf("walking into it again from the same cell: %+v", cmds)
	}
	if cmds := dungeon.Look(db, st); len(cmds) != 1 {
		t.Errorf("the button after the bump: %+v", cmds)
	}
	dungeon.Turn(st, 1) // west, a step away and back
	if moved, _ := dungeon.Forward(db, st); !moved {
		t.Fatal("could not step away")
	}
	dungeon.Turn(st, 2)
	if moved, _ := dungeon.Forward(db, st); !moved {
		t.Fatal("could not step back")
	}
	dungeon.Turn(st, 1) // south again
	if _, cmds := dungeon.Forward(db, st); len(cmds) != 1 {
		t.Errorf("walking into it after moving: %+v", cmds)
	}
	// the stairs (solid, no trigger) take the party down when walked into
	st.Pos = rpg.Pos{In: "maze", X: 2, Y: 3, Facing: 1}
	if _, cmds := dungeon.Forward(db, st); len(cmds) != 1 || cmds[0].Move != "maze2" {
		t.Errorf("walking into the stairs: %+v", cmds)
	}
}

// TestLookTilesAsk: a sign (look: wall) and a trigger: look tile do not go off when walked
// into: they ask (its ask, or 「調べる」, and 「やめる」). 「やめる」 does nothing (a once tile is
// not spent) and walking into it again asks again; the first answer runs its do. The
// button in front asks the same.
func TestLookTilesAsk(t *testing.T) {
	const signs = `
locations:
  hall:
    name: Hall
    dungeon:
      map: |
        ######
        #NSco#
        ##...#
        ######
      tiles:
        S: {start: true, facing: west}
        N: {name: Sign, look: wall, ask: Read, do: [{say: sign}]}
        c: {name: Crystal, solid: "true", trigger: look, do: [{say: crystal}]}
        o: {name: Orb, solid: "true", trigger: look, once: true, do: [{give: {item: potion}}]}
`
	db := testgame.Load(t, map[string]string{"data/hall.yaml": signs})
	st := testgame.State(db, 1)
	st.Location = "hall"
	dungeon.Ensure(db, st)
	asked := func(cmds []data.Command, prompt, yes string) {
		t.Helper()
		if len(cmds) != 1 || len(cmds[0].Choice) != 2 || cmds[0].Prompt != prompt ||
			cmds[0].Choice[0].Label != yes || cmds[0].Choice[1].Label != "やめる" || len(cmds[0].Choice[1].Do) != 0 {
			t.Fatalf("want the question of %s (%s / やめる): %+v", prompt, yes, cmds)
		}
	}
	st.Pos = rpg.Pos{In: "hall", X: 2, Y: 1, Facing: 3}
	moved, cmds := dungeon.Forward(db, st)
	if moved {
		t.Fatal("walked into the sign")
	}
	asked(cmds, "Sign", "Read")
	_, cmds = dungeon.Forward(db, st) // again from the same cell: asked again (not like a bump tile)
	asked(cmds, "Sign", "Read")
	asked(dungeon.Look(db, st), "Sign", "Read")
	if c := cmds[0].Choice[0].Do; len(c) != 1 || c[0].Say == nil {
		t.Errorf("reading the sign: %+v", c)
	}
	dungeon.Turn(st, 2)
	_, cmds = dungeon.Forward(db, st)
	asked(cmds, "Crystal", "調べる")
	// the once orb: 「やめる」 leaves it, the first answer spends it
	st.Pos = rpg.Pos{In: "hall", X: 4, Y: 2, Facing: 0}
	d := headless.New(db, st)
	potions := st.Items["potion"]
	_, cmds = dungeon.Forward(db, st)
	asked(cmds, "Orb", "調べる")
	d.Answers = []int{1}
	if err := d.Run(context.Background(), cmds); err != nil {
		t.Fatal(err)
	}
	if st.Items["potion"] != potions || dungeon.Spent(st, "hall", 4, 1) {
		t.Fatalf("「やめる」 spent the orb (potions %d, was %d)", st.Items["potion"], potions)
	}
	_, cmds = dungeon.Forward(db, st)
	asked(cmds, "Orb", "調べる")
	d.Answers = []int{0}
	if err := d.Run(context.Background(), cmds); err != nil {
		t.Fatal(err)
	}
	if st.Items["potion"] != potions+1 || !dungeon.Spent(st, "hall", 4, 1) {
		t.Fatalf("the first answer: potions %d (was %d), spent %v", st.Items["potion"], potions, dungeon.Spent(st, "hall", 4, 1))
	}
	if _, cmds = dungeon.Forward(db, st); cmds != nil {
		t.Errorf("the spent orb asks again: %+v", cmds)
	}
}

// 壁の 階段: look: door の solid な タイルは 向かって 進むと（ぶつかると）動き、絵は looks の pictures で 区画ごとに 替わる。
func TestStairsOnAWallAndPictures(t *testing.T) {
	const y = `
locations:
  deep:
    name: Deep
    dungeon:
      pictures: {stairs.png: stairs_a.png}
      looks:
        - {when: "flag.deep", pictures: {stairs.png: stairs_b.png}}
      map: |
        #####
        #S.>#
        #####
      tiles:
        S: {start: true, facing: east}
        ">": {name: Down, look: door, solid: "true", picture: stairs.png, do: [{move: camp}]}
`
	db := testgame.Load(t, map[string]string{"data/deep.yaml": y, "assets/stairs.png": "-", "assets/stairs_a.png": "-", "assets/stairs_b.png": "-"})
	st := testgame.State(db, 1)
	st.Location = "deep"
	dungeon.Ensure(db, st)
	pic := data.Picture{File: "stairs.png"}
	if f := dungeon.Styled(db, st).Pic(pic).File; f != "stairs_a.png" {
		t.Errorf("the dungeon's picture = %s, want stairs_a.png", f)
	}
	st.Flags["deep"] = true
	if f := dungeon.Styled(db, st).Pic(pic).File; f != "stairs_b.png" {
		t.Errorf("the look's picture = %s, want stairs_b.png", f)
	}
	if f := dungeon.Of(db, st).Pic(pic).File; f != "stairs_a.png" {
		t.Errorf("the look changed the dungeon itself: %s", f)
	}
	dungeon.Forward(db, st)
	if c := dungeon.Front(db, st); c.Look != "door" || c.Passable() {
		t.Fatalf("the stairs: look %s, passable %v", c.Look, c.Passable())
	}
	if moved, cmds := dungeon.Forward(db, st); moved || len(cmds) != 1 || cmds[0].Move != "camp" {
		t.Errorf("walking into the stairs: moved %v, %+v", moved, cmds)
	}
}

// TestPlaceInFrontOfSolid は、通れない物（壁の階段）の文字に着くと、その前のマスにそれを向いて立つことを確かめる。
func TestPlaceInFrontOfSolid(t *testing.T) {
	db, st := setup(t)
	dungeon.Place(db, st, "maze", ">", "")
	if st.Pos.X != 2 || st.Pos.Y != 3 || st.Pos.Facing != 1 {
		t.Fatalf("not in front of the stairs facing them: %+v", st.Pos)
	}
	if f := dungeon.Front(db, st); f.Char != '>' {
		t.Fatalf("facing %c", f.Char)
	}
}
