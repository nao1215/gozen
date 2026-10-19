// Package command implements the gozen mosaic process contract. It never exits the process.
package command

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/pflag"

	"github.com/nao1215/gozen/internal/clihelp"
)

type output struct {
	io.Writer
	err error
}

func (o *output) Write(b []byte) (int, error) {
	n, err := o.Writer.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	o.err = errors.Join(o.err, err)
	return n, err
}
func (o *output) printf(s string, v ...any) { o.write(fmt.Sprintf(s, v...)) }
func (o *output) println(v ...any)          { o.write(fmt.Sprintln(v...)) }
func (o *output) write(s string) {
	if o.err == nil {
		_, o.err = io.WriteString(o, s)
	}
}

type cli struct{ out, diag *output }
type argumentError struct{ error }

// Run returns 0 for success/help, 2 for argument errors, and 1 for processing or
// I/O errors. Caller owns writers. No process state or working directory changes.
func Run(args []string, out, diagnostics io.Writer) int {
	c := &cli{&output{Writer: out}, &output{Writer: diagnostics}}
	var err error
	switch {
	case len(args) == 0:
		err = argumentError{fmt.Errorf("a command is required: auto, inspect, or manual; use --help")}
	case args[0] == "-h" || args[0] == "--help":
		c.out.write(topPage.String(newFlagSet("gozen mosaic")))
	case args[0] == "auto":
		err = c.autoCmd(args[1:])
	case args[0] == "inspect":
		err = c.inspectCmd(args[1:])
	case args[0] == "manual":
		err = c.manualCmd(args[1:])
	default:
		err = argumentError{fmt.Errorf("unknown command %q: use auto, inspect, or manual; see --help", args[0])}
	}
	if errors.Is(err, clihelp.ErrHelp) {
		err = nil
	}
	err = errors.Join(err, c.out.err, c.diag.err)
	if err == nil {
		return 0
	}
	c.diag.println(err)
	var arg argumentError
	if errors.As(err, &arg) {
		c.diag.println("Run \"gozen mosaic --help\" for usage. See " + clihelp.README)
		return 2
	}
	c.diag.println(requirements)
	return 1
}

func newFlagSet(name string) *pflag.FlagSet { return clihelp.NewFlagSet(name) }

// parse parses options; on --help it prints the page and returns clihelp.ErrHelp.
func (cli *cli) parse(fs *pflag.FlagSet, args []string, page clihelp.Page) error {
	help, err := clihelp.Parse(fs, args)
	if err != nil {
		return argumentError{err}
	}
	if help {
		cli.out.write(page.String(fs))
		return clihelp.ErrHelp
	}
	return nil
}
