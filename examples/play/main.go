// Command play runs Wayfinder, an original sample game that shows the public runner.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen/runner"
)

// The game's YAML (game.yaml, data/) and its generated pictures (assets/) are
// embedded as they are laid out in a game directory.
//
//go:embed game
var files embed.FS

func gameFS() (fstest.MapFS, error) {
	sub, err := fs.Sub(files, "game")
	if err != nil {
		return nil, err
	}
	fsys := fstest.MapFS{"assets/regular.ttf": {Data: goregular.TTF}}
	err = fs.WalkDir(sub, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, name)
		if err != nil {
			return err
		}
		fsys[name] = &fstest.MapFile{Data: b}
		return nil
	})
	return fsys, err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fsys, err := gameFS()
	if err == nil {
		err = runner.Run(ctx, fsys, ".", runner.Config{Args: os.Args[1:], Output: os.Stdout})
	}
	if err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
