package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/rpg"
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
	var wins, losses, flees, draws, turns, survivors int
	for i := range *n {
		rng := rand.New(rand.NewPCG(*seed, uint64(i))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		st := rpg.New(db, rng)
		if *party != "" {
			st.Party = nil
			for _, id := range strings.Split(*party, ",") {
				if db.Actors[id] == nil {
					return fmt.Errorf("unknown actor %q", id)
				}
				st.Join(id)
			}
		}
		if *level > 0 {
			for _, a := range st.Members() {
				a.GainExp(a.ExpFor(*level) - a.Exp)
				a.Recover()
			}
		}
		for _, a := range st.Members() {
			if *tree {
				a.SpendSP()
			}
		}
		if *gear != "" {
			for _, id := range strings.Split(*gear, ",") {
				if db.Items[id] == nil {
					return fmt.Errorf("unknown item %q", id)
				}
				st.AddItem(id, len(st.Members()))
			}
			for _, a := range st.Members() {
				st.EquipBest(a)
				a.Recover()
			}
		}
		b := battle.New(db, st, troop)
		turn, begun := 0, false

		headless.AutoBattle(db, st, b, nil, *maxTurns, func(steps []battle.Step) {
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
		turns += b.Turn
		switch b.Result {
		case battle.Won:
			wins++
			for _, u := range b.Party {
				if u.Alive() {
					survivors++
				}
			}
		case battle.Lost:
			losses++
		case battle.Fled:
			flees++
		case battle.Ongoing:
			draws++
		}
	}
	c.out.Printf("%s: %d battles  win %.0f%%  lose %.0f%%  fled %.0f%%  draw %.0f%%  avg turns %.1f  avg standing after a win %.1f\n",
		troop, *n, pct(wins, *n), pct(losses, *n), pct(flees, *n), pct(draws, *n), float64(turns)/float64(*n), float64(survivors)/float64(max(1, wins)))
	return nil
}

func pct(a, b int) float64 { return 100 * float64(a) / float64(b) }
