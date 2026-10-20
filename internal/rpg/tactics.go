package rpg

import (
	"fmt"
	"slices"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

// TacticsVersion is the storage format version of per-member tactics. It is independent of the save wrapper and the YAML spec version.
const TacticsVersion = 1

const (
	TacticsAuto   = "auto"
	TacticsManual = "manual"
	TacticsPreset = "preset"
)

// ActorTactics is the committed tactics of one party member. An empty manual row list is distinguished from auto.
// Free-form expressions and display names are not stored; it keeps the row order, stable IDs, thresholds and enabled state.
type ActorTactics struct {
	Preset  string           `json:"preset,omitempty"`
	Version int              `json:"version"`
	Mode    string           `json:"mode"`
	Rows    []data.TacticRow `json:"rows"`
}

func (t ActorTactics) check() error {
	if t.Version != TacticsVersion {
		return fmt.Errorf(lang.T("作戦形式版 %d には対応していない（対応版 %d）"), t.Version, TacticsVersion)
	}
	if t.Mode != TacticsAuto && t.Mode != TacticsManual && t.Mode != TacticsPreset {
		return fmt.Errorf(lang.T("未知の作戦モード %q です"), t.Mode)
	}
	if t.Mode != TacticsManual && len(t.Rows) != 0 {
		return fmt.Errorf("%s", lang.T("おまかせの作戦には手動の行を保存できません"))
	}
	if (t.Mode == TacticsPreset) != (t.Preset != "") {
		return fmt.Errorf("%s", lang.T("作戦の型の指定が不正です"))
	}
	return nil
}

func (t ActorTactics) clone() ActorTactics {
	t.Rows = slices.Clone(t.Rows)
	for i := range t.Rows {
		if p := t.Rows[i].Percent; p != nil {
			t.Rows[i].Percent = new(*p)
		}
	}
	return t
}

// TacticsDraft is an independent working copy for editing. Cancelled or uncommitted content does not leak into saves or battles.
func (a *Actor) TacticsDraft() ActorTactics {
	if a.Tactics == nil {
		return ActorTactics{Version: TacticsVersion, Mode: TacticsAuto}
	}
	return a.Tactics.clone()
}

// SetTactics validates the format and commits the draft. Later changes to the draft do not affect the member.
// Rows with vanished IDs or unlearned skills are kept; CompileTactics reports the reason for each row.
func (a *Actor) SetTactics(t ActorTactics) error {
	if err := t.check(); err != nil {
		return err
	}
	if err := a.checkSimpleTactics(t); err != nil {
		return err
	}
	t = t.clone()
	a.Tactics = &t
	return nil
}

// CompiledTactic is the compile result at the same index as the stored row or the auto definition. Invalid or malformed rows are not squeezed out.
type CompiledTactic struct {
	Condition string // condition text for display at battle start; the expression is not re-evaluated
	Rule      data.Rule
	Enabled   bool
	Errors    []error
}

// CompileTactics checks against the current learned skills, equipment and catalog. Stored rows are not rewritten.
// Unlearned auto rows are also kept for execution, preserving the existing evaluation order of when and chance randomness.
// It returns nil for an undefined auto, and non-nil for manual or an empty auto. Expressions and randomness are not evaluated.
func (a *Actor) CompileTactics() []CompiledTactic {
	manual := a.Tactics != nil && a.Tactics.Mode == TacticsManual
	var rows []data.TacticRow
	if manual {
		rows = a.Tactics.Rows
	} else if template, ok := a.tacticTemplate(a.TacticsDraft()); ok {
		rows = template
	} else {
		return nil
	}
	out := make([]CompiledTactic, len(rows))
	skills := a.Skills()
	for i, row := range rows {
		known := skills
		if !manual {
			// Whether an unlearned skill is usable is decided on the battle side. Enemy-only skills are not exposed as choices.
			if action := a.db.TacticActions[row.Action]; action != nil {
				known = []string{action.Use}
			}
		}
		r, errs := a.db.CompileTactic(row, known)
		label := row.Condition
		if c := a.db.TacticConditions[row.Condition]; c != nil {
			label = c.Name
		}
		if row.Percent != nil {
			label += fmt.Sprintf(lang.T("（%d%%）"), *row.Percent)
		}
		out[i] = CompiledTactic{Condition: label, Enabled: row.Enabled, Errors: errs}
		if r != nil {
			out[i].Rule = *r
		}
	}
	return out
}

// autoTacticTemplate picks in the same priority order as the existing AI: the member itself, then the job.
// Games with no setting for the named AI, and directly written AI, use the existing rules.
func (a *Actor) autoTacticTemplate() ([]data.TacticRow, bool) {
	ref := a.Def().AI
	if rows, ok := a.db.Game.Battle.AutoTactics[ref.Name]; ok && ref.Name != "" {
		return rows, true
	}
	if len(a.db.Rules(ref)) == 0 {
		ref = a.Class().AI
	}
	rows, ok := a.db.Game.Battle.AutoTactics[ref.Name]
	return rows, ok && ref.Name != ""
}

// AutoTactics is a display/editing copy that duplicates the auto rows of currently usable skills in order.
// Conditions on MP, inventory and party composition are evaluated in battle, so rows for items with a count of 0 are kept.
// It does not change committed manual rows, SP, randomness or inventory. It returns nil if undefined.
func (a *Actor) AutoTactics() []data.TacticRow {
	rows, ok := a.autoTacticTemplate()
	if !ok {
		return nil
	}
	if a.db.Game.Battle.SimpleTactics {
		return (ActorTactics{Rows: rows}).clone().Rows
	}
	out := make([]data.TacticRow, 0, len(rows))
	skills := a.Skills()
	for _, row := range rows {
		action := a.db.TacticActions[row.Action]
		if action == nil || !row.Enabled {
			continue
		}
		if action.Use != "" && action.Use != a.db.Game.Battle.Attack && action.Use != a.db.Game.Battle.Guard && !slices.Contains(skills, action.Use) {
			continue
		}
		if row.Percent != nil {
			row.Percent = new(*row.Percent)
		}
		out = append(out, row)
	}
	return out
}

// checkSimpleTactics keeps rows for skills not yet learned and only guarantees the shape of the simple editor.
func (a *Actor) checkSimpleTactics(t ActorTactics) error {
	if !a.db.Game.Battle.SimpleTactics {
		return nil
	}
	if len(t.Rows) > data.TacticRowLimit {
		return fmt.Errorf("%s", lang.T("作戦は５行までです"))
	}
	if t.Mode == TacticsPreset {
		if _, ok := a.db.Game.Battle.TacticPresets[t.Preset]; !ok {
			return fmt.Errorf("%s", lang.T("作戦の型が見つかりません"))
		}
	}
	for _, row := range t.Rows {
		known := []string{}
		if action := a.db.TacticActions[row.Action]; action != nil {
			known = append(known, action.Use)
		}
		if _, errs := a.db.CompileTactic(row, known); len(errs) != 0 {
			return errs[0]
		}
	}
	return nil
}

// TacticRows is the draft rows including the preset. Simple tactics also show unlearned rows so the row numbers line up.
func (a *Actor) TacticRows(t ActorTactics) []data.TacticRow {
	if t.Mode == TacticsManual {
		return t.clone().Rows
	}
	rows, _ := a.tacticTemplate(t)
	return (ActorTactics{Rows: rows}).clone().Rows
}

func (a *Actor) tacticTemplate(t ActorTactics) ([]data.TacticRow, bool) {
	if t.Mode != TacticsPreset {
		return a.autoTacticTemplate()
	}
	p, ok := a.db.Game.Battle.TacticPresets[t.Preset]
	if !ok {
		return []data.TacticRow{}, true
	}
	ref := a.Def().AI
	if rows, ok := p.Rows[ref.Name]; ok && ref.Name != "" {
		return rows, true
	}
	if len(a.db.Rules(ref)) == 0 {
		ref = a.Class().AI
	}
	rows, ok := p.Rows[ref.Name]
	if !ok {
		return []data.TacticRow{}, true
	}
	return rows, true
}
