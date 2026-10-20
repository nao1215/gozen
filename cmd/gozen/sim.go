package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/headless/balance"
	data "github.com/nao1215/gozen/schema"
)

func (c *command) sim(args []string) error {
	fs := clihelp.NewFlagSet("gozen sim battle")
	n := fs.IntP("battles", "n", 200, "battles to fight")
	party := fs.String("party", "", "the party, as actor `IDS` separated by commas (default: the starting party)")
	level := fs.Int("level", 0, "the party's level (default: each actor's own)")
	seed := fs.Uint64("seed", 1, "the random seed; the same seed gives the same result")
	verbose := fs.BoolP("verbose", "v", false, "print the messages of the first battle")
	maxTurns := fs.Int("turns", 60, "a battle longer than this many turns is a draw")
	noTree := fs.Bool("no-tree", false, "do not spend the skill points (by default they go evenly to the open nodes of the lowest level)")
	gear := fs.String("gear", "", "item `IDS`, separated by commas, to give the party and wear where they fit best")
	states := fs.StringArray("state", nil, "start from this saved state `FILE` (a note's .save from F9, or gozen sim dive --arrival-saves) with everyone recovered, instead of a new game; repeat it to measure several")
	asIs := fs.Bool("as-is", false, "with --state, keep the saved HP, MP, and states instead of recovering everyone")
	streak := fs.Int("streak", 0, "also fight the troop again and again without recovery from each start, up to `N` wins, and print how many were won in a row")
	hpBand := fs.Bool("hp", false, "also print the party's HP left after a win (10th, 50th, and 90th percentile)")
	format := fs.String("format", "text", "the output: text, or json (an array with one result for each start)")
	if err := c.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return cli.Invalid(diag.Operands, "expected <game-dir> <troop>")
	}
	dir, troop := fs.Arg(0), fs.Arg(1)
	if *n <= 0 || *maxTurns <= 0 || *level < 0 {
		return cli.Invalid(diag.BadValue, "--battles and --turns must be positive; --level must be nonnegative")
	}
	fsys, err := gameFS(dir)
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
	if db.Troops[troop] == nil {
		return cli.Invalid(diag.UnknownName, "unknown troop %q", troop)
	}
	if *asIs && len(*states) == 0 {
		return cli.Invalid(diag.OptionNeedsAnother, "--as-is needs --state")
	}
	if *streak < 0 {
		return cli.Invalid(diag.BadValue, "--streak must be nonnegative")
	}
	if *format != "text" && *format != "json" {
		return cli.Invalid(diag.NotAChoice, "--format is text or json, not %q", *format)
	}
	if len(*states) > 0 && *party != "" {
		return cli.Invalid(diag.OptionConflict, "--state and --party do not go together")
	}
	if len(*states) > 1 && *verbose {
		return cli.Invalid(diag.OptionConflict, "--verbose goes with one start")
	}
	prep := balance.Prep{Level: *level, Tree: !*noTree, Gear: commaList(*gear), AsIs: *asIs}
	starts := []string{""}
	if len(*states) > 0 {
		starts = *states
	}
	var results []simResult
	for _, file := range starts {
		var start balance.Start
		if file != "" {
			b, err := os.ReadFile(file) //nolint:gosec // The caller names the saved state to read.
			if err != nil {
				return err
			}
			start = balance.SavedStart(db, b, *seed, prep)
		} else {
			start = balance.FreshStart(db, *seed, commaList(*party), prep)
		}
		turn, begun := 0, false
		o, err := balance.Sim(db, troop, *n, *maxTurns, start, func(i int, b *battle.Battle, steps []battle.Step) {
			if !*verbose || i != 0 || *format == "json" {
				return
			}
			if !begun {
				begun = true
			} else if b.Turn != turn && b.Result == battle.Ongoing {
				turn = b.Turn
				c.out.Printf("-- turn %d\n", b.Turn)
			}
			for _, s := range steps {
				if s.Kind == battle.Event {
					c.out.Println("  [event]")
					continue
				}
				c.out.Println("  " + s.Text)
			}
		})
		if err != nil {
			return err
		}
		r := simResult{State: file, Troop: troop, Battles: *n, Won: o.Won, Lost: o.Lost, Fled: o.Fled, Draw: o.Draw,
			WinRate: o.WinRate(), AvgTurns: o.AvgTurns(), AvgStanding: o.AvgStanding()}
		if lo, mid, hi, ok := o.HPBand(); ok && *hpBand {
			r.HP = &band{lo, mid, hi}
		}
		if *streak > 0 {
			wins, err := balance.Streak(db, troop, *n, *maxTurns, *streak, start)
			if err != nil {
				return err
			}
			r.Streak = &band{float64(balance.Percentile(wins, 0.1)), float64(balance.Percentile(wins, 0.5)), float64(balance.Percentile(wins, 0.9))}
		}
		results = append(results, r)
		if *format == "json" {
			continue
		}
		head := troop
		if file != "" && len(starts) > 1 {
			head = file + " " + troop
		}
		c.out.Printf("%s: %d battles  win %.0f%%  lose %.0f%%  fled %.0f%%  draw %.0f%%  avg turns %.1f  avg standing after a win %.1f\n",
			head, *n, pct(o.Won, *n), pct(o.Lost, *n), pct(o.Fled, *n), pct(o.Draw, *n), o.AvgTurns(), o.AvgStanding())
		if r.HP != nil {
			c.out.Printf("hp left after a win: p10 %.0f%%  median %.0f%%  p90 %.0f%%\n", 100*r.HP.P10, 100*r.HP.P50, 100*r.HP.P90)
		}
		if r.Streak != nil {
			c.out.Printf("wins in a row without recovery (up to %d): p10 %.0f  median %.0f  p90 %.0f\n", *streak, r.Streak.P10, r.Streak.P50, r.Streak.P90)
		}
	}
	if *format == "json" {
		b, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		c.out.Println(string(b))
	}
	return nil
}

// simResult is one start's result in gozen sim battle --format json.
type simResult struct {
	State       string  `json:"state,omitempty"`
	Troop       string  `json:"troop"`
	Battles     int     `json:"battles"`
	Won         int     `json:"won"`
	Lost        int     `json:"lost"`
	Fled        int     `json:"fled"`
	Draw        int     `json:"draw"`
	WinRate     float64 `json:"win_rate"`
	AvgTurns    float64 `json:"avg_turns"`
	AvgStanding float64 `json:"avg_standing"`
	HP          *band   `json:"hp_left,omitempty"`
	Streak      *band   `json:"streak,omitempty"`
}

// band is the 10th, 50th and 90th percentile.
type band struct {
	P10 float64 `json:"p10"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
}

// commaList splits the --party and --gear lists. It returns nil for an empty string and otherwise keeps empty elements (reported as unknown IDs).
func commaList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(b) }
