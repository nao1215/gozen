package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

const armiesClash = "generals: {scout: {troops: 400, might: 2, army: men}}\n  clash: {layout: below, armies: {men: [x.png, x.png, x.png]}}"

func armiesGame(t *testing.T, replacement string) string {
	t.Helper()
	game := strings.Replace(testgame.CampaignGame, "generals: {scout: {troops: 400, might: 2}}", replacement, 1)
	if game == testgame.CampaignGame {
		t.Fatal("the generals are not in the game")
	}
	return game
}

// TestClashArmies: the soldier poses of an army, the layout and the kinds are read.
func TestClashArmies(t *testing.T) {
	db, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": armiesGame(t, armiesClash)}))
	if err != nil {
		t.Fatal(err)
	}
	cl := &db.Game.Campaign.Clash
	if db.Game.Campaign.Generals["scout"].Army != "men" || cl.Layout != "below" {
		t.Fatalf("army %q, layout %q", db.Game.Campaign.Generals["scout"].Army, cl.Layout)
	}
}

func TestClashArmyErrors(t *testing.T) {
	for _, c := range []struct{ name, generals, want string }{
		{"a general without an army", "generals: {scout: {troops: 400, might: 2}}\n  clash: {armies: {men: [x.png, x.png]}}", "needs an army (men)"},
		{"an unknown kind", "generals: {scout: {troops: 400, might: 2, army: elves}}\n  clash: {armies: {men: [x.png, x.png]}}", `army "elves" is not in clash.armies`},
		{"a missing picture", "generals: {scout: {troops: 400, might: 2, army: men}}\n  clash: {armies: {men: [x.png, gone.png]}}", "no picture assets/gone.png"},
		{"no pose", "generals: {scout: {troops: 400, might: 2, army: men}}\n  clash: {armies: {men: []}}", "at least one picture"},
		{"a layout", "generals: {scout: {troops: 400, might: 2, army: men}}\n  clash: {layout: over, armies: {men: [x.png, x.png]}}", `layout is front or below, not "over"`},
		{"a kind with no armies", "generals: {scout: {troops: 400, might: 2, army: men}}", `army "men" is not in clash.armies (none are written)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": armiesGame(t, c.generals)}))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}
