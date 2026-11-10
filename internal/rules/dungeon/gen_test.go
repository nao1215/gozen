package dungeon

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestAMadeFloorIsAllReachable(t *testing.T) {
	g := &data.Generate{
		Width: 21, Height: 21, Rooms: 4, Start: "S",
		Place: []data.Put{{Char: ">", Far: true}, {Char: "T"}, {Char: "T"}},
	}
	for seed := range uint64(40) {
		m := MazeKinds(g, rand.New(rand.NewPCG(seed, 1)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
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
	a := MazeKinds(g, rand.New(rand.NewPCG(5, 2)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
	b := MazeKinds(g, rand.New(rand.NewPCG(5, 2)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
	if a != b {
		t.Fatal("the same seed made two floors")
	}
}

func TestEventsAtSetPlaces(t *testing.T) {
	at := [2]int{3, 3}
	g := &data.Generate{
		Width: 21, Height: 21, Rooms: 3, Start: "S",
		Place: []data.Put{{Char: "E", At: &at}, {Char: "W", Steps: 6}, {Char: ">", Far: true}},
	}
	for seed := range uint64(20) {
		m := MazeKinds(g, rand.New(rand.NewPCG(seed, 3)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
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
		g := &data.Generate{
			Width: side, Height: side, Rooms: max(1, 5*side*side/(25*25)), Start: "S",
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
			},
		}
		for seed := range uint64(150) {
			m := MazeKinds(g, rand.New(rand.NewPCG(seed, uint64(floor)*7919+1)), func(r rune) Kind { return Kind{Solid: solid(r)} }, func(e data.Expr) int { //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
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

// Place on a wall (place's wall): the stairs entrance is a wall tile with exactly one open neighbor. far is the one whose neighbor is far away, steps is near the arrival point.
func TestPutInAWall(t *testing.T) {
	g := &data.Generate{
		Width: 21, Height: 21, Rooms: 4, Start: "S",
		Place: []data.Put{{Char: ">", Far: true, Wall: true}, {Char: "<", Wall: true, Steps: 2}, {Char: "T"}},
	}
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
				if ch == '>' && near < far*2/3 { // a spot that cannot be a wall entrance, such as the back of a room, may be the farthest
					t.Errorf("seed %d: the stairs down beside a cell %d steps away (the farthest %d):\n%s", seed, near, far, m)
				}
			}
		}
	}
}

// near: place in front of the down stairs. If the stairs are at a dead end or corridor, placing it cuts off just the stairs tile (and a few tiles between) from the rest of the floor.
func TestPutNearTheStairs(t *testing.T) {
	g := &data.Generate{
		Width: 21, Height: 21, Rooms: 4, Start: "S",
		Place: []data.Put{{Char: ">", Far: true}, {Char: "!", Near: ">"}, {Char: "T"}, {Char: "T"}},
	}
	placed, cut := 0, 0
	for seed := range uint64(60) {
		m := MazeKinds(g, rand.New(rand.NewPCG(seed, 1)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
		rows := strings.Split(strings.TrimRight(m, "\n"), "\n")
		sx, sy, ax, ay, gx, gy := 0, 0, -1, -1, -1, -1
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
		// treating the guard as a wall, the stairs are unreachable but the tile before the guard is reachable
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
	// If the stairs are at a dead end, the guard blocks the way. In a room and the like, you only stand on the tile before the stairs (you can go around; the game's descend stops it).
	if placed != 60 || cut < 20 {
		t.Errorf("the guard stood in front of the stairs on %d of 60 floors, and shut them off on %d (want 60 and 20 or more)", placed, cut)
	}
}

// A drawn floor may keep a part behind a solid tile (a locked door, solid: "!flag.key"): the
// stairs and the chest are still placed, in the part walked to, and shut nothing there.
func TestDrawnFloorWithAShutPartGetsItsPlacements(t *testing.T) {
	g := &data.Generate{Start: "S", Place: []data.Put{{Char: ">", Far: true}, {Char: "T"}}}
	kind := func(r rune) Kind {
		switch r {
		case 'L', 'T':
			return Kind{Solid: true}
		case '>':
			return Kind{Carry: true}
		}
		return Kind{}
	}
	one := func(data.Expr) int { return 1 }
	open := "#########\n#S......#\n#.......#\n#.......#\n#########"
	shut := "#########\n#S...#..#\n#....L..#\n#....#..#\n#########"
	for _, c := range []struct{ name, m string }{{"open", open}, {"shut", shut}} {
		out := Drawn(g, c.m, rand.New(rand.NewPCG(1, 1)), kind, one) //nolint:gosec // Deterministic test.
		if strings.Count(out, ">") != 1 || strings.Count(out, "T") != 1 {
			t.Errorf("%s: stairs %d, chests %d (want 1 and 1):\n%s", c.name, strings.Count(out, ">"), strings.Count(out, "T"), out)
		}
		for row := range strings.SplitSeq(out, "\n") {
			if c.name == "shut" && strings.ContainsAny(row[min(len(row), 6):], ">T") {
				t.Errorf("something is placed behind the shut door, where no way is checked:\n%s", out)
			}
		}
	}
}

// Drawn gives no map for a floor the check would refuse, instead of walking off its grid.
func TestDrawnRefusesAMapItCannotWalk(t *testing.T) {
	g := &data.Generate{Start: "S", Place: []data.Put{{Char: ">", Far: true}, {Char: "T", At: &[2]int{1, 1}}}}
	for name, m := range map[string]string{
		"no start":          "###\n#.#\n###",
		"an open edge":      "###\n#S.\n###",
		"the start on edge": "#S#\n#.#\n###",
		"two rows":          "###\n#S#",
		"two columns":       "#S\n#.\n##",
		"uneven rows":       "###\n#S##\n###",
		"nothing":           "",
	} {
		if out := Drawn(g, m, rand.New(rand.NewPCG(1, 1)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }); out != "" { //nolint:gosec // Deterministic test.
			t.Errorf("%s: want no map, got\n%s", name, out)
		}
	}
}

// A maze of corridors only (rooms: 0) could have every dead end filled, to its last cell:
// no floor to start on (seed 3726 of a 9x9 floor), or the start alone and no stairs down.
func TestMazeOfCorridorsOnlyKeepsItsFloor(t *testing.T) {
	g := &data.Generate{Width: 9, Height: 9, Start: "S", Loops: 1e-9, Place: []data.Put{{Char: ">", Far: true}}}
	for _, seed := range []uint64{3726, 14531, 0, 1, 2} {
		m := MazeKinds(g, rand.New(rand.NewPCG(seed, 1)), func(rune) Kind { return Kind{} }, func(data.Expr) int { return 1 }) //nolint:gosec // Deterministic test.
		if open := strings.Count(m, "."); strings.Count(m, "S") != 1 || strings.Count(m, ">") != 1 || open+2 < minMazeFloor {
			t.Errorf("seed %d: want a start, stairs down and %d cells of floor or more:\n%s", seed, minMazeFloor, m)
		}
	}
	// the same through checked data and a dive (rooms: 0 passes the check)
	game := strings.Replace(strings.Replace(countGame, "rooms: 4", "rooms: 0", 1), "width: 21\n        height: 21", "width: 9\n        height: 9", 1)
	db := testgame.Load(t, map[string]string{"data/deep.yaml": game})
	st := testgame.State(db, 1)
	st.Location = "deep"
	st.Vars[VarSeed] = 32682.0
	Dive(db, st, "deep", 1)
	if m := db.Locations["deep"].Dungeon.Map; strings.Count(m, ">") != 1 {
		t.Errorf("dive_seed 32682: no stairs down:\n%s", m)
	}
}

const countGame = `
locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 21
        height: 21
        rooms: 4
        start: S
        place:
          - {char: ">", far: true}
          - {char: T, count: "1 + floor"}
      tiles:
        S: {start: true}
        T: {name: Chest, solid: "true", once: true, after: ".", do: [{say: chest}]}
        ">": {name: Stairs, solid: "true", do: [{say: down}]}
`

// A floor is made again from its seed when a save is loaded, so what shapes it reads the
// floor's number only: a count or a size of the state (which the check refuses) would make
// another map, and the chests opened and the cells seen on the floor would be forgotten.
func TestAFloorMadeAgainIsTheSameMap(t *testing.T) {
	for _, field := range []string{"count", "size"} {
		db := testgame.Load(t, map[string]string{"data/deep.yaml": countGame})
		g := db.Locations["deep"].Dungeon.Generate
		of := func(src string) data.Expr {
			p, err := expr.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			return data.Expr{Expr: p}
		}
		if field == "count" { // unchecked data: put past the check
			g.Place[1].Count = of("1 + var.luck + rand(6)")
		} else {
			g.Size = of("9 + 2 * var.luck + 2 * rand(4)")
		}
		st := testgame.State(db, 5)
		st.Location = "deep"
		Dive(db, st, "deep", 1)
		want := db.Locations["deep"].Dungeon.Map
		st.Done["deep/1@3,3"] = true // a chest opened on this floor
		st.Vars["luck"] = 2.0        // the game goes on
		for range 8 {                // a save loaded
			Remake(db, st)
			if got := db.Locations["deep"].Dungeon.Map; got != want {
				t.Fatalf("%s: the floor made again is another map:\n%s\nwas\n%s", field, got, want)
			}
		}
		if !st.Done["deep/1@3,3"] {
			t.Errorf("%s: the opened chest was forgotten", field)
		}
	}
}

// The seed is a number of the save: its bits are the same on every platform.
func TestSeedBits(t *testing.T) {
	for seed, want := range map[float64]uint64{
		0:                  0,
		32682.9:            32682,
		4294967295:         4294967295,
		-1:                 math.MaxUint64,
		-5.5:               math.MaxUint64 - 4,
		math.Inf(-1):       1 << 63,
		-1e300:             1 << 63,
		math.Inf(1):        math.MaxUint64,
		1e300:              math.MaxUint64,
		1 << 63:            1 << 63,
		math.MaxUint32 + 1: 1 << 32,
	} {
		if got := seedBits(seed); got != want {
			t.Errorf("seedBits(%g) = %d, want %d", seed, got, want)
		}
	}
	if got := seedBits(math.NaN()); got != 0 {
		t.Errorf("seedBits(NaN) = %d, want 0", got)
	}
}
