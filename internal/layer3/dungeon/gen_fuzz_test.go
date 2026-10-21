package dungeon

import (
	"math/rand/v2"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// FuzzMaze: whatever the size, rooms, loops and pieces asked for, a made floor is walled all round, has its start once,
// has every open cell reachable from the start, and is the same for the same seed.
func FuzzMaze(f *testing.F) {
	f.Add(uint64(1), uint8(21), uint8(21), uint8(4), uint8(1), uint8(2), uint8(8), true, false)
	f.Add(uint64(7), uint8(9), uint8(9), uint8(0), uint8(1), uint8(0), uint8(0), false, true)
	f.Add(uint64(3), uint8(61), uint8(11), uint8(30), uint8(3), uint8(9), uint8(100), true, true)
	f.Add(uint64(0), uint8(0), uint8(0), uint8(255), uint8(255), uint8(255), uint8(255), false, false)
	f.Fuzz(func(t *testing.T, seed uint64, w, h, rooms, stairs, chests, loops uint8, far, solid bool) {
		g := &data.Generate{
			Width: int(w % 64), Height: int(h % 64), Rooms: int(rooms % 24), Start: "S", Loops: float64(loops%101) / 100,
			Place: []data.Put{{Char: ">", Far: far}, {Char: "T"}},
		}
		n := 1 + int(stairs%3) + int(chests%4)
		count := func(data.Expr) int { return n }
		kind := func(r rune) Kind { return Kind{Solid: solid && (r == 'T' || r == '>')} }
		m := MazeKinds(g, rand.New(rand.NewPCG(seed, 1)), kind, count)     //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		again := MazeKinds(g, rand.New(rand.NewPCG(seed, 1)), kind, count) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		if m != again {
			t.Fatal("the same seed made two floors")
		}
		rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
		if len(rows) < 9 {
			t.Fatalf("a floor is at least 9 rows, got %d", len(rows))
		}
		if strings.Count(m, "S") != 1 {
			t.Fatalf("the start must appear once:\n%s", m)
		}
		sx, sy, open := 0, 0, 0
		for y, r := range rows {
			if len([]rune(r)) != len([]rune(rows[0])) {
				t.Fatalf("rows of one width:\n%s", m)
			}
			for x, c := range []rune(r) {
				if c != '#' {
					open++
				}
				if c == 'S' {
					sx, sy = x, y
				}
				if (x == 0 || y == 0 || y == len(rows)-1 || x == len([]rune(r))-1) && c != '#' {
					t.Fatalf("the edge is open at %d,%d:\n%s", x, y, m)
				}
			}
		}
		grid := make([][]rune, len(rows))
		for i, r := range rows {
			grid[i] = []rune(r)
		}
		seen := map[[2]int]bool{{sx, sy}: true}
		q := [][2]int{{sx, sy}}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if n[0] < 0 || n[1] < 0 || n[1] >= len(grid) || n[0] >= len(grid[n[1]]) || seen[n] || grid[n[1]][n[0]] == '#' {
					continue
				}
				seen[n] = true
				q = append(q, n)
			}
		}
		if len(seen) != open {
			t.Fatalf("%d of %d open cells are reachable from the start:\n%s", len(seen), open, m)
		}
	})
}
