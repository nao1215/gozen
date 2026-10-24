package main

import (
	"runtime"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/distribute"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
)

func (c *command) desktop(args []string) error {
	f := clihelp.NewFlagSet("gozen pack desktop")
	bundle := f.String("runtime", "", "the unpacked runtime bundle `DIR`; required")
	target := f.String("target", runtime.GOOS+"-"+runtime.GOARCH, "the destination platform: linux, windows, or darwin with amd64 or arm64")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 2 || *bundle == "" {
		return cli.Invalid(diag.OptionRequired, "usage: gozen pack desktop --runtime DIR [--target OS-ARCH] <game-dir> <out-dir>")
	}
	switch *target {
	case "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64", "darwin-amd64", "darwin-arm64":
	default:
		return cli.Invalid(diag.BadValue, "unsupported desktop target %q", *target)
	}
	if err := distribute.Desktop(f.Arg(0), f.Arg(1), *bundle, *target, clihelp.Version()); err != nil {
		return err
	}
	c.out.Printf("created %s for %s\n", f.Arg(1), *target)
	return nil
}
