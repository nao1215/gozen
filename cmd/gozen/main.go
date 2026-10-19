// Command gozen checks, simulates, documents, and packages game definitions, and
// runs the production helpers (prompt, mosaic) as subcommands.
package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	mosaiccmd "github.com/nao1215/gozen/internal/mosaic/command"
	promptcmd "github.com/nao1215/gozen/internal/promptgen/command"
)

type command struct{ out, stderr *cli.Output }

var usages = map[string]string{
	"check":   "[-no-layout] <game-dir>",
	"layout":  "<game-dir>",
	"sim":     "<game-dir> <troop> [flags]",
	"curve":   "<game-dir> <location> [flags]",
	"floors":  "<game-dir> <location> [flags]",
	"trace":   "<trace.jsonl> [flags]",
	"book":    "<game-dir>",
	"design":  "<game-dir> [flags]",
	"memos":   "<memo-dir> [-out DIR]",
	"release": "-inspect FILE [-overlay DIR] <game-dir> <out-dir>",
	"trial":   "[flags] <game-dir> [out-dir]",
	"shots":   "-bin FILE (-raw | -overlay DIR) [flags] <game-dir> <list.yaml>",
	"version": "",
}

// delegated lists subcommands whose argument parsing and exit codes belong to
// their own internal command packages.
var delegated = [][2]string{
	{"prompt", "[lint] --config FILE [flags]   (see gozen prompt --help)"},
	{"mosaic", "<auto|inspect|manual> [flags] <game-dir> ...   (see gozen mosaic --help)"},
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func (c *command) parse(f *flag.FlagSet, args []string) error {
	parts := strings.Fields(f.Name())
	sub := parts[len(parts)-1]
	req := "No external runtime dependencies; cgo is not required."
	if sub == "shots" {
		req = "A Gozen game binary and its graphics runtime; Linux unattended capture needs Xvfb. See docs/development/graphics.md."
	}
	return cli.Parse(f, args, "gozen "+sub+" "+usages[sub], req, c.out)
}
func (c *command) help() {
	c.out.Println("Usage: gozen <command> [arguments]")
	for _, s := range []string{"check", "layout", "sim", "curve", "floors", "trace", "book", "design", "memos", "release", "trial", "shots"} {
		c.out.Printf("  %-8s %s\n", s, usages[s])
	}
	for _, d := range delegated {
		c.out.Printf("  %-8s %s\n", d[0], d[1])
	}
	c.out.Printf("  %-8s %s\n  %-8s %s\n", "version", "print the build version", "help", "[command]")
	c.out.Println("\nRequirements:\n  No external runtime dependencies for checks, simulations, reports, and prompt; cgo is not required.\n  shots launches a graphics game; Linux unattended capture needs Xvfb.\n  mosaic inference needs a gozen built with cgo, an ONNX Runtime shared library, and user-supplied models.\n  See https://github.com/nao1215/gozen#gozen")
}

// version reports the module version recorded by go install, or devel for source builds.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}

func run(args []string, out, stderr io.Writer) int {
	// "gozen help <command>" is the same as "gozen <command> --help".
	if len(args) == 2 && args[0] == "help" {
		args = []string{args[1], "--help"}
		if args[0] == "help" {
			args = args[1:]
		}
	}
	if len(args) > 0 {
		switch args[0] {
		case "prompt":
			return promptcmd.Run(args[1:], out, stderr)
		case "mosaic":
			return mosaiccmd.Run(args[1:], out, stderr)
		}
	}
	c := &command{&cli.Output{Writer: out}, &cli.Output{Writer: stderr}}
	err := c.execute(args)
	if errors.Is(err, flag.ErrHelp) {
		err = nil
	}
	err = errors.Join(err, c.out.Err, c.stderr.Err)
	if err == nil {
		return 0
	}
	c.stderr.Println(err)
	var u *cli.UsageError
	if errors.As(err, &u) {
		return 2
	}
	return 1
}
func (c *command) execute(args []string) error {
	if len(args) == 0 {
		return cli.Invalid("usage: gozen <command> [arguments]; use --help")
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		if len(args) != 1 {
			return cli.Invalid("help takes no operands")
		}
		c.help()
		return nil
	}
	sub := args[0]
	args = args[1:]
	switch sub {
	case "sim":
		return c.sim(args)
	case "curve":
		return c.curve(args)
	case "floors":
		return c.floorsCmd(args)
	case "trace":
		return c.traceCmd(args)
	case "design":
		return c.design(args)
	case "release":
		return c.release(args)
	case "trial":
		return c.trialCmd(args)
	case "shots":
		return c.shots(args)
	case "version":
		f := flag.NewFlagSet("gozen version", flag.ContinueOnError)
		if err := c.parse(f, args); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return cli.Invalid("version takes no operands")
		}
		c.out.Println("gozen", version())
		return nil
	case "check", "layout", "book", "memos":
		f := flag.NewFlagSet("gozen "+sub, flag.ContinueOnError)
		var memoOut *string
		var noLayout *bool
		if sub == "memos" {
			memoOut = f.String("out", "", "new output directory (default <memo-dir>-html)")
		}
		if sub == "check" {
			noLayout = f.Bool("no-layout", false, "check definitions, references, expressions, and assets only; skip the font-based text layout check")
		}
		if err := c.parse(f, args); err != nil {
			return err
		}
		if f.NArg() != 1 {
			return cli.Invalid("%s expects one directory", sub)
		}
		switch sub {
		case "check":
			if *noLayout {
				return c.checkStructure(f.Arg(0))
			}
			return c.check(f.Arg(0))
		case "layout":
			return c.layout(f.Arg(0), c.out)
		case "book":
			return c.book(f.Arg(0))
		case "memos":
			return c.memosCmd(f.Args(), *memoOut)
		}
	}
	return cli.Invalid("unknown command %q; use --help", sub)
}
