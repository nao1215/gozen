package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/play"
)

// simScript is gozen sim script: the script of run --dev --script played without a window,
// writing the same trace (play.Simulate).
func (c *command) simScript() clihelp.Operation {
	fs := clihelp.NewFlagSet("gozen sim script")
	script := fs.String("script", "", "the script `FILE` (the commands of run --script); - reads it line by line from the standard input; required")
	trace := fs.String("trace", "", "write the trace as JSON Lines to this `FILE`; - is the standard output (the default with --script -)")
	seed := fs.Uint64("seed", 1, "the random seed; the same seed gives the same result")
	saveDir := fs.String("save-dir", "", "keep the saves in this `DIR` (default: in memory, gone at the end)")
	start := fs.String("start", "", "start a new game at this `LOCATION`")
	fight := fs.String("battle", "", "with --start, begin with a battle against this `TROOP`")
	party := fs.String("party", "", "the party, as actor `IDS` separated by commas")
	level := fs.Int("level", 0, "the party's `LEVEL`")
	flags := fs.String("flags", "", "the `FLAGS` to set, separated by commas; var.name=number sets a variable")
	gold := fs.Int("gold", 0, "the money, in `GOLD`")
	items := fs.String("items", "", "the `ITEMS` to put in the bag, as id or id=count separated by commas")
	floor := fs.Int("floor", 0, "with --start at a generated dungeon, start on this `FLOOR`")
	calm := fs.Bool("calm", false, "meet no random battles")
	event := fs.String("event", "", "start this `EVENT` at once (an event id, or location.action) and read on to --line")
	line := fs.Int("line", 0, "with --event, stop at this `LINE`, counting lines and questions from its start")
	answers := fs.String("answers", "", "with --event, the `ANSWERS` to the questions on the way, separated by commas (0 is the first)")
	return clihelp.Operation{Flags: fs, Run: func(parent context.Context) (err error) {
		if fs.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		if *script == "" {
			return cliio.Invalid(diag.OptionRequired, "--script FILE is required")
		}
		if *level < 0 || *gold < 0 || *floor < 0 || *line < 0 {
			return cliio.Invalid(diag.BadValue, "--level, --gold, --floor, and --line must be nonnegative")
		}
		interactive := *script == "-"
		if interactive && *trace == "" {
			*trace = "-"
		}
		dir := fs.Arg(0)
		if _, err := gameFS(dir); err != nil {
			return err
		}
		opts := play.Options{
			FS: os.DirFS(dir), Seed: *seed, SaveDir: *saveDir,
			Location: *start, Battle: *fight, Party: commaList(*party), Level: *level, Flags: commaList(*flags),
			Gold: *gold, Items: commaList(*items), Floor: *floor, Calm: *calm, Event: *event, Line: *line,
		}
		for _, a := range commaList(*answers) {
			n, err := strconv.Atoi(strings.TrimSpace(a))
			if err != nil {
				return cliio.Invalid(diag.BadValue, "--answers %q: %v", *answers, err)
			}
			opts.Answers = append(opts.Answers, n)
		}
		var cfg play.SimulateConfig
		if interactive {
			cfg.Lines = os.Stdin
		} else {
			b, err := os.ReadFile(*script)
			if err != nil {
				return err
			}
			opts.Script = string(b)
		}
		switch *trace {
		case "":
		case "-":
			opts.Trace = c.out.Writer
		default:
			if d := filepath.Dir(*trace); d != "." {
				if err := os.MkdirAll(d, 0o700); err != nil {
					return err
				}
			}
			f, err := os.Create(*trace)
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, f.Close()) }()
			opts.Trace = f
		}
		ctx, stop := signal.NotifyContext(parent, os.Interrupt)
		defer stop()
		res, err := play.Simulate(ctx, opts, cfg)
		if arg, ok := errors.AsType[*play.ArgumentError](err); ok {
			return cliio.Invalid(diag.BadValue, "%v", arg.Err)
		}
		if err != nil {
			return fmt.Errorf("sim script: %w", err)
		}
		if *trace == "-" {
			return nil
		}
		return simScriptSummary(c.out.Writer, res, *trace)
	}}
}

// simScriptSummary prints how far the run went when the trace is not on the standard output.
func simScriptSummary(w io.Writer, res play.SimulateResult, trace string) error {
	msg := fmt.Sprintf("%d ticks", res.Ticks)
	if trace != "" {
		msg += fmt.Sprintf(", %d trace lines in %s", res.Lines, trace)
	}
	_, err := fmt.Fprintln(w, msg)
	return err
}
