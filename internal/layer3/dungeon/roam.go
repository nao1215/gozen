package dungeon

import (
	"math"
	"strconv"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// The roamers (data.Roamer): strong foes seen walking a made floor. A dive places them
// away from the start; each time the party walks a cell they walk one too (a chaser comes
// after the party once it sees it). Meeting one, by walking into it or being walked into,
// starts its battle; won, the roamer is gone from the floor (the battle's on_win sets
// var.roamer_beaten, read at the next step); fled, it stays but lets the party go for
// roamFledSteps steps (on_escape sets var.roamer_fled): so it does not catch up on the very next step after fleeing and force a rematch.

// VarRoamerBeaten holds the number (index + 1) of the roamer whose battle was just won.
const VarRoamerBeaten = "roamer_beaten"

// VarRoamCalm is the step count (State.Steps) before which no roamer chases: the first
// steps on a floor are the party's own.
const VarRoamCalm = "roam_calm"

// VarRoamerFled holds the number (index + 1) of the roamer the party just fled from.
const VarRoamerFled = "roamer_fled"

// VarHuntFrom is the step count (State.Steps) from which the wait of a roamer with an appear condition started counting. It is set on arriving at a floor and
// on meeting such a roamer. It does not appear until the wait steps have passed (it does not show up right after arriving or meeting).
const VarHuntFrom = "hunt_from"

// VarHuntNear is the number of steps (floor cells walked) to the nearest chaser (move: hunt) on the floor.
// HuntFar when there is no chaser, outside a dungeon, or it is unreachable. Written each time Hunters is called (the data uses it for "cannot do X while one is near" checks).
const VarHuntNear = "hunt_near"

// HuntFar is the value of VarHuntNear meaning "none".
const HuntFar = 999

// roamCalmSteps is how many steps a floor's roamers let the party walk before chasing.
const roamCalmSteps = 30

// roamFledSteps is how many steps a roamer the party fled from lets it go before chasing.
const roamFledSteps = 8

// placeRoamers puts the floor's roamers on cells of floor far enough from the party.
func placeRoamers(db *data.DB, st *rpg.State, d *data.Dungeon) {
	st.Roamers = nil
	if len(d.Roamers) == 0 {
		return
	}
	floor := optional.Value[float64](st.Vars[VarFloor])
	env := expr.Chain{expr.Map{"floor": floor}, st.Env()}
	w, h := d.Size()
	dist := walkDistances(d, st, st.Pos.X, st.Pos.Y)
	st.Vars[VarRoamCalm] = float64(st.Steps + roamCalmSteps)
	st.Vars[VarHuntFrom] = float64(st.Steps)
	most := 0 // the farthest walk on the floor: roamers go to its far half
	for _, v := range dist {
		most = max(most, v)
	}
	for kind, r := range d.Roamers {
		if r.Appear.Set() { // roamers that appear by condition are not placed when diving in (Hunters brings them out)
			continue
		}
		n := 1
		if r.Count.Set() {
			v, err := r.Count.Number(env)
			if err != nil {
				continue
			}
			n = int(v)
		}
		far := r.Far
		if !r.HasFar() || (db.Game.SchemaVersion == data.LegacySchemaVersion && far <= 0) {
			far = max(6, most*3/5)
		}
		var cells [][2]int
		open := 0
		if r.Move == "guard" {
			open = 4
		}
		for ; len(cells) == 0 && open >= 0; open-- {
			for f := far; (f > 0 || (f == 0 && far == 0)) && len(cells) == 0; f -= 2 { // nearer only when the floor has no such cell
				for y := range h {
					for x := range w {
						if dist[y*w+x] >= f && d.At(x, y) == data.CellFloor && roamerAt(st, x, y) < 0 && openAround(d, x, y) >= open {
							cells = append(cells, [2]int{x, y})
						}
					}
				}
			}
		}
		for range n {
			if len(cells) == 0 {
				break
			}
			i := st.Rand.IntN(len(cells))
			c := cells[i]
			cells = append(cells[:i], cells[i+1:]...)
			troop := pickTroop(st, r.Troops)
			if troop == "" {
				continue
			}
			st.Roamers = append(st.Roamers, rpg.Roamer{Kind: kind, Troop: troop, X: c[0], Y: c[1]})
		}
	}
}

// openAround is how many of the 4 cells next to x, y are plain floor.
func openAround(d *data.Dungeon, x, y int) int {
	w, h := d.Size()
	n := 0
	for f := range 4 {
		nx, ny := x+dx[f], y+dy[f]
		if nx >= 0 && ny >= 0 && nx < w && ny < h && d.At(nx, ny) == data.CellFloor {
			n++
		}
	}
	return n
}

func pickTroop(st *rpg.State, picks []data.TroopPick) string {
	total := 0.0
	var ok []data.TroopPick
	env := st.Env()
	for _, p := range picks {
		if p.When.Set() {
			if b, err := p.When.Bool(env); err != nil || !b {
				continue
			}
		}
		if p.Weight <= 0 {
			p.Weight = 1
		}
		ok = append(ok, p)
		total += p.Weight
	}
	x := st.Rand.Float64() * total
	for _, p := range ok {
		if x -= p.Weight; x < 0 {
			return p.Troop
		}
	}
	return ""
}

// roamerAt is the index of the roamer at x, y (-1 when none).
func roamerAt(st *rpg.State, x, y int) int {
	for i, r := range st.Roamers {
		if r.X == x && r.Y == y {
			return i
		}
	}
	return -1
}

// roamerTile is the tile a roamer shows as: its picture, its name, and its battle when
// the party meets it.
func roamerTile(d *data.Dungeon, st *rpg.State, i int) *data.Tile {
	r := st.Roamers[i]
	if r.Kind < 0 || r.Kind >= len(d.Roamers) {
		return nil
	}
	k := d.Roamers[r.Kind]
	size := k.Size
	if size == 0 {
		size = 0.8
	}
	pic := k.Picture
	if pic.File == "" {
		pic = k.Icon // if there is no picture for its figure, put a marker
	}
	if len(k.Meet) > 0 {
		return &data.Tile{Name: k.Name, Picture: pic, Size: size, Mark: k.Mark, Trigger: "step", Do: k.Meet}
	}
	beaten, err := expr.Parse(strconv.Itoa(i + 1))
	if err != nil {
		return nil
	}
	return &data.Tile{
		Name: k.Name, Picture: pic, Size: size, Mark: k.Mark, Trigger: "step",
		Do: []data.Command{{
			Battle: r.Troop,
			OnWin:  []data.Command{{SetV: map[string]data.Expr{"var." + VarRoamerBeaten: {Expr: beaten}}}},
			OnRun:  []data.Command{{SetV: map[string]data.Expr{"var." + VarRoamerFled: {Expr: beaten}}}},
		}},
	}
}

// SettleRoamers takes off the roamer whose battle was won, and calms the one fled from.
func SettleRoamers(st *rpg.State) {
	if f := optional.Value[float64](st.Vars[VarRoamerFled]); f > 0 {
		delete(st.Vars, VarRoamerFled)
		if i := int(f) - 1; i < len(st.Roamers) {
			st.Roamers[i].Calm = st.Steps + roamFledSteps
		}
	}
	n := optional.Value[float64](st.Vars[VarRoamerBeaten])
	if n <= 0 {
		return
	}
	delete(st.Vars, VarRoamerBeaten)
	if i := int(n) - 1; i < len(st.Roamers) {
		st.Roamers = append(st.Roamers[:i], st.Roamers[i+1:]...)
	}
}

// meet is the battle of the roamer on the party's cell (nil when none). A roamer with meet
// leaves the floor as it is met, and its meet commands run (instead of its battle).
func meet(d *data.Dungeon, st *rpg.State) []data.Command {
	if i := roamerAt(st, st.Pos.X, st.Pos.Y); i >= 0 {
		if r := st.Roamers[i]; r.Kind >= 0 && r.Kind < len(d.Roamers) && len(d.Roamers[r.Kind].Meet) > 0 {
			k := d.Roamers[r.Kind]
			st.Roamers = append(st.Roamers[:i], st.Roamers[i+1:]...)
			if len(st.Roamers) == 0 {
				st.Roamers = nil
			}
			if k.Appear.Set() {
				st.Vars[VarHuntFrom] = float64(st.Steps) // after meeting, it does not appear for wait steps
			}
			return append([]data.Command{}, k.Meet...)
		}
		if t := roamerTile(d, st, i); t != nil {
			return t.Do
		}
	}
	return nil
}

func moveRoamers(d *data.Dungeon, st *rpg.State, from [2]int) {
	if len(st.Roamers) == 0 {
		return
	}
	dist := walkDistances(d, st, st.Pos.X, st.Pos.Y)
	for i := range st.Roamers {
		r := &st.Roamers[i]
		if r.Kind < 0 || r.Kind >= len(d.Roamers) { // a kind the dungeon no longer has (data reloaded under a running game)
			continue
		}
		k := d.Roamers[r.Kind]
		if k.Move == "guard" {
			continue
		}
		r.Walk++
		n := every(k, st)
		moves := 1
		if n >= 1 {
			if s := int(n); s > 1 && r.Walk%s != 0 { // one cell every `every` steps: rest on this step
				continue
			}
		} else { // every is less than 1: several cells per step (2 cells for 0.5)
			moves = max(1, int(math.Round(1/n)))
		}
		for m := range moves {
			if !stepRoamer(d, st, dist, from, i, m == 0) {
				break
			}
		}
	}
}

// stepRoamer walks roamer i a cell (see moveRoamers) and reports whether it moved on (false:
// it stood still, or stepped into the party).
// Only the first cell of a step may jump into the party (it was shown beside the party before).
func stepRoamer(d *data.Dungeon, st *rpg.State, dist []int, from [2]int, i int, first bool) bool {
	w, _ := d.Size()
	r := &st.Roamers[i]
	k := d.Roamers[r.Kind]
	sight := k.Sight
	if sight <= 0 {
		sight = 4
	}
	here := dist[r.Y*w+r.X]
	var best [2]int
	found := false
	calm := optional.Value[float64](st.Vars[VarRoamCalm])
	hunt := k.Move == "hunt" && here >= 0 && st.Steps >= r.Calm // a chaser knows the way even from far away
	if hunt || k.Move == "chase" && here >= 0 && here <= sight && float64(st.Steps) >= calm && st.Steps >= r.Calm {
		beside := abs(r.X-from[0])+abs(r.Y-from[1]) == 1
		if first && beside && here <= 2 { // it was visible next door: follow the path walked and pounce
			r.X, r.Y = st.Pos.X, st.Pos.Y
			return false
		}
		if here == 1 { // just arrived next door: stop where it can be seen
			return false
		}
		for f := range 4 {
			x, y := r.X+dx[f], r.Y+dy[f]
			if !roamable(d, st, x, y, i) {
				continue
			}
			if v := dist[y*w+x]; v >= 1 && v < here { // until it comes next door (entering from next door is only the pounce above)
				best, found = [2]int{x, y}, true
				break
			}
		}
	}
	if !found && k.Move == "hunt" { // a chaser does not wander (it waits if the way is blocked)
		return false
	}
	if !found && st.Rand.Float64() < 0.7 {
		var opts [][2]int
		for f := range 4 {
			x, y := r.X+dx[f], r.Y+dy[f]
			if roamable(d, st, x, y, i) && (x != st.Pos.X || y != st.Pos.Y) {
				opts = append(opts, [2]int{x, y})
			}
		}
		if len(opts) > 0 {
			best, found = opts[st.Rand.IntN(len(opts))], true
		}
	}
	if found {
		r.X, r.Y = best[0], best[1]
	}
	return found
}

// roamable reports whether roamer i may walk into x, y: open floor (no tile, no door) with
// no other roamer.
func roamable(d *data.Dungeon, st *rpg.State, x, y, i int) bool {
	w, h := d.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		return false
	}
	if c := d.At(x, y); c != data.CellFloor {
		// A chaser (hunt) also passes doors and walkable floor traps (it does not stop at a room's door or a trapped corridor). Others use plain floor only
		if i < 0 || i >= len(st.Roamers) || st.Roamers[i].Kind < 0 || st.Roamers[i].Kind >= len(d.Roamers) || d.Roamers[st.Roamers[i].Kind].Move != "hunt" {
			return false
		}
		if CellAt(d, st, x, y).Solid {
			return false
		}
	}
	j := roamerAt(st, x, y)
	return j < 0 || j == i
}

// every is how many of the party's steps a roamer takes to walk a cell (1 at least).
// Less than 1 is more than a cell a step (0.5: 2 cells).
func every(k data.Roamer, st *rpg.State) float64 {
	if !k.Every.Set() {
		return 1
	}
	v, err := k.Every.Number(floorEnv(st))
	if err != nil || v <= 0 {
		return 1
	}
	if v < 1 {
		return v
	}
	return float64(int(v))
}

// floorEnv is the state's names with floor (the floor's number).
func floorEnv(st *rpg.State) expr.Env {
	floor := optional.Value[float64](st.Vars[VarFloor])
	return expr.Chain{expr.Map{"floor": floor}, st.Env()}
}

// Hunters puts out and takes off the roamers with appear (data.Roamer.Appear) on the floor
// the party is on, and reports the commands that follow (on_appear, on_leave). One whose
// appear holds comes, after the floor's wait, onto the plain floor farthest from the party
// by the walk; one whose appear no longer holds leaves. Called when nothing else runs
// (after a step, a battle, an event), as the auto events.
func Hunters(db *data.DB, st *rpg.State) []data.Command {
	d := Of(db, st)
	if d == nil || d.Generate == nil || st.Pos.In != st.Location {
		if _, ok := st.Vars[VarHuntNear]; ok {
			st.Vars[VarHuntNear] = float64(HuntFar)
		}
		return nil
	}
	defer func() { st.Vars[VarHuntNear] = float64(huntNear(d, st)) }()
	var out []data.Command
	env := floorEnv(st)
	for kind, k := range d.Roamers {
		if !k.Appear.Set() {
			continue
		}
		ok, err := k.Appear.Bool(env)
		if err != nil {
			continue
		}
		i := -1
		for j, r := range st.Roamers {
			if r.Kind == kind {
				i = j
				break
			}
		}
		switch {
		case !ok && i >= 0:
			st.Roamers = append(st.Roamers[:i], st.Roamers[i+1:]...)
			if len(st.Roamers) == 0 {
				st.Roamers = nil
			}
			out = append(out, k.OnLeave...)
		case ok && i < 0:
			from := optional.Value[float64](st.Vars[VarHuntFrom])
			if k.Wait.Set() {
				if w, err := k.Wait.Number(env); err == nil && float64(st.Steps)-from < w {
					continue
				}
			}
			if !huntIn(d, st, kind) {
				continue
			}
			out = append(out, k.OnAppear...)
		}
	}
	return out
}

// huntIn puts a roamer of kind on the plain floor farthest from the party by the walk (one
// of them at random), and reports whether there was such a cell. Its far keeps it that many
// steps from the floor's start (so the up stairs near the arrival point are not blocked, leaving no escape).
func huntIn(d *data.Dungeon, st *rpg.State, kind int) bool {
	w, h := d.Size()
	dist := walkDistances(d, st, st.Pos.X, st.Pos.Y)
	var fromStart []int
	if far := d.Roamers[kind].Far; far > 0 && d.Generate != nil && d.Generate.Start != "" {
		start := []rune(d.Generate.Start)[0]
		for y := 0; y < h && fromStart == nil; y++ {
			for x := range w {
				if d.At(x, y) == start {
					fromStart = walkDistances(d, st, x, y)
					break
				}
			}
		}
		for f := far; fromStart != nil && f > 0; f-- { // if there is no such floor, bring it closer
			ok := false
			for i, v := range fromStart {
				if v >= f && dist[i] > 0 && d.At(i%w, i/w) == data.CellFloor {
					ok = true
					break
				}
			}
			if ok {
				for i, v := range fromStart {
					if v >= 0 && v < f {
						dist[i] = -1
					}
				}
				break
			}
		}
	}
	most := 0
	var cells [][2]int
	for i, v := range dist {
		x, y := i%w, i/w
		if v <= 0 || d.At(x, y) != data.CellFloor || roamerAt(st, x, y) >= 0 {
			continue
		}
		if v > most {
			most, cells = v, nil
		}
		if v == most {
			cells = append(cells, [2]int{x, y})
		}
	}
	if len(cells) == 0 {
		return false
	}
	c := cells[st.Rand.IntN(len(cells))]
	st.Roamers = append(st.Roamers, rpg.Roamer{Kind: kind, X: c[0], Y: c[1]})
	return true
}

// HuntersOn are the roamers on the floor that hunt (move: hunt): the ones the map always shows.
func HuntersOn(db *data.DB, st *rpg.State) []rpg.Roamer {
	d := Of(db, st)
	if d == nil {
		return nil
	}
	var out []rpg.Roamer
	for _, r := range st.Roamers {
		if r.Kind >= 0 && r.Kind < len(d.Roamers) && d.Roamers[r.Kind].Move == "hunt" {
			out = append(out, r)
		}
	}
	return out
}

// huntNear is the walk from the party to the nearest roamer that hunts (HuntFar: none or not reached).
func huntNear(d *data.Dungeon, st *rpg.State) int {
	near := HuntFar
	var dist []int
	w, _ := d.Size()
	for _, r := range st.Roamers {
		if r.Kind < 0 || r.Kind >= len(d.Roamers) || d.Roamers[r.Kind].Move != "hunt" {
			continue
		}
		if dist == nil {
			dist = walkDistances(d, st, st.Pos.X, st.Pos.Y)
		}
		if i := r.Y*w + r.X; i >= 0 && i < len(dist) && dist[i] >= 0 {
			near = min(near, dist[i])
		}
	}
	return near
}

// walkDistances are the steps from x, y to every cell over open cells (-1: not reached).
func walkDistances(d *data.Dungeon, st *rpg.State, x0, y0 int) []int {
	w, h := d.Size()
	dist := make([]int, w*h)
	for i := range dist {
		dist[i] = -1
	}
	dist[y0*w+x0] = 0
	queue := [][2]int{{x0, y0}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for f := range 4 {
			x, y := p[0]+dx[f], p[1]+dy[f]
			if x < 0 || y < 0 || x >= w || y >= h || dist[y*w+x] >= 0 {
				continue
			}
			if !CellAt(d, st, x, y).Passable() {
				continue
			}
			dist[y*w+x] = dist[p[1]*w+p[0]] + 1
			queue = append(queue, [2]int{x, y})
		}
	}
	return dist
}
