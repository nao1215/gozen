package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestMergeKeyOverride: a key written beside a YAML merge (<<: *base) overrides the merged
// one, as YAML means, in data/ and in game.yaml; a key written twice is still an error.
func TestMergeKeyOverride(t *testing.T) {
	for _, src := range []string{
		"items:\n  base: &b {name: A, price: 1}\n  copy: {<<: *b, price: 2}\n",
		"items:\n  base: &b {name: A, price: 1}\n  copy:\n    price: 2\n    <<: *b\n",
	} {
		db, err := testgame.TryLoad(map[string]string{"data/z.yaml": src})
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if it := db.Items["copy"]; it == nil || it.Price != 2 || it.Name != "A" {
			t.Fatalf("%q: copy %+v", src, it)
		}
	}
	game := strings.Replace(testgame.Game, "start: {location: camp,", "start: {<<: {location: field, gold: 5}, location: camp,", 1)
	if game == testgame.Game {
		t.Fatal("the game.yaml case does not change the game")
	}
	db, err := testgame.TryLoad(map[string]string{"game.yaml": game})
	if err != nil {
		t.Fatalf("game.yaml: %v", err)
	}
	if db.Game.Start.Location != "camp" || db.Game.Start.Gold != 100 {
		t.Fatalf("game.yaml start %+v", db.Game.Start)
	}
	if _, err := testgame.TryLoad(map[string]string{"data/z.yaml": "items:\n  copy: {name: A, price: 1, price: 2}\n"}); err == nil || !strings.Contains(err.Error(), "price") {
		t.Fatalf("a key written twice: %v", err)
	}
}
