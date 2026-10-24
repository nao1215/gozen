//go:build !js

// Command gozen-player starts the game directory beside its executable.
// Maintainers build it for distribution; authors use gozen pack desktop.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/nao1215/gozen/internal/play"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err) //nolint:errcheck // Terminal error path: failure to report still exits nonzero.
		os.Exit(1)
	}
}

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return play.Main(ctx, os.DirFS(filepath.Join(filepath.Dir(exe), "game")), ".", os.Args[1:], play.EntryConfig{Output: os.Stdout})
}
