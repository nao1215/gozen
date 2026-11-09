package distribute

import (
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/gamepackage"
)

//go:embed index.html
var webPage []byte

// Web assembles a static website from a game and a matching WASM runtime.
// The runtime must carry the wasm_exec.js from the same Go build. The player is served
// gzipped (gozen-player.wasm.gz) and the game's files made smaller (shrinkWeb): the page
// and the player undo the gzip themselves, so any static host serves the site.
// reuse, when not empty, is an earlier web export of the game: a file whose content has
// not changed since is taken from it instead of being converted again.
func Web(ctx context.Context, src, dst, bundle, version, reuse string) error {
	earlier, err := readEarlier(reuse)
	if err != nil {
		return err
	}
	return cli.Publish(dst, []string{src, bundle, reuse}, func(stage string) error {
		r, err := prepareRuntime(bundle, stage, "js-wasm", version, "gozen-player.wasm")
		if err != nil {
			return err
		}
		for _, file := range []struct{ name, hash string }{{"gozen-player.wasm", r.SHA256}, {"wasm_exec.js", r.WASMExecSHA256}} {
			name, hash := file.name, file.hash
			if err := copyFile(filepath.Join(bundle, name), filepath.Join(stage, name), 0o600); err != nil {
				return err
			}
			if err := verifyChecksum(filepath.Join(stage, name), hash); err != nil {
				return err
			}
		}
		f, err := os.Open(filepath.Join(stage, "gozen-player.wasm")) //nolint:gosec // Newly staged runtime file.
		if err != nil {
			return err
		}
		var header [8]byte
		n, readErr := io.ReadFull(f, header[:])
		closeErr := f.Close()
		if readErr != nil || n != len(header) || !bytes.Equal(header[:], []byte{0, 'a', 's', 'm', 1, 0, 0, 0}) {
			return errors.New("runtime player is not WebAssembly version 1")
		}
		if closeErr != nil {
			return closeErr
		}
		if err := copyLicenses(bundle, stage); err != nil {
			return err
		}
		if err := gzipPlayer(filepath.Join(stage, "gozen-player.wasm")); err != nil {
			return err
		}
		files, db, err := CopyGame(ctx, src, filepath.Join(stage, "game"))
		if err != nil {
			return err
		}
		if err := checkWebPhonePackages(ctx, os.DirFS(filepath.Join(stage, "game")), db.Game.Phone); err != nil {
			return err
		}
		manifest, err := shrinkWeb(filepath.Join(stage, "game"), files, voiceDir(db), earlier)
		if err != nil {
			return err
		}
		b, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		if err := cli.WriteNew(filepath.Join(stage, "game", "files.json"), b); err != nil {
			return err
		}
		page, err := pageFor(src, stage, db)
		if err != nil {
			return err
		}
		return cli.WriteNew(filepath.Join(stage, "index.html"), page)
	})
}

// gzipPlayer replaces the player with gozen-player.wasm.gz (a fifth of its size).
func gzipPlayer(name string) error {
	b, err := os.ReadFile(name) //nolint:gosec // The newly staged player.
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if err := cli.WriteNew(name+".gz", buf.Bytes()); err != nil {
		return err
	}
	return os.Remove(name)
}

// Native bundles may exceed the browser's bounded in-memory archive reader.
// Reject them before publishing a site whose startup would fetch unusable data.
func checkWebPhonePackages(ctx context.Context, fsys fs.FS, cfg *gamedata.PhoneApps) error {
	if cfg == nil {
		return nil
	}
	b, err := fs.ReadFile(fsys, cfg.Catalog)
	if err != nil {
		return err
	}
	catalog, err := gamepackage.Authenticate(b, cfg.CatalogSHA256)
	if err != nil {
		return err
	}
	for _, app := range cfg.Apps {
		if app.Install != "bundled" {
			continue
		}
		version := app.BundledVersion
		if version == "" {
			version = app.Version
		}
		for _, release := range catalog.Releases {
			if release.ID != app.Package || release.Version != version {
				continue
			}
			if release.Size > gamepackage.MaxArchiveBytes || release.UnpackedSize > gamepackage.MaxUnpackedBytes {
				return fmt.Errorf("bundled app %s exceeds browser package limits (64 MiB archive, 256 MiB unpacked)", app.ID)
			}
			f, err := fsys.Open(app.Path)
			if err != nil {
				return err
			}
			archive, err := io.ReadAll(io.LimitReader(f, release.Size+1))
			if err = errors.Join(err, f.Close()); err != nil {
				return err
			}
			installed, err := gamepackage.VerifyContext(ctx, release, archive)
			if err != nil {
				return fmt.Errorf("bundled app %s browser validation: %w", app.ID, err)
			}
			if err := installed.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
