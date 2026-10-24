package distribute

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/cmd/internal/cli"
)

//go:embed index.html
var webPage []byte

// Web assembles a static website from a game and a matching WASM runtime.
// The runtime must carry the wasm_exec.js from the same Go build.
func Web(src, dst, bundle, version string) error {
	return cli.Publish(dst, []string{src, bundle}, func(stage string) error {
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
		files, err := CopyGame(src, filepath.Join(stage, "game"))
		if err != nil {
			return err
		}
		manifest := make([]map[string]any, 0, len(files))
		for _, name := range files {
			info, err := os.Stat(filepath.Join(stage, "game", filepath.FromSlash(name)))
			if err != nil {
				return err
			}
			manifest = append(manifest, map[string]any{"name": name, "file": name, "size": info.Size()})
		}
		b, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		if err := cli.WriteNew(filepath.Join(stage, "game", "files.json"), b); err != nil {
			return err
		}
		return cli.WriteNew(filepath.Join(stage, "index.html"), webPage)
	})
}
