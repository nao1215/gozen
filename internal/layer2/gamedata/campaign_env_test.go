package gamedata

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
)

// TestHasRegionValue: a region holds a value it declares or one the play has set, and no other.
func TestHasRegionValue(t *testing.T) {
	db := &DB{Regions: map[string]*Region{
		"home":  {Values: map[string]float64{"food": 2, "relics": 0}},
		"field": {Values: map[string]float64{"food": 5}},
	}}
	st := CampaignState{Vars: map[string]any{RegionVar("field", "loot"): 3}}
	cases := []struct {
		id, name string
		want     bool
	}{
		{"home", "relics", true},
		{"field", "relics", false},
		{"field", "food", true},
		{"field", "loot", true},
		{"nowhere", "food", false},
	}
	for _, c := range cases {
		if got := HasRegionValue(db, st, c.id, c.name); got != c.want {
			t.Errorf("HasRegionValue(%q, %q) = %v, want %v", c.id, c.name, got, c.want)
		}
	}
}

func TestCampaignEnvironmentDefaultsAndOverrides(t *testing.T) {
	db := &DB{Regions: map[string]*Region{"home": {Owner: "blue", Troops: 20, Values: map[string]float64{"food": 3}, Next: []string{"field"}}, "field": {Troops: 10, Next: []string{"home"}}}, Factions: map[string]*Faction{"blue": {Capital: "home"}, "red": {Name: "Red"}}}
	st := CampaignState{Vars: map[string]any{}}
	check := func(source string, want any) {
		t.Helper()
		parsed, err := expr.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		got, err := parsed.Eval(CampaignEnv(db, st))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s = %#v, %v; want %#v", source, got, err, want)
		}
	}
	for _, tc := range []struct {
		source string
		want   any
	}{
		{"campaign.turn", 1.0},
		{"campaign.turns", 0.0},
		{"campaign.player", ""},
		{"campaign.week", 0.0},
		{"campaign.year", 0.0},
		{"campaign.actions", 1.0},
		{"campaign.actions_left", 1.0},
		{"general", nil},
		{"target", nil},
		{"generals.missing", nil},
		{"orders.missing", nil},
		{"region.missing", nil},
		{"faction.missing", nil},
		{"faction.blue.name", "blue"},
		{"faction.blue.color", ""},
		{"faction.blue.player", false},
		{"faction.blue.capital", "home"},
		{"faction.blue.sum_food", 3.0},
		{"faction.red.alive", false},
		{"faction.red.troops", 0.0},
		{"next_to('field', 'blue')", true},
		{"next_to('missing', 'blue')", false},
	} {
		check(tc.source, tc.want)
	}
	db.Game.Campaign = &Campaign{Player: "blue", Calendar: "week", Actions: MustExpr("campaign.turn / 2"), Generals: map[string]General{"captain": {Troops: 12}}, Orders: map[string]*Order{"rest": {}}}
	st.JoinedIDs = []string{"captain"}
	st.Vars[VarTurn] = 53.0
	st.Vars[VarGeneral] = "captain"
	st.Vars[VarTarget] = "field"
	st.Vars[RegionVar("home", "owner")] = false // malformed saved overrides fall back to authored defaults
	st.Vars[RegionVar("home", "troops")] = "invalid"
	st.Vars[GeneralVar("captain", "troops")] = "invalid"
	for _, tc := range []struct {
		source string
		want   any
	}{
		{"campaign.week", 1.0},
		{"campaign.year", 2.0},
		{"campaign.actions", 26.0},
		{"region.home.owner", "blue"},
		{"region.home.troops", 20.0},
		{"general.name", "captain"},
		{"general.ready", true},
		{"general.troops", 12.0},
		{"target.id", "field"},
		{"faction.blue.player", true},
	} {
		check(tc.source, tc.want)
	}
	st.Vars[VarActions] = 2.0
	st.Vars[GeneralVar("captain", "out")] = 1.0
	st.Vars[RegionVar("home", "owner")] = "red"
	check("campaign.actions_left", 2.0)
	check("general.ready", false)
	check("faction.red.troops", 20.0)
	check("faction.blue.alive", false)
	for _, source := range []string{"next_to()", "next_to(1, 'blue')", "next_to('home', false)", "faction.blue.unknown", "campaign.unknown"} {
		parsed, err := expr.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parsed.Eval(CampaignEnv(db, st)); err == nil {
			t.Errorf("invalid expression accepted: %s", source)
		}
	}
	if _, ok := CampaignEnv(db, st).Lookup("unknown"); ok {
		t.Fatal("unknown root claimed by campaign environment")
	}
}
