package main

import (
	"encoding/json"
	"sort"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/schema"
)

type breedRow struct {
	ID          string   `json:"id"`
	Family      string   `json:"family"`
	Rank        int      `json:"rank"`
	Steps       int      `json:"steps"`
	Parents     []string `json:"parents,omitempty"`
	Sex         string   `json:"sex"`
	ParentSexes []string `json:"parent_sexes,omitempty"`
}

func (c *command) breedCmd(args []string) error {
	f := clihelp.NewFlagSet("gozen sim breed")
	format := f.String("format", "text", "output format: text or json")
	if err := c.parse(f, args); err != nil {
		return err
	}
	if f.NArg() != 1 {
		return cli.Invalid(diag.Operands, "sim breed expects one game directory")
	}
	if *format != "text" && *format != "json" {
		return cli.Invalid(diag.NotAChoice, "--format is text or json, not %q", *format)
	}
	fsys, err := gameFS(f.Arg(0))
	if err != nil {
		return err
	}
	db, err := data.Load(fsys, ".")
	if err != nil {
		return err
	}
	paths := db.BreedReach()
	var reached []breedRow
	var missing []string
	for id, e := range db.Enemies {
		if p, ok := paths[id]; ok {
			row := breedRow{ID: id, Family: e.Family, Rank: e.Rank, Steps: p.Steps, Sex: p.Sex}
			if p.Steps > 0 {
				row.Parents = p.Parents[:]
				row.ParentSexes = p.ParentSexes[:]
			}
			reached = append(reached, row)
		} else {
			missing = append(missing, id)
		}
	}
	sort.Slice(reached, func(i, j int) bool {
		if reached[i].Steps != reached[j].Steps {
			return reached[i].Steps < reached[j].Steps
		}
		return reached[i].ID < reached[j].ID
	})
	sort.Strings(missing)
	if *format == "json" {
		b, err := json.MarshalIndent(struct {
			Reachable   []breedRow `json:"reachable"`
			Unreachable []string   `json:"unreachable"`
		}{reached, missing}, "", "  ")
		if err != nil {
			return err
		}
		c.out.Println(string(b))
		return nil
	}
	for _, row := range reached {
		if row.Steps == 0 {
			c.out.Printf("%s: capture (%s, %s, rank %d)\n", row.ID, row.Sex, row.Family, row.Rank)
		} else {
			c.out.Printf("%s: %d generations, %s (%s) x %s (%s) => %s (%s, rank %d)\n", row.ID, row.Steps, row.Parents[0], row.ParentSexes[0], row.Parents[1], row.ParentSexes[1], row.Sex, row.Family, row.Rank)
		}
	}
	for _, id := range missing {
		c.out.Printf("%s: unreachable\n", id)
	}
	return nil
}
