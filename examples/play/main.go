// Command play runs an original, generated-art demonstration of the public runner.
package main

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"image/png"
	"os"
	"os/signal"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen/runner"
)

//go:embed game/*.yaml
var files embed.FS

func gameFS() (fstest.MapFS, error) {
	game, err := files.ReadFile("game/game.yaml")
	if err != nil {
		return nil, err
	}
	world, err := files.ReadFile("game/world.yaml")
	if err != nil {
		return nil, err
	}
	fsys := fstest.MapFS{"game.yaml": {Data: game}, "data/world.yaml": {Data: world}, "assets/regular.ttf": {Data: goregular.TTF}}
	for name, img := range artwork() {
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return nil, err
		}
		fsys["assets/"+name] = &fstest.MapFile{Data: b.Bytes()}
	}
	return fsys, nil
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
