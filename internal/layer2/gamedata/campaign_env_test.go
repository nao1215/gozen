package gamedata

import "testing"

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
