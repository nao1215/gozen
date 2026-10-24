//go:build !js

package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sort"
	"testing"

	"github.com/nao1215/gozen"
)

// talk answers the choices of the talk between the turns.
var talk = gozen.DefaultPolicy{Prefer: []string{"Take her in", "Accept the truce"}}

func loadGame(t testing.TB) *gozen.Game {
	t.Helper()
	fsys := os.DirFS("game")
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func newCampaign(t testing.TB, g *gozen.Game, seed uint64) *gozen.Session {
	t.Helper()
	s, err := gozen.New(g, gozen.Config{Seed: seed, Policy: talk})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func query[T any](t testing.TB, s *gozen.Session, src string) T {
	t.Helper()
	var v T
	if err := s.Query(src, &v); err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

// view is what a strategy reads: the regions, the generals and the campaign, each as
// the expressions give them (region, generals, campaign).
type view struct {
	Regions  map[string]region
	Generals map[string]general
	Campaign struct {
		Turn        float64 `json:"turn"`
		ActionsLeft float64 `json:"actions_left"`
	}
	Gold   float64
	Orders map[string]struct {
		Open bool `json:"open"`
	}
}

type region struct {
	Name   string   `json:"name"`
	Owner  string   `json:"owner"`
	Troops float64  `json:"troops"`
	Border bool     `json:"border"`
	Next   []string `json:"next"`
}

type general struct {
	Troops float64 `json:"troops"`
	Might  float64 `json:"might"`
	Ready  bool    `json:"ready"`
	Joined bool    `json:"joined"`
}

func read(t testing.TB, s *gozen.Session) view {
	t.Helper()
	return view{
		Regions:  query[map[string]region](t, s, "region"),
		Generals: query[map[string]general](t, s, "generals"),
		Campaign: query[struct {
			Turn        float64 `json:"turn"`
			ActionsLeft float64 `json:"actions_left"`
		}](t, s, "campaign"),
		Gold: query[float64](t, s, "gold"),
		Orders: query[map[string]struct {
			Open bool `json:"open"`
		}](t, s, "orders"),
	}
}

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// strategy is the next command: an order with its general and target, or the end of the turn.
type strategy func(v view, rng *rand.Rand) string

// greedy marches when the odds look good (the strongest ready general on the weakest
// target), else drafts into the strongest general, else develops, else ends the turn.
func greedy(v view, _ *rand.Rand) string {
	best, bestG, bestR := 1.15, "", ""
	for _, g := range sorted(v.Generals) {
		gen := v.Generals[g]
		if !gen.Ready || gen.Troops < 100 {
			continue
		}
		for _, r := range sorted(v.Regions) {
			if !v.Regions[r].Border {
				continue
			}
			if odds := gen.Troops * (1 + gen.Might/10) / (v.Regions[r].Troops*1.25 + 1); odds > best {
				best, bestG, bestR = odds, g, r
			}
		}
	}
	if bestG != "" && v.Orders["attack"].Open {
		return fmt.Sprintf("- order: {id: attack, general: %s, target: %s}\n", bestG, bestR)
	}
	if v.Gold >= 50 && v.Orders["draft"].Open {
		strongest := ""
		for _, g := range sorted(v.Generals) {
			if v.Generals[g].Joined && (strongest == "" || v.Generals[g].Troops > v.Generals[strongest].Troops) {
				strongest = g
			}
		}
		return fmt.Sprintf("- order: {id: draft, general: %s}\n", strongest)
	}
	if v.Orders["develop"].Open {
		return "- order: {id: develop, target: vale}\n"
	}
	return "- end_turn: true\n"
}

// random gives any order the turn allows, with any general and target.
func random(v view, rng *rand.Rand) string {
	if rng.IntN(8) == 0 {
		return "- end_turn: true\n"
	}
	var ready, joined, targets, own []string
	for _, g := range sorted(v.Generals) {
		if v.Generals[g].Ready {
			ready = append(ready, g)
		}
		if v.Generals[g].Joined {
			joined = append(joined, g)
		}
	}
	for _, r := range sorted(v.Regions) {
		if v.Regions[r].Border {
			targets = append(targets, r)
		}
		if v.Regions[r].Owner == "crown" {
			own = append(own, r)
		}
	}
	switch n := rng.IntN(3); {
	case n == 0 && len(ready) > 0 && len(targets) > 0 && v.Orders["attack"].Open:
		return fmt.Sprintf("- order: {id: attack, general: %s, target: %s}\n", ready[rng.IntN(len(ready))], targets[rng.IntN(len(targets))])
	case n == 1 && v.Gold >= 50 && v.Orders["draft"].Open:
		return fmt.Sprintf("- order: {id: draft, general: %s}\n", joined[rng.IntN(len(joined))])
	case v.Orders["develop"].Open:
		return fmt.Sprintf("- order: {id: develop, target: %s}\n", own[rng.IntN(len(own))])
	}
	return "- end_turn: true\n"
}

// outcome is how a campaign ended.
type outcome struct {
	won, lost bool
	turn      float64
	regions   float64
}

// march gives commands until the campaign ends (or the turn untilTurn begins, when not 0).
func march(t testing.TB, s *gozen.Session, plan strategy, rng *rand.Rand, untilTurn float64) outcome {
	t.Helper()
	for range 500 {
		if query[bool](t, s, "over") || query[bool](t, s, "cleared") {
			break
		}
		v := read(t, s)
		if untilTurn > 0 && v.Campaign.Turn >= untilTurn {
			return outcome{}
		}
		if err := s.Do(context.Background(), plan(v, rng)); err != nil {
			t.Fatalf("turn %v: %v", v.Campaign.Turn, err)
		}
	}
	o := outcome{won: query[bool](t, s, "cleared"), lost: query[bool](t, s, "over"), turn: query[float64](t, s, "campaign.turn"), regions: query[float64](t, s, "faction.crown.regions")}
	if o.won == o.lost {
		t.Fatalf("the campaign did not end once: won %v, lost %v", o.won, o.lost)
	}
	return o
}

func campaign(t testing.TB, g *gozen.Game, seed uint64, plan strategy) outcome {
	t.Helper()
	return march(t, newCampaign(t, g, seed), plan, rand.New(rand.NewPCG(seed, 7)), 0) //nolint:gosec // Test play, not security.
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
		wins := 0
		var winTurns []float64
		held := map[float64]int{}
		for seed := uint64(1); seed <= runs; seed++ {
			o := campaign(t, g, seed, c.plan)
			if o.won {
				wins++
				winTurns = append(winTurns, o.turn)
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
		if a, b := campaign(t, g, seed, greedy), campaign(t, g, seed, greedy); a != b {
			t.Fatalf("seed %d: %+v then %+v", seed, a, b)
		}
	}
}

func state(t testing.TB, s *gozen.Session) string {
	t.Helper()
	return fmt.Sprint(query[any](t, s, "var"), query[any](t, s, "gold"), query[any](t, s, "party"), query[any](t, s, "over"), query[any](t, s, "cleared"))
}

// TestSaveRestore saves at the start of turn 5 and restores with another seed: the save
// keeps the random numbers, so the restored campaign ends exactly as the saved one.
func TestSaveRestore(t *testing.T) {
	g := loadGame(t)
	saved := 0
	for seed := uint64(1); seed <= 20; seed++ {
		s := newCampaign(t, g, seed)
		march(t, s, greedy, nil, 5)
		if query[bool](t, s, "over") || query[bool](t, s, "cleared") {
			continue
		}
		saved++
		save, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		r, err := gozen.Restore(g, save, gozen.Config{Seed: seed + 1000, Policy: talk})
		if err != nil {
			t.Fatal(err)
		}
		if a, b := state(t, s), state(t, r); a != b {
			t.Fatalf("seed %d: the restored state differs:\n%s\n%s", seed, a, b)
		}
		march(t, s, greedy, nil, 0)
		march(t, r, greedy, nil, 0)
		if a, b := state(t, s), state(t, r); a != b {
			t.Fatalf("seed %d: the saved and the restored campaign end apart:\n%s\n%s", seed, a, b)
		}
	}
	if saved == 0 {
		t.Fatal("every campaign ended before turn 5")
	}
}

// TestOrderChoices picks Attack from the menu as a player would: it asks for the general,
// who else marches, and the target, and the Policy answers.
func TestOrderChoices(t *testing.T) {
	g := loadGame(t)
	var asked [][]string
	s, err := gozen.New(g, gozen.Config{Seed: 1, Policy: gozen.DefaultPolicy{Prefer: []string{"Hilde", "Brookford"}}, OnRequest: func(q gozen.Request) {
		if q.Kind == "choice" {
			asked = append(asked, q.Options)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Begin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Do(context.Background(), "- pick: Attack\n"); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(asked) != "[[Corin (500 troops) Hilde (600 troops) Cancel] [Corin (500 troops) March now Cancel] [Brookford (free, 350 troops) Crossway (free, 600 troops) Cancel]]" {
		t.Fatalf("asked %v", asked)
	}
	if got := query[string](t, s, "region.ford.owner"); got != "crown" {
		t.Fatalf("Brookford is %q", got)
	}
}

// TestTalkBranches explores the end of turn 2: the talk of turn 3 is the only choice, so
// Explore finds both answers to Sable, and only taking her in makes her a general.
func TestTalkBranches(t *testing.T) {
	g := loadGame(t)
	s := newCampaign(t, g, 1)
	if err := s.Do(context.Background(), "- end_turn: true\n"); err != nil {
		t.Fatal(err)
	}
	paths, err := gozen.Explore(context.Background(), s, "- end_turn: true\n", gozen.ExploreOptions{Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if len(path.Errs) > 0 {
			t.Fatalf("answers %v: %v", path.Answers, path.Errs)
		}
		answer := query[string](t, path.End, "chose.sable")
		if joined := query[bool](t, path.End, "generals.sable.joined"); joined != (answer == "0") {
			t.Errorf("answers %v: chose %q but joined %v", path.Answers, answer, joined)
		}
		seen[answer] = true
	}
	if !seen["0"] || !seen["1"] {
		t.Fatalf("Explore reached the answers %v, want both", seen)
	}
}
