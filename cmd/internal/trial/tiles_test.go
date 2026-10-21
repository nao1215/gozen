package trial

import (
	"strings"
	"testing"
	"testing/fstest"
)

// drawnFloorGame is a made dungeon whose second floor is drawn by hand with a door and a boss tile.
func drawnFloorGame() fstest.MapFS {
	return fstest.MapFS{
		"assets/x.png": {Data: []byte(pic)},
		"game.yaml": {Data: []byte(`schema_version: 1
id: drawn
title: Drawn
start: {location: camp, party: [scout]}
battle: {variance: 0, critical: 0, escape: 1}
`)},
		"data/a.yaml": {Data: []byte(`classes:
  walker: {name: Walker, stats: {mhp: 30, mmp: 5, atk: 6, def: 2, mat: 1, mdf: 1, agi: 5, luk: 1}, exp_for: level * 10}
actors: {scout: {name: Scout, class: walker}}
skills:
  attack: {name: Tap, effects: [{damage: 6, variance: 0}]}
  guard: {name: Wait, scope: self}
locations:
  camp:
    name: Camp
    actions: [{label: Descend, do: [{dive: tower, floor: 1}]}]
  tower:
    name: Tower
    dungeon:
      generate:
        width: 11
        height: 11
        start: S
        place: [{char: '>', far: true}]
        floors:
          2:
            map: |
              #######
              #S.G.B#
              #######
      tiles:
        S: {start: true}
        '>': {do: [{dive: tower, floor: var.floor + 1}]}
        G: {name: Gate, do: [{say: The gate creaks open.}]}
        B: {name: Boss, do: [{say: The boss waits.}]}
        U: {name: Unused, do: [{say: Never drawn.}]}
`)},
	}
}

// A tile drawn into a floor's own map is used, though no place lists it.
func TestBuildKeepsTilesDrawnInAFloorMap(t *testing.T) {
	cfg, err := Parse([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Build(drawnFloorGame(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Check(); err != nil {
		t.Fatalf("the trial copy must load: %v", err)
	}
	world := string(res.FS()["data/a.yaml"].Data)
	for _, want := range []string{"The gate creaks open.", "The boss waits."} {
		if !strings.Contains(world, want) {
			t.Errorf("a tile drawn in the map was dropped (%q):\n%s", want, world)
		}
	}
	if strings.Contains(world, "Never drawn.") {
		t.Errorf("a tile nothing draws or places is dropped:\n%s", world)
	}
}
