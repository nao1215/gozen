package play

import (
	"image"
	"reflect"
	"strconv"
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

// TestTacticsToggleByTheMark: a tap on a row's ON/OFF mark turns the row off
// and on again without going through the actions; a tap on the row's text
// only chooses the row.
func TestTacticsToggleByTheMark(t *testing.T) {
	for _, simple := range []bool{false, true} {
		t.Run(strconv.FormatBool(simple), func(t *testing.T) {
			g, s, _ := btlTactics(t, simple, Options{})
			if s.presets != nil {
				s.editing = true
			}
			s.refresh(g)
			s.actions.sel = tacticEdit
			r := s.rows.rowRect(1)
			mark, text := image.Pt(r.Max.X-30, r.Min.Y+r.Dy()/2), image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
			if simple {
				mark = image.Pt(r.Min.X+30, r.Min.Y+r.Dy()/2)
			}
			enabled := func() bool { return s.currentRows(g)[1].Enabled }
			if !enabled() {
				t.Fatalf("the fixture's second row starts off")
			}
			g.in = input{click: true, pdown: true, mouse: text}
			s.update(g)
			if !enabled() || s.rows.sel != 1 {
				t.Fatalf("a tap on the row's text: enabled %v row %d", enabled(), s.rows.sel)
			}
			g.in = input{click: true, pdown: true, mouse: mark}
			s.update(g)
			if enabled() || s.rows.sel != 1 || s.actions.sel != tacticEdit {
				t.Fatalf("a tap on the mark: enabled %v row %d action %d", enabled(), s.rows.sel, s.actions.sel)
			}
			g.in = input{click: true, pdown: true, mouse: mark}
			s.update(g)
			if !enabled() {
				t.Fatalf("a second tap on the mark did not turn the row on again")
			}
		})
	}
}
