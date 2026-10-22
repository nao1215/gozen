package battle

import (
	"math"
	"slices"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// transform changes a living foe in place. Its final form alone pays rewards.
func (b *Battle) transform(u, t *Unit, e *data.Effect) []Step {
	if t == nil || !t.Enemy || t.Fled || b.DB.Enemies[e.Transform] == nil {
		return []Step{{Kind: Message, Text: "The form did not change."}}
	}
	before := t.Picture()
	ratio := float64(t.HP) / float64(t.MHP())
	hp := 0
	hpMode := ""
	if e.TransformHP.Set() {
		hpMode = e.TransformHP.String()
	}
	switch hpMode {
	case "", "ratio":
	case "full":
	default:
		v, err := e.TransformHP.Number(b.effEnv(u, t))
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return []Step{{Kind: Message, Text: "The form did not change."}}
		}
		hp = int(math.Round(v))
	}
	n := newEnemyUnit(b, e.Transform, t.Index)
	if !e.TransformClear {
		n.States = t.States
		n.Guard = t.Guard
	}
	switch hpMode {
	case "", "ratio":
		hp = int(math.Round(ratio * float64(n.MHP())))
	case "full":
		hp = n.MHP()
	}
	t.Name, t.EID, t.Def, t.base = n.Name, n.EID, n.Def, n.base
	t.HP, t.MP = max(1, min(n.MHP(), hp)), n.MP
	t.States, t.Guard, t.fired, t.firedHP = n.States, n.Guard, map[int]bool{}, map[int]bool{}
	b.St.Vars["bestiary."+t.EID+".seen"] = true
	msg := e.Message
	if msg == "" {
		msg = "{target} changed form!"
	}
	return []Step{{Kind: Transform, User: u, Target: t, Before: before, Text: b.text(msg, u, t, "", 0), HP: t.HP, MP: t.MP}}
}

// hpTriggers fires the first crossed threshold after damage, before a fatal fall.
func (b *Battle) hpTriggers(t *Unit, before int) []Step {
	if t == nil || t.Def == nil || len(t.Def.HPTriggers) == 0 || t.Fled {
		return nil
	}
	maximum := float64(t.MHP())
	for i, trigger := range t.Def.HPTriggers {
		if t.firedHP[i] || float64(before)/maximum < trigger.Below || float64(t.HP)/maximum >= trigger.Below {
			continue
		}
		oldID := t.EID
		t.firedHP[i] = true
		steps := b.transform(t, t, &data.Effect{Transform: trigger.Transform, TransformHP: trigger.HP, TransformClear: trigger.Clear})
		if t.EID == oldID {
			t.firedHP[i] = true
		}
		return steps
	}
	return nil
}

// merge takes the first Count eligible living foes in troop order, including the user.
// The replacement occupies the middle selected slot and pays one reward.
func (b *Battle) merge(u *Unit, e *data.Effect) []Step {
	m := e.Merge
	if !u.Enemy || m == nil || b.DB.Enemies[m.Into] == nil || m.Count < 2 {
		return []Step{{Kind: Message, Text: "The merge failed."}}
	}
	members := []*Unit{u}
	for _, f := range b.Foes {
		if f != u && f.Alive() && slices.Contains(m.From, f.EID) {
			members = append(members, f)
			if len(members) == m.Count {
				break
			}
		}
	}
	if len(members) < m.Count || !slices.Contains(m.From, u.EID) {
		return []Step{{Kind: Message, Text: "There are not enough allies to merge."}}
	}
	slices.SortFunc(members, func(a, z *Unit) int { return a.Index - z.Index })
	center := members[len(members)/2]
	index := slices.Index(b.Foes, center)
	for _, member := range members {
		if member.Index < center.Index {
			index--
		}
	}
	sum, rates := 0, 0.0
	for _, f := range members {
		sum += f.HP
		rates += float64(f.HP) / float64(f.MHP())
	}
	n := newEnemyUnit(b, m.Into, index)
	switch m.HP {
	case "", "sum":
		n.HP = min(n.MHP(), sum)
	case "ratio":
		n.HP = min(n.MHP(), max(1, int(math.Round(rates/float64(len(members))*float64(n.MHP())))))
	}
	for _, f := range members {
		f.Fled = true // consumed forms do not pay rewards or count as defeats
	}
	b.Foes = slices.DeleteFunc(b.Foes, func(f *Unit) bool { return slices.Contains(members, f) })
	index = min(index, len(b.Foes))
	b.Foes = slices.Insert(b.Foes, index, n)
	for i, f := range b.Foes {
		f.Index = i
	}
	b.St.Vars["bestiary."+n.EID+".seen"] = true
	msg := e.Message
	if msg == "" {
		msg = "{target} appeared from the merge!"
	}
	return []Step{{Kind: Merge, User: u, Target: n, Members: members, Text: b.text(msg, u, n, "", 0), HP: n.HP, MP: n.MP}}
}
