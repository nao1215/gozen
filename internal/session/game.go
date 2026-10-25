package session

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/layer1/yamlpath"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Diagnostic is one problem of a game: file, line, column, logical path, message and
// severity ("error" or "warning"), the same fields as gozen check --format json.
type Diagnostic = gamedata.Diagnostic

// Diagnostics are the problems of a game, in file and line order.
type Diagnostics = gamedata.Diagnostics

// Game is a game loaded from YAML. It does not change after Load, and goroutines may
// share it: each Session plays on its own copy of what changes while playing.
type Game struct {
	fsys fs.FS
	db   *gamedata.DB // the definitions; each session plays on its own fork of them
}

// Load reads game.yaml and data/ under dir in fsys and checks them. A problem in the
// definitions returns an error that is ErrInvalid and holds the Diagnostics; a
// missing directory returns the fs error. fsys is borrowed: keep it open.
//
// Each extra file system is laid over the game directory, in order: its files replace
// or add to the game's (a test's own data/zz_test.yaml, a changed game.yaml).
//
// Load also sets the language of the engine's own words (menus, battle messages) from
// game.yaml's language and text. That setting is one for the process, not one a game:
// the game loaded last decides it for every session (a constraint for now; ADR 0021).
func Load(fsys fs.FS, dir string, extra ...fs.FS) (*Game, error) {
	if fsys == nil {
		return nil, fmt.Errorf("%w: nil filesystem", ErrInvalid)
	}
	if len(extra) > 0 {
		sub, err := fs.Sub(fsys, dir)
		if err != nil {
			return nil, err
		}
		fsys, dir = layered{append([]fs.FS{sub}, extra...)}, "."
	}
	if sub, err := fs.Sub(fsys, dir); err == nil {
		// The language of engine text (menus and so on) is game.yaml's language. There is one per
		// process (a work loaded later wins).
		if err := gamedata.UseWords(sub); err != nil {
			return nil, err
		}
	}
	db, err := gamedata.LoadDir(fsys, dir)
	if err != nil {
		var ds Diagnostics
		if errors.As(err, &ds) && !errors.Is(err, fs.ErrNotExist) {
			return nil, invalid{ds}
		}
		return nil, err
	}
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		return nil, err
	}
	return &Game{fsys: sub, db: db}, nil
}

// Validate is Load that returns every problem, warnings included, instead of a game.
// It is what gozen check reports, without the fonts, the text layout and the warnings of
// flags set but never read or read but never set (the game's Go code may use them).
func Validate(fsys fs.FS, dir string) Diagnostics {
	if fsys == nil {
		return Diagnostics{{Message: "nil filesystem", Severity: "error"}}
	}
	db, err := gamedata.LoadDir(fsys, dir)
	if err != nil {
		return gamedata.Diagnose(err, "error")
	}
	warnings := append(append(db.AIWarnings(), db.SetWarnings()...), db.SceneWarnings()...)
	return gamedata.Diagnose(errors.Join(warnings...), "warning")
}

// ID is the game's id in game.yaml.
func (g *Game) ID() string { return g.db.Game.ID }

// IDs are the IDs of a section of data/ ("events", "locations", "items", "troops", ...)
// in the order they are written. An unknown section has none.
func (g *Game) IDs(section string) []string {
	return append([]string(nil), g.db.Ordered(section)...)
}

// FS is the game's directory (borrowed from Load; do not close it).
func (g *Game) FS() fs.FS { return g.fsys }

// Check checks a list of commands (YAML starting with "- ") or one expression against
// the game, without running it: the same checks as Load. Nil means no problem; the file
// of each diagnostic is "<script>". The sets of stats and levels are checked as for a
// session with Config.Dev; Session.Do refuses them without it.
func (g *Game) Check(src string) Diagnostics {
	return gamedata.CheckScript(g.db, src, true)
}

// YAML is the definition at a logical path as written, in YAML: a section of data/ (any
// file under it)
// ("events.inn.do", "locations.camp.people[0]") or of game.yaml ("start.party"). The
// defaults are not filled in. An unknown path is ErrNotFound.
func (g *Game) YAML(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: an empty path", ErrNotFound)
	}
	root, err := g.source(strings.SplitN(path, ".", 2)[0])
	if err != nil {
		return nil, err
	}
	b, err := yaml.Marshal(root)
	if err != nil {
		return nil, err
	}
	out, err := yamlpath.At(b, path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}
	return out, err
}

// source is the YAML that holds a top-level key: game.yaml, or the data/ files merged.
func (g *Game) source(key string) (yaml.MapSlice, error) {
	read := func(name string) (yaml.MapSlice, error) {
		b, err := fs.ReadFile(g.fsys, name)
		if err != nil {
			return nil, err
		}
		var m yaml.MapSlice
		if err := yaml.UnmarshalWithOptions(b, &m, yaml.UseOrderedMap()); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, name, err)
		}
		return m, nil
	}
	game, err := read("game.yaml")
	if err != nil {
		return nil, err
	}
	for _, it := range game {
		if fmt.Sprint(it.Key) == key {
			return game, nil
		}
	}
	var files []string // the .yaml and .yml files under data/ in name order, as in Load (including data/scenes/ and so on)
	err = fs.WalkDir(g.fsys, "data", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")) {
			files = append(files, p)
		}
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var merged yaml.MapSlice
	for _, f := range files {
		m, err := read(f)
		if err != nil {
			return nil, err
		}
		for _, it := range m {
			if fmt.Sprint(it.Key) != key {
				continue
			}
			sec, ok := it.Value.(yaml.MapSlice)
			if !ok {
				continue
			}
			if i := indexOf(merged, key); i >= 0 {
				if prev, ok := merged[i].Value.(yaml.MapSlice); ok {
					merged[i].Value = append(prev, sec...)
				}
			} else {
				merged = append(merged, yaml.MapItem{Key: key, Value: sec})
			}
		}
	}
	if merged == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return merged, nil
}

func indexOf(m yaml.MapSlice, key string) int {
	for i, it := range m {
		if fmt.Sprint(it.Key) == key {
			return i
		}
	}
	return -1
}
