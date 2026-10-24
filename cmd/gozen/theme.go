package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/cmd/internal/themes"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
)

// themeList prints the dungeon themes the command carries.
func (c *command) themeList() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen theme list")
	c.reporting(f)
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 0 {
			return cliio.Invalid(diag.Operands, "theme list takes no operands")
		}
		items := themes.List()
		if c.report.Machine() {
			type item struct {
				ID      string `json:"id"`
				Summary string `json:"summary"`
			}
			rows := make([]item, 0, len(items))
			for _, t := range items {
				row := item{t.Name, t.Summary}
				rows = append(rows, row)
				if c.report.Format == "jsonl" {
					if err := c.report.Item(c.out, t.Name, "ok", row); err != nil {
						return err
					}
				}
			}
			if c.report.Format == "jsonl" {
				return c.report.Result(c.out, "ok", struct {
					Count int `json:"count"`
				}{len(rows)})
			}
			return c.report.Result(c.out, "ok", struct {
				Items []item `json:"items"`
			}{rows})
		}
		for _, t := range items {
			c.out.Printf("%-8s %s\n", t.Name, t.Summary)
		}
		return nil
	}}
}

// themeAdd copies a dungeon theme into a game's assets.
func (c *command) themeAdd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen theme add")
	var name string
	f.StringVar(&name, "name", "", "the theme `NAME`; required")
	force := f.Bool("force", false, "replace the files the game already has")
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		if name == "" {
			return cliio.Invalid(diag.OptionRequired, "--name NAME is required")
		}
		dir := f.Arg(0)
		if !themes.Has(name) {
			return cliio.Invalid(diag.UnknownName, "unknown theme %q: run gozen theme list", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "game.yaml")); err != nil {
			return diag.GameUnreadable.Wrap(err)
		}
		r, err := themes.Add(name, dir, *force)
		if err != nil {
			return err
		}
		for _, p := range r.Written {
			c.out.Println("wrote", p)
		}
		for _, p := range r.Kept {
			c.out.Println("kept", p, "(already there; --force replaces it)")
		}
		if len(r.Written) > 0 || len(r.Kept) > 0 {
			c.out.Printf("theme %s is in %s: write `theme: %s` in dungeon (or in a look) of your data\n", name, dir, name)
		}
		return nil
	}}
}
