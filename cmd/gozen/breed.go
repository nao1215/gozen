package main

import (
	"context"
	"sort"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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

func (c *command) breedCmd() clihelp.Operation {
	f := clihelp.NewFlagSet("gozen sim breed")
	c.reporting(f)
	return clihelp.Operation{Flags: f, Run: func(_ context.Context) (err error) {
		if f.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "sim breed expects one game directory")
		}
		fsys, err := gameFS(f.Arg(0))
		if err != nil {
			return err
		}
		db, err := data.LoadDir(fsys, ".")
		if err != nil {
			return err
		}
		paths := db.BreedReach()
		reached := []breedRow{}
		missing := []string{}
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
		if c.report.Machine() {
			if c.report.Format == "jsonl" {
				for _, row := range reached {
					if err := c.report.Item(c.out, row.ID, "ok", row); err != nil {
						return err
					}
				}
				for _, id := range missing {
					if err := c.report.Item(c.out, id, "unreachable", struct {
						ID string `json:"id"`
					}{id}); err != nil {
						return err
					}
				}
			}
			return c.report.Result(c.out, "ok", struct {
				Reachable   []breedRow `json:"reachable"`
				Unreachable []string   `json:"unreachable"`
			}{reached, missing})
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
	}}
}
