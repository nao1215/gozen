package gamedata_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestSiteSceneExample loads the maps and painted scenes example of the site's guide
// (website/content/writing-a-game.md) as written: the places, pictures and troop it names
// but does not define are stubs.
func TestSiteSceneExample(t *testing.T) {
	b, err := os.ReadFile("../../../website/content/writing-a-game.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	at := strings.Index(doc, "### Maps and painted scenes")
	if at < 0 {
		t.Fatal("no section on maps and painted scenes")
	}
	body := doc[at:]
	start := strings.Index(body, "```yaml\n") + len("```yaml\n")
	end := strings.Index(body[start:], "```")
	example := body[start : start+end]
	files := map[string]string{
		"game.yaml":      "schema_version: 3\nid: example\n" + strings.Replace(testgame.Game, "location: camp", "location: plaza", 1),
		"data/town.yaml": example + "  shop: {name: Shop, exits: [{to: plaza}]}\n  gate: {name: Gate, exits: [{to: town}]}\ntroops:\n  wolves: {members: [mote]}\n",
	}
	for _, p := range regexp.MustCompile(`[\w/]+\.(?:jpg|png)`).FindAllString(example, -1) {
		files["assets/"+p] = "a picture"
	}
	db, err := testgame.TryLoad(files)
	if err != nil {
		t.Fatalf("the site's example does not load:\n%v", err)
	}
	if db.Locations["plaza"].Stage == nil || db.Locations["town"].Map == nil {
		t.Fatal("the example lost its stage or map")
	}
}
