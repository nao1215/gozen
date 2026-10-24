package gamedata

import (
	"fmt"
	"io/fs"
)

// LoadDir reads a game directory rooted at path within fsys. Use "." for its root.
// It validates references, expressions and numeric presence. On error it returns nil;
// a problem in the definitions is returned as Diagnostics (errors.As).
// It does not close or mutate fsys; keep it available while using the DB's assets.
func LoadDir(fsys fs.FS, path string) (*DB, error) {
	if fsys == nil || !fs.ValidPath(path) {
		return nil, fmt.Errorf("load game %q: %w", path, fs.ErrInvalid)
	}
	sub, err := fs.Sub(fsys, path)
	if err != nil {
		return nil, err
	}
	db, err := Load(sub)
	if err != nil {
		return nil, Diagnose(err, "error")
	}
	return db, nil
}
