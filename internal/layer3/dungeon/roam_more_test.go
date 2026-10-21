package dungeon_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

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
