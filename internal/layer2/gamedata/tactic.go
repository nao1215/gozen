package gamedata

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/nao1215/gozen/expr"
	lang "github.com/nao1215/gozen/text"
)

// TacticRow は選択式の作戦１行。表示名や式を保存せず、カタログの安定 ID を参照する。
// Enabled のゼロ値は無効。Percent は閾値条件だけに必要で、nil と０を区別する。
type TacticRow struct {
	Enabled   bool   `yaml:"enabled" json:"enabled"`
	Condition string `yaml:"condition" json:"condition"`
	Percent   *int   `yaml:"percent,omitempty" json:"percent,omitempty"`
	Target    string `yaml:"target,omitempty" json:"target,omitempty"`
	Action    string `yaml:"action" json:"action"`
}

// TacticCondition は条件の選択肢。template の式・抽選・優先度は作者だけが編集する。
// Subject の target は単体候補の絞り込み、self/allies/foes は発動条件を表す。
type TacticCondition struct {
	Name     string  `yaml:"name"`
	Kind     string  `yaml:"kind"`
	Subject  string  `yaml:"subject,omitempty"`
	State    string  `yaml:"state,omitempty"`
	When     Expr    `yaml:"when,omitempty"`
	Chance   Expr    `yaml:"chance,omitempty"`
	Priority int     `yaml:"priority,omitempty"`
	Weight   float64 `yaml:"weight,omitempty"`
	Once     bool    `yaml:"once,omitempty"`
	Say      string  `yaml:"say,omitempty"`
}

// TacticTarget は効果範囲と選び方を結び付ける。Selector の式は編集側に公開しない。
// Scope は Usage.Scope と同じ名前。self だけは one_ally にも使える。
type TacticTarget struct {
	Name     string `yaml:"name"`
	Scope    string `yaml:"scope"`
	Selector Target `yaml:"selector,omitempty"`
}

// TacticRowLimit は簡易作戦の１人あたりの上限。
const TacticRowLimit = 5

// TacticPreset は作者が用意する型。仲間の名前付き AI ごとに５行まで定義する。
type TacticPreset struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Rows        map[string][]TacticRow `yaml:"rows"`
}

// TacticAction は仲間が選べる行動と自動で選ぶ対象。
type TacticAction struct {
	Target string `yaml:"target,omitempty"`
	Name   string `yaml:"name"`
	Use    string `yaml:"use,omitempty"`
	Item   string `yaml:"item,omitempty"`
}

// CheckTacticCatalog は作戦の定義だけを DB.Check と同じ位置付き診断で検査する。
func (db *DB) CheckTacticCatalog() []error {
	c := &checker{db: db}
	c.tacticCatalog()
	return sortedDiagnostics(c.errs)
}

// CompileTactic は行を検査し、既存 AI の１規則へ変換する。無効行は nil を返す。
// skills は仲間の現在の習得技（装備由来を含む）。通常攻撃・防御だけは省ける。
// 不正行は呼び出し側が原文と診断を保持する。DB・習得技・乱数・所持品は変更しない。
func (db *DB) CompileTactic(row TacticRow, skills []string) (*Rule, []error) {
	c := &checker{db: db}
	rule := c.tacticRow(row)
	if row.Enabled && rule.Use != "" && rule.Use != db.Game.Battle.Attack && rule.Use != db.Game.Battle.Guard && !slices.Contains(skills, rule.Use) {
		c.errorf("tactic.action", lang.T("技 %q はこの仲間の習得技ではありません"), rule.Use)
	}
	if len(c.errs) != 0 {
		return nil, sortedDiagnostics(c.errs)
	}
	if !row.Enabled {
		return nil, nil
	}
	return &rule, nil
}

// マップを Go から構築した場合も、Order の追加忘れで検査を飛ばさない。
func tacticIDs[T any](m map[string]T) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (c *checker) tacticCatalog() {
	if c.db.Game.Battle.SimpleTactics && len(c.db.TacticConditions) > 8 {
		c.errorf("tactic_conditions", "%s", lang.T("簡易作戦の条件は８個までです"))
	}
	for _, id := range tacticIDs(c.db.Game.Battle.TacticPresets) {
		p := c.db.Game.Battle.TacticPresets[id]
		w := "game.yaml battle.tactic_presets." + id
		c.tacticName(w, id, p.Name)
		if slices.Contains([]string{"manual", "accept", "cancel"}, id) {
			c.errorf(w, "%s", lang.T("この型IDは画面操作のため予約されています"))
		}
		if c.db.Game.Battle.SimpleTactics {
			for _, ai := range tacticIDs(c.db.Game.Battle.AutoTactics) {
				if _, ok := p.Rows[ai]; !ok {
					c.errorf(w+".rows", lang.T("作戦の型に AI %q の行がありません"), ai)
				}
			}
		}
		for _, ai := range tacticIDs(p.Rows) {
			c.tacticRows(w+".rows."+ai, ai, p.Rows[ai])
		}
	}
	for _, id := range tacticIDs(c.db.TacticConditions) {
		c.tacticCondition(id, c.db.TacticConditions[id])
	}
	for _, id := range tacticIDs(c.db.TacticTargets) {
		c.tacticTarget(id, c.db.TacticTargets[id])
	}
	for _, id := range tacticIDs(c.db.TacticActions) {
		c.tacticAction(id, c.db.TacticActions[id])
	}
	for _, id := range tacticIDs(c.db.Game.Battle.AutoTactics) {
		c.tacticRows("game.yaml battle.auto_tactics."+id, id, c.db.Game.Battle.AutoTactics[id])
	}
}

func (c *checker) tacticRows(w, id string, rows []TacticRow) {
	if _, ok := c.db.AI[id]; !ok || id == "" {
		c.errorf(w, lang.T("おまかせ作戦の参照先 AI %q がありません"), id)
	}
	if c.db.Game.Battle.SimpleTactics && len(rows) > TacticRowLimit {
		c.errorf(w, "%s", lang.T("作戦は５行までです"))
	}
	for i, row := range rows {
		local := &checker{db: c.db}
		local.tacticRow(row)
		for _, err := range local.errs {
			var d *diagnostic
			if !errors.As(err, &d) {
				c.errs = append(c.errs, err)
				continue
			}
			if strings.HasPrefix(d.where, "tactic.") {
				c.errorf(fmt.Sprintf("%s[%d].%s", w, i, strings.TrimPrefix(d.where, "tactic.")), "%s", d.message)
			} else {
				c.errs = append(c.errs, err)
			}
		}
	}
}

func (c *checker) tacticName(w, id, name string) {
	if !gameIDPattern.MatchString(id) {
		c.errorf(w, "%s", lang.T("ID は英数字で始まる英数字・_・- にしてください"))
	}
	if strings.TrimSpace(name) == "" {
		c.errorf(w+".name", "%s", lang.T("表示名が必要です"))
	}
}

func (c *checker) tacticCondition(id string, d *TacticCondition) {
	w := "tactic_conditions." + id
	if d == nil {
		c.errorf(w, "%s", lang.T("条件の定義が必要です"))
		return
	}
	c.tacticName(w, id, d.Name)
	c.expr(w+".when", d.When, BattleNames)
	c.expr(w+".chance", d.Chance, BattleNames)
	c.text(w+".say", d.Say, BattleNames)
	c.numberRange(w+".weight", d.Weight, 0, math.Inf(1))
	if d.Kind == "template" {
		if !d.When.Set() {
			c.errorf(w+".when", "%s", lang.T("おまかせの条件には when が必要です"))
		}
	} else if d.When.Set() || d.Chance.Set() || d.Priority != 0 || d.Weight != 0 || d.Once || d.Say != "" {
		c.errorf(w, "%s", lang.T("固定の式・抽選・優先度・台詞は template だけに指定できます"))
	}
	switch d.Kind {
	case "always", "template":
		if d.Subject != "" || d.State != "" {
			c.errorf(w, "%s", lang.T("この条件には subject・state を指定できません"))
		}
	case "hp_below", "hp_at_most", "mp_below", "mp_at_most", "state", "not_state", "alive", "dead":
		if !slices.Contains([]string{"self", "allies", "foes", "target"}, d.Subject) {
			c.errorf(w+".subject", "%s", lang.T("subject は self・allies・foes・target のいずれかです"))
		}
		if d.Kind == "state" || d.Kind == "not_state" {
			if c.db.States[d.State] == nil {
				c.errorf(w+".state", lang.T("未知の状態 %q です"), d.State)
			}
		} else if d.State != "" {
			c.errorf(w+".state", "%s", lang.T("state は状態条件だけに指定できます"))
		}
	default:
		c.errorf(w+".kind", lang.T("未知の条件の種類 %q です"), d.Kind)
	}
}

func targetSpecified(t Target) bool {
	return t.Preset != "" || t.Side != "" || t.Where.Set() || t.Pick != "" || t.By.Set()
}

func (c *checker) tacticTarget(id string, d *TacticTarget) {
	w := "tactic_targets." + id
	if d == nil {
		c.errorf(w, "%s", lang.T("対象の定義が必要です"))
		return
	}
	c.tacticName(w, id, d.Name)
	if !slices.Contains(Scopes, d.Scope) {
		c.errorf(w+".scope", lang.T("未知の効果範囲 %q です"), d.Scope)
	}
	t := d.Selector
	c.aiTarget(w+".selector", t)
	if t.Preset != "" && (t.Side != "" || t.Where.Set() || t.Pick != "" || t.By.Set()) {
		c.errorf(w+".selector", "%s", lang.T("対象の略記と詳細指定は併用できません"))
	}
	if t.By.Set() && t.Pick != "min" && t.Pick != "max" {
		c.errorf(w+".selector.by", "%s", lang.T("by は min・max だけに指定できます"))
	}
	if AITargetIsGroup(d.Scope) || d.Scope == "random_enemy" || d.Scope == "self" {
		if targetSpecified(t) {
			c.errorf(w+".selector", "%s", lang.T("この効果範囲に単体の絞り込みは使えません。発動条件を指定してください"))
		}
		return
	}
	if d.Scope == "one_enemy" && (t.Side == "allies" || t.Preset == "self" || strings.HasPrefix(t.Preset, "ally_")) ||
		(d.Scope == "one_ally" || d.Scope == "dead_ally") && t.Side == "foes" ||
		d.Scope == "dead_ally" && (t.Preset == "self" || t.Preset == "ally_lowest_hp_rate") ||
		d.Scope == "one_ally" && t.Preset == "ally_dead" {
		c.errorf(w+".selector", "%s", lang.T("効果範囲と対象の側・生死が一致しません"))
	}
}

func (c *checker) tacticAction(id string, d *TacticAction) {
	w := "tactic_actions." + id
	if d == nil {
		c.errorf(w, "%s", lang.T("行動の定義が必要です"))
		return
	}
	c.tacticName(w, id, d.Name)
	if (d.Target != "" || c.db.Game.Battle.SimpleTactics) && c.db.TacticTargets[d.Target] == nil {
		c.errorf(w+".target", lang.T("未知の作戦対象 %q です"), d.Target)
	}
	r := Rule{Use: d.Use, Item: d.Item}
	if (d.Use == "") == (d.Item == "") {
		c.errorf(w, "%s", lang.T("use と item のどちらか一つが必要です"))
		return
	}
	u := c.db.RuleUsage(r)
	if u == nil {
		field, id := "use", d.Use
		if d.Item != "" {
			field, id = "item", d.Item
		}
		c.errorf(w+"."+field, lang.T("行動 %q が存在しないか、使用効果がありません"), id)
		return
	}
	if !slices.Contains(u.Where, "battle") {
		c.errorf(w, "%s", lang.T("戦闘で使える行動だけを指定してください"))
	}
	if target := c.db.TacticTargets[d.Target]; target != nil && u.Scope != target.Scope && (target.Scope != "self" || u.Scope != "one_ally") {
		c.errorf(w+".target", "%s", lang.T("行動と自動対象の効果範囲が一致しません"))
	}
}

func (c *checker) tacticRow(row TacticRow) Rule {
	if c.db.Game.Battle.SimpleTactics {
		if row.Target != "" {
			c.errorf("tactic.target", "%s", lang.T("簡易作戦の対象は行動から自動で決めます"))
		}
		if row.Percent != nil {
			c.errorf("tactic.percent", "%s", lang.T("簡易作戦の条件に数値は指定できません"))
		}
	}
	if row.Target == "" {
		if action := c.db.TacticActions[row.Action]; action != nil {
			row.Target = action.Target
		}
	}
	cond, target, action := c.db.TacticConditions[row.Condition], c.db.TacticTargets[row.Target], c.db.TacticActions[row.Action]
	for _, v := range []struct {
		field, id string
		missing   bool
	}{
		{"condition", row.Condition, cond == nil}, {"target", row.Target, target == nil}, {"action", row.Action, action == nil},
	} {
		if v.missing {
			c.errorf("tactic."+v.field, lang.T("未知の作戦 ID %q です"), v.id)
		}
	}
	if len(c.errs) != 0 {
		return Rule{}
	}
	c.tacticCondition(row.Condition, cond)
	c.tacticTarget(row.Target, target)
	c.tacticAction(row.Action, action)
	if len(c.errs) != 0 {
		return Rule{}
	}
	threshold := strings.HasPrefix(cond.Kind, "hp_") || strings.HasPrefix(cond.Kind, "mp_")
	if threshold && row.Percent == nil {
		c.errorf("tactic.percent", "%s", lang.T("閾値には０〜100の整数が必要です"))
	} else if row.Percent != nil && (!threshold || *row.Percent < 0 || *row.Percent > 100) {
		c.errorf("tactic.percent", "%s", lang.T("閾値は HP・MP 条件の０〜100だけ指定できます"))
	}
	u := c.db.RuleUsage(Rule{Use: action.Use, Item: action.Item})
	if u.Scope != target.Scope && (target.Scope != "self" || u.Scope != "one_ally") {
		c.errorf("tactic.target", lang.T("行動の効果範囲 %s と対象 %s が一致しません"), u.Scope, target.Scope)
	}
	if cond.Subject == "target" {
		if target.Scope != "one_enemy" && target.Scope != "one_ally" && target.Scope != "dead_ally" {
			c.errorf("tactic.condition", "%s", lang.T("対象条件は単体だけに使えます。全体技には allies・foes・self の発動条件を指定してください"))
		}
		if cond.Kind == "dead" && target.Scope != "dead_ally" || cond.Kind == "alive" && target.Scope == "dead_ally" {
			c.errorf("tactic.condition", "%s", lang.T("条件と対象の生死が一致しません"))
		}
		// 略記を詳細指定に変えると既存の抽選の意味を変えうるため、明示的な対象テンプレートを使う。
		if target.Selector.Preset != "" {
			c.errorf("tactic.target", "%s", lang.T("対象条件を足す場合は略記でなく詳細指定の対象を使ってください"))
		}
	}
	if len(c.errs) != 0 {
		return Rule{}
	}
	r := Rule{Use: action.Use, Item: action.Item, Target: target.Selector,
		When: cond.When, Chance: cond.Chance, Priority: cond.Priority, Weight: cond.Weight, Once: cond.Once, Say: cond.Say}
	if target.Scope == "self" && u.Scope == "one_ally" {
		r.Target = Target{Preset: "self"}
	}
	if cond.Kind != "always" && cond.Kind != "template" {
		e, err := expr.Parse(tacticPredicate(cond, row.Percent))
		if err != nil {
			c.errorf("tactic.condition", "%v", err)
			return Rule{}
		}
		if cond.Subject == "target" {
			if r.Target.Where.Set() {
				e, err = expr.Parse("(" + r.Target.Where.String() + ") && (" + e.String() + ")")
			}
			r.Target.Where = Expr{e}
		} else {
			r.When = Expr{e}
		}
		if err != nil {
			c.errorf("tactic.condition", "%v", err)
		}
	}
	if c.db.Game.SchemaVersion == CurrentSchemaVersion && r.Weight == 0 {
		r.Weight = 1
	}
	// 式を実行せずに複写する。戻り値の UnmarshalYAML でカタログの式を上書きできない。
	r.When, r.Chance = copyTacticExpr(r.When), copyTacticExpr(r.Chance)
	r.Target.Where, r.Target.By = copyTacticExpr(r.Target.Where), copyTacticExpr(r.Target.By)
	c.numericValues([]Rule{r}, "tactic.rule")
	c.rules("tactic.rule", []Rule{r})
	return r
}

func copyTacticExpr(e Expr) Expr {
	if e.Set() {
		copy := *e.Expr
		return Expr{&copy}
	}
	return Expr{}
}

func tacticPredicate(d *TacticCondition, percent *int) string {
	subject := d.Subject
	group := subject == "allies" || subject == "foes"
	if subject == "target" {
		subject = "t"
	}
	switch d.Kind {
	case "alive", "dead":
		if group {
			return subject + "." + d.Kind + " > 0"
		}
		return subject + "." + d.Kind
	case "state", "not_state":
		// 状態 ID は式の文字列リテラルとして引用し、ハイフンもそのまま扱う。
		if group {
			with := subject + "[" + strconv.Quote("with_"+d.State) + "]"
			if d.Kind == "not_state" {
				return subject + ".alive > " + with
			}
			return with + " > 0"
		}
		test := subject + ".states[" + strconv.Quote(d.State) + "]"
		if d.Kind == "not_state" {
			test = "!" + test
		}
		return test
	default:
		field := d.Kind[:2] + "_rate"
		op := " < "
		if strings.HasSuffix(d.Kind, "at_most") {
			op = " <= "
		}
		if group {
			// 空集団の min は０になるため、生存者の存在を別に確かめる。
			return fmt.Sprintf("%s.alive > 0 && %s.min_%s%s%d / 100", subject, subject, field, op, *percent)
		}
		return fmt.Sprintf("%s.%s%s%d / 100", subject, field, op, *percent)
	}
}
