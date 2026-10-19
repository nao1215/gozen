package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/internal/headless/balance"
)

func (c *command) sim(args []string) error {
	fs := clihelp.NewFlagSet("gozen sim")
	n := fs.IntP("battles", "n", 200, "battles to fight")
	party := fs.String("party", "", "the party, as actor `IDS` separated by commas (default: the starting party)")
	level := fs.Int("level", 0, "the party's level (default: each actor's own)")
	seed := fs.Uint64("seed", 1, "the random seed; the same seed gives the same result")
	verbose := fs.BoolP("verbose", "v", false, "print the messages of the first battle")
	maxTurns := fs.Int("turns", 60, "a battle longer than this many turns is a draw")
	noTree := fs.Bool("no-tree", false, "do not spend the skill points (by default they go evenly to the open nodes of the lowest level)")
	gear := fs.String("gear", "", "item `IDS`, separated by commas, to give the party and wear where they fit best")
	state := fs.String("state", "", "start from this saved state `FILE` (a note's .save from F9) with everyone recovered, instead of a new game")
	asIs := fs.Bool("as-is", false, "with --state, keep the saved HP, MP, and states instead of recovering everyone")
	streak := fs.Int("streak", 0, "also fight the troop again and again without recovery from each start, up to `N` wins, and print how many were won in a row")
	hpBand := fs.Bool("hp", false, "also print the party's HP left after a win (10th, 50th, and 90th percentile)")
	if err := c.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return cli.Invalid("expected <game-dir> <troop>")
	}
	dir, troop := fs.Arg(0), fs.Arg(1)
	if *n <= 0 || *maxTurns <= 0 || *level < 0 {
		return cli.Invalid("--battles and --turns must be positive; --level must be nonnegative")
	}
	if err := gamedata.UseWords(os.DirFS(dir)); err != nil {
		return err
	}
	db, err := data.Load(os.DirFS(dir), ".")
	if err != nil {
		return err
	}
	if db.Troops[troop] == nil {
		return fmt.Errorf("unknown troop %q", troop)
	}
	if *asIs && *state == "" {
		return cli.Invalid("--as-is needs --state")
	}
	if *streak < 0 {
		return cli.Invalid("--streak must be nonnegative")
	}
	prep := balance.Prep{Level: *level, Tree: !*noTree, Gear: commaList(*gear), AsIs: *asIs}
	var start balance.Start
	if *state != "" {
		if *party != "" {
			return cli.Invalid("--state and --party do not go together")
		}
		b, err := os.ReadFile(*state)
		if err != nil {
			return err
		}
		start = balance.SavedStart(db, b, *seed, prep)
	} else {
		start = balance.FreshStart(db, *seed, commaList(*party), prep)
	}
	turn, begun := 0, false
	o, err := balance.Sim(db, troop, *n, *maxTurns, start, func(i int, b *battle.Battle, steps []battle.Step) {
		if !*verbose || i != 0 {
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
	c.out.Printf("%s: %d battles  win %.0f%%  lose %.0f%%  fled %.0f%%  draw %.0f%%  avg turns %.1f  avg standing after a win %.1f\n",
		troop, *n, pct(o.Won, *n), pct(o.Lost, *n), pct(o.Fled, *n), pct(o.Draw, *n), o.AvgTurns(), o.AvgStanding())
	if lo, mid, hi, ok := o.HPBand(); ok && *hpBand {
		c.out.Printf("hp left after a win: p10 %.0f%%  median %.0f%%  p90 %.0f%%\n", 100*lo, 100*mid, 100*hi)
	}
	if *streak > 0 {
		wins, err := balance.Streak(db, troop, *n, *maxTurns, *streak, start)
		if err != nil {
			return err
		}
		c.out.Printf("wins in a row without recovery (up to %d): p10 %d  median %d  p90 %d\n", *streak,
			balance.Percentile(wins, 0.1), balance.Percentile(wins, 0.5), balance.Percentile(wins, 0.9))
	}
	return nil
}

// commaList は --party・--gear の並び。空なら nil、それ以外は空の要素も残す（未知の ID として報せる）。
func commaList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(b) }
