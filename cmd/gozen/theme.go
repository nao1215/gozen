package main

import (
	"os"
	"path/filepath"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/themes"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
)

// themeList prints the dungeon themes the command carries.
func (c *command) themeList(args []string) error {
	f := clihelp.NewFlagSet("gozen theme list")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return cli.Invalid(diag.Operands, "theme list takes no operands")
	}
	for _, t := range themes.List() {
		c.out.Printf("%-8s %s\n", t.Name, t.Summary)
	}
	return nil
}

// themeAdd copies a dungeon theme into a game's assets.
func (c *command) themeAdd(args []string) error {
	f := clihelp.NewFlagSet("gozen theme add")
	force := f.Bool("force", false, "replace the files the game already has")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 2 {
		return cli.Invalid(diag.Operands, "theme add expects a theme name and a game directory")
	}
	name, dir := f.Arg(0), f.Arg(1)
	if !themes.Has(name) {
		return cli.Invalid(diag.UnknownName, "unknown theme %q: run gozen theme list", name)
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
}
