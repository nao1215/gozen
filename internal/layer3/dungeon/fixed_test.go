package dungeon_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/optional"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/testgame"
)

const fixedGame = `
locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 21
        height: 21
        rooms: 4
        size: "floor <= 2 ? 13 : 17"
        start: "@"
        seed: 7
        floors:
          2: {seed: 99}
          3:
            note: 手書きの 階
            map: |
              #########
              #@..#..>#
              #.#.#.#.#
              #.#...#.#
              #.#####.#
              #...T...#
              #.......#
              #.......#
              #########
        place:
          - {char: ">", far: true}
          - {char: T, count: "2"}
          - {char: X, count: "1"}
      tiles:
        "@": {}
        T: {name: Chest, solid: "true", once: true, after: ".", do: [{say: chest}]}
        X: {name: Trap, trigger: step, once: true, do: [{say: trap}]}
        ">": {name: Down, trigger: step, do: [{dive: deep, floor: "var.floor + 1"}]}
`

// 種を 固定すると、セーブ（状態の 種）が 違っても 同じ 地図。階ごとの 種は その 階だけ 変える。
func TestFixedSeedMakesOneMap(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": fixedGame})
	maps := map[int]map[string]bool{}
	for seed := range 5 {
		st := testgame.State(db, uint64(seed))
		st.Vars[dungeon.VarSeed] = float64(seed*31 + 5) // 前の セーブの 種
		st.Location = "deep"
		for floor := 1; floor <= 2; floor++ {
			dungeon.Dive(db, st, "deep", floor, true)
			if maps[floor] == nil {
				maps[floor] = map[string]bool{}
			}
			maps[floor][db.Locations["deep"].Dungeon.Map] = true
		}
		if v := optional.Value[float64](st.Vars[dungeon.VarSeed]); v != 7 {
			t.Errorf("the save's seed is %v, want the data's 7", v)
		}
	}
	for floor, ms := range maps {
		if len(ms) != 1 {
			t.Errorf("B%d: %d maps for 5 saves, want 1", floor, len(ms))
		}
	}
}

// 手書きの 階は 書いたまま。書いた 文字は count に 数え、足りない 分だけ 置く。
func TestDrawnFloor(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": fixedGame})
	st := testgame.State(db, 1)
	st.Location = "deep"
	dungeon.Dive(db, st, "deep", 3, true)
	d := db.Locations["deep"].Dungeon
	m := d.Map
	if got := strings.Count(m, ">"); got != 1 {
		t.Errorf("%d stairs, want the 1 drawn\n%s", got, m)
	}
	if got := strings.Count(m, "T"); got != 2 {
		t.Errorf("%d chests, want 2 (1 drawn, 1 placed)\n%s", got, m)
	}
	if got := strings.Count(m, "X"); got != 1 {
		t.Errorf("%d traps, want 1 placed\n%s", got, m)
	}
	if x, y, _ := d.Find('>'); x != 7 || y != 1 {
		t.Errorf("the drawn stairs moved to %d,%d\n%s", x, y, m)
	}
	if st.Pos.X != 1 || st.Pos.Y != 1 {
		t.Errorf("arrived at %d,%d, want the drawn start 1,1", st.Pos.X, st.Pos.Y)
	}
	w, _ := d.Size()
	if w != 9 {
		t.Errorf("width %d, want the drawn 9", w)
	}
}

// 地図が 変わった（前の セーブの 種の 地図）階では、マスの 記録（開けた 宝箱・見た マス）を 忘れる。
// 同じ 地図なら 残す。歩きつくした ほうび（complete）は 残す。
func TestOldRecordsForgottenOnANewMap(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/deep.yaml": fixedGame})
	st := testgame.State(db, 1)
	st.Location = "deep"
	st.Done["deep/1@3,3"] = true // 前の 地図で 開けた 宝箱
	st.Done["deep/1#complete"] = true
	st.Done["deep/10@3,3"] = true // ほかの 階
	dungeon.Dive(db, st, "deep", 1, true)
	if st.Done["deep/1@3,3"] {
		t.Error("a chest opened on the old map is kept on the new one")
	}
	if !st.Done["deep/1#complete"] || !st.Done["deep/10@3,3"] {
		t.Error("the complete of the floor or another floor's records were forgotten")
	}
	st.Done["deep/1@5,5"] = true
	dungeon.Dive(db, st, "deep", 2, true)
	dungeon.Dive(db, st, "deep", 1, true)
	if !st.Done["deep/1@5,5"] {
		t.Error("a record of the same map was forgotten")
	}
}

func TestIndependentGamesInParallel(t *testing.T) {
	for seed := uint64(1); seed <= 8; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			t.Parallel()
			db := testgame.Load(t, map[string]string{"data/deep.yaml": fixedGame})
			st := testgame.State(db, seed)
			st.Location = "deep"
			dungeon.Dive(db, st, "deep", 2, true)
			want := db.Locations["deep"].Dungeon.Map
			for range 3 {
				dungeon.Remake(db, st)
				if got := db.Locations["deep"].Dungeon.Map; got != want {
					t.Fatal("independent game changed map")
				}
			}
		})
	}
}
