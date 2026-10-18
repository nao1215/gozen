package data_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestBattleHookReferencesAndMeterLimit(t *testing.T) {
	for _, hook := range []string{"before", "after", "after_win"} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + "  " + hook + ": absent\n"})
		if err == nil || !strings.Contains(err.Error(), "battle."+hook) {
			t.Fatalf("%s: %v", hook, err)
		}
	}
	_, err := testgame.TryLoad(map[string]string{"game.yaml": testgame.Game + "\nmeters: [{label: Energy, value: 'var.energy', max_value: 'unknown.limit'}]\n"})
	if err == nil || !strings.Contains(err.Error(), "max_value") {
		t.Fatalf("invalid limit accepted: %v", err)
	}
	db := testgame.Load(t, map[string]string{"game.yaml": testgame.Game + "\nmeters: [{label: Energy, value: 'var.energy', max_value: 'var.capacity'}]\n"})
	if !db.Game.Meters[0].MaxValue.Set() {
		t.Fatal("dynamic limit lost during loading")
	}
}
