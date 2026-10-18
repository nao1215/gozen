package dungeon

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func TestAMadeFloorIsAllReachable(t *testing.T) {
	g := &data.Generate{Width: 21, Height: 21, Rooms: 4, Start: "S",
		Place: []data.Put{{Char: ">", Far: true}, {Char: "T"}, {Char: "T"}}}
	for seed := range uint64(40) {
		m := Maze(g, rand.New(rand.NewPCG(seed, 1)), func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
		var sx, sy int
		open := 0
		for y, r := range rows {
			for x, c := range r {
				if c != '#' {
					open++
				}
				if c == 'S' {
					sx, sy = x, y
				}
			}
		}
		if strings.Count(m, "S") != 1 || strings.Count(m, ">") != 1 || strings.Count(m, "T") != 2 {
			t.Fatalf("seed %d: the tiles are not all placed:\n%s", seed, m)
		}
		seen := map[[2]int]bool{{sx, sy}: true}
		q := [][2]int{{sx, sy}}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if !seen[n] && rows[n[1]][n[0]] != '#' {
					seen[n] = true
					q = append(q, n)
				}
			}
		}
		if len(seen) != open {
			t.Fatalf("seed %d: %d of %d open cells reached:\n%s", seed, len(seen), open, m)
		}
		for x := range rows[0] {
			if rows[0][x] != '#' || rows[len(rows)-1][x] != '#' {
				t.Fatalf("seed %d: the edge is open", seed)
			}
		}
	}
	// the same seed makes the same floor
	a := Maze(g, rand.New(rand.NewPCG(5, 2)), func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
	b := Maze(g, rand.New(rand.NewPCG(5, 2)), func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
	if a != b {
		t.Fatal("the same seed made two floors")
	}
}

func TestEventsAtSetPlaces(t *testing.T) {
	at := [2]int{3, 3}
	g := &data.Generate{Width: 21, Height: 21, Rooms: 3, Start: "S",
		Place: []data.Put{{Char: "E", At: &at}, {Char: "W", Steps: 6}, {Char: ">", Far: true}}}
	for seed := range uint64(20) {
		m := Maze(g, rand.New(rand.NewPCG(seed, 3)), func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
		if rows[3][3] != 'E' && rows[3][3] != 'S' {
			t.Fatalf("seed %d: no E at (3,3):\n%s", seed, m)
		}
		if strings.Count(m, "W") != 1 {
			t.Fatalf("seed %d: no W placed:\n%s", seed, m)
		}
		// everything reachable still (the cell at was joined)
		var sx, sy, open int
		for y, r := range rows {
			for x, c := range r {
				if c != '#' {
					open++
				}
				if c == 'S' {
					sx, sy = x, y
				}
			}
		}
		seen := map[[2]int]bool{{sx, sy}: true}
		q := [][2]int{{sx, sy}}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if !seen[n] && rows[n[1]][n[0]] != '#' {
					seen[n] = true
					q = append(q, n)
				}
			}
		}
		if len(seen) != open {
			t.Fatalf("seed %d: %d of %d reached:\n%s", seed, len(seen), open, m)
		}
	}
}

func TestSolidThingsNeverShutTheWay(t *testing.T) {
	solidSet := "TPGRZf"
	solid := func(r rune) bool { return strings.ContainsRune(solidSet, r) }
	corner := [2]int{1, 1}
	for floor := 1; floor <= 20; floor++ {
		side := 13
		switch {
		case floor > 15:
			side = 21
		case floor > 10:
			side = 19
		case floor > 5:
			side = 17
		case floor > 2:
			side = 15
		}
		g := &data.Generate{Width: side, Height: side, Rooms: max(1, 5*side*side/(25*25)), Start: "S",
			Place: []data.Put{
				{Char: ">", Far: true},
				{Char: "T", Count: num(2 + floor/4)},
				{Char: "P", Count: num(b2i(floor%3 == 0))},
				{Char: "G", Count: num(b2i(floor%5 == 0))},
				{Char: "H", Count: num(b2i(floor == 7)), At: &corner},
				{Char: "R", Count: num(1), Steps: 9},
				{Char: "e", Count: num(2 + floor/8)},
				{Char: "Z", Count: num(b2i(floor >= 3))},
				{Char: "f", Count: num(b2i(floor >= 11 && floor <= 15)), Far: true},
				{Char: "X", Count: num(1 + floor/4)},
				{Char: "y", Count: num(b2i(floor >= 4)), Far: true},
			}}
		for seed := range uint64(150) {
			m := MazeSolid(g, rand.New(rand.NewPCG(seed, uint64(floor)*7919+1)), solid, func(e data.Expr) int { //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
				if !e.Set() {
					return 1
				}
				n, err := e.Number(expr.Map{})
				if err != nil {
					t.Fatal(err)
				}
				return int(n)
			})
			rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
			if strings.Count(m, ">") != 1 {
				t.Fatalf("floor %d seed %d: no stairs down:\n%s", floor, seed, m)
			}
			if floor == 7 && strings.Count(m, "H") != 1 {
				t.Fatalf("floor %d seed %d: no H:\n%s", floor, seed, m)
			}
			var sx, sy int
			for y, r := range rows {
				if x := strings.IndexRune(r, 'S'); x >= 0 {
					sx, sy = x, y
				}
			}
			seen := map[[2]int]bool{{sx, sy}: true}
			q := [][2]int{{sx, sy}}
			for len(q) > 0 {
				c := q[0]
				q = q[1:]
				for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
					n := [2]int{c[0] + d[0], c[1] + d[1]}
					if r := rune(rows[n[1]][n[0]]); !seen[n] && r != '#' && !solid(r) {
						seen[n] = true
						q = append(q, n)
					}
				}
			}
			for y, r := range rows {
				for x, c := range r {
					if c == '#' {
						continue
					}
					ok := seen[[2]int{x, y}]
					if solid(c) {
						ok = seen[[2]int{x - 1, y}] || seen[[2]int{x + 1, y}] || seen[[2]int{x, y - 1}] || seen[[2]int{x, y + 1}]
					}
					if !ok {
						t.Fatalf("floor %d seed %d: %c at (%d,%d) cannot be walked to:\n%s", floor, seed, c, x, y, m)
					}
				}
			}
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func num(n int) data.Expr {
	p, err := expr.Parse(strconv.Itoa(n))
	if err != nil {
		panic(err)
	}
	return data.Expr{Expr: p}
}

// 壁に 置く（place の wall）: 階段の 入口は 壁の マスで、開いた 隣が ちょうど 1 つ。far は その 隣が 遠い 物、steps は 着いた 所の 近く。
func TestPutInAWall(t *testing.T) {
	g := &data.Generate{Width: 21, Height: 21, Rooms: 4, Start: "S",
		Place: []data.Put{{Char: ">", Far: true, Wall: true}, {Char: "<", Wall: true, Steps: 2}, {Char: "T"}}}
	kind := func(r rune) Kind { return Kind{Solid: r == '>' || r == '<' || r == 'T'} }
	for seed := range uint64(40) {
		m := MazeKinds(g, rand.New(rand.NewPCG(seed, 1)), kind, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
		dist := map[[2]int]int{}
		var q [][2]int
		for y, r := range rows {
			if x := strings.IndexRune(r, 'S'); x >= 0 {
				dist[[2]int{x, y}] = 0
				q = append(q, [2]int{x, y})
			}
		}
		far := 0
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			far = max(far, dist[c])
			for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if _, ok := dist[n]; !ok && !strings.ContainsRune("#<>T", rune(rows[n[1]][n[0]])) {
					dist[n] = dist[c] + 1
					q = append(q, n)
				}
			}
		}
		for _, ch := range "<>" {
			if strings.Count(m, string(ch)) != 1 {
				t.Fatalf("seed %d: %q placed %d times:\n%s", seed, ch, strings.Count(m, string(ch)), m)
			}
			for y, r := range rows {
				x := strings.IndexRune(r, ch)
				if x < 0 {
					continue
				}
				if x == 0 || y == 0 || x == len(r)-1 || y == len(rows)-1 {
					t.Fatalf("seed %d: %q on the edge:\n%s", seed, ch, m)
				}
				open, near := 0, -1
				for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
					n := [2]int{x + d[0], y + d[1]}
					if rows[n[1]][n[0]] != '#' {
						open++
						if v, ok := dist[n]; ok {
							near = v
						}
					}
				}
				if open != 1 || near < 0 {
					t.Fatalf("seed %d: %q at %d,%d has %d open cells beside it (want 1, walked to):\n%s", seed, ch, x, y, open, m)
				}
				if ch == '>' && near < far*2/3 { // 部屋の 奥など 壁の 入口に できない 所が いちばん 遠い ことも ある
					t.Errorf("seed %d: the stairs down beside a cell %d steps away (the farthest %d):\n%s", seed, near, far, m)
				}
			}
		}
	}
}

// near: 下り階段の前に置く。階段が行き止まりや廊下にあれば、置くと階段のマスだけが（と間の数マスが）残りの床から切れる。
func TestPutNearTheStairs(t *testing.T) {
	g := &data.Generate{Width: 21, Height: 21, Rooms: 4, Start: "S",
		Place: []data.Put{{Char: ">", Far: true}, {Char: "!", Near: ">"}, {Char: "T"}, {Char: "T"}}}
	placed, cut := 0, 0
	for seed := range uint64(60) {
		m := Maze(g, rand.New(rand.NewPCG(seed, 1)), func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
		var sx, sy, ax, ay, gx, gy = 0, 0, -1, -1, -1, -1
		for y, r := range rows {
			for x, c := range r {
				switch c {
				case 'S':
					sx, sy = x, y
				case '>':
					ax, ay = x, y
				case '!':
					gx, gy = x, y
				}
			}
		}
		if gx < 0 {
			t.Fatalf("seed %d: no guard:\n%s", seed, m)
		}
		placed++
		if abs(gx-ax)+abs(gy-ay) != 1 {
			t.Fatalf("seed %d: the guard (%d,%d) is not beside the stairs (%d,%d):\n%s", seed, gx, gy, ax, ay, m)
		}
		// 番を壁と見ると階段へは行けず、番の手前までは行ける
		seen := map[[2]int]bool{{sx, sy}: true}
		q := [][2]int{{sx, sy}}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if !seen[n] && rows[n[1]][n[0]] != '#' && rows[n[1]][n[0]] != '!' {
					seen[n] = true
					q = append(q, n)
				}
			}
		}
		if !seen[[2]int{ax, ay}] {
			cut++
		}
		beside := false
		for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
			beside = beside || seen[[2]int{gx + d[0], gy + d[1]}]
		}
		if !beside {
			t.Fatalf("seed %d: the guard cannot be come up to:\n%s", seed, m)
		}
		if strings.Count(m, "T") != 2 {
			t.Fatalf("seed %d: the chests are not all placed:\n%s", seed, m)
		}
	}
	// 階段が行き止まりにあれば番が道を塞ぐ。部屋の中などでは、階段の手前のマスに立つだけ（回り込める。ゲームの descend が止める）
	if placed != 60 || cut < 20 {
		t.Errorf("the guard stood in front of the stairs on %d of 60 floors, and shut them off on %d (want 60 and 20 or more)", placed, cut)
	}
}
