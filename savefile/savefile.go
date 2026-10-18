package savefile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config selects the application's save root. An empty Root uses UserConfigDir/gozen.
type Config struct{ Root string }

// Prepare creates a private directory for id. It never probes or migrates other
// applications' saves. id must be a single portable ASCII path component.
func Prepare(id string, cfg Config) (string, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\:") {
		return "", fmt.Errorf("save id %q: %w", id, fs.ErrInvalid)
	}
	for _, r := range id {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !valid {
			return "", fmt.Errorf("save id %q: %w", id, fs.ErrInvalid)
		}
	}
	upper := strings.ToUpper(id)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" ||
		(len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9') {
		return "", fmt.Errorf("save id %q: %w", id, fs.ErrInvalid)
	}
	root := cfg.Root
	if root == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(base, "gozen")
	}
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Load reads bytes from path within fsys without closing or modifying fsys.
func Load(fsys fs.FS, path string) ([]byte, error) {
	if fsys == nil || !fs.ValidPath(path) {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrInvalid}
	}
	return fs.ReadFile(fsys, path)
}

// Write syncs and closes a private temporary file before replacing path.
// Parent directories are created with mode 0700. Failed writes preserve the old
// destination. Rename durability across power failure is filesystem-dependent.
func Write(path string, b []byte) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".save-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		if removeErr := os.Remove(tmp); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
