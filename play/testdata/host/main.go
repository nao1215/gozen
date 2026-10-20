// Command host runs the game directory given with --dir through play.Main: the window of
// the graphics tests and of the CLI's screenshot scenarios (GOZEN_GAME_BIN). It has no game
// of its own.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/nao1215/gozen/play"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := play.Main(ctx, os.DirFS("."), ".", os.Args[1:], play.Config{Output: os.Stdout}); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
