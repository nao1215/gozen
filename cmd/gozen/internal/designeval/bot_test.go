package designeval

import (
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

// A troop that guards several floors is counted on the same floor in every run of
// the same configuration: the shallowest.
func TestBossOfPicksTheShallowestFloorEveryRun(t *testing.T) {
	floors := map[int]bool{}
	for range 200 {
		b := &bot{
			db:     &data.DB{Troops: map[string]*data.Troop{}},
			c:      &Config{Game: Game{Bosses: map[int]string{5: "guardian", 10: "guardian", 15: "guardian"}}},
			bosses: map[int]*Boss{},
		}
		floors[b.bossOf("guardian").Floor] = true
	}
	if len(floors) != 1 || !floors[5] {
		t.Fatalf("the same configuration counted the troop's fight on %d different floors: %v", len(floors), floors)
	}
}
