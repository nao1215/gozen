//go:build !js

// Command campaign runs Five Banners, the campaign sample of Gozen: a war of five regions
// over ten turns (schema_version 5, ADR 0030), in English and Japanese. It is a module of
// its own, so that its pictures do not come with the gozen module.
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
