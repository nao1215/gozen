package main

import (
	"context"
	"errors"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	data "github.com/nao1215/gozen/schema"
)

type routeEnding struct {
	Kind    string   `json:"kind"`
	Paths   int      `json:"paths"`
	Example []string `json:"example,omitempty"`
}

type routeOption struct {
	Prompt string `json:"prompt"`
	Option string `json:"option"`
}

type routesReport struct {
	Paths              int           `json:"paths"`
	Truncated          bool          `json:"truncated"`
	TimedOut           bool          `json:"timed_out"`
	Endings            []routeEnding `json:"endings"`
	UnreachableEndings []string      `json:"unreachable_endings"`
	UnofferedOptions   []routeOption `json:"unoffered_options"`
	FlagsSetNeverRead  []string      `json:"flags_set_never_read"`
	Errors             []string      `json:"errors,omitempty"`
}

func (c *command) routes() clihelp.Operation {
	fs := clihelp.NewFlagSet("gozen sim routes")
	maxPaths := fs.Int("paths", 300, "maximum paths to explore")
	maxDepth := fs.Int("depth", 20000, "maximum event requests per path")
	timeout := fs.Duration("timeout", 30*time.Second, "maximum exploration time (for example 10s or 2m)")
	c.reporting(fs)
	failUnreachable := fs.Bool("fail-unreachable", false, "exit 1 when an ending, option, or flag is unreachable")
	return clihelp.Operation{Flags: fs, Run: func(ctx context.Context) (err error) {
		if fs.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected <game-dir>")
		}
		if *maxPaths <= 0 || *maxDepth <= 0 || *timeout <= 0 {
			return cliio.Invalid(diag.BadValue, "--paths, --depth, and --timeout must be positive")
		}
		fsys, err := gameFS(fs.Arg(0))
		if err != nil {
			return err
		}
		if err := gamedata.UseWords(fsys); err != nil {
			return err
		}
		db, err := data.Load(fsys, ".")
		if err != nil {
			return err
		}
		st := rpg.New(db, rand.New(rand.NewPCG(1, 2))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		var initial []data.Command
		if event := db.Events[db.Game.Start.Event]; event != nil {
			initial = event.Do
		}
		paths := headless.Explore(db, st, initial, headless.Config{Context: ctx, MaxPaths: *maxPaths, MaxSteps: *maxDepth, Seed: 1})
		report := routesReport{Paths: len(paths), Endings: []routeEnding{}, UnreachableEndings: []string{}, UnofferedOptions: []routeOption{}, FlagsSetNeverRead: []string{}}
		counts := map[string]int{}
		examples := map[string][]string{}
		offered := map[string]bool{}
		for _, p := range paths {
			if p.Left > 0 {
				report.Truncated = true
			}
			for _, e := range p.Errs {
				report.Errors = append(report.Errors, e.Error())
			}
			kind := ""
			if p.Cleared {
				kind = "clear"
			} else if p.Over {
				kind = "game_over"
			}
			if kind != "" {
				counts[kind]++
				if examples[kind] == nil {
					examples[kind] = append([]string(nil), p.Taken...)
				}
			}
			for _, step := range p.Steps {
				if step.Kind == "choice" {
					for _, option := range step.Options {
						offered[step.Prompt+"\x00"+option] = true
					}
				}
			}
		}
		if ctx.Err() != nil {
			report.TimedOut = true
			report.Truncated = false
			report.Errors = append(report.Errors, "exploration timed out")
		}
		for kind, n := range counts {
			report.Endings = append(report.Endings, routeEnding{Kind: kind, Paths: n, Example: examples[kind]})
		}
		if db.Game.GameOver.Event != "" && counts["game_over"] == 0 {
			report.UnreachableEndings = append(report.UnreachableEndings, "game_over")
		}
		if hasClear(db) && counts["clear"] == 0 {
			report.UnreachableEndings = append(report.UnreachableEndings, "clear")
		}
		for _, ev := range db.Events {
			collectOptions(ev.Do, offered, &report.UnofferedOptions)
		}
		for _, loc := range db.Locations {
			collectOptions(loc.OnEnter, offered, &report.UnofferedOptions)
			for _, person := range loc.People {
				collectOptions(person.Talk, offered, &report.UnofferedOptions)
			}
			for _, action := range loc.Actions {
				collectOptions(action.Do, offered, &report.UnofferedOptions)
			}
			for _, exit := range loc.Exits {
				collectOptions(exit.Do, offered, &report.UnofferedOptions)
			}
		}
		for _, warning := range db.FlagWarnings() {
			message := warning.Error()
			const prefix, suffix = "flag.", " is set but nothing reads it"
			if i := strings.LastIndex(message, prefix); i >= 0 && strings.Contains(message, suffix) {
				name, _, _ := strings.Cut(message[i+len(prefix):], " ")
				if name != "" {
					report.FlagsSetNeverRead = append(report.FlagsSetNeverRead, name)
				}
			}
		}
		sort.Strings(report.FlagsSetNeverRead)
		sort.Slice(report.Endings, func(i, j int) bool { return report.Endings[i].Kind < report.Endings[j].Kind })
		sort.Strings(report.UnreachableEndings)
		sort.Slice(report.UnofferedOptions, func(i, j int) bool {
			a, b := report.UnofferedOptions[i], report.UnofferedOptions[j]
			if a.Prompt != b.Prompt {
				return a.Prompt < b.Prompt
			}
			return a.Option < b.Option
		})
		var failure error
		if len(report.Errors) > 0 {
			failure = errors.New(strings.Join(report.Errors, "; "))
		}
		if *failUnreachable && (len(report.UnreachableEndings) > 0 || len(report.UnofferedOptions) > 0 || len(report.FlagsSetNeverRead) > 0) {
			failure = errors.Join(failure, errors.New("unreachable endings, choice options, or flags found"))
		}
		if c.report.Machine() {
			status := "ok"
			if failure != nil {
				status = "failed"
			}
			if c.report.Format == "jsonl" {
				for _, ending := range report.Endings {
					if err := c.report.Item(c.out, ending.Kind, "ok", ending); err != nil {
						return err
					}
				}
			}
			if err := c.report.Result(c.out, status, report); err != nil {
				return err
			}
			if failure != nil {
				return cliio.ErrReported
			}
			return nil
		}
		printRoutes(c, report)
		return failure
	}}
}

func hasClear(db *data.DB) bool {
	for _, ev := range db.Events {
		if commandsHaveClear(ev.Do) {
			return true
		}
	}
	for _, loc := range db.Locations {
		if commandsHaveClear(loc.OnEnter) {
			return true
		}
		for _, person := range loc.People {
			if commandsHaveClear(person.Talk) {
				return true
			}
		}
		for _, action := range loc.Actions {
			if commandsHaveClear(action.Do) {
				return true
			}
		}
		for _, exit := range loc.Exits {
			if commandsHaveClear(exit.Do) {
				return true
			}
		}
		if loc.Dungeon != nil {
			for _, tile := range loc.Dungeon.Tiles {
				if commandsHaveClear(tile.Do) {
					return true
				}
			}
		}
	}
	return false
}

func commandsHaveClear(cs []data.Command) bool {
	for _, c := range cs {
		if c.Clear || commandsHaveClear(c.Then) || commandsHaveClear(c.Else) || commandsHaveClear(c.OnWin) || commandsHaveClear(c.OnLose) || commandsHaveClear(c.OnRun) {
			return true
		}
	}
	return false
}

func collectOptions(cs []data.Command, offered map[string]bool, out *[]routeOption) {
	for _, c := range cs {
		for _, o := range c.Choice {
			if !offered[c.Prompt+"\x00"+o.Label] {
				*out = append(*out, routeOption{Prompt: c.Prompt, Option: o.Label})
			}
			collectOptions(o.Do, offered, out)
		}
		collectOptions(c.Then, offered, out)
		collectOptions(c.Else, offered, out)
		collectOptions(c.OnWin, offered, out)
		collectOptions(c.OnLose, offered, out)
		collectOptions(c.OnRun, offered, out)
	}
}

func printRoutes(c *command, r routesReport) {
	c.out.Printf("Explored %d paths", r.Paths)
	if r.Truncated {
		c.out.Print(" (path limit reached)")
	}
	if r.TimedOut {
		c.out.Print(" (time limit reached)")
	}
	c.out.Println()
	for _, e := range r.Endings {
		c.out.Printf("%s: %d paths\n", e.Kind, e.Paths)
		if len(e.Example) > 0 {
			c.out.Printf("  example: %v\n", e.Example)
		}
	}
	for _, e := range r.UnreachableEndings {
		c.out.Printf("unreachable ending: %s\n", e)
	}
	for _, o := range r.UnofferedOptions {
		c.out.Printf("unoffered option: %q (%q)\n", o.Option, o.Prompt)
	}
	for _, flag := range r.FlagsSetNeverRead {
		c.out.Printf("flag set but never read: %s\n", flag)
	}
	for _, err := range r.Errors {
		c.out.Printf("exploration: %s\n", err)
	}
}
