package rpg

import (
	"fmt"
	"slices"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/text"
)

// TacticsVersion は仲間別の作戦の保存形式版。セーブの包みや YAML の仕様版とは独立。
const TacticsVersion = 1

const (
	TacticsAuto   = "auto"
	TacticsManual = "manual"
	TacticsPreset = "preset"
)

// ActorTactics は仲間ごとの確定済み作戦。manual の空行列も auto と区別する。
// 自由な式や表示名は保存せず、行の順序・安定 ID・閾値・有効状態を保持する。
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

// TacticsDraft は編集用の独立した控え。取消・未確定の内容は保存や戦闘へ漏れない。
func (a *Actor) TacticsDraft() ActorTactics {
	if a.Tactics == nil {
		return ActorTactics{Version: TacticsVersion, Mode: TacticsAuto}
	}
	return a.Tactics.clone()
}

// SetTactics は形式を確かめて控えを確定する。以降の下書きの変更は仲間に影響しない。
// 消えた ID や未習得技の行も残し、CompileTactics で行ごとの理由を取得する。
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

// CompiledTactic は保存行またはおまかせ定義と同じ添字の変換結果。無効・不正な行も詰めない。
type CompiledTactic struct {
	Condition string // 戦闘開始時の表示用条件。式を再評価しない。
	Rule      data.Rule
	Enabled   bool
	Errors    []error
}

// CompileTactics は現在の習得・装備とカタログで検査する。保存した行は書き換えない。
// おまかせの未習得行も実行用には残し、従来の when・chance の乱数評価順を保つ。
// おまかせ未定義は nil、手動またはおまかせの空は非 nil を返す。式と乱数は評価しない。
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
			// 未習得の可否判定は戦闘側で行う。敵用の技を選択肢へ公開しない。
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

// autoTacticTemplate は既存 AI と同じ仲間自身→職業の優先順で選ぶ。
// 名前付き AI に設定がない作品・直接記述の AI は既存の規則を使う。
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

// AutoTactics は現在使える技のおまかせ行を順番どおりに複写する表示・編集用の控え。
// MP・所持品・仲間構成の条件は戦況で評価するため、所持数０の品の行も保持する。
// 手動の確定行・SP・乱数・所持品は変更しない。未定義なら nil を返す。
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

// checkSimpleTactics は習得前の行を保持しつつ、簡易編集の形だけを保証する。
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

// TacticRows は型も含めた下書きの行。簡易作戦は未習得行も表示して行番号を揃える。
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
