package campaign_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/testgame"
)

// clashGame is the test campaign with attack shown as units, the field held by the troop
// two, and extra written under campaign.clash.
func clashGame(t *testing.T, clash, general string) *data.DB {
	t.Helper()
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view, generals: 4}",
		"generals: {scout: {troops: 400, might: 2}}",
		"generals: {scout: {troops: 400, might: 2"+general+"}}\n  clash: {"+clash+"}",
	).Replace(testgame.CampaignGame)
	fields := strings.Replace(testgame.CampaignData, "field: {name: Field, next: [home, hill, keep], troops: 100,", "field: {name: Field, next: [home, hill, keep], troops: 101, defend: two,", 1)
	return testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
}

func fight(t *testing.T, db *data.DB, seed uint64, tactics map[string]string) *campaign.Clash {
	t.Helper()
	st := testgame.State(db, seed)
	cl := campaign.FightView(db, st, "attack", "scout", nil, "field", tactics)
	if cl == nil {
		t.Fatal("no clash")
	}
	return cl
}

// firstTargets are the foes the player's unit struck before any foe was routed.
func firstTargets(cl *campaign.Clash) []int {
	var out []int
	for _, h := range cl.Hits {
		if h.Side == 0 {
			out = append(out, h.To)
		}
		if h.Routed {
			break
		}
	}
	return out
}

// TestFightViewTactics: a tactic picks the foe a unit strikes (the fewest or the most
// troops of the two motes, 50 and 51), the order's tactic over the general's own.
func TestFightViewTactics(t *testing.T) {
	db := clashGame(t, `tactics: {weak: {label: Weak, pick: least, by: t.troops}, strong: {label: Strong, pick: most, by: t.troops}}`, ", tactic: strong")
	for seed := uint64(1); seed <= 5; seed++ {
		for tactic, want := range map[string]int{"weak": 0, "strong": 1, "": 1} {
			var given map[string]string
			if tactic != "" {
				given = map[string]string{"scout": tactic}
			}
			for _, to := range firstTargets(fight(t, db, seed, given)) {
				if to != want {
					t.Fatalf("seed %d, tactic %q: struck %d, want %d", seed, tactic, to, want)
				}
			}
		}
	}
}

// TestFightViewSkill: a skill charged after its rounds is used instead of a strike, takes
// its power and gives its heal back.
func TestFightViewSkill(t *testing.T) {
	db := clashGame(t, `rounds: 3, hit: "1", skills: {blow: {label: Blow, charge: 2, power: "30", heal: "5"}}`, ", skills: [blow]")
	cl := fight(t, db, 1, nil)
	var used []campaign.Hit
	for _, h := range cl.Hits {
		if h.Side == 0 && h.Skill != "" {
			used = append(used, h)
		}
	}
	if len(used) != 1 || used[0].Round != 2 || used[0].Skill != "blow" || used[0].Loss != 30 {
		t.Fatalf("skills used %+v, want blow once in round 2 taking 30", used)
	}
	if used[0].Heal < 1 || used[0].Heal > 5 {
		t.Fatalf("heal %d, want up to 5 (the troops lost by then)", used[0].Heal)
	}
}

// TestFightViewDuel: clash.units 1 makes a clash of one unit a side.
func TestFightViewDuel(t *testing.T) {
	db := clashGame(t, `units: 1`, "")
	cl := fight(t, db, 1, nil)
	if len(cl.Sides[0]) != 1 || len(cl.Sides[1]) != 1 || cl.Front != 1 {
		t.Fatalf("sides %d and %d, front %d, want 1, 1, 1", len(cl.Sides[0]), len(cl.Sides[1]), cl.Front)
	}
	if cl.Sides[1][0].Troops+lost(cl, 1) != 101 {
		t.Fatalf("the one foe holds %g of 101", cl.Sides[1][0].Max)
	}
}

func lost(cl *campaign.Clash, side int) float64 {
	n := 0.0
	for _, h := range cl.Hits {
		if h.Of == side {
			n += float64(h.Loss)
		}
	}
	return n
}
