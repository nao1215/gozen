package battle

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/text"
)

// actionTactic は採用した行の控え。先決めの防御も行動するまで表示しない。
type actionTactic struct {
	row       int
	condition string
	selected  int
	results   []RuleEvaluation
}

func (b *Battle) ruleNumber(u *Unit, index int) int {
	if index < 0 {
		return 0
	}
	if u.tactics != nil && u.tactics.manual {
		return index + 1
	}
	// おまかせの編集画面は未習得・OFF の行を除いた順で表示する。
	n := 0
	for i, r := range b.Rules(u) {
		if i > index {
			break
		}
		if u.tactics != nil && u.tactics.reasons[i] != "" || r.Use != "" && !b.Knows(u, r.Use) {
			continue
		}
		n++
	}
	return n
}

func (b *Battle) conditionLabel(u *Unit, index int) string {
	if index < 0 {
		return lang.T("成立行なし")
	}
	if u.tactics != nil && u.tactics.labels[index] != "" {
		return u.tactics.labels[index]
	}
	r := b.Rules(u)[index]
	var parts []string
	if r.When.Set() {
		label := r.When.String()
		// 旧 AI の同じ式もカタログの短い名前で表示する。式や抽選は実行しない。
		for _, id := range slices.Sorted(maps.Keys(b.DB.TacticConditions)) {
			c := b.DB.TacticConditions[id]
			if c.Kind == "template" && c.When.String() == r.When.String() {
				label = c.Name
				break
			}
		}
		parts = append(parts, label)
	}
	if r.Target.Where.Set() {
		parts = append(parts, r.Target.Where.String())
	}
	if len(parts) == 0 {
		return lang.T("いつでも")
	}
	return strings.Join(parts, " / ")
}

func (b *Battle) tacticLog(a Action, targets []*Unit, action string) string {
	var names []string
	for _, t := range targets {
		if !slices.Contains(names, t.Name) {
			names = append(names, t.Name)
		}
	}
	to := strings.Join(names, "・")
	if to == "" {
		to = lang.T("対象なし")
	}
	if a.tactic.row == 0 {
		return fmt.Sprintf(lang.T("［%s・%s → %s・%s］"), a.User.Name, a.tactic.condition, to, action)
	}
	return fmt.Sprintf(lang.T("［%sの作戦%d・%s → %s・%s］"), a.User.Name, a.tactic.row, a.tactic.condition, to, action)
}

// battleAdvice は実際に観測した問題だけを保持する。戦術の良し悪しや敗因を推測しない。
type battleAdvice struct {
	reason string
	actor  string
	row    int
	rank   int
}

func (b *Battle) noteAdvice(reason string, u *Unit, row, rank int) {
	if b.advice.rank >= rank {
		return
	}
	b.advice = battleAdvice{reason: reason, row: row, rank: rank}
	if u != nil {
		b.advice.actor = u.Name
	}
}

func (b *Battle) observeTactics(u *Unit, results []RuleEvaluation, selected int) {
	rules := b.Rules(u)
	selectedCare := selected >= 0 && b.careNeeded(u, rules[selected], results[selected].Target)
	for _, r := range results {
		// 先頭優先で選択済みの行より後の不足は、今回の行動を妨げていない。
		before := selected < 0 || r.Index < selected || !b.gambit(u)
		if before && r.Reason == RuleMPShortage && b.Knows(u, r.Skill) {
			b.noteAdvice("mp_shortage", u, b.ruleNumber(u, r.Index), 4)
		}
		if before && r.Reason == RuleItemShortage {
			b.noteAdvice("item_shortage", u, b.ruleNumber(u, r.Index), 4)
		}
		if !selectedCare && r.Reason == RuleApplicable && r.Index != selected &&
			(r.Selection == "later_rule" || r.Selection == "lower_priority") && b.careNeeded(u, rules[r.Index], r.Target) {
			b.noteAdvice("healing_delayed", u, b.ruleNumber(u, r.Index), 3)
		}
	}
}

// careNeeded は成立済みの回復候補が傷ついた味方へ届く場合だけを数える。
// 効果の条件式を再実行せず、条件付き効果や MP 回復を HP 回復と断定しない。
func (b *Battle) careNeeded(u *Unit, r data.Rule, target *DecisionUnit) bool {
	use := b.DB.RuleUsage(r)
	if use == nil {
		return false
	}
	for _, e := range use.Effects {
		if e.When.Set() || e.Stat == "mp" || !e.Heal.Set() && !e.Revive.Set() {
			continue
		}
		for _, t := range u.allies() {
			reached := use.Scope == "all_allies" || use.Scope == "all_dead_allies" || use.Scope == "self" && t == u ||
				(use.Scope == "one_ally" || use.Scope == "dead_ally") && target != nil && !target.Enemy && target.Index == t.Index
			if reached && (e.Heal.Set() && t.Alive() && t.HP < t.Stat("mhp") || e.Revive.Set() && !t.Alive()) {
				return true
			}
		}
	}
	return false
}

func (b *Battle) observeFalls(steps []Step) {
	for _, s := range steps {
		if s.Kind == Fall && s.Target != nil && !s.Target.Enemy {
			b.noteAdvice("fallen", s.Target, 0, 2)
		}
	}
}

func (b *Battle) saveAdvice() {
	if b.Result == Ongoing {
		return
	}
	for _, u := range b.Party {
		if !u.Alive() {
			b.noteAdvice("fallen", u, 0, 2)
			continue
		}
		if u.MP == 0 {
			for i, r := range b.Rules(u) {
				if u.tactics != nil && u.tactics.reasons[i] != "" {
					continue
				}
				if sk := b.DB.Skills[r.Use]; sk != nil && sk.MP > 0 && b.Knows(u, r.Use) {
					b.noteAdvice("mp_empty", u, b.ruleNumber(u, i), 1)
					break
				}
			}
		}
	}
	if b.Result == Lost {
		if b.advice.rank <= 2 {
			b.advice = battleAdvice{reason: "lost", rank: 2}
		}
	}
	if b.advice.reason == "" {
		return // 問題のない勝利で、酒場へ持ち帰る記録を消さない。
	}
	b.St.Vars["battle_advice_reason"] = b.advice.reason
	b.St.Vars["battle_advice_actor"] = b.advice.actor
	b.St.Vars["battle_advice_row"] = float64(b.advice.row)
}
