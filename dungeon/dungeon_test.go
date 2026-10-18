package dungeon_test

import (
	"context"
	"testing"

	"github.com/nao1215/gozen/dungeon"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

// 独自の探索操作で条件付きの地形と保存復元を一貫して扱う。
func TestHostWalkSaveAndResume(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/path.yaml": `locations:
  path:
    name: Path
    dungeon:
      map: |
        #######
        #S.T..#
        #######
      tiles:
        S: {start: true, facing: east}
        T: {name: Cache, once: true, when: "!flag.found", do: [{give: {item: potion}}, {set: {flag.found: true}}]}
`})
	st := testgame.State(db, 9)
	st.Location = "path"
	dungeon.Place(db, st, "path", "", "")
	before := st.ItemCount("potion", false)
	for range 2 {
		moved, cmds := dungeon.Forward(db, st)
		if !moved {
			t.Fatal("path unexpectedly blocked")
		}
		if err := headless.New(db, st).Run(context.Background(), cmds); err != nil {
			t.Fatal(err)
		}
	}
	if !st.Flags["found"] || st.ItemCount("potion", false) != before+1 || !dungeon.Spent(st, "path", 3, 1) {
		t.Fatal("one-time action was not applied")
	}
	encoded, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := rpg.Parse(db, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Pos != st.Pos || !dungeon.Seen(db, restored, "path", 3, 1) {
		t.Fatal("position or map was lost")
	}
	cell := dungeon.CellAt(dungeon.Of(db, restored), restored, 3, 1)
	if cell.Char != '.' || !cell.Passable() {
		t.Fatalf("conditional cell: %+v", cell)
	}
	dungeon.Turn(restored, 2)
	if moved, _ := dungeon.Forward(db, restored); !moved {
		t.Fatal("return blocked")
	}
	dungeon.Turn(restored, 2)
	_, cmds := dungeon.Forward(db, restored)
	if err := headless.New(db, restored).Run(context.Background(), cmds); err != nil {
		t.Fatal(err)
	}
	if restored.ItemCount("potion", false) != before+1 {
		t.Fatal("reward repeated after resume")
	}
}
