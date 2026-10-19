package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/internal/headless/balance"
)

func (c *command) sim(args []string) error {
	fs := flag.NewFlagSet("gozen sim", flag.ContinueOnError)
	n := fs.Int("n", 200, "battles to fight")
	party := fs.String("party", "", "the party (actor ids, comma separated; default: the starting party)")
	level := fs.Int("level", 0, "the party's level (default: each actor's own)")
	seed := fs.Uint64("seed", 1, "random seed")
	verbose := fs.Bool("v", false, "print the first battle's messages")
	maxTurns := fs.Int("turns", 60, "a battle longer than this is a draw")
	tree := fs.Bool("tree", true, "spend the skill points (on the open node of the lowest level, evenly)")
	gear := fs.String("gear", "", "items to give the party and wear where they fit best (comma separated)")
	state := fs.String("state", "", "start from this saved state (a note's .save from F9) with everyone recovered, instead of a new game")
	hpBand := fs.Bool("hp", false, "also print the party's HP left after a win (10th, 50th and 90th percentile)")
	if err := c.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return cli.Invalid("expected <game-dir> <troop>")
	}
	dir, troop := fs.Arg(0), fs.Arg(1)
	if *n <= 0 || *maxTurns <= 0 || *level < 0 {
		return cli.Invalid("-n and -turns must be positive; -level must be nonnegative")
	}
	db, err := data.Load(os.DirFS(dir), ".")
	if err != nil {
		return err
	}
	if db.Troops[troop] == nil {
		return fmt.Errorf("unknown troop %q", troop)
	}
	prep := balance.Prep{Level: *level, Tree: *tree, Gear: commaList(*gear)}
	var start balance.Start
	if *state != "" {
		if *party != "" {
			return cli.Invalid("-state and -party do not go together")
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
	return nil
}

// commaList は -party・-gear の並び。空なら nil、それ以外は空の要素も残す（未知の ID として報せる）。
func commaList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(b) }
