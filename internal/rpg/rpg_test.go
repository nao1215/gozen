package rpg_test

import (
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestActorGrowth(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Members()[0]
	if a.Level != 1 || a.Stat("mhp") != 50 || a.HP != 50 {
		t.Fatalf("level 1: level %d mhp %d hp %d", a.Level, a.Stat("mhp"), a.HP)
	}
	if got := a.Stat("atk"); got != 12+3 {
		t.Errorf("atk with the stick = %d, want 15", got)
	}
	if a.ExpNext() != 40 {
		t.Errorf("exp to level 2 = %d, want 40", a.ExpNext())
	}
	ups := a.GainExp(90) // level 3 needs 90
	if len(ups) != 2 || a.Level != 3 {
		t.Fatalf("level ups = %d, level %d", len(ups), a.Level)
	}
	if len(ups[1].Skills) != 1 || ups[1].Skills[0] != "heal" {
		t.Errorf("skills learned at 3 = %v", ups[1].Skills)
	}
	if a.HP != 70 {
		t.Errorf("hp after levelling = %d, want 70 (raised with the max)", a.HP)
	}
}

func TestEquip(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Members()[0]
	st.AddItem("blade", 1)
	if err := st.Equip(a, "weapon", "blade"); err != nil {
		t.Fatal(err)
	}
	if a.Equip["weapon"] != "blade" || st.Items["stick"] != 1 || st.Items["blade"] != 0 {
		t.Errorf("after equipping: %v bag %v", a.Equip, st.Items)
	}
	st.AddItem("hat", 1)
	if err := st.Equip(a, "armor", "hat"); err == nil {
		t.Error("a ranger equipped a hat (not in the class's kinds)")
	}
	if err := st.Equip(a, "weapon", "potion"); err == nil {
		t.Error("equipped a potion")
	}
	if got := a.StatWith("atk", "weapon", "stick"); got != 15 {
		t.Errorf("atk with the stick instead = %d", got)
	}
	if a.Equip["weapon"] != "blade" {
		t.Error("StatWith changed the equipment")
	}
}

func TestSaveLoad(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Join("guide")
	st.Flags["met"] = true
	if err := st.SetPath("var.count", 3.0); err != nil {
		t.Fatal(err)
	}
	st.Members()[1].HP = 7
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Party) != 2 || got.Members()[1].HP != 7 || !got.Flag("met") || got.Vars["count"] != 3.0 || got.Gold != 100 {
		t.Errorf("loaded %+v", got)
	}
	if got.Members()[0].Stat("atk") != 15 {
		t.Error("the loaded actor lost its data")
	}
}

func TestFieldEnv(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.Members()[0].HP = 10
	for src, want := range map[string]bool{
		"gold == 100":                 true,
		"item.potion == 2":            true,
		"item.stick == 1":             true, // equipped items count
		"party.min_hp_rate < 0.25":    true,
		"party.alive == 1":            true,
		"actor.scout.in_party":        true,
		"actor.guide == nil":          true,
		"flag.anything":               false,
		"var.nothing == 0":            true,
		"location == 'camp'":          true,
		"party.members[0].level == 1": true,
	} {
		e := mustExpr(t, src)
		if got, err := e.Bool(st.Env()); err != nil || got != want {
			t.Errorf("%s = %v (%v), want %v", src, got, err, want)
		}
	}
}

// Starting items that were used up do not come back on load.
func TestLoadKeepsUsedStartItems(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	st.AddItem("potion", -2) // use up the 2 starting potions
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := got.Items["potion"]; n != 0 {
		t.Errorf("使い切った potion がロードで %d 個に戻った", n)
	}
}

// Equipment that vanished from the data or does not fit its slot is removed on load, and EquipBest does not crash.
func TestLoadDropsUnknownEquipment(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Members()[0]
	a.Equip["weapon"] = "gone"
	a.Equip["armor"] = "blade" // does not fit the slot
	a.Equip["nowhere"] = "stick"
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := got.Members()[0]
	if len(h.Equip) != 0 {
		t.Errorf("無効な装備が残った: %v", h.Equip)
	}
	got.AddItem("blade", 1)
	got.EquipBest(h) // must not crash on a nil reference
	if h.Equip["weapon"] != "blade" {
		t.Errorf("EquipBest の後の装備 %v", h.Equip)
	}
}

// Learned skills and states that vanished from the data are dropped on load (so the skill screen never dereferences nil).
func TestLoadDropsUnknownSkillsAndStates(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Members()[0]
	a.Learnt = append(a.Learnt, "gone_skill", "fire")
	a.States["gone_state"] = 3
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := got.Members()[0]
	if slices.Contains(h.Learnt, "gone_skill") || !slices.Contains(h.Learnt, "fire") {
		t.Errorf("習得スキル %v", h.Learnt)
	}
	if _, ok := h.States["gone_state"]; ok {
		t.Errorf("消えた状態が残った: %v", h.States)
	}
	for _, s := range h.Skills() {
		if db.Skills[s] == nil {
			t.Errorf("Skills に未知の %q", s)
		}
	}
}

func TestWearing(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	for src, want := range map[string]any{
		"party.wearing_stick":          1.0, // the scout starts with the stick on
		"party.wearing_blade":          0.0,
		"party.members[0].wears.stick": true,
		"party.members[0].wears.hat":   false,
	} {
		v, err := mustExpr(t, src).Eval(st.Env())
		if err != nil || v != want {
			t.Errorf("%s = %v (%v), want %v", src, v, err, want)
		}
	}
}

// In an old save's skill tree, levels above the cap and levels of vanished nodes are returned to SP.
func TestLoadFitsSkillTree(t *testing.T) {
	db := testgame.Load(t, nil)
	c := db.Classes["ranger"]
	c.Tree = append(c.Tree, gamedata.TreeNode{ID: "cure", Skill: "heal", Max: 5})
	st := testgame.State(db, 1)
	a := st.Members()[0]
	a.SP = 2
	a.Tree["cure"] = 8
	a.Tree["gone"] = 3
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := got.Members()[0]
	if h.Tree["cure"] != 5 || h.SkillLevel("heal") != 5 {
		t.Errorf("上限を超えた Lv: %v", h.Tree)
	}
	if _, ok := h.Tree["gone"]; ok {
		t.Errorf("消えたノードが残った: %v", h.Tree)
	}
	if h.SP != 2+3+3 {
		t.Errorf("SP = %d、want 8", h.SP)
	}
}

// pages('cg/x') is how many CG images there are for that name (x.jpg, x.002.jpg ...). The second-time text is chosen by the image count.
func TestFieldEnvPages(t *testing.T) {
	db := testgame.Load(t, map[string]string{"assets/cg/x.jpg": "x", "assets/cg/x.002.jpg": "x", "assets/cg/x_y.jpg": "x"})
	st := testgame.State(db, 1)
	for src, want := range map[string]bool{
		"pages('cg/x') == 2":       true,
		"pages('cg/x_y') == 1":     true,
		"pages('cg/none') == 0":    true,
		"pages('cg/x.*.jpg') == 1": true,
	} {
		e := mustExpr(t, src)
		if got, err := e.Bool(st.Env()); err != nil || got != want {
			t.Errorf("%s = %v (%v), want %v", src, got, err, want)
		}
	}
}

// Upgrades lists only bag items stronger than the current ones and ignores manual slots. Equipping with EquipUpgrades empties it.
func TestUpgrades(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	a := st.Members()[0]
	st.AddItem("blade", 1)
	up := st.Upgrades(a)
	if up["weapon"] != "blade" {
		t.Fatalf("Upgrades = %v, want blade in weapon", up)
	}
	db.Game.Slots[0].Manual = true // make the weapon slot a manually decided slot
	if _, ok := st.Upgrades(a)["weapon"]; ok {
		t.Errorf("manual の 枠を 勧めた: %v", st.Upgrades(a))
	}
	db.Game.Slots[0].Manual = false
	if n := st.EquipUpgrades(a); n != 1 || a.Equip["weapon"] != "blade" {
		t.Errorf("EquipUpgrades = %d, equip %v", n, a.Equip)
	}
	if len(st.Upgrades(a)) != 0 {
		t.Errorf("着けた 後も 勧める: %v", st.Upgrades(a))
	}
}
