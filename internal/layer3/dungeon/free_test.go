package dungeon

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func freeFixture(t testing.TB, rows, config, tiles string) (*data.DB, *rpg.State) {
	t.Helper()
	prefix, _, _ := strings.Cut(testgame.Data, "    name: Camp")
	_, suffix, _ := strings.Cut(testgame.Data, "  field:\n")
	spec := fmt.Sprintf("\n    dungeon:\n      tiles: {S: {start: true, facing: east}%s}\n      space:\n        free_move: {%s}\n        levels: [%q]\n", tiles, config, rows)
	db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 8\nid: continuous-test\n" + testgame.Game, "data/main.yaml": prefix + "    name: Camp" + spec + "  field:\n" + suffix})
	st := testgame.State(db, 17)
	Ensure(db, st)
	return db, st
}

func freeContinue([]data.Command) bool { return true }

func TestFreePositionTurningAndDistance(t *testing.T) {
	db, st := freeFixture(t, "############\n#S.........#\n#..........#\n############", "speed: 1.6, acceleration: 0", "")
	var c FreeController
	for range 10 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.SpacePose == nil || math.Abs(st.SpacePose.X-(1.5+1.6/6)) > 1e-8 || st.Pos.X != 1 || st.Steps != 0 {
		t.Fatalf("off-center pose: %+v, %+v", st.SpacePose, st.Pos)
	}
	x, z := st.SpacePose.X, st.SpacePose.Z
	c.Update(db, st, FreeInput{}, freeContinue)
	for range 10 {
		c.Update(db, st, FreeInput{Turn: 1}, freeContinue)
	}
	if st.SpacePose.X != x || st.SpacePose.Z != z || st.SpacePose.Yaw == math.Pi/2 || st.Steps != 0 {
		t.Fatal("turning changed distance or retained cardinal yaw")
	}
	for range 10 {
		c.Update(db, st, FreeInput{Turn: -1}, freeContinue)
	}
	for range 50 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.Steps != 1 || math.Abs(st.SpacePose.Travel-.6) > 1e-8 {
		t.Fatalf("distance accounting: steps=%d pose=%+v", st.Steps, st.SpacePose)
	}
}

func TestFreeDiagonalsAndStages(t *testing.T) {
	db, st := freeFixture(t, "####################\n#S.................#\n#..................#\n#..................#\n####################", "speed: 1, acceleration: 0, stages: [{after: 0.4, speed: 2}, {after: 1.2, speed: 3}]", "")
	var c FreeController
	for range 24 {
		c.Update(db, st, FreeInput{Forward: 1, Strafe: 1}, freeContinue)
	}
	if math.Abs(math.Hypot(st.SpacePose.X-1.5, st.SpacePose.Z-1.5)-.4) > 1e-8 {
		t.Fatalf("diagonal speed: %+v", st.SpacePose)
	}
	if c.speed != 1 {
		t.Fatalf("stage advanced early: %v", c.speed)
	}
	c.Update(db, st, FreeInput{Forward: 1, Strafe: 1}, freeContinue)
	if c.speed != 2 {
		t.Fatalf("stage did not advance: %v", c.speed)
	}
	c.Update(db, st, FreeInput{Forward: -1, Strafe: -1}, freeContinue)
	if c.speed != 1 {
		t.Fatal("reversal retained fast stage")
	}
	c.Update(db, st, FreeInput{}, freeContinue)
	if c.speed != 0 || c.held != 0 {
		t.Fatal("release retained acceleration")
	}
}

func TestFreeStageSmoothingAndReset(t *testing.T) {
	db, st := freeFixture(t, "####################\n#S.................#\n####################", "speed: 1, acceleration: 0.2, stages: [{after: 0.4, speed: 2}]", "")
	var c FreeController
	for range 24 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if math.Abs(st.SpacePose.X-1.9) > 1e-8 {
		t.Fatalf("base speed drift: %+v", st.SpacePose)
	}
	for range 6 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if math.Abs(c.speed-1.5) > 1e-8 {
		t.Fatalf("stage interpolation: %v", c.speed)
	}
	c.Reset()
	before := st.SpacePose.X
	c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	if math.Abs(st.SpacePose.X-before-1.0/60) > 1e-8 {
		t.Fatal("reset retained stage velocity")
	}
}

func TestFreeCollisionAndTriggerTraversal(t *testing.T) {
	db, st := freeFixture(t, "############\n#S.AB.C....#\n#..........#\n############", "speed: 12, acceleration: 0", ", A: {do: [{set: {var.visited: 1}}]}, B: {do: [{set: {var.visited: 2}}]}, C: {solid: true, trigger: bump, do: [{set: {var.contact: 1}}]}")
	var c FreeController
	var entries []int
	contacts := 0
	dispatch := func(cmds []data.Command) bool {
		if _, ok := cmds[0].SetV["var.contact"]; ok {
			contacts++
		} else {
			entries = append(entries, st.Pos.X)
		}
		return true
	}
	for range 90 {
		c.Update(db, st, FreeInput{Forward: 1}, dispatch)
	}
	if fmt.Sprint(entries) != "[3 4]" || contacts != 1 || st.SpacePose.X > 6-freeRadius+1e-8 {
		t.Fatalf("entries=%v contacts=%d pose=%+v", entries, contacts, st.SpacePose)
	}
	steps := st.Steps
	for range 30 {
		c.Update(db, st, FreeInput{Forward: 1}, dispatch)
	}
	if st.Steps != steps || contacts != 1 || c.held != 0 {
		t.Fatal("wall push advanced steps, stage or bump")
	}
	c.Update(db, st, FreeInput{}, dispatch)
	c.Update(db, st, FreeInput{Forward: 1}, dispatch)
	if contacts != 2 {
		t.Fatal("release did not rearm contact")
	}
}

func TestFreeSaveAndLegacyPlacement(t *testing.T) {
	db, st := freeFixture(t, "########\n#S.....#\n#......#\n########", "speed: 1.6", "")
	var c FreeController
	for range 8 {
		c.Update(db, st, FreeInput{Forward: 1, Turn: 1}, freeContinue)
	}
	want := *st.SpacePose
	saved, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Update(db, loaded, FreeInput{}, freeContinue)
	if *loaded.SpacePose != want || c.speed != 0 {
		t.Fatalf("restore changed pose: %+v want %+v", loaded.SpacePose, want)
	}
	Place(db, loaded, "camp", "S", "east")
	c.Update(db, loaded, FreeInput{}, freeContinue)
	if loaded.SpacePose.X != 1.5 || loaded.SpacePose.Z != 1.5 || loaded.SpacePose.Yaw != math.Pi/2 {
		t.Fatal("explicit placement did not reset pose")
	}
}

func TestFreeStairsAndSideEntry(t *testing.T) {
	db, err := data.Load(os.DirFS("../../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &data.SpaceFreeMove{Speed: 2, TurnSpeed: 90}
	zero := 0.0
	cfg.Acceleration = &zero
	db.Locations["courtyard"].Dungeon.Space.FreeMove = cfg
	st := testgame.State(db, 11)
	Ensure(db, st)
	st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10, Facing: 0}
	var c FreeController
	for range 60 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.Pos.Level != 0 || math.Abs(st.SpacePose.Z-8.5) > 1e-8 {
		t.Fatalf("mid-stair: %+v %+v", st.Pos, st.SpacePose)
	}
	saved, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	st, err = rpg.Parse(db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 60 {
		c.Update(db, st, FreeInput{Forward: -1}, freeContinue)
	}
	if st.Pos.Level != 0 || math.Abs(st.SpacePose.Z-10.5) > 1e-8 {
		t.Fatalf("stair reversal: %+v", st.SpacePose)
	}
	for range 120 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.Pos.Level != 1 || math.Abs(st.SpacePose.Z-6.5) > 1e-8 {
		t.Fatalf("upper landing: %+v %+v", st.Pos, st.SpacePose)
	}
	for range 120 {
		c.Update(db, st, FreeInput{Forward: -1}, freeContinue)
	}
	if st.Pos.Level != 0 {
		t.Fatalf("descent: %+v", st.SpacePose)
	}
	st.Pos = rpg.Pos{In: "courtyard", X: 7, Y: 8, Facing: 3}
	st.SpacePose = nil
	for range 90 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.SpacePose.X < 7+freeRadius-1e-8 {
		t.Fatalf("stair side entry: %+v", st.SpacePose)
	}
	// The authored upper bridge joins the foot of the second stair from its side.
	// First-tread entry remains valid; entering halfway up the run stays blocked.
	st.Pos = rpg.Pos{In: "courtyard", Level: 1, X: 13, Y: 6, Facing: 1}
	st.SpacePose = nil
	for range 30 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if math.Abs(st.SpacePose.X-14.5) > 1e-8 {
		t.Fatalf("bridge cannot reach stair foot: %+v", st.SpacePose)
	}
}

func TestFreeLongSegmentEntriesAndInterruptions(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(strconv.FormatBool(stop), func(t *testing.T) {
			db, st := freeFixture(t, "############\n#S.AB.C....#\n############", "speed: 12", ", A: {do: [{set: {var.n: 1}}]}, B: {do: [{set: {var.n: 2}}]}, C: {do: [{set: {var.n: 3}}]}")
			ensureFreePose(db, st)
			var c FreeController
			var entries []int
			c.move(db, st, db.Locations["camp"].Dungeon, 7, 0, func([]data.Command) bool { entries = append(entries, st.Pos.X); return !stop })
			want := "[3 4 6]"
			if stop {
				want = "[3]"
			}
			if fmt.Sprint(entries) != want {
				t.Fatalf("entries %v want %s", entries, want)
			}
			if stop && st.Pos.X != 3 {
				t.Fatalf("movement continued past blocker: %+v", st.Pos)
			}
		})
	}
	db, st := freeFixture(t, "############\n#S.AB.C....#\n############", "speed: 12", ", A: {do: [{set: {var.n: 1}}]}, B: {do: [{set: {var.n: 2}}]}, C: {do: [{set: {var.n: 3}}]}")
	ensureFreePose(db, st)
	var c FreeController
	calls := 0
	c.move(db, st, db.Locations["camp"].Dungeon, 7, 0, func([]data.Command) bool { calls++; Place(db, st, "camp", "S", "east"); return true })
	if calls != 1 || st.Pos.X != 1 {
		t.Fatalf("teleport did not interrupt: calls=%d pos=%+v", calls, st.Pos)
	}
}

func TestFreeSlidingAndInteraction(t *testing.T) {
	db, st := freeFixture(t, "########\n#S#....#\n#.#....#\n#.#....#\n#......#\n########", "speed: 2, acceleration: 0", "")
	var c FreeController
	for range 60 {
		c.Update(db, st, FreeInput{Forward: 1, Strafe: 1}, freeContinue)
	}
	if st.SpacePose.X > 2-freeRadius+1e-8 || st.SpacePose.Z < 2.8 {
		t.Fatalf("wall slide: %+v", st.SpacePose)
	}
	db, st = freeFixture(t, "########\n#S.N...#\n#......#\n########", "", ", N: {solid: true, trigger: look, do: [{set: {flag.met: true}}]}")
	ensureFreePose(db, st)
	if Look(db, st) != nil {
		t.Fatal("interaction exceeded reach")
	}
	st.Pos.X = 2
	st.SpacePose = nil
	ensureFreePose(db, st)
	if Look(db, st) == nil {
		t.Fatal("reachable target was missed")
	}
	st.SpacePose.Yaw = math.Pi/4 - .02 // Hysteresis keeps east, but the ray misses N.
	if Look(db, st) != nil {
		t.Fatal("cardinal rounding selected off-ray target")
	}
	if freeFacing(math.Pi/4+.02, 0) != 0 || freeFacing(math.Pi/4+.06, 0) != 1 || freeFacing(math.Pi/4-.02, 1) != 1 {
		t.Fatal("facing hysteresis changed")
	}
	db, st = freeFixture(t, "#####\n#S#.#\n#.N.#\n#####", "", ", N: {solid: true, trigger: look, do: [{set: {flag.met: true}}]}")
	ensureFreePose(db, st)
	st.SpacePose.Yaw = 3 * math.Pi / 4
	if Look(db, st) != nil {
		t.Fatal("interaction ray leaked through a wall corner")
	}
}

func TestFreeEncounterDistanceAndAreas(t *testing.T) {
	for _, speed := range []int{1, 2, 12} {
		t.Run(strconv.Itoa(speed), func(t *testing.T) {
			db, st := freeFixture(t, "############\n#S.........#\n############", fmt.Sprintf("speed: %d, acceleration: 0", speed), "")
			db.Locations["camp"].Encounters = db.Locations["field"].Encounters
			var c FreeController
			rolls := 0
			dispatch := func(cmds []data.Command) bool {
				if cmds[0].Battle != "one" {
					t.Fatalf("unexpected commands %+v", cmds)
				}
				rolls++
				return true
			}
			for range 180 / speed {
				c.Update(db, st, FreeInput{Forward: 1}, dispatch)
			}
			if st.Steps != 3 || rolls != 3 {
				t.Fatalf("speed %d steps %d rolls %d", speed, st.Steps, rolls)
			}
			for range 60 {
				c.Update(db, st, FreeInput{Turn: 1}, dispatch)
			}
			if rolls != 3 {
				t.Fatal("turning rolled an encounter")
			}
		})
	}
	db, st := freeFixture(t, "############\n#S.........#\n############", "speed: 2, acceleration: 0", "")
	db.Locations["camp"].Encounters = db.Locations["field"].Encounters
	db.Locations["camp"].Encounters.Rate = data.Expr{Expr: testgame.Expr("pos.x >= 3 ? 1 : 0")}
	var c FreeController
	rolls := 0
	dispatch := func([]data.Command) bool { rolls++; return true }
	ensureFreePose(db, st)
	c.move(db, st, db.Locations["camp"].Dungeon, 3, 0, dispatch)
	if rolls != 1 || math.Abs(st.SpacePose.Encounter-.5) > 1e-7 {
		t.Fatalf("area travel rolls=%d residual=%v", rolls, st.SpacePose.Encounter)
	}
	for range 60 {
		c.Update(db, st, FreeInput{Forward: -1}, dispatch)
	}
	if st.SpacePose.Encounter != 0 {
		t.Fatal("leaving encounter area retained residual")
	}
}

func TestFreeFootprintBoundsAndDynamicSolids(t *testing.T) {
	db, st := freeFixture(t, "######\n#S#..#\n##...#\n#....#\n######", "speed: 12, acceleration: 0", "")
	var c FreeController
	for range 60 {
		c.Update(db, st, FreeInput{Forward: 1, Strafe: 1}, freeContinue)
	}
	if st.Pos.X != 1 || st.Pos.Y != 1 {
		t.Fatal("footprint clipped through diagonal corner")
	}
	db, st = freeFixture(t, "...\n.S.\n...", "speed: 12, acceleration: 0", "")
	for range 60 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.SpacePose.X > 3-freeRadius+1e-8 {
		t.Fatalf("crossed map boundary: %+v", st.SpacePose)
	}
	db, st = freeFixture(t, "########\n#S.AB..#\n########", "speed: 12, acceleration: 0", ", A: {do: [{set: {flag.closed: true}}]}, B: {solid: flag.closed}")
	ensureFreePose(db, st)
	c.move(db, st, db.Locations["camp"].Dungeon, 5, 0, func([]data.Command) bool { st.Flags["closed"] = true; return true })
	if st.Pos.X != 3 || st.SpacePose.X > 4-freeRadius+1e-8 {
		t.Fatal("entry event's new collider was ignored")
	}
}

func TestFreeRestoreRepairsAndStairHeadroom(t *testing.T) {
	db, st := freeFixture(t, "######\n#S...#\n######", "speed: 2, acceleration: 0", "")
	var c FreeController
	c.Update(db, st, FreeInput{}, freeContinue)
	st.SpacePose.Travel = 2
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SpacePose != nil || !loaded.LoadReport.Changed() {
		t.Fatal("invalid distance residual was restored")
	}
	c.Update(db, loaded, FreeInput{}, freeContinue)
	loaded.SpacePose.X = 1.1 // Center is in a floor cell, but the footprint overlaps the wall.
	b, err = loaded.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Update(db, loaded, FreeInput{}, freeContinue)
	if loaded.SpacePose.X != 1.5 {
		t.Fatalf("restored footprint overlaps wall: %+v", loaded.SpacePose)
	}
	db, err = data.Load(os.DirFS("../../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	db.Locations["courtyard"].Dungeon.Space.FreeMove = &data.SpaceFreeMove{Speed: 2, TurnSpeed: 90, Acceleration: &zero}
	st = testgame.State(db, 7)
	Ensure(db, st)
	st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10}
	st.Put = map[string]string{"courtyard/level/1@6,8": "."}
	for range 90 {
		c.Update(db, st, FreeInput{Forward: 1}, freeContinue)
	}
	if st.SpacePose.Z < 9+freeRadius-1e-8 {
		t.Fatalf("walked into closed stair headroom: %+v", st.SpacePose)
	}
}
