package rpg_test

import (
	"fmt"
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func eval(t *testing.T, st *rpg.State, src string) any {
	t.Helper()
	e, err := expr.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Eval(st.Env())
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return v
}

// TestCampaignNames: the campaign's names read the definitions at the start and the
// variables once they are set.
func TestCampaignNames(t *testing.T) {
	db := testgame.LoadCampaign(t, nil)
	st := testgame.State(db, 1)
	for src, want := range map[string]any{
		"region.home.owner":        "blue",
		"region.field.owner":       "",
		"region.keep.troops":       500.0,
		"region.home.wealth":       20.0,
		"region.hill.wealth":       0.0,
		"region.home.name":         "Home",
		"region.keep.capital":      true,
		"region.field.border":      true,
		"region.hill.border":       false,
		"len(region.field.next)":   3.0,
		"region['keep'].troops":    500.0,
		"faction.blue.regions":     1.0,
		"faction.red.troops":       500.0,
		"faction.blue.sum_wealth":  20.0,
		"faction.red.alive":        true,
		"faction.blue.player":      true,
		"faction.red.color":        "#c03030",
		"campaign.turn":            1.0,
		"campaign.actions":         1.0,
		"campaign.actions_left":    1.0,
		"campaign.player":          "blue",
		"campaign.turns":           6.0,
		"generals.scout.troops":    400.0,
		"generals.scout.might":     2.0,
		"generals.scout.joined":    true,
		"generals.scout.ready":     true,
		"next_to('field', 'blue')": true,
		"next_to('hill', 'blue')":  false,
		"target == nil":            true,
		"general == nil":           true,
	} {
		if got := eval(t, st, src); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
}

// TestCampaignSet: set writes a region's owner, troops and values, the bound target and
// general, and a general's troops; a wrong owner or field is an error.
func TestCampaignSet(t *testing.T) {
	db := testgame.LoadCampaign(t, nil)
	st := testgame.State(db, 1)
	must := func(path string, v any) {
		t.Helper()
		if err := st.SetPath(path, v); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	must("region.field.owner", "blue")
	must("region.field.troops", 250.7)
	must("region.field.wealth", 12.5)
	must("var.campaign_target", "hill")
	must("target.troops", -5)
	must("var.campaign_general", "scout")
	must("general.troops", 650)
	for src, want := range map[string]any{
		"faction.blue.regions":    2.0,
		"region.field.troops":     250.0,
		"region.field.wealth":     12.5,
		"faction.blue.sum_wealth": 32.5,
		"target.troops":           0.0,
		"generals.scout.troops":   650.0,
		"next_to('hill', 'blue')": true,
		"region.hill.border":      true,
		"var.region_field_owner":  "blue",
		"general.name":            "Scout",
	} {
		if got := eval(t, st, src); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	for path, v := range map[string]any{
		"region.field.owner": "green", "region.moat.troops": 1, "region.field.gold": 1,
		"generals.ghost.troops": 1, "general.might": 1,
	} {
		if err := st.SetPath(path, v); err == nil {
			t.Errorf("set %s = %v was taken", path, v)
		}
	}
	// A save keeps the campaign's state (it is in the variables).
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	back, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := eval(t, back, "faction.blue.regions"); got != 2.0 {
		t.Fatalf("after a save: %v", got)
	}
}
