package main

import (
	"context"
	"crypto/rand"
	"strings"

	"github.com/nao1215/gozen/internal/base/cliio"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/cmd/internal/starter"
	"github.com/nao1215/gozen/internal/base/clihelp"
	"github.com/nao1215/gozen/internal/base/diag"
	data "github.com/nao1215/gozen/internal/model/gamedata"
)

func (c *command) newGame() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen new")
	c.reporting(f)
	template := f.String("template", "rpg", "the game template: rpg, novel, or campaign")
	id := f.String("id", "", "persistent save identity (default: a new random game ID)")
	title := f.String("title", "My game", "the game's title")
	clihelp.Choices(f.Lookup("template"), starter.Templates()...)
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "new expects one new game directory")
		}
		if strings.TrimSpace(f.Arg(0)) == "" {
			return cliio.Invalid(diag.BadPath, "new expects a nonempty game directory")
		}
		if f.Changed("id") && !data.ValidGameID(*id) {
			return cliio.Invalid(diag.BadValue, "--id must start with a letter or digit and contain only ASCII letters, digits, '_' or '-'")
		}
		if strings.TrimSpace(*title) == "" {
			return cliio.Invalid(diag.BadValue, "--title must not be empty")
		}
		if *id == "" {
			*id = "game-" + strings.ToLower(rand.Text()[:12])
		}
		cfg := starter.Config{Template: *template, ID: *id, Title: *title}
		if err := cli.Publish(f.Arg(0), nil, func(stage string) error { return starter.Write(stage, cfg) }); err != nil {
			return err
		}
		if c.report.Machine() {
			return c.report.Result(c.out, "ok", struct {
				Directory string `json:"directory"`
				Template  string `json:"template"`
				ID        string `json:"id"`
				Title     string `json:"title"`
			}{f.Arg(0), *template, *id, *title})
		}
		c.out.Printf("created %s game in %s (id: %s)\n", *template, f.Arg(0), *id)
		c.out.Println("Open that directory, then run: gozen check .")
		c.out.Println("Play it with: gozen run .")
		return nil
	}}
}
