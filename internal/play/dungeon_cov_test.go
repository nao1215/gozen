package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

// The first floor of the sample dungeon: the party arrives at S (1,1) facing south, in a
// corridor running down the west side (docs/architecture/scenes.md is about maps and scenes;
// the dungeon's controls are in the guide for game authors: up goes ahead, left and right turn
// by a quarter, down turns round).

func wldDungeon(t *testing.T, loc string) *Game {
	t.Helper()
	g := wldSample(t, Options{Location: loc, Calm: true})
	ds, ok := g.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	screen := wldScreen(g)
	for i := 0; i < 600 && (i < 5 || ds.transT > 0 || g.busy()); i++ {
		wldStep(t, g, screen)
	}
	return g
}

func TestWldDungeonWalkAndTurn(t *testing.T) {
	g := wldDungeon(t, "b1")
	if p := g.st.Pos; p.X != 1 || p.Y != 1 || p.Facing != 2 {
		t.Fatalf("arrived at %+v", p)
	}
	wldPlay(t, g, "press up\nwait 14\npress up\nwait 14\n", 200)
	if p := g.st.Pos; p.X != 1 || p.Y != 3 {
		t.Fatalf("two steps south: %+v", p)
	}
	// down turns round; two quarter turns to the right come back
	wldPlay(t, g, "press down\nwait 14\n", 100)
	if g.st.Pos.Facing != 0 {
		t.Fatalf("turn round: facing %d", g.st.Pos.Facing)
	}
	wldPlay(t, g, "press right\nwait 14\n", 100)
	if g.st.Pos.Facing != 1 {
		t.Fatalf("right of north is east: facing %d", g.st.Pos.Facing)
	}
	wldPlay(t, g, "press left\nwait 14\npress left\nwait 14\n", 100)
	if g.st.Pos.Facing != 3 {
		t.Fatalf("two lefts from east is west: facing %d", g.st.Pos.Facing)
	}
	// walking into the wall does not move the party
	before := g.st.Pos
	wldPlay(t, g, "press up\nwait 14\n", 100)
	if g.st.Pos != before || g.st.Moved {
		t.Fatalf("bumped but moved: %+v -> %+v", before, g.st.Pos)
	}
}

func TestWldDungeonClickTurnsAndAdvances(t *testing.T) {
	g := wldDungeon(t, "b1")
	// the left third of the screen turns left, the right third turns right, the middle goes ahead
	wldPlay(t, g, "click 100 400\nwait 14\n", 100)
	if g.st.Pos.Facing != 1 {
		t.Fatalf("left click: facing %d", g.st.Pos.Facing)
	}
	wldPlay(t, g, "click 1100 400\nwait 14\n", 100)
	if g.st.Pos.Facing != 2 {
		t.Fatalf("right click: facing %d", g.st.Pos.Facing)
	}
	wldPlay(t, g, "click 640 400\nwait 14\n", 100)
	if g.st.Pos.Y != 2 {
		t.Fatalf("middle click goes ahead: %+v", g.st.Pos)
	}
}

func TestWldDungeonMenuAndBack(t *testing.T) {
	g := wldDungeon(t, "b1")
	wldPlay(t, g, "press menu\nwait 10\n", 100)
	if len(g.stack) != 2 {
		t.Fatalf("menu not open: %d scenes", len(g.stack))
	}
	if _, ok := g.stack[1].(*menu); !ok {
		t.Fatalf("scene %T", g.stack[1])
	}
	wldPlay(t, g, "press cancel\nwait 10\n", 100)
	if len(g.stack) != 1 {
		t.Fatalf("menu not closed: %d scenes", len(g.stack))
	}
}

func TestWldDungeonLeverRunsOnceAndChangesTile(t *testing.T) {
	g := wldDungeon(t, "b1")
	wldPlay(t, g, "walk L\nuntil\nwait 30\n", 3000)
	if !g.st.Flags["cell_open"] {
		t.Fatalf("the lever did not open the cell (pos %+v)", g.st.Pos)
	}
	if !g.st.Done["b1/3,1"] && len(g.st.Done) == 0 {
		t.Fatalf("a once tile left no mark: %v", g.st.Done)
	}
}

func TestWldDungeonMapMarksAndLeavingByStairs(t *testing.T) {
	g := wldDungeon(t, "b1")
	shown := 0
	for _, c := range g.st.Mapped[g.st.Location] {
		if c == 1 {
			shown++
		}
	}
	if shown == 0 {
		t.Fatal("standing in a dungeon marks nothing as seen")
	}
	wldPlay(t, g, "walk <\nuntil\nwait 60\n", 3000)
	if g.st.Location == "b1" {
		t.Fatalf("the up stairs did not leave the floor: %+v", g.st.Pos)
	}
}

func TestWldDungeonBannerOfMadeFloor(t *testing.T) {
	g := wldSample(t, Options{Location: "b3", Floor: 2, Calm: true})
	wldFrames(t, g, 60)
	ds, ok := g.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	f, _ := g.st.Vars["floor"].(float64)
	if ds.shownFloor != f || ds.shownFloor < 1 {
		t.Fatalf("banner floor %v, state floor %v", ds.shownFloor, f)
	}
}

func wldMaze(t *testing.T, loc string, x, y, facing int, calm ...bool) *Game {
	t.Helper()
	g := wldWorld(t, loc, Options{Calm: len(calm) == 0 || calm[0]})
	ds, ok := g.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	screen := wldScreen(g)
	for i := 0; i < 600 && (i < 5 || ds.transT > 0 || g.busy()); i++ {
		wldStep(t, g, screen)
	}
	g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = x, y, facing
	return g
}

// A slide tile carries the party on cell by cell until the way is shut or it comes to another
// tile; a spin tile turns it when it stops there (the Tile documentation of the schema).
func TestWldDungeonSlideStopsAtOtherTileAndSpins(t *testing.T) {
	g := wldMaze(t, "fx_maze", 4, 3, 1)
	wldPlay(t, g, "press up\nwait 60\n", 200)
	if p := g.st.Pos; p.X != 8 || p.Y != 3 {
		t.Fatalf("slid to %+v, want the spin tile at x=8", p)
	}
	if g.st.Pos.Facing != 3 {
		t.Fatalf("spin back from east should face west, got %d", g.st.Pos.Facing)
	}
}

func TestWldDungeonOnceTileAndLooks(t *testing.T) {
	g := wldMaze(t, "fx_maze", 1, 3, 1)
	if a := lookArea(g); a != "" {
		t.Fatalf("area before the switch: %q", a)
	}
	wldPlay(t, g, "press up\nwait 40\n", 200)
	if !g.st.Flags["deep"] {
		t.Fatalf("the switch did not run (pos %+v)", g.st.Pos)
	}
	if a := lookArea(g); a != "Deep end" {
		t.Fatalf("look area after the switch: %q", a)
	}
	// the same tile does not run twice
	g.st.Flags["deep"] = false
	g.st.Pos.X, g.st.Pos.Facing = 3, 3
	wldPlay(t, g, "press up\nwait 40\npress right\nwait 20\npress right\nwait 20\npress up\nwait 40\n", 400)
	if g.st.Flags["deep"] {
		t.Fatal("a once tile ran a second time")
	}
}

func TestWldDungeonDoorsBlockAndViewsDraw(t *testing.T) {
	g := wldMaze(t, "fx_maze", 7, 5, 1)
	wldPlay(t, g, "press up\nwait 20\n", 100)
	if p := g.st.Pos; p.X != 7 {
		t.Fatalf("walked through a solid door: %+v", p)
	}
	// look at the tiles from where they can be seen: a flat picture and a standing one, a painted
	// door and a decal on a wall
	for _, v := range [][3]int{{1, 1, 1}, {2, 1, 1}, {1, 5, 1}, {4, 5, 1}, {1, 3, 1}, {5, 3, 1}, {8, 5, 3}, {8, 1, 3}, {3, 1, 1}, {6, 1, 1}} {
		g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = v[0], v[1], v[2]
		wldFrames(t, g, 3)
	}
	if g.failure != nil {
		t.Fatal(g.failure)
	}
}

func TestWldDungeonGaugeAndMeters(t *testing.T) {
	g := wldMaze(t, "fx_maze", 1, 1, 1, false)
	g.st.Vars["debt"] = 5.0
	wldPlay(t, g, "press up\nwait 14\n", 300)
	if g.st.Pos.X != 2 {
		t.Fatalf("pos %+v", g.st.Pos)
	}
	// each step fills the gauge by the rate (0.05); a battle empties it
	got := dungeon.Gauge(g.st)
	if len(g.stack) > 1 {
		if got != 0 {
			t.Fatalf("a battle began but the gauge is %v", got)
		}
		return
	}
	if got != 0.05 {
		t.Fatalf("the gauge after one step: %v, want the rate 0.05", got)
	}
	// a fuller gauge is drawn in other colors; drawing it changes nothing in the rules
	for _, v := range []float64{0.5, 0.8} {
		g.st.Vars["gauge"] = v
		wldFrames(t, g, 2)
		if dungeon.Gauge(g.st) != v {
			t.Fatalf("drawing the gauge changed it: %v", dungeon.Gauge(g.st))
		}
	}
}

func TestWldDungeonNoMap(t *testing.T) {
	g := wldMaze(t, "fx_nomap", 1, 1, 1)
	if d := dungeon.Of(g.db, g.st); d == nil || !d.NoMap {
		t.Fatal("fixture should be a dungeon without a map")
	}
	wldFrames(t, g, 5)
}

func TestWldDungeonHunters(t *testing.T) {
	h := wldWorld(t, "fx_hunt", Options{Calm: true})
	wldFrames(t, h, 80)
	if len(dungeon.HuntersOn(h.db, h.st)) == 0 {
		t.Fatal("the hunting foe never appeared on its floor")
	}
}
