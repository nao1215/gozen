package play

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func sysScenesGame(t *testing.T, opts Options) *Game {
	t.Helper()
	opts.FS = testgame.ScenesFS(nil)
	sysShotNames = nil
	opts.NoSound = true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.Script != "" && opts.ShotDir == "" {
		opts.ShotDir = t.TempDir()
	}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

// sysScene names the kind of screen at the bottom of the stack.
func sysScene(g *Game) string {
	switch g.stack[0].(type) {
	case *atlasScene:
		return "map"
	case *stageScene:
		return "stage"
	case *dungeonScene:
		return "dungeon"
	case *field:
		return "field"
	case *title:
		return "title"
	}
	return "other"
}

// TestMoveChoosesTheScreenOfThePlace: moving between a map, a painted scene and a plain
// place shows the screen each kind of place needs, and counts the step.
func TestMoveChoosesTheScreenOfThePlace(t *testing.T) {
	g := sysScenesGame(t, Options{Seed: 1, Location: "world"})
	if sysScene(g) != "map" {
		t.Fatalf("start on %s", sysScene(g))
	}
	steps := g.st.Steps
	for _, c := range []struct{ to, kind string }{
		{"town", "map"}, {"plaza", "stage"}, {"shop", "field"}, {"town", "map"}, {"plaza", "stage"}, {"town", "map"},
	} {
		g.Move(c.to)
		if sysScene(g) != c.kind || g.st.Location != c.to {
			t.Fatalf("moved to %s: %s at %s", c.to, sysScene(g), g.st.Location)
		}
		sysFrames(t, g, 2)
	}
	if g.st.Steps != steps+6 {
		t.Fatalf("steps %d, want %d", g.st.Steps, steps+6)
	}
	// moving from a stage to another stage reuses the screen.
	g.Move("plaza")
	first := g.stack[0]
	g.Move("plaza")
	if g.stack[0] != first {
		t.Fatal("a stage was rebuilt for the same place")
	}
}

// TestSaveAndLoadBringBackTheScreenOfThePlace: a save made on a map, in a painted scene and
// in a plain place loads back to the same kind of screen at the same place.
func TestSaveAndLoadBringBackTheScreenOfThePlace(t *testing.T) {
	g := sysScenesGame(t, Options{Seed: 1, Location: "world"})
	for i, c := range []struct{ at, kind string }{{"town", "map"}, {"plaza", "stage"}, {"shop", "field"}} {
		g.Move(c.at)
		sysFrames(t, g, 2)
		g.st.Gold = 100 + i
		if err := g.save(i); err != nil {
			t.Fatal(err)
		}
	}
	g.Move("world")
	for i, c := range []struct{ at, kind string }{{"town", "map"}, {"plaza", "stage"}, {"shop", "field"}} {
		st, err := g.load(i)
		if err != nil {
			t.Fatal(err)
		}
		g.loadSavedGame(st)
		sysFrames(t, g, 3)
		if g.st.Location != c.at || sysScene(g) != c.kind || g.st.Gold != 100+i {
			t.Fatalf("slot %d: at %s on %s with %d gold", i+1, g.st.Location, sysScene(g), g.st.Gold)
		}
	}
}

// TestScriptWalkOnAMapAndInAPaintedScene: `walk NAME` takes the token to a point of a map and the
// leader to a spot of a painted scene.
func TestScriptWalkOnAMapAndInAPaintedScene(t *testing.T) {
	covSlow(t)
	g := sysScenesGame(t, Options{Seed: 1, Location: "town", Script: "wait 5\nwalk square\nwait 30\nquit"})
	if !sysFrames(t, g, 4000) {
		t.Fatal("the script did not end")
	}
	if g.st.Location != "plaza" || sysScene(g) != "stage" {
		t.Fatalf("after walking to the square: %s on %s", g.st.Location, sysScene(g))
	}
}

// TestScriptWalkToASpotRunsItsEvent: in a painted scene `walk SPOT` takes the leader to the spot and
// its event shows its line.
func TestScriptWalkToASpotRunsItsEvent(t *testing.T) {
	covSlow(t)
	g := sysScenesGame(t, Options{Seed: 1, Location: "plaza", Script: "wait 5\nwalk bench\nwait 20\nquit"})
	if !sysFrames(t, g, 6000) {
		t.Fatal("the script did not end")
	}
	found := false
	for _, n := range sysShotNames {
		if strings.HasPrefix(n, "walk_bench_w") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the bench line was not reached: %v", sysShotNames)
	}
}
