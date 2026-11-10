package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestBreedingMinPlusIsBounded: BreedReach, which every load runs, tries every plus
// value up to the largest requirement, so a requirement above MaxBreedingPlus is
// refused instead of keeping the loader busy.
func TestBreedingMinPlusIsBounded(t *testing.T) {
	files := testgame.MonsterFiles()
	files["data/monsters.yaml"] = strings.Replace(files["data/monsters.yaml"], "child: boss}", "child: boss, min_plus: 2000000000}", 1)
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "min_plus: want at most 999") {
		t.Fatalf("min_plus: 2000000000 = %v, want the bound reported", err)
	}
}
