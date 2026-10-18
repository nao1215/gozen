// Package checkdata loads and reports structural validation without graphics.
package checkdata

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/nao1215/gozen/data"
)

// Load writes a count summary to out and warnings to diagnostics, borrowing all inputs.
func Load(fsys fs.FS, out, diagnostics io.Writer) (*data.DB, error) {
	db, err := data.Load(fsys, ".")
	if err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintf(out, "ok: %d locations, %d events, %d actors, %d skills, %d items, %d enemies, %d troops, %d ai sets\n", len(db.Locations), len(db.Events), len(db.Actors), len(db.Skills), len(db.Items), len(db.Enemies), len(db.Troops), len(db.AI)); err != nil {
		return nil, err
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
