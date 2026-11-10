package campaign_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/campaign"
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
	files := map[string]string{"game.yaml": game, "data/campaign.yaml": fields}
	if strings.Contains(clash, "armies") { // the foes of the field need a kind of army too
		files["data/main.yaml"] = strings.Replace(testgame.Data, "    name: Mote\n", "    name: Mote\n    army: monsters\n", 1)
	}
	return testgame.LoadCampaign(t, files)
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

// TestFactionClash: a faction with generals marches as a clash of units when the
// campaign shows marches so, the strongest of its generals up to clash.units leading.
func TestFactionClash(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view}",
		"generals: {scout: {troops: 400, might: 2}}",
		"generals: {scout: {troops: 400, might: 2}}\n  clash: {units: 1}",
	).Replace(testgame.CampaignGame)
	fields := strings.NewReplacer(
		"capital: keep\n", "capital: keep\n    generals: [mote, mote]\n",
		"field: {name: Field, next: [home, hill, keep], troops: 100,", "field: {name: Field, next: [home, hill, keep], troops: 100, defend: two,",
	).Replace(testgame.CampaignData)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
	if gs := campaign.FactionGenerals(db, "red"); len(gs) != 1 || gs[0] != "mote" {
		t.Fatalf("red marches with %v, want one mote (clash.units 1)", gs)
	}
	if !campaign.UsesClashes(db) {
		t.Fatal("the campaign shows its marches as clashes")
	}
	moved := false
	for seed := uint64(1); seed <= 5; seed++ {
		st := testgame.State(db, seed)
		lines := campaign.FactionTurn(db, st, "red")
		if len(lines) > 0 {
			moved = true
		}
	}
	if !moved {
		t.Fatal("red never marched")
	}
}

// TestFightViewArmies: a unit carries the army kind of its general or its enemy.
func TestFightViewArmies(t *testing.T) {
	db := clashGame(t, `armies: {men: [x.png], monsters: [x.png]}`, ", army: men")
	cl := fight(t, db, 1, nil)
	if got := cl.Sides[0][0].Army; got != "men" {
		t.Errorf("the general's army is %q, want men", got)
	}
	for _, u := range cl.Sides[1] {
		if u.Army != "monsters" {
			t.Errorf("the foe %s has army %q, want monsters", u.Name, u.Army)
		}
	}
}

// TestFightViewEmptyArmy: a general with no troops leads no march (Generals, Check), and
// a clash it is sent into all the same is lost before a strike: 0 troops take no region.
func TestFightViewEmptyArmy(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: any, target: border, march: true, fight: clash_view}",
	).Replace(testgame.CampaignGame)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game})
	st := testgame.State(db, 1)
	st.Vars[data.GeneralVar("scout", "troops")] = 0.0
	st.Vars[data.RegionVar("field", "troops")] = 1.0
	o := campaign.Order(db, "attack")
	if gs := campaign.Generals(db, st, o); len(gs) != 0 {
		t.Fatalf("generals who can march: %v, want none (scout has no troops)", gs)
	}
	if why := campaign.Check(db, st, o, "scout", nil, "field"); why == "" {
		t.Fatal("check let a general with no troops march")
	}
	// the draft is no march: the general without troops can still be given it
	if gs := campaign.Generals(db, st, campaign.Order(db, "draft")); len(gs) != 1 {
		t.Fatalf("generals who can draft: %v, want scout", gs)
	}
	cl := campaign.FightView(db, st, "attack", "scout", nil, "field", nil)
	if cl.Result != "lost" || len(cl.Hits) != 0 || data.RegionOwner(db, st.Campaign(), "field") == "blue" {
		t.Fatalf("an army of 0 troops: result %q, hits %+v, owner now %q", cl.Result, cl.Hits, data.RegionOwner(db, st.Campaign(), "field"))
	}
}

// TestFightViewEmptyFoeUnit: a garrison smaller than its defend troop leaves units of 0
// troops, which are routed from the start and strike nobody.
func TestFightViewEmptyFoeUnit(t *testing.T) {
	db := clashGame(t, `rounds: 1`, "")
	st := testgame.State(db, 1)
	st.Vars[data.RegionVar("field", "troops")] = 1.0 // two motes share 1: 0 and 1
	cl := campaign.FightView(db, st, "attack", "scout", nil, "field", nil)
	if !cl.Sides[1][0].Routed {
		t.Fatalf("the foe unit of 0 troops stands: %+v", cl.Sides[1][0])
	}
	for _, h := range cl.Hits {
		if h.Side == 1 && h.At == 0 || h.Side == 0 && h.To == 0 {
			t.Fatalf("a strike by or at the foe unit of 0 troops: %+v", h)
		}
	}
}

// TestFightViewWonKeepsTroopCount: the generals' troops and the garrison after a region
// is taken add up to the troops left of the clash (the garrison's shares are whole).
func TestFightViewWonKeepsTroopCount(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view, generals: 2}",
		"generals: {scout: {troops: 400, might: 2}}",
		"generals: {scout: {troops: 3, might: 2}, porter: {troops: 2}}",
	).Replace(testgame.CampaignGame)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/porter.yaml": "actors: {porter: {name: Porter, class: ranger}}"})
	won := 0
	for seed := uint64(1); seed <= 5; seed++ {
		st := testgame.State(db, seed)
		st.Vars[data.RegionVar("field", "troops")] = 1.0
		cl := campaign.FightView(db, st, "attack", "scout", []string{"porter"}, "field", nil)
		if cl.Result != "won" {
			continue
		}
		won++
		left := cl.Sides[0][0].Troops + cl.Sides[0][1].Troops
		cs := st.Campaign()
		after := data.GeneralTroops(db, cs, "scout") + data.GeneralTroops(db, cs, "porter") + data.Garrison(db, cs, "field")
		if after != left {
			t.Fatalf("seed %d: %g troops left of the clash, %g after it (generals %g and %g, garrison %g)", seed, left, after,
				data.GeneralTroops(db, cs, "scout"), data.GeneralTroops(db, cs, "porter"), data.Garrison(db, cs, "field"))
		}
	}
	if won == 0 {
		t.Fatal("no seed won: nothing was compared")
	}
}

// TestCheckCapsGeneralsAtClashUnits: clash.units caps the generals of an order shown as
// units, so Check refuses those beyond it instead of leaving them out of the clash.
func TestCheckCapsGeneralsAtClashUnits(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view, generals: 2}",
		"generals: {scout: {troops: 400, might: 2}}",
		"generals: {scout: {troops: 400, might: 2}, porter: {troops: 400}}\n  clash: {units: 1}",
	).Replace(testgame.CampaignGame)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/porter.yaml": "actors: {porter: {name: Porter, class: ranger}}"})
	st := testgame.State(db, 1)
	st.Join("porter")
	o := campaign.Order(db, "attack")
	if why := campaign.Check(db, st, o, "scout", []string{"porter"}, "field"); why == "" {
		t.Fatalf("check let two generals into a clash of %d unit a side", campaign.Cap(db, o))
	}
	if why := campaign.Check(db, st, o, "porter", nil, "field"); why != "" {
		t.Fatalf("check refused one general: %s", why)
	}
}

// TestFactionClashWithNoTroops: a faction whose march would send 0 troops (floor(1 * 0.6))
// does not march: units of 0 troops take no region.
func TestFactionClashWithNoTroops(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view}",
	).Replace(testgame.CampaignGame)
	fields := strings.NewReplacer(
		"capital: keep\n", "capital: keep\n    generals: [mote]\n",
		"      - {draft: \"50\"}\n", "",
		"{march: true, keep: 300, where: \"s.troops * 0.6 > t.troops * 1.25\", pick: min, by: t.troops}", "{march: true}",
	).Replace(testgame.CampaignData)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
	st := testgame.State(db, 1)
	st.Vars[data.RegionVar("keep", "troops")] = 1.0
	st.Vars[data.RegionVar("field", "troops")] = 1.0
	lines := campaign.FactionTurn(db, st, "red")
	cs := st.Campaign()
	if len(lines) != 0 || data.RegionOwner(db, cs, "field") == "red" || data.Garrison(db, cs, "keep") != 1 {
		t.Fatalf("red sending floor(1*0.6)=0 troops: %v (field held by %q, keep %g, field %g)", lines, data.RegionOwner(db, cs, "field"), data.Garrison(db, cs, "keep"), data.Garrison(db, cs, "field"))
	}
}

// TestFactionClashDrawReturnsSurvivors: the troops left of a faction's march fought as a
// clash of units that does not take the region go back to the region they left.
func TestFactionClashDrawReturnsSurvivors(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: clash_view}",
		"generals: {scout: {troops: 400, might: 2}}",
		"generals: {scout: {troops: 400, might: 2}}\n  clash: {rounds: 1, hit: '10'}",
	).Replace(testgame.CampaignGame)
	fields := strings.NewReplacer(
		"capital: keep\n", "capital: keep\n    generals: [mote]\n",
		"      - {draft: \"50\"}\n", "",
		"{march: true, keep: 300, where: \"s.troops * 0.6 > t.troops * 1.25\", pick: min, by: t.troops}", "{march: true, send: 0.5, pick: min, by: t.troops}",
	).Replace(testgame.CampaignData)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
	st := testgame.State(db, 1)
	campaign.FactionTurn(db, st, "red")
	cs := st.Campaign()
	// one round of one strike a side for 10: 250 marched on field (100), 240 come back
	if data.Garrison(db, cs, "keep") != 490 || data.Garrison(db, cs, "field") != 90 || data.RegionOwner(db, cs, "field") == "red" {
		t.Fatalf("after a drawn clash: keep %g, field %g held by %q, want 490 and 90", data.Garrison(db, cs, "keep"), data.Garrison(db, cs, "field"), data.RegionOwner(db, cs, "field"))
	}
}
