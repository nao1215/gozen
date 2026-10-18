package rpg

import (
	"maps"
)

// SP の振り方の計画（修練所の勧め）。PlanSP は SpendSP と同じ振り方で、実際には振らずに
// 上げるノードの並びだけを返す。ApplySP はその並びのとおりに振る。勧めた計画と振った結果が
// 食い違わないように、SpendSP も PlanSP と ApplySP でできている。

// PlanSP is how SpendSP would spend the points now (the nodes to raise, in order), without
// spending them.
func (a *Actor) PlanSP() []string {
	if a.SP <= 0 {
		return nil
	}
	c := *a
	c.Tree = maps.Clone(a.Tree)
	if c.Tree == nil {
		c.Tree = map[string]int{}
	}
	return c.spend()
}

// ApplySP raises the nodes of a plan in order, as far as they open; the nodes raised.
func (a *Actor) ApplySP(plan []string) []string {
	var raised []string
	for _, id := range plan {
		if !a.Raise(id) {
			break
		}
		raised = append(raised, id)
	}
	return raised
}

// NodeRaise is a node of a plan: its level before and after.
type NodeRaise struct {
	Node     string
	From, To int
}

// PlanSteps are the nodes of a plan with their levels before and after, in the tree's order.
func (a *Actor) PlanSteps(plan []string) []NodeRaise {
	n := map[string]int{}
	for _, id := range plan {
		n[id]++
	}
	var out []NodeRaise
	for _, node := range a.Class().Tree {
		if n[node.ID] > 0 {
			out = append(out, NodeRaise{Node: node.ID, From: a.Tree[node.ID], To: a.Tree[node.ID] + n[node.ID]})
		}
	}
	return out
}

// NodeName is the name of a node of the actor's tree (its skill's when it has none).
func (a *Actor) NodeName(id string) string {
	n := a.Node(id)
	if n == nil {
		return id
	}
	if n.Name != "" {
		return n.Name
	}
	if s := a.db.Skills[n.Skill]; s != nil {
		return s.Name
	}
	return id
}
