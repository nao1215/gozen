// Package checkdata loads and reports structural validation without graphics.
package checkdata

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/puppet"

	data "github.com/nao1215/gozen/schema"
)

// Load writes a count summary to out and warnings to diagnostics, borrowing all inputs.
func Load(fsys fs.FS, out, diagnostics io.Writer) (*data.DB, error) {
	if err := gamedata.UseWords(fsys); err != nil {
		return nil, unreadable(err)
	}
	db, err := data.Load(fsys, ".")
	if err != nil {
		return nil, unreadable(err)
	}
	// data.Load checks a puppet's YAML and files; the picture sizes and positions
	// are checked by loading it as the game does (without the GPU).
	for _, id := range slices.Sorted(maps.Keys(db.Actors)) {
		if p := db.Actors[id].Puppet; p != "" {
			if _, err := puppet.Load(fsys, "assets/"+p); err != nil {
				return nil, diag.Shape.Errorf("actors.%s.puppet: %w", id, err)
			}
		}
	}
	if _, err = fmt.Fprintf(out, "ok: %d locations, %d events, %d actors, %d skills, %d items, %d enemies, %d troops, %d ai sets\n", len(db.Locations), len(db.Events), len(db.Actors), len(db.Skills), len(db.Items), len(db.Enemies), len(db.Troops), len(db.AI)); err != nil {
		return nil, err
	}
	for _, w := range languageWarnings(fsys, &db.Game) {
		if _, err = fmt.Fprintln(diagnostics, diag.LanguageWords.Mark("warning: "+w)); err != nil {
			return nil, err
		}
	}
	for _, warnings := range [][]error{db.AIWarnings(), db.SetWarnings(), db.SceneWarnings()} {
		for _, w := range warnings {
			if _, err = fmt.Fprintf(diagnostics, "warning: %v\n", w); err != nil {
				return nil, err
			}
		}
	}
	return db, nil
}

// Diagnose loads the game like Load and returns every problem as diagnostics: the
// errors that stop it loading, or else the warnings. db is nil when it did not load.
func Diagnose(fsys fs.FS) (*data.DB, gamedata.Diagnostics) {
	if err := gamedata.UseWords(fsys); err != nil {
		return nil, gamedata.Diagnose(unreadable(err), "error")
	}
	db, err := data.Load(fsys, ".")
	if err != nil {
		return nil, gamedata.Diagnose(unreadable(err), "error")
	}
	var out gamedata.Diagnostics
	for _, id := range slices.Sorted(maps.Keys(db.Actors)) {
		if p := db.Actors[id].Puppet; p != "" {
			if _, err := puppet.Load(fsys, "assets/"+p); err != nil {
				out = append(out, gamedata.Diagnostic{Path: "actors." + id + ".puppet", Message: err.Error(), Severity: "error", Code: diag.Shape.String()})
			}
		}
	}
	if len(out) > 0 {
		return nil, out
	}
	for _, w := range languageWarnings(fsys, &db.Game) {
		out = append(out, gamedata.Diagnostic{File: "game.yaml", Path: "language", Message: w, Severity: "warning", Code: diag.LanguageWords.String()})
	}
	for _, warnings := range [][]error{db.AIWarnings(), db.SetWarnings(), db.SceneWarnings()} {
		out = append(out, gamedata.Diagnose(errors.Join(warnings...), "warning")...)
	}
	return db, out
}

// unreadable gives the code of an unreadable game to an error that is not a
// diagnostic of the definitions: the directory or game.yaml is missing.
func unreadable(err error) error {
	// A missing game.yaml comes as one line without a position.
	if (errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission)) && !strings.Contains(err.Error(), "\n") {
		return diag.GameUnreadable.Wrap(err)
	}
	return err
}
