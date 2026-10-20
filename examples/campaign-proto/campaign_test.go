package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nao1215/gozen"
)

// The campaign as the Go side has to know it: the engine has no regions, so the harness
// repeats the borders, the names and the generals that the YAML spells out.
var (
	regions = []string{"vale", "ford", "cross", "hill", "well"}
	borders = map[string][]string{
		"vale":  {"cross", "ford"},
		"ford":  {"cross", "vale"},
		"cross": {"ford", "hill", "well", "vale"},
		"hill":  {"cross", "well"},
		"well":  {"cross", "hill"},
	}
	generals = []string{"maren", "ox", "tilly", "kes"}
)

// planner answers the choices of the orders with the labels the strategy wants, and the
// choices of the talk that breaks into the turns with its own answers.
type planner struct {
	prefer []string
	talk   map[string]string // a word of the talk's options to the option to take
}

func (p *planner) Decide(_ context.Context, _ *gozen.Session, q gozen.Request) int {
	if q.Kind != "choice" {
		return gozen.Auto
	}
	for _, want := range p.prefer {
		for i, o := range q.Options {
			if strings.HasPrefix(o, want) {
				return i
			}
		}
	}
	for _, o := range q.Options {
		if want, ok := p.talk[o]; ok {
			for i, o2 := range q.Options {
				if o2 == want {
					return i
				}
			}
		}
	}
	// Nothing wanted (an order whose target is gone): take Cancel, the last option.
	return len(q.Options) - 1
}

// view is what a strategy reads of the state, through Query.
type view struct {
	turn, ap, gold float64
	own            map[string]string
	troops         map[string]float64
	name           map[string]string
	gt, mt         map[string]float64
	ready          map[string]bool // has joined and has not marched this turn
	has            map[string]bool
}

func read(t testing.TB, s *gozen.Session) view {
	t.Helper()
	v := view{
		own: map[string]string{}, troops: map[string]float64{}, name: map[string]string{},
		gt: map[string]float64{}, mt: map[string]float64{}, ready: map[string]bool{}, has: map[string]bool{},
	}
	q := func(src string, out any) {
		if err := s.Query(src, out); err != nil {
			t.Fatalf("%s: %v", src, err)
		}
	}
	q("var.turn", &v.turn)
	q("var.ap", &v.ap)
	q("gold", &v.gold)
	for _, r := range regions {
		var own, name string
		var tr float64
		q("var.own_"+r, &own)
		q("var.nm_"+r, &name)
		q("var.tr_"+r, &tr)
		v.own[r], v.name[r], v.troops[r] = own, name, tr
	}
	for _, g := range generals {
		var gt, mt float64
		var acted bool
		q("var.gt_"+g, &gt)
		q("var.mt_"+g, &mt)
		q("flag.acted_"+g, &acted)
		has := g == "maren" || g == "ox"
		if !has {
			q("flag.has_"+g, &has)
		}
		v.gt[g], v.mt[g], v.has[g] = gt, mt, has
		v.ready[g] = has && !acted && gt >= 100
	}
	return v
}

func title(id string) string { return strings.ToUpper(id[:1]) + id[1:] }

// targets are the regions the Crown can attack: not its own, next to one of its own.
func (v view) targets() []string {
	var out []string
	for _, r := range regions {
		if v.own[r] == "Crown" {
			continue
		}
		for _, n := range borders[r] {
			if v.own[n] == "Crown" {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// order is one order: the menu entry to pick and the options to take in its choices.
type order struct {
	pick   string
	prefer []string
}

type strategy func(v view, rng *rand.Rand) order

// greedy marches when the odds look good (the strongest ready general on the weakest
// target), else drafts into the strongest general, else develops, else ends the turn.
func greedy(v view, _ *rand.Rand) order {
	if v.ap <= 0 {
		return order{pick: "End turn"}
	}
	best, bestG, bestR := 1.15, "", ""
	for _, g := range generals {
		if !v.ready[g] {
			continue
		}
		for _, r := range v.targets() {
			odds := v.gt[g] * (1 + v.mt[g]/10) / (v.troops[r]*1.25 + 1)
			if odds > best {
				best, bestG, bestR = odds, g, r
			}
		}
	}
	if bestG != "" {
		return order{pick: "Attack", prefer: []string{title(bestG) + " (", v.name[bestR] + " ("}}
	}
	if v.gold >= 50 {
		strongest := ""
		for _, g := range generals {
			if v.has[g] && (strongest == "" || v.gt[g] > v.gt[strongest]) {
				strongest = g
			}
		}
		return order{pick: "Draft", prefer: []string{title(strongest) + " ("}}
	}
	return order{pick: "Develop", prefer: []string{v.name["vale"] + " ("}}
}

// random takes any order the menu offers, with any general and target.
func random(v view, rng *rand.Rand) order {
	if v.ap <= 0 || rng.IntN(8) == 0 {
		return order{pick: "End turn"}
	}
	var ready []string
	for _, g := range generals {
		if v.ready[g] {
			ready = append(ready, g)
		}
	}
	targets := v.targets()
	switch n := rng.IntN(3); {
	case n == 0 && len(ready) > 0 && len(targets) > 0:
		g, r := ready[rng.IntN(len(ready))], targets[rng.IntN(len(targets))]
		return order{pick: "Attack", prefer: []string{title(g) + " (", v.name[r] + " ("}}
	case n == 1 && v.gold >= 50:
		var has []string
		for _, g := range generals {
			if v.has[g] {
				has = append(has, g)
			}
		}
		return order{pick: "Draft", prefer: []string{title(has[rng.IntN(len(has))]) + " ("}}
	default:
		var own []string
		for _, r := range regions {
			if v.own[r] == "Crown" {
				own = append(own, r)
			}
		}
		return order{pick: "Develop", prefer: []string{v.name[own[rng.IntN(len(own))]] + " ("}}
	}
}

// outcome is how a campaign ended.
type outcome struct {
	won, lost bool
	turn      float64
	regions   float64
	orders    int
}

func loadGame(t testing.TB) *gozen.Game {
	t.Helper()
	fsys, err := gameFS()
	if err != nil {
		t.Fatal(err)
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

var answers = map[string]string{"Take her in": "Take her in", "Accept the truce": "Accept the truce"}

func newCampaign(t testing.TB, g *gozen.Game, seed uint64) (*gozen.Session, *planner) {
	t.Helper()
	p := &planner{talk: answers}
	s, err := gozen.New(g, gozen.Config{Seed: seed, Policy: p})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, p
}

func ended(t testing.TB, s *gozen.Session) bool {
	t.Helper()
	var cleared bool
	if err := s.Query("cleared", &cleared); err != nil {
		t.Fatal(err)
	}
	return cleared
}

// march gives orders until the campaign ends (or the turn untilTurn begins, when not 0).
func march(t testing.TB, s *gozen.Session, p *planner, plan strategy, rng *rand.Rand, untilTurn float64) outcome {
	t.Helper()
	var o outcome
	for range 500 {
		if ended(t, s) {
			break
		}
		v := read(t, s)
		if untilTurn > 0 && v.turn >= untilTurn {
			return o
		}
		next := plan(v, rng)
		p.prefer = next.prefer
		if err := s.Do(context.Background(), "- pick: "+next.pick+"\n"); err != nil {
			t.Fatalf("turn %v, %s %v: %v", v.turn, next.pick, next.prefer, err)
		}
		o.orders++
	}
	if !ended(t, s) {
		t.Fatal("the campaign did not end in 500 orders")
	}
	v := read(t, s)
	if err := s.Query("flag.won", &o.won); err != nil {
		t.Fatal(err)
	}
	if err := s.Query("flag.lost", &o.lost); err != nil {
		t.Fatal(err)
	}
	o.turn = v.turn
	for _, r := range regions {
		if v.own[r] == "Crown" {
			o.regions++
		}
	}
	return o
}

func campaign(t testing.TB, g *gozen.Game, seed uint64, plan strategy) outcome {
	t.Helper()
	s, p := newCampaign(t, g, seed)
	return march(t, s, p, plan, rand.New(rand.NewPCG(seed, 7)), 0) //nolint:gosec // Test play, not security.
}

// TestWinRate plays 200 campaigns with each strategy and reports the win rate and the
// turns, the numbers a sim campaign command would print.
func TestWinRate(t *testing.T) {
	g := loadGame(t)
	const runs = 200
	rates := map[string]float64{}
	for _, c := range []struct {
		name string
		plan strategy
	}{{"greedy", greedy}, {"random", random}} {
		wins, losses := 0, 0
		var winTurns []float64
		held := map[float64]int{}
		for seed := uint64(1); seed <= runs; seed++ {
			o := campaign(t, g, seed, c.plan)
			if o.won == o.lost {
				t.Fatalf("%s seed %d: won %v and lost %v", c.name, seed, o.won, o.lost)
			}
			if o.won {
				wins++
				winTurns = append(winTurns, o.turn)
			} else {
				losses++
			}
			held[o.regions]++
		}
		sort.Float64s(winTurns)
		median := 0.0
		if len(winTurns) > 0 {
			median = winTurns[len(winTurns)/2]
		}
		rates[c.name] = float64(wins) / runs
		t.Logf("%s: won %d of %d (%.0f%%), median turn of a win %.0f, regions held at the end %v",
			c.name, wins, runs, 100*rates[c.name], median, held)
	}
	if r := rates["greedy"]; r < 0.4 || r > 0.7 {
		t.Errorf("greedy wins %.0f%%, want 40%% to 70%%", 100*r)
	}
	if rates["random"] >= rates["greedy"] {
		t.Errorf("random play wins as often as greedy play (%.2f >= %.2f)", rates["random"], rates["greedy"])
	}
}

// TestSameSeed plays the same campaign twice: the same seed gives the same end.
func TestSameSeed(t *testing.T) {
	g := loadGame(t)
	for seed := uint64(1); seed <= 20; seed++ {
		a, b := campaign(t, g, seed, greedy), campaign(t, g, seed, greedy)
		if a != b {
			t.Fatalf("seed %d: %+v then %+v", seed, a, b)
		}
	}
}

// snapshot is every declared name of the state, read with Query.
func snapshot(t *testing.T, s *gozen.Session) map[string]any {
	t.Helper()
	b, err := os.ReadFile("game/game.yaml")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for line := range strings.SplitSeq(string(b), "\n") {
		name, _, ok := strings.Cut(strings.TrimSpace(line), ": {type:")
		if !ok {
			continue
		}
		var v any
		if err := s.Query(name, &v); err != nil {
			t.Fatal(err)
		}
		out[name] = v
	}
	for _, n := range []string{"gold", "party", "location"} {
		var v any
		if err := s.Query(n, &v); err != nil {
			t.Fatal(err)
		}
		out[n] = v
	}
	return out
}

func diff(a, b map[string]any) []string {
	var out []string
	for k, v := range a {
		if fmt.Sprint(v) != fmt.Sprint(b[k]) {
			out = append(out, fmt.Sprintf("%s: %v != %v", k, v, b[k]))
		}
	}
	slices.Sort(out)
	return out
}

// TestSaveRestore saves at the start of turn 5 and restores: the state is the same, and
// the restored campaign plays on to an end. A save made between commands does not keep
// the random numbers (a restored session draws from Config.Seed again), so the ends of
// the original and of the restored campaign are only counted; a Clone, which copies the
// random numbers, must end the same every time (see NOTES.md).
func TestSaveRestore(t *testing.T) {
	g := loadGame(t)
	saved, restoredSame := 0, 0
	for seed := uint64(1); seed <= 20; seed++ {
		s, p := newCampaign(t, g, seed)
		march(t, s, p, greedy, nil, 5)
		if ended(t, s) {
			continue
		}
		saved++
		save, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		pr := &planner{talk: answers}
		r, err := gozen.Restore(g, save, gozen.Config{Seed: seed, Policy: pr})
		if err != nil {
			t.Fatal(err)
		}
		if d := diff(snapshot(t, s), snapshot(t, r)); len(d) > 0 {
			t.Fatalf("seed %d: the restored state differs: %v", seed, d)
		}
		c, err := s.Clone()
		if err != nil {
			t.Fatal(err)
		}
		// The clone keeps the session's Config, so its Policy is the same planner.
		cloned := march(t, c, p, greedy, nil, 0)
		a := march(t, s, p, greedy, nil, 0)
		b := march(t, r, pr, greedy, nil, 0)
		if a != cloned {
			t.Errorf("seed %d: the clone ended %+v, the original %+v", seed, cloned, a)
		}
		if a == b {
			restoredSame++
		}
	}
	if saved == 0 {
		t.Fatal("every campaign ended before turn 5")
	}
	t.Logf("of %d campaigns saved at turn 5, %d ended the same after Restore", saved, restoredSame)
}

// TestTalkBranches explores the end of turn 2: the talk of turn 3 is the only choice, so
// Explore finds both answers to Tilly, and only taking her in makes her a general.
func TestTalkBranches(t *testing.T) {
	g := loadGame(t)
	s, err := gozen.New(g, gozen.Config{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Do(context.Background(), "- pick: End turn\n"); err != nil {
		t.Fatal(err)
	}
	paths, err := gozen.Explore(context.Background(), s, "- pick: End turn\n", gozen.ExploreOptions{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if len(path.Errs) > 0 {
			t.Fatalf("answers %v: %v", path.Answers, path.Errs)
		}
		var tilly string
		var has bool
		if err := path.End.Query("chose.tilly", &tilly); err != nil {
			t.Fatal(err)
		}
		if err := path.End.Query("flag.has_tilly", &has); err != nil {
			t.Fatal(err)
		}
		if has != (tilly == "0") {
			t.Errorf("answers %v: chose %q but has_tilly %v", path.Answers, tilly, has)
		}
		seen[tilly] = true
	}
	if !seen["0"] || !seen["1"] {
		t.Fatalf("Explore reached the answers %v, want both", seen)
	}
}
