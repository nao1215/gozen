// Package distribute assembles runtime files for desktop and browser delivery.
// It belongs to command tooling; gameplay does not depend on export policy.
package distribute

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/gamepackage"
)

// copyGame selects runtime content from a prepared game tree into an empty
// staging directory. Approval-controlled inputs must first use release.
// It returns sorted slash-separated filenames for the browser manifest, and the game
// loaded from the copy, which it checks.
func copyGame(ctx context.Context, src, dst string) ([]string, *gamedata.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if _, err := os.Lstat(filepath.Join(src, "asset-policy.yaml")); err == nil {
		return nil, nil, errors.New("game has an asset policy; prepare it with gozen release before exporting")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	if info, err := os.Lstat(src); err != nil {
		return nil, nil, err
	} else if !info.IsDir() {
		return nil, nil, errors.New("game input must be a directory, not a symlink or file")
	}
	var files []string
	game := os.DirFS(src)
	err := cli.WalkRuntime(game, func(name string, d fs.DirEntry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if d.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if err := cli.CopyFile(game, name, target, 0o600); err != nil {
			return err
		}
		files = append(files, name)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	db, err := gamedata.Load(os.DirFS(dst))
	if err != nil {
		return nil, nil, fmt.Errorf("exported game: %w", err)
	}
	if db.Game.Phone != nil {
		extra, err := copyPhonePackages(ctx, src, dst, db.Game.Phone, files)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, extra...)
		slices.Sort(files)
	}
	return files, db, nil
}

// copyPhonePackages includes only declared catalog and bundle files. They are
// verified before export; unrelated archives and authoring records stay out.
func copyPhonePackages(ctx context.Context, src, dst string, cfg *gamedata.PhoneApps, existing []string) ([]string, error) {
	paths := []string{cfg.Catalog}
	for _, a := range cfg.Apps {
		if a.Install == "bundled" {
			paths = append(paths, a.Path)
		}
	}
	var copied []string
	for _, name := range paths {
		if slices.Contains(existing, name) || slices.Contains(copied, name) {
			continue
		}
		if !gamepackage.PortablePath(name) {
			return nil, fmt.Errorf("invalid phone package path %q", name)
		}
		if err := copyPhoneFile(src, dst, name); err != nil {
			return nil, err
		}
		copied = append(copied, name)
	}
	catalogBytes, err := fs.ReadFile(os.DirFS(dst), cfg.Catalog)
	if err != nil {
		return nil, err
	}
	catalog, err := gamepackage.Authenticate(catalogBytes, cfg.CatalogSHA256)
	if err != nil {
		return nil, err
	}
	for _, app := range cfg.Apps {
		if app.Install == "bundled" {
			if err := validatePhoneBundle(ctx, os.DirFS(dst), catalog, app); err != nil {
				return nil, err
			}
		}
	}
	return copied, nil
}

func copyPhoneFile(src, dst, name string) error {
	full := src
	for part := range strings.SplitSeq(name, "/") {
		full = filepath.Join(full, part)
		info, err := os.Lstat(full)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symlinks cannot be exported: %s", name)
		}
	}
	return cli.CopyFile(os.DirFS(src), name, filepath.Join(dst, filepath.FromSlash(name)), 0o600)
}

func validatePhoneBundle(ctx context.Context, fsys fs.FS, catalog gamepackage.Catalog, app gamedata.PhoneApp) error {
	version := app.BundledVersion
	if version == "" {
		version = app.Version
	}
	for _, r := range catalog.Releases {
		if r.ID != app.Package || r.Version != version {
			continue
		}
		inst, err := gamepackage.OpenArchive(ctx, fsys, app.Path, r)
		if err != nil {
			return fmt.Errorf("bundled app %s: %w", app.ID, err)
		}
		_, err = gamedata.Load(inst.FS)
		if err = errors.Join(err, inst.Close()); err != nil {
			return fmt.Errorf("bundled app %s: %w", app.ID, err)
		}
		return nil
	}
	return fmt.Errorf("bundled app %s requires an exact approved version", app.ID)
}
