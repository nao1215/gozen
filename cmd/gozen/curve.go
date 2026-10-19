package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/headless/balance"
)

func (c *command) curve(args []string) error {
	fs := clihelp.NewFlagSet("gozen curve")
	o := balance.Config{}
	fs.IntVar(&o.Floors, "floors", 20, "how many floors to go down")
	fs.IntVar(&o.Battles, "battles", 7, "battles fought on each floor")
	bosses := fs.String("boss", "", "the bosses as `FLOOR:TROOP` pairs separated by commas, fought before going down from that floor")
	shops := fs.String("shops", "", "shop `IDS`, separated by commas, whose equipment is bought when it is better and affordable")
	fs.IntVar(&o.Tries, "tries", 30, "battles against each boss to measure its win rate")
	fs.Float64Var(&o.Grind, "grind", 0, "before a boss, fight more battles on its floor until the win rate reaches this `RATE` (0 to 1)")
	fs.Uint64Var(&o.Seed, "seed", 1, "the random seed; the same seed gives the same result")
	fs.IntVar(&o.MaxTurns, "turns", 60, "a battle longer than this many turns is a draw")
	fs.IntVar(&o.RestEvery, "rest", 0, "rest (HP and MP full) only every `N` floors and after a boss; 0 rests on every floor")
	supplies := fs.String("supplies", "", "`ITEM:COUNT` pairs, separated by commas, bought at each rest (with --rest)")
	fs.BoolVar(&o.Events, "events", false, "run the game's after_win event after each win and the game_over event after a loss")
	fs.StringVar(&o.RestCost, "rest-cost", "", "the gold a rest costs, as an `EXPR` such as \"members * 10\"")
	fs.StringVar(&o.RestEvent, "rest-event", "", "an `EVENT` run at each rest")
	fs.StringVar(&o.BackItem, "back-item", "", "the `ITEM` used up to go back to rest (buy it with --supplies)")
	fs.Float64Var(&o.WornHP, "worn-hp", 0, "go back early (with --rest) when the party's HP at the end of a floor is below this `RATE` (0 to 1)")
	fs.Float64Var(&o.WornMP, "worn-mp", 0, "go back early (with --rest) when the party's MP is below this `RATE` (0 to 1)")
	fs.StringVar(&o.SceneEvent, "scene-event", "", "an `EVENT` the player sees after a won battle whenever --scene-when holds (a reward scene)")
	fs.StringVar(&o.SceneWhen, "scene-when", "", "the `EXPR` that opens --scene-event, such as a full gauge")
	if err := c.parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return cli.Invalid("expected <game-dir> <location>")
	}
	dir, loc := fs.Arg(0), fs.Arg(1)
	o.Location = loc
	if o.Floors <= 0 || o.Battles < 0 || o.Tries <= 0 || o.MaxTurns <= 0 || o.RestEvery < 0 {
		return cli.Invalid("invalid floor, battle, try, turn, or rest count")
	}
	for _, v := range []float64{o.Grind, o.WornHP, o.WornMP} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return cli.Invalid("rates must be finite values in [0,1]")
		}
	}
	db, err := data.Load(os.DirFS(dir), ".")
	if err != nil {
		return err
	}
	if db.Locations[loc] == nil {
		return fmt.Errorf("unknown location %q", loc)
	}
	o.Bosses = map[int]string{}
	for _, v := range split(*bosses) {
		a, b, ok := strings.Cut(v, ":")
		f, err := strconv.Atoi(a)
		if !ok || err != nil || f <= 0 || b == "" || o.Bosses[f] != "" {
			return cli.Invalid("--boss: expected unique positive floor:troop pairs")
		}
		o.Bosses[f] = b
	}
	for f, t := range o.Bosses {
		if db.Troops[t] == nil {
			return fmt.Errorf("--boss %d: unknown troop %q", f, t)
		}
	}
	if *supplies != "" {
		if o.Supplies, err = parseSupplies(db, *supplies); err != nil {
			return err
		}
	}
	if o.RestEvent != "" && db.Events[o.RestEvent] == nil {
		return fmt.Errorf("--rest-event: unknown event %q", o.RestEvent)
	}
	if o.SceneEvent != "" && db.Events[o.SceneEvent] == nil {
		return fmt.Errorf("--scene-event: unknown event %q", o.SceneEvent)
	}
	if (o.SceneEvent == "") != (o.SceneWhen == "") {
		return fmt.Errorf("--scene-event and --scene-when go together")
	}
	if o.SceneWhen != "" {
		if _, err := expr.Parse(o.SceneWhen); err != nil {
			return fmt.Errorf("--scene-when: %w", err)
		}
	}
	if o.BackItem != "" && db.Items[o.BackItem] == nil {
		return fmt.Errorf("--back-item: unknown item %q", o.BackItem)
	}
	if *shops != "" {
		o.Shops = strings.Split(*shops, ",")
		for _, id := range o.Shops {
			if db.Shops[id] == nil {
				return cli.Invalid("unknown shop %q", id)
			}
		}
	}
	if o.RestCost != "" {
		if _, err := expr.Parse(o.RestCost); err != nil {
			return cli.Invalid("--rest-cost: %v", err)
		}
	}
	r, err := balance.Run(db, o)
	if err != nil {
		return err
	}
	c.out.Printf("%-5s %-4s %-7s %-6s %-6s %-7s %-6s %-6s %-6s %-7s %s\n", "floor", "Lv", "battles", "lost", "turns", "hp_left", "mp", "gold", "scenes", "boss", "troops")
	for _, f := range r.Floors {
		boss := "-"
		if f.Boss != "" {
			boss = fmt.Sprintf("%3.0f%%", 100*f.BossRate)
			if f.Grinding > 0 {
				boss += fmt.Sprintf("→%.0f%%(+%d戦)", 100*f.BossRateEnd, f.Grinding)
			}
		}
		c.out.Printf("B%-4d %-4d %-7d %-6d %-6.1f %-7.2f %-6.2f %-6d %-6d %-7s %s\n", f.Floor, f.Level, f.Battles, f.Lost, f.Turns, f.HPLeft, f.MPLeft, f.Gold, f.Scenes, boss, strings.Join(f.Troops, " "))
		if f.Spent+f.Unpaid+f.Stranded+f.GoldLost > 0 {
			c.out.Printf("      spent %dG, lost %dG, unpaid rests %d, back without %s %d\n", f.Spent, f.GoldLost, f.Unpaid, o.BackItem, f.Stranded)
		}
	}
	c.out.Println("\nskills used (the run's own battles and the boss tries):")
	unused := r.Unused()
	for _, id := range r.Order {
		type kv struct {
			k string
			v int
		}
		var list []kv
		for k, v := range r.Used[id] {
			list = append(list, kv{k, v})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v || list[i].v == list[j].v && list[i].k < list[j].k })
		var parts []string
		for _, e := range list {
			parts = append(parts, fmt.Sprintf("%s %d", name(db, e.k), e.v))
		}
		c.out.Printf("  %s Lv%d: %s\n", r.Names[id], r.Level[id], strings.Join(parts, ", "))
		if u := unused[id]; len(u) > 0 {
			var names []string
			for _, s := range u {
				names = append(names, name(db, s))
			}
			c.out.Printf("    never used: %s\n", strings.Join(names, ", "))
		}
	}
	return nil
}

func parseSupplies(db *data.DB, s string) (map[string]int, error) {
	out := map[string]int{}
	for _, p := range strings.Split(s, ",") {
		id, n, ok := strings.Cut(strings.TrimSpace(p), ":")
		if !ok || id == "" || n == "" {
			return nil, fmt.Errorf("--supplies %q: want item:count", p)
		}
		if db.Items[id] == nil {
			return nil, fmt.Errorf("--supplies %q: unknown item %q", p, id)
		}
		c, err := strconv.Atoi(n)
		if err != nil || c <= 0 {
			return nil, fmt.Errorf("--supplies %q: the count must be a positive integer", p)
		}
		out[id] += c
	}
	return out, nil
}

func name(db *data.DB, id string) string {
	if sk := db.Skills[id]; sk != nil {
		return sk.Name
	}
	if it := db.Items[id]; it != nil {
		return it.Name
	}
	return id
}
