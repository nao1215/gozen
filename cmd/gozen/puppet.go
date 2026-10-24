package main

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer5/puppet"
)

// puppetCmd checks a moving portrait and, with --out, writes it with every
// difference picture cut down to the part that differs.
func (c *command) puppetCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen puppet")
	out := f.StringP("out", "o", "", "write the puppet with small difference pictures to this new `DIR`")
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "puppet expects one puppet YAML file")
		}
		file := f.Arg(0)
		dir, name := filepath.Split(file)
		if dir == "" {
			dir = "."
		}
		rig, err := puppet.Load(os.DirFS(dir), name)
		if err != nil {
			return err
		}
		c.out.Print(puppet.Describe(rig))
		if *out == "" {
			return nil
		}
		spec, files := puppet.Pack(rig)
		b, err := yaml.Marshal(spec)
		if err != nil {
			return err
		}
		err = cli.Publish(*out, nil, func(stage string) error {
			if err := cli.WriteNew(filepath.Join(stage, name), b); err != nil {
				return err
			}
			for n, img := range files {
				var buf bytes.Buffer
				if err := png.Encode(&buf, img); err != nil {
					return err
				}
				if err := cli.WriteNew(filepath.Join(stage, n), buf.Bytes()); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		c.out.Printf("wrote %s (%d pictures)\n", filepath.ToSlash(filepath.Join(*out, name)), len(files))
		return nil
	}}
}
