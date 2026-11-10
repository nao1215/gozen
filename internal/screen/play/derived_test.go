package play

import (
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
)

// A derived value is made once for a DB and again when the DB is replaced, as a
// live reload does.
func TestDerivedIsRemadeForNewData(t *testing.T) {
	var d derived[int]
	made := 0
	build := func() int { made++; return made }
	first, second := &data.DB{}, &data.DB{}
	if d.of(first, build) != 1 || d.of(first, build) != 1 {
		t.Fatalf("the value was made %d times for one DB", made)
	}
	if d.of(second, build) != 2 || made != 2 {
		t.Fatalf("the value was not made again for new data: made %d", made)
	}
}

// The lines of an event and the panel's index of names follow a replaced DB.
func TestEventLinesAndPanelUsesFollowTheData(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	g.eventLines("nothing")
	uses := g.panelUses()
	if g.sceneLines.db != g.db || g.uses.db != g.db {
		t.Fatal("the values were not made for the game's data")
	}
	g.db = data.Fork(g.db)
	g.eventLines("nothing")
	if g.sceneLines.db != g.db {
		t.Fatal("the lines kept the old data")
	}
	if again := g.panelUses(); g.uses.db != g.db || len(again) != len(uses) {
		t.Fatalf("the index kept the old data: %d names, was %d", len(again), len(uses))
	}
}
