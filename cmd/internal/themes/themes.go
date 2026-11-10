// Package themes holds the dungeon themes the gozen command copies into a game
// (gozen theme add, ADR 0032). Only the command imports it: the pictures are embedded
// here, not in the runtime, so a game that uses no theme does not carry them.
package themes

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

//go:embed assets
var assets embed.FS

// summaries describes each theme in one line; a test keeps it equal to the embedded folders.
var summaries = map[string]string{
	"cave":    "rough rock walls, a dirt and stone floor",
	"ice":     "blocks of ice, a frosted stone floor",
	"ruins":   "stone walls grown with moss and vines, a bare stone floor",
	"castle":  "stone walls, a wooden floor, a brick ceiling",
	"crystal": "purple rock set with crystals",
	"temple":  "white stone with teal tiles",
	"sewer":   "wet stone, grates and pipes",
	"forest":  "walls of roots, an earth floor",
	"tomb":    "sandstone blocks with carved signs",
	"keep":    "gray stone with pillars and cracked flags",
	"dream":   "purple walls of eyes, a sleeping face above",
	"mine":    "timbered shafts, a cobbled floor",
	"vine":    "dark bricks hung with ivy, a mossy floor",
}

// licenseName is the file the license of the pictures is copied as, beside each theme.
const licenseName = "LICENSE.txt"

// Theme is one theme: its name and what it looks like.
type Theme struct{ Name, Summary string }

// List is the themes, sorted by name.
func List() []Theme {
	ents, err := fs.ReadDir(assets, "assets")
	if err != nil {
		panic(err) // the embedded folder is part of the binary
	}
	var l []Theme
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		l = append(l, Theme{e.Name(), summaries[e.Name()]})
	}
	return l
}

// Has reports whether name is one of the themes.
func Has(name string) bool {
	return slices.ContainsFunc(List(), func(t Theme) bool { return t.Name == name })
}

// Files is what Add did with each file of a theme: written, or kept because the game
// already had it.
type Files struct{ Written, Kept []string }

// Add copies the theme name (and the license of its pictures) into assets/dungeon/themes/<name>/
// of the game in gameDir.
// A file the game already has is kept, unless force replaces it. Paths in the result are
// relative to gameDir and slash-separated.
// gameDir must exist, and nothing is written outside it: a folder or a file on the
// way that is a link out of the game is an error, and force replaces a link by the
// theme's file instead of writing to what the link names.
func Add(name, gameDir string, force bool) (r Files, err error) {
	if !Has(name) {
		return r, fmt.Errorf("no theme %q", name)
	}
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return r, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	for _, f := range append(slices.Clone(data.ThemeFiles), licenseName) {
		rel := path.Join("assets", data.ThemeDir(name), f)
		dst := filepath.FromSlash(rel)
		if info, err := root.Lstat(dst); err == nil {
			if !force {
				r.Kept = append(r.Kept, rel)
				continue
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				if err := root.Remove(dst); err != nil {
					return r, err
				}
			}
		}
		from := path.Join("assets", name, f)
		if f == licenseName {
			from = "assets/LICENSE"
		}
		b, err := assets.ReadFile(from)
		if err != nil {
			return r, err
		}
		if err := root.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
			return r, err
		}
		if err := root.WriteFile(dst, b, 0o644); err != nil {
			return r, err
		}
		r.Written = append(r.Written, rel)
	}
	return r, nil
}
