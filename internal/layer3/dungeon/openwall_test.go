package dungeon_test

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

const madeOpenWall = `
locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 17
        height: 17
        rooms: 3
        start: "@"
        place:
          - {char: T, count: "1"}
      tiles:
        "@": {}
        T: {name: Chest, solid: "true", once: true, after: ".", do: [{say: chest}]}
        "}": {name: Down, look: door, solid: "true", symbol: "↓", do: [{say: down}]}
      complete:
        - open_wall: "}"
`

func TestOpenWallPutsTheTileInAWallBesideTheParty(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": madeOpenWall})
	for seed := range 20 {
		st := testgame.State(db, uint64(seed))
		st.Vars[dungeon.VarSeed] = float64(seed*31 + 5)
		st.Location = "deep"
		dungeon.Dive(db, st, "deep", 2, true)
		d := dungeon.Of(db, st)
		if !dungeon.OpenWall(db, st, "}") {
			t.Fatalf("seed %d: no wall opened", seed)
		}
		front := dungeon.Front(db, st)
		if front.Char != '}' || !front.Solid || front.Look != "door" {
			t.Fatalf("seed %d: the party does not face the opened wall: %+v", seed, front)
		}
		if d.At(front.X, front.Y) != data.CellWall {
			t.Errorf("seed %d: opened a cell that was not a wall of the map", seed)
		}
		// 階を 離れて 戻っても 残り、セーブにも 残る。dive の at で その前に 着く
		dungeon.Dive(db, st, "deep", 3, true)
		if c := dungeon.CellAt(dungeon.Of(db, st), st, front.X, front.Y); c.Char == '}' && dungeon.Of(db, st).At(front.X, front.Y) == data.CellWall {
			// 別の 階の 同じ 座標に 出ては いけない
			t.Errorf("seed %d: the opened wall shows on another floor", seed)
		}
		b, err := st.Save()
		if err != nil {
			t.Fatal(err)
		}
		st2, err := rpg.Parse(db, b, nil)
		if err != nil {
			t.Fatal(err)
		}
		dungeon.DiveAt(db, st2, "deep", 2, "}", "")
		if f := dungeon.Front(db, st2); f.Char != '}' || f.X != front.X || f.Y != front.Y {
			t.Errorf("seed %d: dive at the opened wall arrived in front of %+v, want %d,%d", seed, f, front.X, front.Y)
		}
	}
}

// 隣に 壁が 無い（広間の 真ん中）ときは、いちばん 近い 床に 面した 壁の 前へ 行って それを 向く。
func TestOpenWallInTheMiddleOfAHall(t *testing.T) {
	const hall = `
locations:
  hall:
    name: Hall
    dungeon:
      map: |
        #######
        #.....#
        #.....#
        #..@..#
        #.....#
        #######
      tiles:
        "@": {start: true}
        "}": {name: Down, look: door, solid: "true", do: [{say: down}]}
      complete:
        - open_wall: "}"
`
	db := testgame.Load(t, map[string]string{"data/hall.yaml": hall})
	st := testgame.State(db, 1)
	st.Location = "hall"
	dungeon.Place(db, st, "hall", "", "north")
	if !dungeon.OpenWall(db, st, "}") {
		t.Fatal("no wall opened")
	}
	if st.Pos.X != 3 || st.Pos.Y != 4 {
		t.Errorf("the party is at %d,%d, want 3,4 (the nearest cell beside a wall)", st.Pos.X, st.Pos.Y)
	}
	if f := dungeon.Front(db, st); f.Char != '}' || f.Y != 5 {
		t.Errorf("the party faces %+v, want the opened wall", f)
	}
}

func TestCompleteComesLaterWhenLeftBeforeIt(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": madeOpenWall})
	st := testgame.State(db, 3)
	st.Vars[dungeon.VarSeed] = 41.0
	st.Location = "deep"
	dungeon.Dive(db, st, "deep", 2, true)
	walkWhole(db, st) // 見ただけ（一歩も歩いていない）
	if dungeon.Completed(st, "deep") {
		t.Fatal("completed without a step")
	}
	dungeon.Dive(db, st, "deep", 2, true) // 潜り直す
	if dungeon.Completed(st, "deep") {
		t.Fatal("arriving marked the floor completed without its complete")
	}
	for range 4 {
		if moved, cmds := dungeon.Forward(db, st); moved {
			if len(cmds) == 0 || cmds[0].OpenWall != "}" {
				t.Fatalf("the first step on a floor seen whole does not give its complete: %+v", cmds)
			}
			return
		}
		dungeon.Turn(st, 1)
	}
	t.Fatal("could not step")
}
