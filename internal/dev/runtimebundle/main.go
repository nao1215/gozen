// Command runtimebundle writes metadata for a freshly built distribution player.
// It runs on the build host, including when the player targets another OS.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	f, err := os.Open(*player)
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
	b, err := json.MarshalIndent(map[string]string{
		"version": *version, "target": *target,
		"player": filepath.Base(*player), "sha256": hex.EncodeToString(h.Sum(nil)),
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o750); err != nil {
		return err
	}
	return os.WriteFile(*out, append(b, '\n'), 0o600)
}
