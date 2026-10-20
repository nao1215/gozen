//go:build !js

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
	fsys, err := gameFS()
	if err == nil {
		err = play.Main(ctx, fsys, ".", os.Args[1:], play.Config{Output: os.Stdout})
	}
	if err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
