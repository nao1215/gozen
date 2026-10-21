package dungeon_test

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestTriggerLookableAndAskLabel(t *testing.T) {
	cases := []struct {
		name     string
		tile     data.Tile
		solid    bool
		trigger  string
		lookable bool
	}{
		{"floor tile", data.Tile{Do: []data.Command{{Heal: "all"}}}, false, "step", false},
		{"solid tile", data.Tile{Do: []data.Command{{Heal: "all"}}}, true, "bump", true},
		{"solid tile with nothing to do", data.Tile{}, true, "bump", false},
		{"wall sign", data.Tile{Look: "wall", Do: []data.Command{{Heal: "all"}}}, true, "look", true},
		{"stated look", data.Tile{Trigger: "look", Do: []data.Command{{Heal: "all"}}}, false, "look", true},
		{"stated step on a solid", data.Tile{Trigger: "step", Do: []data.Command{{Heal: "all"}}}, true, "step", false},
	}
	for _, c := range cases {
		if got := dungeon.Trigger(&c.tile, c.solid); got != c.trigger {
			t.Errorf("%s: Trigger = %q, want %q", c.name, got, c.trigger)
		}
		if got := dungeon.Lookable(&c.tile, c.solid); got != c.lookable {
			t.Errorf("%s: Lookable = %v, want %v", c.name, got, c.lookable)
		}
	}
	if got := dungeon.AskLabel(&data.Tile{Ask: "Open the lid"}); got != "Open the lid" {
		t.Errorf("AskLabel = %q", got)
	}
	if got := dungeon.AskLabel(&data.Tile{}); got == "" {
		t.Error("a tile with no ask has a default label")
	}
}

func TestNextOutsideADungeonHasNoWay(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	if m, ok := dungeon.Next(db, st, 'G'); ok || m != "" {
		t.Errorf("%q %v", m, ok)
	}
	if moved, cmds := dungeon.Forward(db, st); moved || cmds != nil {
		t.Error("nothing to walk in a camp")
	}
	if cmds := dungeon.Look(db, st); cmds != nil {
		t.Error("nothing to look at in a camp")
	}
}

const spinGame = `
locations:
  turntable:
    name: Turntable
    dungeon:
      map: |
        #######
        #S.r..#
        #######
      tiles:
        S: {start: true, facing: east}
        r: {name: Turntable, spin: SPIN}
`

// TestSpinTilesTurnTheParty: a spin tile turns the party where it stops on it, in the way written.
func TestSpinTilesTurnTheParty(t *testing.T) {
	for _, c := range []struct {
		spin   string
		facing []int // allowed facings after (0 north, 1 east, 2 south, 3 west)
	}{
		{"right", []int{2}},
		{"left", []int{0}},
		{"back", []int{3}},
		{"random", []int{2, 3, 0}},
	} {
		db := testgame.Load(t, map[string]string{"data/t.yaml": replaceSpin(spinGame, c.spin)})
		st := testgame.State(db, 7)
		st.Location = "turntable"
		dungeon.Place(db, st, "turntable", "", "")
		for range 2 {
			if moved, _ := dungeon.Forward(db, st); !moved {
				t.Fatal("blocked")
			}
		}
		if st.Pos.X != 3 {
			t.Fatalf("%s: at %d", c.spin, st.Pos.X)
		}
		ok := false
		for _, f := range c.facing {
			ok = ok || st.Pos.Facing == f
		}
		if !ok {
			t.Errorf("%s: facing %d, want one of %v", c.spin, st.Pos.Facing, c.facing)
		}
	}
}

func replaceSpin(src, spin string) string {
	out := ""
	for i := 0; i < len(src); {
		if i+4 <= len(src) && src[i:i+4] == "SPIN" {
			out += spin
			i += 4
			continue
		}
		out += string(src[i])
		i++
	}
	return out
}
