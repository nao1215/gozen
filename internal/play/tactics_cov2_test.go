package play

import (
	"image"
	"slices"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// btlClick puts the pointer in the middle of a row of a list and clicks.
func btlClick(g *Game, l *list, row int) {
	r := l.rowRect(row)
	g.in.mouse = image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	g.in.click = true
	g.in.moved = true
}

// TestTacticsClickFocusesAColumn: a click on a row of a column moves the focus there, in the main
// screen and in the preset screen.
func TestTacticsClickFocusesAColumn(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	btlClick(g, s.actions, 2)
	s.update(g)
	g.in.click, g.in.moved = false, false
	if s.focus != 2 {
		t.Fatalf("focus %d after a click on the operations", s.focus)
	}
	btlClick(g, s.members, 1)
	s.update(g)
	g.in.click, g.in.moved = false, false
	if s.focus != 1 || s.actorID() != "guide" {
		t.Fatalf("focus %d member %s: a click on a member chooses it and moves to its rows", s.focus, s.actorID())
	}
	if s.rows.sel != 0 {
		t.Fatalf("row cursor %d after changing member", s.rows.sel)
	}
}

// TestTacticsSimpleClicksAndLimits: in the simple editor, clicking picks the column, the done
// screen goes back to the preset list, an unusable draft is not stored, and the explanation
// of a row names a skill not learned yet softly.
func TestTacticsSimpleClicksAndLimits(t *testing.T) {
	g, s, done := btlTactics(t, true, Options{})
	btlClick(g, s.presets, 2)
	s.update(g)
	g.in.click, g.in.moved = false, false
	if s.focus != 1 {
		t.Fatalf("focus %d", s.focus)
	}
	// "careful" has its description shown with the list.
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "careful" })
	g.Draw(btlImage())
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "manual" })
	g.Draw(btlImage())
	// Build my own, then go back to the preset list with the "pick a style" operation.
	btlKey(g, btnOK)
	if !s.editing {
		t.Fatal("not editing")
	}
	btlKey(g, btnOK)
	btlTacticsAct(g, s, 0, tacticToggle)
	if rows := s.rows.items; len(rows) == 0 || !strings.HasPrefix(rows[0].label, "○ ") {
		t.Fatalf("a row turned off is not marked: %+v", s.rows.items)
	}
	btlTacticsAct(g, s, 0, tacticAutoAll)
	if s.editing || s.focus != 1 {
		t.Fatalf("editing %v focus %d", s.editing, s.focus)
	}
	// A row the member cannot use yet is explained softly.
	s.drafts["scout"] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "any", Action: "big"}}}
	s.editing = true
	s.refresh(g)
	s.rows.sel = 0
	if got := s.description(g); !strings.Contains(strings.ToLower(got), "learn") && !strings.Contains(got, "覚") {
		t.Fatalf("description %q", got)
	}
	// Six rows are refused at accept, and the screen stays open.
	rows := make([]data.TacticRow, 6)
	for i := range rows {
		rows[i] = data.TacticRow{Enabled: true, Condition: "any", Action: "attack"}
	}
	s.drafts["scout"] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: rows}
	s.dirty["scout"] = true
	s.editing = true
	s.refresh(g)
	btlTacticsAct(g, s, 0, tacticAccept)
	if *done || g.stack[len(g.stack)-1] != s {
		t.Fatal("an unusable draft closed the screen")
	}
	if tt := g.st.Actors["scout"].Tactics; tt != nil && tt.Mode == rpg.TacticsManual {
		t.Fatalf("an unusable draft was stored: %+v", tt)
	}
}

// TestTacticsSimpleCancelAndPresetsWithoutMembers: cancel closes the preset screen, "cancel" in the list
// too, and with nobody to choose for, choosing a style does nothing.
func TestTacticsSimpleCancelAndPresetsWithoutMembers(t *testing.T) {
	covSlow(t)
	g, s, done := btlTactics(t, true, Options{})
	btlKey(g, btnCancel)
	if !*done || g.stack[len(g.stack)-1] == s {
		t.Fatal("cancel did not close the preset screen")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	g, s, done = btlTactics(t, true, Options{})
	s.focus = 1
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "cancel" })
	btlKey(g, btnOK)
	if !*done {
		t.Fatal("the cancel entry did not close")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	g, s, _ = btlTactics(t, true, Options{})
	s.ids = nil
	s.drafts = map[string]rpg.ActorTactics{}
	s.members.items = nil
	s.refresh(g)
	s.focus = 1
	for _, id := range []string{"manual", "careful"} {
		s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == id })
		btlKey(g, btnOK)
		if len(s.drafts) != 0 || s.editing {
			t.Fatalf("%s: a draft was made for nobody", id)
		}
	}
	g.Draw(btlImage())
	s.rows.items = nil
	s.editing = true
	g.Draw(btlImage()) // the empty-list note of the simple editor
}

// TestTacticsMoveAtTheEnds: moving the first row up or the last row down changes nothing.
func TestTacticsMoveAtTheEnds(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	before := slices.Clone(s.currentRows(g))
	s.rows.sel = 0
	s.actions.sel = tacticMoveUp
	s.act(g)
	s.rows.sel = len(before) - 1
	s.actions.sel = tacticMoveDown
	s.act(g)
	if s.dirty["scout"] {
		t.Fatal("a move that goes nowhere made the draft dirty")
	}
	for i, r := range s.currentRows(g) {
		if r.Action != before[i].Action {
			t.Fatalf("row %d changed", i)
		}
	}
}

// TestTacticsEditorEdgeCases: the editor's default row when the catalog has no "always" condition
// first, or no action fits; its click and left key; the unlearned note; the limit of the simple
// editor; and a row's threshold copied so editing it leaves the draft alone until accepted.
func TestTacticsEditorEdgeCases(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	g.db.TacticConditions["a_hurt"] = &data.TacticCondition{Name: "Hurt first", Kind: "hp_below", Subject: "allies"}
	e := newTacticEditor(g, s, true)
	if e.row.Condition == "a_hurt" || e.row.Condition == "" {
		t.Fatalf("default row %+v", e.row)
	}
	saved := g.db.TacticActions
	g.db.TacticActions = map[string]*data.TacticAction{}
	if r := e.defaultRow(g); r.Condition != "" || !r.Enabled {
		t.Fatalf("no action fits, row %+v", r)
	}
	g.db.TacticActions = saved

	// A threshold row: the editor works on a copy.
	pct := 40
	s.drafts["scout"] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "low_hp", Percent: &pct, Target: "ally", Action: "potion"}}}
	s.refresh(g)
	e = newTacticEditor(g, s, false)
	*e.row.Percent = 90
	if pct != 40 || *s.drafts["scout"].Rows[0].Percent != 40 {
		t.Fatal("the editor changed the draft's threshold before it was accepted")
	}
	g.push(e)
	// Left key leaves the choices; moving in the fields refreshes the choices; no choice made does nothing.
	btlKey(g, btnRight)
	if !e.choosing {
		t.Fatal("not choosing")
	}
	btlKey(g, btnDown) // moves in the options
	if !e.choosing {
		t.Fatal("moving in the options ended the choice")
	}
	btlKey(g, btnLeft)
	if e.choosing {
		t.Fatal("left did not leave the choices")
	}
	btlKey(g, btnDown)
	if e.fields.sel != 1 {
		t.Fatalf("field %d", e.fields.sel)
	}
	// A click on a field opens its choices; a click on a choice takes it.
	e.fields.sel = tacticCondition
	e.refresh(g)
	btlClick(g, e.fields, tacticCondition)
	e.update(g)
	g.in.click, g.in.moved = false, false
	if !e.choosing {
		t.Fatal("a click on a field did not open its choices")
	}
	want := e.options.items[1].id
	btlClick(g, e.options, 1)
	e.update(g)
	g.in.click, g.in.moved = false, false
	if e.choosing || e.row.Condition != want {
		t.Fatalf("a click on a choice: choosing %v condition %q, want %q", e.choosing, e.row.Condition, want)
	}
	// An action not learned yet says so in the description.
	e.fields.sel = tacticAction
	e.refresh(g)
	e.choosing = true
	e.options.sel = slices.IndexFunc(e.options.items, func(it listItem) bool { return it.id == "big" })
	if got := e.description(g); !strings.Contains(strings.ToLower(got), "not") && !strings.Contains(got, "未習得") {
		t.Fatalf("description %q", got)
	}
	g.Draw(btlImage())
	g.pop()
}

// TestTacticEditorSimpleLimit: adding a sixth row in the simple editor is refused with a buzz and
// leaves the draft as it was.
func TestTacticEditorSimpleLimit(t *testing.T) {
	g, s, _ := btlTactics(t, true, Options{})
	rows := make([]data.TacticRow, data.TacticRowLimit)
	for i := range rows {
		rows[i] = data.TacticRow{Enabled: true, Condition: "any", Action: "attack"}
	}
	s.drafts["scout"] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: rows}
	s.editing = true
	s.refresh(g)
	e := newTacticEditor(g, s, true)
	g.push(e)
	e.apply(g)
	if g.stack[len(g.stack)-1] != e || len(s.drafts["scout"].Rows) != data.TacticRowLimit {
		t.Fatalf("a sixth row was added: %d rows", len(s.drafts["scout"].Rows))
	}
	// The simple editor has a default row too.
	if e.row.Condition != "any" || e.row.Action != "attack" {
		t.Fatalf("default simple row %+v", e.row)
	}
}

// TestTacticReasonsClick: a click on a column of the reasons screen moves the focus there.
func TestTacticReasonsClick(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	r := newTacticReasons(g, nil)
	r.history.items = []listItem{{label: "a"}, {label: "b"}}
	r.rules.items = []listItem{{label: "x"}}
	g.push(r)
	btlClick(g, r.rules, 0)
	r.update(g)
	g.in.click, g.in.moved = false, false
	if _, ok := g.stack[len(g.stack)-1].(*tacticText); !ok {
		t.Fatalf("a click on a rule opened %T", g.stack[len(g.stack)-1])
	}
	g.pop()
	btlClick(g, r.history, 1)
	r.update(g)
	g.in.click, g.in.moved = false, false
	if r.history.sel != 1 || r.focus != 1 {
		t.Fatalf("history cursor %d focus %d: a click on a history row chooses it and moves to its rules", r.history.sel, r.focus)
	}
}
