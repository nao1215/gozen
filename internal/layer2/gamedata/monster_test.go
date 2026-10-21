package gamedata_test

import (
	"slices"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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
