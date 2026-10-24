package main

import (
	"runtime"

	"github.com/nao1215/gozen/cmd/internal/distribute"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/cliio"
	"github.com/nao1215/gozen/internal/diag"
)

func (c *command) exportGame(args []string) error {
	f := clihelp.NewFlagSet("gozen export")
	bundle := f.String("runtime", "", "the unpacked matching runtime bundle `DIR`; required")
	target := f.String("target", runtime.GOOS+"-"+runtime.GOARCH, "web, or linux/windows/darwin with amd64/arm64")
	out := f.StringP("out", "o", "", "write the game to this new `DIR`; required")
	c.reporting(f)
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cliio.Invalid(diag.Operands, "expected one game directory")
	}
	if *bundle == "" || *out == "" {
		return cliio.Invalid(diag.OptionRequired, "export requires --runtime DIR and --out DIR")
	}
	var err error
	switch *target {
	case "web":
		err = distribute.Web(f.Arg(0), *out, *bundle, clihelp.Version())
	case "linux-amd64", "linux-arm64", "windows-amd64", "windows-arm64", "darwin-amd64", "darwin-arm64":
		err = distribute.Desktop(f.Arg(0), *out, *bundle, *target, clihelp.Version())
	default:
		return cliio.Invalid(diag.BadValue, "unsupported export target %q", *target)
	}
	if err != nil {
		return err
	}
	if c.report.Machine() {
		return c.report.Result(c.out, "ok", struct {
			Directory string `json:"directory"`
			Target    string `json:"target"`
		}{*out, *target})
	}
	c.out.Printf("created %s for %s\n", *out, *target)
	return nil
}
