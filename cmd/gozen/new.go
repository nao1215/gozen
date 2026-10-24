package main

import (
	"crypto/rand"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func (c *command) newGame(args []string) error {
	f := clihelp.NewFlagSet("gozen new")
	template := f.String("template", "rpg", "the game template: rpg, novel, or campaign")
	id := f.String("id", "", "persistent save identity (default: a new random game ID)")
	title := f.String("title", "My game", "the game's title")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cli.Invalid(diag.Operands, "new expects one new game directory")
	}
	if !starter.Has(*template) {
		return cli.Invalid(diag.NotAChoice, "--template is rpg, novel, or campaign, not %q", *template)
	}
	if f.Changed("id") && !data.ValidGameID(*id) {
		return cli.Invalid(diag.BadValue, "--id must start with a letter or digit and contain only ASCII letters, digits, '_' or '-'")
	}
	if strings.TrimSpace(*title) == "" {
		return cli.Invalid(diag.BadValue, "--title must not be empty")
	}
	if *id == "" {
		*id = "game-" + strings.ToLower(rand.Text()[:12])
	}
	cfg := starter.Config{Template: *template, ID: *id, Title: *title}
	if err := cli.Publish(f.Arg(0), nil, func(stage string) error { return starter.Write(stage, cfg) }); err != nil {
		return err
	}
	c.out.Printf("created %s game in %s (id: %s)\n", *template, f.Arg(0), *id)
	c.out.Println("Open that directory, then run: gozen check .")
	c.out.Println("Play it with: gozen play run .")
	return nil
}
