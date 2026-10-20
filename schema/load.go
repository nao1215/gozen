package schema

import (
	"fmt"
	"io/fs"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// LegacySchemaVersion accepts versionless YAML definitions.
const LegacySchemaVersion = gamedata.LegacySchemaVersion

// Load reads a game directory rooted at path within fsys. Use "." for its root.
// It validates references, expressions and numeric presence. On error it returns nil;
// a problem in the definitions is returned as Diagnostics (errors.As).
// It does not close or mutate fsys; keep it available while using the DB's assets.
func Load(fsys fs.FS, path string) (*DB, error) {
	if fsys == nil || !fs.ValidPath(path) {
		return nil, fmt.Errorf("load game %q: %w", path, fs.ErrInvalid)
	}
	sub, err := fs.Sub(fsys, path)
	if err != nil {
		return nil, err
	}
	db, err := gamedata.Load(sub)
	if err != nil {
		return nil, gamedata.Diagnose(err, "error")
	}
	return db, nil
}

// Diagnostic is one problem in a game's YAML: file, line, column, logical path, message
// and severity. gozen check --format json prints the same fields.
type Diagnostic = gamedata.Diagnostic

// Diagnostics are the problems of a game; Load returns them as its error.
type Diagnostics = gamedata.Diagnostics
