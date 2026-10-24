// Command space runs a neutral street, interior and staircase demonstration.
package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/signal"

	"github.com/nao1215/gozen/play"
)

//go:embed game
var game embed.FS

func main() {
	if err := run(); err != nil {
		if _, e := fmt.Fprintln(os.Stderr, err); e != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return play.Main(ctx, game, "game", os.Args[1:], play.Config{Output: os.Stdout})
}
