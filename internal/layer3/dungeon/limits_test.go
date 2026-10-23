package dungeon

import (
	"math"
	"math/rand/v2"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestInvalidGridSizeDoesNotAllocatePaths(t *testing.T) {
	if cellCount(math.MaxInt, 2) != 0 || cellCount(-1, 2) != 0 || cellCount(data.MaxDungeonSide, data.MaxDungeonSide) != data.MaxDungeonSide*data.MaxDungeonSide {
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

func TestCellCountBoundsBeforeMultiplication(t *testing.T) {
	for _, side := range []int{math.MinInt, -1, 0, 1, data.MaxDungeonSide, data.MaxDungeonSide + 1, math.MaxInt} {
		for _, other := range []int{math.MinInt, -1, 0, 1, data.MaxDungeonSide, data.MaxDungeonSide + 1, math.MaxInt} {
			want := 0
			if side > 0 && side <= data.MaxDungeonSide && other > 0 && other <= data.MaxDungeonSide {
				want = side * other
			}
			if got := cellCount(side, other); got != want {
				t.Errorf("cellCount(%d, %d) = %d, want %d", side, other, got, want)
			}
		}
	}
}

func TestPathRejectsOutsideStartEvenWhenGoalIsEmpty(t *testing.T) {
	d := &data.Dungeon{Grid: [][]rune{[]rune(".")}}
	for _, pos := range []rpg.Pos{{X: -1}, {Y: -1}, {X: 1}, {Y: 1}} {
		st := &rpg.State{Pos: pos}
		if move, ok := next(d, st, data.CellNone, false); ok {
			t.Errorf("outside position %+v returned %q", pos, move)
		}
	}
}

func TestOversizedGridDoesNotChangeExploration(t *testing.T) {
	for _, tall := range []bool{false, true} {
		db := testgame.Load(t, nil)
		st := testgame.State(db, 1)
		d := &data.Dungeon{}
		if tall {
			d.Grid = make([][]rune, data.MaxDungeonSide+1)
			for i := range d.Grid {
				d.Grid[i] = []rune(".")
			}
		} else {
			d.Grid = [][]rune{make([]rune, data.MaxDungeonSide+1)}
		}
		db.Locations[st.Location].Dungeon = d
		before := len(st.Mapped)
		Reveal(db, st)
		if len(st.Mapped) != before || Shown(db, st) != nil || Seen(db, st, st.Location, 0, 0) {
			t.Fatal("oversized grid changed exploration")
		}
		if _, ok := Next(db, st, 'G'); ok {
			t.Fatal("oversized grid returned a path")
		}
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
