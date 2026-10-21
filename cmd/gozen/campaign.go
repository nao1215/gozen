package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/nao1215/gozen/cmd/gozen/internal/campaignsim"
	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	data "github.com/nao1215/gozen/schema"
)

// campaignCmd is gozen sim campaign: many campaigns played headless with a strategy, and
// what they came to (ADR 0030).
func (c *command) campaignCmd(args []string) error {
	fs := clihelp.NewFlagSet("gozen sim campaign")
	n := fs.IntP("campaigns", "n", 200, "campaigns to play")
	seed := fs.Uint64("seed", 1, "the random seed of the first campaign (the next ones count up from it); the same seed gives the same result")
	as := fs.String("as", "greedy", "the player's strategy: greedy, random, or ai (the player's faction's own ai rules)")
	tactic := fs.String("tactic", "", "the tactic every general marches with in a clash shown as units (a march then takes the most generals it can); empty: each general's own")
	maxOrders := fs.Int("orders", 2000, "orders a campaign may give before it is cut")
	format := fs.String("format", "text", "the output: text, or json")
	if err := c.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return cli.Invalid(diag.Operands, "expected <game-dir>")
	}
	if *n <= 0 || *maxOrders <= 0 {
		return cli.Invalid(diag.BadValue, "--campaigns and --orders must be positive")
	}
	if !slices.Contains(campaignsim.Strategies, *as) {
		return cli.Invalid(diag.NotAChoice, "--as is %s, not %q", strings.Join(campaignsim.Strategies, ", "), *as)
	}
	if *format != "text" && *format != "json" {
		return cli.Invalid(diag.NotAChoice, "--format is text or json, not %q", *format)
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
		return cli.Invalid(diag.UnknownName, "the game has no campaign (game.yaml campaign)")
	}
	if _, ok := db.Game.Campaign.Clash.DeclaredTactics()[*tactic]; *tactic != "" && !ok {
		return cli.Invalid(diag.NotAChoice, "--tactic is %s, not %q", strings.Join(db.Game.Campaign.Clash.TacticIDs(), ", "), *tactic)
	}
	r, err := campaignsim.Run(context.Background(), db, campaignsim.Config{Runs: *n, Seed: *seed, As: *as, MaxOrder: *maxOrders, Tactic: *tactic})
	if err != nil {
		return err
	}
	if *format == "json" {
		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		c.out.Println(string(b))
		return nil
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
