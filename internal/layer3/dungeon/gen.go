package dungeon

import (
	"hash/fnv"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/rpg"
)

// The floors made anew (data.Generate): a dive makes the floor's map from a seed kept in the
// state (so a saved game makes the same floor again), and forgets what was seen and opened
// on the floor before.

// The state's variables of a made dungeon.
const (
	VarFloor = "floor"     // the floor's number (1 the first)
	VarSeed  = "dive_seed" // the seed of the dive
	VarDeep  = "deepest"   // the deepest floor reached
)

// Dive makes floor floor of the dungeon loc (a new dive when the seed is to change) and
// puts the party at its start.
func Dive(db *data.DB, st *rpg.State, loc string, floor int, newSeed bool) {
	DiveAt(db, st, loc, floor, "", "")
}

// DiveAt is Dive arriving at the first cell with the character at ("" for the start tile)
// facing facing (see Place: a solid thing such as the stairs down in a wall is arrived in
// front of). The floor keeps its map and what was done on it, as on every dive.
func DiveAt(db *data.DB, st *rpg.State, loc string, floor int, at, facing string) {
	l := db.Locations[loc]
	if l == nil || l.Dungeon == nil || l.Dungeon.Generate == nil {
		Place(db, st, loc, "", "")
		return
	}
	if st.Vars[VarSeed] == nil {
		st.Vars[VarSeed] = float64(st.Rand.Uint32())
	}
	fixSeed(l.Dungeon.Generate, st)
	floor = max(1, floor)
	st.Vars[VarFloor] = float64(floor)
	if d := optional.Value[float64](st.Vars[VarDeep]); float64(floor) > d {
		st.Vars[VarDeep] = float64(floor)
	}
	Make(db, st, loc)
	st.Vars["gauge"] = 0.0
	delete(st.Vars, VarRoamerBeaten)
	delete(st.Vars, VarRoamerFled)
	if at == "" {
		at = l.Dungeon.Generate.Start
	}
	Place(db, st, loc, at, facing)
	placeRoamers(db, st, l.Dungeon)
}

// Make makes the map of the floor the state is on (after loading a saved game, too): the
// floor's hand-drawn map with the rest placed on it, or a maze made from the seed (the
// data's when it fixes one, else the save's).
func Make(db *data.DB, st *rpg.State, loc string) {
	d := db.Locations[loc].Dungeon
	g := d.Generate
	seed := optional.Value[float64](st.Vars[VarSeed])
	floor := optional.Value[float64](st.Vars[VarFloor])
	s := uint64(seed)
	if fs, ok := g.SeedFor(int(floor)); ok {
		s = fs
	}
	rng := rand.New(rand.NewPCG(s, uint64(floor)*7919+1)) //nolint:gosec // Deterministic gameplay randomness, not security-sensitive data.
	env := expr.Chain{expr.Map{"floor": floor}, st.Env()}
	kind := func(r rune) Kind {
		t := d.Tiles[string(r)]
		if t == nil {
			return Kind{}
		}
		solid := t.Solid.Set() || t.Look == "wall" // a tile that may stand in the way (a chest, a crystal, a hut)
		return Kind{
			Solid: solid,
			Slide: t.Slide,
			Carry: !solid && Trigger(t, false) == "step" && carries(db, t.Do, map[string]bool{}),
		}
	}
	count := func(e data.Expr) int {
		if !e.Set() {
			return 1
		}
		n, err := e.Number(env)
		if err != nil {
			return 1
		}
		return int(n)
	}
	if m := g.Drawn(int(floor)); m != "" {
		d.SetMap(Drawn(g, m, rng, kind, count))
		keepRecords(st, loc, d.Map)
		return
	}
	if g.Size.Set() { // the floor's own size: small floors first
		if n, err := g.Size.Number(env); err == nil {
			side := int(n) | 1
			gg := *g
			gg.Width, gg.Height = max(9, min(g.Width, side)), max(9, min(g.Height, side))
			gg.Rooms = max(1, g.Rooms*gg.Width*gg.Height/(g.Width*g.Height))
			if db.Game.SchemaVersion == data.CurrentSchemaVersion && g.Rooms == 0 {
				gg.Rooms = 0
			}
			g = &gg
		}
	}
	d.SetMap(MazeKinds(g, rng, kind, count))
	keepRecords(st, loc, d.Map)
}

// fixSeed puts the data's seed in the state when the data fixes it (a save from before
// keeps its own no longer: its maps are made anew, see keepRecords).
func fixSeed(g *data.Generate, st *rpg.State) {
	if g != nil && g.Seed != nil {
		st.Vars[VarSeed] = float64(*g.Seed)
	}
}

// keepRecords forgets what was seen and opened on the floor when its map is not the one
// they were kept for (a save from before the seed was fixed or the floor drawn by hand):
// a chest opened at a cell is no chest of the new map. What the floor gave once walked
// whole (its complete) is kept.
func keepRecords(st *rpg.State, loc, m string) {
	if st.Done == nil {
		st.Done = map[string]bool{}
	}
	key := floorKey(st, loc)
	h := fnv.New64a()
	h.Write([]byte(m))
	id := key + "#map=" + strconv.FormatUint(h.Sum64(), 36)
	if st.Done[id] {
		return
	}
	for k := range st.Done {
		if strings.HasPrefix(k, key+"@") || strings.HasPrefix(k, key+"#map=") {
			delete(st.Done, k)
		}
	}
	delete(st.Mapped, key)
	for k := range st.Put { // the walls opened on the old map
		if strings.HasPrefix(k, key+"@") {
			delete(st.Put, k)
		}
	}
	st.Done[id] = true
}

// Remake makes the map again if the party stands in a made dungeon whose map is not its
// floor's (a game loaded).
func Remake(db *data.DB, st *rpg.State) {
	l := db.Locations[st.Location]
	if l == nil || l.Dungeon == nil || l.Dungeon.Generate == nil || st.Vars[VarSeed] == nil {
		return
	}
	fixSeed(l.Dungeon.Generate, st)
	Make(db, st, st.Location)
}

func fmtF(v any) string {
	f := optional.Value[float64](v)
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Maze is a floor's map: rooms joined by a winding maze of corridors (a grid of odd cells,
// walls between), some walls opened to make loops, the dead ends mostly filled back, the
// start in a room and the other characters placed on the floor (far ones as far as can be
// walked from the start).
func Maze(g *data.Generate, rng *rand.Rand, count func(data.Expr) int) string {
	return MazeSolid(g, rng, nil, count)
}

// MazeSolid is Maze knowing which characters cannot be walked through (solid, nil for none):
// one is never put where it would shut a way (the stairs down, the far ones, any open cell
// would stay walkable from the start), and the ones put anywhere go to dead ends first.
func MazeSolid(g *data.Generate, rng *rand.Rand, solid func(rune) bool, count func(data.Expr) int) string {
	if solid == nil {
		solid = func(rune) bool { return false }
	}
	return MazeKinds(g, rng, func(r rune) Kind { return Kind{Solid: solid(r)} }, count)
}

// Kind is how a character of a made map is walked (for placing it where it shuts no way).
type Kind struct {
	Solid bool   // cannot be walked into (a chest, a hut)
	Slide string // carries the party on (the tile's slide: north..west or facing)
	Carry bool   // stepping on it takes the party elsewhere (a warp, a pit): not walked through
}

// MazeKinds is MazeSolid knowing more of the characters (kind): slides are placed only where
// every open cell can still be walked to from the start and back, and so are the tiles that
// carry the party away (a pit is not put in the only way on).
func MazeKinds(g *data.Generate, rng *rand.Rand, kind func(rune) Kind, count func(data.Expr) int) string {
	grid, rooms := mazeGrid(g, rng)
	return placeOn(g, grid, rooms, rng, kind, count, false)
}

// room is a room of a made maze (its floor's top left and size).
type room struct{ x, y, w, h int }

// mazeGrid is a maze of rooms and corridors (walls #, floor ., doors D) and its rooms.
func mazeGrid(g *data.Generate, rng *rand.Rand) ([][]rune, []room) {
	w, h := g.Width|1, g.Height|1
	w, h = max(w, 9), max(h, 9)
	grid := make([][]rune, h)
	for y := range grid {
		grid[y] = []rune(strings.Repeat("#", w))
	}
	open := func(x, y int) { grid[y][x] = '.' }
	isWall := func(x, y int) bool { return x > 0 && y > 0 && x < w-1 && y < h-1 && grid[y][x] == '#' }
	// rooms
	var rooms []room
	for range g.Rooms * 6 {
		if len(rooms) >= g.Rooms {
			break
		}
		rw, rh := 3+2*rng.IntN(2), 3+2*rng.IntN(2)
		rx, ry := 1+2*rng.IntN(max(1, (w-rw-1)/2)), 1+2*rng.IntN(max(1, (h-rh-1)/2))
		if rx+rw >= w-1 || ry+rh >= h-1 {
			continue
		}
		ok := true
		for _, o := range rooms {
			if rx-1 < o.x+o.w && rx+rw+1 > o.x && ry-1 < o.y+o.h && ry+rh+1 > o.y {
				ok = false
			}
		}
		if !ok {
			continue
		}
		rooms = append(rooms, room{rx, ry, rw, rh})
		for y := ry; y < ry+rh; y++ {
			for x := rx; x < rx+rw; x++ {
				open(x, y)
			}
		}
	}
	// the maze in what is left (growing tree from every unopened odd cell)
	dirs := [][2]int{{0, -2}, {2, 0}, {0, 2}, {-2, 0}}
	for sy := 1; sy < h-1; sy += 2 {
		for sx := 1; sx < w-1; sx += 2 {
			if grid[sy][sx] != '#' {
				continue
			}
			open(sx, sy)
			stack := [][2]int{{sx, sy}}
			for len(stack) > 0 {
				c := stack[len(stack)-1]
				var next [][2]int
				for _, d := range dirs {
					nx, ny := c[0]+d[0], c[1]+d[1]
					if isWall(nx, ny) {
						next = append(next, [2]int{nx, ny})
					}
				}
				if len(next) == 0 {
					stack = stack[:len(stack)-1]
					continue
				}
				n := next[rng.IntN(len(next))]
				open((c[0]+n[0])/2, (c[1]+n[1])/2)
				open(n[0], n[1])
				stack = append(stack, n)
			}
		}
	}
	// the rooms joined to the maze: a door or two in their walls
	for _, r := range rooms {
		var spots [][2]int
		for x := r.x; x < r.x+r.w; x += 2 {
			spots = append(spots, [2]int{x, r.y - 1}, [2]int{x, r.y + r.h})
		}
		for y := r.y; y < r.y+r.h; y += 2 {
			spots = append(spots, [2]int{r.x - 1, y}, [2]int{r.x + r.w, y})
		}
		rng.Shuffle(len(spots), func(i, j int) { spots[i], spots[j] = spots[j], spots[i] })
		n := 0
		for _, s := range spots {
			if n >= 1+rng.IntN(2) {
				break
			}
			if isWall(s[0], s[1]) {
				grid[s[1]][s[0]] = 'D'
				n++
			}
		}
	}
	// loops: a few walls between two corridors opened
	loops := g.LoopRate()
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			if grid[y][x] != '#' || rng.Float64() >= loops {
				continue
			}
			if (grid[y][x-1] == '.' && grid[y][x+1] == '.' && grid[y-1][x] == '#' && grid[y+1][x] == '#') ||
				(grid[y-1][x] == '.' && grid[y+1][x] == '.' && grid[y][x-1] == '#' && grid[y][x+1] == '#') {
				open(x, y)
			}
		}
	}
	// dead ends mostly filled back (a few kept: something may wait in them)
	for range 3 {
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				if grid[y][x] != '.' || rng.Float64() < 0.35 {
					continue
				}
				n := 0
				for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
					if grid[y+d[1]][x+d[0]] != '#' {
						n++
					}
				}
				if n <= 1 { // a dead end (or a cell left alone by filling its neighbour)
					grid[y][x] = '#'
				}
			}
		}
	}
	for shut := true; shut; {
		shut = false
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				if grid[y][x] != data.CellDoor {
					continue
				}
				n := 0
				for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
					if grid[y+d[1]][x+d[0]] != '#' {
						n++
					}
				}
				if n <= 1 {
					grid[y][x] = '#'
					shut = true
				}
			}
		}
	}
	return grid, rooms
}

// Drawn is a hand-drawn floor (m: rows of # . D and tile characters, the start drawn once)
// with what place asks for placed on it as on a made floor: a character drawn counts
// toward its count, and only the rest are placed.
func Drawn(g *data.Generate, m string, rng *rand.Rand, kind func(rune) Kind, count func(data.Expr) int) string {
	var grid [][]rune
	for _, ln := range strings.Split(strings.Trim(m, "\n"), "\n") {
		grid = append(grid, []rune(ln))
	}
	return placeOn(g, grid, nil, rng, kind, count, true)
}

// placeOn puts the start and the characters of place on the floor of grid (drawn: a
// hand-drawn floor, its start and other characters already there).
func placeOn(g *data.Generate, grid [][]rune, rooms []room, rng *rand.Rand, kind func(rune) Kind, count func(data.Expr) int, drawn bool) string {
	h, w := len(grid), len(grid[0])
	// what is drawn already counts toward place's count
	have := map[rune]int{}
	if drawn {
		for _, row := range grid {
			for _, r := range row {
				have[r]++
			}
		}
	}
	need := make([]int, len(g.Place))
	for i, p := range g.Place {
		ch := []rune(p.Char)[0]
		n := count(p.Count)
		used := min(max(0, n), have[ch])
		have[ch] -= used
		need[i] = n - used
	}
	// the cells asked for by place (at): opened and joined to the nearest open cell
	for i, p := range g.Place {
		if p.At == nil || need[i] <= 0 {
			continue
		}
		x, y := min(max(1, p.At[0]), w-2), min(max(1, p.At[1]), h-2)
		if grid[y][x] == '#' {
			grid[y][x] = '.'
			// the way through the walls to the nearest open cell
			type c2 = [2]int
			prev := map[c2]c2{{x, y}: {x, y}}
			q := []c2{{x, y}}
			var hit *c2
			for len(q) > 0 && hit == nil {
				c := q[0]
				q = q[1:]
				for _, dd := range []c2{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
					n := c2{c[0] + dd[0], c[1] + dd[1]}
					if n[0] < 1 || n[1] < 1 || n[0] > w-2 || n[1] > h-2 {
						continue
					}
					if _, seen := prev[n]; seen {
						continue
					}
					prev[n] = c
					if grid[n[1]][n[0]] != '#' {
						hit = &n
						break
					}
					q = append(q, n)
				}
			}
			if hit != nil {
				for c := prev[*hit]; c != (c2{x, y}); c = prev[c] {
					grid[c[1]][c[0]] = '.'
				}
			}
		}
	}
	// the start: in a room (or anywhere open)
	var floor [][2]int
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			if grid[y][x] == '.' {
				floor = append(floor, [2]int{x, y})
			}
		}
	}
	var start [2]int
	if drawn {
		for y, row := range grid {
			for x, r := range row {
				if string(r) == g.Start {
					start = [2]int{x, y}
				}
			}
		}
	} else {
		start = floor[rng.IntN(len(floor))]
		if len(rooms) > 0 {
			r := rooms[rng.IntN(len(rooms))]
			start = [2]int{r.x + r.w/2, r.y + r.h/2}
		}
	}
	// how far each cell is walked from the start
	dist := map[[2]int]int{start: 0}
	q := [][2]int{start}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for _, d := range [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
			n := [2]int{c[0] + d[0], c[1] + d[1]}
			if _, seen := dist[n]; seen || grid[n[1]][n[0]] == '#' {
				continue
			}
			dist[n] = dist[c] + 1
			q = append(q, n)
		}
	}
	for y := 1; y < h-1 && !drawn; y++ { // what cannot be walked to from the start is wall (a drawn floor is as drawn)
		for x := 1; x < w-1; x++ {
			if _, ok := dist[[2]int{x, y}]; !ok && grid[y][x] != '#' {
				grid[y][x] = '#'
			}
		}
	}
	var reach [][2]int
	for _, c := range floor {
		if _, ok := dist[c]; ok && c != start {
			reach = append(reach, c)
		}
	}
	rng.Shuffle(len(reach), func(i, j int) { reach[i], reach[j] = reach[j], reach[i] })
	taken := map[[2]int]bool{start: true}
	for y, row := range grid { // a drawn floor's characters stay where they are
		for x, r := range row {
			if drawn && r != '#' && r != '.' && r != data.CellDoor {
				taken[[2]int{x, y}] = true
			}
		}
	}
	if g.Start != "" && !drawn {
		grid[start[1]][start[0]] = []rune(g.Start)[0]
	}
	// blocks says whether putting ch at c would shut a way: some open cell no longer walkable
	// from the start, a solid thing no longer reachable from a side, or (with slides and
	// carrying tiles) a cell from which the start can no longer be walked back to.
	steps4 := [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	var guards [][2]int // what near put in front of the stairs: not a wall to the rest (the party beats it)
	blocks := func(c [2]int, ch rune) bool {
		was := grid[c[1]][c[0]]
		grid[c[1]][c[0]] = ch
		defer func() { grid[c[1]][c[0]] = was }()
		for _, g := range guards {
			if g != c {
				old := grid[g[1]][g[0]]
				grid[g[1]][g[0]] = '.'
				defer func() { grid[g[1]][g[0]] = old }()
			}
		}
		return !walkable(grid, start, kind)
	}
	checked := func(ch rune) bool { k := kind(ch); return k.Solid || k.Slide != "" || k.Carry }
	byDoor := func(c [2]int) bool {
		for _, d := range steps4 {
			if grid[c[1]+d[1]][c[0]+d[0]] == data.CellDoor {
				return true
			}
		}
		return false
	}
	// fits says whether ch may go at c (free, not just inside a door when solid, and not shutting a way)
	fits := func(c [2]int, ch rune) bool {
		return !taken[c] && (!kind(ch).Solid || !byDoor(c)) && (!checked(ch) || !blocks(c, ch))
	}
	deadEnd := func(c [2]int) bool {
		n := 0
		for _, d := range steps4 {
			if grid[c[1]+d[1]][c[0]+d[0]] != '#' {
				n++
			}
		}
		return n <= 1
	}
	for i, p := range g.Place {
		ch := []rune(p.Char)[0]
		if p.At != nil {
			if need[i] > 0 {
				c := [2]int{min(max(1, p.At[0]), w-2), min(max(1, p.At[1]), h-2)}
				if !fits(c, ch) { // taken (the stairs down, say): the nearest free cell
					best, bd := [2]int{-1, -1}, -1
					for _, o := range reach {
						if d := abs(o[0]-c[0]) + abs(o[1]-c[1]); fits(o, ch) && (best[0] < 0 || d < bd) {
							best, bd = o, d
						}
					}
					c = best
				}
				if c[0] >= 0 {
					taken[c] = true
					grid[c[1]][c[0]] = ch
				}
			}
			continue
		}
		if p.Near != "" {
			for range need[i] {
				c, ok := nearSpot(grid, start, taken, []rune(p.Near)[0], kind)
				if ok {
					guards = append(guards, c)
				}
				if !ok {
					break
				}
				taken[c] = true
				grid[c[1]][c[0]] = ch
			}
			continue
		}
		if p.Wall {
			for range need[i] {
				c, ok := wallSpot(grid, dist, taken, kind, rng, p)
				if !ok {
					break
				}
				taken[c] = true
				grid[c[1]][c[0]] = ch
			}
			continue
		}
		for range need[i] {
			best, bd := [2]int{-1, -1}, -1
			for pass := 0; pass < 3 && best[0] < 0; pass++ {
				for _, c := range reach {
					if taken[c] {
						continue
					}
					if p.Steps > 0 { // the nearest to that many steps
						if d := abs(dist[c] - p.Steps); (best[0] < 0 || d < bd) && fits(c, ch) {
							best, bd = c, d
						}
						continue
					}
					if p.Far {
						if dist[c] > bd && fits(c, ch) {
							best, bd = c, dist[c]
						}
						continue
					}
					// not right by the start; a solid thing in a dead end first (pass 0)
					// (pass 2: a crowded floor, then nearer the start too)
					if (dist[c] >= 3 || pass == 2 && dist[c] >= 1) && (pass >= 1 || !kind(ch).Solid || deadEnd(c)) && fits(c, ch) {
						best = c
						break
					}
				}
				if p.Steps > 0 || p.Far {
					break
				}
			}
			if best[0] < 0 {
				break
			}
			taken[best] = true
			grid[best[1]][best[0]] = ch
		}
	}
	var b strings.Builder
	for _, row := range grid {
		b.WriteString(string(row))
		b.WriteByte('\n')
	}
	return b.String()
}

// nearSpot is the cell that stands in front of the cell of near (the stairs down, say): the
// nearest open cell, within a few steps of it, that cuts it off from the start when put solid
// (the way into a dead end; a corridor's cell when it is not in a dead end), else the free cell
// beside it nearest to the start. Not taken, not a
// door, and only a few cells (and walls' solid things) go with it: they are marked taken, so
// that nothing is put behind the guard.
func nearSpot(grid [][]rune, start [2]int, taken map[[2]int]bool, near rune, kind func(rune) Kind) ([2]int, bool) {
	var s [2]int
	found := false
	for y, row := range grid {
		for x, r := range row {
			if r == near && !found {
				s, found = [2]int{x, y}, true
			}
		}
	}
	if !found {
		return [2]int{}, false
	}
	dirs := [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	reach := func() map[[2]int]bool {
		seen := map[[2]int]bool{start: true}
		q := [][2]int{start}
		for len(q) > 0 {
			c := q[0]
			q = q[1:]
			for _, d := range dirs {
				n := [2]int{c[0] + d[0], c[1] + d[1]}
				if n[1] < 0 || n[1] >= len(grid) || n[0] < 0 || n[0] >= len(grid[0]) || seen[n] || grid[n[1]][n[0]] == '#' {
					continue
				}
				seen[n] = true
				q = append(q, n)
			}
		}
		return seen
	}
	before := reach()
	// the cells round the stairs, nearest first
	order := [][2]int{}
	dist := map[[2]int]int{s: 0}
	for q := [][2]int{s}; len(q) > 0; {
		c := q[0]
		q = q[1:]
		for _, d := range dirs {
			n := [2]int{c[0] + d[0], c[1] + d[1]}
			if _, ok := dist[n]; ok || grid[n[1]][n[0]] == '#' || dist[c] >= 6 {
				continue
			}
			dist[n] = dist[c] + 1
			order = append(order, n)
			q = append(q, n)
		}
	}
	for _, n := range order {
		if taken[n] || n == start || grid[n[1]][n[0]] != '.' {
			continue
		}
		grid[n[1]][n[0]] = '#'
		after := reach()
		grid[n[1]][n[0]] = '.'
		if after[s] {
			continue // the stairs can be come to another way
		}
		var pocket [][2]int
		for c := range before {
			if !after[c] && c != n && c != s && !kind(grid[c[1]][c[0]]).Solid {
				pocket = append(pocket, c)
			}
		}
		if len(pocket) <= 5 { // a few cells between it and the stairs go with it
			for _, c := range pocket {
				taken[c] = true // nothing is put behind the guard
			}
			return n, true
		}
	}
	// stairs in a room or in a loop (no one cell cuts them off): the free cell beside them on
	// the side the start is nearer, standing in front of them (the party can still walk round it)
	from := map[[2]int]int{start: 0}
	for q := [][2]int{start}; len(q) > 0; {
		c := q[0]
		q = q[1:]
		for _, d := range dirs {
			n := [2]int{c[0] + d[0], c[1] + d[1]}
			if _, ok := from[n]; ok || grid[n[1]][n[0]] == '#' {
				continue
			}
			from[n] = from[c] + 1
			q = append(q, n)
		}
	}
	best, bd := [2]int{-1, -1}, 1<<30
	for _, d := range dirs {
		n := [2]int{s[0] + d[0], s[1] + d[1]}
		if dd, ok := from[n]; ok && !taken[n] && n != start && grid[n[1]][n[0]] == '.' && dd < bd {
			best, bd = n, dd
		}
	}
	return best, best[0] >= 0
}

// wallSpot is a wall cell to put p in (place's wall: stairs drawn on a wall): inside the
// border, not taken, beside exactly one open cell walked to from the start (not a door, a
// solid thing, a slide or a carrying tile). The last cell of a dead end will do as well (it
// is walled in the same way, and the farthest cells are often there). The end of a straight dead end comes first (seen
// down the corridor), then any wall of a corridor or a room. With far the open cell beside
// it is the farthest (a dead end's end within 3 steps of the farthest wins), with steps the
// nearest to that many steps; else one at random, 3 or more steps away when there is one.
func wallSpot(grid [][]rune, dist map[[2]int]int, taken map[[2]int]bool, kind func(rune) Kind, rng *rand.Rand, p data.Put) ([2]int, bool) {
	h, w := len(grid), len(grid[0])
	steps4 := [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	type spot struct {
		c, n [2]int
		end  bool // the end of a straight dead end
	}
	var spots []spot
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			c := [2]int{x, y}
			if taken[c] {
				continue
			}
			if _, walked := dist[c]; grid[y][x] != '#' && (grid[y][x] != '.' || !walked) {
				continue
			}
			var n [2]int
			open := 0
			for _, d := range steps4 {
				if grid[y+d[1]][x+d[0]] != '#' {
					open++
					n = [2]int{x + d[0], y + d[1]}
				}
			}
			if open != 1 {
				continue
			}
			if _, ok := dist[n]; !ok {
				continue
			}
			if r := grid[n[1]][n[0]]; r == data.CellDoor {
				continue
			} else if k := kind(r); k.Solid || k.Slide != "" || k.Carry {
				continue
			}
			// n の 向こう側（c の 反対）だけが 開いた 行き止まり
			back := [2]int{2*n[0] - c[0], 2*n[1] - c[1]}
			end := true
			for _, d := range steps4 {
				m := [2]int{n[0] + d[0], n[1] + d[1]}
				if m == c {
					continue
				}
				if open := grid[m[1]][m[0]] != '#'; open != (m == back) {
					end = false
				}
			}
			spots = append(spots, spot{c, n, end})
		}
	}
	if len(spots) == 0 {
		return [2]int{}, false
	}
	switch {
	case p.Far:
		far := 0
		for _, s := range spots {
			far = max(far, dist[s.n])
		}
		score := func(s spot) int {
			if s.end && dist[s.n] >= far-3 {
				return 1000 + dist[s.n]
			}
			return dist[s.n]
		}
		best := 0
		for i, s := range spots {
			if score(s) > score(spots[best]) {
				best = i
			}
		}
		return spots[best].c, true
	case p.Steps > 0:
		best := -1
		for i, s := range spots {
			d := abs(dist[s.n] + 1 - p.Steps)
			if best < 0 {
				best = i
				continue
			}
			bd := abs(dist[spots[best].n] + 1 - p.Steps)
			if d < bd || d == bd && s.end && !spots[best].end {
				best = i
			}
		}
		return spots[best].c, true
	}
	rng.Shuffle(len(spots), func(i, j int) { spots[i], spots[j] = spots[j], spots[i] })
	for pass := range 2 {
		for _, s := range spots {
			if s.end && (pass == 1 || dist[s.n] >= 3) {
				return s.c, true
			}
		}
	}
	closed := func(c, except [2]int) bool { // c の まわりが except の ほかは 壁
		for _, d := range steps4 {
			m := [2]int{c[0] + d[0], c[1] + d[1]}
			if m != except && grid[m[1]][m[0]] != '#' {
				return false
			}
		}
		return true
	}
	for pass := range 2 {
		for _, s := range spots {
			e := [2]int{2*s.c[0] - s.n[0], 2*s.c[1] - s.n[1]} // くぼみの 奥
			if pass == 0 && dist[s.n] < 3 || grid[s.c[1]][s.c[0]] != '#' || e[0] < 1 || e[1] < 1 || e[0] > w-2 || e[1] > h-2 || taken[e] {
				continue
			}
			if grid[e[1]][e[0]] == '#' && closed(s.c, s.n) && closed(e, s.c) {
				grid[s.c[1]][s.c[0]] = '.'
				return e, true
			}
		}
	}
	for pass := range 2 {
		for _, s := range spots {
			if pass == 1 || dist[s.n] >= 3 {
				return s.c, true
			}
		}
	}
	return spots[0].c, true
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// walkable reports whether, on grid, every open cell (not a wall, a solid thing, a slide or a
// carrying tile) can be walked to from start, every solid thing has a side walked to, and
// the start can be walked back to from everywhere walked to (no slide leaves the party where
// it cannot come back). Slides carry the party as SlideEnd does (stopping before a wall, a
// door or a solid thing, and on any other tile).
func walkable(grid [][]rune, start [2]int, kind func(rune) Kind) bool {
	h, w := len(grid), len(grid[0])
	at := func(x, y int) rune {
		if x < 0 || y < 0 || x >= w || y >= h {
			return '#'
		}
		return grid[y][x]
	}
	open := func(r rune) bool { return r != '#' && r != ' ' && !kind(r).Solid }
	dirs := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} // north east south west
	// move is where a step from x, y the way f ends (ok false: no step, or carried away)
	move := func(x, y, f int) (int, int, bool) {
		nx, ny := x+dirs[f][0], y+dirs[f][1]
		r := at(nx, ny)
		if !open(r) {
			return 0, 0, false
		}
		k := kind(r)
		if k.Carry {
			return 0, 0, false
		}
		if k.Slide == "" {
			return nx, ny, true
		}
		dir := slideDir(k.Slide, f)
		seen := map[[3]int]bool{}
		for {
			key := [3]int{nx, ny, dir}
			if seen[key] {
				break
			}
			seen[key] = true
			mx, my := nx+dirs[dir][0], ny+dirs[dir][1]
			r := at(mx, my)
			if !open(r) || r == 'D' {
				break
			}
			nx, ny = mx, my
			if r == '.' {
				continue
			}
			k := kind(r)
			if k.Slide == "" {
				if k.Carry {
					return 0, 0, false
				}
				break
			}
			dir = slideDir(k.Slide, dir)
		}
		return nx, ny, true
	}
	idx := func(x, y int) int { return y*w + x }
	seen := make([]bool, w*h)
	back := make([][]int, w*h) // back[n]: the cells with a step to n
	seen[idx(start[0], start[1])] = true
	q := []int{idx(start[0], start[1])}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		x, y := p%w, p/w
		for f := range 4 {
			nx, ny, ok := move(x, y, f)
			if !ok {
				continue
			}
			n := idx(nx, ny)
			back[n] = append(back[n], p)
			if !seen[n] {
				seen[n] = true
				q = append(q, n)
			}
		}
	}
	for y := range h {
		for x := range w {
			r := grid[y][x]
			k := kind(r)
			switch {
			case r == '#' || r == ' ' || k.Slide != "":
			case k.Solid || k.Carry: // come up to from a side (a carrying tile: stepped on from there)
				ok := false
				for _, d := range dirs {
					if nx, ny := x+d[0], y+d[1]; nx >= 0 && ny >= 0 && nx < w && ny < h && seen[idx(nx, ny)] {
						ok = true
					}
				}
				if !ok {
					return false
				}
			default:
				if !seen[idx(x, y)] {
					return false
				}
			}
		}
	}
	// and back: every cell walked to has a way to the start
	home := make([]bool, w*h)
	home[idx(start[0], start[1])] = true
	q = []int{idx(start[0], start[1])}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		for _, b := range back[p] {
			if !home[b] {
				home[b] = true
				q = append(q, b)
			}
		}
	}
	for i := range seen {
		if seen[i] && !home[i] {
			return false
		}
	}
	return true
}

// slideDir is the way a slide carries the party moving the way f.
func slideDir(slide string, f int) int {
	if slide == "facing" {
		return f & 3
	}
	if i := data.FacingIndex(slide); i >= 0 {
		return i
	}
	return f & 3
}

// carries reports whether the commands take the party elsewhere without asking (a move or a
// dive, maybe in an event called or a branch; not in a choice, which may be turned down).
func carries(db *data.DB, cmds []data.Command, seen map[string]bool) bool {
	for _, c := range cmds {
		if c.Move != "" || c.Dive != "" {
			return true
		}
		if c.Call != "" && !seen[c.Call] && db.Events[c.Call] != nil {
			seen[c.Call] = true
			if carries(db, db.Events[c.Call].Do, seen) {
				return true
			}
		}
		if carries(db, c.Then, seen) || carries(db, c.Else, seen) {
			return true
		}
	}
	return false
}
