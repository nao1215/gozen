package play

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestResumeOnTheMap: a save in a place with resume_to: a map comes back on the map's point
// leading to that place (ADR 0022), not on a spot of the old place.
func TestResumeOnTheMap(t *testing.T) {
	db := testgame.LoadScenes(t, map[string]string{"data/town.yaml": strings.Replace(testgame.ScenesData, "  shop:\n    name: Shop\n", "  shop:\n    name: Shop\n    resume_to: town\n", 1)})
	st := testgame.State(db, 1)
	st.Location, st.Spot = "shop", "gate"
	resumeAt(db, st)
	if st.Location != "town" || st.Spot != "shop" {
		t.Fatalf("resumed at %s/%s, want town/shop", st.Location, st.Spot)
	}
}
