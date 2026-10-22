package gamedata_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestLineageShortestChoiceFamilyAndReferences(t *testing.T) {
	files := testgame.MonsterFiles()
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "  boss:\n", "  boss:\n    recruit: {chance: 0.2, level: 1}\n", 1)
	db := testgame.Load(t, files)
	lin := data.BuildLineage(db)
	root := lin.Tree("boss", nil)
	if root == nil || root.Recipe.Kind != data.RecipeCapture || root.Choices != 2 {
		t.Fatalf("shortest recipe = %+v", root)
	}
	root = lin.Tree("boss", map[string]int{"boss": 1})
	if root.Recipe.Kind != data.RecipePair || len(root.Parents) != 2 || root.Parents[0].Species != "mote" || root.Parents[1].Ref != "seen" {
		t.Fatalf("selected pair and repeated parent = %+v", root)
	}
	files = testgame.MonsterFiles()
	files["data/monsters.yaml"] = strings.Replace(files["data/monsters.yaml"], "  pairs: [{parents: [mote, mote], child: boss}]", "  pairs: []", 1)
	files["data/monsters.yaml"] = strings.Replace(files["data/monsters.yaml"], "children: [mote, boss]", "children: [boss]", 1)
	files["data/main.yaml"] = strings.Replace(files["data/main.yaml"], "    rank: 2", "    rank: 1", 1)
	db = testgame.Load(t, files)
	root = data.BuildLineage(db).Tree("boss", nil)
	if root == nil || root.Recipe.Kind != data.RecipeFamily || root.Recipe.Families != [2]string{"slime", "slime"} || len(root.Parents) != 2 {
		t.Fatalf("family recipe has no concrete parents: %+v", root)
	}
}
