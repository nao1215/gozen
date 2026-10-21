package battle_test

import (
	"maps"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

const menuData = `
items:
  salve: {name: Salve, price: 3, use: {scope: one_ally, where: [menu], effects: [{heal: "10"}]}}
  battle_only: {name: Smoke, price: 3, use: {scope: one_ally, where: [battle], effects: [{heal: "10"}]}}
  keepsake: {name: Keepsake, kind: key}
  flare: {name: Flare, price: 3, use: {scope: one_enemy, where: [menu], effects: [{damage: "5"}]}}
  reviver: {name: Reviver, price: 3, use: {scope: dead_ally, where: [menu], effects: [{revive: "b.mhp"}]}}
  wand: {name: Wand, price: 3, use: {scope: one_ally, where: [menu], effects: [{remove_state: poison}]}}
skills:
  mend_all: {name: Mend All, mp: 2, scope: all_allies, where: [menu], effects: [{heal: "10"}]}
  rise_all: {name: Rise All, mp: 2, scope: all_dead_allies, where: [menu], effects: [{revive: "b.mhp"}]}
  blessing: {name: Blessing, mp: 1, scope: self, where: [menu], effects: [{add_state: sleep}]}
  weak_flare: {name: Weak Flare, scope: one_enemy, where: [menu], effects: [{damage: "5"}]}
  costly: {name: Costly, mp: 9999, scope: self, where: [menu], effects: [{heal: "1"}]}
`

// TestMenuUseRefusals: outside battle only skills and items marked for the menu, on allies, with enough MP and stock, and only if
// they would do something, can be used; a refusal leaves the MP and the stock as they were.
func TestMenuUseRefusals(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/menu.yaml": menuData})
	st := testgame.State(db, 1)
	st.Join("guide")
	scout, guide := st.Actors["scout"], st.Actors["guide"]
	for _, id := range []string{"salve", "battle_only", "keepsake", "flare", "reviver", "wand"} {
		st.AddItem(id, 1)
	}
	scout.HP, guide.HP = 5, 5
	mp := scout.MP
	check := func(label string, skill, item string, target int) {
		t.Helper()
		before := maps.Clone(st.Items)
		var to = scout
		if target == 1 {
			to = guide
		}
		if target < 0 {
			to = nil
		}
		if msgs, err := battle.UseInMenu(db, st, scout, skill, item, to); err == nil {
			t.Errorf("%s: expected a refusal, got %v", label, msgs)
		}
		if scout.MP != mp || !maps.Equal(before, st.Items) {
			t.Errorf("%s: a refusal changed the MP or the stock", label)
		}
	}
	check("an item for battle only", "", "battle_only", 0)
	check("a key", "", "keepsake", 0)
	check("an item against an enemy", "", "flare", 0)
	check("an unknown item", "", "nothing", 0)
	check("a skill against an enemy", "weak_flare", "", 0)
	check("an unknown skill", "nothing", "", 0)
	check("a skill that costs too much", "costly", "", 0)
	check("a revive item on someone standing", "", "reviver", 0)
	check("a skill on the dead with no one fallen", "rise_all", "", 0)
	check("an item with no target", "", "salve", -1)
	st.Items["salve"] = 0
	check("an item that is not held", "", "salve", 0)
	st.Items["salve"] = 1

	// someone who has fallen cannot be given a one-ally item
	guide.HP = 0
	check("a one-ally item on the fallen", "", "salve", 1)
}

func TestMenuUseSpendsOnlyWhenItWorks(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/menu.yaml": menuData})
	st := testgame.State(db, 1)
	st.Join("guide")
	scout, guide := st.Actors["scout"], st.Actors["guide"]
	st.AddItem("salve", 2)
	st.AddItem("wand", 1)
	scout.HP = 10
	msgs, err := battle.UseInMenu(db, st, scout, "", "salve", scout)
	if err != nil || len(msgs) == 0 || scout.HP != 20 || st.Items["salve"] != 1 {
		t.Fatalf("a salve heals and is used up: %v %v hp %d left %d", msgs, err, scout.HP, st.Items["salve"])
	}
	// removing a state nobody has: allowed, but says nothing happened, and the item is used
	msgs, err = battle.UseInMenu(db, st, scout, "", "wand", scout)
	if err != nil || len(msgs) != 1 || st.Items["wand"] != 0 {
		t.Fatalf("a wand with nothing to remove: %v %v", msgs, err)
	}
	// a skill for everyone costs MP once
	scout.HP, guide.HP, scout.MP = 1, 1, 5
	msgs, err = battle.UseInMenu(db, st, scout, "mend_all", "", nil)
	if err != nil || scout.HP != 11 || guide.HP != 11 || scout.MP != 3 || len(msgs) != 2 {
		t.Fatalf("a party-wide skill: %v %v hp %d %d mp %d", msgs, err, scout.HP, guide.HP, scout.MP)
	}
	// reviving the fallen
	guide.HP = 0
	if _, err := battle.UseInMenu(db, st, scout, "rise_all", "", nil); err != nil || guide.HP != guide.Stat("mhp") || scout.MP != 1 {
		t.Fatalf("revive all: %v hp %d mp %d", err, guide.HP, scout.MP)
	}
	// a state put on the user stays with its turns
	if _, err := battle.UseInMenu(db, st, scout, "blessing", "", nil); err != nil {
		t.Fatal(err)
	}
	if scout.States["sleep"] <= 0 || scout.MP != 0 {
		t.Errorf("states %v mp %d", scout.States, scout.MP)
	}
	// no user at all falls back to the first member
	st.AddItem("salve", 1)
	scout.HP = 1
	if _, err := battle.UseInMenu(db, st, nil, "", "salve", scout); err != nil || scout.HP != 11 {
		t.Errorf("a nil user is the first member: %v hp %d", err, scout.HP)
	}
}

func TestMenuQueries(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/menu.yaml": menuData})
	st := testgame.State(db, 1)
	st.Join("guide")
	guide := st.Actors["guide"]
	guide.HP = 0
	if !battle.Usable(db, "", "salve") || battle.Usable(db, "", "battle_only") || battle.Usable(db, "", "keepsake") || battle.Usable(db, "", "nothing") {
		t.Error("Usable follows where: [menu] on items")
	}
	if !battle.Usable(db, "mend_all", "") || battle.Usable(db, "attack", "") || battle.Usable(db, "nothing", "") {
		t.Error("Usable follows where: [menu] on skills")
	}
	if !battle.NeedsTarget(db, "", "salve") || !battle.NeedsTarget(db, "", "reviver") || battle.NeedsTarget(db, "mend_all", "") || battle.NeedsTarget(db, "blessing", "") {
		t.Error("only one_ally and dead_ally ask for a member")
	}
	if battle.NeedsTarget(db, "", "keepsake") {
		t.Error("an item with no use asks for no one")
	}
	cases := []struct {
		skill, item string
		who         string
		want        bool
	}{
		{"", "salve", "scout", true},
		{"", "salve", "guide", false},
		{"", "reviver", "guide", true},
		{"", "reviver", "scout", false},
		{"mend_all", "", "guide", true},
		{"", "keepsake", "scout", false},
	}
	for _, c := range cases {
		if got := battle.CanTarget(db, c.skill, c.item, st.Actors[c.who]); got != c.want {
			t.Errorf("CanTarget(%s%s, %s) = %v, want %v", c.skill, c.item, c.who, got, c.want)
		}
	}
	if battle.CanTarget(db, "", "salve", nil) {
		t.Error("no member is no target")
	}
}
