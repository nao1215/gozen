package campaign_test

import (
	"math"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/campaign"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestMarchDistributesLargeFiniteTroops(t *testing.T) {
	game := strings.NewReplacer(
		"generals: {scout: {troops: 400, might: 2}}", "generals: {scout: {troops: 400, might: 2}, porter: {troops: 400}}\n  clash: {power: '2', defense: '1', won: 'a.troops / 2', garrison: 0}",
		"target: border, march: true", "target: border, march: true, generals: 2",
	).Replace(testgame.CampaignGame)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/porter.yaml": "actors: {porter: {name: Porter, class: ranger}}"})
	st := testgame.State(db, 1)
	for _, id := range []string{"scout", "porter"} {
		st.Vars[data.GeneralVar(id, "troops")] = 1e200
	}
	if _, err := st.Save(); err != nil {
		t.Fatal(err)
	}
	campaign.Give(db, st, "attack", "scout", []string{"porter"}, "field")
	for _, id := range []string{"scout", "porter"} {
		got := data.GeneralTroops(db, st.Campaign(), id)
		if math.IsInf(got, 0) || math.IsNaN(got) || math.Abs(got/5e199-1) > 1e-12 {
			t.Errorf("%s troops = %g, want 5e199", id, got)
		}
	}
	if _, err := st.Save(); err != nil {
		t.Fatalf("a march made a valid state unsaveable: %v", err)
	}
	// Even when the sum of individually finite armies overflows, a fixed
	// authored survival count is divided in proportion and remains saveable.
	for _, id := range []string{"scout", "porter"} {
		st.Vars[data.GeneralVar(id, "troops")] = 1e308
	}
	db.Game.Campaign.Clash.Won = data.MustExpr("1000")
	campaign.Give(db, st, "attack", "scout", []string{"porter"}, "field")
	for _, id := range []string{"scout", "porter"} {
		if got := data.GeneralTroops(db, st.Campaign(), id); got != 500 {
			t.Errorf("%s troops = %g, want 500", id, got)
		}
	}
	if _, err := st.Save(); err != nil {
		t.Fatal(err)
	}
}

// TestFactionMarchHeldReturnsSurvivors: the troops a faction's march has left when the
// region holds go back to the region they left, as a general's go back to the general.
func TestFactionMarchHeldReturnsSurvivors(t *testing.T) {
	fields := strings.NewReplacer(
		"      - {draft: \"50\"}\n", "",
		"{march: true, keep: 300, where: \"s.troops * 0.6 > t.troops * 1.25\", pick: min, by: t.troops}", "{march: true, send: 0.5}",
	).Replace(testgame.CampaignData)
	game := strings.Replace(testgame.CampaignGame, "generals: {scout: {troops: 400, might: 2}}", "generals: {scout: {troops: 400, might: 2}}\n  clash: {power: '1', defense: '2', lost: 'a.troops * 0.5', held: 'b.troops'}", 1)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
	st := testgame.State(db, 1)
	campaign.FactionTurn(db, st, "red")
	cs := st.Campaign()
	// keep 500 sends 250, the clash's lost says 125 are left of them
	if got := data.Garrison(db, cs, "keep"); got != 375 {
		t.Fatalf("keep has %g after a held march of 250 with 125 left, want 375 (field %g, hill %g)", got, data.Garrison(db, cs, "field"), data.Garrison(db, cs, "hill"))
	}
}

// TestAfterBattleLostReadsActor: a march lost in a battle (fight: battle) works the
// clash's lost out with the general's level and stats, as a march by numbers (Give) does.
func TestAfterBattleLostReadsActor(t *testing.T) {
	game := strings.NewReplacer(
		"attack: {label: Attack, general: ready, target: border, march: true}",
		"attack: {label: Attack, general: ready, target: border, march: true, fight: battle}",
		"generals: {scout: {troops: 400, might: 2}}",
		"generals: {scout: {troops: 400, might: 2}}\n  clash: {lost: 'a.level * 100'}",
	).Replace(testgame.CampaignGame)
	fields := strings.Replace(testgame.CampaignData, "field: {name: Field, next: [home, hill, keep], troops: 100,", "field: {name: Field, next: [home, hill, keep], troops: 100, defend: two,", 1)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
	st := testgame.State(db, 1)
	if campaign.Fight(db, st, "attack", "scout", "field") == nil {
		t.Fatal("no battle")
	}
	campaign.AfterBattle(db, st, false)
	want := float64(st.Actors["scout"].Level) * 100
	if got := data.GeneralTroops(db, st.Campaign(), "scout"); got != want {
		t.Fatalf("scout (level %d) has %g troops after a lost battle with lost: a.level * 100, want %g", st.Actors["scout"].Level, got, want)
	}
}

// TestFactionFollowsEveryRuleThatHolds: a faction follows its rules in order, each that
// holds (ADR 0030), not only the first: red drafts 50 in keep, then marches from it.
func TestFactionFollowsEveryRuleThatHolds(t *testing.T) {
	db := testgame.LoadCampaign(t, nil)
	st := testgame.State(db, 1)
	lines := campaign.FactionTurn(db, st, "red")
	cs := st.Campaign()
	// keep 500 drafts to 550 and sends floor(550 * 0.6) = 330 on field
	if len(lines) != 1 || data.RegionOwner(db, cs, "field") != "red" || data.Garrison(db, cs, "keep") != 220 {
		t.Fatalf("red: %v, field held by %q, keep %g, want one march taking field and keep 220", lines, data.RegionOwner(db, cs, "field"), data.Garrison(db, cs, "keep"))
	}
}

// TestFactionMarchOfNoTroops: a region whose share to send is floor(garrison*send) = 0
// troops marches on nobody: no line, no region marked as fought.
func TestFactionMarchOfNoTroops(t *testing.T) {
	fields := strings.NewReplacer(
		"      - {draft: \"50\"}\n", "",
		"{march: true, keep: 300, where: \"s.troops * 0.6 > t.troops * 1.25\", pick: min, by: t.troops}", "{march: true}",
	).Replace(testgame.CampaignData)
	db := testgame.LoadCampaign(t, map[string]string{"data/campaign.yaml": fields})
	st := testgame.State(db, 1)
	st.Vars[data.RegionVar("keep", "troops")] = 1.0
	lines := campaign.FactionTurn(db, st, "red")
	if len(lines) != 0 || data.Fought(st.Campaign(), "field") != 0 {
		t.Fatalf("a march of floor(1*0.6)=0 troops was fought: %v, field fought on turn %g", lines, data.Fought(st.Campaign(), "field"))
	}
}

// TestAfterBattleLostNotFinite: a lost formula that overflows or is not a number leaves
// the general a count of troops a save can hold, after a battle as in a clash by numbers.
func TestAfterBattleLostNotFinite(t *testing.T) {
	for _, lost := range []string{"pow(10, 308) * 10", "pow(10, 308) * 10 - pow(10, 308) * 10"} {
		game := strings.NewReplacer(
			"attack: {label: Attack, general: ready, target: border, march: true}",
			"attack: {label: Attack, general: ready, target: border, march: true, fight: battle}",
			"generals: {scout: {troops: 400, might: 2}}",
			"generals: {scout: {troops: 400, might: 2}}\n  clash: {lost: '"+lost+"'}",
		).Replace(testgame.CampaignGame)
		fields := strings.Replace(testgame.CampaignData, "field: {name: Field, next: [home, hill, keep], troops: 100,", "field: {name: Field, next: [home, hill, keep], troops: 100, defend: two,", 1)
		db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/campaign.yaml": fields})
		st := testgame.State(db, 1)
		campaign.Fight(db, st, "attack", "scout", "field")
		campaign.AfterBattle(db, st, false)
		got := data.GeneralTroops(db, st.Campaign(), "scout")
		_, err := st.Save()
		if math.IsNaN(got) || math.IsInf(got, 0) || err != nil {
			t.Errorf("lost: %s left scout %g troops; save: %v", lost, got, err)
		}
	}
}

// TestFactionDraftNotFinite: a draft that overflows or is not a number drafts nobody: the
// garrison stays what it was and the state can be saved.
func TestFactionDraftNotFinite(t *testing.T) {
	for _, draft := range []string{"pow(10, 308) * 10", "pow(10, 308) * 10 - pow(10, 308) * 10"} {
		fields := strings.NewReplacer(
			"- {draft: \"50\"}", "- {draft: \""+draft+"\"}",
			"{march: true, keep: 300,", "{march: true, keep: 9999,",
		).Replace(testgame.CampaignData)
		db := testgame.LoadCampaign(t, map[string]string{"data/campaign.yaml": fields})
		st := testgame.State(db, 1)
		campaign.FactionTurn(db, st, "red")
		got := data.Garrison(db, st.Campaign(), "keep")
		if _, err := st.Save(); got != 500 || err != nil {
			t.Errorf("draft: %s left keep %g troops, want 500; save: %v", draft, got, err)
		}
	}
}

// TestEstimateNotFinite: a power or defense that is not finite makes no march look good,
// as a formula that fails does.
func TestEstimateNotFinite(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, "generals: {scout: {troops: 400, might: 2}}", "generals: {scout: {troops: 400, might: 2}}\n  clash: {power: 'pow(10, 308) * 10'}", 1)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game})
	if power, defense := campaign.Estimate(db, testgame.State(db, 1), "scout", "field"); power != 0 || defense != 1 {
		t.Fatalf("estimate %g against %g, want 0 against 1", power, defense)
	}
}
