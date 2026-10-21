package gamedata

import (
	"io/fs"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/diag"
)

// ThemeRoot is the folder of a game's assets (relative to assets/) that holds its dungeon
// themes, one folder for each.
const ThemeRoot = "dungeon/themes"

// The files of a dungeon theme folder. A theme is these five files and nothing else.
const (
	ThemeWall    = "wall.jpg"
	ThemeFloor   = "floor.jpg"
	ThemeCeiling = "ceiling.jpg"
	ThemeDoor    = "door.jpg"
	ThemeColors  = "theme.yaml"
)

// ThemeFiles lists the files of a theme folder, in the order they are copied and checked.
var ThemeFiles = []string{ThemeWall, ThemeFloor, ThemeCeiling, ThemeDoor, ThemeColors}

// ThemeDir is the folder of the theme name relative to assets/.
func ThemeDir(name string) string { return ThemeRoot + "/" + name }

// ThemeName reports whether s can be a theme name: lowercase letters, digits, - and _,
// starting with a letter.
func ThemeName(s string) bool {
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '_'):
		default:
			return false
		}
	}
	return s != ""
}

// themeColors is theme.yaml: the colors of the far fog and of the map.
type themeColors struct {
	Floor   Color `yaml:"floor"`
	Ceiling Color `yaml:"ceiling"`
}

// readTheme reads the colors of the theme, failing when the folder lacks a file of the theme.
func (db *DB) readTheme(name string) (themeColors, error) {
	var tc themeColors
	if !ThemeName(name) {
		return tc, fs.ErrInvalid
	}
	for _, f := range ThemeFiles {
		if !db.Exists("assets/" + ThemeDir(name) + "/" + f) {
			return tc, fs.ErrNotExist
		}
	}
	b, err := fs.ReadFile(db.FS, "assets/"+ThemeDir(name)+"/"+ThemeColors)
	if err != nil {
		return tc, err
	}
	err = yaml.UnmarshalWithOptions(b, &tc, yaml.Strict())
	return tc, err
}

// applyThemes fills what the dungeon and its looks leave out from their themes. A theme that
// cannot be read fills nothing; gozen check reports it.
func (db *DB) applyThemes(d *Dungeon) {
	fillTheme(db, d.Theme, &d.Wall, &d.Door, &d.FloorTex, &d.CeilingTex, &d.Floor, &d.Ceiling)
	for i := range d.Looks {
		lk := &d.Looks[i]
		fillTheme(db, lk.Theme, &lk.Wall, &lk.Door, &lk.FloorTex, &lk.CeilingTex, &lk.Floor, &lk.Ceiling)
	}
}

func fillTheme(db *DB, name string, wall, door, floorTex, ceilingTex *Picture, floor, ceiling *Color) {
	if name == "" {
		return
	}
	tc, err := db.readTheme(name)
	if err != nil {
		return
	}
	set := func(p *Picture, file string) {
		if p.File == "" {
			p.File = ThemeDir(name) + "/" + file
		}
	}
	set(wall, ThemeWall)
	set(door, ThemeDoor)
	set(floorTex, ThemeFloor)
	set(ceilingTex, ThemeCeiling)
	if !floor.Valid {
		*floor = tc.Floor
	}
	if !ceiling.Valid {
		*ceiling = tc.Ceiling
	}
}

// theme reports a theme name the game does not have, or whose theme.yaml cannot be read.
func (c *checker) theme(where, name string) {
	if name == "" {
		return
	}
	if !ThemeName(name) {
		c.errorf(diag.NotAWord, where, "%q is not a theme name (lowercase letters, digits, - and _)", name)
		return
	}
	for _, f := range ThemeFiles {
		if !c.db.Exists("assets/" + ThemeDir(name) + "/" + f) {
			c.errorf(diag.ThemeMissing, where, "no assets/%s/%s: run `gozen theme add %s <game-dir>`", ThemeDir(name), f, name)
			return
		}
	}
	if _, err := c.db.readTheme(name); err != nil {
		c.errorf(diag.Shape, where, "assets/%s/%s cannot be read: %v", ThemeDir(name), ThemeColors, err)
	}
}
