package gamedata_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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
	for _, name := range []string{"brick_wall.jpg", "brick_floor.jpg", "brick_ceiling.jpg"} {
		b, readErr := os.ReadFile("../../../examples/space/game/assets/dungeon/" + name) //nolint:gosec // Fixed sample asset names.
		if readErr != nil {
			t.Fatal(readErr)
		}
		fixture["assets/dungeon/"+name] = &fstest.MapFile{Data: b}
	}
	if _, err := data.Load(fixture); err != nil {
		t.Fatalf("valid fixture: %v", err)
	}
	for _, tc := range []struct{ name, old, new string }{
		{"legacy map conflict", "      depth: 16", "      map: S\n      depth: 16"},
		{"too long stair", "length: 4", "length: 9"},
		{"missing level", "level: 1", "level: 8"},
		{"bad direction", "facing: north, length", "facing: nowhere, length"},
		{"blocked headroom", "#._..___", "#....___"},
		{"off-map stairs", "x: 7, y: 10", "x: 100, y: 10"},
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
