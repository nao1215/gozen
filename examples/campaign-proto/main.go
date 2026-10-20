// Command campaign-proto runs Five Banners, a stage 0 prototype of a region-conquest
// strategy game written only with the YAML words Gozen has today. NOTES.md lists what was
// hard to write. It is a module of its own, so that its pictures do not come with the
// gozen module.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/signal"

	"github.com/nao1215/gozen/play"
)

// The game's YAML (game.yaml, data/) and its pictures and font (assets/).
//
//go:embed game
var files embed.FS

func gameFS() (fs.FS, error) { return fs.Sub(files, "game") }

func main() {
	if err := run(); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

// run returns before main exits, so the deferred stop always runs.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fsys, err := gameFS()
	if err != nil {
		return err
	}
	return play.Main(ctx, fsys, ".", os.Args[1:], play.Config{Output: os.Stdout})
}
