package rpg_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const treeData = `
classes:
  adept:
    name: Adept
    stats: {mhp: "30 + level * 5", mmp: "10", atk: "5", def: "5", mat: "5", mdf: "5", agi: "5", luk: "5"}
    exp_for: "level * 10"
    max_level: 5
    sp: 2
    start_sp: 3
    tree:
      - {id: spark, skill: fire, max: 2}
      - {id: toughness, stats: {mhp: 10}, max: 3}
      - {id: surge, skill: heal, requires: [{node: spark, level: 2}, {node: toughness}], max: 1}
actors:
  monk: {name: Monk, class: adept}
  veteran: {name: Veteran, class: adept, level: 3, tree: {toughness: 1}}
`

func treeState(t *testing.T) (*rpg.State, *rpg.Actor) {
	t.Helper()
	db := testgame.Load(t, map[string]string{"data/tree.yaml": treeData})
	st := testgame.State(db, 1)
	st.Join("monk")
	return st, st.Actors["monk"]
}

// TestRaisingTheSkillTree: a point is spent on an open node, a node that needs others stays shut until they are raised,
// a full node and an empty purse refuse, and giving the points back empties the tree.
func TestRaisingTheSkillTree(t *testing.T) {
	_, monk := treeState(t)
	if monk.SP != 3 {
		t.Fatalf("starting SP %d", monk.SP)
	}
	if monk.Node("nothing") != nil || monk.NodeOpen("nothing") || monk.NodeReached("nothing") || monk.Raise("nothing") {
		t.Error("a node that does not exist is neither open nor raised")
	}
	if monk.NodeOpen("surge") || monk.Raise("surge") || monk.NodeReached("surge") {
		t.Error("surge needs spark 2 and toughness")
	}
	hp := monk.Stat("mhp")
	monk.HP = hp
	if !monk.Raise("toughness") || monk.SP != 2 || monk.Stat("mhp") != hp+10 || monk.HP != hp+10 {
		t.Errorf("toughness: sp %d mhp %d hp %d", monk.SP, monk.Stat("mhp"), monk.HP)
	}
	if !monk.Raise("spark") || !monk.Raise("spark") {
		t.Fatal("spark twice")
	}
	if monk.Raise("spark") {
		t.Error("spark is at its top")
	}
	if monk.SP != 0 || monk.NodeOpen("surge") {
		t.Errorf("no points left, so nothing is open: sp %d", monk.SP)
	}
	if !monk.NodeReached("surge") {
		t.Error("the requirements of surge are met")
	}
	skills := strings.Join(monk.Skills(), ",")
	if !strings.Contains(skills, "fire") {
		t.Errorf("a raised skill node teaches its skill: %s", skills)
	}
	if monk.SkillLevel("fire") != 2 || monk.SkillLevel("nothing") != 0 {
		t.Errorf("skill levels %d %d", monk.SkillLevel("fire"), monk.SkillLevel("nothing"))
	}
	monk.ResetTree()
	if len(monk.Tree) != 0 || monk.SP != 3 {
		t.Errorf("tree %v sp %d: all the points come back", monk.Tree, monk.SP)
	}
	if monk.HP > monk.Stat("mhp") {
		t.Errorf("HP %d over the maximum %d after the tree went", monk.HP, monk.Stat("mhp"))
	}
}

func TestStartingTreeFitsTheLevel(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tree.yaml": treeData})
	st := testgame.State(db, 1)
	st.Join("veteran")
	v := st.Actors["veteran"]
	if v.Level != 3 || v.Tree["toughness"] != 1 {
		t.Fatalf("level %d tree %v", v.Level, v.Tree)
	}
	// SP: start 3 plus 2 for each level after the first, minus 1 spent
	if v.SP != 3+2*2-1 {
		t.Errorf("SP %d, want %d", v.SP, 3+2*2-1)
	}
}

func TestExperienceStopsAtTheClassMaximum(t *testing.T) {
	_, monk := treeState(t)
	monk.GainExp(1_000_000)
	if monk.Level != 5 {
		t.Errorf("level %d, want the class maximum 5", monk.Level)
	}
	if monk.ExpNext() != 0 {
		t.Errorf("at the maximum nothing more is needed: %d", monk.ExpNext())
	}
	before := monk.Level
	if ups := monk.GainExp(100); len(ups) != 0 || monk.Level != before {
		t.Errorf("no more level ups: %v", ups)
	}
}

func TestResistanceOfStatesAndGear(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/res.yaml": `
items:
  ward: {name: Ward, kind: hat, slot: armor, state_resist: {sleep: 0.5}}
`})
	st := testgame.State(db, 1)
	scout := st.Actors["scout"]
	if scout.StateResist("sleep") != 1 {
		t.Errorf("no gear, no resistance: %v", scout.StateResist("sleep"))
	}
	scout.Equip["armor"] = "ward"
	if got := scout.StateResist("sleep"); got != 0.5 {
		t.Errorf("a ward halves it: %v", got)
	}
	if got := scout.StateResist("poison"); got != 1 {
		t.Errorf("other states are unaffected: %v", got)
	}
}

func TestEquipmentRefusalsAndRemoval(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	scout := st.Actors["scout"]
	st.AddItem("blade", 1)
	if err := st.Equip(scout, "weapon", "blade"); err != nil {
		t.Fatal(err)
	}
	if st.Items["stick"] != 1 {
		t.Error("the old weapon goes back in the bag")
	}
	if err := st.Equip(scout, "weapon", ""); err != nil {
		t.Fatalf("taking a weapon off: %v", err)
	}
	if _, ok := scout.Equip["weapon"]; ok || st.Items["blade"] != 1 {
		t.Errorf("equip %v bag %v", scout.Equip, st.Items)
	}
	if err := st.Equip(scout, "weapon", "blade"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ slot, item string }{{"weapon", "ghost"}, {"nowhere", "blade"}, {"armor", "blade"}, {"weapon", "potion"}, {"armor", "hat"}} {
		if err := st.Equip(scout, c.slot, c.item); err == nil {
			t.Errorf("equip %s into %s was accepted", c.item, c.slot)
		}
	}
	if scout.Equip["weapon"] != "blade" {
		t.Errorf("a refused equip changed the gear: %v", scout.Equip)
	}
	delete(st.Items, "stick")
	if err := st.Equip(scout, "weapon", "stick"); err == nil {
		t.Error("equipping what the bag does not hold")
	}
}

func TestTacticsFormatIsChecked(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	scout := st.Actors["scout"]
	if got := scout.TacticsDraft(); got.Mode != rpg.TacticsAuto || got.Version != rpg.TacticsVersion {
		t.Errorf("a fresh draft is auto: %+v", got)
	}
	for name, tac := range map[string]rpg.ActorTactics{
		"another version":            {Version: 9, Mode: rpg.TacticsAuto},
		"an unknown mode":            {Version: rpg.TacticsVersion, Mode: "wild"},
		"auto with rows":             {Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto, Rows: []data.TacticRow{{Enabled: true, Condition: "any", Action: "attack"}}},
		"a preset mode with no name": {Version: rpg.TacticsVersion, Mode: rpg.TacticsPreset},
		"a preset name with auto":    {Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto, Preset: "x"},
	} {
		if err := scout.SetTactics(tac); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// a draft is a copy: editing it does not change the committed tactics
	manual := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "any", Action: "attack"}}}
	if err := scout.SetTactics(manual); err != nil {
		t.Fatal(err)
	}
	draft := scout.TacticsDraft()
	draft.Rows[0].Enabled = false
	if !scout.Tactics.Rows[0].Enabled {
		t.Error("the draft shares rows with the committed tactics")
	}
	manual.Rows[0].Action = "guard"
	if scout.Tactics.Rows[0].Action != "attack" {
		t.Error("SetTactics keeps a copy")
	}
	if rows := scout.TacticRows(rpg.ActorTactics{Mode: rpg.TacticsManual, Rows: manual.Rows}); len(rows) != 1 || rows[0].Action != "guard" {
		t.Errorf("manual rows are the rows given: %+v", rows)
	}
	// rows whose ids this game does not know are kept, and compiling says why they cannot run
	compiled := scout.CompileTactics()
	if len(compiled) != 1 || !compiled[0].Enabled || len(compiled[0].Errors) == 0 {
		t.Errorf("compiled %+v", compiled)
	}
}
