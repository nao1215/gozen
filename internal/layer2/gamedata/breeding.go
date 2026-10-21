package gamedata

import (
	"cmp"
	"slices"
)

// MonsterClass is the actor growth class of a monster species.
func (db *DB) MonsterClass(species string) string {
	e := db.Enemies[species]
	if e == nil {
		return ""
	}
	if e.Recruit != nil && e.Recruit.Class != "" {
		return e.Recruit.Class
	}
	return e.MonsterClass
}

// BreedResult returns the offspring for two species at the supplied parent levels and
// plus values. A specific pair takes priority over a family rule. The result is empty
// if the combination is unavailable. It does not change the game or any state.
func (db *DB) BreedResult(a, b string, levelA, levelB, plusA, plusB int) string {
	if db == nil || db.Breeding == nil || db.Enemies[a] == nil || db.Enemies[b] == nil {
		return ""
	}
	r := db.Breeding
	if levelA < max(1, r.MinLevel) || levelB < max(1, r.MinLevel) {
		return ""
	}
	for _, p := range r.Pairs {
		if samePair(p.Parents, a, b) && levelA >= p.MinLevel && levelB >= p.MinLevel && plusA+plusB >= p.MinPlus {
			return p.Child
		}
	}
	familyA, familyB := db.Enemies[a].Family, db.Enemies[b].Family
	rank := (db.Enemies[a].Rank + db.Enemies[b].Rank) / 2
	for _, f := range r.Families {
		if !samePair(f.Parents, familyA, familyB) {
			continue
		}
		children := slices.Clone(f.Children)
		slices.SortFunc(children, func(a, b string) int {
			return cmp.Or(cmp.Compare(db.Enemies[a].Rank, db.Enemies[b].Rank), cmp.Compare(a, b))
		})
		result := ""
		for _, id := range children {
			if db.Enemies[id].Rank <= rank {
				result = id
			}
		}
		return result
	}
	return ""
}

func samePair(pair []string, a, b string) bool {
	return len(pair) == 2 && (pair[0] == a && pair[1] == b || pair[0] == b && pair[1] == a)
}

// BreedReach gives the fewest generations known from repeatable captures and one
// recipe for each species. A missing species is unreachable with these rules.
func (db *DB) BreedReach() map[string]BreedPath {
	paths := map[string]BreedPath{}
	if db.Breeding == nil {
		for id, e := range db.Enemies {
			if e.Recruit != nil && e.Recruit.Chance > 0 {
				paths[id] = BreedPath{}
			}
		}
		return paths
	}
	// Plus values above the largest requirement are equivalent for reachability.
	maxPlus := 0
	for _, pair := range db.Breeding.Pairs {
		maxPlus = max(maxPlus, pair.MinPlus)
	}
	type key struct {
		species string
		plus    int
	}
	states := map[key]BreedPath{}
	for id, e := range db.Enemies {
		if e.Recruit != nil && e.Recruit.Chance > 0 {
			states[key{id, 0}] = BreedPath{}
		}
	}
	ids := db.Ordered("enemies")
	for changed := true; changed; {
		changed = false
		for _, a := range ids {
			classA := db.Classes[db.MonsterClass(a)]
			if classA == nil {
				continue
			}
			levelA := classA.Max
			if levelA < db.Breeding.MinLevel {
				continue
			}
			for _, b := range ids {
				classB := db.Classes[db.MonsterClass(b)]
				if classB == nil {
					continue
				}
				levelB := classB.Max
				if levelB < db.Breeding.MinLevel {
					continue
				}
				for plusA := 0; plusA <= maxPlus; plusA++ {
					pa, ok := states[key{a, plusA}]
					if !ok {
						continue
					}
					for plusB := 0; plusB <= maxPlus; plusB++ {
						pb, ok := states[key{b, plusB}]
						if !ok {
							continue
						}
						child := db.BreedResult(a, b, levelA, levelB, plusA, plusB)
						if child == "" {
							continue
						}
						plus := min(maxPlus, plusA+plusB+max(1, (levelA+levelB)/20))
						k := key{child, plus}
						steps := max(pa.Steps, pb.Steps) + 1
						if old, ok := states[k]; !ok || steps < old.Steps || steps == old.Steps && (a < old.Parents[0] || a == old.Parents[0] && b < old.Parents[1]) {
							states[k] = BreedPath{Steps: steps, Parents: [2]string{a, b}}
							changed = true
						}
					}
				}
			}
		}
	}
	for k, path := range states {
		if old, ok := paths[k.species]; !ok || path.Steps < old.Steps || path.Steps == old.Steps && (path.Parents[0] < old.Parents[0] || path.Parents[0] == old.Parents[0] && path.Parents[1] < old.Parents[1]) {
			paths[k.species] = path
		}
	}
	return paths
}

// BreedPath is a species' shortest number of breeding generations and a recipe.
// A zero-step path is directly capturable and has empty Parents.
type BreedPath struct {
	Steps   int       `json:"steps"`
	Parents [2]string `json:"parents,omitempty"`
}
