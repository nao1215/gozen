// Command runtimebundle writes metadata for a freshly built distribution player.
// It runs on the build host, including when the player targets another OS.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err) //nolint:errcheck // Terminal error path: failure to report still exits nonzero.
		os.Exit(1)
	}
}

func run() error {
	player := flag.String("player", "", "compiled player path")
	target := flag.String("target", "", "GOOS-GOARCH")
	version := flag.String("version", "", "engine release version")
	out := flag.String("out", "", "metadata output path")
	flag.Parse()
	if *player == "" || *target == "" || *version == "" || *out == "" {
		return errors.New("player, target, version and out are required")
	}
	meta := map[string]string{"version": *version, "target": *target, "player": filepath.Base(*player)}
	hash, err := checksum(*player)
	if err != nil {
		return err
	}
	meta["sha256"] = hash
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root, err := exec.CommandContext(ctx, "go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("locate build toolchain: %w", err)
	}
	goRoot := strings.TrimSpace(string(root))
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		return err
	}
	license, err := os.ReadFile(filepath.Join(goRoot, "LICENSE")) //nolint:gosec // License of the active build toolchain.
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(*out), "GO-LICENSE"), license, 0o600); err != nil { //nolint:gosec // Explicit maintainer-selected bundle output.
		return err
	}
	if *target == "js-wasm" {
		source := filepath.Join(goRoot, "lib", "wasm", "wasm_exec.js")
		b, err := os.ReadFile(source) //nolint:gosec // Matching JavaScript runtime from the active Go toolchain.
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(*out), "wasm_exec.js"), b, 0o600); err != nil { //nolint:gosec // Explicit maintainer-selected bundle output.
			return err
		}
		sum := sha256.Sum256(b)
		meta["wasm_exec_sha256"] = hex.EncodeToString(sum[:])
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o600)
}

func checksum(name string) (string, error) {
	f, err := os.Open(name) //nolint:gosec // Maintainer-selected compiled player.
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, readErr := io.Copy(h, f)
	closeErr := f.Close()
	if readErr != nil {
		return "", readErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
