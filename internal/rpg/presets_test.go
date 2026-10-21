package rpg_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

const simpleCatalog = `
tactic_conditions:
  any: {name: Always, kind: always}
  low: {name: Hurt, kind: hp_below, subject: self}
tactic_targets:
  enemy: {name: Opponent, scope: one_enemy}
  self: {name: Self, scope: self}
tactic_actions:
  attack: {name: Strike, use: attack, target: enemy}
  guard: {name: Brace, use: guard, target: self}
  fire: {name: Spark, use: fire, target: enemy}
ai:
  capability:
    - {use: fire}
    - {use: attack}
`

func presetGame(simple bool) string {
	game := testgame.Game + `
  tactic_presets:
    careful:
      name: Careful
      rows:
        capability:
          - {enabled: true, condition: any, action: guard}
          - {enabled: true, condition: any, action: attack}
    empty: {name: Empty, rows: {capability: [{enabled: true, condition: any, action: attack}]}}
  auto_tactics:
    capability:
      - {enabled: true, condition: any, action: fire}
      - {enabled: true, condition: any, action: attack}
`
	if simple {
		game = strings.Replace(game, "battle:\n", "battle:\n  simple_tactics: true\n", 1)
	}
	return game
}

func presetState(t *testing.T, simple bool) (*rpg.State, *rpg.Actor) {
	t.Helper()
	db := testgame.Load(t, map[string]string{
		"game.yaml":         presetGame(simple),
		"data/tactics.yaml": simpleCatalog,
	})
	db.Classes["ranger"].AI = data.AIRef{Name: "capability"}
	st := testgame.State(db, 3)
	return st, st.Actors["scout"]
}

func actions(rows []data.TacticRow) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Action
	}
	return strings.Join(out, ",")
}

// TestPresetTacticsGiveTheirRowsOfTheMembersAI: a preset is a set of rows per AI; a member whose AI it does not name gets none.
func TestPresetTacticsGiveTheirRowsOfTheMembersAI(t *testing.T) {
	_, scout := presetState(t, false)
	careful := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsPreset, Preset: "careful"}
	if err := scout.SetTactics(careful); err != nil {
		t.Fatal(err)
	}
	if got := actions(scout.TacticRows(careful)); got != "guard,attack" {
		t.Errorf("the careful preset: %q", got)
	}
	empty := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsPreset, Preset: "empty"}
	if got := actions(scout.TacticRows(empty)); got != "attack" {
		t.Errorf("the empty preset: %q", got)
	}
	ghost := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsPreset, Preset: "ghost"}
	if got := scout.TacticRows(ghost); got == nil || len(got) != 0 {
		t.Errorf("an unknown preset gives an empty list, not nil: %#v", got)
	}
	auto := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto}
	if got := actions(scout.TacticRows(auto)); got != "fire,attack" {
		t.Errorf("auto rows are the game's rows for the member's AI: %q", got)
	}
	manual := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "any", Target: "self", Action: "guard"}}}
	rows := scout.TacticRows(manual)
	rows[0].Action = "attack"
	if manual.Rows[0].Action != "guard" {
		t.Error("TacticRows hands out a copy")
	}
	// the compiled form of a preset follows the preset
	scout.Tactics = &careful
	if got := scout.CompileTactics(); len(got) != 2 {
		t.Errorf("compiled %d rows, want the preset's 2", len(got))
	}
}

// TestSimpleTacticsKeepFiveRows: with simple tactics a member has at most five rows, rows are checked in shape only
// (an unlearned skill stays), and a preset must exist.
func TestSimpleTacticsKeepFiveRows(t *testing.T) {
	_, scout := presetState(t, true)
	row := data.TacticRow{Enabled: true, Condition: "any", Action: "attack"}
	six := make([]data.TacticRow, 6)
	for i := range six {
		six[i] = row
	}
	if err := scout.SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: six}); err == nil {
		t.Error("six rows were accepted")
	}
	if err := scout.SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: six[:data.TacticRowLimit]}); err != nil {
		t.Errorf("five rows: %v", err)
	}
	if err := scout.SetTactics(rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsPreset, Preset: "ghost"}); err == nil {
		t.Error("simple tactics need an existing preset")
	}
	bad := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "nothing", Action: "attack"}}}
	if err := scout.SetTactics(bad); err == nil {
		t.Error("a row naming a condition the game does not have")
	}
	if got := actions(scout.AutoTactics()); got != "fire,attack" {
		t.Errorf("with simple tactics the auto rows show every row, learned or not: %q", got)
	}
}

// TestAnOldSaveIsMovedToSimpleTactics: a save made with a target written on each row loads into a game with simple tactics
// with the targets dropped (the action decides it) and the change reported; rows that cannot be read go back to auto.
func TestAnOldSaveIsMovedToSimpleTactics(t *testing.T) {
	st, scout := presetState(t, false) // the same game before it turned simple
	scout.Tactics = &rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "any", Target: "enemy", Action: "attack"}}}
	b, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	simpleDB := st.DB
	simpleDB.Game.Battle.SimpleTactics = true
	got, err := rpg.Parse(simpleDB, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := got.Actors["scout"].Tactics.Rows
	if len(rows) != 1 || rows[0].Target != "" || rows[0].Action != "attack" {
		t.Errorf("rows %+v", rows)
	}
	found := false
	for _, c := range got.LoadReport.Changes {
		found = found || strings.HasSuffix(c.Path, "/tactics")
	}
	if !found || !got.LoadReport.Changed() {
		t.Errorf("the change is reported: %+v", got.LoadReport)
	}
	// a row the simple editor cannot read puts the member back to auto
	scout.Tactics = &rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{{Enabled: true, Condition: "gone", Action: "attack"}}}
	b, err = st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err = rpg.Parse(simpleDB, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := got.Actors["scout"].Tactics.Mode; m != rpg.TacticsAuto {
		t.Errorf("mode %q, want auto", m)
	}
}
