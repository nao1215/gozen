package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/assetpolicy"

	"github.com/nao1215/gozen/internal/layer1/assetfs"
)

func (c *command) release() clihelp.Operation {
	fl := clihelp.NewFlagSet("gozen release")
	var out string
	fl.StringVarP(&out, "out", "o", "", "write the approved tree to this new `DIR`; required")
	inspect := fl.String("inspect", "", "the inspection records `FILE` (JSON Lines) from production; required")
	overlay := fl.String("overlay", "", "take replaced assets from this `DIR` (default: the game's approval settings)")
	prefix := fl.String("prefix", "assets", "the asset `DIR` inside the game, separated by /")
	return clihelp.Operation{Flags: fl, Run: func(_ context.Context) (err error) {
		if fl.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		if out == "" || *inspect == "" {
			return cliio.Invalid(diag.OptionRequired, "release requires --inspect FILE and --out DIR")
		}
		src := fl.Arg(0)
		if !cli.Relative(*prefix) {
			return cliio.Invalid(diag.BadPath, "invalid --prefix")
		}
		policy, ins, outputs, err := releaseInputs(src, overlay, *inspect)
		if err != nil {
			return diag.ApprovalRecords.Wrap(err)
		}
		if err := checkReferences(src, *overlay, *prefix, policy, ins, outputs); err != nil {
			return err
		}
		var res assetfs.Result
		err = cli.Publish(out, []string{src, *overlay}, func(stage string) error {
			var e error
			res, e = (assetfs.Tree{Src: src, Top: *overlay, Prefix: *prefix, Dst: stage, Outputs: outputs, Skip: []string{"asset-policy.yaml"}}).Build(ins)
			return e
		})
		if err != nil {
			return err
		}
		c.out.Printf("%s: %d files, %d replaced (%d approval records)\n", out, res.Files, len(res.Replaced), len(ins))
		return nil
	}}
}

func checkReferences(src, top, prefix string, policy assetpolicy.Policy, records []assetfs.Approval, outputs []string) error {
	files, missing, err := assetpolicy.References(os.DirFS(src), prefix, policy.Extensions)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return diag.ReferencedAssetMissing.Errorf("referenced assets are missing: %v", missing)
	}
	gate := assetfs.Gate{Base: os.DirFS(src), Top: os.DirFS(top), Prefix: prefix, Records: records, Outputs: outputs, Watch: []string{""}}
	for _, file := range files {
		if err := gate.Check(file); err != nil {
			return diag.Unapproved.Wrap(err)
		}
	}
	return nil
}

func releaseInputs(src string, overlay *string, inspect string) (assetpolicy.Policy, []assetfs.Approval, []string, error) {
	p, err := assetpolicy.Load(os.DirFS(src), "asset-policy.yaml")
	if err != nil {
		return p, nil, nil, err
	}
	if *overlay == "" {
		if p.Overlay == "" {
			return p, nil, nil, errors.New("give the replacement directory with --overlay or in the game's asset policy")
		}
		*overlay = filepath.Join(src, p.Overlay)
	}
	records, outputs, err := p.ReadRecords(os.DirFS(*overlay))
	if err != nil {
		return p, nil, nil, err
	}
	ins, err := p.ReadInspections(inspect, records)
	return p, ins, outputs, err
}
