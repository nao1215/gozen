package gamedata

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Dungeon struct {
	Map     string           `yaml:"map"`
	Tiles   map[string]*Tile `yaml:"tiles"`
	Wall    Picture          `yaml:"wall"`    // the walls' texture
	Door    Picture          `yaml:"door"`    // the doors' texture
	Floor   Color            `yaml:"floor"`   // colors of the floor and the ceiling
	Ceiling Color            `yaml:"ceiling"` //
	// FloorTex と CeilingTex は 床と 天井の 模様（1 マスに 絵 1 枚。色 floor・ceiling を 掛ける。無ければ 色だけ）
	FloorTex   Picture `yaml:"floor_texture"`
	CeilingTex Picture `yaml:"ceiling_texture"`
	Depth      int     `yaml:"depth"` // how many cells ahead are seen (default 5)
	Dark       float64 `yaml:"-"`     // how fast it darkens far away, 0..1 (default 0.5)
	// DarkSet は書かれた darkness（未指定なら nil。0 を書けば暗くならない）。読み込み後は Dark を使う。
	DarkSet *float64 `yaml:"darkness"`
	NoMap   bool     `yaml:"no_map"` // no map in the corner
	// Pictures は タイルの 絵の 差し替え（元の 絵の ファイル → 代わりの 絵）。looks の pictures で 区画ごとに 変える
	// （階段の 絵を 区画の 壁に 合わせる、など）。
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
	// Walked は、歩きつくした（探索 100%）階の スタートに 着いた ときの コマンド（下り階段へ 飛ぶか 聞く、など）。
	// 前に var.floor_left（まだ 残っている once の 置き物の 数。宝箱など 地図に しるしの ある 物）を 入れる。
	Walked []Command `yaml:"walked"`

	Grid [][]rune `yaml:"-"` // the map, rows of the same width
}

// DungeonLook is another look of a dungeon (see Dungeon.Looks). What it leaves empty stays
// as the dungeon says.
type DungeonLook struct {
	When Expr `yaml:"when"`
	// Area は この 見た目の 間、場所の area（左上の 小さい 行と 階の 帯）の 代わりに 出す 名前（区画の 名前）
	Area       string  `yaml:"area"`
	Wall       Picture `yaml:"wall"`
	Door       Picture `yaml:"door"`
	Floor      Color   `yaml:"floor"`
	Ceiling    Color   `yaml:"ceiling"`
	FloorTex   Picture `yaml:"floor_texture"`
	CeilingTex Picture `yaml:"ceiling_texture"`
	// Dark は書かれた darkness（未指定なら 0。0 を書いたかどうかは DarkSet か HasDark で分かる）。
	Dark     float64  `yaml:"-"`
	DarkSet  *float64 `yaml:"darkness"`
	BattleBG Picture  `yaml:"battle_background"`
	Music    MusicCue `yaml:"music"`  // the song while this look holds (the area's own; default the location's)
	NoMap    bool     `yaml:"no_map"` // no map in the corner while this look holds (a dark floor)
	// BattleMusic は この 見た目の 間の 雑魚戦の 曲（敵グループの music が 先。無ければ 場所の battle_music）
	BattleMusic MusicCue `yaml:"battle_music"`
	// Pictures は この 見た目の 間の タイルの 絵の 差し替え（元の 絵 → 代わりの 絵。迷宮の pictures に 重ねる）
	Pictures map[string]string `yaml:"pictures"`
}

// Pic is the picture p as the dungeon shows it now (swapped by its pictures).
func (d *Dungeon) Pic(p Picture) Picture {
	if f, ok := d.Pictures[p.File]; ok && f != "" {
		p.File = f
	}
	return p
}

// HasDark は、この見た目が darkness を書いているか（0 を書いたときも true）。
func (lk DungeonLook) HasDark() bool { return lk.DarkSet != nil }

// Roamer is a kind of strong foe walking a made floor (see Dungeon.Roamers).
type Roamer struct {
	farGiven bool        // 仕様版１で far を明記した。０も距離の指定として扱う。
	Name     string      `yaml:"name"`    // shown when it is in front of the party
	Picture  Picture     `yaml:"picture"` // standing in its cell
	Size     float64     `yaml:"size"`    // the picture's height, a wall's being 1 (default 0.8)
	Mark     Color       `yaml:"mark"`    // its color on the map
	Count    Expr        `yaml:"count"`   // how many on a floor (an expression of floor; default 1)
	Troops   []TroopPick `yaml:"troops"`  // its battle, drawn by weight from those whose when holds
	// Move is how it walks: chase (comes after the party once it sees it within sight cells,
	// else wanders), wander (a cell a step at random) or guard (stays where it is).
	// hunt（追う者）は 見えなくても いつも 一番 近い 道で 寄って くる（地図に いつも 出る）。
	Move  string `yaml:"move"`
	Sight int    `yaml:"sight"` // default 4
	// Far: placed at least this many cells from the start (default 6). appear の 者は、階の
	// スタート（上り階段の そば）から この 歩数の 内には 出ない（既定 0。逃げ道を 塞がない）。
	Far int `yaml:"far"`
	// Appear は 出る 条件（書けば 潜った 時には 置かず、条件が 成り立った ときに その 階の 一番 遠い 床に 1 体
	// 出る。成り立たなく なると 消える）。count とは 一緒に 書かない。
	Appear Expr `yaml:"appear"`
	// Wait は 階に 着いて から（と、会った 後）出るまでの 歩数（floor の 式。既定 0）。
	Wait Expr `yaml:"wait"`
	// Every は 遊び手が 何歩 歩くと 1 マス 動くか（floor の 式。既定 1。3 なら 3 歩に 1 マス）。
	Every Expr `yaml:"every"`
	// Icon は 地図に 出す 小さな 絵（無ければ mark の 色の 丸）。picture が 無ければ 景色にも これを 立てる。
	Icon Picture `yaml:"icon"`
	// OnAppear・OnLeave は 出た とき・条件が 崩れて 消えた ときの コマンド（階を 移って 消える ときは 動かない）。
	OnAppear []Command `yaml:"on_appear"`
	OnLeave  []Command `yaml:"on_leave"`
	// Meet は 触れた ときの コマンド（書けば troops の 戦闘の 代わり。触れると その 階から 消える）。
	Meet []Command `yaml:"meet"`
}

// Generate is how a dungeon's floors are made: the size of the maze (odd numbers), how
// many rooms, and the characters placed on its floor (count an expression of floor).
type Generate struct {
	numbersResolved bool    // 仕様版１ではループ率０を保持する。
	Width           int     `yaml:"width"`
	Height          int     `yaml:"height"`
	Rooms           int     `yaml:"rooms"`
	Size            Expr    `yaml:"size"`
	Loops           float64 `yaml:"loops"` // the part of the walls between corridors opened (default 0.08)
	Start           string  `yaml:"start"` // the character the party arrives at
	Place           []Put   `yaml:"place"` // the others, placed apart from the start
	// Seed は 地図の 種（書けば どの セーブでも 同じ 地図。書かなければ セーブごとに 決めた 種）。
	// 階ごとの 種と 手書きの 地図は Floors に 書く。
	Seed *uint64 `yaml:"seed"`
	// Floors は 階ごとの 作り方（キーは 階の 番号）: その 階だけの 種か、手で 書いた 地図。
	Floors map[int]FloorPlan `yaml:"floors"`
}

// FloorPlan は 生成の 迷宮の 1 つの 階の 作り方（Generate.Floors）。
type FloorPlan struct {
	// Seed は この 階の 種（Generate の seed より 先）。
	Seed *uint64 `yaml:"seed"`
	// Map は 手で 書いた この 階の 地図（# . D と タイルの 文字。start の 文字を 1 つ 含む）。
	// 地図に 書いた 文字は place の count に 数え、足りない 分だけ place が 置く（書いた 数が 多ければ そのまま）。
	// 階の 広さ（size）は 使わない。何か 書いた 理由は Note に。
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
	// (default: lang の 「調べる」).
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
		w = max(w, len([]rune(ln)))
	}
	d.Grid = nil
	for _, ln := range lines {
		row := []rune(ln)
		for len(row) < w {
			row = append(row, CellNone)
		}
		d.Grid = append(d.Grid, row)
	}
	if d.Depth == 0 {
		d.Depth = 5
	}
	d.Dark = 0.5 // 未指定と 0 を分ける: 書いてあれば 0 でもそのまま
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
		c.errorf(w, "a dungeon has no people, actions, exits or search: put them on tiles")
	}
	c.picture(w+".wall", d.Wall)
	c.picture(w+".door", d.Door)
	c.pictures(w+".pictures", d.Pictures)
	c.looks(w, d)
	c.roamers(w, d)
	c.commands(w+".complete", d.Complete, FieldNames)
	c.commands(w+".walked", d.Walked, FieldNames)
	if d.Dark < 0 || d.Dark > 1 {
		c.errorf(w+".darkness", "want 0 to 1")
	}
	if d.Depth < 0 {
		c.errorf(w+".depth", "want 1 or more")
	}
	// 地図と配置のしかたは、生成（generate）か手書き（map）かで分かれる。タイルの検査は共通。
	used := map[rune]bool{}
	notUsed := "neither on the map nor any tile's else"
	if g := d.Generate; g != nil {
		notUsed = "not placed by generate (nor any tile's else or after)"
		c.generate(w, d, used)
	} else {
		if strings.TrimSpace(d.Map) == "" {
			c.errorf(w+".map", "the map is empty")
			return
		}
		for y, row := range d.Grid {
			for x, r := range row {
				if !builtin(r) && d.Tile(r) == nil {
					c.errorf(fmt.Sprintf("%s.map (row %d, column %d)", w, y+1, x+1), "%q is neither # . D nor a tile", string(r))
				}
				used[r] = true
			}
		}
		if _, ok := d.StartChar(); !ok {
			c.errorf(w, "no tile with start: true (where the party arrives)")
		}
	}
	// open_wall で 壁に 開ける タイル（探索 100% で 現れる 階段など）も 使われて いる
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
			c.errorf(ww, "a tile is one character other than # . D and space")
			continue
		}
		if !used[r[0]] {
			c.errorf(ww, "%s", notUsed)
		}
		c.tile(ww, d, key, t)
	}
}

// generate は生成の迷宮の作り方（大きさ・着くマス・置く物とその数の式）を確かめ、
// 置かれる文字を used に入れる。
func (c *checker) generate(w string, d *Dungeon, used map[rune]bool) {
	g := d.Generate
	if strings.TrimSpace(d.Map) != "" {
		c.errorf(w+".map", "a dungeon with generate has no map")
	}
	if g.Width < 9 || g.Height < 9 {
		c.errorf(w+".generate", "width and height are 9 or more")
	}
	if g.Width%2 == 0 || g.Height%2 == 0 {
		c.errorf(w+".generate", "width and height are odd numbers")
	}
	if g.Rooms < 0 {
		c.errorf(w+".generate.rooms", "want 0 or more")
	}
	c.expr(w+".generate.size", g.Size, FieldNames)
	if g.Loops < 0 || g.Loops > 1 {
		c.errorf(w+".generate.loops", "want 0 to 1")
	}
	placeChar := func(ww, ch string) {
		r := []rune(ch)
		if len(r) != 1 || d.Tiles[ch] == nil {
			c.errorf(ww, "%q is not a tile", ch)
			return
		}
		used[r[0]] = true
	}
	placeChar(w+".generate.start", g.Start)
	for i, p := range g.Place {
		ww := fmt.Sprintf("%s.generate.place[%d]", w, i)
		placeChar(ww+".char", p.Char)
		c.expr(ww+".count", p.Count, FieldNames) // floor（階の番号）も使える（組み込みの名前と同じなので検査は通る）
		if p.At != nil && (p.At[0] < 0 || p.At[0] >= g.Width || p.At[1] < 0 || p.At[1] >= g.Height) {
			c.errorf(ww+".at", "%v is outside the map (%dx%d)", *p.At, g.Width, g.Height)
		}
		if p.Steps < 0 {
			c.errorf(ww+".steps", "want 0 or more")
		}
		if t := d.Tiles[p.Char]; p.Wall && t != nil && !t.Solid.Set() && t.Look != "wall" {
			c.errorf(ww+".wall", "a tile put in a wall cannot be walked into: give it solid (or look: wall)")
		}
		if p.Wall && p.At != nil {
			c.errorf(ww+".wall", "wall is put by far or steps (or anywhere), not at")
		}
		n := 0
		if p.Near != "" {
			r := []rune(p.Near)
			if len(r) != 1 || d.Tiles[p.Near] == nil {
				c.errorf(ww+".near", "%q is not a tile character", p.Near)
			} else if !slices.ContainsFunc(g.Place[:i], func(q Put) bool { return q.Char == p.Near }) {
				c.errorf(ww+".near", "%q is not placed before this one", p.Near)
			}
			if p.Wall || p.Far || p.At != nil || p.Steps > 0 {
				c.errorf(ww, "near goes alone: not with far, at, steps or wall")
			}
		}
		for _, set := range []bool{p.Far, p.At != nil, p.Steps > 0} {
			if set {
				n++
			}
		}
		if n > 1 {
			c.errorf(ww, "far, at and steps are ways to place it: give one")
		}
	}
	for _, f := range slices.Sorted(maps.Keys(g.Floors)) {
		ww := fmt.Sprintf("%s.generate.floors.%d", w, f)
		fp := g.Floors[f]
		if f < 1 {
			c.errorf(ww, "a floor is 1 or more")
		}
		m := g.Drawn(f)
		if m == "" {
			if fp.Seed == nil {
				c.errorf(ww, "give seed or map")
			}
			continue
		}
		if fp.Seed != nil {
			c.errorf(ww+".seed", "a drawn floor needs no seed (the map is the floor)")
		}
		rows := strings.Split(m, "\n")
		width := len([]rune(rows[0]))
		starts := 0
		for y, row := range rows {
			rr := []rune(row)
			if len(rr) != width {
				c.errorf(fmt.Sprintf("%s.map (row %d)", ww, y+1), "the rows are of one width (%d, not %d)", width, len(rr))
			}
			for x, r := range rr {
				edge := y == 0 || x == 0 || y == len(rows)-1 || x == len(rr)-1
				switch {
				case edge && r != CellWall:
					c.errorf(fmt.Sprintf("%s.map (row %d, column %d)", ww, y+1, x+1), "the edge of a drawn floor is wall (#)")
				case !builtin(r) && d.Tile(r) == nil:
					c.errorf(fmt.Sprintf("%s.map (row %d, column %d)", ww, y+1, x+1), "%q is neither # . D nor a tile", string(r))
				case string(r) == g.Start:
					starts++
				}
				used[r] = true
			}
		}
		if starts != 1 {
			c.errorf(ww+".map", "the start %q is drawn once (not %d times)", g.Start, starts)
		}
	}
}

// tile は 1 つのタイルを確かめる（生成でも手書きでも同じ）。
func (c *checker) tile(ww string, d *Dungeon, key string, t *Tile) {
	switch t.Look {
	case "floor", "wall", "door":
	default:
		c.errorf(ww+".look", "want floor, wall or door, not %q", t.Look)
	}
	switch t.Trigger {
	case "", "step", "look", "bump":
	default:
		c.errorf(ww+".trigger", "want step, look or bump, not %q", t.Trigger)
	}
	if t.Ask != "" && t.Trigger != "look" && (t.Trigger != "" || t.Look != "wall") {
		c.errorf(ww+".ask", "ask is the answer of a trigger look tile (or a look: wall one)")
	}
	if t.Facing != "" && FacingIndex(t.Facing) < 0 {
		c.errorf(ww+".facing", "want north, east, south or west, not %q", t.Facing)
	}
	if t.Slide != "" {
		if t.Slide != "facing" && FacingIndex(t.Slide) < 0 {
			c.errorf(ww+".slide", "want north, east, south, west or facing, not %q", t.Slide)
		}
		if t.Look != "floor" || t.Solid.Set() {
			c.errorf(ww+".slide", "a slide is a floor one walks onto (not solid, look floor)")
		}
	}
	switch t.Spin {
	case "", "right", "left", "back", "random":
	default:
		c.errorf(ww+".spin", "want right, left, back or random, not %q", t.Spin)
	}
	if t.After != "" {
		if r := []rune(t.After); len(r) != 1 || (!builtin(r[0]) && d.Tiles[t.After] == nil) {
			c.errorf(ww+".after", "%q is neither # . D nor a tile", t.After)
		}
		if !t.Once {
			c.errorf(ww+".after", "after is for a tile with once: true")
		}
	}
	if t.Else != "" {
		seen := map[string]bool{key: true}
		for e := t.Else; e != ""; {
			er := []rune(e)
			if len(er) != 1 || (!builtin(er[0]) && d.Tiles[e] == nil) {
				c.errorf(ww+".else", "%q is neither # . D nor a tile", e)
				break
			}
			if seen[e] {
				c.errorf(ww+".else", "the tiles' else go round in a circle")
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
		c.errorf(ww+".flat", "a picture laid on the floor: give picture, and look floor")
	}
	if t.Flat && t.Size > 1 {
		c.errorf(ww+".size", "a picture on the floor is a cell at most (1)")
	}
	if t.Size < 0 {
		c.errorf(ww+".size", "want more than 0")
	}
	c.picture(ww+".picture", t.Picture)
	c.expr(ww+".when", t.When, FieldNames)
	c.expr(ww+".solid", t.Solid, FieldNames)
	c.commands(ww+".do", t.Do, FieldNames)
}

// arrival checks a move's at and facing against where it goes.
func (c *checker) arrival(w, to, at, facing string) {
	if facing != "" && FacingIndex(facing) < 0 {
		c.errorf(w+".facing", "want north, east, south or west, not %q", facing)
	}
	if at == "" {
		return
	}
	l := c.db.Locations[to]
	if l == nil {
		return
	}
	if l.Dungeon == nil {
		c.errorf(w+".at", "%s is not a dungeon", to)
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
		c.errorf(w+".at", "%q is not placed by the generate of %s (nor opened by open_wall)", at, to)
		return
	}
	if _, _, ok := l.Dungeon.Find(r[0]); len(r) != 1 || !ok {
		c.errorf(w+".at", "%q is not on the map of %s", at, to)
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
			c.errorf(ww+".when", "a look needs when")
		}
		c.expr(ww+".when", lk.When, FieldNames)
		if lk.Music.Song != "" {
			c.music(ww+".music", lk.Music)
		}
		c.music(ww+".battle_music", lk.BattleMusic)
		c.picture(ww+".wall", lk.Wall)
		c.picture(ww+".door", lk.Door)
		c.pictures(ww+".pictures", lk.Pictures)
		c.picture(ww+".battle_background", lk.BattleBG)
		if lk.Dark < 0 || lk.Dark > 1 {
			c.errorf(ww+".darkness", "want 0 to 1")
		}
	}
}

// pictures は 絵の 差し替えの 代わりの 絵が あるかを 見る。
func (c *checker) pictures(w string, m map[string]string) {
	for _, from := range slices.Sorted(maps.Keys(m)) {
		if from == "" || m[from] == "" {
			c.errorf(w, "a swap is from a picture to a picture (%q: %q)", from, m[from])
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
			c.errorf(ww, "roamers walk made floors only (a dungeon with generate)")
		}
		c.picture(ww+".picture", r.Picture)
		if r.Icon.File != "" {
			c.picture(ww+".icon", r.Icon)
		}
		c.expr(ww+".count", r.Count, FieldNames)
		c.expr(ww+".appear", r.Appear, FieldNames)
		c.expr(ww+".wait", r.Wait, FieldNames)
		c.expr(ww+".every", r.Every, FieldNames)
		if r.Appear.Set() && r.Count.Set() {
			c.errorf(ww+".count", "a roamer with appear comes one at a time (no count)")
		}
		if r.Picture.File == "" && r.Icon.File == "" {
			c.errorf(ww+".picture", "a roamer needs a picture or an icon")
		}
		c.commands(ww+".on_appear", r.OnAppear, FieldNames)
		c.commands(ww+".on_leave", r.OnLeave, FieldNames)
		c.commands(ww+".meet", r.Meet, FieldNames)
		if len(r.Troops) == 0 && len(r.Meet) == 0 {
			c.errorf(ww+".troops", "a roamer needs a troop (or meet)")
		}
		for j, t := range r.Troops {
			_, ok := c.db.Troops[t.Troop]
			c.ref(fmt.Sprintf("%s.troops[%d]", ww, j), "troop", t.Troop, ok)
			c.expr(fmt.Sprintf("%s.troops[%d].when", ww, j), t.When, FieldNames)
		}
		switch r.Move {
		case "", "chase", "wander", "guard", "hunt":
		default:
			c.errorf(ww+".move", "want chase, wander, guard or hunt, not %q", r.Move)
		}
	}
}

// LoopRate は通路間を開ける割合。旧仕様と Go のゼロ値の０だけを補完する。
func (g Generate) LoopRate() float64 {
	if !g.numbersResolved && g.Loops == 0 {
		return 0.08
	}
	return g.Loops
}

// HasFar は開始地点からの距離を明記したか。Go の正値による指定も使える。
func (r Roamer) HasFar() bool { return r.farGiven || r.Far != 0 }
