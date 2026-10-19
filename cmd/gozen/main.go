// Command gozen checks, simulates, documents, and packages game definitions, and
// runs the production helpers (prompt, mosaic) as subcommands.
package main

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	mosaiccmd "github.com/nao1215/gozen/internal/mosaic/command"
	promptcmd "github.com/nao1215/gozen/internal/promptgen/command"
)

type command struct{ out, stderr *cli.Output }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// parse parses a subcommand's options. On --help it prints the page and returns
// clihelp.ErrHelp, which run turns into exit 0.
func (c *command) parse(fs *pflag.FlagSet, args []string) error {
	help, err := clihelp.Parse(fs, args)
	if err != nil {
		return cli.Invalid("%v", err)
	}
	if help {
		parts := strings.Fields(fs.Name())
		c.out.Print(pages[parts[len(parts)-1]].String(fs))
		return clihelp.ErrHelp
	}
	return nil
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
	if errors.Is(err, clihelp.ErrHelp) {
		err = nil
	}
	err = errors.Join(err, c.out.Err, c.stderr.Err)
	if err == nil {
		return 0
	}
	c.stderr.Println(err)
	var u *cli.UsageError
	if errors.As(err, &u) {
		help := "gozen --help"
		if len(args) > 0 && pages[args[0]].Summary != "" {
			help = "gozen " + args[0] + " --help"
		}
		c.stderr.Println(`Run "` + help + `" for usage.`)
		return 2
	}
	return 1
}
func (c *command) execute(args []string) error {
	if len(args) == 0 {
		return cli.Invalid("usage: gozen <command> [OPTIONS] [ARGUMENTS]; use --help")
	}
	if strings.HasPrefix(args[0], "-") {
		// Options of gozen itself: -h/--help and -v/--version.
		fs := clihelp.NewFlagSet("gozen")
		ver := fs.BoolP("version", "v", false, "print the gozen version and exit")
		help, err := clihelp.Parse(fs, args)
		if err != nil {
			return cli.Invalid("%v", err)
		}
		if fs.NArg() != 0 {
			return cli.Invalid("options go after the command: gozen <command> [OPTIONS]; use --help")
		}
		switch {
		case help:
			c.out.Print(topPage.String(fs))
		case *ver:
			c.out.Println("gozen", clihelp.Version())
		default:
			return cli.Invalid("usage: gozen <command> [OPTIONS] [ARGUMENTS]; use --help")
		}
		return nil
	}
	sub := args[0]
	args = args[1:]
	switch sub {
	case "help":
		if len(args) != 0 {
			return cli.Invalid("help takes at most one command")
		}
		return c.execute([]string{"--help"})
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
	case "puppet":
		return c.puppetCmd(args)
	case "version":
		f := clihelp.NewFlagSet("gozen version")
		if err := c.parse(f, args); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return cli.Invalid("version takes no operands")
		}
		c.out.Println("gozen", clihelp.Version())
		return nil
	case "check", "layout", "book", "memos":
		f := clihelp.NewFlagSet("gozen " + sub)
		var memoOut *string
		var noLayout *bool
		if sub == "memos" {
			memoOut = f.StringP("out", "o", "", "write the HTML to this new `DIR` (default: <memo-dir>-html)")
		}
		if sub == "check" {
			noLayout = f.Bool("no-layout", false, "check definitions, references, expressions, and assets only; do not read the font or check the text layout")
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
