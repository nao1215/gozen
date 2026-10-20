package rpg

import (
	"maps"
)

// SP allocation plan (the training hall's advice). PlanSP allocates the same way as SpendSP but does not actually spend, returning only
// the sequence of nodes to raise. ApplySP allocates according to that sequence. So that the advised plan and the actual result
// never diverge, SpendSP is itself built from PlanSP and ApplySP.

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
