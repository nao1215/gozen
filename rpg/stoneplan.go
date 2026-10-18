package rpg

import (
	"slices"
	"sort"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// いちばん強い石の着け方（石屋の提案）。持っている石（袋の中と仲間が着けている石）を全部使い、
// パーティの誰にどの石を着けると、いちばん強くなるかを探す。
//
// 強さ = 着けた石と成り立つ組み合わせの能力値の合計（最大HP・最大MPは 5 分の 1）
//      ＋ 組み合わせで覚える技の価値（for の人が覚えると SkillFor、ほかの人なら SkillOther。
//        もう覚えている技は 0。同じ技はパーティで一番価値の高い一人だけ数える）。
// 同じ強さなら、石を動かす数の少ない着け方。

// The worth of a skill a stone set teaches (see BestStones).
const (
	SkillFor   = 10
	SkillOther = 3
)

// StoneLoadout are the stones each member wears (by actor id, in the order of the data).
type StoneLoadout map[string][]string

// StatPoints is the sum of the stats, max HP and MP counting a fifth.
func StatPoints(st data.Stats) int {
	n := 0
	for k, v := range st {
		if k == "mhp" || k == "mmp" {
			n += v / 5
		} else {
			n += v
		}
	}
	return n
}

// stoneKinds are the slots of the stones of the stone sets ("stone").
func (s *State) stoneKinds() map[string]bool {
	out := map[string]bool{}
	for _, set := range s.DB.StoneSets {
		for _, st := range set.Stones {
			if it := s.DB.Items[st]; it != nil && it.Slot != "" {
				out[it.Slot] = true
			}
		}
	}
	return out
}

// IsStone reports whether an item is a stone (it goes in the slots of the stone sets' stones).
func (s *State) IsStone(id string) bool {
	it := s.DB.Items[id]
	return it != nil && it.Slot != "" && s.stoneKinds()[it.Slot]
}

// StoneSlots are the slots of a member that take stones.
func (s *State) StoneSlots(a *Actor) []string {
	var out []string
	for k := range s.stoneKinds() {
		probe := &data.Item{Slot: k}
		for _, sl := range s.DB.Game.Slots {
			if probe.Fits(sl.ID) && !slices.Contains(out, sl.ID) {
				out = append(out, sl.ID)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return s.slotIndex(out[i]) < s.slotIndex(out[j]) })
	return out
}

func (s *State) slotIndex(id string) int {
	return slices.IndexFunc(s.DB.Game.Slots, func(sl data.Slot) bool { return sl.ID == id })
}

// WornStones are the stones a member wears now.
func (s *State) WornStones(a *Actor) []string {
	var out []string
	for _, sl := range s.StoneSlots(a) {
		if it := a.Equip[sl]; it != "" && s.IsStone(it) {
			out = append(out, it)
		}
	}
	return s.sortStones(out)
}

func (s *State) sortStones(st []string) []string {
	order := map[string]int{}
	for i, id := range s.DB.Ordered("items") {
		order[id] = i
	}
	sort.SliceStable(st, func(i, j int) bool { return order[st[i]] < order[st[j]] })
	return st
}

// mayWear reports whether a member may wear a stone (a set only for others still lets
// anyone wear its stones for their stats).
func (s *State) mayWear(a *Actor, id string) bool { return a.CanEquip(id) }

// skillWorth is what a set's skill is worth on a.
func (s *State) skillWorth(a *Actor, set *data.StoneSet) int {
	if set.Skill == "" || slices.Contains(a.OwnSkills(), set.Skill) {
		return 0
	}
	if slices.Contains(set.For, a.ID) {
		return SkillFor
	}
	return SkillOther
}

// StoneScore is how strong the party is with the stones (see the top of the file).
func (s *State) StoneScore(l StoneLoadout) int {
	n := 0
	skills := map[string]int{}
	for _, a := range s.Members() {
		for _, st := range l[a.ID] {
			n += StatPoints(s.DB.Items[st].Stats)
		}
		for _, id := range SetsOf(s.DB, a.ID, l[a.ID]) {
			set := s.DB.StoneSets[id]
			n += StatPoints(set.Stats)
			if w := s.skillWorth(a, set); w > skills[set.Skill] {
				skills[set.Skill] = w
			}
		}
	}
	for _, w := range skills {
		n += w
	}
	return n
}

// StonesNow are the stones the members wear now.
func (s *State) StonesNow() StoneLoadout {
	out := StoneLoadout{}
	for _, a := range s.Members() {
		out[a.ID] = s.WornStones(a)
	}
	return out
}

// StonePool are the stones held: in the bag and worn.
func (s *State) StonePool() map[string]int {
	pool := map[string]int{}
	for id, n := range s.Items {
		if n > 0 && s.IsStone(id) {
			pool[id] += n
		}
	}
	for _, a := range s.Members() {
		for _, st := range s.WornStones(a) {
			pool[st]++
		}
	}
	return pool
}

// stoneCore is the stones of the sets a member would make, and what they give.
type stoneCore struct {
	stones []string
	worth  int // the sets' stats and skills
}

// cores are the ways a could make sets with the stones held: every stone used by a set.
func (s *State) cores(a *Actor, pool map[string]int, slots int) []stoneCore {
	var kinds []string
	for _, id := range s.DB.Ordered("items") {
		if pool[id] == 0 || !s.mayWear(a, id) {
			continue
		}
		for _, set := range s.DB.StoneSets {
			if slices.Contains(set.Stones, id) {
				kinds = append(kinds, id)
				break
			}
		}
	}
	out := []stoneCore{{}}
	var pick []string
	var walk func(from int)
	walk = func(from int) {
		if len(pick) > 0 {
			sets := SetsOf(s.DB, a.ID, pick)
			need := map[string]int{}
			worth := 0
			for _, id := range sets {
				set := s.DB.StoneSets[id]
				c := map[string]int{}
				for _, st := range set.Stones {
					c[st]++
				}
				for st, n := range c {
					need[st] = max(need[st], n)
				}
				worth += StatPoints(set.Stats) + s.skillWorth(a, set)
			}
			total := 0
			for _, n := range need {
				total += n
			}
			if len(sets) > 0 && total == len(pick) { // どの石も組み合わせに使われている
				out = append(out, stoneCore{stones: slices.Clone(pick), worth: worth})
			}
		}
		if len(pick) == slots {
			return
		}
		for i := from; i < len(kinds); i++ {
			used := 0
			for _, p := range pick {
				if p == kinds[i] {
					used++
				}
			}
			if used >= pool[kinds[i]] {
				continue
			}
			pick = append(pick, kinds[i])
			walk(i)
			pick = pick[:len(pick)-1]
		}
	}
	walk(0)
	sort.SliceStable(out, func(i, j int) bool { return out[i].worth > out[j].worth })
	return out
}

// BestStones is the strongest way to wear the stones held, its strength, and the
// strength now (the same loadout as now when nothing is stronger).
func (s *State) BestStones() (best StoneLoadout, score, now int) {
	members := s.Members()
	cur := s.StonesNow()
	now = s.StoneScore(cur)
	pool := s.StonePool()
	slots := make([]int, len(members))
	total := 0
	for i, a := range members {
		slots[i] = len(s.StoneSlots(a))
		total += slots[i]
	}
	// 石の能力の大きい順に、全部の枠に入る分（上限の見積もり）
	var vals []int
	for id, n := range pool {
		for range n {
			vals = append(vals, StatPoints(s.DB.Items[id].Stats))
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(vals)))
	top := 0
	for i := 0; i < len(vals) && i < total; i++ {
		top += max(0, vals[i])
	}
	cores := make([][]stoneCore, len(members))
	rest := make([]int, len(members)+1) // 後の人の組み合わせの価値の上限
	for i, a := range members {
		cores[i] = s.cores(a, pool, slots[i])
	}
	for i := len(members) - 1; i >= 0; i-- {
		rest[i] = rest[i+1] + cores[i][0].worth
	}
	best, score = cur, now
	bestMoves := 0
	left := map[string]int{}
	for id, n := range pool {
		left[id] = n
	}
	chosen := make([][]string, len(members))
	nodes := 0
	var walk func(i, worth int)
	walk = func(i, worth int) {
		nodes++
		if nodes > 200000 || worth+rest[i]+top < score {
			return
		}
		if i == len(members) {
			l := s.fillStones(members, slots, chosen, left, cur)
			sc := s.StoneScore(l)
			mv := stoneMoves(cur, l)
			if sc > score || (sc == score && mv < bestMoves) {
				best, score, bestMoves = l, sc, mv
			}
			return
		}
		for _, c := range cores[i] {
			ok := true
			for _, st := range c.stones {
				left[st]--
				if left[st] < 0 {
					ok = false
				}
			}
			if ok {
				chosen[i] = c.stones
				walk(i+1, worth+c.worth)
			}
			for _, st := range c.stones {
				left[st]++
			}
		}
	}
	walk(0, 0)
	return best, score, now
}

// fillStones puts the stones left in the slots left, the strongest first (to whoever
// wears that stone now, else the first with room).
func (s *State) fillStones(members []*Actor, slots []int, chosen [][]string, left map[string]int, cur StoneLoadout) StoneLoadout {
	out := StoneLoadout{}
	room := make([]int, len(members))
	for i, a := range members {
		out[a.ID] = slices.Clone(chosen[i])
		room[i] = slots[i] - len(chosen[i])
	}
	var rest []string
	for _, id := range s.DB.Ordered("items") {
		for range left[id] {
			rest = append(rest, id)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		return StatPoints(s.DB.Items[rest[i]].Stats) > StatPoints(s.DB.Items[rest[j]].Stats)
	})
	for _, st := range rest {
		if StatPoints(s.DB.Items[st].Stats) <= 0 {
			break
		}
		at := -1
		for i, a := range members {
			if room[i] > 0 && s.mayWear(a, st) && count(cur[a.ID], st) > count(out[a.ID], st) {
				at = i
				break
			}
		}
		if at < 0 {
			for i, a := range members {
				if room[i] > 0 && s.mayWear(a, st) {
					at = i
					break
				}
			}
		}
		if at >= 0 {
			out[members[at].ID] = append(out[members[at].ID], st)
			room[at]--
		}
	}
	for id := range out {
		out[id] = s.sortStones(out[id])
	}
	return out
}

func count(xs []string, x string) int {
	n := 0
	for _, y := range xs {
		if y == x {
			n++
		}
	}
	return n
}

// stoneMoves is how many stones go on someone who did not wear them.
func stoneMoves(from, to StoneLoadout) int {
	n := 0
	for id, st := range to {
		had := map[string]int{}
		for _, x := range from[id] {
			had[x]++
		}
		for _, x := range st {
			if had[x] > 0 {
				had[x]--
			} else {
				n++
			}
		}
	}
	return n
}

// ApplyStones puts the stones on as the loadout says: what nobody keeps goes back to
// the bag first, then each member takes theirs (max HP and MP gained come with HP and MP).
func (s *State) ApplyStones(l StoneLoadout) {
	before, beforeMP := map[string]int{}, map[string]int{}
	want := map[string]map[string]int{}
	for _, a := range s.Members() {
		before[a.ID], beforeMP[a.ID] = a.Stat("mhp"), a.Stat("mmp")
		want[a.ID] = map[string]int{}
		for _, st := range l[a.ID] {
			want[a.ID][st]++
		}
		for _, sl := range s.StoneSlots(a) {
			it := a.Equip[sl]
			if it == "" || !s.IsStone(it) {
				continue
			}
			if want[a.ID][it] > 0 {
				want[a.ID][it]--
			} else {
				_ = s.Equip(a, sl, "") //nolint:errcheck // Removing existing equipment with an empty item cannot fail.
			}
		}
	}
	for _, a := range s.Members() {
		for _, st := range l[a.ID] {
			if want[a.ID][st] == 0 {
				continue
			}
			for _, sl := range s.StoneSlots(a) {
				if a.Equip[sl] == "" && s.DB.Items[st].Fits(sl) {
					if s.Equip(a, sl, st) == nil {
						want[a.ID][st]--
					}
					break
				}
			}
		}
		if up := a.Stat("mhp") - before[a.ID]; up > 0 && a.HP > 0 {
			a.HP += up
		}
		if up := a.Stat("mmp") - beforeMP[a.ID]; up > 0 {
			a.MP += up
		}
		a.Clamp()
	}
}
