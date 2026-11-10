package play

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/model/rpg"
)

// Accepting takes every draft or none: when a later member's draft is refused, the members
// before it keep the tactics they had and the screen stays open.
func TestTacticsAcceptIsAllOrNothing(t *testing.T) {
	g, s, done := btlTactics(t, false, Options{})
	if len(s.ids) < 2 {
		t.Fatalf("members %v", s.ids)
	}
	first, second := s.ids[0], s.ids[1]
	btlTacticsAct(g, s, 0, tacticToggle) // a draft of the first member that can be taken
	if !s.dirty[first] {
		t.Fatalf("the first member %q has no draft", first)
	}
	before := g.st.Actors[first].TacticsDraft()
	s.drafts[second], s.dirty[second] = rpg.ActorTactics{Version: rpg.TacticsVersion + 1, Mode: rpg.TacticsAuto}, true
	s.actions.sel, s.focus = tacticAccept, 2
	s.act(g)
	if *done || g.stack[len(g.stack)-1] != s {
		t.Fatal("the screen closed although a draft was refused")
	}
	if got := g.st.Actors[first].TacticsDraft(); !reflect.DeepEqual(got, before) {
		t.Fatalf("the first member took its draft although the second was refused: %+v", got)
	}
}
