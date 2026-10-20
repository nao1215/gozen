package battle

import (
	"cmp"
	"slices"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// rules returns the AUTO rules in order: the enemy itself, the tactic set at battle start, the ally itself, then the job.
// Empty and invalid rows are kept, and the shared DB rules are never modified.
func (b *Battle) rules(u *Unit) []data.Rule {
	if u.Def != nil {
		return b.DB.Rules(u.Def.AI)
	}
	if u.tactics != nil {
		return u.tactics.rules
	}
	if r := b.DB.Rules(u.Actor.Def().AI); len(r) > 0 {
		return r
	}
	return b.DB.Rules(u.Actor.Class().AI)
}

// knows reports whether u may use the skill: a party member only the skills they have
// learned (and the ATTACK and GUARD skills), an enemy any skill.
func (b *Battle) knows(u *Unit, skill string) bool {
	if u.Actor == nil || skill == b.DB.Game.Battle.Attack || skill == b.DB.Game.Battle.Guard {
		return true
	}
	if u.tactics != nil {
		return slices.Contains(u.tactics.skills, skill)
	}
	return slices.Contains(u.Actor.Skills(), skill)
}

// candidate is a rule that holds now, with the target it would take.
type candidate struct {
	Index  int
	Rule   *data.Rule
	Target *Unit
}

// candidates returns the rules that currently hold, with their targets. It runs expressions and lotteries, so it is not re-run for explanations.
func (b *Battle) candidates(u *Unit) []candidate {
	return b.evaluateRules(u, nil)
}

// explainRules performs the same evaluation as Candidates exactly once and also returns each row's result.
// It does not consume once, but expressions and lotteries advance the RNG. Use OnDecision to explain a decision already made.
func (b *Battle) explainRules(u *Unit) ([]candidate, []RuleEvaluation) {
	var results []RuleEvaluation
	cands := b.evaluateRules(u, &results)
	return cands, results
}

func (b *Battle) evaluateRules(u *Unit, results *[]RuleEvaluation) []candidate {
	rules := b.rules(u)
	env := b.Env(u)
	var out []candidate
	for i := range rules {
		r := &rules[i]
		candidate := candidate{Index: i, Rule: r}
		var reason RuleReason
		var detail string
		if u.tactics != nil && u.tactics.reasons[i] != "" {
			reason, detail = u.tactics.reasons[i], u.tactics.details[i]
		} else {
			candidate, reason, detail = b.ruleApplicability(u, i, r, env)
		}
		if results != nil {
			*results = append(*results, RuleEvaluation{
				Index: i, Skill: r.Use, Item: r.Item,
				Reason: reason, Detail: detail, Target: decisionUnit(candidate.Target),
			})
		}
		if reason == RuleApplicable {
			out = append(out, candidate)
		}
	}
	return out
}

// ruleApplicability keeps the existing order once -> when -> chance -> MP -> learned -> item -> target.
// It returns only the first reason that stopped the rule, without re-evaluating expressions or drawing extra lotteries for investigation.
func (b *Battle) ruleApplicability(u *Unit, i int, r *data.Rule, env expr.Env) (candidate, RuleReason, string) {
	c := candidate{Index: i, Rule: r}
	if r.Once && u.fired[i] {
		return c, RuleOnceUsed, ""
	}
	if ok, err := r.When.Bool(env); err != nil {
		return c, RuleConditionError, err.Error()
	} else if !ok {
		return c, RuleConditionFalse, ""
	}
	if r.Chance.Set() {
		p, err := r.Chance.Number(env)
		if err != nil {
			return c, RuleChanceError, err.Error()
		}
		if b.Rand.Float64() >= p {
			return c, RuleChanceFailed, ""
		}
	}
	use := b.DB.RuleUsage(*r)
	if use == nil {
		return c, RuleInvalidAction, ""
	}
	if r.Use != "" {
		if u.MP < b.DB.Skills[r.Use].MP {
			return c, RuleMPShortage, ""
		}
		if !b.knows(u, r.Use) {
			return c, RuleSkillUnlearned, ""
		}
	} else {
		if u.Enemy {
			return c, RuleEnemyItem, ""
		}
		if b.St.Items[r.Item] <= 0 {
			return c, RuleItemShortage, ""
		}
	}
	t, ok := b.pickTarget(u, use.Scope, r.Target)
	if !ok {
		return c, RuleNoTarget, ""
	}
	c.Target = t
	return c, RuleApplicable, ""
}

// Decide picks an action from the rules that hold and consumes the chosen once.
// Manual tactics and gambits come first; otherwise it draws by weight among the highest priority. With no candidates it uses a normal attack.
func (b *Battle) Decide(u *Unit) Action {
	return b.decide(u, DecisionDirect)
}

func (b *Battle) decide(u *Unit, timing DecisionTiming) Action {
	var results []RuleEvaluation
	var record *[]RuleEvaluation
	// Ally advice also uses the actual evaluation result, avoiding a re-evaluation just for observation.
	if !u.Enemy || b.OnDecision != nil {
		record = &results
	}
	cands := b.evaluateRules(u, record)
	a := Action{User: u, Skill: b.DB.Game.Battle.Attack}
	selected := -1
	if len(cands) > 0 {
		pick := cands[0]
		if !b.gambit(u) {
			top := slices.MaxFunc(cands, func(x, y candidate) int { return cmp.Compare(x.Rule.Priority, y.Rule.Priority) }).Rule.Priority
			cands = slices.DeleteFunc(cands, func(c candidate) bool {
				if c.Rule.Priority != top {
					if record != nil {
						results[c.Index].Selection = "lower_priority"
					}
					return true
				}
				return false
			})
			total := 0.0
			for _, c := range cands {
				total += weight(c.Rule)
			}
			x := b.Rand.Float64() * total
			pick = cands[len(cands)-1]
			for _, c := range cands {
				if x -= weight(c.Rule); x < 0 {
					pick = c
					break
				}
			}
		}
		selected = pick.Index
		a = Action{User: u, Skill: pick.Rule.Use, Item: pick.Rule.Item, Target: pick.Target, Say: pick.Rule.Say}
		if record != nil {
			for _, c := range cands {
				selection := "weight_not_selected"
				if b.gambit(u) {
					selection = "later_rule"
				}
				if c.Index == selected {
					selection = "selected"
				}
				results[c.Index].Selection = selection
			}
		}
	}
	committed := timing != DecisionTurnStart || b.earlyAction(a)
	if !u.Enemy {
		a.tactic = &actionTactic{row: b.ruleNumber(u, selected), condition: b.conditionLabel(u, selected), selected: selected, results: results}
	}
	if selected >= 0 && committed {
		u.fired[selected] = true
	}
	if b.OnDecision != nil {
		// Keep the advice check at action time unaffected even if the notification target edits the reserve.
		reported := slices.Clone(results)
		for i := range reported {
			if reported[i].Target != nil {
				reported[i].Target = new(*reported[i].Target)
			}
		}
		mode := "priority_weight"
		if b.gambit(u) {
			mode = "gambit"
		}
		b.OnDecision(Decision{
			Troop: b.TroopID, Turn: b.Turn, Phase: b.phase, Timing: timing,
			Mode: mode, User: *decisionUnit(u), Rules: reported, Selected: selected,
			Committed: committed, Skill: a.Skill, Item: a.Item, Target: decisionUnit(a.Target),
		})
	}
	return a
}

// earlyAction reports whether the skill is a defend or speed-specified one fixed at turn start. It does not evaluate expressions.
func (b *Battle) earlyAction(a Action) bool {
	return a.Item == "" && a.Skill != "" && (a.Skill == b.DB.Game.Battle.Guard || b.usage(&a) != nil && b.usage(&a).Speed.Set())
}

func weight(r *data.Rule) float64 {
	if r.Weight <= 0 {
		return 1
	}
	return r.Weight
}

// pickTarget chooses the target of a rule. Scopes reaching a whole side need none (nil,
// true) but still need someone there; for one_* scopes the target spec picks among the
// side the scope names.
func (b *Battle) pickTarget(u *Unit, scope string, t data.Target) (*Unit, bool) {
	if data.AITargetIsGroup(scope) {
		switch scope {
		case "all_enemies":
			return nil, len(living(u.foes())) > 0
		case "all_dead_allies":
			return nil, len(fallen(u.allies())) > 0
		default:
			return nil, true
		}
	}
	var pool []*Unit
	switch scope {
	case "one_enemy", "random_enemy":
		pool = living(u.foes())
	case "one_ally":
		pool = living(u.allies())
	case "self":
		pool = []*Unit{u}
	case "dead_ally":
		pool = fallen(u.allies())
	}
	spec := preset(t)
	switch spec.Side {
	case "foes":
		if scope == "one_ally" || scope == "dead_ally" {
			break // a healing skill cannot be aimed at the foes
		}
		pool = living(u.foes())
	case "allies":
		if scope == "one_enemy" {
			pool = living(u.allies())
		}
	}
	if spec.self {
		pool = []*Unit{u}
	}
	env := b.Env(u)
	if spec.Where.Set() {
		pool = slices.DeleteFunc(slices.Clone(pool), func(c *Unit) bool {
			ok, err := spec.Where.Bool(expr.Chain{expr.Map{"t": c}, env})
			return err != nil || !ok
		})
	}
	if len(pool) == 0 {
		return nil, false
	}
	if scope == "random_enemy" {
		return nil, true
	}
	switch spec.Pick {
	case "min", "max":
		var best *Unit
		var bk float64
		for _, c := range pool {
			k, err := spec.By.Number(expr.Chain{expr.Map{"t": c}, env})
			if err != nil {
				continue
			}
			if best == nil || (spec.Pick == "min" && k < bk) || (spec.Pick == "max" && k > bk) {
				best, bk = c, k
			}
		}
		return best, best != nil
	case "first":
		return pool[0], true
	}
	if u.Enemy && scope == "one_enemy" && !slices.Contains(u.allies(), pool[0]) {
		return b.frontFirst(pool), true
	}
	return pool[b.Rand.IntN(len(pool))], true
}

type targetSpec struct {
	data.Target
	self bool
}

// preset turns a named target into its spec.
func preset(t data.Target) targetSpec {
	s := targetSpec{Target: t}
	by := func(src string) data.Expr {
		e, err := expr.Parse(src)
		if err != nil {
			return data.Expr{}
		}
		return data.Expr{Expr: e}
	}
	switch t.Preset {
	case "self":
		s.self = true
	case "weakest", "lowest_hp_rate":
		s.Pick, s.By = "min", by("t.hp_rate")
	case "ally_lowest_hp_rate":
		s.Side, s.Pick, s.By = "allies", "min", by("t.hp_rate")
	case "lowest_hp":
		s.Pick, s.By = "min", by("t.hp")
	case "highest_hp":
		s.Pick, s.By = "max", by("t.hp")
	case "strongest":
		s.Pick, s.By = "max", by("t.atk + t.mat")
	case "first", "ally_dead":
		s.Pick = "first"
	}
	if s.Pick == "" {
		s.Pick = "random"
	}
	return s
}
