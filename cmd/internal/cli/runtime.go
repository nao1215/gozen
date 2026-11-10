package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// RuntimePath reports whether a slash-separated path of a game tree ships with
// the game: its YAML, its runtime asset formats and its license notices. Hidden
// paths, programming files and authoring records do not. gozen export and
// gozen trial both select with it, so a trial holds what an export of it takes.
func RuntimePath(name string, dir bool) bool {
	for part := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	top, _, nested := strings.Cut(name, "/")
	switch top {
	case "data", "lang", "streams":
		return dir || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
	case "assets", "LICENSES":
		// Runtime assets may include fonts, puppet YAML, audio and notices.
		// Programming files and authoring records are never runtime assets.
		if dir || top == "LICENSES" || Notice(name) {
			return true
		}
		switch strings.ToLower(path.Ext(name)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ogg", ".wav", ".mp3", ".ttf", ".otf", ".ttc", ".yaml", ".yml", ".txt", ".md":
			return true
		case ".json":
			return name == "assets/faces.json"
		}
		return false
	default:
		return !nested && !dir && (name == "game.yaml" || name == "LICENSE" || name == "LICENSE.txt" || name == "LICENSE.md" || name == "NOTICE" || name == "NOTICE.txt" || name == "ASSETS.md")
	}
}

// Notice reports whether a file is named as a license or a notice, which ships
// from anywhere under assets/ whatever its extension.
func Notice(name string) bool {
	base := path.Base(name)
	return strings.HasPrefix(base, "LICENSE") || strings.HasPrefix(base, "NOTICE")
}

// WalkRuntime calls visit, in lexical order, for each directory and file of the
// game tree that RuntimePath selects. A symlink anywhere in the tree fails the
// walk, as do a selected name that is not portable and a selected file that is
// not a regular one: none of them can be distributed.
func WalkRuntime(fsys fs.FS, visit func(name string, d fs.DirEntry) error) error {
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks cannot be exported: %s", name)
		}
		if !RuntimePath(name, d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !Relative(name) {
			return fmt.Errorf("non-portable game path: %s", name)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("not a regular file: %s", name)
		}
		return visit(name, d)
	})
}

// CopyFile copies the regular file name of fsys to the new file dst, creating
// its directory. It refuses a symlink or any other irregular file, where a
// filesystem can tell (os.DirFS can), and never overwrites.
func CopyFile(fsys fs.FS, name, dst string, mode fs.FileMode) (err error) {
	info, err := fs.Lstat(fsys, name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", name)
	}
	in, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, in.Close()) }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode) //nolint:gosec // Caller-owned staging directory; never overwrites an existing file.
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, out.Close()) }()
	_, err = io.Copy(out, in)
	return err
}
