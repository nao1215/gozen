package dungeon_test

import (
	"os"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestSpaceStairsAndRestore(t *testing.T) {
	db, err := data.Load(os.DirFS("../../../examples/space/game"))
	if err != nil {
		t.Fatal(err)
	}
	st := testgame.State(db, 7)
	dungeon.Ensure(db, st)
	st.Pos = rpg.Pos{In: "courtyard", X: 7, Y: 10, Facing: 0}
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
		{In: "courtyard", X: 8, Y: 8, Facing: 3},
		{In: "courtyard", X: 7, Y: 8, Facing: 1},
		{In: "courtyard", Level: 1, X: 9, Y: 8, Facing: 1},
	} {
		st.Pos = p
		if moved, _ := dungeon.Forward(db, st); moved || st.Pos != p {
			t.Fatalf("unsafe sideways step from %+v", p)
		}
	}
}
