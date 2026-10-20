package rpg

import (
	"fmt"
	"slices"
	"sort"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// Check each equipment-stone combination (stone_sets) one by one for whether it can be assembled
// from the stones held (in the bag
// and worn by party members), and if so decide whose slot each goes in (for when the player
// chooses at the stone shop; the
// best overall way to wear them is in stoneplan.go).
//
// Combinations of the same family count as one (a variant with upgraded stones mixed in is the
// same as the original).
// Stones of a combination a party member currently satisfies are not used (removing them would
// break that combination).

// StoneOffer is the representative of an assemblable family, who wears it, and its value.
type StoneOffer struct {
	Set   string
	Actor string
	Score int
}

// setScore is how much a stone set gives: its stat points and SkillFor for a skill.
func setScore(s *data.StoneSet) int {
	n := statPoints(s.Stats)
	if s.Skill != "" {
		n += skillFor
	}
	return n
}

// setFamilyKey distinguishes explicit IDs from legacy-format names as separate domains.
func setFamilyKey(set *data.StoneSet) string {
	if set.Family != "" {
		return "id:" + set.Family
	}
	return "name:" + set.Name
}

// SetFamily returns the sets of the same family in data order. When the ID is omitted, they are
// grouped by name.
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

// SetHolds reports whether a party member satisfies a set of the same family.
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
		// First free slots; if none, slots whose stone is not used in a combination.
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
				if free[it] > locked[it] { // this stone is not used in a combination
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
	// Whether there are enough stones (in the bag; if short, stones other members do not use in a
	// combination).
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

// SetFor picks the set in the family that fits a. Ordered by fewest stones to take from other
// members, then highest stone ability.
// On a tie it keeps data order; ok is false if nothing can be assembled.
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

// SetWearers returns the members who can wear a set of the same family, in for's order, then party
// order.
// A dedicated set is limited to the for members.
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

// StoneOffers returns, per family, one set that can be assembled from what is held and that nobody
// satisfies.
// Sorted by value descending; ties keep data order.
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
		out = append(out, StoneOffer{Set: set, Actor: ws[0].ID, Score: setScore(ss)})
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
	if up := a.Stat("mhp") - before; up > 0 && a.HP > 0 { // HP and MP also rise by the amount the max HP and MP increased
		a.HP += up
	}
	if up := a.Stat("mmp") - beforeMP; up > 0 {
		a.MP += up
	}
	a.clamp()
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
