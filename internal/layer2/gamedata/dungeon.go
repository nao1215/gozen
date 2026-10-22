package gamedata

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nao1215/gozen/internal/diag"
)

type Dungeon struct {
	Map string `yaml:"map"`
	// Theme names a folder of assets/dungeon/themes/ (see ThemeDir) that fills wall, door,
	// floor_texture, ceiling_texture, floor and ceiling where they are not written. Written
	// values win. Loading fills them; gozen check reports a theme that is not in the game.
	Theme   string           `yaml:"theme"`
	Tiles   map[string]*Tile `yaml:"tiles"`
	Wall    Picture          `yaml:"wall"`    // the walls' texture
	Door    Picture          `yaml:"door"`    // the doors' texture
	Floor   Color            `yaml:"floor"`   // colors of the floor and the ceiling
	Ceiling Color            `yaml:"ceiling"` //
	// FloorTex and CeilingTex are the floor and ceiling textures (one picture per cell, tinted by
	// the floor and ceiling colors; color only if absent).
	FloorTex   Picture `yaml:"floor_texture"`
	CeilingTex Picture `yaml:"ceiling_texture"`
	Depth      int     `yaml:"depth"` // how many cells ahead are seen (default 5)
	Dark       float64 `yaml:"-"`     // how fast it darkens far away, 0..1 (default 0.5)
	// DarkSet is the darkness as written (nil if unspecified; writing 0 means not dark). Use Dark
	// after loading.
	DarkSet *float64 `yaml:"darkness"`
	NoMap   bool     `yaml:"no_map"` // no map in the corner
	// Pictures replaces tile pictures (original picture file -> replacement). Set per section in
	// looks' pictures
	// (for example to match the stairs picture to a section's walls).
	Pictures map[string]string `yaml:"pictures"`
	// Looks change how the dungeon looks while their when holds (the first that holds):
	// deeper floors of a made dungeon with other walls, colors and battle background.
	Looks []DungeonLook `yaml:"looks"`
	// Roamers are strong foes seen walking on a made floor (symbols): each moves a cell when
	// the party walks; meeting one starts its battle, and beaten it is gone from the floor.
	Roamers []Roamer `yaml:"roamers"`
	// Generate makes the map anew on each dive (the dive command): a maze of rooms and
	// corridors with the tiles placed (instead of map).
	Generate *Generate `yaml:"generate"`
	// Complete is what happens once the whole floor has been seen (explored reaches 100):
	// a reward for walking all of it. Once for each floor (a made floor anew on each dive).
	Complete []Command `yaml:"complete"`
	// Walked is the command run on arriving at the start of a fully explored (100%) floor (for
	// example asking
	// whether to jump to the down stairs).
	// var.floor_left (the number of remaining once-only objects that are marked on the map, such
	// as treasure chests) is set beforehand.
	Walked []Command `yaml:"walked"`

	Grid [][]rune `yaml:"-"` // the map, rows of the same width
}

// MaxDungeonSide bounds each map dimension. Path finding and generation allocate
// per-cell buffers; a 255 by 255 map keeps those buffers bounded.
const MaxDungeonSide = 255

// MaxDungeonRooms bounds generation attempts and room placement work.
const MaxDungeonRooms = 1024

// DungeonLook is another look of a dungeon (see Dungeon.Looks). What it leaves empty stays
// as the dungeon says.
type DungeonLook struct {
	When Expr `yaml:"when"`
	// Theme is the dungeon's Theme for this look: it fills what the look does not write.
	Theme string `yaml:"theme"`
	// Area is the name (the section's name) shown in place of the place's area (the small top-left
	// line and the floor band) during this look.
	Area       string  `yaml:"area"`
	Wall       Picture `yaml:"wall"`
	Door       Picture `yaml:"door"`
	Floor      Color   `yaml:"floor"`
	Ceiling    Color   `yaml:"ceiling"`
	FloorTex   Picture `yaml:"floor_texture"`
	CeilingTex Picture `yaml:"ceiling_texture"`
	// Dark is the darkness as written (0 if unspecified; DarkSet or HasDark tells whether 0 was
	// written).
	Dark     float64  `yaml:"-"`
	DarkSet  *float64 `yaml:"darkness"`
	BattleBG Picture  `yaml:"battle_background"`
	Music    MusicCue `yaml:"music"`  // the song while this look holds (the area's own; default the location's)
	NoMap    bool     `yaml:"no_map"` // no map in the corner while this look holds (a dark floor)
	// BattleMusic is the music for ordinary fights during this look (the enemy group's music takes
	// priority; otherwise the place's battle_music).
	BattleMusic MusicCue `yaml:"battle_music"`
	// Pictures replaces tile pictures during this look (original -> replacement; layered over the
	// dungeon's pictures).
	Pictures map[string]string `yaml:"pictures"`
}

// Pic is the picture p as the dungeon shows it now (swapped by its pictures).
func (d *Dungeon) Pic(p Picture) Picture {
	if f, ok := d.Pictures[p.File]; ok && f != "" {
		p.File = f
	}
	return p
}

// HasDark reports whether this look writes a darkness (also true when 0 was written).
func (lk DungeonLook) HasDark() bool { return lk.DarkSet != nil }

// Roamer is a kind of strong foe walking a made floor (see Dungeon.Roamers).
type Roamer struct {
	farGiven bool        // spec version 1 states far explicitly; 0 also counts as a distance setting.
	Name     string      `yaml:"name"`    // shown when it is in front of the party
	Picture  Picture     `yaml:"picture"` // standing in its cell
	Size     float64     `yaml:"size"`    // the picture's height, a wall's being 1 (default 0.8)
	Mark     Color       `yaml:"mark"`    // its color on the map
	Count    Expr        `yaml:"count"`   // how many on a floor (an expression of floor; default 1)
	Troops   []TroopPick `yaml:"troops"`  // its battle, drawn by weight from those whose when holds
	// Move is how it walks: chase (comes after the party once it sees it within sight cells,
	// else wanders), wander (a cell a step at random) or guard (stays where it is).
	// A hunt foe (a pursuer) approaches by the nearest path even when unseen (it is always shown
	// on the map).
	Move  string `yaml:"move"`
	Sight int    `yaml:"sight"` // default 4
	// Far is the minimum number of steps from the start (default 6). An appear foe is not placed
	// within
	// this many steps of the floor's start (near the up stairs); default 0, so as not to block the
	// escape route.
	Far int `yaml:"far"`
	// Appear is the condition (when written, the foe is not placed on arrival but appears at the
	// farthest floor cell
	// of that floor once the condition holds, and vanishes when it stops holding). Not combined
	// with count.
	Appear Expr `yaml:"appear"`
	// Wait is the number of steps after arriving on the floor (and after a meeting) before it
	// appears (a floor expression; default 0).
	Wait Expr `yaml:"wait"`
	// Every is how many steps the player must take for it to move one cell (a floor expression;
	// default 1; 3 means one cell per 3 steps).
	Every Expr `yaml:"every"`
	// Icon is the small picture shown on the map (a circle in the mark color if absent). Without
	// picture, it is also placed in the scene.
	Icon Picture `yaml:"icon"`
	// OnAppear and OnLeave are the commands run when it appears and when it vanishes because the
	// condition broke (not run when it vanishes by changing floors).
	OnAppear []Command `yaml:"on_appear"`
	OnLeave  []Command `yaml:"on_leave"`
	// Meet is the command run on contact (replaces the troops fight when written; the foe leaves
	// the floor on contact).
	Meet []Command `yaml:"meet"`
}

// Generate is how a dungeon's floors are made: the size of the maze (odd numbers), how
// many rooms, and the characters placed on its floor (count an expression of floor).
type Generate struct {
	numbersResolved bool    // spec version 1 keeps a loop rate of 0.
	Width           int     `yaml:"width"`
	Height          int     `yaml:"height"`
	Rooms           int     `yaml:"rooms"`
	Size            Expr    `yaml:"size"`
	Loops           float64 `yaml:"loops"` // the part of the walls between corridors opened (default 0.08)
	Start           string  `yaml:"start"` // the character the party arrives at
	Place           []Put   `yaml:"place"` // the others, placed apart from the start
	// Seed is the map seed (when written, every save gets the same map; otherwise a seed decided
	// per save).
	// Per-floor seeds and hand-written maps go in Floors.
	Seed *uint64 `yaml:"seed"`
	// Floors is how each floor is made (keyed by floor number): either a seed for that floor alone
	// or a hand-written map.
	Floors map[int]FloorPlan `yaml:"floors"`
}

// FloorPlan is how one floor of a generated dungeon is made (Generate.Floors).
type FloorPlan struct {
	// Seed is this floor's seed (takes priority over Generate's seed).
	Seed *uint64 `yaml:"seed"`
	// Map is this floor's hand-written map (# . D and tile characters; contains one start
	// character).
	// Characters written in the map count toward place's count, and place fills only the shortfall
	// (if more are written, they stay as they are).
	// The floor size (size) is not used. Put the reason for writing anything in Note.
	Map  string `yaml:"map"`
	Note string `yaml:"note"`
}

// SeedFor is the seed of the floor's map when the data fixes it (ok false: the save's own seed).
func (g *Generate) SeedFor(floor int) (uint64, bool) {
	if f, ok := g.Floors[floor]; ok && f.Seed != nil {
		return *f.Seed, true
	}
	if g.Seed != nil {
		return *g.Seed, true
	}
	return 0, false
}

// Drawn is the hand-drawn map of the floor ("" for a made one).
func (g *Generate) Drawn(floor int) string {
	return strings.Trim(g.Floors[floor].Map, "\n")
}

// Put is a character placed on a made floor: how many (an expression of floor, the floor's
// number), the farthest from the start (far: stairs down) or anywhere.
type Put struct {
	Char  string `yaml:"char"`
	Count Expr   `yaml:"count"` // default 1
	Far   bool   `yaml:"far"`
	// At puts it at a cell of the map (x, y from the top left; the cell is opened and joined
	// to the rest): an event at a set place. Steps puts it about that many steps (walked) from
	// the start: an event met on the way.
	At    *[2]int `yaml:"at"`
	Steps int     `yaml:"steps"`
	// Wall puts it in a wall (a solid tile: stairs drawn on the wall, look: door): in a wall
	// cell beside exactly one open cell, at the end of a dead end first. With far or steps,
	// the open cell beside it is the one counted.
	Wall bool `yaml:"wall"`
	// Near puts it on the open cell beside the cell of another character placed before it
	// (the stairs down: a guard in front of them), when walling that cell off shuts away
	// only the other character's cell (a dead end). Nothing is put where no such cell is.
	Near string `yaml:"near"`
}

// Tile is what a character of a dungeon's map is.
type Tile struct {
	Name    string    `yaml:"name"`    // shown when it is in front of the party
	Look    string    `yaml:"look"`    // floor (default), wall or door
	Picture Picture   `yaml:"picture"` // standing in the cell (on the wall for look: wall)
	Size    float64   `yaml:"size"`    // the picture's height, a wall's being 1 (default 0.6)
	When    Expr      `yaml:"when"`    // the tile is there while this holds; else it is Else
	Else    string    `yaml:"else"`    // the character it is otherwise (default ".")
	Solid   Expr      `yaml:"solid"`   // the party cannot walk in while this holds
	Trigger string    `yaml:"trigger"` // step, look or bump
	Once    bool      `yaml:"once"`
	After   string    `yaml:"after"` // with once: the character it is once used (an opened chest)
	Do      []Command `yaml:"do"`
	Start   bool      `yaml:"start"`  // where the party arrives (when a move has no at)
	Facing  string    `yaml:"facing"` // the way the party faces on arriving here
	Mark    Color     `yaml:"mark"`   // its color on the map (default: none, a plain cell)
	Symbol  string    `yaml:"symbol"` // a letter drawn over the mark on the map (↑ for stairs up, say)
	// Ask is the answer that sets off a trigger look tile, asked when the party walks into
	// it (or presses the button in front of it), beside the answer that does nothing
	// (default: the lang entry for 「調べる」).
	Ask string `yaml:"ask"`
	// Question is the question a trigger look tile asks (default: its name). A tile that
	// looks like a harmless one but is not (a chest that bites) can hint at it here.
	Question string `yaml:"question"`
	// Hint is written after the name when it is in front of the party (the engine writes
	// nothing there otherwise): what stepping on it does, so the party can choose to walk round it (a floor hurting who steps on it).
	Hint string `yaml:"hint"`
	// Slide carries the party on from the cell: north, east, south or west (a stream, a
	// rail), or facing (on the way it walked: ice). It goes on cell by cell until the way is
	// shut (a wall, a door, something solid) or it comes to another tile (not a slide), where
	// it stops; another slide tile on the way turns it to that tile's way.
	Slide string `yaml:"slide"`
	// Spin turns the party when it stops on the cell: right, left, back or random.
	Spin string `yaml:"spin"`
	// Flat lays the picture on the floor of the cell (a magic circle, an arrow cut in the
	// stone) instead of standing it up: seen in perspective, size the side of its square
	// (default 0.6), its top toward the north of the map (a slide's toward its way).
	Flat bool `yaml:"flat"`
	// Away turns a flat picture's top away from the party, whichever way it looks (the
	// steps of a hole going down: seen from the side they looked like a grate).
	Away bool `yaml:"away"`
}

// SlideWay is the facing a slide tile carries to (0..3), given the way the party moves
// (for slide: facing); -1 when the tile does not slide.
func (t *Tile) SlideWay(moving int) int {
	if t == nil || t.Slide == "" {
		return -1
	}
	if t.Slide == "facing" {
		return moving & 3
	}
	return FacingIndex(t.Slide)
}

// The built-in characters of a map.
const (
	CellWall  = '#'
	CellFloor = '.'
	CellDoor  = 'D'
	CellNone  = ' '
)

// Facings are the ways to face, clockwise from north (up on the map).
var Facings = []string{"north", "east", "south", "west"}

// FacingIndex is the index of a facing in Facings (-1 when unknown; "" is north).
func FacingIndex(s string) int {
	switch s {
	case "", "north", "n", "up":
		return 0
	case "east", "e", "right":
		return 1
	case "south", "s", "down":
		return 2
	case "west", "w", "left":
		return 3
	}
	return -1
}

// parse makes the grid from the map.
func (d *Dungeon) parse() {
	lines := strings.Split(strings.TrimRight(d.Map, "\n "), "\n")
	w := 0
	for _, ln := range lines {
		w = max(w, min(utf8.RuneCountInString(ln), MaxDungeonSide))
	}
	d.Grid = nil
	for _, ln := range lines[:min(len(lines), MaxDungeonSide)] {
		row := make([]rune, 0, w)
		for _, r := range ln {
			if len(row) == w {
				break
			}
			row = append(row, r)
		}
		for len(row) < w {
			row = append(row, CellNone)
		}
		d.Grid = append(d.Grid, row)
	}
	if d.Depth == 0 {
		d.Depth = 5
	}
	d.Dark = 0.5 // distinguish unspecified from 0: if written, even 0 is kept as is
	if d.DarkSet != nil {
		d.Dark = *d.DarkSet
	}
	for i := range d.Looks {
		if lk := &d.Looks[i]; lk.DarkSet != nil {
			lk.Dark = *lk.DarkSet
		}
	}
	for _, t := range d.Tiles {
		if t.Size == 0 {
			t.Size = 0.6
		}
		if t.Look == "" {
			t.Look = "floor"
		}
	}
}

// Size is the map's width and height.
func (d *Dungeon) Size() (int, int) {
	if len(d.Grid) == 0 {
		return 0, 0
	}
	return len(d.Grid[0]), len(d.Grid)
}

// At is the character at x, y (a space outside the map).
func (d *Dungeon) At(x, y int) rune {
	if y < 0 || y >= len(d.Grid) || x < 0 || x >= len(d.Grid[y]) {
		return CellNone
	}
	return d.Grid[y][x]
}

// Find is the first cell with the character (reading the rows top to bottom).
func (d *Dungeon) Find(c rune) (x, y int, ok bool) {
	for y, row := range d.Grid {
		for x, r := range row {
			if r == c {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// Tile is the definition of a character (nil for the built-in ones).
func (d *Dungeon) Tile(c rune) *Tile { return d.Tiles[string(c)] }

// StartChar is the character where the party arrives when a move has no at.
func (d *Dungeon) StartChar() (rune, bool) {
	for _, row := range d.Grid {
		for _, r := range row {
			if t := d.Tile(r); t != nil && t.Start {
				return r, true
			}
		}
	}
	return 0, false
}

func builtin(c rune) bool {
	return c == CellWall || c == CellFloor || c == CellDoor || c == CellNone
}

// checkDungeon reports the mistakes of a dungeon location.
func (c *checker) dungeon(w string, l *Location) {
	d := l.Dungeon
	w += ".dungeon"
	if len(l.People) > 0 || len(l.Actions) > 0 || len(l.Exits) > 0 || l.Encounters.Search != "" {
		c.errorf(diag.Shape, w, "a dungeon has no people, actions, exits or search: put them on tiles")
	}
	c.theme(w+".theme", d.Theme)
	c.picture(w+".wall", d.Wall)
	c.picture(w+".door", d.Door)
	c.pictures(w+".pictures", d.Pictures)
	c.looks(w, d)
	c.roamers(w, d)
	c.commands(w+".complete", d.Complete, FieldNames)
	c.commands(w+".walked", d.Walked, FieldNames)
	if d.Dark < 0 || d.Dark > 1 {
		c.errorf(diag.OutOfRange, w+".darkness", "want 0 to 1")
	}
	if d.Depth < 0 {
		c.errorf(diag.OutOfRange, w+".depth", "want 1 or more")
	}
	// The way the map and placements are made depends on whether it is generated (generate) or
	// hand-written (map). Tile checks are shared.
	used := map[rune]bool{}
	notUsed := "neither on the map nor any tile's else"
	if g := d.Generate; g != nil {
		notUsed = "not placed by generate (nor any tile's else or after)"
		c.generate(w, d, used)
	} else {
		if strings.TrimSpace(d.Map) == "" {
			c.errorf(diag.Shape, w+".map", "the map is empty")
			return
		}
		width, height := mapDimensions(d.Map)
		if width > MaxDungeonSide || height > MaxDungeonSide {
			c.errorf(diag.OutOfRange, w+".map", "map width and height must be at most %d cells", MaxDungeonSide)
			return
		}
		for y, row := range d.Grid {
			for x, r := range row {
				if !builtin(r) && d.Tile(r) == nil {
					c.errorf(diag.NotAWord, fmt.Sprintf("%s.map (row %d, column %d)", w, y+1, x+1), "%q is neither # . D nor a tile", string(r))
				}
				used[r] = true
			}
		}
		if _, ok := d.StartChar(); !ok {
			c.errorf(diag.Shape, w, "no tile with start: true (where the party arrives)")
		}
	}
	// Also used: tiles opened into a wall by open_wall (such as stairs that appear at 100%
	// exploration)
	c.db.EachCommand(func(cmd *Command) {
		if r := []rune(cmd.OpenWall); len(r) == 1 && d.Tiles[cmd.OpenWall] != nil {
			used[r[0]] = true
		}
	})
	for _, t := range d.Tiles {
		if r := []rune(t.Else); len(r) == 1 {
			used[r[0]] = true // a tile that is only what another becomes
		}
		if r := []rune(t.After); len(r) == 1 {
			used[r[0]] = true
		}
	}
	for _, key := range slices.Sorted(maps.Keys(d.Tiles)) {
		t := d.Tiles[key]
		ww := fmt.Sprintf("%s.tiles.%s", w, key)
		r := []rune(key)
		if len(r) != 1 || builtin(r[0]) {
			c.errorf(diag.Shape, ww, "a tile is one character other than # . D and space")
			continue
		}
		if !used[r[0]] {
			c.errorf(diag.Shape, ww, "%s", notUsed)
		}
		c.tile(ww, d, key, t)
	}
}

// mapDimensions reads the author's full dimensions even though parse keeps only
// a bounded grid until Check can report the invalid map.
func mapDimensions(m string) (width, height int) {
	for row := range strings.SplitSeq(strings.TrimRight(m, "\n "), "\n") {
		width = max(width, utf8.RuneCountInString(row))
		height++
	}
	return width, height
}

// generate checks how a generated dungeon is made (size, arrival cell, the objects to place and
// their count expressions) and
// records the characters that will be placed in used.
func (c *checker) generate(w string, d *Dungeon, used map[rune]bool) {
	g := d.Generate
	if strings.TrimSpace(d.Map) != "" {
		c.errorf(diag.Shape, w+".map", "a dungeon with generate has no map")
	}
	if g.Width < 9 || g.Height < 9 {
		c.errorf(diag.OutOfRange, w+".generate", "width and height are 9 or more")
	}
	if g.Width > MaxDungeonSide || g.Height > MaxDungeonSide {
		c.errorf(diag.OutOfRange, w+".generate", "width and height must be at most %d cells", MaxDungeonSide)
	}
	if g.Rooms > MaxDungeonRooms {
		c.errorf(diag.OutOfRange, w+".generate.rooms", "want at most %d rooms", MaxDungeonRooms)
	}
	if g.Width%2 == 0 || g.Height%2 == 0 {
		c.errorf(diag.OutOfRange, w+".generate", "width and height are odd numbers")
	}
	if g.Rooms < 0 {
		c.errorf(diag.OutOfRange, w+".generate.rooms", "want 0 or more")
	}
	c.expr(w+".generate.size", g.Size, FloorNames)
	if g.Loops < 0 || g.Loops > 1 {
		c.errorf(diag.OutOfRange, w+".generate.loops", "want 0 to 1")
	}
	placeChar := func(ww, ch string) {
		r := []rune(ch)
		if len(r) != 1 || d.Tiles[ch] == nil {
			c.errorf(diag.UnknownReference, ww, "%q is not a tile", ch)
			return
		}
		used[r[0]] = true
	}
	placeChar(w+".generate.start", g.Start)
	for i, p := range g.Place {
		ww := fmt.Sprintf("%s.generate.place[%d]", w, i)
		placeChar(ww+".char", p.Char)
		c.expr(ww+".count", p.Count, FloorNames)
		if p.At != nil && (p.At[0] < 0 || p.At[0] >= g.Width || p.At[1] < 0 || p.At[1] >= g.Height) {
			c.errorf(diag.OutOfRange, ww+".at", "%v is outside the map (%dx%d)", *p.At, g.Width, g.Height)
		}
		if p.Steps < 0 {
			c.errorf(diag.OutOfRange, ww+".steps", "want 0 or more")
		}
		if t := d.Tiles[p.Char]; p.Wall && t != nil && !t.Solid.Set() && t.Look != "wall" {
			c.errorf(diag.Shape, ww+".wall", "a tile put in a wall cannot be walked into: give it solid (or look: wall)")
		}
		if p.Wall && p.At != nil {
			c.errorf(diag.Shape, ww+".wall", "wall is put by far or steps (or anywhere), not at")
		}
		n := 0
		if p.Near != "" {
			r := []rune(p.Near)
			if len(r) != 1 || d.Tiles[p.Near] == nil {
				c.errorf(diag.UnknownReference, ww+".near", "%q is not a tile character", p.Near)
			} else if !slices.ContainsFunc(g.Place[:i], func(q Put) bool { return q.Char == p.Near }) {
				c.errorf(diag.UnknownReference, ww+".near", "%q is not placed before this one", p.Near)
			}
			if p.Wall || p.Far || p.At != nil || p.Steps > 0 {
				c.errorf(diag.Shape, ww, "near goes alone: not with far, at, steps or wall")
			}
		}
		for _, set := range []bool{p.Far, p.At != nil, p.Steps > 0} {
			if set {
				n++
			}
		}
		if n > 1 {
			c.errorf(diag.Shape, ww, "far, at and steps are ways to place it: give one")
		}
	}
	for _, f := range slices.Sorted(maps.Keys(g.Floors)) {
		ww := fmt.Sprintf("%s.generate.floors.%d", w, f)
		fp := g.Floors[f]
		if f < 1 {
			c.errorf(diag.OutOfRange, ww, "a floor is 1 or more")
		}
		m := g.Drawn(f)
		if m == "" {
			if fp.Seed == nil {
				c.errorf(diag.Shape, ww, "give seed or map")
			}
			continue
		}
		if fp.Seed != nil {
			c.errorf(diag.Shape, ww+".seed", "a drawn floor needs no seed (the map is the floor)")
		}
		rows := strings.Split(m, "\n")
		if width, height := mapDimensions(m); width > MaxDungeonSide || height > MaxDungeonSide {
			c.errorf(diag.OutOfRange, ww+".map", "map width and height must be at most %d cells", MaxDungeonSide)
			continue
		}
		width := len([]rune(rows[0]))
		starts := 0
		for y, row := range rows {
			rr := []rune(row)
			if len(rr) != width {
				c.errorf(diag.Shape, fmt.Sprintf("%s.map (row %d)", ww, y+1), "the rows are of one width (%d, not %d)", width, len(rr))
			}
			for x, r := range rr {
				edge := y == 0 || x == 0 || y == len(rows)-1 || x == len(rr)-1
				switch {
				case edge && r != CellWall:
					c.errorf(diag.Shape, fmt.Sprintf("%s.map (row %d, column %d)", ww, y+1, x+1), "the edge of a drawn floor is wall (#)")
				case !builtin(r) && d.Tile(r) == nil:
					c.errorf(diag.NotAWord, fmt.Sprintf("%s.map (row %d, column %d)", ww, y+1, x+1), "%q is neither # . D nor a tile", string(r))
				case string(r) == g.Start:
					starts++
				}
				used[r] = true
			}
		}
		if starts != 1 {
			c.errorf(diag.Shape, ww+".map", "the start %q is drawn once (not %d times)", g.Start, starts)
		}
	}
}

// tile checks one tile (the same for generated and hand-written).
func (c *checker) tile(ww string, d *Dungeon, key string, t *Tile) {
	switch t.Look {
	case "floor", "wall", "door":
	default:
		c.errorf(diag.NotAWord, ww+".look", "want floor, wall or door, not %q", t.Look)
	}
	switch t.Trigger {
	case "", "step", "look", "bump":
	default:
		c.errorf(diag.NotAWord, ww+".trigger", "want step, look or bump, not %q", t.Trigger)
	}
	if t.Ask != "" && t.Trigger != "look" && (t.Trigger != "" || t.Look != "wall") {
		c.errorf(diag.Shape, ww+".ask", "ask is the answer of a trigger look tile (or a look: wall one)")
	}
	if t.Facing != "" && FacingIndex(t.Facing) < 0 {
		c.errorf(diag.NotAWord, ww+".facing", "want north, east, south or west, not %q", t.Facing)
	}
	if t.Slide != "" {
		if t.Slide != "facing" && FacingIndex(t.Slide) < 0 {
			c.errorf(diag.NotAWord, ww+".slide", "want north, east, south, west or facing, not %q", t.Slide)
		}
		if t.Look != "floor" || t.Solid.Set() {
			c.errorf(diag.Shape, ww+".slide", "a slide is a floor one walks onto (not solid, look floor)")
		}
	}
	switch t.Spin {
	case "", "right", "left", "back", "random":
	default:
		c.errorf(diag.NotAWord, ww+".spin", "want right, left, back or random, not %q", t.Spin)
	}
	if t.After != "" {
		if r := []rune(t.After); len(r) != 1 || (!builtin(r[0]) && d.Tiles[t.After] == nil) {
			c.errorf(diag.NotAWord, ww+".after", "%q is neither # . D nor a tile", t.After)
		}
		if !t.Once {
			c.errorf(diag.Shape, ww+".after", "after is for a tile with once: true")
		}
	}
	if t.Else != "" {
		seen := map[string]bool{key: true}
		for e := t.Else; e != ""; {
			er := []rune(e)
			if len(er) != 1 || (!builtin(er[0]) && d.Tiles[e] == nil) {
				c.errorf(diag.NotAWord, ww+".else", "%q is neither # . D nor a tile", e)
				break
			}
			if seen[e] {
				c.errorf(diag.Shape, ww+".else", "the tiles' else go round in a circle")
				break
			}
			seen[e] = true
			if d.Tiles[e] == nil {
				break
			}
			e = d.Tiles[e].Else
		}
	}
	if t.Flat && (t.Look != "floor" || t.Picture.File == "") {
		c.errorf(diag.Shape, ww+".flat", "a picture laid on the floor: give picture, and look floor")
	}
	if t.Flat && t.Size > 1 {
		c.errorf(diag.OutOfRange, ww+".size", "a picture on the floor is a cell at most (1)")
	}
	if t.Size < 0 {
		c.errorf(diag.OutOfRange, ww+".size", "want more than 0")
	}
	c.picture(ww+".picture", t.Picture)
	c.expr(ww+".when", t.When, FieldNames)
	c.expr(ww+".solid", t.Solid, FieldNames)
	c.commands(ww+".do", t.Do, FieldNames)
}

// arrival checks a move's at and facing against where it goes.
func (c *checker) arrival(w, to, at, facing string) {
	if facing != "" && FacingIndex(facing) < 0 {
		c.errorf(diag.NotAWord, w+".facing", "want north, east, south or west, not %q", facing)
	}
	if at == "" {
		return
	}
	l := c.db.Locations[to]
	if l == nil {
		return
	}
	switch {
	case l.Stage != nil:
		if l.Stage.Spots[at] == nil {
			c.errorf(diag.UnknownReference, w+".at", "%s has no spot %q", to, at)
		}
		return
	case l.Map != nil:
		if l.Map.Points[at] == nil {
			c.errorf(diag.UnknownReference, w+".at", "%s has no point %q", to, at)
		}
		return
	case l.Dungeon == nil:
		c.errorf(diag.UnknownReference, w+".at", "%s is not a dungeon, a map or a stage", to)
		return
	}
	r := []rune(at)
	if g := l.Dungeon.Generate; g != nil && len(r) == 1 { // a made dungeon: a character it places
		if at == g.Start {
			return
		}
		for _, p := range g.Place {
			if p.Char == at {
				return
			}
		}
		opened := false // a tile that open_wall puts (stairs that appear)
		c.db.EachCommand(func(cmd *Command) { opened = opened || cmd.OpenWall == at })
		if opened && l.Dungeon.Tiles[at] != nil {
			return
		}
		c.errorf(diag.UnknownReference, w+".at", "%q is not placed by the generate of %s (nor opened by open_wall)", at, to)
		return
	}
	if _, _, ok := l.Dungeon.Find(r[0]); len(r) != 1 || !ok {
		c.errorf(diag.UnknownReference, w+".at", "%q is not on the map of %s", at, to)
	}
}

// SetMap replaces the map (a made floor).
func (d *Dungeon) SetMap(m string) {
	d.Map = m
	d.parse()
}

// looks reports the mistakes of a dungeon's other looks.
func (c *checker) looks(w string, d *Dungeon) {
	for i, lk := range d.Looks {
		ww := fmt.Sprintf("%s.looks[%d]", w, i)
		if !lk.When.Set() {
			c.errorf(diag.Shape, ww+".when", "a look needs when")
		}
		c.expr(ww+".when", lk.When, FieldNames)
		if lk.Music.Song != "" {
			c.music(ww+".music", lk.Music)
		}
		c.music(ww+".battle_music", lk.BattleMusic)
		c.theme(ww+".theme", lk.Theme)
		c.picture(ww+".wall", lk.Wall)
		c.picture(ww+".door", lk.Door)
		c.pictures(ww+".pictures", lk.Pictures)
		c.picture(ww+".battle_background", lk.BattleBG)
		if lk.Dark < 0 || lk.Dark > 1 {
			c.errorf(diag.OutOfRange, ww+".darkness", "want 0 to 1")
		}
	}
}

// pictures checks that replacement pictures exist for the picture replacements.
func (c *checker) pictures(w string, m map[string]string) {
	for _, from := range slices.Sorted(maps.Keys(m)) {
		if from == "" || m[from] == "" {
			c.errorf(diag.Shape, w, "a swap is from a picture to a picture (%q: %q)", from, m[from])
			continue
		}
		c.picture(w+"."+from, Picture{File: m[from]})
	}
}

// roamers reports the mistakes of a dungeon's roamers.
func (c *checker) roamers(w string, d *Dungeon) {
	for i, r := range d.Roamers {
		ww := fmt.Sprintf("%s.roamers[%d]", w, i)
		if d.Generate == nil {
			c.errorf(diag.Shape, ww, "roamers walk made floors only (a dungeon with generate)")
		}
		c.picture(ww+".picture", r.Picture)
		if r.Icon.File != "" {
			c.picture(ww+".icon", r.Icon)
		}
		c.expr(ww+".count", r.Count, FloorNames)
		c.expr(ww+".appear", r.Appear, FloorNames)
		c.expr(ww+".wait", r.Wait, FloorNames)
		c.expr(ww+".every", r.Every, FloorNames)
		if r.Appear.Set() && r.Count.Set() {
			c.errorf(diag.Shape, ww+".count", "a roamer with appear comes one at a time (no count)")
		}
		if r.Picture.File == "" && r.Icon.File == "" {
			c.errorf(diag.Shape, ww+".picture", "a roamer needs a picture or an icon")
		}
		c.commands(ww+".on_appear", r.OnAppear, FieldNames)
		c.commands(ww+".on_leave", r.OnLeave, FieldNames)
		c.commands(ww+".meet", r.Meet, FieldNames)
		if len(r.Troops) == 0 && len(r.Meet) == 0 {
			c.errorf(diag.Shape, ww+".troops", "a roamer needs a troop (or meet)")
		}
		for j, t := range r.Troops {
			_, ok := c.db.Troops[t.Troop]
			c.ref(fmt.Sprintf("%s.troops[%d]", ww, j), "troop", t.Troop, ok)
			c.expr(fmt.Sprintf("%s.troops[%d].when", ww, j), t.When, FieldNames)
		}
		switch r.Move {
		case "", "chase", "wander", "guard", "hunt":
		default:
			c.errorf(diag.NotAWord, ww+".move", "want chase, wander, guard or hunt, not %q", r.Move)
		}
	}
}

// LoopRate is the fraction of corridors opened between each other. Only the legacy spec and Go's
// zero value 0 are filled in.
func (g Generate) LoopRate() float64 {
	if !g.numbersResolved && g.Loops == 0 {
		return 0.08
	}
	return g.Loops
}

// HasFar reports whether the distance from the start point was stated. A positive Go value also
// works.
func (r Roamer) HasFar() bool { return r.farGiven || r.Far != 0 }
