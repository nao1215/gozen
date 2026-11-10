package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/rules/event"
)

// The offer's picture is taken away by the update once it has left, never by a draw: a
// frame drawn twice, or not at all, changes no state.
func TestRecruitLeavesInTheUpdate(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	g.tick = 5
	g.recruit = &recruitView{offer: &event.RecruitOffer{Name: "Mote"}, start: g.tick}
	g.endRecruit(false)
	g.tick += recruitLeaveTicks
	g.drawRecruit(covScreen())
	if g.recruit == nil {
		t.Fatal("drawing took the offer's picture away")
	}
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if g.recruit != nil {
		t.Fatal("the update left the picture that has gone")
	}
}
