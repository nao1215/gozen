// Package checkdata loads and reports structural validation without graphics.
package checkdata

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/puppet"

	"github.com/nao1215/gozen/data"
)

// Load writes a count summary to out and warnings to diagnostics, borrowing all inputs.
func Load(fsys fs.FS, out, diagnostics io.Writer) (*data.DB, error) {
	if err := gamedata.UseWords(fsys); err != nil {
		return nil, err
	}
	db, err := data.Load(fsys, ".")
	if err != nil {
		return nil, err
	}
	// data.Load checks a puppet's YAML and files; the picture sizes and positions
	// are checked by loading it as the game does (without the GPU).
	for _, id := range slices.Sorted(maps.Keys(db.Actors)) {
		if p := db.Actors[id].Puppet; p != "" {
			if _, err := puppet.Load(fsys, "assets/"+p); err != nil {
				return nil, fmt.Errorf("actors.%s.puppet: %w", id, err)
			}
		}
	}
	if _, err = fmt.Fprintf(out, "ok: %d locations, %d events, %d actors, %d skills, %d items, %d enemies, %d troops, %d ai sets\n", len(db.Locations), len(db.Events), len(db.Actors), len(db.Skills), len(db.Items), len(db.Enemies), len(db.Troops), len(db.AI)); err != nil {
		return nil, err
	}
	for _, w := range languageWarnings(fsys, &db.Game) {
		if _, err = fmt.Fprintf(diagnostics, "warning: %s\n", w); err != nil {
			return nil, err
		}
	}
	for _, warnings := range [][]error{db.AIWarnings(), db.SetWarnings()} {
		for _, w := range warnings {
			if _, err = fmt.Fprintf(diagnostics, "warning: %v\n", w); err != nil {
				return nil, err
			}
		}
	}
	return db, nil
}
