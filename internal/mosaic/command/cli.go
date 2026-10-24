// Package command registers mosaic and model-management CLI operations.
package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
)

type cli struct {
	out, diag *cliio.Output
	report    *cliio.Report
}

// Config provides invocation-owned streams and the action selected by dispatch.
// Constructing an operation performs no file, model or environment reads.
type Config struct {
	Action              string
	Output, Diagnostics *cliio.Output
	Report              *cliio.Report
}

// New registers the selected mosaic or models operation for the root dispatcher.
func New(cfg Config) clihelp.Operation {
	c := &cli{out: cfg.Output, diag: cfg.Diagnostics, report: cfg.Report}
	var op clihelp.Operation
	switch cfg.Action {
	case "mosaic auto":
		op = c.autoCmd()
	case "mosaic inspect":
		op = c.inspectCmd()
	case "models list":
		op = c.fetchCmd(true)
	case "models fetch":
		op = c.fetchCmd(false)
	case "mosaic list", "mosaic show", "mosaic view", "mosaic add", "mosaic drop", "mosaic grow", "mosaic skip", "mosaic mark", "mosaic clear":
		op = c.manualCmd(cfg.Action[len("mosaic "):])
	default:
		op = clihelp.Operation{Flags: newFlagSet("gozen " + cfg.Action), Page: topPage, Run: func(context.Context) error {
			if cfg.Action != "mosaic" {
				return cliio.Invalid(diag.UnknownCommand, "unknown command %q", cfg.Action)
			}
			return cliio.Invalid(diag.CommandMissing, "select a mosaic action; use --help")
		}}
	}
	cfg.Report.Command = cfg.Action
	op.Flags.StringVar(&cfg.Report.Format, "format", "text", "result encoding: text, json, or jsonl")
	clihelp.Choices(op.Flags.Lookup("format"), "text", "json", "jsonl")
	op.Page.Groups = append(append([]clihelp.Group{}, op.Page.Groups...), clihelp.Group{Title: "Result", Flags: []string{"format"}})
	run := op.Run
	op.Run = func(ctx context.Context) error {
		if err := cfg.Report.Validate(); err != nil {
			return err
		}
		if cfg.Report.Machine() {
			c.diag = &cliio.Output{Writer: progress{*cfg.Report, cfg.Diagnostics}}
		}
		err := run(ctx)
		if c.diag.Err != nil {
			err = errors.Join(err, c.diag.Err)
		}
		if err == nil || errors.Is(err, cliio.ErrReported) {
			return err
		}
		if _, ok := errors.AsType[*cliio.UsageError](err); ok {
			return err
		}
		return fmt.Errorf("%w\n%s", err, requirements)
	}
	return op
}

func newFlagSet(name string) *pflag.FlagSet { return clihelp.NewFlagSet(name) }

func (c *cli) result(data any) error {
	if c.report != nil && c.report.Machine() {
		return c.report.Result(c.out, "ok", data)
	}
	return nil
}

// progress keeps runtime messages separate from machine-readable results.
type progress struct {
	report cliio.Report
	out    *cliio.Output
}

func (p progress) Write(b []byte) (int, error) {
	err := json.NewEncoder(p.out).Encode(struct {
		Version  int    `json:"version"`
		Command  string `json:"command"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	}{1, p.report.Command, "info", strings.TrimSuffix(string(b), "\n")})
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
