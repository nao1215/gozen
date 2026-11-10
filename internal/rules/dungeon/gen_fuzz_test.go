package dungeon

import (
	"math/rand/v2"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

// FuzzMaze: whatever the size, rooms, loops and pieces asked for, a made floor is walled all round, has its start once,
// has its stairs down (the one piece asked for once) once, has every open cell reachable from the start, and is the
// same for the same seed.
func FuzzMaze(f *testing.F) {
	f.Add(uint64(1), uint8(21), uint8(21), uint8(4), uint8(1), uint8(2), uint8(8), true, false)
	f.Add(uint64(7), uint8(9), uint8(9), uint8(0), uint8(1), uint8(0), uint8(0), false, true)
	f.Add(uint64(3), uint8(61), uint8(11), uint8(30), uint8(3), uint8(9), uint8(100), true, true)
	f.Add(uint64(0), uint8(0), uint8(0), uint8(255), uint8(255), uint8(255), uint8(255), false, false)
	f.Add(uint64(3726), uint8(9), uint8(9), uint8(0), uint8(0), uint8(0), uint8(0), true, false) // corridors only, every dead end filled
	f.Fuzz(func(t *testing.T, seed uint64, w, h, rooms, stairs, chests, loops uint8, far, solid bool) {
		g := &data.Generate{
			Width: int(w % 64), Height: int(h % 64), Rooms: int(rooms % 24), Start: "S", Loops: float64(loops%101) / 100,
			Place: []data.Put{{Char: ">", Far: far}, {Char: "T", Count: num(1)}},
		}
		n := 1 + int(stairs%3) + int(chests%4)
		count := func(e data.Expr) int { // the stairs once, the chests n times
			if !e.Set() {
				return 1
			}
			return n
		}
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
		if strings.Count(m, ">") != 1 {
			t.Fatalf("the stairs down must appear once:\n%s", m)
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

// FuzzDrawn: whatever is drawn, a hand-drawn floor is either refused (no map) or keeps its
// size, its walls and its start, with the stairs down at most once.
func FuzzDrawn(f *testing.F) {
	f.Add(uint64(1), "#########\n#S......#\n#.......#\n#########", false)
	f.Add(uint64(2), "#########\n#S...#..#\n#....L..#\n#....#..#\n#########", true)
	f.Add(uint64(3), "###\n#.#\n###", true)
	f.Add(uint64(4), "###\n#S.\n###", false)
	f.Add(uint64(5), "#S\n##", true)
	f.Add(uint64(6), "#####\n#S>T#\n#####", true)
	f.Fuzz(func(t *testing.T, seed uint64, m string, solid bool) {
		g := &data.Generate{Start: "S", Place: []data.Put{{Char: ">", Far: true}, {Char: "T", At: &[2]int{2, 2}}}}
		kind := func(r rune) Kind { return Kind{Solid: solid && (r == 'T' || r == 'L'), Carry: r == '>'} }
		out := Drawn(g, m, rand.New(rand.NewPCG(seed, 1)), kind, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		if out == "" {
			return
		}
		was, rows := strings.Split(strings.Trim(m, "\n"), "\n"), strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(rows) != len(was) {
			t.Fatalf("%d rows drawn, %d made:\n%s", len(was), len(rows), out)
		}
		for y := range rows {
			a, b := []rune(was[y]), []rune(rows[y])
			if len(a) != len(b) {
				t.Fatalf("row %d changed its width:\n%s", y, out)
			}
			for x := range a {
				if edge := y == 0 || x == 0 || y == len(rows)-1 || x == len(a)-1; edge && b[x] != '#' {
					t.Fatalf("the edge is open at %d,%d:\n%s", x, y, out)
				}
				if a[x] != b[x] && a[x] != '.' && a[x] != '#' && a[x] != 'D' { // a tile drawn stays where it is
					t.Fatalf("the drawn %q at %d,%d became %q:\n%s", a[x], x, y, b[x], out)
				}
			}
		}
		if strings.Count(out, "S") != strings.Count(m, "S") || strings.Count(out, ">") > max(1, strings.Count(m, ">")) {
			t.Fatalf("the start or the stairs were placed anew:\n%s", out)
		}
	})
}
