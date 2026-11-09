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

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/base/clihelp"
	"github.com/nao1215/gozen/internal/base/cliio"
	"github.com/nao1215/gozen/internal/base/diag"
	"github.com/nao1215/gozen/internal/screen/play"
)

// scriptConfig holds the shared settings for direct and batch scripted runs.
type scriptConfig struct {
	dir     string
	script  string
	trace   string
	seed    uint64
	saveDir string
	start   string
	fight   string
	party   string
	level   int
	flags   string
	gold    int
	items   string
	floor   int
	calm    bool
	event   string
	line    int
	answers string
}

// scriptFlags is the common input grammar for script invocations and batch cases.
func scriptFlags(cfg *scriptConfig) *pflag.FlagSet {
	fs := clihelp.NewFlagSet("gozen sim script")
	fs.StringVar(&cfg.script, "script", "", "the script `FILE` (the commands of run --script); - reads it line by line from the standard input; required")
	fs.StringVar(&cfg.trace, "trace", "", "write the trace as JSON Lines to this `FILE`; - is the standard output (the default with --script -)")
	fs.Uint64Var(&cfg.seed, "seed", 1, "the random seed; the same seed gives the same result")
	fs.StringVar(&cfg.saveDir, "save-dir", "", "keep the saves in this `DIR` (default: in memory, gone at the end)")
	fs.StringVar(&cfg.start, "start", "", "start a new game at this `LOCATION`")
	fs.StringVar(&cfg.fight, "battle", "", "with --start, begin with a battle against this `TROOP`")
	fs.StringVar(&cfg.party, "party", "", "the party, as actor `IDS` separated by commas")
	fs.IntVar(&cfg.level, "level", 0, "the party's `LEVEL`")
	fs.StringVar(&cfg.flags, "flags", "", "the `FLAGS` to set, separated by commas; var.name=number sets a variable")
	fs.IntVar(&cfg.gold, "gold", 0, "the money, in `GOLD`")
	fs.StringVar(&cfg.items, "items", "", "the `ITEMS` to put in the bag, as id or id=count separated by commas")
	fs.IntVar(&cfg.floor, "floor", 0, "with --start at a generated dungeon, start on this `FLOOR`")
	fs.BoolVar(&cfg.calm, "calm", false, "meet no random battles")
	fs.StringVar(&cfg.event, "event", "", "start this `EVENT` at once (an event id, or location.action) and read on to --line")
	fs.IntVar(&cfg.line, "line", 0, "with --event, stop at this `LINE`, counting lines and questions from its start")
	fs.StringVar(&cfg.answers, "answers", "", "with --event, the `ANSWERS` to the questions on the way, separated by commas (0 is the first)")
	return fs
}

func (c *command) simScript() clihelp.Operation {
	var cfg scriptConfig
	fs := scriptFlags(&cfg)
	return clihelp.Operation{Flags: fs, Run: func(ctx context.Context) error {
		if fs.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		cfg.dir = fs.Arg(0)
		return c.executeScript(ctx, cfg)
	}}
}

// executeScript runs typed settings without command dispatch or argument parsing.
func (c *command) executeScript(parent context.Context, cfg scriptConfig) (err error) {
	if cfg.script == "" {
		return cliio.Invalid(diag.OptionRequired, "--script FILE is required")
	}
	if cfg.level < 0 || cfg.gold < 0 || cfg.floor < 0 || cfg.line < 0 {
		return cliio.Invalid(diag.BadValue, "--level, --gold, --floor, and --line must be nonnegative")
	}
	interactive := cfg.script == "-"
	if interactive && cfg.trace == "" {
		cfg.trace = "-"
	}
	dir := cfg.dir
	if _, err := gameFS(dir); err != nil {
		return err
	}
	fsys := os.DirFS(dir)
	opts := play.Options{
		FS: fsys, DB: c.games.definitions(dir, fsys), Seed: cfg.seed, SaveDir: cfg.saveDir,
		Location: cfg.start, Battle: cfg.fight, Party: commaList(cfg.party), Level: cfg.level, Flags: commaList(cfg.flags),
		Gold: cfg.gold, Items: commaList(cfg.items), Floor: cfg.floor, Calm: cfg.calm, Event: cfg.event, Line: cfg.line,
	}
	for _, a := range commaList(cfg.answers) {
		n, err := strconv.Atoi(strings.TrimSpace(a))
		if err != nil {
			return cliio.Invalid(diag.BadValue, "--answers %q: %v", cfg.answers, err)
		}
		opts.Answers = append(opts.Answers, n)
	}
	var simulation play.SimulateConfig
	if interactive {
		simulation.Lines = os.Stdin
	} else {
		b, err := os.ReadFile(cfg.script)
		if err != nil {
			return err
		}
		opts.Script = string(b)
	}
	switch cfg.trace {
	case "":
	case "-":
		opts.Trace = c.out.Writer
	default:
		if d := filepath.Dir(cfg.trace); d != "." {
			if err := os.MkdirAll(d, 0o700); err != nil {
				return err
			}
		}
		f, err := os.Create(cfg.trace)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, f.Close()) }()
		opts.Trace = f
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt)
	defer stop()
	res, err := play.Simulate(ctx, opts, simulation)
	if arg, ok := errors.AsType[*play.ArgumentError](err); ok {
		return cliio.Invalid(diag.BadValue, "%v", arg.Err)
	}
	if err != nil {
		return fmt.Errorf("sim script: %w", err)
	}
	if cfg.trace == "-" {
		return nil
	}
	return simScriptSummary(c.out.Writer, res, cfg.trace)
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
