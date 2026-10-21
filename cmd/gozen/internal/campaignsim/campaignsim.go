// Package campaignsim plays a game's campaign many times, headless, with a player's
// strategy, and measures it (gozen sim campaign, ADR 0030): the win rate, the turns, the
// regions held turn by turn, the order the factions fall in, and the orders given.
package campaignsim

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"

	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/rpg"
)

// Strategies are the player's strategies: greedy marches when the odds (the clash's
// formulas with rand() at its middle) are good, else gives the first other order it can
// (one with a general first); random gives any order it can, or ends the turn; ai plays
// the player's faction by its own ai rules, as the other factions play.
var Strategies = []string{"greedy", "random", "ai"}

// Config is a measurement: how many campaigns, from which seed, with which strategy.
type Config struct {
	Runs     int
	Seed     uint64
	As       string
	MaxOrder int // orders a campaign may give before it is cut (0: 2000)
	// Tactic is the tactic every general marches with in a clash shown as units ("": each
	// general's own); a march then takes the most generals it can, the strongest first,
	// and is counted as order:tactic.
	Tactic         string
	IncludeRegions bool
}

// Result is what the campaigns came to.
type Result struct {
	Game    string  `json:"game"`
	Runs    int     `json:"runs"`
	Seed    uint64  `json:"seed"`
	As      string  `json:"as"`
	Tactic  string  `json:"tactic,omitempty"`
	Won     int     `json:"won"`
	Lost    int     `json:"lost"`
	Cut     int     `json:"cut"`
	WinRate float64 `json:"win_rate"`
	// Turns are how many campaigns ended in each turn, won and lost.
	Turns struct {
		Won  map[int]int `json:"won"`
		Lost map[int]int `json:"lost"`
	} `json:"turns"`
	// Regions is the mean count of regions each faction held at the start of each turn,
	// over the campaigns that reached it.
	Regions []TurnRegions `json:"regions_by_turn"`
	// Fallen are the factions that lost their last region, in the order of the mean turn
	// they fell in.
	Fallen []Fall `json:"fallen"`
	// Orders is how many times each order was given, and end_turn how many turns ended.
	Orders map[string]int `json:"orders"`
	// Marches is how the player's marches went: won or held.
	Marches       map[string]int  `json:"marches"`
	RegionMetrics []RegionBalance `json:"region_metrics,omitempty"`
	meanTurns     float64
}

// Comparison is a set of measurements made with the same seed range for each variant.
type Comparison struct {
	Game string          `json:"game"`
	Mode string          `json:"mode"`
	Runs int             `json:"runs_per_variant"`
	Seed uint64          `json:"seed"`
	Rows []ComparisonRow `json:"results"`
}

// ComparisonRow is the summary for one strategy or tactic.
type ComparisonRow struct {
	Name            string          `json:"name"`
	Won             int             `json:"won"`
	WinRate         float64         `json:"win_rate"`
	MeanTurns       float64         `json:"mean_turns"`
	MeanRegionsHeld float64         `json:"mean_regions_held"`
	Regions         []RegionBalance `json:"regions"`
}

// RegionBalance reports ownership changes and the player's outcomes for one region.
type RegionBalance struct {
	Region              string          `json:"region"`
	MeanChanges         float64         `json:"mean_changes"`
	MeanFirstTakenTurn  float64         `json:"mean_first_taken_turn,omitempty"`
	EndHolders          map[string]int  `json:"end_holders"`
	PlayerWinRateByTurn map[int]float64 `json:"player_win_rate_by_turn"`
}

// TurnRegions is the mean of regions held at the start of a turn.
type TurnRegions struct {
	Turn      int                `json:"turn"`
	Campaigns int                `json:"campaigns"`
	Mean      map[string]float64 `json:"mean"`
}

// Fall is how often and how late a faction fell.
type Fall struct {
	Faction  string  `json:"faction"`
	Runs     int     `json:"runs"`
	MeanTurn float64 `json:"mean_turn"`
}

// Run plays cfg.Runs campaigns of db, the i-th from seed cfg.Seed+i.
func Run(ctx context.Context, db *data.DB, cfg Config) (*Result, error) {
	if db.Game.Campaign == nil {
		return nil, errors.New("the game has no campaign (game.yaml campaign)")
	}
	if !slices.Contains(Strategies, cfg.As) {
		return nil, fmt.Errorf("--as is one of %v, not %q", Strategies, cfg.As)
	}
	if cfg.MaxOrder <= 0 {
		cfg.MaxOrder = 2000
	}
	r := &Result{Game: db.Game.ID, Runs: cfg.Runs, Seed: cfg.Seed, As: cfg.As, Orders: map[string]int{}, Marches: map[string]int{}, Tactic: cfg.Tactic}
	r.Turns.Won, r.Turns.Lost = map[int]int{}, map[int]int{}
	sums := map[int]map[string]float64{}
	regionChanges := map[string]int{}
	regionFirst := map[string]int{}
	regionFirstRuns := map[string]int{}
	regionEnds := map[string]map[string]int{}
	regionHeldWins := map[string]map[int]int{}
	regionHeldRuns := map[string]map[int]int{}
	totalTurns := 0
	reached := map[int]int{}
	falls := map[string][]int{}
	for i := range cfg.Runs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		seed := cfg.Seed + uint64(i)
		one, err := play(ctx, db, cfg, seed, r)
		if err != nil {
			return nil, fmt.Errorf("campaign %d (seed %d): %w", i+1, seed, err)
		}
		for turn, held := range one.held {
			reached[turn]++
			if sums[turn] == nil {
				sums[turn] = map[string]float64{}
			}
			for f, n := range held {
				sums[turn][f] += float64(n)
			}
		}
		for f, turn := range one.fell {
			falls[f] = append(falls[f], turn)
		}
		totalTurns += one.turn
		if cfg.IncludeRegions {
			for region, changes := range one.regionChanges {
				regionChanges[region] += changes
			}
			for region, turn := range one.regionFirst {
				regionFirst[region] += turn
				regionFirstRuns[region]++
			}
			for region, faction := range one.regionEnds {
				if regionEnds[region] == nil {
					regionEnds[region] = map[string]int{}
				}
				regionEnds[region][faction]++
			}
			for turn, owners := range one.regionHeld {
				for region, owner := range owners {
					if owner == db.Game.Campaign.Player {
						if regionHeldRuns[region] == nil {
							regionHeldRuns[region] = map[int]int{}
							regionHeldWins[region] = map[int]int{}
						}
						regionHeldRuns[region][turn]++
						if one.won {
							regionHeldWins[region][turn]++
						}
					}
				}
			}
		}
		switch {
		case one.won:
			r.Won++
			r.Turns.Won[one.turn]++
		case one.lost:
			r.Lost++
			r.Turns.Lost[one.turn]++
		default:
			r.Cut++
		}
	}
	if cfg.IncludeRegions {
		for _, region := range data.RegionIDs(db) {
			metric := RegionBalance{Region: region, MeanChanges: float64(regionChanges[region]) / float64(max(1, cfg.Runs)), EndHolders: regionEnds[region], PlayerWinRateByTurn: map[int]float64{}}
			if regionFirstRuns[region] > 0 {
				metric.MeanFirstTakenTurn = float64(regionFirst[region]) / float64(regionFirstRuns[region])
			}
			for turn, count := range regionHeldRuns[region] {
				metric.PlayerWinRateByTurn[turn] = float64(regionHeldWins[region][turn]) / float64(count)
			}
			r.RegionMetrics = append(r.RegionMetrics, metric)
		}
	}
	if cfg.Runs > 0 {
		r.WinRate = float64(r.Won) / float64(cfg.Runs)
		r.meanTurns = float64(totalTurns) / float64(cfg.Runs)
	}
	turns := make([]int, 0, len(sums))
	for t := range sums {
		turns = append(turns, t)
	}
	sort.Ints(turns)
	for _, t := range turns {
		mean := map[string]float64{}
		for f, s := range sums[t] {
			mean[f] = s / float64(reached[t])
		}
		r.Regions = append(r.Regions, TurnRegions{Turn: t, Campaigns: reached[t], Mean: mean})
	}
	for f, ts := range falls {
		total := 0
		for _, t := range ts {
			total += t
		}
		r.Fallen = append(r.Fallen, Fall{Faction: f, Runs: len(ts), MeanTurn: float64(total) / float64(len(ts))})
	}
	sort.Slice(r.Fallen, func(i, j int) bool {
		if r.Fallen[i].MeanTurn != r.Fallen[j].MeanTurn {
			return r.Fallen[i].MeanTurn < r.Fallen[j].MeanTurn
		}
		return r.Fallen[i].Faction < r.Fallen[j].Faction
	})
	return r, nil
}

// Compare measures every declared strategy or tactic on the same sequence of seeds.
func Compare(ctx context.Context, db *data.DB, cfg Config, mode string) (*Comparison, error) {
	if mode != "strategies" && mode != "tactics" {
		return nil, fmt.Errorf("comparison mode is strategies or tactics, not %q", mode)
	}
	names := Strategies
	if mode == "tactics" {
		names = db.Game.Campaign.Clash.TacticIDs()
	}
	out := &Comparison{Game: db.Game.ID, Mode: mode, Runs: cfg.Runs, Seed: cfg.Seed}
	for _, name := range names {
		variant := cfg
		variant.IncludeRegions = true
		if mode == "strategies" {
			variant.As = name
		} else {
			variant.Tactic = name
		}
		r, err := Run(ctx, db, variant)
		if err != nil {
			return nil, err
		}
		held := 0.0
		for _, tr := range r.Regions {
			held += tr.Mean[cfgPlayer(db)]
		}
		out.Rows = append(out.Rows, ComparisonRow{
			Name: name, Won: r.Won, WinRate: r.WinRate,
			MeanTurns: r.meanTurns, MeanRegionsHeld: held / float64(max(1, len(r.Regions))), Regions: r.RegionMetrics,
		})
	}
	return out, nil
}

func cfgPlayer(db *data.DB) string { return db.Game.Campaign.Player }

// one is how one campaign went.
type one struct {
	won, lost     bool
	turn          int
	held          map[int]map[string]int
	fell          map[string]int
	regionChanges map[string]int
	regionFirst   map[string]int
	regionEnds    map[string]string
	regionHeld    map[int]map[string]string
}

func play(ctx context.Context, db *data.DB, cfg Config, seed uint64, r *Result) (*one, error) {
	st := rpg.New(db, rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))) //nolint:gosec // Gameplay randomness, not security.
	rng := rand.New(rand.NewPCG(seed, 1))                                   //nolint:gosec // The strategy's randomness, not security.
	d := headless.New(db, st)
	if err := d.Start(); err != nil {
		return nil, err
	}
	o := &one{held: map[int]map[string]int{}, fell: map[string]int{}}
	if cfg.IncludeRegions {
		o.regionChanges, o.regionFirst = map[string]int{}, map[string]int{}
		o.regionEnds, o.regionHeld = map[string]string{}, map[int]map[string]string{}
	}
	note := func() {
		turn := data.Turn(db, st.Campaign())
		if _, ok := o.held[turn]; ok {
			return
		}
		held := map[string]int{}
		for _, f := range data.FactionIDs(db) {
			n := len(data.RegionsOf(db, st.Campaign(), f))
			held[f] = n
			if _, gone := o.fell[f]; n == 0 && !gone {
				o.fell[f] = turn
			}
		}
		o.held[turn] = held
		if cfg.IncludeRegions {
			owners := map[string]string{}
			for _, id := range data.RegionIDs(db) {
				owners[id] = data.RegionOwner(db, st.Campaign(), id)
			}
			o.regionHeld[turn] = owners
		}
	}
	note()
	for range cfg.MaxOrder {
		if d.Over || d.Cleared {
			break
		}
		cmd, id := next(db, st, cfg.As, rng)
		if cmd.Order != nil && cfg.Tactic != "" {
			if o := db.Game.Campaign.Orders[cmd.Order.ID]; o != nil && o.March && cmd.Order.General != "" {
				cmd.Order = withAll(db, st, o, cmd.Order, cfg.Tactic)
				id += ":" + cfg.Tactic
			}
		}
		r.Orders[id]++
		turnBefore := data.Turn(db, st.Campaign())
		var before map[string]string
		if cfg.IncludeRegions {
			before = map[string]string{}
			for _, id := range data.RegionIDs(db) {
				before[id] = data.RegionOwner(db, st.Campaign(), id)
			}
		}
		if err := d.Run(ctx, []data.Command{cmd}); err != nil {
			return nil, err
		}
		if cmd.Order != nil {
			if res := optional.Value[string](st.Vars[data.VarResult]); res != "" {
				r.Marches[res]++
			}
		}
		if cfg.IncludeRegions {
			for _, id := range data.RegionIDs(db) {
				after := data.RegionOwner(db, st.Campaign(), id)
				if after != before[id] {
					o.regionChanges[id]++
					if after != "" {
						if _, seen := o.regionFirst[id]; !seen {
							o.regionFirst[id] = turnBefore
						}
					}
				}
			}
		}
		note()
	}
	o.won, o.lost, o.turn = d.Cleared, d.Over, data.Turn(db, st.Campaign())
	if cfg.IncludeRegions {
		for _, id := range data.RegionIDs(db) {
			o.regionEnds[id] = data.RegionOwner(db, st.Campaign(), id)
		}
	}
	return o, nil
}

// withAll is a march with the most generals it can take, the strongest first after its
// leader, all with the tactic.
func withAll(db *data.DB, st *rpg.State, o *data.Order, oc *data.OrderCmd, tactic string) *data.OrderCmd {
	next := *oc
	var others []string
	for _, g := range campaign.Generals(db, st, o) {
		if g != oc.General {
			others = append(others, g)
		}
	}
	slices.SortStableFunc(others, func(a, b string) int {
		return cmp.Compare(data.GeneralTroops(db, st.Campaign(), b), data.GeneralTroops(db, st.Campaign(), a))
	})
	next.With = others[:min(len(others), campaign.Cap(db, o)-1)]
	next.Tactics = map[string]string{oc.General: tactic}
	for _, g := range next.With {
		next.Tactics[g] = tactic
	}
	return next.MarchNow()
}

// next is the strategy's next command and the name it is counted under.
func next(db *data.DB, st *rpg.State, as string, rng *rand.Rand) (data.Command, string) {
	end := data.Command{EndTurn: true}
	if as == "ai" {
		campaign.FactionTurn(db, st, db.Game.Campaign.Player)
		return end, "end_turn"
	}
	type move struct {
		id, general, target string
		march               bool
		score               float64
	}
	var moves []move
	c := db.Game.Campaign
	for _, id := range c.OrderIDs() {
		o := c.Orders[id]
		if o == nil || campaign.Open(db, st, o) != "" {
			continue
		}
		generals := []string{""}
		if o.General != "" {
			generals = campaign.Generals(db, st, o)
		}
		targets := []string{""}
		if o.Target != "" {
			targets = campaign.Targets(db, st, o)
		}
		for _, g := range generals {
			for _, t := range targets {
				m := move{id: id, general: g, target: t, march: o.March}
				if o.March {
					power, defense := campaign.Estimate(db, st, g, t)
					m.score = power / max(defense, 1)
				} else if g != "" {
					m.score = data.GeneralTroops(db, st.Campaign(), g)
				}
				moves = append(moves, m)
			}
		}
	}
	if len(moves) == 0 {
		return end, "end_turn"
	}
	var pick move
	switch as {
	case "random":
		if rng.IntN(len(moves)+1) == len(moves) {
			return end, "end_turn"
		}
		pick = moves[rng.IntN(len(moves))]
	default: // greedy
		best := -1
		for i, m := range moves {
			if m.march && m.score > 1.15 && (best < 0 || m.score > moves[best].score) {
				best = i
			}
		}
		if best < 0 { // no good march: an order with a general (the strongest), else the first other
			for i, m := range moves {
				if !m.march && m.general != "" && (best < 0 || m.score > moves[best].score) {
					best = i
				}
			}
		}
		if best < 0 {
			for i, m := range moves {
				if !m.march {
					best = i
					break
				}
			}
		}
		if best < 0 {
			return end, "end_turn"
		}
		pick = moves[best]
	}
	return data.Command{Order: &data.OrderCmd{ID: pick.id, General: pick.general, Target: pick.target}}, pick.id
}
