package campaign_test

import (
	"math"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
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
