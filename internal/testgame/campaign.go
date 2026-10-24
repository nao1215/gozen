package testgame

import (
	"maps"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// CampaignGame is the expedition at schema_version 5 as a campaign of four regions: blue
// (the player, from home) against red (from keep), with the scout as the general.
var CampaignGame = "schema_version: 5\nid: campaign-test\n" + Game + `
campaign:
  location: camp
  player: blue
  turns: 6
  actions: "1 + floor(faction.blue.regions / 2)"
  generals: {scout: {troops: 400, might: 2}}
  orders:
    attack: {label: Attack, general: ready, target: border, march: true}
    develop: {label: Develop, target: own, do: [{set: {target.wealth: "target.wealth + 10"}}]}
    draft: {label: Draft, when: "gold >= 20", general: any, do: [{gold: -20}, {set: {general.troops: "general.troops + 100"}}]}
`

// CampaignData are the regions and the factions of CampaignGame.
const CampaignData = `
factions:
  blue: {name: Blue, color: "#3060c0", capital: home}
  red:
    name: Red
    color: "#c03030"
    capital: keep
    ai:
      - {draft: "50"}
      - {march: true, keep: 300, where: "s.troops * 0.6 > t.troops * 1.25", pick: min, by: t.troops}
regions:
  home: {name: Home, next: [field], owner: blue, troops: 300, values: {wealth: 20}}
  field: {name: Field, next: [home, hill, keep], troops: 100, values: {wealth: 10}}
  hill: {name: Hill, next: [field, keep], troops: 150}
  keep: {name: Keep, next: [field, hill], owner: red, troops: 500, values: {wealth: 15}}
`

// CampaignFiles are the files of the campaign with extra laid over them.
func CampaignFiles(extra map[string]string) map[string]string {
	files := map[string]string{"game.yaml": CampaignGame, "data/campaign.yaml": CampaignData}
	maps.Copy(files, extra)
	return files
}

// LoadCampaign loads the campaign with extra laid over its files.
func LoadCampaign(t testing.TB, extra map[string]string) *data.DB {
	t.Helper()
	return Load(t, CampaignFiles(extra))
}

// TryCampaign loads the campaign with one replacement in its data (old -> new), for the
// checks' tests.
func TryCampaign(old, replacement string) (*data.DB, error) {
	return TryLoad(CampaignFiles(map[string]string{"data/campaign.yaml": strings.Replace(CampaignData, old, replacement, 1)}))
}
