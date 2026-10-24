package distribute

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
)

// Runtime identifies a maintainer-built player and its exact bytes.
type Runtime struct {
	Version        string `json:"version"`
	Target         string `json:"target"`
	Player         string `json:"player"`
	SHA256         string `json:"sha256"`
	WASMExecSHA256 string `json:"wasm_exec_sha256,omitempty"`
}

// Desktop assembles an existing game with a matching local runtime bundle.
// The caller provides the CLI version; foreign targets are inspected, never run.
func Desktop(src, dst, bundle, target, version string) error {
	return cli.Publish(dst, []string{src, bundle}, func(stage string) error {
		player := "gozen-player"
		if strings.HasPrefix(target, "windows-") {
			player += ".exe"
		}
		r, err := prepareRuntime(bundle, stage, target, version, player)
		if err != nil {
			return err
		}
		source := filepath.Join(bundle, player)
		if err := copyFile(source, filepath.Join(stage, player), 0o755); err != nil {
			return err
		}
		if err := verifyPlayer(filepath.Join(stage, player), r); err != nil {
			return err
		}
		if err := copyLicenses(bundle, stage); err != nil {
			return err
		}
		if _, err := CopyGame(src, filepath.Join(stage, "game")); err != nil {
			return err
		}
		return nil
	})
}

func verifyPlayer(name string, r Runtime) error {
	if err := verifyChecksum(name, r.SHA256); err != nil {
		return err
	}
	info, err := buildinfo.ReadFile(name)
	if err != nil {
		return fmt.Errorf("runtime player: %w", err)
	}
	if info.Path != "github.com/nao1215/gozen/cmd/gozen-player" {
		return errors.New("runtime binary is not the Gozen player")
	}
	var goos, goarch string
	for _, s := range info.Settings {
		switch s.Key {
		case "GOOS":
			goos = s.Value
		case "GOARCH":
			goarch = s.Value
		}
	}
	if goos+"-"+goarch != r.Target {
		return fmt.Errorf("runtime binary target does not match %s", r.Target)
	}
	return nil
}

func verifyChecksum(name, expected string) error {
	f, err := os.Open(name) //nolint:gosec // Newly staged player, copied from the selected runtime.
	if err != nil {
		return err
	}
	h := sha256.New()
	_, readErr := io.Copy(h, f)
	closeErr := f.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(h.Sum(nil)) != expected {
		return fmt.Errorf("runtime checksum mismatch: %s", filepath.Base(name))
	}
	return nil
}

// prepareRuntime validates and stages the runtime metadata.
func prepareRuntime(bundle, stage, target, version, player string) (Runtime, error) {
	var r Runtime
	b, err := os.ReadFile(filepath.Join(bundle, "runtime.json")) //nolint:gosec // Explicit caller-selected runtime bundle.
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("runtime metadata: %w", err)
	}
	if r.Version != version || r.Target != target {
		return r, fmt.Errorf("runtime is %s for %s; need %s for %s", r.Version, r.Target, version, target)
	}
	if r.Player != player {
		return r, fmt.Errorf("runtime player must be %s", player)
	}
	if err := cli.WriteNew(filepath.Join(stage, "runtime.json"), b); err != nil {
		return r, err
	}
	return r, nil
}

func copyLicenses(bundle, stage string) error {
	for _, name := range []string{"LICENSE", "GO-LICENSE", "THIRD_PARTY_LICENSES"} {
		root := filepath.Join(bundle, name)
		if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(bundle, p)
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return copyFile(p, filepath.Join(stage, "runtime-licenses", rel), 0o600)
		}); err != nil {
			return err
		}
	}

	return nil
}
