package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

// The dungeon screen (the guide for game authors): up goes ahead, left and right turn by a
// quarter, down turns round, the left and right thirds of a click turn and the middle goes
// ahead; the menu key opens the menu; a tile with do runs when stepped on or walked into; a
// once tile runs once; a solid door stops the party. The fixture's first room is a corridor
// east of S (1,1) with a column of cells going south at x=1.

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
	if x >= 0 {
		g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = x, y, facing
	}
	return g
}

func TestWldDungeonControls(t *testing.T) {
	g := wldMaze(t, "fx_maze", -1, 0, 0)
	if p := g.st.Pos; p.X != 1 || p.Y != 1 || p.Facing != 1 {
		t.Fatalf("arrived at %+v, want S facing east", p)
	}
	set := func(x, y, f int) { g.st.Pos.X, g.st.Pos.Y, g.st.Pos.Facing = x, y, f }
	t.Run("turn right and walk", func(t *testing.T) {
		wldPlay(t, g, "press right\nwait 14\n", 100)
		if g.st.Pos.Facing != 2 {
			t.Fatalf("right of east is south: facing %d", g.st.Pos.Facing)
		}
		wldPlay(t, g, "press up\nwait 14\npress up\nwait 14\n", 200)
		if p := g.st.Pos; p.X != 1 || p.Y != 3 {
			t.Fatalf("two steps south: %+v", p)
		}
	})
	t.Run("down turns round, left turns by a quarter", func(t *testing.T) {
		wldPlay(t, g, "press down\nwait 14\n", 100)
		if g.st.Pos.Facing != 0 {
			t.Fatalf("turn round: facing %d", g.st.Pos.Facing)
		}
		wldPlay(t, g, "press left\nwait 14\n", 100)
		if g.st.Pos.Facing != 3 {
			t.Fatalf("left of north is west: facing %d", g.st.Pos.Facing)
		}
	})
	t.Run("a wall stops the party", func(t *testing.T) {
		before := g.st.Pos
		wldPlay(t, g, "press up\nwait 14\n", 100)
		if g.st.Pos != before || g.st.Moved {
			t.Fatalf("bumped but moved: %+v -> %+v", before, g.st.Pos)
		}
	})
	t.Run("clicks", func(t *testing.T) {
		set(1, 1, 1)
		// the left third of the screen turns left, the right third turns right, the middle goes ahead
		wldPlay(t, g, "click 100 400\nwait 14\n", 100)
		if g.st.Pos.Facing != 0 {
			t.Fatalf("left click: facing %d", g.st.Pos.Facing)
		}
		wldPlay(t, g, "click 1100 400\nwait 14\n", 100)
		if g.st.Pos.Facing != 1 {
			t.Fatalf("right click: facing %d", g.st.Pos.Facing)
		}
		wldPlay(t, g, "click 640 400\nwait 14\n", 100)
		if g.st.Pos.X != 2 {
			t.Fatalf("middle click goes ahead: %+v", g.st.Pos)
		}
	})
	t.Run("the menu opens and closes", func(t *testing.T) {
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
	})
	t.Run("a once tile runs once and the map marks the cells seen", func(t *testing.T) {
		shown := 0
		for _, c := range g.st.Mapped[g.st.Location] {
			if c == 1 {
				shown++
			}
		}
		if shown == 0 {
			t.Fatal("standing in a dungeon marks nothing as seen")
		}
		set(1, 3, 1)
		wldPlay(t, g, "press up\nwait 40\n", 200)
		if !g.st.Flags["deep"] || len(g.st.Done) == 0 {
			t.Fatalf("the switch did not run (pos %+v, done %v)", g.st.Pos, g.st.Done)
		}
		if a := lookArea(g); a != "Deep end" {
			t.Fatalf("look area after the switch: %q", a)
		}
		g.st.Flags["deep"] = false
		set(3, 3, 3)
		wldPlay(t, g, "press up\nwait 40\n", 200)
		if g.st.Flags["deep"] {
			t.Fatal("a once tile ran a second time")
		}
	})
	t.Run("a script walks to a tile", func(t *testing.T) {
		set(8, 3, 3)
		g.st.Flags["deep"] = false
		wldPlay(t, g, "walk E\nuntil\nwait 60\n", 3000)
		if g.st.Location != "camp" {
			t.Fatalf("the exit tile did not leave the floor: %q at %+v", g.st.Location, g.st.Pos)
		}
	})
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

func TestWldDungeonDoorsBlockAndViewsDraw(t *testing.T) {
	g := wldMaze(t, "fx_maze", 7, 5, 1)
	wldPlay(t, g, "press up\nwait 20\n", 100)
	if p := g.st.Pos; p.X != 7 {
		t.Fatalf("walked through a solid door: %+v", p)
	}
	if a := lookArea(g); a != "" {
		t.Fatalf("area before the switch: %q", a)
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

// A made floor shows its number as a banner when it is entered, and a foe that hunts is always
// on the map.
func TestWldDungeonMadeFloorBannerAndHunters(t *testing.T) {
	h := wldWorld(t, "fx_hunt", Options{Calm: true, Floor: 2})
	wldFrames(t, h, 80)
	ds, ok := h.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("scene %T", h.stack[0])
	}
	f, _ := h.st.Vars[dungeon.VarFloor].(float64)
	if f != 2 || ds.shownFloor != f {
		t.Fatalf("banner floor %v, state floor %v", ds.shownFloor, f)
	}
	if len(dungeon.HuntersOn(h.db, h.st)) == 0 {
		t.Fatal("the hunting foe never appeared on its floor")
	}
}
