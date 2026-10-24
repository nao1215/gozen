// Package distribute assembles runtime files for desktop and browser delivery.
// It belongs to command tooling; gameplay does not depend on export policy.
package distribute

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
)

// CopyGame selects runtime content from a prepared game tree into an empty
// staging directory. Approval-controlled inputs must first use pack release.
// It returns sorted slash-separated filenames for the browser manifest.
func CopyGame(src, dst string) ([]string, error) {
	if _, err := os.Lstat(filepath.Join(src, "asset-policy.yaml")); err == nil {
		return nil, errors.New("game has an asset policy; prepare it with gozen pack release before exporting")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var files []string
	err := filepath.WalkDir(src, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, name)
		if err != nil {
			return err
		}
		if rel == "." {
			if !d.IsDir() {
				return errors.New("game input must be a directory, not a symlink or file")
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks cannot be exported: %s", rel)
		}
		if !runtimePath(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !cli.Relative(rel) {
			return fmt.Errorf("non-portable game path: %s", rel)
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, filepath.FromSlash(rel)), 0o750)
		}
		if err := copyFile(name, filepath.Join(dst, filepath.FromSlash(rel)), 0o600); err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, err := gamedata.Load(os.DirFS(dst)); err != nil {
		return nil, fmt.Errorf("exported game: %w", err)
	}
	return files, nil
}

func runtimePath(name string, dir bool) bool {
	for part := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	top, _, nested := strings.Cut(name, "/")
	switch top {
	case "data", "lang":
		return dir || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
	case "assets", "LICENSES":
		// Runtime assets may include fonts, puppet YAML, audio and notices.
		// Programming files and authoring records are never runtime assets.
		if dir || top == "LICENSES" || strings.HasPrefix(path.Base(name), "LICENSE") || strings.HasPrefix(path.Base(name), "NOTICE") {
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

func copyFile(src, dst string, mode fs.FileMode) (err error) {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", src)
	}
	in, err := os.Open(src) //nolint:gosec // Explicit selected file, checked as regular before copying.
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
