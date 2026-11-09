package gamedata_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestBestiaryWarningAndEnemyReferences(t *testing.T) {
	base := map[string]string{
		"game.yaml":      "schema_version: 4\nid: bestiary\nui: {bestiary: true}\n" + testgame.Game,
		"data/main.yaml": testgame.Data,
	}
	db := testgame.Load(t, base)
	if warning := errors.Join(db.SceneWarnings()...); warning == nil || !strings.Contains(warning.Error(), "GZN0115") || !strings.Contains(warning.Error(), "enemies.mote.lore") {
		t.Fatalf("missing lore warning = %v", warning)
	}
	base["data/main.yaml"] = strings.Replace(base["data/main.yaml"], "events:\n", "events:\n  query_enemy:\n    do: [{if: bestiary.missing.seen, then: [{gold: 1}]}]\n", 1)
	if _, err := testgame.TryLoad(base); err == nil || !strings.Contains(err.Error(), `unknown enemy "missing"`) {
		t.Fatalf("unknown bestiary ID error = %v", err)
	}
	base["data/main.yaml"] = strings.Replace(base["data/main.yaml"], "bestiary.missing.seen", "bestiary.mote.seen", 1)
	base["data/main.yaml"] += "\n# lore is optional but suppresses the encounter warning\n"
	// The warning belongs to encounter data; adding lore is accepted by the enemy definition.
	base["data/main.yaml"] = strings.Replace(base["data/main.yaml"], "  mote:\n", "  mote:\n    lore: Lives among old stones.\n", 1)
	base["data/main.yaml"] = strings.Replace(base["data/main.yaml"], "  boss:\n", "  boss:\n    lore: Keeps watch over the camp road.\n", 1)
	db = testgame.Load(t, base)
	if warning := errors.Join(db.SceneWarnings()...); warning != nil {
		t.Fatalf("lore did not suppress the warning: %v", warning)
	}
}
