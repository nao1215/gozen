package battle

// RuleReason は実際に評価した箇所の結果。後続の未評価項目は推測しない。
type RuleReason string

const (
	RuleApplicable     RuleReason = "applicable"
	RuleOnceUsed       RuleReason = "once_used"
	RuleConditionFalse RuleReason = "condition_false"
	RuleConditionError RuleReason = "condition_error"
	RuleChanceFailed   RuleReason = "chance_failed"
	RuleChanceError    RuleReason = "chance_error"
	RuleMPShortage     RuleReason = "mp_shortage"
	RuleSkillUnlearned RuleReason = "skill_unlearned"
	RuleItemShortage   RuleReason = "item_shortage"
	RuleEnemyItem      RuleReason = "enemy_item"
	RuleInvalidAction  RuleReason = "invalid_action"
	RuleNoTarget       RuleReason = "no_target"
	RuleDisabled       RuleReason = "disabled"
	RuleInvalidTactic  RuleReason = "invalid_tactic"
)

// DecisionTiming は式の phase と区別した評価時点。
type DecisionTiming string

const (
	DecisionDirect    DecisionTiming = "direct"
	DecisionTurnStart DecisionTiming = "turn_start"
	DecisionAction    DecisionTiming = "action"
)

// DecisionUnit は判断時の識別子。同じ種類の敵は陣営内の Index で区別する。
// 可変の Unit や DB の参照を含めず、記録した後に戦況が変わっても値を保つ。
type DecisionUnit struct {
	ID    string `json:"id"`
	Enemy bool   `json:"enemy"`
	Index int    `json:"index"`
}

func decisionUnit(u *Unit) *DecisionUnit {
	if u == nil {
		return nil
	}
	id := u.EID
	if u.Actor != nil {
		id = u.Actor.ID
	}
	return &DecisionUnit{ID: id, Enemy: u.Enemy, Index: u.Index}
}

// RuleEvaluation は１行を評価した結果。Selection は選択時だけ設定する。
// Target は候補として選んだ単体対象で、実際の効果範囲ではない。
type RuleEvaluation struct {
	Index     int           `json:"index"`
	Skill     string        `json:"skill,omitempty"`
	Item      string        `json:"item,omitempty"`
	Reason    RuleReason    `json:"reason"`
	Detail    string        `json:"detail,omitempty"`
	Target    *DecisionUnit `json:"target,omitempty"`
	Selection string        `json:"selection,omitempty"`
}

// Decision は１回の実評価で得た判断の控え。Selected が -1 なら通常攻撃へのフォールバック。
// Committed が false のターン開始時の候補は、行動時に選び直す（once も消費しない）。
// 記録のために Candidates・EvaluateRules・Decide を再度呼ばない。
type Decision struct {
	Troop     string           `json:"troop"`
	Turn      int              `json:"turn"`
	Phase     string           `json:"phase"`
	Timing    DecisionTiming   `json:"timing"`
	Mode      string           `json:"mode"`
	User      DecisionUnit     `json:"user"`
	Rules     []RuleEvaluation `json:"rules"`
	Selected  int              `json:"selected"`
	Committed bool             `json:"committed"`
	Skill     string           `json:"skill,omitempty"`
	Item      string           `json:"item,omitempty"`
	Target    *DecisionUnit    `json:"target,omitempty"`
}
