package session_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	gozen "github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

func campaignGame(t *testing.T, extra map[string]string) *gozen.Game {
	t.Helper()
	fsys := fstest.MapFS{"data/main.yaml": {Data: []byte(testgame.Data)}, "assets/x.png": {Data: []byte("x")}}
	for n, s := range testgame.CampaignFiles(extra) {
		fsys[n] = &fstest.MapFile{Data: []byte(s)}
	}
	g, err := gozen.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func do(t *testing.T, s *gozen.Session, script string) {
	t.Helper()
	if err := s.Do(t.Context(), script); err != nil {
		t.Fatalf("%s: %v", script, err)
	}
}

// TestCampaignOrder: an order picked from the menu asks for its general and its target,
// marches, spends the actions and binds them for its do; the next turn lets the other
// faction draft and the general march again.
func TestCampaignOrder(t *testing.T) {
	g := campaignGame(t, nil)
	var asked []gozen.Request
	s := session(t, g, gozen.Config{Seed: 3, OnRequest: func(q gozen.Request) {
		if q.Kind == "choice" {
			asked = append(asked, q)
		}
	}})
	do(t, s, "- pick: Attack\n")
	if len(asked) != 2 || asked[0].Options[0] != "Scout (400 troops)" || asked[1].Options[0] != "Field (free, 100 troops)" || asked[1].Options[len(asked[1].Options)-1] != "Cancel" {
		t.Fatalf("choices: %+v", asked)
	}
	for src, want := range map[string]any{
		"region.field.owner": "blue", "campaign.result": "won", "campaign.actions_left": 0.0,
		"generals.scout.acted": true, "campaign.target": "field", "campaign.general": "scout",
		"region.field.troops + generals.scout.troops < 400": true,
	} {
		if got := query[any](t, s, src); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	// With no actions left the orders leave the menu; the turn ends.
	if err := s.Do(t.Context(), "- pick: Develop\n"); err == nil {
		t.Fatal("Develop was in the menu with no actions left")
	}
	do(t, s, "- pick: End turn\n")
	for src, want := range map[string]any{
		"campaign.turn": 2.0, "campaign.actions_left == 1 + floor(faction.blue.regions / 2)": true,
		"generals.scout.acted": false, "campaign.target": "",
	} {
		if got := query[any](t, s, src); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	// An order with its target written asks nothing and writes through target.
	do(t, s, "- set: {var.campaign_actions: 5}\n")
	do(t, s, "- order: {id: develop, target: home}\n")
	if got := query[float64](t, s, "region.home.wealth"); got != 30 {
		t.Fatalf("develop: wealth %v", got)
	}
	do(t, s, "- order: {id: draft, general: scout}\n")
	if got := query[float64](t, s, "gold"); got != 80 {
		t.Fatalf("draft: gold %v", got)
	}
}

// TestCampaignEnds: losing the capital is a game over, holding every region clears the
// game, and the turn after the last loses.
func TestCampaignEnds(t *testing.T) {
	g := campaignGame(t, nil)
	s := session(t, g, gozen.Config{Seed: 1})
	do(t, s, "- set: {region.home.owner: \"'red'\"}\n")
	if !query[bool](t, s, "over") || query[bool](t, s, "cleared") {
		t.Fatal("losing the capital is not a game over")
	}
	s = session(t, g, gozen.Config{Seed: 1})
	do(t, s, "- set: {region.field.owner: \"'blue'\", region.hill.owner: \"'blue'\", region.keep.owner: \"'blue'\"}\n")
	if !query[bool](t, s, "cleared") || query[bool](t, s, "over") {
		t.Fatal("holding every region does not clear the game")
	}
	s = session(t, g, gozen.Config{Seed: 1, Dev: true})
	do(t, s, "- set: {region.keep.troops: 0}\n")
	for range 6 {
		do(t, s, "- end_turn: true\n")
	}
	if !query[bool](t, s, "over") || query[float64](t, s, "campaign.turn") != 7 {
		t.Fatalf("after the last turn: over %v, turn %v", query[bool](t, s, "over"), query[float64](t, s, "campaign.turn"))
	}
}

// TestCampaignFaction: the faction's rules draft into its regions and march on the
// weakest border it can beat.
func TestCampaignFaction(t *testing.T) {
	g := campaignGame(t, nil)
	s := session(t, g, gozen.Config{Seed: 5})
	do(t, s, "- set: {region.keep.troops: 2000, region.hill.troops: 900, region.field.troops: 50}\n- end_turn: true\n")
	// Keep drafts 50 (2050), then marches 60% (1230) on field, the weakest neighbour.
	if got := query[string](t, s, "region.field.owner"); got != "red" {
		t.Fatalf("field is %q", got)
	}
	if got := query[float64](t, s, "region.keep.troops"); got != 820 {
		t.Fatalf("keep has %v", got)
	}
	if got := query[string](t, s, "region.hill.owner"); got != "" {
		t.Fatalf("hill is %q (one march a turn)", got)
	}
}

// seeded is a random source for test play.
func seeded(a, b uint64) *rand.Rand {
	return rand.New(rand.NewPCG(a, b)) //nolint:gosec // Test play, not security.
}

// randomPolicy plays a campaign at random: any order, general and target.
type randomPolicy struct{ rng *rand.Rand }

func (p *randomPolicy) Decide(_ context.Context, _ *gozen.Session, q gozen.Request) int {
	if q.Kind != "choice" {
		return gozen.Auto
	}
	return p.rng.IntN(len(q.Options))
}

func randomCampaign(t *testing.T, s *gozen.Session, rng *rand.Rand, orders int) {
	t.Helper()
	for range orders {
		if query[bool](t, s, "over") || query[bool](t, s, "cleared") {
			return
		}
		pick := []string{"Attack", "Develop", "Draft", "End turn"}[rng.IntN(4)]
		// An order not in the menu now is an error that changes nothing.
		if err := s.Do(t.Context(), "- pick: "+pick+"\n"); err != nil && !strings.Contains(err.Error(), "pick:") {
			t.Fatal(err)
		}
		for _, r := range []string{"home", "field", "hill", "keep"} {
			if query[float64](t, s, "region."+r+".troops") < 0 {
				t.Fatalf("%s has fewer than 0 troops", r)
			}
		}
		if query[float64](t, s, "generals.scout.troops") < 0 {
			t.Fatal("the scout has fewer than 0 troops")
		}
	}
}

func campaignState(t *testing.T, s *gozen.Session) string {
	t.Helper()
	return fmt.Sprint(query[any](t, s, "var"), query[any](t, s, "gold"), query[any](t, s, "over"), query[any](t, s, "cleared"))
}

// TestCampaignSaveRestore: a save between orders keeps the random numbers, so a restored
// campaign (from another seed) plays on exactly as the one saved; troops never go below 0.
func TestCampaignSaveRestore(t *testing.T) {
	g := campaignGame(t, nil)
	for seed := uint64(1); seed <= 20; seed++ {
		p := &randomPolicy{seeded(seed, 1)}
		s := session(t, g, gozen.Config{Seed: seed, Policy: p})
		randomCampaign(t, s, seeded(seed, 2), 8)
		save, err := s.Save()
		if err != nil {
			t.Fatal(err)
		}
		r, err := gozen.Restore(g, save, gozen.Config{Seed: 999, Policy: &randomPolicy{seeded(seed, 3)}})
		if err != nil {
			t.Fatal(err)
		}
		if a, b := campaignState(t, s), campaignState(t, r); a != b {
			t.Fatalf("seed %d: the restored state differs:\n%s\n%s", seed, a, b)
		}
		p.rng = seeded(seed, 3)
		randomCampaign(t, s, seeded(seed, 4), 40)
		randomCampaign(t, r, seeded(seed, 4), 40)
		if a, b := campaignState(t, s), campaignState(t, r); a != b {
			t.Fatalf("seed %d: the saved and the restored campaign end apart:\n%s\n%s", seed, a, b)
		}
	}
}

// cave is a small dungeon beside the campaign: the game is an RPG and a campaign at once.
const cave = `
locations:
  cave:
    name: Cave
    dungeon:
      wall: x.png
      floor: "#b4c2c6"
      map: |
        #####
        #S..#
        #####
      tiles:
        S: {start: true, facing: east}
`

// TestCampaignWithDungeon: a game with a dungeon and a campaign goes between them with
// move, and they share the state: the gold and the experience won in a battle, the flags,
// and the general's stats, which a march reads as a.atk.
func TestCampaignWithDungeon(t *testing.T) {
	game := testgame.CampaignGame + "  \n"
	game = strings.Replace(game, "  generals: {scout:", "  clash: {power: \"a.troops + a.atk * 100\"}\n  generals: {scout:", 1)
	g := campaignGame(t, map[string]string{"game.yaml": game, "data/cave.yaml": cave})
	s := session(t, g, gozen.Config{Seed: 2, Dev: true})
	before := query[float64](t, s, "actor.scout.atk")
	do(t, s, "- battle: one\n- set: {flag.ghost: true}\n- move: cave\n")
	if query[string](t, s, "location") != "cave" || query[string](t, s, "region.home.owner") != "blue" {
		t.Fatal("the campaign's state is not there in the dungeon")
	}
	gold, exp := query[float64](t, s, "gold"), query[float64](t, s, "actor.scout.exp")
	if gold <= 100 || exp <= 0 {
		t.Fatalf("the battle gave gold %v, exp %v", gold, exp)
	}
	do(t, s, "- set: {actor.scout.level: 5}\n- move: camp\n")
	atk := query[float64](t, s, "actor.scout.atk")
	if atk <= before || !query[bool](t, s, "flag.ghost") || query[float64](t, s, "gold") != gold {
		t.Fatalf("back in the campaign: atk %v (was %v), flag %v, gold %v", atk, before, query[bool](t, s, "flag.ghost"), query[float64](t, s, "gold"))
	}
	// Without the general's attack the march (400) could not take a garrison of 1000
	// (defense 1250); with it (400 + atk * 100) it does.
	do(t, s, "- set: {region.field.troops: 1000}\n- order: {id: attack, general: scout, target: field}\n")
	if query[string](t, s, "region.field.owner") != "blue" {
		t.Fatalf("the march did not read the general's attack %v", atk)
	}
}

// TestCampaignBattle: an order with fight: battle on a region with defend fights the
// game's battle, the general alone against the region's troop; a win takes the region
// with the share of the troops of the HP left, a loss halves the general's troops and the
// region holds. The battle is a request a Policy answers.
func TestCampaignBattle(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, "attack: {label: Attack, general: ready, target: border, march: true}", "attack: {label: Attack, general: ready, target: border, march: true, fight: battle}", 1)
	data := strings.Replace(testgame.CampaignData, "field: {name: Field, next: [home, hill, keep], troops: 100,", "field: {name: Field, next: [home, hill, keep], troops: 100, defend: two,", 1)
	g := campaignGame(t, map[string]string{"game.yaml": game, "data/campaign.yaml": data})
	var battles []gozen.Request
	s := session(t, g, gozen.Config{Seed: 4, OnRequest: func(q gozen.Request) {
		if q.Kind == "battle" {
			battles = append(battles, q)
		}
	}})
	do(t, s, "- order: {id: attack, general: scout, target: field}\n")
	if len(battles) != 1 {
		t.Fatalf("battles: %+v", battles)
	}
	won := query[string](t, s, "campaign.result") == "won"
	if won != (query[string](t, s, "region.field.owner") == "blue") || query[bool](t, s, "over") {
		t.Fatalf("result %v, field %q, over %v", won, query[string](t, s, "region.field.owner"), query[bool](t, s, "over"))
	}
	left := query[float64](t, s, "generals.scout.troops + (region.field.owner == 'blue' ? region.field.troops : 0)")
	if left <= 0 || left > 400 {
		t.Fatalf("troops after the battle: %v", left)
	}
	// A lost battle (answered Lost) is no game over: the general's troops halve.
	s = session(t, g, gozen.Config{Seed: 4, Policy: lose{}})
	do(t, s, "- order: {id: attack, general: scout, target: field}\n")
	if query[bool](t, s, "over") || query[string](t, s, "region.field.owner") != "" || query[float64](t, s, "generals.scout.troops") != 200 {
		t.Fatalf("after a lost battle: over %v, field %q, troops %v", query[bool](t, s, "over"), query[string](t, s, "region.field.owner"), query[float64](t, s, "generals.scout.troops"))
	}
}

// lose answers every battle Lost.
type lose struct{ gozen.DefaultPolicy }

func (lose) Decide(_ context.Context, _ *gozen.Session, q gozen.Request) int {
	if q.Kind == "battle" {
		return gozen.Lost
	}
	return 0
}

// TestCampaignKichikuStyle: a campaign counted by orders (each order per_turn times a
// turn), a march that may be given until a region is taken, generals marching together,
// a beaten general away for some turns, and a calendar of weeks.
func TestCampaignKichikuStyle(t *testing.T) {
	game := strings.NewReplacer(
		"  turns: 6\n", "  turns: 60\n  calendar: week\n  count: orders\n  defeat: {out: 2}\n",
		"  generals: {scout: {troops: 400, might: 2}}", "  generals: {scout: {troops: 400, might: 2}, guide: {troops: 300}}",
		"attack: {label: Attack, general: ready, target: border, march: true}", "attack: {label: Attack, general: ready, target: border, march: true, until_taken: true, generals: 2}",
		"  location: camp\n  player: blue", "  location: camp\n  player: blue\n  clash: {lost: \"0\"}",
	).Replace(testgame.CampaignGame)
	g := campaignGame(t, map[string]string{"game.yaml": game})
	// From the menu: the leader, then who else (Guide), then the target.
	var prompts []string
	m := session(t, g, gozen.Config{Seed: 5, OnRequest: func(q gozen.Request) {
		if q.Kind == "choice" {
			prompts = append(prompts, q.Text)
		}
	}})
	do(t, m, "- join: guide\n- pick: Attack\n")
	if strings.Join(prompts, "|") != "Who leads?|Who else marches?|Where to?" || !query[bool](t, m, "generals.guide.acted") {
		t.Fatalf("asked %v", prompts)
	}
	s := session(t, g, gozen.Config{Seed: 5, Dev: true})
	do(t, s, "- join: guide\n")
	// Two generals march on the field together: their troops add up and are shared after.
	do(t, s, "- order: {id: attack, general: scout, with: [guide], target: field}\n")
	if query[string](t, s, "region.field.owner") != "blue" || !query[bool](t, s, "generals.guide.acted") || query[float64](t, s, "campaign.taken") != 1 {
		t.Fatalf("the joint march: field %q, guide acted %v", query[string](t, s, "region.field.owner"), query[bool](t, s, "generals.guide.acted"))
	}
	// A region was taken: the attack closes for the turn; develop is open once.
	if query[bool](t, s, "orders.attack.open") || !query[bool](t, s, "orders.develop.open") {
		t.Fatal("after a region was taken, the attack is still open (or develop is closed)")
	}
	do(t, s, "- order: {id: develop, target: home}\n")
	if query[bool](t, s, "orders.develop.open") {
		t.Fatal("develop is open twice in a turn")
	}
	do(t, s, "- end_turn: true\n")
	// Until a region is taken, the attack stays open after a march that fails; a general
	// whose troops it brings to 0 is away for two turns.
	do(t, s, "- set: {region.hill.troops: 99999, region.home.troops: 99999, region.keep.troops: 0}\n- order: {id: attack, general: scout, target: hill}\n")
	if !query[bool](t, s, "orders.attack.open") || query[float64](t, s, "generals.scout.out") != 2 || query[bool](t, s, "generals.scout.ready") {
		t.Fatalf("after a lost march: attack open %v, scout out %v", query[bool](t, s, "orders.attack.open"), query[float64](t, s, "generals.scout.out"))
	}
	do(t, s, "- end_turn: true\n- set: {generals.scout.troops: 100}\n- end_turn: true\n")
	if !query[bool](t, s, "generals.scout.ready") {
		t.Fatal("the scout did not come back after two turns")
	}
	do(t, s, "- set: {region.home.troops: 99999, region.field.troops: 99999}\n")
	for range 50 {
		do(t, s, "- end_turn: true\n")
	}
	if w, y := query[float64](t, s, "campaign.week"), query[float64](t, s, "campaign.year"); w != 2 || y != 2 {
		t.Fatalf("turn %v is week %v of year %v", query[float64](t, s, "campaign.turn"), w, y)
	}
}

// TestCampaignClashView: an order with fight: clash_view plays a clash of units by the
// rules (the region's defend troop holding it), writes the troops back, shows itself as a
// request of kind clash and runs on_win, on_lose or on_draw by how it went.
func TestCampaignClashView(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		`attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view, on_win: [{say: "Won."}], on_lose: [{say: "Lost."}], on_draw: [{say: "Drawn."}]}`,
	).Replace(testgame.CampaignGame)
	data := strings.Replace(testgame.CampaignData, "field: {name: Field, next: [home, hill, keep], troops: 100,", "field: {name: Field, next: [home, hill, keep], troops: 100, defend: two,", 1)
	g := campaignGame(t, map[string]string{"game.yaml": game, "data/campaign.yaml": data})
	results := map[string]int{}
	for seed := uint64(1); seed <= 12; seed++ {
		var kinds []string
		var said string
		s := session(t, g, gozen.Config{Seed: seed, OnRequest: func(q gozen.Request) {
			kinds = append(kinds, q.Kind+":"+q.Text)
			if q.Kind == "say" {
				said = q.Text
			}
		}})
		do(t, s, "- set: {region.field.troops: "+strconv.FormatUint(100*seed, 10)+"}\n- order: {id: attack, general: scout, target: field}\n")
		result := query[string](t, s, "campaign.clash")
		results[result]++
		want := map[string]string{"won": "Won.", "lost": "Lost.", "draw": "Drawn."}[result]
		if !slices.Contains(kinds, "clash:"+result) || said != want {
			t.Fatalf("seed %d: requests %v, said %q, want %q", seed, kinds, said, want)
		}
		owner := query[string](t, s, "region.field.owner")
		if (result == "won") != (owner == "blue") || query[float64](t, s, "generals.scout.troops") < 0 || query[float64](t, s, "region.field.troops") < 0 {
			t.Fatalf("seed %d: %s but field is %q", seed, result, owner)
		}
	}
	if results["won"] == 0 || results["won"] == 12 {
		t.Fatalf("results %v: the garrisons from 100 to 1200 should not all end alike", results)
	}
}
