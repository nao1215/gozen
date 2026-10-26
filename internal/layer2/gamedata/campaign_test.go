package gamedata_test

import (
	"fmt"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestCampaignLoads: the campaign fixture loads without errors or warnings, and its state
// names are generated.
func TestCampaignLoads(t *testing.T) {
	db := testgame.LoadCampaign(t, nil)
	if w := db.SceneWarnings(); len(w) > 0 {
		t.Fatalf("warnings: %v", w)
	}
	names := db.StateRegistry()
	for _, n := range []string{"var.campaign_turn", "var.campaign_actions", "var.campaign_target", "var.region_home_owner", "var.region_keep_troops", "var.region_hill_wealth", "var.general_scout_troops"} {
		if _, ok := names[n]; !ok {
			t.Errorf("no generated name %s", n)
		}
	}
	if len(db.Regions) != 4 || len(db.Factions) != 2 || db.Game.Campaign == nil {
		t.Fatalf("regions %d, factions %d, campaign %v", len(db.Regions), len(db.Factions), db.Game.Campaign)
	}
	if got := strings.Join(db.Game.Campaign.OrderIDs(), ","); got != "attack,develop,draft" {
		t.Fatalf("orders in the order written: %s", got)
	}
}

// TestCampaignErrors: gozen check reports a campaign's mistakes with where they are.
func TestCampaignErrors(t *testing.T) {
	for _, c := range []struct{ name, old, replacement, want string }{
		{"a border written on one side", "field: {name: Field, next: [home, hill, keep]", "field: {name: Field, next: [hill, keep]", "home borders field, but field's next does not name home"},
		{"an unknown neighbour", "keep: {name: Keep, next: [field, hill]", "keep: {name: Keep, next: [field, hill, nowhere]", `unknown region "nowhere"`},
		{"overlapping areas", "troops: 150}\n  keep: {name: Keep, next: [field, hill], owner: red, troops: 500, values: {wealth: 15}}", "troops: 150, area: [[0, 0], [10, 0], [10, 10], [0, 10]]}\n  keep: {name: Keep, next: [field, hill], owner: red, troops: 500, values: {wealth: 15}, area: [[5, 5], [15, 5], [15, 15]]}", "the area overlaps hill's"},
		{"a landmark too wide", "hill: {name: Hill, next: [field, keep], troops: 150}", "hill: {name: Hill, next: [field, keep], troops: 150, landmark_scale: 2}", "landmark_scale is a share"},
		{"a landmark grounded past 1", "hill: {name: Hill, next: [field, keep], troops: 150}", "hill: {name: Hill, next: [field, keep], troops: 150, landmark_ground: 1.5}", "landmark_ground is 0 to 1"},
		{"an area of two points", "hill: {name: Hill, next: [field, keep], troops: 150}", "hill: {name: Hill, next: [field, keep], troops: 150, area: [[0, 0], [10, 0]]}", "a polygon needs three points"},
		{"a region bordering itself", "hill: {name: Hill, next: [field, keep]", "hill: {name: Hill, next: [field, keep, hill]", "does not border itself"},
		{"an unknown owner", "owner: red, troops: 500", "owner: rde, troops: 500", `unknown faction "rde"`},
		{"a capital held by another", "capital: keep", "capital: hill", "capital hill is held by"},
		{"an unknown capital", "capital: keep", "capital: moat", `unknown region "moat"`},
		{"pick min without by", "pick: min, by: t.troops", "pick: min", "pick: min needs by"},
		{"a rule that drafts and marches", `{draft: "50"}`, `{draft: "50", march: true}`, "one of them"},
		{"an unknown pick", "pick: min, by: t.troops", "pick: least, by: t.troops", `pick is first, min, max or random, not "least"`},
		{"a value named like a field", "values: {wealth: 15}", "values: {owner: 15}", "a value's name"},
		{"a value named like the record of a fight", "values: {wealth: 15}", "values: {fought: 15}", "a value's name"},
		{"two regions making one saved name", "hill: {name: Hill, next: [field, keep], troops: 150}", "hill: {name: Hill, next: [field, keep], troops: 150, values: {top_wealth: 1}}\n  hill_top: {name: Hilltop, troops: 10, values: {wealth: 1}}", `var.region_hill_top_wealth is made by both`},
		{"an unknown defending troop", "hill: {name: Hill, next: [field, keep], troops: 150}", "hill: {name: Hill, next: [field, keep], troops: 150, defend: ghosts}", `unknown troop "ghosts"`},
		{"an unknown name in a rule", `"s.troops * 0.6 > t.troops * 1.25"`, `"s.troops * 0.6 > u.troops"`, `unknown name "u"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := testgame.TryCampaign(c.old, c.replacement)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
			if !strings.Contains(err.Error(), "campaign.yaml") {
				t.Fatalf("no file and line: %v", err)
			}
		})
	}
}

// TestCampaignGameErrors: the mistakes of game.yaml's campaign and of the names.
func TestCampaignGameErrors(t *testing.T) {
	for _, c := range []struct{ name, old, replacement, want string }{
		{"an unknown player", "player: blue", "player: bleu", `unknown faction "bleu"`},
		{"an unknown place", "location: camp", "location: tent", `unknown location "tent"`},
		{"an unknown general", "generals: {scout: ", "generals: {ghost: ", `unknown actor "ghost"`},
		{"an unknown target word", "target: own", "target: mine", `target is border, own or any, not "mine"`},
		{"an unknown fight", "general: ready, target: border, march: true}", "general: ready, target: border, march: true, fight: duel}", `fight is battle or clash_view, not "duel"`},
		{"a fight without a march", "general: ready, target: border, march: true}", "general: ready, target: border, fight: battle}", "fight: battle decides a march"},
		{"a march without a target", "general: ready, target: border, march: true", "general: ready, march: true", "a march needs a general and a target"},
		{"an unknown region in an expression", `when: "gold >= 20"`, `when: "region.moat.troops > 0"`, `unknown region "moat"`},
		{"an unknown field of a region", `when: "gold >= 20"`, `when: "region.home.gold > 0"`, `a region has no field "gold"`},
		{"an unknown value in a sum", `when: "gold >= 20"`, `when: "faction.blue.sum_gold > 0"`, `no region has the value "gold"`},
		{"a set of an unknown faction", `target.wealth: "target.wealth + 10"`, `target.owner: "'bleu'"`, `unknown faction "bleu"`},
		{"a set of an unknown field", `target.wealth: "target.wealth + 10"`, `target.gold: "1"`, `set can change a region's owner, troops and values`},
		{"actions reading the party", `actions: "1 + floor(faction.blue.regions / 2)"`, `actions: "members"`, `unknown name "members"`},
		{"units above four", "generals: {scout: {troops: 400, might: 2}}", "generals: {scout: {troops: 400, might: 2}}\n  clash: {units: 5}", "units are 1 to 4"},
		{"an unknown tactic of a general", "might: 2}}", "might: 2, tactic: rush}}", `unknown tactic "rush"`},
		{"an unknown skill of a general", "might: 2}}", "might: 2, skills: [blow]}}", `unknown skill "blow"`},
		{"a tactic's unknown pick", "generals: {scout: {troops: 400, might: 2}}", "generals: {scout: {troops: 400, might: 2}}\n  clash: {tactics: {rush: {label: Rush, pick: first}}}", `pick is least, most or random, not "first"`},
		{"a tactic reading an unknown name", "generals: {scout: {troops: 400, might: 2}}", "generals: {scout: {troops: 400, might: 2}}\n  clash: {tactics: {rush: {label: Rush, pick: least, by: b.troops}}}", `unknown name "b"`},
		{"the campaign at version 4", "schema_version: 5", "schema_version: 4", "campaign needs schema_version: 5"},
	} {
		t.Run(c.name, func(t *testing.T) {
			game := strings.Replace(testgame.CampaignGame, c.old, c.replacement, 1)
			if game == testgame.CampaignGame {
				t.Fatalf("%q is not in the game", c.old)
			}
			_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": game}))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

// TestCampaignWarnings: regions in two parts, a faction that borders no one, and a win
// that holds at the start are warnings, not errors.
func TestCampaignWarnings(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"an island", map[string]string{"data/island.yaml": "regions: {isle: {name: Isle, owner: red, troops: 10}}"}, "cannot be reached from home"},
		{"a win at the start", map[string]string{"game.yaml": strings.Replace(testgame.CampaignGame, "turns: 6", "turns: 6\n  win: \"faction.blue.regions >= 1\"", 1)}, "campaign.win holds at the start"},
		{"a lose at the start", map[string]string{"game.yaml": strings.Replace(testgame.CampaignGame, "turns: 6", "turns: 6\n  lose: \"region.keep.owner == 'red'\"", 1)}, "campaign.lose holds at the start"},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := testgame.LoadCampaign(t, c.files)
			if w := fmt.Sprint(db.SceneWarnings()); !strings.Contains(w, c.want) {
				t.Fatalf("want %q, got %s", c.want, w)
			}
		})
	}
}

// TestCampaignCommands: order names an order, a general and a region the campaign has;
// end_turn needs a campaign; the orders and the end of the turn are in the place's menu.
func TestCampaignCommands(t *testing.T) {
	for _, c := range []struct{ do, want string }{
		{"[{order: siege}]", `unknown order "siege"`},
		{"[{order: {id: attack, general: ghost}}]", `"ghost" is not a general`},
		{"[{order: {id: attack, target: moat}}]", `unknown region "moat"`},
	} {
		_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"data/x.yaml": "events: {x: {do: " + c.do + "}}"}))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.do, c.want, err)
		}
	}
	_, err := testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 5\nid: plain\n" + testgame.Game, "data/x.yaml": "events: {x: {do: [{end_turn: true}]}}"})
	if err == nil || !strings.Contains(err.Error(), "end_turn needs game.yaml's campaign") {
		t.Errorf("end_turn without a campaign: %v", err)
	}
	db := testgame.LoadCampaign(t, nil)
	var labels []string
	for _, a := range db.Locations["camp"].Actions {
		labels = append(labels, a.Label)
	}
	if got := strings.Join(labels, ","); !strings.HasPrefix(got, "Attack,Develop,Draft,") || !strings.HasSuffix(got, ",End turn") {
		t.Fatalf("the menu: %s", got)
	}
}

// TestCampaignMapWarnings: the campaign's map and the regions are compared: a region
// without its point, a road between regions that do not border, a border without a road.
func TestCampaignMapWarnings(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, "  location: camp\n", "  location: war\n", 1)
	war := `
locations:
  war:
    name: War
    map:
      picture: x.png
      points:
        home: {at: [1, 1]}
        field: {at: [2, 2]}
        keep: {at: [3, 3]}
      roads:
        - {between: [home, field]}
        - {between: [home, keep]}
`
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game, "data/war.yaml": war})
	w := fmt.Sprint(db.SceneWarnings())
	for _, want := range []string{"region hill has no point", "a road joins home and keep", "field borders keep, but the campaign's map has no road"} {
		if !strings.Contains(w, want) {
			t.Errorf("no warning %q in %s", want, w)
		}
	}
}

// TestCampaignHeadingNames: the expressions of campaign.heading are checked like any text's.
func TestCampaignHeadingNames(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, "campaign:\n", "campaign:\n  heading: \"Turn {campaign.trun}\"\n", 1)
	_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": game}))
	if err == nil || !strings.Contains(err.Error(), "campaign.heading") {
		t.Fatalf("a misspelt name in the heading: %v", err)
	}
}

// TestCampaignSortieLook: the sortie's background must exist.
func TestCampaignSortieLook(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, "campaign:\n", "campaign:\n  sortie: {background: gone.png}\n", 1)
	_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": game}))
	if err == nil || !strings.Contains(err.Error(), "campaign.sortie.background") {
		t.Fatalf("a missing sortie background: %v", err)
	}
}

// TestCampaignOrderView: order_view takes list or icons, and the icons must exist.
func TestCampaignOrderView(t *testing.T) {
	for _, c := range []struct{ add, want string }{
		{"  order_view: tiles\n", "want list or icons"},
		{"  end_turn_button: {icon: gone.png}\n", "campaign.end_turn_button.icon"},
	} {
		game := strings.Replace(testgame.CampaignGame, "campaign:\n", "campaign:\n"+c.add, 1)
		_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": game}))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", strings.TrimSpace(c.add), c.want, err)
		}
	}
}

// TestCampaignActionsReadTurn: campaign.actions may read campaign.turn (only the actions
// themselves are out of its reach), at the check and on a play.
func TestCampaignActionsReadTurn(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, `actions: "1 + floor(faction.blue.regions / 2)"`, `actions: "campaign.turn + 1"`, 1)
	db := testgame.LoadCampaign(t, map[string]string{"game.yaml": game})
	st := data.CampaignState{JoinedIDs: db.Game.Start.Party, Vars: map[string]any{data.VarTurn: 3.0}}
	if n := data.TurnActions(db, st); n != 4 {
		t.Fatalf("turn 3 has %g actions, want 4", n)
	}
	for _, self := range []string{"campaign.actions + 1", "campaign.actions_left"} {
		game := strings.Replace(testgame.CampaignGame, `actions: "1 + floor(faction.blue.regions / 2)"`, `actions: "`+self+`"`, 1)
		_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": game}))
		if err == nil || !strings.Contains(err.Error(), "campaign.actions") {
			t.Errorf("actions %q: %v", self, err)
		}
	}
}

// TestCampaignMapViews: region_roads and region_borders take selected or always.
func TestCampaignMapViews(t *testing.T) {
	game := strings.Replace(testgame.CampaignGame, "  location: camp\n", "  location: war\n", 1)
	for _, c := range []struct{ view, want string }{{"region_roads: always\n      region_borders: selected", ""}, {"region_borders: sometimes", "want selected or always"}} {
		war := "\nlocations:\n  war:\n    name: War\n    map:\n      picture: x.png\n      " + c.view + "\n      points:\n        home: {at: [1, 1]}\n"
		_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"game.yaml": game, "data/war.yaml": war}))
		if c.want == "" && err != nil && strings.Contains(err.Error(), "region_") {
			t.Errorf("%s: %v", c.view, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: want %q, got %v", c.view, c.want, err)
		}
	}
}

// TestFactionWordsAndCallLoops: a misspelt faction compared with an owner or given to
// next_to is an error, and so is an event that calls itself on every run; a call under an
// if is not.
func TestFactionWordsAndCallLoops(t *testing.T) {
	for _, c := range []struct{ events, want string }{
		{`{x: {trigger: auto, when: "region.home.owner == 'bleu'", do: [{gold: 1}]}}`, `unknown faction "bleu"`},
		{`{x: {do: [{if: "target.owner != 'rde'", then: [{gold: 1}]}]}}`, `unknown faction "rde"`},
		{`{x: {do: [{if: "next_to('field', 'grn')", then: [{gold: 1}]}]}}`, `unknown faction "grn"`},
		{`{x: {do: [{if: "next_to('moat', 'red')", then: [{gold: 1}]}]}}`, `unknown region "moat"`},
		{`{x: {do: [{gold: 1}, {call: y}]}, y: {do: [{call: x}]}}`, "calls itself on every run"},
	} {
		_, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"data/x.yaml": "events: " + c.events}))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.events, c.want, err)
		}
	}
	for _, ok := range []string{
		`{x: {trigger: auto, when: "region.home.owner == 'red' || region.field.owner == ''", do: [{gold: 1}]}}`,
		`{x: {do: [{if: "gold < 9", then: [{gold: 1}, {call: x}]}]}}`,
	} {
		if _, err := testgame.TryLoad(testgame.CampaignFiles(map[string]string{"data/x.yaml": "events: " + ok})); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
}
