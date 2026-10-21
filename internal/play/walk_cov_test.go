package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

const sysCorridor = `locations:
  hall:
    name: Corridor
    dungeon:
      map: |
        #########
        #E..S..F#
        #########
      tiles:
        S: {start: true, facing: east}
        E: {do: [{say: "West end."}], once: true}
        F: {do: [{say: "East end."}], once: true}
`

// TestScriptWalkReachesATileInADungeon: `walk X` in a script finds the tile in the dungeon,
// turns toward it and walks; the tile's event runs on arrival.
func TestScriptWalkReachesATileInADungeon(t *testing.T) {
	covSlow(t)
	g := sysOpen(t, Options{Seed: 1, Location: "hall", Script: "wait 5\nwalk F\nlines done\nquit"},
		map[string]string{"data/hall.yaml": sysCorridor})
	if !sysFrames(t, g, 3000) {
		t.Fatal("the script did not end")
	}
	if g.st.Pos.X != 7 || g.st.Pos.Y != 1 {
		t.Fatalf("stopped at %+v, want the east end (7,1)", g.st.Pos)
	}
	if !dungeon.Spent(g.st, "hall", 7, 1) {
		t.Fatal("the tile's once event did not run")
	}
}

// TestScriptWalkTurnsAroundToTheOtherSide: a tile behind the player is reached by turning.
func TestScriptWalkTurnsAroundToTheOtherSide(t *testing.T) {
	covSlow(t)
	g := sysOpen(t, Options{Seed: 1, Location: "hall", Script: "wait 5\nwalk E\nlines done\nquit"},
		map[string]string{"data/hall.yaml": sysCorridor})
	if !sysFrames(t, g, 3000) {
		t.Fatal("the script did not end")
	}
	if g.st.Pos.X != 1 {
		t.Fatalf("stopped at %+v, want the west end", g.st.Pos)
	}
}

// TestScriptFaceTurnsWithoutWalkingOntoTheTile: `face X` stops in front of the tile.
func TestScriptFaceTurnsWithoutWalkingOntoTheTile(t *testing.T) {
	covSlow(t)
	g := sysOpen(t, Options{Seed: 1, Location: "hall", Script: "wait 5\nface E\nwait 5\nquit"},
		map[string]string{"data/hall.yaml": sysCorridor})
	if !sysFrames(t, g, 3000) {
		t.Fatal("the script did not end")
	}
	if g.st.Pos.X != 2 {
		t.Fatalf("stopped at %+v, want next to the tile (2,1)", g.st.Pos)
	}
}

// TestScriptWalkToAMissingTileGivesUp: a tile that is not on the map ends the command with no
// move at all.
func TestScriptWalkToAMissingTileGivesUp(t *testing.T) {
	g := sysOpen(t, Options{Seed: 1, Location: "hall", Script: "wait 5\nwalk Z\nwait 5\nquit"},
		map[string]string{"data/hall.yaml": sysCorridor})
	if !sysFrames(t, g, 500) {
		t.Fatal("the script did not end")
	}
	if g.st.Pos.X != 4 {
		t.Fatalf("moved to %+v", g.st.Pos)
	}
	if g.walkArrived() {
		t.Fatal("arrived without a goal")
	}
}

// TestScriptWalkOnAFieldDoesNothing: where nothing can walk the command ends at once.
func TestScriptWalkOnAFieldDoesNothing(t *testing.T) {
	g := sysOpen(t, Options{Seed: 1, Location: "camp", Script: "wait 2\nwalk E\nwait 2\nquit"}, nil)
	if !sysFrames(t, g, 300) {
		t.Fatal("the script did not end")
	}
	if b, done := g.walkStep("E"); !done || b != 0 {
		t.Fatalf("walkStep = %v %v", b, done)
	}
}
