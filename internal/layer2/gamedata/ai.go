package gamedata

import (
	"fmt"

	lang "github.com/nao1215/gozen/text"
)

// RuleUsage は規則が参照する技・品の効果を返す。不正・削除済み・使用不可なら nil。
// 読み込みの検査と、編集後の規則を使う実行側で同じ参照解決を使う。
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

// CheckRules は単独の規則一覧にも、DB.Check と同じ参照・式・対象の検査を適用する。
// 戦況や習得状況は評価しない。位置のない規則は rules からの論理パスを返す。
func (db *DB) CheckRules(rules []Rule) []error {
	c := &checker{db: db}
	c.numericValues(rules, "rules")
	c.rules("rules", rules)
	return sortedDiagnostics(c.errs)
}

// AITargetIsGroup は AI の対象選択をせず、効果範囲側で対象を決める scope かを返す。
func AITargetIsGroup(scope string) bool {
	switch scope {
	case "all_enemies", "all_allies", "all_dead_allies", "everyone", "none":
		return true
	}
	return false
}

// RuleWarnings は受け入れ可能だが、指定の一部が使われない規則を診断する。
// 旧仕様の戦い方を変えないため、CheckRules のエラーとは区別する。
func (db *DB) RuleWarnings(rules []Rule) []error {
	c := &checker{db: db}
	c.ruleWarnings("rules", rules)
	return sortedDiagnostics(c.errs)
}

// AIWarnings は名前付き・職業・仲間・敵へ直接書いた規則の警告を元の位置付きで返す。
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
				c.errorf(w, lang.T("scope %s では target の全項目を無視する。発動条件は when に書く"), use.Scope)
			}
			continue
		}
		if use.Scope == "random_enemy" && specified {
			c.errorf(w, "%s", lang.T("random_enemy の target は発動可否だけに使う。pick・by は評価せず、命中先は全生存敵から毎回抽選する"))
		}
		if use.Scope == "self" && specified {
			c.errorf(w, "%s", lang.T("self の target は発動可否だけに使い、効果は常に自分へ適用する"))
		}
		if t.Side == "foes" && (use.Scope == "one_ally" || use.Scope == "dead_ally") {
			c.errorf(w+".side", lang.T("scope %s では side foes を無視する"), use.Scope)
		}
		if t.By.Set() && t.Pick != "min" && t.Pick != "max" {
			c.errorf(w+".by", "%s", lang.T("by は pick min または max のときだけ評価する"))
		}
	}
}
