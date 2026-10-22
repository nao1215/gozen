package dungeon

import (
	"math"
	"math/rand/v2"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestInvalidGridSizeDoesNotAllocatePaths(t *testing.T) {
	if validSize(math.MaxInt, 2) || validSize(-1, 2) || !validSize(data.MaxDungeonSide, data.MaxDungeonSide) {
		t.Fatal("unexpected size boundary")
	}
	d := &data.Dungeon{Grid: [][]rune{make([]rune, data.MaxDungeonSide+1)}}
	if got := walkDistances(d, nil, 0, 0); got != nil {
		t.Fatalf("distances for oversized map: %d", len(got))
	}
	if walkable(d.Grid, [2]int{0, 0}, func(rune) Kind { return Kind{} }) {
		t.Fatal("oversized map reported walkable")
	}
}

func TestGenerationClampsUncheckedSizes(t *testing.T) {
	g := &data.Generate{Width: math.MaxInt, Height: -1, Rooms: -1}
	//nolint:gosec // Deterministic dungeon generation test seed, not security-sensitive data.
	grid, _ := mazeGrid(g, rand.New(rand.NewPCG(1, 2)))
	if len(grid) != 9 || len(grid[0]) != data.MaxDungeonSide {
		t.Fatalf("grid is %dx%d", len(grid[0]), len(grid))
	}
}

func TestDrawnRejectsRaggedUncheckedMap(t *testing.T) {
	//nolint:gosec // Deterministic dungeon generation test seed, not security-sensitive data.
	rng := rand.New(rand.NewPCG(1, 2))
	if got := Drawn(&data.Generate{}, "###\n##", rng, func(rune) Kind { return Kind{} }, func(data.Expr) int { return 0 }); got != "" {
		t.Fatalf("ragged map was accepted: %q", got)
	}
}
