package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/nao1215/gozen/internal/diag"
)

// Relative accepts a portable slash-separated path within a supplied filesystem.
func Relative(name string) bool {
	if name == "." {
		return true
	}
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") {
		return false
	}
	for part := range strings.SplitSeq(name, "/") {
		if strings.TrimRight(part, ". ") != part {
			return false
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) || strings.ContainsRune(`<>"|?*`, r) {
				return false
			}
		}
	}
	return true
}

// ID is a single portable component, suitable for generated file names.
func ID(name string) bool { return name != "." && Relative(name) && !strings.Contains(name, "/") }

// Canonical resolves symlinks in the existing prefix even for a new destination.
func Canonical(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty path")
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	p := absolute
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			for _, s := range slices.Backward(suffix) {
				resolved = filepath.Join(resolved, s)
			}
			return resolved, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		suffix = append(suffix, filepath.Base(p))
		p = parent
	}
}

func within(p, root string) bool {
	rel, err := filepath.Rel(root, p)
	if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)) {
		return true
	}
	// File identity also catches case and Unicode aliases on filesystems that
	// normalize names, without assuming every macOS volume is case-insensitive.
	base, err := os.Stat(root)
	if err != nil {
		return false
	}
	for current := p; ; current = filepath.Dir(current) {
		if info, err := os.Stat(current); err == nil && os.SameFile(info, base) {
			return true
		}
		if filepath.Dir(current) == current {
			return false
		}
	}
}

// Destination rejects existing outputs and overlap with any protected input tree.
func Destination(dst string, protected ...string) error {
	if _, err := os.Lstat(dst); err == nil {
		return diag.OutputExists.Errorf("output already exists: %s", dst)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	d, err := Canonical(dst)
	if err != nil {
		return err
	}
	for _, src := range protected {
		if src == "" {
			continue
		}
		s, err := Canonical(src)
		if err != nil {
			return err
		}
		if within(d, s) || within(s, d) {
			return diag.OutputOverlaps.Errorf("output must be separate from input: %s", dst)
		}
	}
	return nil
}

// Publish builds in an isolated staging tree and reserves a new destination.
// Existing destinations, including empty directories and symlinks, are never replaced.
func Publish(dst string, protected []string, build func(string) error) (err error) {
	if err = Destination(dst, protected...); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dst), ".gozen-stage-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(stage)) }()
	if err = build(stage); err != nil {
		return err
	}
	if err = os.Mkdir(dst, 0o750); err != nil {
		return err
	}
	if err = os.CopyFS(dst, os.DirFS(stage)); err != nil {
		return errors.Join(err, os.RemoveAll(dst))
	}
	return nil
}

// WriteNew creates a file exclusively and reports both write and close failures.
func WriteNew(name string, b []byte) (err error) {
	if err = os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // Explicit caller-selected input or file within a newly reserved output tree.
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			err = errors.Join(err, os.Remove(name))
		}
	}()
	n, err := f.Write(b)
	if err == nil && n != len(b) {
		err = fmt.Errorf("short write: %s", name)
	}
	return err
}
