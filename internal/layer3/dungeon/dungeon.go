// Package dungeon is the rules of the dungeons (data.Dungeon) without a screen: where
// the party is and faces, what each cell of the map is now, walking and turning, what
// happens on a step or on looking at a tile, the battles met on the way, and the map of
// the cells seen. The screen (play) and the headless driver both use it.
package dungeon

import (
	"fmt"
	"maps"
	"strings"

	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	world "github.com/nao1215/gozen/internal/layer3/field"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// The steps of each facing (north, east, south, west) on the map.
var (
	dx = [4]int{0, 1, 0, -1}
	dy = [4]int{-1, 0, 1, 0}
)

// Of is the dungeon the party is in (nil at a location that is not one).
func Of(db *data.DB, st *rpg.State) *data.Dungeon {
	if l := db.Locations[st.Location]; l != nil {
		return l.Dungeon
	}
	return nil
}

// Styled is the dungeon the party is in as it looks now: the first of its looks whose
// when holds laid over it (a copy; nil when the party is not in a dungeon).
func Styled(db *data.DB, st *rpg.State) *data.Dungeon {
	d := Of(db, st)
	if d == nil || len(d.Looks) == 0 {
		return d
	}
	env := st.Env()
	for _, lk := range d.Looks {
		if ok, err := lk.When.Bool(env); err != nil || !ok {
			continue
		}
		c := *d
		if lk.Wall.File != "" {
			c.Wall = lk.Wall
		}
		if lk.Door.File != "" {
			c.Door = lk.Door
		}
		if lk.Floor.Valid {
			c.Floor = lk.Floor
		}
		if lk.Ceiling.Valid {
			c.Ceiling = lk.Ceiling
		}
		// pattern: for a look that only changes the color, do not keep the original pattern (the upper section's pattern must not appear on another section's floor)
		if lk.FloorTex.File != "" || lk.Floor.Valid {
			c.FloorTex = lk.FloorTex
		}
		if lk.CeilingTex.File != "" || lk.Ceiling.Valid {
			c.CeilingTex = lk.CeilingTex
		}
		if lk.HasDark() { // darkness: 0 (no darkening) also overrides
			c.Dark = lk.Dark
		}
		if lk.NoMap {
			c.NoMap = true
		}
		if len(lk.Pictures) > 0 { // a picture replacement is layered over the dungeon's own
			c.Pictures = maps.Clone(d.Pictures)
			if c.Pictures == nil {
				c.Pictures = map[string]string{}
			}
			maps.Copy(c.Pictures, lk.Pictures)
		}
		return &c
	}
	return d
}

// BattleBG is the battle background a look of the dungeon gives (empty if none).
func BattleBG(db *data.DB, st *rpg.State) data.Picture {
	d := Of(db, st)
	if d == nil {
		return data.Picture{}
	}
	env := st.Env()
	for _, lk := range d.Looks {
		if ok, err := lk.When.Bool(env); err == nil && ok {
			return lk.BattleBG
		}
	}
	return data.Picture{}
}

// Music is the song a look of the dungeon gives (empty if none).
func Music(db *data.DB, st *rpg.State) data.MusicCue {
	d := Of(db, st)
	if d == nil {
		return data.MusicCue{}
	}
	env := st.Env()
	for _, lk := range d.Looks {
		if ok, err := lk.When.Bool(env); err == nil && ok {
			return lk.Music
		}
	}
	return data.MusicCue{}
}

// BattleMusic is the battle music at the current place. The enemy group's music comes first, then the dungeon look's (the first one in looks that
// holds) battle_music, the place's battle_music, and game.yaml's battle.music, in that order
// (empty if none. The ordinary-battle music changes per band of floors, and a guardian keeps its enemy group's music).
func BattleMusic(db *data.DB, st *rpg.State, troop *data.Troop) data.MusicCue {
	if troop != nil && troop.Music.Song != "" {
		return troop.Music
	}
	if d := Of(db, st); d != nil {
		env := st.Env()
		for _, lk := range d.Looks {
			if ok, err := lk.When.Bool(env); err == nil && ok {
				if lk.BattleMusic.Song != "" {
					return lk.BattleMusic
				}
				break
			}
		}
	}
	if l := db.Locations[st.Location]; l != nil && l.BattleMusic.Song != "" {
		return l.BattleMusic
	}
	return db.Game.Battle.Music
}

// Place puts the party in the dungeon loc on the first cell with the character at (""
// for its start tile), facing facing ("" for the tile's facing, else as it was). A cell
// that cannot be stood on (a solid thing, the stairs in a wall) is arrived in front of,
// facing it. It does nothing for a location that is not a dungeon.
func Place(db *data.DB, st *rpg.State, loc, at, facing string) {
	l := db.Locations[loc]
	if l == nil || l.Dungeon == nil {
		return
	}
	d := l.Dungeon
	var c rune
	if at != "" {
		c = []rune(at)[0]
	} else if s, ok := d.StartChar(); ok {
		c = s
	}
	x, y, ok := d.Find(c)
	if !ok {
		x, y, ok = findPut(st, loc, c) // a tile open_wall put on this floor
	}
	if !ok && at != "" {
		// the character is not on this floor (stairs that have not appeared yet): the start
		if s, has := d.StartChar(); has {
			c = s
			x, y, ok = d.Find(c)
		}
		if g := d.Generate; !ok && g != nil && g.Start != "" {
			c = []rune(g.Start)[0]
			x, y, ok = d.Find(c)
		}
	}
	if !ok {
		return
	}
	f := st.Pos.Facing
	if t := d.Tile(c); t != nil && t.Facing != "" {
		f = data.FacingIndex(t.Facing)
	}
	if i := data.FacingIndex(facing); facing != "" && i >= 0 {
		f = i
	}
	if st.Pos.In != loc { // the roamers are the last floor's (a dive places the new floor's after)
		st.Roamers = nil
	}
	st.Pos = rpg.Pos{In: loc, X: x, Y: y, Facing: f & 3}
	st.Bumped = ""
	front := false // arrived in front of a solid thing (the stairs down in a wall), facing it
	// On reaching something impassable (stairs in a wall) or something that acts when stepped on (a descent in the floor), stand next to it and face it
	// (on arriving on something that acts when stepped on, you would not move until you left and came back, and could not tell what is underfoot)
	stepOn := false
	if t := d.Tile(c); t != nil && len(t.Do) > 0 && Trigger(t, false) == "step" {
		stepOn = true
	}
	if c := CellAt(d, st, x, y); stepOn || !c.Passable() || c.Look == "wall" || c.Look == "door" {
		for i := range 4 {
			nx, ny := x-dx[i], y-dy[i] // the cell from which facing i looks at x, y
			if standable(d, nx, ny) && CellAt(d, st, nx, ny).Passable() && CellAt(d, st, nx, ny).Look != "door" {
				st.Pos = rpg.Pos{In: loc, X: nx, Y: ny, Facing: i}
				front = true
				break
			}
		}
	}
	if !front && facing == "" && (d.Tile(c) == nil || d.Tile(c).Facing == "") {
		// no way given: face a way that is open (not the wall in front on arriving)
		for range 4 {
			fx, fy := Rel(st.Pos, 1, 0)
			if CellAt(d, st, fx, fy).Passable() {
				break
			}
			st.Pos.Facing = (st.Pos.Facing + 1) & 3
		}
	}
	Reveal(db, st)
}

// Ensure places the party at the start of the dungeon it is in when its position is
// not in it (a new game or a test play starting there).
func Ensure(db *data.DB, st *rpg.State) {
	if l := db.Locations[st.Location]; l != nil && l.Dungeon != nil && l.Dungeon.Generate != nil && st.Vars[VarSeed] == nil {
		Dive(db, st, st.Location, 1, true) // a made dungeon started in (a test play): its first floor
		return
	}
	Remake(db, st)
	d := Of(db, st)
	if d == nil {
		return
	}
	if st.Pos.In != st.Location || !standable(d, st.Pos.X, st.Pos.Y) {
		// a save from before the map changed (a smaller map, a wall where it stood): the start
		Place(db, st, st.Location, "", "")
	}
	// the roamers of a save from before their kinds or the map changed: the ones that no
	// longer fit are gone (moveRoamers and roamerTile index by them)
	w, h := d.Size()
	kept := st.Roamers[:0]
	for _, r := range st.Roamers {
		if r.Kind >= 0 && r.Kind < len(d.Roamers) && r.X >= 0 && r.Y >= 0 && r.X < w && r.Y < h && standable(d, r.X, r.Y) &&
			(d.At(r.X, r.Y) == data.CellFloor || d.Roamers[r.Kind].Move == "hunt") {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 {
		kept = nil
	}
	st.Roamers = kept
}

// standable reports whether the party can stand on x, y of the dungeon: in the map, not in
// a wall of the map itself (a tile made solid after the party came onto it does not count).
func standable(d *data.Dungeon, x, y int) bool {
	w, h := d.Size()
	if x < 0 || y < 0 || x >= w || y >= h {
		return false
	}
	c := d.At(x, y)
	if c == data.CellWall || c == data.CellNone {
		return false
	}
	t := d.Tile(c)
	return t == nil || t.Look != "wall"
}

// Cell is what a cell of the map is now.
type Cell struct {
	X, Y  int
	Char  rune       // the character it is now (after the tiles' when and else)
	Tile  *data.Tile // nil for the built-in characters
	Look  string     // wall, floor or door
	Solid bool       // the party cannot walk in
}

// Passable reports whether the party can walk into the cell.
func (c Cell) Passable() bool { return !c.Solid }

// CellAt is the cell at x, y as it is now.
func CellAt(d *data.Dungeon, st *rpg.State, x, y int) Cell {
	c := d.At(x, y)
	if len(st.Put) > 0 { // a cell an event changed (open_wall: the stairs that appeared in a wall)
		if p, ok := st.Put[doneKey(st, st.Location, x, y)]; ok && p != "" {
			c = []rune(p)[0]
		}
	}
	env := st.Env()
	for range 8 {
		t := d.Tile(c)
		if t != nil && t.Once && t.After != "" && st.Done[doneKey(st, st.Location, x, y)] {
			c = []rune(t.After)[0] // used: what it becomes (an opened chest)
			continue
		}
		if t == nil || !t.When.Set() {
			break
		}
		if ok, err := t.When.Bool(env); err == nil && ok {
			break
		}
		c = '.'
		if t.Else != "" {
			c = []rune(t.Else)[0]
		}
	}
	out := Cell{X: x, Y: y, Char: c, Tile: d.Tile(c)}
	// Strong enemies stand on the floor (a chaser also on floor traps. They do not show in the scenery inside a door)
	if t := out.Tile; len(d.Roamers) > 0 && (c == data.CellFloor || t != nil && t.Look != "wall" && t.Look != "door") {
		if i := roamerAt(st, x, y); i >= 0 {
			if t := roamerTile(d, st, i); t != nil {
				out.Tile, out.Look = t, "floor"
				return out
			}
		}
	}
	switch c {
	case data.CellWall, data.CellNone:
		out.Look, out.Solid = "wall", true
	case data.CellDoor:
		out.Look = "door"
	case data.CellFloor:
		out.Look = "floor"
	default:
		t := out.Tile
		out.Look = t.Look
		out.Solid = t.Look == "wall"
		if t.Solid.Set() {
			if ok, err := t.Solid.Bool(env); err == nil && ok {
				out.Solid = true
			}
		}
	}
	return out
}

// Rel is the map cell f cells ahead of p and s to its right (negative: behind, left).
func Rel(p rpg.Pos, f, s int) (int, int) {
	d, r := p.Facing&3, (p.Facing+1)&3
	return p.X + f*dx[d] + s*dx[r], p.Y + f*dy[d] + s*dy[r]
}

// Front is the cell in front of the party.
func Front(db *data.DB, st *rpg.State) Cell {
	x, y := Rel(st.Pos, 1, 0)
	return CellAt(Of(db, st), st, x, y)
}

// Turn turns the party by quarter turns (1 right, -1 left, 2 around).
func Turn(st *rpg.State, n int) { st.Pos.Facing = (st.Pos.Facing + n + 8) & 3 }

// Forward walks one cell ahead. It reports whether the party moved, and the commands
// that follow: the tile stepped on, or else maybe a random battle.
func Forward(db *data.DB, st *rpg.State) (bool, []data.Command) {
	d := Of(db, st)
	if d == nil {
		return false, nil
	}
	SettleRoamers(st)
	from := [2]int{st.Pos.X, st.Pos.Y} // where the step starts (a roamer next to it was seen there)
	c := Front(db, st)
	if !c.Passable() {
		// bumped into it: for a trigger bump tile (the default for solid), its do runs
		st.Moved = false
		return false, bump(st, c)
	}
	st.Moved = true
	st.Pos.X, st.Pos.Y = c.X, c.Y
	st.Bumped = ""
	st.Steps++
	Reveal(db, st)
	// each cell's step commands are taken once a step (a once tile is spent by taking them:
	// taken twice, the second gets nothing, and an event not once would run twice)
	fired := map[[2]int]bool{}
	fire := func(c Cell) []data.Command {
		k := [2]int{c.X, c.Y}
		if fired[k] {
			return nil
		}
		fired[k] = true
		return trigger(st, c, "step")
	}
	var held []data.Command // a door's own commands: the party stops in it and they run
	if c.Look == "door" {
		held = fire(c)
	}
	if c.Look == "door" && held == nil {
		// through a door: on into the cell beyond, not standing in the doorway (its sides
		// are only wall and dark)
		if n := Front(db, st); n.Passable() && n.Look != "door" {
			st.Pos.X, st.Pos.Y = n.X, n.Y
			st.Steps++
			Reveal(db, st)
			c = n
		}
	}
	// a slide carries the party on (its own words first), a spin turns it where it stops
	c, slid := slideOn(db, d, st, c, fire)
	spin(st, c)
	step := func() []data.Command { // the commands of the cell stopped on
		cmds := append(held, fire(c)...)
		held = nil
		if len(cmds) == 0 {
			return nil
		}
		return cmds
	}
	with := func(cmds []data.Command) []data.Command {
		if len(slid) == 0 {
			return cmds
		}
		return append(append([]data.Command{}, slid...), cmds...)
	}
	if cmds := complete(d, st); cmds != nil {
		// the whole floor seen: the reward first, then what the step sets off
		if more := meet(d, st); more != nil {
			return true, with(append(cmds, more...))
		}
		return true, with(append(cmds, step()...))
	}
	if cmds := meet(d, st); cmds != nil { // walked into a roamer
		return true, with(cmds)
	}
	if cmds := step(); cmds != nil {
		return true, with(cmds)
	}
	moveRoamers(d, st, from)
	if cmds := meet(d, st); cmds != nil { // a roamer walked into the party
		return true, with(cmds)
	}
	if len(slid) > 0 {
		return true, slid
	}
	loc := db.Locations[st.Location]
	if loc.Encounters.Rate.Set() && !st.Calm {
		p, err := loc.Encounters.Rate.Number(st.Env())
		if g := loc.Encounters.Gauge; g > 0 && err == nil { // the gauge fills; the chance is its fill
			fill := min(1, Gauge(st)+p)
			st.Vars["gauge"] = fill
			p = fill * g
		}
		if err == nil && st.Rand.Float64() < p {
			st.Vars["gauge"] = 0.0
			if t := world.PickTroop(db, st); t != "" {
				return true, []data.Command{{Battle: t}}
			}
		}
	}
	return true, nil
}

// Look is what happens on looking at the cell in front (pressing the button): a trigger
// bump tile's commands, or a trigger look tile's question (see Ask).
func Look(db *data.DB, st *rpg.State) []data.Command {
	if Of(db, st) == nil {
		return nil
	}
	return trigger(st, Front(db, st), "look")
}

// Trigger is the way a tile is set off: step (stepped on), bump (walked into, or the
// button in front of it) or look (asked first: walked into, stepped on or the button in
// front of it gives a choice of its ask and 「やめる」). A tile that cannot be walked into
// is bump unless it is a wall's (a sign: look); one that can is step.
func Trigger(t *data.Tile, solid bool) string {
	switch {
	case t.Trigger != "":
		return t.Trigger
	case t.Look == "wall":
		return "look"
	case solid:
		return "bump"
	}
	return "step"
}

// Lookable reports whether walking into the tile sets it off (bump) or asks (look).
func Lookable(t *data.Tile, solid bool) bool {
	way := Trigger(t, solid)
	return len(t.Do) > 0 && (way == "look" || way == "bump")
}

// AskLabel is the answer that sets off a trigger look tile: its ask, or 「調べる」.
func AskLabel(t *data.Tile) string {
	if t.Ask != "" {
		return t.Ask
	}
	return lang.T("調べる")
}

// bump is the commands of walking into the cell c that cannot be walked into: its tile's
// when it has trigger bump, its question when trigger look. Walked into again from the
// same cell, a bump tile does nothing (a locked door does not say it is locked on every
// press of the button held): the party has to move to another cell first (st.Bumped, not
// saved; the screen clears it when the button is pressed anew, so only holding it is
// stopped). A look tile asks again (the question is the guard; 「やめる」 then ↑ asks anew).
func bump(st *rpg.State, c Cell) []data.Command {
	key := fmt.Sprintf("%s@%d,%d>%d,%d", floorKey(st, st.Location), st.Pos.X, st.Pos.Y, c.X, c.Y)
	if st.Bumped == key {
		return nil
	}
	cmds := trigger(st, c, "bump")
	if cmds != nil && (c.Tile == nil || Trigger(c.Tile, c.Solid || c.Tile.Look == "wall") != "look") {
		st.Bumped = key
	}
	return cmds
}

// trigger is the commands of the tile of c set off the way how (step, bump or look):
// its do when its trigger is that way (a bump tile by the button too), and for a trigger
// look tile, whatever the way, a choice of its ask (the do) and 「やめる」 (nothing). A
// once tile is spent when its do is given, or for a look tile when the ask is chosen.
func trigger(st *rpg.State, c Cell, how string) []data.Command {
	t := c.Tile
	if t == nil || len(t.Do) == 0 {
		return nil
	}
	way := Trigger(t, c.Solid || t.Look == "wall")
	switch {
	case way == "look", way == how, way == "bump" && how == "look": // bump: the button in front of it too
	default:
		return nil
	}
	key := doneKey(st, st.Location, c.X, c.Y)
	if t.Once && st.Done[key] {
		return nil
	}
	if way == "look" {
		do := t.Do
		if t.Once {
			do = append([]data.Command{{Done: key}}, t.Do...)
		}
		q := t.Name
		if t.Question != "" {
			q = t.Question
		}
		return []data.Command{{Prompt: q, Choice: []data.ChoiceOption{
			{Label: AskLabel(t), Do: do},
			{Label: lang.T("やめる")},
		}}}
	}
	if t.Once {
		st.Done[key] = true
	}
	return t.Do
}

// complete is the dungeon's complete commands when the floor is seen whole and they have
// not been given on it yet (once a floor, kept by floorKey with the map: a floor seen
// whole by a warp gives them on the next step, and walking it again gives nothing).
func complete(d *data.Dungeon, st *rpg.State) []data.Command {
	if len(d.Complete) == 0 {
		return nil
	}
	if e := optional.Value[float64](st.Vars[VarExplored]); e < 100 {
		return nil
	}
	key := completeKey(st, st.Location)
	if st.Done[key] {
		return nil
	}
	st.Done[key] = true
	return append([]data.Command{}, d.Complete...)
}

// Completed reports whether the floor of loc the party knows has given its complete.
func Completed(st *rpg.State, loc string) bool { return st.Done[completeKey(st, loc)] }

func completeKey(st *rpg.State, loc string) string { return floorKey(st, loc) + "#complete" }

// Spent reports whether a tile with once has been set off.
func Spent(st *rpg.State, loc string, x, y int) bool { return st.Done[doneKey(st, loc, x, y)] }

func doneKey(st *rpg.State, loc string, x, y int) string {
	return fmt.Sprintf("%s@%d,%d", floorKey(st, loc), x, y)
}

// floorKey names the floor of loc the party knows: the location, and for a made dungeon
// its floor too (each floor keeps its own map and opened chests).
func floorKey(st *rpg.State, loc string) string {
	if l := st.DB.Locations[loc]; l != nil && l.Dungeon != nil && l.Dungeon.Generate != nil {
		f := optional.Value[float64](st.Vars[VarFloor])
		return loc + "/" + fmtF(f)
	}
	return loc
}

func Reveal(db *data.DB, st *rpg.State) {
	d := Of(db, st)
	if d == nil {
		return
	}
	w, h := d.Size()
	key := floorKey(st, st.Location)
	m := st.Mapped[key]
	if len(m) != w*h {
		m = make([]byte, w*h)
		st.Mapped[key] = m
	}
	for oy := -1; oy <= 1; oy++ {
		for ox := -1; ox <= 1; ox++ {
			x, y := st.Pos.X+ox, st.Pos.Y+oy
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			if ox == 0 && oy == 0 {
				m[y*w+x] = 1
				continue
			}
			// what cannot be walked into, and what takes the party away on a step (a warp, a
			// pit: seen from beside it, so that a floor can be seen whole without falling)
			if c := CellAt(d, st, x, y); !c.Passable() || (c.Tile != nil && Trigger(c.Tile, false) == "step" && carries(db, c.Tile.Do, map[string]bool{})) {
				m[y*w+x] = 1
			}
		}
	}
	st.Vars[VarExplored] = float64(Explored(d, m))
}

// VarExplored is the variable holding how much of the floor the party has seen, in
// percent (the cells one can stand on).
const VarExplored = "explored"

// Explored is the percent (0..100, rounded down) of the dungeon's open cells seen in m.
func Explored(d *data.Dungeon, m []byte) int {
	open, seen := 0, 0
	for y, row := range d.Grid {
		for x, r := range row {
			if r == data.CellWall || r == data.CellNone {
				continue
			}
			open++
			if i := y*len(row) + x; i < len(m) && m[i] != 0 {
				seen++
			}
		}
	}
	if open == 0 {
		return 0
	}
	return seen * 100 / open
}

// Seen reports whether the cell of the dungeon loc has been seen.
func Seen(db *data.DB, st *rpg.State, loc string, x, y int) bool {
	l := db.Locations[loc]
	if l == nil || l.Dungeon == nil {
		return false
	}
	w, h := l.Dungeon.Size()
	m := st.Mapped[floorKey(st, loc)]
	return x >= 0 && y >= 0 && x < w && y < h && len(m) == w*h && m[y*w+x] != 0
}

func Shown(db *data.DB, st *rpg.State) []bool {
	d := Of(db, st)
	if d == nil {
		return nil
	}
	w, h := d.Size()
	m := st.Mapped[floorKey(st, st.Location)]
	out := make([]bool, w*h)
	if len(m) != w*h {
		return out
	}
	open := func(x, y int) bool { r := d.At(x, y); return r != data.CellWall && r != data.CellNone }
	// a wall may be filled in when no open cell around it is still unseen
	fill := func(x, y int) bool {
		if open(x, y) {
			return false
		}
		for oy := -1; oy <= 1; oy++ {
			for ox := -1; ox <= 1; ox++ {
				nx, ny := x+ox, y+oy
				if nx >= 0 && ny >= 0 && nx < w && ny < h && open(nx, ny) && m[ny*w+nx] == 0 {
					return false
				}
			}
		}
		return true
	}
	var queue [][2]int
	for i, v := range m {
		if v != 0 {
			out[i] = true
			queue = append(queue, [2]int{i % w, i / w})
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for oy := -1; oy <= 1; oy++ {
			for ox := -1; ox <= 1; ox++ {
				nx, ny := p[0]+ox, p[1]+oy
				if nx < 0 || ny < 0 || nx >= w || ny >= h || out[ny*w+nx] || !fill(nx, ny) {
					continue
				}
				out[ny*w+nx] = true
				queue = append(queue, [2]int{nx, ny})
			}
		}
	}
	return out
}

// VarLeft is the variable set before a dungeon's walked commands: how many things on the
// floor that are set off once and shown on the map (chests and the like) are still there.
const VarLeft = "floor_left"

// Walked is the dungeon's walked commands when the party has just arrived at the start of
// a floor it has seen whole (explored 100%), with VarLeft set; else nil. Arriving anywhere
// else (the stairs up, a jump to the stairs down) does not give them.
func Walked(db *data.DB, st *rpg.State) []data.Command {
	d := Of(db, st)
	if d == nil || len(d.Walked) == 0 {
		return nil
	}
	start, ok := d.StartChar()
	if g := d.Generate; g != nil && g.Start != "" {
		start, ok = []rune(g.Start)[0], true
	}
	if !ok || d.At(st.Pos.X, st.Pos.Y) != start {
		return nil
	}
	w, h := d.Size()
	m := st.Mapped[floorKey(st, st.Location)]
	if len(m) != w*h || Explored(d, m) < 100 {
		return nil
	}
	left := 0
	for y := range h {
		for x := range w {
			c := CellAt(d, st, x, y)
			t := d.Tile(c.Char)
			if t != nil && t.Once && t.Mark.Valid && Trigger(t, !c.Passable()) != "step" && !Spent(st, st.Location, x, y) {
				left++
			}
		}
	}
	st.Vars[VarLeft] = float64(left)
	return append([]data.Command{}, d.Walked...)
}

// Gauge is how full the encounter gauge is (0..1).
func Gauge(st *rpg.State) float64 {
	g := optional.Value[float64](st.Vars["gauge"])
	return g
}

// findPut is the first cell of the floor of loc that open_wall turned into c.
func findPut(st *rpg.State, loc string, c rune) (int, int, bool) {
	prefix := floorKey(st, loc) + "@"
	bx, by, found := 0, 0, false
	for k, v := range st.Put {
		if v != string(c) || !strings.HasPrefix(k, prefix) {
			continue
		}
		var x, y int
		if _, err := fmt.Sscanf(k[len(prefix):], "%d,%d", &x, &y); err != nil {
			continue
		}
		if !found || y < by || (y == by && x < bx) { // the same cell each time (map order is random)
			bx, by, found = x, y, true
		}
	}
	return bx, by, found
}

// OpenWall turns a wall of the map next to the party into the tile ch (stairs that appear
// when the floor is seen whole): the wall in front first, then right, left and behind. With
// no wall beside the party (the middle of a hall), the nearest wall that faces an open cell,
// and the party goes to the cell in front of it. The party faces it. The change is kept on
// the floor (st.Put, saved). It does nothing outside a dungeon or for a tile not in it.
func OpenWall(db *data.DB, st *rpg.State, ch string) bool {
	d := Of(db, st)
	if d == nil || len([]rune(ch)) != 1 || d.Tile([]rune(ch)[0]) == nil {
		return false
	}
	w, h := d.Size()
	wall := func(x, y int) bool {
		if x < 0 || y < 0 || x >= w || y >= h || d.At(x, y) != data.CellWall {
			return false
		}
		_, put := st.Put[doneKey(st, st.Location, x, y)]
		return !put
	}
	// the walls beside a cell, the way it faces first
	beside := func(x, y, facing int) (int, bool) {
		for _, turn := range []int{0, 1, 3, 2} {
			f := (facing + turn) & 3
			if wall(x+dx[f], y+dy[f]) {
				return f, true
			}
		}
		return 0, false
	}
	type cell struct{ x, y int }
	from := cell{st.Pos.X, st.Pos.Y}
	seen := map[cell]bool{from: true}
	queue := []cell{from}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if f, ok := beside(c.x, c.y, st.Pos.Facing); ok {
			if st.Put == nil {
				st.Put = map[string]string{}
			}
			st.Put[doneKey(st, st.Location, c.x+dx[f], c.y+dy[f])] = ch
			st.Pos.X, st.Pos.Y, st.Pos.Facing = c.x, c.y, f
			st.Bumped = ""
			Reveal(db, st)
			return true
		}
		for f := range 4 {
			n := cell{c.x + dx[f], c.y + dy[f]}
			if seen[n] || !standable(d, n.x, n.y) || !CellAt(d, st, n.x, n.y).Passable() {
				continue
			}
			seen[n] = true
			queue = append(queue, n)
		}
	}
	return false
}
