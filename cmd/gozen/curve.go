package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/internal/cliio"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"

	"github.com/nao1215/gozen/cmd/internal/cli"
	"github.com/nao1215/gozen/internal/clihelp"
	"github.com/nao1215/gozen/internal/diag"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/headless/balance"
	data "github.com/nao1215/gozen/schema"

	lang "github.com/nao1215/gozen/internal/text"
)

func (c *command) curve() clihelp.Operation {
	fs := clihelp.NewFlagSet("gozen sim dive")
	var loc string
	fs.StringVar(&loc, "location", "", "the location `ID`; required")
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
	guards := fs.String("guards", "", "the troops before the stairs down of the other floors, as `FLOOR:TROOP` pairs separated by commas, fought at the end of the floor's battles")
	fs.BoolVar(&o.Retreat, "retreat", false, "with --guards and --grind, go back to rest, buy, and train when the win rate against a guard is below the grind rate")
	fs.BoolVar(&o.GuardsOnly, "guards-only", false, "measure the guards' win rates without fighting them")
	fs.BoolVar(&o.NoTown, "no-town", false, "never go back to town: no rest, shop, or skill points between floors")
	fs.StringVar(&o.FloorEvent, "floor-event", "", "an `EVENT` run on arriving at each floor (the game's own happenings, such as someone joining)")
	saves := fs.String("arrival-saves", "", "write the party's state on arriving at each boss floor to this new `DIR` as floor-<N>.json (read it with gozen sim battle --state)")
	c.reporting(fs)
	return clihelp.Operation{Flags: fs, Run: func(_ context.Context) (err error) {
		if fs.NArg() != 1 {
			return cliio.Invalid(diag.Operands, "expected one game directory")
		}
		if loc == "" {
			return cliio.Invalid(diag.OptionRequired, "--location ID is required")
		}
		dir := fs.Arg(0)
		o.Location = loc
		if o.Floors <= 0 || o.Battles < 0 || o.Tries <= 0 || o.MaxTurns <= 0 || o.RestEvery < 0 {
			return cliio.Invalid(diag.BadValue, "invalid floor, battle, try, turn, or rest count")
		}
		for _, v := range []float64{o.Grind, o.WornHP, o.WornMP} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
				return cliio.Invalid(diag.BadValue, "rates must be finite values in [0,1]")
			}
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
		if db.Locations[loc] == nil {
			return cliio.Invalid(diag.UnknownName, "unknown location %q", loc)
		}
		o.Bosses = map[int]string{}
		for _, v := range split(*bosses) {
			a, b, ok := strings.Cut(v, ":")
			f, err := strconv.Atoi(a)
			if !ok || err != nil || f <= 0 || b == "" || o.Bosses[f] != "" {
				return cliio.Invalid(diag.BadValue, "--boss: expected unique positive floor:troop pairs")
			}
			o.Bosses[f] = b
		}
		for f, t := range o.Bosses {
			if db.Troops[t] == nil {
				return cliio.Invalid(diag.UnknownName, "--boss %d: unknown troop %q", f, t)
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
			return errors.New("--scene-event and --scene-when go together")
		}
		if o.SceneWhen != "" {
			if _, err := expr.Parse(o.SceneWhen); err != nil {
				return fmt.Errorf("--scene-when: %w", err)
			}
		}
		if o.BackItem != "" && db.Items[o.BackItem] == nil {
			return fmt.Errorf("--back-item: unknown item %q", o.BackItem)
		}
		o.Guards = map[int]string{}
		for _, v := range split(*guards) {
			a, b, ok := strings.Cut(v, ":")
			f, err := strconv.Atoi(a)
			if !ok || err != nil || f <= 0 || db.Troops[b] == nil || o.Guards[f] != "" {
				return cliio.Invalid(diag.BadValue, "--guards: expected unique positive floor:troop pairs of known troops")
			}
			o.Guards[f] = b
		}
		if len(o.Guards) == 0 {
			o.Guards = nil
		}
		if o.FloorEvent != "" && db.Events[o.FloorEvent] == nil {
			return fmt.Errorf("--floor-event: unknown event %q", o.FloorEvent)
		}
		if *saves != "" {
			if err := cli.Destination(*saves, dir); err != nil {
				return err
			}
			if err := os.Mkdir(*saves, 0o750); err != nil {
				return err
			}
			var saveErr error
			o.Arrive = func(floor int, st *rpg.State) {
				b, err := st.Save()
				if err == nil {
					err = os.WriteFile(filepath.Join(*saves, fmt.Sprintf("floor-%d.json", floor)), b, 0o600)
				}
				saveErr = errors.Join(saveErr, err)
			}
			defer func() { err = errors.Join(err, saveErr) }()
		}
		if *shops != "" {
			o.Shops = strings.Split(*shops, ",")
			for _, id := range o.Shops {
				if db.Shops[id] == nil {
					return cliio.Invalid(diag.UnknownName, "unknown shop %q", id)
				}
			}
		}
		if o.RestCost != "" {
			if _, err := expr.Parse(o.RestCost); err != nil {
				return cliio.Invalid(diag.BadValue, "--rest-cost: %v", err)
			}
		}
		r, err := balance.Run(db, o)
		if err != nil {
			return err
		}
		if c.report.Machine() {
			if c.report.Format == "jsonl" {
				for _, floor := range r.Floors {
					if err := c.report.Item(c.out, strconv.Itoa(floor.Floor), "ok", floor); err != nil {
						return err
					}
				}
			}
			return c.report.Result(c.out, "ok", r)
		}
		c.out.Printf("%-5s %-4s %-7s %-6s %-6s %-7s %-6s %-6s %-6s %-7s %s\n", "floor", "Lv", "battles", "lost", "turns", "hp_left", "mp", "gold", "scenes", "boss", "troops")
		for _, f := range r.Floors {
			boss := "-"
			if f.Boss != "" {
				boss = fmt.Sprintf("%3.0f%%", 100*f.BossRate)
				if f.Grinding > 0 {
					boss += fmt.Sprintf(lang.T("→%.0f%%(+%d戦)"), 100*f.BossRateEnd, f.Grinding)
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
	}}
}

func parseSupplies(db *data.DB, s string) (map[string]int, error) {
	out := map[string]int{}
	for p := range strings.SplitSeq(s, ",") {
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
