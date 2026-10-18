// Command rpgtool checks, simulates, documents, and packages game definitions.
package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
)

type command struct{ out, stderr *cli.Output }

var usages = map[string]string{
	"check":   "<game-dir>",
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
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func (c *command) parse(f *flag.FlagSet, args []string) error {
	parts := strings.Fields(f.Name())
	sub := parts[len(parts)-1]
	req := "No external runtime dependencies; cgo is not required."
	if sub == "shots" {
		req = "A Gozen game binary and its graphics runtime; Linux unattended capture needs Xvfb. See docs/development/graphics.md."
	}
	return cli.Parse(f, args, "rpgtool "+sub+" "+usages[sub], req, c.out)
}
func (c *command) help() {
	c.out.Println("Usage: rpgtool <command> [arguments]")
	for _, s := range []string{"check", "layout", "sim", "curve", "floors", "trace", "book", "design", "memos", "release", "trial", "shots"} {
		c.out.Printf("  %-8s %s\n", s, usages[s])
	}
	c.out.Println("\nRequirements:\n  No external runtime dependencies for checks, simulations, and reports; cgo is not required.\n  shots launches a graphics game; Linux unattended capture needs Xvfb.\n  See https://github.com/nao1215/gozen#rpgtool")
}
func run(args []string, out, stderr io.Writer) int {
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
		return cli.Invalid("usage: rpgtool <command> [arguments]; use --help")
	}
	if args[0] == "--help" || args[0] == "-h" {
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
	case "check", "layout", "book", "memos":
		f := flag.NewFlagSet("rpgtool "+sub, flag.ContinueOnError)
		var memoOut *string
		if sub == "memos" {
			memoOut = f.String("out", "", "new output directory (default <memo-dir>-html)")
		}
		if err := c.parse(f, args); err != nil {
			return err
		}
		if f.NArg() != 1 {
			return cli.Invalid("%s expects one directory", sub)
		}
		switch sub {
		case "check":
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
