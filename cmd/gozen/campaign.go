package main

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/cmd/gozen/internal/campaignsim"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	data "github.com/nao1215/gozen/schema"
)

// campaignCmd is gozen sim campaign: many campaigns played headless with a strategy, and
// what they came to (ADR 0030).
func (c *command) campaignCmd() clihelp.Operation {
	fs := clihelp.NewFlagSet("gozen sim campaign")
	n := fs.IntP("campaigns", "n", 200, "campaigns to play")
	seed := fs.Uint64("seed", 1, "the random seed of the first campaign (the next ones count up from it); the same seed gives the same result")
	as := fs.String("as", "greedy", "the player's strategy: greedy, random, or ai (the player's faction's own ai rules)")
	tactic := fs.String("tactic", "", "the tactic every general marches with in a clash shown as units (a march then takes the most generals it can); empty: each general's own")
	maxOrders := fs.Int("orders", 2000, "orders a campaign may give before it is cut")
	c.reporting(fs)
	compare := fs.String("compare", "", "compare strategies or declared tactics on the same seeds")
	return clihelp.Operation{Flags: fs, Run: func(ctx context.Context) (err error) {
		if fs.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected <game-dir>")
		}
		if *n <= 0 || *maxOrders <= 0 {
			return cliio.Invalid(diag.BadValue, "--campaigns and --orders must be positive")
		}
		if !slices.Contains(campaignsim.Strategies, *as) {
			return cliio.Invalid(diag.NotAChoice, "--as is %s, not %q", strings.Join(campaignsim.Strategies, ", "), *as)
		}
		if *compare != "" && *compare != "strategies" && *compare != "tactics" {
			return cliio.Invalid(diag.NotAChoice, "--compare is strategies or tactics, not %q", *compare)
		}
		fsys, err := gameFS(fs.Arg(0))
		if err != nil {
			return err
		}
		if err := gamedata.UseWords(fsys); err != nil {
			return err
		}
		db, err := data.Load(fsys, ".")
		if err != nil {
			return err
		}
		if db.Game.Campaign == nil {
			return cliio.Invalid(diag.UnknownName, "the game has no campaign (game.yaml campaign)")
		}
		if _, ok := db.Game.Campaign.Clash.DeclaredTactics()[*tactic]; *tactic != "" && !ok {
			return cliio.Invalid(diag.NotAChoice, "--tactic is %s, not %q", strings.Join(db.Game.Campaign.Clash.TacticIDs(), ", "), *tactic)
		}
		cfg := campaignsim.Config{Runs: *n, Seed: *seed, As: *as, MaxOrder: *maxOrders, Tactic: *tactic}
		if *compare != "" {
			r, err := campaignsim.Compare(ctx, db, cfg, *compare)
			if err != nil {
				return err
			}
			if c.report.Machine() {
				if c.report.Format == "jsonl" {
					for _, row := range r.Rows {
						if err := c.report.Item(c.out, row.Name, "ok", row); err != nil {
							return err
						}
					}
				}
				return c.report.Result(c.out, "ok", r)
			}
			c.out.Printf("%s: compare %s, %d campaigns per entry from seed %d\n", r.Game, r.Mode, r.Runs, r.Seed)
			c.out.Println("entry     win rate   mean turns   mean regions held")
			for _, row := range r.Rows {
				c.out.Printf("%-9s %6.1f%% %11.1f %19.2f\n", row.Name, 100*row.WinRate, row.MeanTurns, row.MeanRegionsHeld)
			}
			c.out.Println("regions: mean changes | mean first taken turn | end holders | player win rate when held by turn")
			for _, row := range r.Rows {
				for _, reg := range row.Regions {
					first := "-"
					if reg.MeanFirstTakenTurn > 0 {
						first = fmt.Sprintf("%.2f", reg.MeanFirstTakenTurn)
					}
					c.out.Printf("  %s/%s: %.2f | %s | %s | %s\n", row.Name, reg.Region, reg.MeanChanges, first, factionCounts(reg.EndHolders), turnRates(reg.PlayerWinRateByTurn))
				}
			}
			return nil
		}
		r, err := campaignsim.Run(ctx, db, cfg)
		if err != nil {
			return err
		}
		if c.report.Machine() {
			return c.report.Result(c.out, "ok", r)
		}
		c.out.Printf("%s: %d campaigns from seed %d as %s\n", r.Game, r.Runs, r.Seed, r.As)
		c.out.Printf("won %d (%.0f%%), lost %d, cut %d\n", r.Won, 100*r.WinRate, r.Lost, r.Cut)
		c.out.Printf("turns of a win: %s\n", histogram(r.Turns.Won))
		c.out.Printf("turns of a loss: %s\n", histogram(r.Turns.Lost))
		c.out.Println("regions held at the start of each turn (mean):")
		for _, t := range r.Regions {
			var parts []string
			for _, f := range gamedata.FactionIDs(db) {
				parts = append(parts, fmt.Sprintf("%s %.1f", f, t.Mean[f]))
			}
			c.out.Printf("  turn %d (%d): %s\n", t.Turn, t.Campaigns, strings.Join(parts, ", "))
		}
		for _, f := range r.Fallen {
			c.out.Printf("fell: %s in %d campaigns, at turn %.1f on average\n", f.Faction, f.Runs, f.MeanTurn)
		}
		ids := make([]string, 0, len(r.Orders))
		for id := range r.Orders {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		var orders []string
		for _, id := range ids {
			orders = append(orders, fmt.Sprintf("%s %d", id, r.Orders[id]))
		}
		c.out.Printf("orders: %s\n", strings.Join(orders, ", "))
		c.out.Printf("marches: won %d, held %d\n", r.Marches["won"], r.Marches["held"])
		return nil
	}}
}

func factionCounts(counts map[string]int) string {
	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			id = "none"
		}
		parts = append(parts, fmt.Sprintf("%s=%d", id, counts[id]))
	}
	return strings.Join(parts, ",")
}

func turnRates(rates map[int]float64) string {
	turns := make([]int, 0, len(rates))
	for turn := range rates {
		turns = append(turns, turn)
	}
	sort.Ints(turns)
	parts := make([]string, 0, len(turns))
	for _, turn := range turns {
		parts = append(parts, fmt.Sprintf("%d=%.0f%%", turn, rates[turn]*100))
	}
	return strings.Join(parts, ",")
}

// histogram is "turn:count" pairs in the order of the turns.
func histogram(m map[int]int) string {
	if len(m) == 0 {
		return "-"
	}
	turns := make([]int, 0, len(m))
	for t := range m {
		turns = append(turns, t)
	}
	sort.Ints(turns)
	parts := make([]string, len(turns))
	for i, t := range turns {
		parts[i] = fmt.Sprintf("%d:%d", t, m[t])
	}
	return strings.Join(parts, " ")
}
