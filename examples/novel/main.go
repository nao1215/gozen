//go:build !js

// Command novel runs the novel sample of Gozen: a short story in the novel window, with
// choices that change the talk and the last picture, in English and Japanese. It is a
// module of its own, so that its pictures and music do not come with the gozen module.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/nao1215/gozen/play"
)

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
