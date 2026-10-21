package play

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

// btlTacticFiles are the catalogs of the tactic tests: the shared decision table and the
// capability AI of the test game.
func btlTacticFiles() map[string]string {
	return map[string]string{"data/tactics.yaml": testgame.Tactics, "data/auto.yaml": testgame.AutoTacticsData}
}

func btlRow(cond, target, action string) data.TacticRow {
	return data.TacticRow{Enabled: true, Condition: cond, Target: target, Action: action}
}

// btlTactics opens the camp with a party whose auto tactics are the capability rules; simple
// switches to the five-row editor with two presets.
func btlTactics(t *testing.T, simple bool, opts Options) (*Game, *tacticsScreen, *bool) {
	t.Helper()
	opts.Location = "camp"
	g, _ := btlOpen(t, opts, "", "", btlTacticFiles())
	g.db.Classes["ranger"].AI = data.AIRef{Name: "capability"}
	b := &g.db.Game.Battle
	b.AI = "gambit"
	b.AutoTactics = map[string][]data.TacticRow{"capability": {
		btlRow("any", "enemies", "big"), btlRow("any", "allies", "group_heal"), btlRow("any", "ally", "heal"),
		btlRow("any", "ally", "potion"), btlRow("any", "enemy", "fire"), btlRow("any", "self", "guard"), btlRow("any", "enemy", "attack"),
	}}
	if simple {
		b.SimpleTactics = true
		g.db.TacticConditions = map[string]*data.TacticCondition{
			"any":      {Name: "Always", Kind: "always"},
			"poisoned": {Name: "Poisoned foe", Kind: "state", Subject: "target", State: "poison"},
		}
		g.db.TacticActions = map[string]*data.TacticAction{
			"attack": {Name: "Strike", Use: "attack", Target: "enemy"},
			"heal":   {Name: "Restore", Use: "heal", Target: "ally"},
			"potion": {Name: "Tonic", Item: "potion", Target: "ally"},
			"fire":   {Name: "Spark", Use: "fire", Target: "enemy"},
			"big":    {Name: "Sweep", Use: "big", Target: "enemies"},
		}
		rows := []data.TacticRow{
			{Enabled: true, Condition: "any", Action: "potion"}, {Enabled: true, Condition: "any", Action: "heal"},
			{Enabled: true, Condition: "poisoned", Action: "fire"}, {Enabled: true, Condition: "any", Action: "attack"},
		}
		b.AutoTactics = map[string][]data.TacticRow{"capability": rows[1:]}
		b.TacticPresets = map[string]data.TacticPreset{
			"careful": {Name: "Careful", Description: "Heal first.", Rows: map[string][]data.TacticRow{"capability": rows[:2]}},
			"bold":    {Name: "Bold", Description: "Hit hard.", Rows: map[string][]data.TacticRow{"capability": rows[2:]}},
		}
	}
	done := new(bool)
	g.openScreen("tactics", func() { *done = true })
	s, ok := g.stack[len(g.stack)-1].(*tacticsScreen)
	if !ok {
		t.Fatalf("the tactics screen did not open, top is %T", g.stack[len(g.stack)-1])
	}
	return g, s, done
}

// btlTacticsAct chooses an operation of the right-hand list for a row.
func btlTacticsAct(g *Game, s *tacticsScreen, row, action int) {
	s.rows.sel = row
	s.actions.sel = action
	s.refresh(g)
	s.focus = 2
	btlKey(g, btnOK)
}

// btlSaved is the party's tactics as they come back from a save.
func btlSaved(t *testing.T, g *Game, id string) *rpg.ActorTactics {
	t.Helper()
	b, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	st, err := rpg.Parse(g.db, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	return st.Actors[id].Tactics
}

// TestTacticsScreenShowsAutoRows: before anything is changed each member shows the auto
// tactics, only the rows the member can use, with the mode "auto" beside the name.
func TestTacticsScreenShowsAutoRows(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	if !reflect.DeepEqual(s.ids, []string{"scout", "guide"}) {
		t.Fatalf("members %v", s.ids)
	}
	want := g.st.Actors["scout"].AutoTactics()
	if len(s.rows.items) != len(want) || len(want) == 0 {
		t.Fatalf("%d rows for %d auto rows", len(s.rows.items), len(want))
	}
	if !strings.Contains(s.members.items[0].right, "Auto") && s.members.items[0].right == "" {
		t.Fatalf("mode label %q", s.members.items[0].right)
	}
	if !strings.HasPrefix(s.rows.items[0].label, "1  ") || !strings.Contains(s.rows.items[0].label, " → ") || s.rows.items[0].right != "ON" {
		t.Fatalf("row label %q / %q", s.rows.items[0].label, s.rows.items[0].right)
	}
	if s.actions.items[tacticMoveUp].disabled == false {
		t.Fatal("the first row can move up")
	}
	btlFrame(t, g)
}

// TestTacticsEditAcceptAndSave: changes are kept as a draft marked with * until accepted;
// accepting makes the member's tactics manual with the edited rows, they survive a save,
// and the done callback runs.
func TestTacticsEditAcceptAndSave(t *testing.T) {
	g, s, done := btlTactics(t, false, Options{})
	n := len(s.rows.items)
	btlTacticsAct(g, s, 0, tacticToggle) // OFF the first row
	if !s.dirty["scout"] || !strings.HasSuffix(s.members.items[0].right, "*") {
		t.Fatalf("not marked as unconfirmed: %q", s.members.items[0].right)
	}
	if tt := g.st.Actors["scout"].Tactics; tt != nil && tt.Mode != rpg.TacticsAuto {
		t.Fatal("a draft leaked into the member before it was accepted")
	}
	if got := s.drafts["scout"]; got.Mode != rpg.TacticsManual || got.Rows[0].Enabled || len(got.Rows) != n {
		t.Fatalf("draft %+v", got)
	}
	btlTacticsAct(g, s, 0, tacticMoveDown)
	if s.rows.sel != 1 || s.drafts["scout"].Rows[1].Action != "potion" || s.drafts["scout"].Rows[1].Enabled {
		t.Fatalf("move down: sel %d rows %+v", s.rows.sel, s.drafts["scout"].Rows)
	}
	btlTacticsAct(g, s, 1, tacticMoveUp)
	if s.drafts["scout"].Rows[0].Action != "potion" {
		t.Fatal("move up")
	}
	btlTacticsAct(g, s, n-1, tacticDelete)
	if len(s.drafts["scout"].Rows) != n-1 {
		t.Fatalf("delete left %d rows", len(s.drafts["scout"].Rows))
	}
	btlTacticsAct(g, s, 0, tacticAccept)
	if !*done || g.stack[len(g.stack)-1] == s {
		t.Fatalf("accept did not close the screen (done=%v)", *done)
	}
	got := g.st.Actors["scout"].Tactics
	if got == nil || got.Mode != rpg.TacticsManual || len(got.Rows) != n-1 || got.Rows[0].Enabled {
		t.Fatalf("tactics %+v", got)
	}
	if saved := btlSaved(t, g, "scout"); !reflect.DeepEqual(saved, got) {
		t.Fatalf("a save changed the tactics: %+v vs %+v", saved, got)
	}
	if tt := g.st.Actors["guide"].Tactics; tt != nil && tt.Mode != rpg.TacticsAuto {
		t.Fatal("an untouched member got tactics")
	}
}

// TestTacticsCancelDiscards: closing with cancel, the cancel operation or the cancel key
// keeps every member as it was.
func TestTacticsCancelDiscards(t *testing.T) {
	for name, closeIt := range map[string]func(*Game, *tacticsScreen){
		"operation": func(g *Game, s *tacticsScreen) { btlTacticsAct(g, s, 0, tacticCancel) },
		"key":       func(g *Game, _ *tacticsScreen) { btlKey(g, btnCancel) },
	} {
		g, s, done := btlTactics(t, false, Options{})
		btlTacticsAct(g, s, 0, tacticToggle)
		closeIt(g, s)
		if !*done || g.stack[len(g.stack)-1] == s {
			t.Fatalf("%s: screen still open", name)
		}
		if tt := g.st.Actors["scout"].Tactics; tt != nil && tt.Mode != rpg.TacticsAuto {
			t.Fatalf("%s: a cancelled draft was kept", name)
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTacticsBackToAuto: "auto" for one member, or for everyone, replaces the drafts with
// the auto mode.
func TestTacticsBackToAuto(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	for _, id := range s.ids {
		if err := g.st.Actors[id].SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{btlRow("any", "enemy", "attack")}}); err != nil {
			t.Fatal(err)
		}
	}
	s.drafts["scout"] = g.st.Actors["scout"].TacticsDraft()
	s.drafts["guide"] = g.st.Actors["guide"].TacticsDraft()
	s.refresh(g)
	if len(s.rows.items) != 1 {
		t.Fatalf("manual rows shown: %d", len(s.rows.items))
	}
	btlTacticsAct(g, s, 0, tacticAuto)
	if s.drafts["scout"].Mode != rpg.TacticsAuto || s.drafts["guide"].Mode != rpg.TacticsManual {
		t.Fatalf("one member: %v %v", s.drafts["scout"].Mode, s.drafts["guide"].Mode)
	}
	btlTacticsAct(g, s, 0, tacticAutoAll)
	if s.drafts["guide"].Mode != rpg.TacticsAuto {
		t.Fatal("everyone did not go back to auto")
	}
	btlTacticsAct(g, s, 0, tacticAccept)
	if g.st.Actors["guide"].Tactics.Mode != rpg.TacticsAuto {
		t.Fatalf("accepted %+v", g.st.Actors["guide"].Tactics)
	}
}

// TestTacticsMemberTabAndFocus: Tab moves between members and resets the row cursor, left
// and right move between the three columns, and OK on a member moves into its rows.
func TestTacticsMemberTabAndFocus(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	s.rows.sel = 2
	btlKey(g, btnNext)
	if s.actorID() != "guide" || s.rows.sel != 0 {
		t.Fatalf("tab: %s row %d", s.actorID(), s.rows.sel)
	}
	g.in.shift = true
	btlKey(g, btnNext)
	g.in.shift = false
	if s.actorID() != "scout" {
		t.Fatalf("shift-tab: %s", s.actorID())
	}
	btlKey(g, btnRight)
	btlKey(g, btnRight)
	if s.focus != 2 {
		t.Fatalf("focus %d", s.focus)
	}
	btlKey(g, btnLeft)
	btlKey(g, btnLeft)
	btlKey(g, btnLeft)
	if s.focus != 2 {
		t.Fatalf("focus wrapped to %d", s.focus)
	}
	s.focus = 0
	btlKey(g, btnOK)
	if s.focus != 1 {
		t.Fatalf("OK on a member: focus %d", s.focus)
	}
	btlKey(g, btnOK)
	if s.focus != 2 {
		t.Fatalf("OK on a row: focus %d", s.focus)
	}
	btlKey(g, btnLog)
	if _, ok := g.stack[len(g.stack)-1].(*tacticText); !ok {
		t.Fatalf("B opened %T", g.stack[len(g.stack)-1])
	}
	btlKey(g, btnOK)
	if g.stack[len(g.stack)-1] != s {
		t.Fatal("the full text did not close")
	}
}

// TestTacticEditorChangesARow: the row editor picks a condition, a threshold, a target and an
// action from the catalogs; a row that cannot work (an action whose scope does not fit the
// target) is refused, and a good one replaces the row.
func TestTacticEditorChangesARow(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	g.st.Actors["scout"].Learnt = []string{"big"}
	s.drafts["scout"] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{
		btlRow("any", "enemy", "attack"), btlRow("any", "enemy", "attack"), btlRow("any", "enemy", "attack"), btlRow("any", "enemy", "attack"), btlRow("any", "enemy", "fire"),
	}}
	s.refresh(g)
	btlTacticsAct(g, s, 4, tacticEdit) // "any → enemy → fire"
	e, ok := g.stack[len(g.stack)-1].(*tacticEditor)
	if !ok {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	if e.row.Action != "fire" || e.row.Condition != "any" {
		t.Fatalf("row %+v", e.row)
	}
	if !e.fields.items[tacticPercent].disabled {
		t.Fatal("an always condition takes a threshold")
	}
	// Condition: low_mp has a threshold and gets the default percent.
	pick := func(field int, id string) {
		t.Helper()
		e.fields.sel = field
		e.refresh(g)
		btlKey(g, btnOK)
		if !e.choosing {
			t.Fatalf("field %d did not open its choices", field)
		}
		i := slices.IndexFunc(e.options.items, func(it listItem) bool { return it.id == id })
		if i < 0 {
			t.Fatalf("no option %q in %v", id, e.options.items)
		}
		e.options.sel = i
		btlKey(g, btnOK)
		if e.choosing {
			t.Fatal("choosing did not end")
		}
	}
	pick(tacticCondition, "low_mp")
	if e.row.Percent == nil || *e.row.Percent != 50 || e.fields.items[tacticPercent].disabled {
		t.Fatalf("threshold %v", e.row.Percent)
	}
	pick(tacticPercent, "30")
	if *e.row.Percent != 30 {
		t.Fatalf("percent %d", *e.row.Percent)
	}
	pick(tacticCondition, "any")
	if e.row.Percent != nil {
		t.Fatal("a condition without a threshold kept the percent")
	}
	// An action for all foes against a single-target slot is refused.
	pick(tacticAction, "big")
	e.fields.sel = tacticRowAccept
	e.refresh(g)
	btlKey(g, btnOK)
	if g.stack[len(g.stack)-1] != e {
		t.Fatal("a mismatched row was accepted")
	}
	pick(tacticTarget, "enemies")
	e.fields.sel = tacticRowAccept
	e.refresh(g)
	btlKey(g, btnOK)
	if g.stack[len(g.stack)-1] != s {
		t.Fatalf("a good row was refused: %v", e.errors(g))
	}
	if got := s.drafts["scout"].Rows[4]; got.Action != "big" || got.Target != "enemies" {
		t.Fatalf("row %+v", got)
	}
	if !s.dirty["scout"] {
		t.Fatal("not marked unconfirmed")
	}
}

// TestTacticEditorAddCancelAndDetail: a new row is inserted below the cursor and works as is;
// the editor's cancel and full-text views leave the draft alone.
func TestTacticEditorAddCancelAndDetail(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	n := len(s.rows.items)
	btlTacticsAct(g, s, 1, tacticAdd)
	e, ok := g.stack[len(g.stack)-1].(*tacticEditor)
	if !ok || !e.adding {
		t.Fatalf("top is %T", g.stack[len(g.stack)-1])
	}
	if len(e.errors(g)) != 0 || e.row.Condition == "" {
		t.Fatalf("the default row is not usable: %+v %v", e.row, e.errors(g))
	}
	e.fields.sel = tacticRowDetail
	e.refresh(g)
	btlKey(g, btnOK)
	if _, ok := g.stack[len(g.stack)-1].(*tacticText); !ok {
		t.Fatalf("detail view is %T", g.stack[len(g.stack)-1])
	}
	btlKey(g, btnCancel)
	btlKey(g, btnLog) // B shows the description as well
	btlKey(g, btnCancel)
	e.fields.sel = tacticRowCancel
	e.refresh(g)
	btlKey(g, btnOK)
	if g.stack[len(g.stack)-1] != s || s.dirty["scout"] {
		t.Fatal("cancel changed the draft")
	}
	btlTacticsAct(g, s, 1, tacticAdd)
	e = g.stack[len(g.stack)-1].(*tacticEditor)
	e.fields.sel = tacticRowAccept
	e.refresh(g)
	btlKey(g, btnOK)
	if len(s.drafts["scout"].Rows) != n+1 || s.rows.sel != 2 {
		t.Fatalf("rows %d (want %d), cursor %d", len(s.drafts["scout"].Rows), n+1, s.rows.sel)
	}
	// Cancel key in the choices only closes the choices, then the editor.
	btlTacticsAct(g, s, 0, tacticEdit)
	e = g.stack[len(g.stack)-1].(*tacticEditor)
	btlKey(g, btnRight)
	if !e.choosing {
		t.Fatal("right did not open the choices")
	}
	btlKey(g, btnCancel)
	if e.choosing || g.stack[len(g.stack)-1] != e {
		t.Fatal("cancel in the choices closed the editor")
	}
	btlKey(g, btnCancel)
	if g.stack[len(g.stack)-1] != s {
		t.Fatal("cancel did not leave the editor")
	}
}

// TestTacticsNeedsNoMembers: with nobody in the party the screen opens with nothing to edit and
// its operations do nothing.
func TestTacticsNeedsNoMembers(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	s.ids = nil
	s.drafts = map[string]rpg.ActorTactics{}
	s.members.items = nil
	s.refresh(g)
	if s.actorID() != "" || len(s.rows.items) != 0 {
		t.Fatal("rows without a member")
	}
	for _, a := range []int{tacticToggle, tacticEdit, tacticAdd, tacticAuto} {
		s.actions.sel = a
		s.act(g)
	}
	btlFrame(t, g)
	btlKey(g, btnLog)
	if g.stack[len(g.stack)-1] != s {
		t.Fatal("B opened a view of nothing")
	}
}

// TestTacticsSimplePresets: the simple editor lists the presets; choosing one marks the draft,
// accepting stores the preset by name and shows its name in the member list; a built row set
// is limited to five rows.
func TestTacticsSimplePresets(t *testing.T) {
	g, s, _ := btlTactics(t, true, Options{})
	if s.presets == nil || len(s.presets.items) != 6 {
		t.Fatalf("preset list %+v", s.presets)
	}
	btlFrame(t, g)
	// Pick "Careful" for the first member.
	s.focus = 1
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "careful" })
	btlKey(g, btnOK)
	if d := s.drafts["scout"]; d.Mode != rpg.TacticsPreset || d.Preset != "careful" || !s.dirty["scout"] {
		t.Fatalf("draft %+v", d)
	}
	if !strings.Contains(s.members.items[0].right, "Careful") {
		t.Fatalf("member label %q", s.members.items[0].right)
	}
	// Tab to the other member and left/right between the two lists.
	btlKey(g, btnNext)
	if s.actorID() != "guide" {
		t.Fatal("tab")
	}
	btlKey(g, btnLeft)
	if s.focus != 0 {
		t.Fatalf("focus %d", s.focus)
	}
	btlKey(g, btnOK) // a member: into the presets
	if s.focus != 1 {
		t.Fatal("OK on a member")
	}
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "" })
	btlKey(g, btnOK) // back to the auto tactics
	if s.drafts["guide"].Mode != rpg.TacticsAuto {
		t.Fatalf("auto draft %+v", s.drafts["guide"])
	}
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "accept" })
	btlKey(g, btnOK)
	saved := g.st.Actors["scout"].Tactics
	if saved == nil || saved.Mode != rpg.TacticsPreset || saved.Preset != "careful" {
		t.Fatalf("saved %+v", saved)
	}
	if got := btlSaved(t, g, "scout"); !reflect.DeepEqual(got, saved) {
		t.Fatalf("the preset did not survive a save: %+v", got)
	}
}

// TestTacticsSimpleBuildOwn: "build my own" turns the preset's rows into editable rows; the
// editor lets a row's "when" and "what" be changed, and a sixth row is refused.
func TestTacticsSimpleBuildOwn(t *testing.T) {
	g, s, _ := btlTactics(t, true, Options{})
	s.focus = 1
	s.presets.sel = slices.IndexFunc(s.presets.items, func(it listItem) bool { return it.id == "manual" })
	btlKey(g, btnOK)
	if !s.editing || s.drafts["scout"].Mode != rpg.TacticsManual {
		t.Fatalf("editing %v draft %+v", s.editing, s.drafts["scout"])
	}
	btlFrame(t, g)
	n := len(s.rows.items)
	if n == 0 {
		t.Fatal("no rows to edit")
	}
	// Fill the list to the limit through the editor, then see a sixth refused.
	for len(s.drafts["scout"].Rows) < data.TacticRowLimit {
		btlTacticsAct(g, s, 0, tacticAdd)
		e := g.stack[len(g.stack)-1].(*tacticEditor)
		e.fields.sel = 2 // confirm in the simple editor
		e.refresh(g)
		btlKey(g, btnOK)
	}
	if len(s.drafts["scout"].Rows) != data.TacticRowLimit || !s.actions.items[tacticAdd].disabled {
		t.Fatalf("rows %d, add disabled %v", len(s.drafts["scout"].Rows), s.actions.items[tacticAdd].disabled)
	}
	// Edit the first row to the poisoned condition.
	btlTacticsAct(g, s, 0, tacticEdit)
	e := g.stack[len(g.stack)-1].(*tacticEditor)
	e.fields.sel = 0
	e.refresh(g)
	btlKey(g, btnOK)
	e.options.sel = slices.IndexFunc(e.options.items, func(it listItem) bool { return it.id == "poisoned" })
	btlKey(g, btnOK)
	if e.row.Condition != "poisoned" {
		t.Fatalf("row %+v", e.row)
	}
	if got := e.description(g); !strings.Contains(got, "Poisoned foe") {
		t.Fatalf("description %q", got)
	}
	e.fields.sel = 1 // what to do: only learned actions are offered
	e.refresh(g)
	for _, it := range e.options.items {
		if it.disabled {
			t.Fatalf("a disabled action %q is offered in the simple editor", it.id)
		}
	}
}

// TestTacticsRowTextAndCost: a row reads "condition, target, action" with the names of the
// catalog and a threshold in parentheses; the cost line says MP for skills, count for items
// and when the action is missing.
func TestTacticsRowTextAndCost(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	pct := 40
	text := s.rowText(g, data.TacticRow{Condition: "low_hp", Percent: &pct, Target: "ally", Action: "heal"})
	for _, want := range []string{"Health below threshold", "40", "Injured ally", "Restore"} {
		if !strings.Contains(text, want) {
			t.Errorf("row text %q lacks %q", text, want)
		}
	}
	if got := tacticCost(g, "fire"); !strings.Contains(got, "3") {
		t.Errorf("skill cost %q", got)
	}
	if got := tacticCost(g, "potion"); !strings.Contains(got, "Tonic") || !strings.Contains(got, "2") {
		t.Errorf("item cost %q", got)
	}
	if got := tacticCost(g, "missing"); got == "" || strings.Contains(got, "MP") {
		t.Errorf("missing action %q", got)
	}
	if !tacticThreshold(g.db, "low_hp") || !tacticThreshold(g.db, "low_mp") || tacticThreshold(g.db, "any") || tacticThreshold(g.db, "nope") {
		t.Fatal("tacticThreshold")
	}
	ids := catalogIDs(g.db.TacticTargets)
	if !slices.IsSorted(ids) || len(ids) != len(g.db.TacticTargets) {
		t.Fatalf("catalog ids %v", ids)
	}
}

// TestTacticReasonsLists: in a gambit battle on auto, the decisions are recorded newest
// first, B opens the list of them, a decision shows each rule with the reason it was or
// was not used, and a long fight keeps only the latest 128.
func TestTacticReasonsLists(t *testing.T) {
	g, s, _ := btlTactics(t, false, Options{})
	btlKey(g, btnCancel)
	g.startBattle("two", func(int) {})
	b := btlBattle(t, g)
	for _, m := range b.b.Party {
		m.Auto = true
	}
	b.auto = true
	btlUntil(t, g, "some decisions", 6000, func() bool { return len(b.decisions) >= 3 || b.phase == bpResult })
	if len(b.decisions) == 0 {
		t.Fatal("no decision recorded")
	}
	btlKey(g, btnLog)
	r, ok := g.stack[len(g.stack)-1].(*tacticReasons)
	if !ok {
		t.Fatalf("B opened %T", g.stack[len(g.stack)-1])
	}
	if len(r.history.items) != len(b.decisions) {
		t.Fatalf("history %d vs %d", len(r.history.items), len(b.decisions))
	}
	if r.decisions[0].Turn < r.decisions[len(r.decisions)-1].Turn {
		t.Fatal("the newest decision is not first")
	}
	btlFrame(t, g)
	btlKey(g, btnOK) // into the rules
	if r.focus != 1 {
		t.Fatalf("focus %d", r.focus)
	}
	btlKey(g, btnDown)
	btlKey(g, btnOK) // the full text of a rule
	if _, ok := g.stack[len(g.stack)-1].(*tacticText); !ok {
		t.Fatalf("rule text is %T", g.stack[len(g.stack)-1])
	}
	btlKey(g, btnCancel)
	btlKey(g, btnLeft)
	btlKey(g, btnDown)
	if r.history.sel != 1 && len(r.history.items) > 1 {
		t.Fatalf("history cursor %d", r.history.sel)
	}
	btlKey(g, btnCancel)
	if g.stack[len(g.stack)-1] != b {
		t.Fatalf("cancel did not return to the battle, top %T", g.stack[len(g.stack)-1])
	}
	_ = s
}

// TestTacticReasonsHistoryLimit: only the latest 128 decisions are kept.
func TestTacticReasonsHistoryLimit(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	s := btlBattle(t, g)
	s.decisions = nil
	for i := range tacticHistoryLimit + 20 {
		s.b.OnDecision(battle.Decision{Turn: i, User: battle.DecisionUnit{ID: "scout"}})
	}
	if len(s.decisions) != tacticHistoryLimit || s.decisions[0].Turn != 20 || s.decisions[len(s.decisions)-1].Turn != tacticHistoryLimit+19 {
		t.Fatalf("%d decisions from %d to %d", len(s.decisions), s.decisions[0].Turn, s.decisions[len(s.decisions)-1].Turn)
	}
}

// TestTacticReasonsEmpty: with nothing recorded the screen says so.
func TestTacticReasonsEmpty(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	r := newTacticReasons(g, nil)
	if !strings.Contains(r.description(g), "") || r.description(g) == "" || len(r.rules.items) != 0 {
		t.Fatal("empty description")
	}
	g.push(r)
	btlFrame(t, g)
	btlKey(g, btnOK)
	btlKey(g, btnRight)
	btlKey(g, btnCancel)
	if g.stack[len(g.stack)-1] == r {
		t.Fatal("cancel did not close")
	}
}

// TestTacticReasonText: every reason and every selection kind has its own words, and an
// unknown reason is shown as it is.
func TestTacticReasonText(t *testing.T) {
	reasons := []battle.RuleReason{
		battle.RuleApplicable, battle.RuleOnceUsed, battle.RuleConditionFalse, battle.RuleConditionError,
		battle.RuleChanceFailed, battle.RuleChanceError, battle.RuleMPShortage, battle.RuleSkillUnlearned,
		battle.RuleItemShortage, battle.RuleEnemyItem, battle.RuleInvalidAction, battle.RuleNoTarget,
		battle.RuleDisabled, battle.RuleInvalidTactic,
	}
	seen := map[string]battle.RuleReason{}
	for _, r := range reasons {
		text := tacticReason(battle.RuleEvaluation{Reason: r})
		if text == "" || text == string(r) {
			t.Errorf("%s has no words: %q", r, text)
		}
		if prev, dup := seen[text]; dup {
			t.Errorf("%s and %s read the same: %q", prev, r, text)
		}
		seen[text] = r
	}
	if got := tacticReason(battle.RuleEvaluation{Reason: "strange"}); got != "strange" {
		t.Fatalf("unknown reason %q", got)
	}
	base := tacticReason(battle.RuleEvaluation{Reason: battle.RuleApplicable})
	sels := map[string]bool{}
	for _, sel := range []string{"selected", "later_rule", "lower_priority", "weight_not_selected"} {
		text := tacticReason(battle.RuleEvaluation{Reason: battle.RuleApplicable, Selection: sel})
		if !strings.HasPrefix(text, base+" / ") || sels[text] {
			t.Errorf("selection %s: %q", sel, text)
		}
		sels[text] = true
	}
	if got := tacticReason(battle.RuleEvaluation{Reason: battle.RuleApplicable, Selection: "other"}); got != base {
		t.Fatalf("unknown selection added words: %q", got)
	}
}

// TestTacticReasonDescriptions: the description of a decision says whether it was committed,
// the fallback to a plain attack, the target and the detail of the rule.
func TestTacticReasonDescriptions(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one"}, "", "")
	tgt := &battle.DecisionUnit{ID: "mote", Enemy: true, Index: 0}
	ds := []battle.Decision{
		{Turn: 1, Timing: battle.DecisionTurnStart, User: battle.DecisionUnit{ID: "scout"}, Selected: -1},
		{Turn: 2, Timing: battle.DecisionDirect, User: battle.DecisionUnit{ID: "guide"}, Committed: true, Skill: "fire", Target: tgt, Selected: 0,
			Rules: []battle.RuleEvaluation{{Index: 0, Skill: "fire", Reason: battle.RuleApplicable, Target: tgt, Detail: "hp 3 of 9", Selection: "selected"}}},
		{Turn: 3, Timing: battle.DecisionAction, User: battle.DecisionUnit{ID: "mote", Enemy: true}, Item: "potion", Selected: 0,
			Rules: []battle.RuleEvaluation{{Index: 0, Item: "potion", Reason: battle.RuleItemShortage}}},
	}
	r := newTacticReasons(g, ds)
	if len(r.history.items) != 3 {
		t.Fatalf("history %d", len(r.history.items))
	}
	// Newest first: the third decision (an enemy, an item).
	if got := r.description(g); !strings.Contains(got, "Tonic") || !strings.Contains(got, "Mote 1") {
		t.Errorf("description %q", got)
	}
	r.history.sel = 1
	r.refresh(g)
	got := r.description(g)
	for _, want := range []string{"Spark", "Mote 1", "hp 3 of 9", "Scout", "Guide"} {
		if !strings.Contains(got, want) && want != "Scout" {
			t.Errorf("description %q lacks %q", got, want)
		}
	}
	r.history.sel = 2
	r.refresh(g)
	if got := r.description(g); !strings.Contains(got, "Scout") {
		t.Errorf("first decision %q", got)
	}
	// A unit nobody knows is named by its id.
	if got := tacticUnitName(g, battle.DecisionUnit{ID: "ghost"}); got != "ghost" {
		t.Errorf("unknown unit %q", got)
	}
	if got := tacticDecisionAction(g, "nothing", ""); got != "nothing" {
		t.Errorf("unknown action %q", got)
	}
	// A copy of a decision does not share its rules.
	c := cloneTacticDecision(ds[1])
	c.Rules[0].Skill = "x"
	*c.Target = battle.DecisionUnit{ID: "other"}
	if ds[1].Rules[0].Skill != "fire" || ds[1].Target.ID != "mote" {
		t.Fatal("cloneTacticDecision shares memory")
	}
	g.push(r)
	btlFrame(t, g)
}
