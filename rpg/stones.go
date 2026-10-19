package rpg

import (
	"fmt"
	"slices"
	"sort"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// 装備石の組み合わせ（stone_sets）を一つずつ、持っている石（袋の中と仲間が着けている石）で組めるか調べ、
// 組めるなら誰のどの枠に着けるかを決める（石屋で自分で選ぶとき。全体のいちばん強い着け方は stoneplan.go）。
//
// 同じ系列の組み合わせは 1 つとして扱う（上の石をまぜた並びは、元の並びと同じ物）。
// 仲間が今成り立たせている組み合わせの石は使わない（外すと組み合わせが切れる）。

// StoneOffer は組める系列の代表と着け手、その価値。
type StoneOffer struct {
	Set   string
	Actor string
	Score int
}

// SetScore is how much a stone set gives: its stat points and SkillFor for a skill.
func SetScore(s *data.StoneSet) int {
	n := StatPoints(s.Stats)
	if s.Skill != "" {
		n += SkillFor
	}
	return n
}

// setFamilyKey は明示 ID と旧形式の名前を別の領域で識別する。
func setFamilyKey(set *data.StoneSet) string {
	if set.Family != "" {
		return "id:" + set.Family
	}
	return "name:" + set.Name
}

// SetFamily は同じ系列のセットをデータの順で返す。ID の省略時は名前でまとめる。
func (s *State) SetFamily(set string) []string {
	ss := s.DB.StoneSets[set]
	if ss == nil {
		return nil
	}
	var out []string
	for _, id := range s.DB.Ordered("stone_sets") {
		if setFamilyKey(s.DB.StoneSets[id]) == setFamilyKey(ss) {
			out = append(out, id)
		}
	}
	return out
}

// SetHolds は同じ系列のセットを成立させている仲間がいるかを返す。
func (s *State) SetHolds(set string) bool {
	fam := s.SetFamily(set)
	for _, a := range s.Members() {
		for _, id := range a.Sets() {
			if slices.Contains(fam, id) {
				return true
			}
		}
	}
	return false
}

// lockedStones are the stones of the sets the actor has working (counted).
func (s *State) lockedStones(a *Actor) map[string]int {
	out := map[string]int{}
	for _, id := range a.Sets() {
		for _, st := range s.DB.StoneSets[id].Stones {
			out[st]++
		}
	}
	return out
}

// spareWorn is how many of a stone the members other than a wear outside their working sets.
func (s *State) spareWorn(a *Actor, stone string) int {
	n := 0
	for _, m := range s.Members() {
		if m == a {
			continue
		}
		worn := 0
		for _, it := range m.Equip {
			if it == stone {
				worn++
			}
		}
		n += max(0, worn-s.lockedStones(m)[stone])
	}
	return n
}

// stonePut is a stone to put in a slot.
type stonePut struct{ slot, item string }

// stonePlan is how to put a set's stones on a: the stones to put in which slots and how
// many come off the other members (ok false: it cannot be done).
func (s *State) stonePlan(a *Actor, set string) (puts []stonePut, taken int, ok bool) {
	ss := s.DB.StoneSets[set]
	if ss == nil || len(ss.Stones) == 0 {
		return nil, 0, false
	}
	for _, st := range ss.Stones {
		if !a.CanEquip(st) {
			return nil, 0, false
		}
	}
	need := map[string]int{}
	for _, st := range ss.Stones {
		need[st]++
	}
	keep := map[string]bool{} // slots whose stone is already one of the set's
	for _, sl := range s.DB.Game.Slots {
		if it := a.Equip[sl.ID]; it != "" && need[it] > 0 {
			need[it]--
			keep[sl.ID] = true
		}
	}
	locked := s.lockedStones(a)
	used := map[string]bool{}
	for _, st := range ss.Stones {
		if need[st] == 0 {
			continue
		}
		need[st]--
		// 空いた枠から。無ければ、組み合わせに使っていない石の枠
		slot := ""
		for _, sl := range s.DB.Game.Slots {
			if !used[sl.ID] && !keep[sl.ID] && a.Equip[sl.ID] == "" && s.DB.Items[st].Fits(sl.ID) {
				slot = sl.ID
				break
			}
		}
		if slot == "" {
			free := map[string]int{}
			for _, sl := range s.DB.Game.Slots {
				if it := a.Equip[sl.ID]; it != "" && !keep[sl.ID] {
					free[it]++
				}
			}
			for _, sl := range s.DB.Game.Slots {
				it := a.Equip[sl.ID]
				if used[sl.ID] || keep[sl.ID] || it == "" || !s.DB.Items[st].Fits(sl.ID) {
					continue
				}
				if free[it] > locked[it] { // この石は組み合わせに使っていない
					slot = sl.ID
					free[it]--
					break
				}
			}
		}
		if slot == "" {
			return nil, 0, false
		}
		used[slot] = true
		puts = append(puts, stonePut{slot, st})
	}
	// 石が足りるか（袋の中、足りなければほかの仲間が組み合わせに使っていない石）
	want := map[string]int{}
	for _, p := range puts {
		want[p.item]++
	}
	for st, n := range want {
		bag := s.Items[st]
		if bag >= n {
			continue
		}
		if bag+s.spareWorn(a, st) < n {
			return nil, 0, false
		}
		taken += n - bag
	}
	return puts, taken, true
}

// stoneValue is the sum of the stats of a set's stones (to take the better stones).
func (s *State) stoneValue(set string) int {
	n := 0
	for _, st := range s.DB.StoneSets[set].Stones {
		for _, v := range s.DB.Items[st].Stats {
			n += v
		}
	}
	return n
}

// SetFor は系列内で a に合うセットを選ぶ。他の仲間から外す石が少なく、石の能力が高い順。
// 同点ならデータの順を保ち、組める物がなければ ok は false。
func (s *State) SetFor(set string, a *Actor) (string, bool) {
	best, bestTaken, bestValue := "", 0, 0
	for _, id := range s.SetFamily(set) {
		_, taken, ok := s.stonePlan(a, id)
		if !ok {
			continue
		}
		v := s.stoneValue(id)
		if best == "" || taken < bestTaken || (taken == bestTaken && v > bestValue) {
			best, bestTaken, bestValue = id, taken, v
		}
	}
	return best, best != ""
}

// SetWearers は同じ系列のセットを着けられる仲間を返す。for の順、続いてパーティ順。
// 専用セットは for の仲間だけにする。
func (s *State) SetWearers(set string) []*Actor {
	ss := s.DB.StoneSets[set]
	if ss == nil {
		return nil
	}
	var order []*Actor
	for _, id := range ss.For {
		for _, a := range s.Members() {
			if a.ID == id {
				order = append(order, a)
			}
		}
	}
	for _, a := range s.Members() {
		if !ss.Only && !slices.Contains(order, a) {
			order = append(order, a)
		}
	}
	var out []*Actor
	for _, a := range order {
		if _, ok := s.SetFor(set, a); ok {
			out = append(out, a)
		}
	}
	return out
}

// StoneOffers は手持ちで組めて誰も成立させていないセットを系列ごとに一つ返す。
// 価値の高い順に並べ、同点ではデータの順を保つ。
func (s *State) StoneOffers() []StoneOffer {
	var out []StoneOffer
	seen := map[string]bool{}
	for _, id := range s.DB.Ordered("stone_sets") {
		ss := s.DB.StoneSets[id]
		key := setFamilyKey(ss)
		if seen[key] {
			continue
		}
		seen[key] = true
		if s.SetHolds(id) {
			continue
		}
		ws := s.SetWearers(id)
		if len(ws) == 0 {
			continue
		}
		set, _ := s.SetFor(id, ws[0])
		out = append(out, StoneOffer{Set: set, Actor: ws[0].ID, Score: SetScore(ss)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// EquipSet puts the stones of a stone set on a: from the bag, or taken off another member
// who wears one outside a working set; what was in the slots goes back to the bag.
func (s *State) EquipSet(a *Actor, set string) error {
	puts, _, ok := s.stonePlan(a, set)
	if !ok {
		return fmt.Errorf("%s cannot put together %s", a.ID, set)
	}
	before, beforeMP := a.Stat("mhp"), a.Stat("mmp")
	for _, p := range puts {
		if s.Items[p.item] <= 0 {
			s.takeSpare(a, p.item)
		}
		if err := s.Equip(a, p.slot, p.item); err != nil {
			return err
		}
	}
	if up := a.Stat("mhp") - before; up > 0 && a.HP > 0 { // 増えた最大HP・MPの分だけ HP・MP も
		a.HP += up
	}
	if up := a.Stat("mmp") - beforeMP; up > 0 {
		a.MP += up
	}
	a.Clamp()
	return nil
}

// takeSpare takes a stone off a member other than a who wears it outside a working set.
func (s *State) takeSpare(a *Actor, stone string) {
	for _, m := range s.Members() {
		if m == a {
			continue
		}
		locked := s.lockedStones(m)[stone]
		worn := 0
		for _, it := range m.Equip {
			if it == stone {
				worn++
			}
		}
		if worn <= locked {
			continue
		}
		for _, sl := range s.DB.Game.Slots {
			if m.Equip[sl.ID] == stone {
				s.unequip(m, sl.ID)
				return
			}
		}
	}
}

// StonePlanFor reports whether a could put on the set's stones (and how many would come
// off the other members).
func (s *State) StonePlanFor(a *Actor, set string) (int, int, bool) {
	puts, taken, ok := s.stonePlan(a, set)
	return len(puts), taken, ok
}
