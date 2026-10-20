package battle

// RuleReason is the result at the place actually evaluated. Later unevaluated items are not
// guessed.
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

// DecisionTiming is the time of evaluation, distinguished from the expression's phase.
type DecisionTiming string

const (
	DecisionDirect    DecisionTiming = "direct"
	DecisionTurnStart DecisionTiming = "turn_start"
	DecisionAction    DecisionTiming = "action"
)

// DecisionUnit is the identifier at decision time. Enemies of the same kind are told apart by
// Index within their side.
// It holds no mutable Unit or DB reference, so its value is kept even if the battle changes after
// recording.
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

// RuleEvaluation is the result of evaluating one line. Selection is set only at selection time.
// Target is the single target chosen as a candidate, not the actual range of the effect.
type RuleEvaluation struct {
	Index     int           `json:"index"`
	Skill     string        `json:"skill,omitempty"`
	Item      string        `json:"item,omitempty"`
	Reason    RuleReason    `json:"reason"`
	Detail    string        `json:"detail,omitempty"`
	Target    *DecisionUnit `json:"target,omitempty"`
	Selection string        `json:"selection,omitempty"`
}

// Decision is the record of the decision from one actual evaluation. If Selected is -1, it is the
// fallback to a normal attack.
// A candidate at turn start where Committed is false is chosen again at action time (and once is
// not consumed).
// Do not call Candidates, EvaluateRules or Decide again for recording.
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
