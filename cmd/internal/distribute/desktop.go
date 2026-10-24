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
	Version string `json:"version"`
	Target  string `json:"target"`
	Player  string `json:"player"`
	SHA256  string `json:"sha256"`
}

// Desktop assembles an existing game with a matching local runtime bundle.
// The caller provides the CLI version; foreign targets are inspected, never run.
func Desktop(src, dst, bundle, target, version string) error {
	return cli.Publish(dst, []string{src, bundle}, func(stage string) error {
		b, err := os.ReadFile(filepath.Join(bundle, "runtime.json")) //nolint:gosec // Explicit caller-selected runtime bundle.
		if err != nil {
			return err
		}
		var r Runtime
		if err := json.Unmarshal(b, &r); err != nil {
			return fmt.Errorf("runtime metadata: %w", err)
		}
		if r.Version != version || r.Target != target {
			return fmt.Errorf("runtime is %s for %s; need %s for %s", r.Version, r.Target, version, target)
		}
		player := "gozen-player"
		if strings.HasPrefix(target, "windows-") {
			player += ".exe"
		}
		if r.Player != player {
			return fmt.Errorf("runtime player must be %s", player)
		}
		source := filepath.Join(bundle, player)
		if err := copyFile(source, filepath.Join(stage, player), 0o755); err != nil {
			return err
		}
		if err := verifyPlayer(filepath.Join(stage, player), r); err != nil {
			return err
		}
		for _, name := range []string{"LICENSE", "THIRD_PARTY_LICENSES"} {
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
		if _, err := CopyGame(src, filepath.Join(stage, "game")); err != nil {
			return err
		}
		return cli.WriteNew(filepath.Join(stage, "runtime.json"), b)
	})
}

func verifyPlayer(name string, r Runtime) error {
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
	if hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		return errors.New("runtime player checksum mismatch")
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
