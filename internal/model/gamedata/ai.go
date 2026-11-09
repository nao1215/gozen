package gamedata

import (
	"fmt"

	"github.com/nao1215/gozen/internal/diag"

	lang "github.com/nao1215/gozen/internal/text"
)

// RuleUsage returns the effect of the skill or item a rule refers to. It returns nil if invalid, deleted or unusable.
// Loading checks and the runtime that uses edited rules share the same reference resolution.
func (db *DB) RuleUsage(r Rule) *Usage {
	if (r.Use == "") == (r.Item == "") {
		return nil
	}
	if r.Use != "" {
		if s := db.Skills[r.Use]; s != nil {
			return &s.Usage
		}
	} else if it := db.Items[r.Item]; it != nil {
		return it.Use
	}
	return nil
}

// CheckRules applies the same reference, expression and target checks as DB.Check to a standalone rule list too.
// It does not evaluate the battle situation or learning state. Rules without a position return a logical path from rules.
func (db *DB) CheckRules(rules []Rule) []error {
	c := &checker{db: db}
	c.numericValues(rules, "rules")
	c.rules("rules", rules)
	return sortedDiagnostics(c.errs)
}

// AITargetIsGroup reports whether the scope decides targets on the effect-range side rather than through the AI's target selection.
func AITargetIsGroup(scope string) bool {
	switch scope {
	case "all_enemies", "all_allies", "all_dead_allies", "everyone", "none":
		return true
	}
	return false
}

// RuleWarnings diagnoses rules that are acceptable but have part of their specification unused.
// It is kept separate from CheckRules errors so that the behavior of the old spec is not changed.
func (db *DB) RuleWarnings(rules []Rule) []error {
	c := &checker{db: db}
	c.ruleWarnings("rules", rules)
	return sortedDiagnostics(c.errs)
}

// AIWarnings returns warnings, with source positions, for rules written directly in named AIs, jobs, party members and enemies.
func (db *DB) AIWarnings() []error {
	c := &checker{db: db}
	for _, id := range db.Ordered("ai") {
		c.ruleWarnings("ai."+id, db.AI[id])
	}
	for _, id := range db.Ordered("classes") {
		c.ruleWarnings("classes."+id+".ai", db.Classes[id].AI.Rules)
	}
	for _, id := range db.Ordered("actors") {
		c.ruleWarnings("actors."+id+".ai", db.Actors[id].AI.Rules)
	}
	for _, id := range db.Ordered("enemies") {
		c.ruleWarnings("enemies."+id+".ai", db.Enemies[id].AI.Rules)
	}
	return sortedDiagnostics(c.errs)
}

func (c *checker) ruleWarnings(where string, rules []Rule) {
	for i, r := range rules {
		use := c.db.RuleUsage(r)
		if use == nil {
			continue
		}
		w := fmt.Sprintf("%s[%d].target", where, i)
		t := r.Target
		specified := t.Preset != "" || t.Side != "" || t.Where.Set() || t.Pick != "" || t.By.Set()
		if AITargetIsGroup(use.Scope) {
			if specified {
				c.errorf(diag.RuleTargetIgnored, w, lang.T("scope %s では target の全項目を無視する。発動条件は when に書く"), use.Scope)
			}
			continue
		}
		if use.Scope == "random_enemy" && specified {
			c.errorf(diag.RuleTargetIgnored, w, "%s", lang.T("random_enemy の target は発動可否だけに使う。pick・by は評価せず、命中先は全生存敵から毎回抽選する"))
		}
		if use.Scope == "self" && specified {
			c.errorf(diag.RuleTargetIgnored, w, "%s", lang.T("self の target は発動可否だけに使い、効果は常に自分へ適用する"))
		}
		if t.Side == "foes" && (use.Scope == "one_ally" || use.Scope == "dead_ally") {
			c.errorf(diag.RuleTargetIgnored, w+".side", lang.T("scope %s では side foes を無視する"), use.Scope)
		}
		if t.By.Set() && t.Pick != "min" && t.Pick != "max" {
			c.errorf(diag.RuleTargetIgnored, w+".by", "%s", lang.T("by は pick min または max のときだけ評価する"))
		}
	}
}
