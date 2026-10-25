package gamedata_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestSpaceValidation(t *testing.T) {
	game, err := os.ReadFile("../../../examples/space/game/game.yaml")
	if err != nil {
		t.Fatal(err)
	}
	world, err := os.ReadFile("../../../examples/space/game/data/world.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fixture := fstest.MapFS{"game.yaml": {Data: game}, "data/world.yaml": {Data: world}}
	for _, name := range []string{"limestone.png", "paving.png", "sky.png"} {
		b, readErr := os.ReadFile("../../../examples/space/game/assets/dungeon/" + name) //nolint:gosec // Fixed sample asset names.
		if readErr != nil {
			t.Fatal(readErr)
		}
		fixture["assets/dungeon/"+name] = &fstest.MapFile{Data: b}
	}
	if _, err := data.Load(fixture); err != nil {
		t.Fatalf("valid fixture: %v", err)
	}
	fixture["data/world.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(string(world), "      space:", "      space:\n        step_ticks: 9\n        turn_ticks: 6", 1))}
	if _, err := data.Load(fixture); err != nil {
		t.Fatalf("valid motion durations: %v", err)
	}
	fixture["data/world.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(string(world), "start: true", "start: true, picture: dungeon/limestone.png, size: 1, solid: true", 1))}
	if _, err := data.Load(fixture); err != nil {
		t.Fatalf("upright space sprite: %v", err)
	}
	for _, tc := range []struct{ name, old, new string }{
		{"too short stair", "length: 4", "length: 2"},
		{"negative step duration", "      space:", "      space:\n        step_ticks: -1"},
		{"excessive turn duration", "      space:", "      space:\n        turn_ticks: 121"},
		{"no lower level", "level: 1", "level: 0"},
		{"too deep", "depth: 16", "depth: 17"},
		{"overlapping stairs", "stairs:", "stairs:\n          - {level: 1, x: 6, y: 10, facing: north, length: 4}"},
		{"unsupported flat sprite", "start: true", "start: true, picture: dungeon/limestone.png, flat: true"},
		{"unsupported wall sprite", "start: true", "start: true, picture: dungeon/limestone.png, look: wall"},
		{"nonfinite sprite", "start: true", "start: true, picture: dungeon/limestone.png, size: .inf"},
		{"oversized sprite", "start: true", "start: true, picture: dungeon/limestone.png, size: 17"},
		{"unsupported slide", "start: true", "start: true, slide: north"},
		{"missing sky", "sky: dungeon/sky.png", "sky: dungeon/missing.png"},
		{"legacy map conflict", "      depth: 16", "      map: S\n      depth: 16"},
		{"too long stair", "length: 4", "length: 9"},
		{"missing level", "level: 1", "level: 8"},
		{"bad direction", "facing: north, length", "facing: nowhere, length"},
		{"blocked headroom", "#________.", "#._______."},
		{"off-map stairs", "x: 6, y: 10", "x: 100, y: 10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(string(world), tc.old, tc.new, 1)
			if src == string(world) {
				t.Fatal("mutation not applied")
			}
			fixture["data/world.yaml"] = &fstest.MapFile{Data: []byte(src)}
			_, err := data.Load(fixture)
			if err == nil {
				t.Fatal("accepted invalid space")
			}
		})
	}
}

// The stair run occupies whole cells, from its entrance edge to the landing
// edge. Standing centers are half a step from either end, not on the cell edge.
func TestSpaceStandingHeights(t *testing.T) {
	s := &data.Space{Stairs: []data.SpaceStair{{Level: 2, X: 3, Y: 5, Facing: "north", Length: 4}}}
	for _, tc := range []struct {
		name        string
		level, x, y int
		want        float64
	}{
		{"floor before flight", 1, 3, 6, 1.5},
		{"first tread center", 1, 3, 5, 1.6875},
		{"last tread center", 1, 3, 2, 2.8125},
		{"upper landing", 2, 3, 1, 3},
		{"floor beside flight", 1, 4, 3, 1.5},
		{"floor below flight", 0, 3, 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := data.SpaceHeight(s, tc.level, tc.x, tc.y); got != tc.want {
				t.Fatalf("standing height = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSpaceVolumeLimits(t *testing.T) {
	prefix, _, _ := strings.Cut(testgame.Data, "    name: Camp")
	_, suffix, _ := strings.Cut(testgame.Data, "  field:\n")
	for _, tc := range []struct {
		name                  string
		side, levels, version int
		valid                 bool
	}{
		{"minimum", 3, 1, 8, true},
		{"maximum", 64, 8, 8, true},
		{"wide", 65, 1, 8, false},
		{"small", 2, 1, 8, false},
		{"too many levels", 3, 9, 8, false},
		{"empty", 3, 0, 8, false},
		{"old schema", 3, 1, 7, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := strings.Repeat(".", tc.side)
			grid := "S" + row[1:] + "\n" + strings.Repeat(row+"\n", tc.side-1)
			levels := make([]string, tc.levels)
			for i := range levels {
				levels[i] = fmt.Sprintf("%q", grid)
			}
			dungeon := "\n    dungeon:\n      tiles: {S: {start: true}}\n      space:\n        levels: [" + strings.Join(levels, ",") + "]"
			_, err := testgame.TryLoad(map[string]string{
				"game.yaml":      fmt.Sprintf("schema_version: %d\nid: space-limits\n", tc.version) + testgame.Game,
				"data/main.yaml": prefix + "    name: Camp" + dungeon + "\n  field:\n" + suffix,
			})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
