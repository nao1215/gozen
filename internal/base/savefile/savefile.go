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

// ValidID reports whether id is a portable save-directory component. The game
// loader uses the same check as Prepare so accepted identities can be saved on
// every supported desktop platform. IDs contain at most 255 ASCII bytes.
// It performs no filesystem operations.
func ValidID(id string) bool {
	if id == "" || len(id) > 255 || id == "." || id == ".." || strings.ContainsAny(id, "/\\:") {
		return false
	}
	for _, r := range id {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_'
		if !valid {
			return false
		}
	}
	return !ReservedName(id)
}

// ReservedName reports whether name, a file name without its extension, is a device
// name of Windows in any case (CON, PRN, AUX, NUL, COM1 to COM9, LPT1 to LPT9): such a
// file cannot be made there. Save ids and the paths of game packages share this list.
func ReservedName(name string) bool {
	upper := strings.ToUpper(name)
	return upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" ||
		(len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9')
}

// Prepare creates a private directory for id. It never probes or migrates other
// applications' saves. id must be a single portable ASCII path component.
func Prepare(id string, cfg Config) (string, error) {
	if !ValidID(id) {
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

// Write syncs and closes a private temporary file before replacing path.
// Parent directories are created with mode 0700. Failed writes preserve the old
// destination. Rename durability across power failure is filesystem-dependent.
func Write(path string, b []byte) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := Stage(dir, ".save-*.tmp", b)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, Unstage(tmp)) }()
	return os.Rename(tmp, path)
}

// Stage writes b to a new private temporary file in dir, named by pattern as
// os.CreateTemp names it, then syncs and closes it. It returns the file's name for the
// caller to publish (a rename, or a hard link that never replaces) and then Unstage. On
// an error no file is left. Write and the package cache share it, so both publish only
// bytes that reached the disk.
func Stage(dir, pattern string, b []byte) (name string, err error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name = f.Name()
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		return "", errors.Join(err, Unstage(name))
	}
	return name, nil
}

// Unstage removes a staged file; one already renamed away is not an error.
func Unstage(name string) error {
	if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
