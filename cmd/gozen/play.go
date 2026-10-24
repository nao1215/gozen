package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/nao1215/gozen/internal/cliio"

	runtime "github.com/nao1215/gozen/internal/play"
)

func (c *command) playRun(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := runtime.Main(ctx, nil, "", args, runtime.EntryConfig{Directory: true, Output: c.out})
	if arg, ok := errors.AsType[*runtime.ArgumentError](err); ok {
		return cliio.Parse(arg.Err)
	}
	if err != nil {
		return fmt.Errorf("run: %w\nStartup requirements: https://nao1215.github.io/gozen/commands/#run", err)
	}
	return nil
}
