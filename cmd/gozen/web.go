package main

import (
	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/distribute"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
)

func (c *command) web(args []string) error {
	f := clihelp.NewFlagSet("gozen pack web")
	bundle := f.String("runtime", "", "the unpacked matching web runtime `DIR`; required")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 2 || *bundle == "" {
		return cli.Invalid(diag.OptionRequired, "usage: gozen pack web --runtime DIR <game-dir> <out-dir>")
	}
	if err := distribute.Web(f.Arg(0), f.Arg(1), *bundle, clihelp.Version()); err != nil {
		return err
	}
	c.out.Printf("created website in %s\n", f.Arg(1))
	return nil
}
