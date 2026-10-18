package dungeon

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestNumericPresenceMapLoops(t *testing.T) {
	load := func(version int, loops string) *data.Generate {
		db := testgame.Load(t, map[string]string{
			"game.yaml":      fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + testgame.Game,
			"data/maze.yaml": "locations: {maze: {dungeon: {generate: {width: 21, height: 21, start: S, rooms: 0, loops: " + loops + "}, tiles: {S: {start: true}}}}}",
		})
		return db.Locations["maze"].Dungeon.Generate
	}
	old, migrated, zero := load(0, "0"), load(1, "0.08"), load(1, "0")
	different := false
	for seed := range uint64(32) {
		makeMap := func(g *data.Generate) string {
			return Maze(g, rand.New(rand.NewPCG(seed, 1)), func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		}
		a, b, c := makeMap(old), makeMap(migrated), makeMap(zero)
		if a != b {
			t.Fatal("旧版の０を0.08へ移した地図が変わった", seed)
		}
		different = different || a != c
		// 部屋も追加ループも無い地図では、開いたマスのグラフが木になる。
		rows := strings.Split(strings.TrimSpace(c), "\n")
		vertices, edges := 0, 0
		for y, row := range rows {
			for x, ch := range row {
				if ch == '#' {
					continue
				}
				vertices++
				if x+1 < len(row) && row[x+1] != '#' {
					edges++
				}
				if y+1 < len(rows) && rows[y+1][x] != '#' {
					edges++
				}
			}
		}
		if edges != vertices-1 {
			t.Fatalf("ループ０でも閉路がある seed=%d V=%d E=%d", seed, vertices, edges)
		}
	}
	if !different {
		t.Fatal("ループ率０を指定しても地図が変わらない")
	}
}

func TestNumericPresenceRoamerDistance(t *testing.T) {
	for _, version := range []int{0, 1} {
		for _, given := range []bool{false, true} {
			far := ""
			if given {
				far = ", far: 0"
			}
			db := testgame.Load(t, map[string]string{
				"game.yaml":      fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + testgame.Game,
				"data/maze.yaml": "locations: {maze: {dungeon: {generate: {width: 15, height: 15, start: S}, tiles: {S: {start: true}}, roamers: [{picture: x.png, count: '999', troops: [{troop: one}]" + far + "}]}}}",
			})
			st := testgame.State(db, 1)
			st.Location = "maze"
			Dive(db, st, "maze", 1, true)
			d := db.Locations["maze"].Dungeon
			dist := walkDistances(d, st, st.Pos.X, st.Pos.Y)
			w, _ := d.Size()
			minimum := 999
			for _, r := range st.Roamers {
				minimum = min(minimum, dist[r.Y*w+r.X])
			}
			if len(st.Roamers) == 0 {
				t.Fatal("徘徊敵が置かれなかった")
			}
			if version == 1 && given {
				if minimum != 1 {
					t.Fatalf("明示した０でも開始地点のそばが候補にない %d", minimum)
				}
			} else if minimum < 6 {
				t.Fatalf("省略と旧版の距離補完が変わった %d", minimum)
			}
		}
	}
}

func TestNumericPresenceScaledMapRooms(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"game.yaml":      "schema_version: 1\nid: neutral\n" + testgame.Game,
		"data/maze.yaml": "locations: {maze: {dungeon: {generate: {width: 21, height: 21, size: '9', rooms: 0, loops: 0, start: S}, tiles: {S: {start: true}}}}}",
	})
	for seed := range uint64(24) {
		st := testgame.State(db, seed)
		st.Location = "maze"
		Dive(db, st, "maze", 1, true)
		d := db.Locations["maze"].Dungeon
		w, h := d.Size()
		if w != 9 || h != 9 {
			t.Fatalf("広さが変わっていない %d,%d", w, h)
		}
		if strings.Contains(d.Map, "D") {
			t.Fatalf("rooms０でも広さの式で部屋の扉ができた seed=%d", seed)
		}
	}
}
