package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"

	runtime "github.com/nao1215/gozen/internal/play"
)

func (c *command) playRun() clihelp.Operation {
	op := runtime.Entry(nil, "", runtime.EntryConfig{Directory: true, Output: c.out})
	start := op.Run
	op.Run = func(parent context.Context) error {
		ctx, stop := signal.NotifyContext(parent, os.Interrupt)
		defer stop()
		err := start(ctx)
		if arg, ok := errors.AsType[*runtime.ArgumentError](err); ok {
			return cliio.Parse(arg.Err)
		}
		if err != nil {
			return fmt.Errorf("run: %w\nStartup requirements: https://nao1215.github.io/gozen/commands/#run", err)
		}
		return nil
	}
	return op
}
