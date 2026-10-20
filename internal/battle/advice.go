package battle

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	lang "github.com/nao1215/gozen/internal/text"
)

// actionTactic is the record of the adopted row. A pre-decided guard is not displayed until it acts.
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
	// The auto edit screen shows rows in order, excluding unlearned and OFF rows.
	n := 0
	for i, r := range b.rules(u) {
		if i > index {
			break
		}
		if u.tactics != nil && u.tactics.reasons[i] != "" || r.Use != "" && !b.knows(u, r.Use) {
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
	r := b.rules(u)[index]
	var parts []string
	if r.When.Set() {
		label := r.When.String()
		// The same expression of the old AI is also shown with the catalog's short name. Expressions and lotteries are not executed.
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
	to := strings.Join(names, lang.T("・"))
	if to == "" {
		to = lang.T("対象なし")
	}
	if a.tactic.row == 0 {
		return fmt.Sprintf(lang.T("［%s・%s → %s・%s］"), a.User.Name, a.tactic.condition, to, action)
	}
	return fmt.Sprintf(lang.T("［%sの作戦%d・%s → %s・%s］"), a.User.Name, a.tactic.row, a.tactic.condition, to, action)
}

// battleAdvice keeps only problems actually observed. It does not guess whether tactics were good or the cause of defeat.
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
	rules := b.rules(u)
	selectedCare := selected >= 0 && b.careNeeded(u, rules[selected], results[selected].Target)
	for _, r := range results {
		// A shortage after a row already chosen by first-match priority did not hinder this action.
		before := selected < 0 || r.Index < selected || !b.gambit(u)
		if before && r.Reason == RuleMPShortage && b.knows(u, r.Skill) {
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

// careNeeded counts only cases where an established healing candidate reaches a wounded ally.
// It does not re-run the effect's condition expression, and does not assume a conditional effect or MP recovery is HP healing.
func (b *Battle) careNeeded(u *Unit, r gamedata.Rule, target *DecisionUnit) bool {
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
			for i, r := range b.rules(u) {
				if u.tactics != nil && u.tactics.reasons[i] != "" {
					continue
				}
				if sk := b.DB.Skills[r.Use]; sk != nil && sk.MP > 0 && b.knows(u, r.Use) {
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
		return // a problem-free win must not erase the record taken back to the tavern
	}
	b.St.Vars["battle_advice_reason"] = b.advice.reason
	b.St.Vars["battle_advice_actor"] = b.advice.actor
	b.St.Vars["battle_advice_row"] = float64(b.advice.row)
}
