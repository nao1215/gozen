// Command rpgcheck validates game definitions without a screen or audio device.
package main

import (
	"errors"
	"flag"
	"io"
	"os"

	"github.com/nao1215/gozen/cmd/internal/checkdata"
	"github.com/nao1215/gozen/cmd/internal/cli"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, stderr io.Writer) int {
	o, e := &cli.Output{Writer: out}, &cli.Output{Writer: stderr}
	f := flag.NewFlagSet("rpgcheck", flag.ContinueOnError)
	err := cli.Parse(f, args, "rpgcheck <game-dir>", "No external runtime dependencies; cgo is not required.", o)
	if errors.Is(err, flag.ErrHelp) {
		if o.Err != nil {
			return 1
		}
		return 0
	}
	if err == nil && f.NArg() != 1 {
		err = cli.Invalid("usage: rpgcheck <game-dir>")
	}
	if err == nil {
		_, err = checkdata.Load(os.DirFS(f.Arg(0)), o, e)
	}
	if err == nil {
		o.Println("Structure checked; layout was not checked. Use rpgtool layout <game-dir>.")
		err = errors.Join(o.Err, e.Err)
	}
	if err != nil {
		e.Println(err)
		var u *cli.UsageError
		if errors.As(err, &u) {
			return 2
		}
		return 1
	}
	return 0
}
