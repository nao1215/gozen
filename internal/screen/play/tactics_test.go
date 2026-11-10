package play

import (
	"image"
	"strconv"
	"testing"
)

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
