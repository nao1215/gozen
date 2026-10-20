package rpg_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestTakeOffTheWorn: take takes from the bag first, then what is worn.
func TestTakeOffTheWorn(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	scout := st.Actors["scout"]
	if scout.Equip["weapon"] != "stick" {
		t.Fatalf("the scout wears %v", scout.Equip)
	}
	st.AddItem("stick", 1)
	st.Take("stick", 2)
	if st.Items["stick"] != 0 || scout.Equip["weapon"] != "" {
		t.Fatalf("after take: bag %d, worn %q", st.Items["stick"], scout.Equip["weapon"])
	}
}
