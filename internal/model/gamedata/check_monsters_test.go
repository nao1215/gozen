package gamedata_test

import (
	"slices"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestMonsterRulesAndReachability(t *testing.T) {
	files := testgame.MonsterFiles()
	db := testgame.Load(t, files)
	if got := db.BreedResult("mote", "mote", 2, 2, 0, 0); got != "boss" {
		t.Fatalf("specific pair = %q", got)
	}
	if got := db.BreedResult("mote", "mote", 1, 2, 0, 0); got != "" {
		t.Fatalf("underlevel pair = %q", got)
	}
	if p := db.BreedReach()["boss"]; p.Steps != 1 || p.Parents != [2]string{"mote", "mote"} {
		t.Fatalf("reach = %+v", p)
	}
	bad := testgame.MonsterFiles()
	bad["data/monsters.yaml"] += "  pairs: [{parents: [mote, missing], child: boss}]\n"
	if _, err := testgame.TryLoad(bad); err == nil {
		t.Fatal("duplicate YAML key was accepted")
	}
	bad = testgame.MonsterFiles()
	bad["data/monsters.yaml"] = strings.Replace(bad["data/monsters.yaml"], "child: boss", "child: missing", 1)
	if _, err := testgame.TryLoad(bad); err == nil || !strings.Contains(err.Error(), "GZN1112") {
		t.Fatalf("unknown child = %v", err)
	}
	bad = testgame.MonsterFiles()
	bad["data/monsters.yaml"] = strings.Replace(bad["data/monsters.yaml"], "  pairs: [{parents: [mote, mote], child: boss}]", "  pairs: []", 1)
	db = testgame.Load(t, bad)
	if _, ok := db.BreedReach()["boss"]; ok {
		t.Fatal("a higher ranked child was marked reachable from low-rank parents")
	}
	withPlus := testgame.MonsterFiles()
	withPlus["data/monsters.yaml"] = strings.Replace(withPlus["data/monsters.yaml"], "child: boss}", "child: boss, min_plus: 10}", 1)
	db = testgame.Load(t, withPlus)
	if p := db.BreedReach()["boss"]; p.Steps != 2 {
		t.Fatalf("plus-gated reach = %+v; want two generations", p)
	}
	noRecipes := testgame.MonsterFiles()
	noRecipes["data/monsters.yaml"] = "monster_families: {slime: Slime}\n"
	db = testgame.Load(t, noRecipes)
	if warnings := db.SceneWarnings(); !slices.ContainsFunc(warnings, func(w error) bool { return strings.Contains(w.Error(), "GZN0116") }) {
		t.Fatalf("missing unreachable warning without breeding: %v", warnings)
	}
}

func TestMonsterCommandsParseInSchemaSix(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	for _, script := range []string{"- reserve: {in: monster_2, out: monster_1}\n", "- release: monster_2\n", "- breed: {parents: [monster_1, monster_2], skills: [fire]}\n"} {
		if ds := data.CheckScript(db, script, false); len(ds) != 0 {
			t.Fatalf("%s: %v", script, ds)
		}
	}
}

func TestMonsterSexRulesAndReachability(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	db.Enemies["mote"].Sex = "female"
	if _, ok := db.BreedReach()["boss"]; ok {
		t.Fatal("female-only parents yielded a child")
	}
	files := testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "  mote:\n    family: slime", "  mote:\n    family: slime\n    sex: female", 1)
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "  boss:\n    family: slime", "  boss:\n    family: slime\n    sex: female", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "pair cannot breed") {
		t.Fatalf("female-only pair = %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/monsters.yaml"] = strings.Replace(files["data/monsters.yaml"], "  pairs: [{parents: [mote, mote], child: boss}]\n", "  pairs: []\n", 1)
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "  mote:\n    family: slime", "  mote:\n    family: slime\n    sex: female", 1)
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "  boss:\n    family: slime", "  boss:\n    family: slime\n    sex: female", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "family rule cannot breed") {
		t.Fatalf("female-only family = %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "family: slime", "family: slime\n    sex: any\n    sex_ratio: 0", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "pair cannot breed") {
		t.Fatalf("zero male ratio = %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "family: slime", "family: slime\n    sex: other", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "sex must be") {
		t.Fatalf("invalid sex = %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "family: slime", "family: slime\n    sex: any\n    sex_ratio: 1.5", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "sex_ratio") {
		t.Fatalf("invalid ratio = %v", err)
	}
}

func TestMonsterGrowthAndCaptureSourceDiagnostics(t *testing.T) {
	files := testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "recruit: {chance: 1, level: 1", "recruit: {chance: 1, level: 100", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "GZN1113") {
		t.Fatalf("recruit above class cap: %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "exp_for: \"level * level * 10\"", "exp_for: \"10\"", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "GZN1113") {
		t.Fatalf("flat monster experience curve: %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "  ranger:\n", "  ranger:\n    max_level: 2\n", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "skill \"heal\" is learned at level 3") {
		t.Fatalf("skill above class cap: %v", err)
	}
	files = testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "troops: [{troop: one}]", "troops: [{troop: boss}]", 1)
	db := testgame.Load(t, files)
	if warnings := db.SceneWarnings(); !slices.ContainsFunc(warnings, func(w error) bool { return strings.Contains(w.Error(), "GZN0117") }) {
		t.Fatalf("recruit without an encounter: %v", warnings)
	}
}

func TestMonsterDefinitionBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, file, old, replacement, want string }{
		{"missing families", "data/monsters.yaml", "monster_families: {slime: Slime}", "monster_families: {}", "define monster_families"},
		{"party too large", "game.yaml", "id: monster-test", "id: monster-test\nmax_party: 5", "at most four"},
		{"unknown family", "data/main.yaml", "family: slime", "family: absent", "unknown or missing monster family"},
		{"zero rank", "data/main.yaml", "rank: 1", "rank: 0", "rank must be positive"},
		{"missing class", "data/main.yaml", "monster_class: ranger", "monster_class: absent", "unknown or missing monster class"},
		{"unknown admission", "data/main.yaml", "recruit: {", "recruit: {admission: unknown, ", "admission must be"},
		{"zero chance", "data/main.yaml", "chance: 1", "chance: 0", "chance must be"},
		{"excess chance", "data/main.yaml", "chance: 1", "chance: 1.01", "chance must be"},
		{"nonfinite chance", "data/main.yaml", "chance: 1", "chance: .nan", "chance"},
		{"missing recruit class", "data/main.yaml", "recruit: {", "recruit: {class: absent, ", "unknown monster class"},
		{"negative recruit level", "data/main.yaml", "chance: 1, level: 1", "chance: 1, level: -1", "level cannot be negative"},
		{"negative recruit bonus", "data/main.yaml", "bonus: 0", "bonus: -1", "bonus cannot be negative"},
		{"missing bonus item", "data/main.yaml", "bonus_item: potion", "bonus_item: absent", "unknown item"},
		{"missing bonus skill", "data/main.yaml", "recruit: {", "recruit: {bonus_skill: absent, ", "unknown skill"},
		{"one parent", "data/monsters.yaml", "parents: [mote, mote]", "parents: [mote]", "parents needs two enemy IDs"},
		{"self recipe", "data/monsters.yaml", "child: boss", "child: mote", "parent-to-self"},
		{"negative recipe level", "data/monsters.yaml", "child: boss", "child: boss, min_level: -1", "min_level and min_plus"},
		{"negative recipe plus", "data/monsters.yaml", "child: boss", "child: boss, min_plus: -1", "min_level and min_plus"},
		{"one family", "data/monsters.yaml", "parents: [slime, slime]", "parents: [slime]", "parents needs two family IDs"},
		{"unknown parent family", "data/monsters.yaml", "parents: [slime, slime]", "parents: [slime, absent]", "unknown family"},
		{"empty children", "data/monsters.yaml", "children: [mote, boss]", "children: []", "children cannot be empty"},
		{"unknown family child", "data/monsters.yaml", "children: [mote, boss]", "children: [absent]", "unknown monster"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := testgame.MonsterFiles()
			if !strings.Contains(files[tc.file], tc.old) {
				t.Fatal("fixture replacement missing")
			}
			files[tc.file] = strings.Replace(files[tc.file], tc.old, tc.replacement, 1)
			if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

// Checking the breeding rules leaves the parents in the order the game writes them: gozen
// book and the lineage read that order, and a check does not change what it checks.
func TestBreedingCheckKeepsTheWrittenParentOrder(t *testing.T) {
	files := testgame.MonsterFiles()
	files["data/monsters.yaml"] = "monster_families: {slime: Slime, beast: Beast}\nbreeding:\n  min_level: 2\n  skill_slots: 1\n" +
		"  pairs: [{parents: [mote, boss], child: cub}]\n  families: [{parents: [slime, beast], children: [mote, boss]}]\n"
	files["data/cub.yaml"] = "enemies:\n  cub:\n    family: beast\n    rank: 1\n    monster_class: ranger\n    name: Cub\n    image: x.png\n    stats: {mhp: 5, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}\n"
	db := testgame.Load(t, files)
	if got := db.Breeding.Pairs[0].Parents; !slices.Equal(got, []string{"mote", "boss"}) {
		t.Errorf("pair parents = %v, want the written [mote boss]", got)
	}
	if got := db.Breeding.Families[0].Parents; !slices.Equal(got, []string{"slime", "beast"}) {
		t.Errorf("family parents = %v, want the written [slime beast]", got)
	}
	files["data/monsters.yaml"] = strings.Replace(files["data/monsters.yaml"], "child: cub}]", "child: cub}, {parents: [boss, mote], child: cub}]", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "is also defined at") {
		t.Errorf("the same pair written in the other order = %v, want a duplicate", err)
	}
}
