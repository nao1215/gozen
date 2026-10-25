package dungeon_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestSpacePositionLevelExpression(t *testing.T) {
	db, err := data.Load(os.DirFS("../../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	level, err := expr.Parse("pos.level")
	if err != nil {
		t.Fatal(err)
	}
	st := testgame.State(db, 7)
	check := func(want int) {
		t.Helper()
		got, err := level.Number(st.Env())
		if err != nil || got != float64(want) {
			t.Fatalf("pos.level = %v, %v; want %d", got, err, want)
		}
		saved, err := st.Save()
		if err != nil {
			t.Fatal(err)
		}
		st, err = rpg.Parse(db, saved, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err = level.Number(st.Env())
		if err != nil || got != float64(want) {
			t.Fatalf("restored pos.level = %v, %v; want %d", got, err, want)
		}
	}
	check(0) // The default position's save omits level, as older saves did.
	st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10, Facing: 0}
	walk := func(steps int) {
		t.Helper()
		for range steps {
			if moved, _ := dungeon.Forward(db, st); !moved {
				t.Fatalf("blocked at %+v", st.Pos)
			}
		}
	}
	walk(4)
	check(1)
	dungeon.Turn(st, 1)
	walk(8)
	dungeon.Turn(st, 1)
	walk(4)
	check(2)
	dungeon.Turn(st, 2)
	walk(4)
	check(1)
	dungeon.Turn(st, -1)
	walk(8)
	dungeon.Turn(st, -1)
	walk(4)
	check(0)
}

func TestSpaceStairsAndRestore(t *testing.T) {
	db, err := data.Load(os.DirFS("../../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	st := testgame.State(db, 7)
	dungeon.Ensure(db, st)
	st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10, Facing: 0}
	for n := 1; n <= 4; n++ {
		moved, cmds := dungeon.Forward(db, st)
		if !moved || len(cmds) > 0 || st.Pos.Y != 10-n {
			t.Fatalf("step %d: %+v, %v", n, st.Pos, cmds)
		}
		want := 0
		if n == 4 {
			want = 1
		}
		if st.Pos.Level != want {
			t.Fatalf("level %d want %d", st.Pos.Level, want)
		}
		saved, e := st.Save()
		if e != nil {
			t.Fatal(e)
		}
		restored, e := rpg.Parse(db, saved, nil)
		if e != nil {
			t.Fatal(e)
		}
		if restored.Pos != st.Pos {
			t.Fatalf("restore: %+v != %+v", restored.Pos, st.Pos)
		}
		dungeon.Ensure(db, restored)
		if restored.Pos != st.Pos {
			t.Fatal("ensure moved a saved stair position")
		}
	}
	dungeon.Turn(st, 2)
	for n := 1; n <= 4; n++ {
		moved, _ := dungeon.Forward(db, st)
		if !moved || st.Pos.Level != 0 || st.Pos.Y != 6+n {
			t.Fatalf("down %d: %+v", n, st.Pos)
		}
	}
	if len(st.Mapped["courtyard/level/0"]) == 0 || len(st.Mapped["courtyard/level/1"]) == 0 {
		t.Fatal("level exploration is not separated")
	}
	// A stair's side is not an entrance, and the atrium cannot be walked across.
	for _, p := range []rpg.Pos{
		{In: "courtyard", X: 7, Y: 8, Facing: 3},
		{In: "courtyard", X: 6, Y: 6, Facing: 2}, // Under the upper landing cannot enter the high end.
		{In: "courtyard", X: 6, Y: 8, Facing: 1},
		{In: "courtyard", Level: 1, X: 9, Y: 6, Facing: 2},
	} {
		st.Pos = p
		if moved, _ := dungeon.Forward(db, st); moved || st.Pos != p {
			t.Fatalf("unsafe sideways step from %+v", p)
		}
	}
}

func TestSpaceAllDirections(t *testing.T) {
	prefix, _, _ := strings.Cut(testgame.Data, "    name: Camp")
	_, suffix, _ := strings.Cut(testgame.Data, "  field:\n")
	for facing, name := range []string{"north", "east", "south", "west"} {
		for _, length := range []int{3, 8} {
			t.Run(fmt.Sprintf("%s/%d", name, length), func(t *testing.T) {
				grid := make([][]byte, 20)
				upper := make([][]byte, 20)
				for y := range grid {
					grid[y] = []byte(strings.Repeat(".", 20))
					upper[y] = []byte(strings.Repeat("_", 20))
				}
				grid[1][1] = 'S'
				stair := data.SpaceStair{X: 9, Y: 9, Length: length, Facing: name}
				dx, dy := data.StairDelta(stair)
				upper[9+length*dy][9+length*dx] = '.'
				rows := func(g [][]byte) string {
					var out strings.Builder
					for _, r := range g {
						out.Write(r)
						out.WriteByte('\n')
					}
					return out.String()
				}
				spec := fmt.Sprintf("\n    dungeon:\n      tiles: {S: {start: true}}\n      space:\n        levels: [%q, %q]\n        stairs: [{level: 1, x: 9, y: 9, facing: %s, length: %d}]", rows(grid), rows(upper), name, length)
				db := testgame.Load(t, map[string]string{"game.yaml": "schema_version: 8\nid: space-directions\n" + testgame.Game, "data/main.yaml": prefix + "    name: Camp" + spec + "\n  field:\n" + suffix})
				st := testgame.State(db, 7)
				start := rpg.Pos{In: "camp", X: 9, Y: 9, Facing: facing}
				for _, steps := range []int{1, length - 1, length} {
					st.Pos = start
					for range steps {
						if moved, _ := dungeon.Forward(db, st); !moved {
							t.Fatalf("ascending: %+v", st.Pos)
						}
					}
					wantLevel := 0
					if steps == length {
						wantLevel = 1
					}
					if st.Pos.Level != wantLevel {
						t.Fatalf("level=%d", st.Pos.Level)
					}
					dungeon.Turn(st, 2)
					for range steps {
						if moved, _ := dungeon.Forward(db, st); !moved {
							t.Fatalf("descending: %+v", st.Pos)
						}
					}
					dungeon.Turn(st, 2)
					if st.Pos != start {
						t.Fatalf("roundtrip: %+v != %+v", st.Pos, start)
					}
				}
				st.Pos = rpg.Pos{In: "camp", Level: 9, X: 200, Y: 200}
				dungeon.Ensure(db, st)
				if st.Pos.Level != 0 || st.Pos.X != 1 || st.Pos.Y != 1 {
					t.Fatalf("invalid save recovery: %+v", st.Pos)
				}
			})
		}
	}
}

func TestSpacePlaceDefaultEntrance(t *testing.T) {
	db, err := data.Load(os.DirFS("../../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	st := testgame.State(db, 7)
	st.Pos = rpg.Pos{In: "courtyard", Level: 1, X: 6, Y: 6}
	dungeon.Place(db, st, "courtyard", "", "")
	if st.Pos.Level != 0 || st.Pos.X != 10 || st.Pos.Y != 15 {
		t.Fatalf("default entrance: %+v", st.Pos)
	}
}
