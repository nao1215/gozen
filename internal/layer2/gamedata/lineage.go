package gamedata

import (
	"cmp"
	"encoding/json"
	"slices"
)

// Recipe kinds of a lineage.
const (
	RecipeCapture = "capture"
	RecipePair    = "pair"
	RecipeFamily  = "family"
)

// DefaultOpenDepth is how many generations a lineage opens by default. The root is depth
// zero, and a node shows its parents when its depth is below this number.
const DefaultOpenDepth = 2

// Recipe is one way to obtain a species: a capture, a specific pair of species or a
// rule for two families. Parents are concrete species IDs; Families names the family
// rule when one applies. Steps is the fewest breeding generations for this recipe.
type Recipe struct {
	Kind     string    `json:"kind"`
	Parents  [2]string `json:"parents,omitempty"`
	Families [2]string `json:"families,omitempty"`
	Steps    int       `json:"steps"`
	MinLevel int       `json:"min_level,omitempty"`
	MinPlus  int       `json:"min_plus,omitempty"`
}

// MarshalJSON leaves out the parents of a capture.
func (r Recipe) MarshalJSON() ([]byte, error) {
	out := struct {
		Kind     string   `json:"kind"`
		Parents  []string `json:"parents,omitempty"`
		Families []string `json:"families,omitempty"`
		Steps    int      `json:"steps"`
		MinLevel int      `json:"min_level,omitempty"`
		MinPlus  int      `json:"min_plus,omitempty"`
	}{Kind: r.Kind, Steps: r.Steps, MinLevel: r.MinLevel, MinPlus: r.MinPlus}
	if r.Kind != RecipeCapture {
		out.Parents = r.Parents[:]
	}
	if r.Kind == RecipeFamily {
		out.Families = r.Families[:]
	}
	return json.Marshal(out)
}

// LineageNode is a species in a breeding family tree. A pair node holds its two parents.
// A capture node is a leaf. Ref names why a node is not expanded: "seen" for a
// species shown earlier in the tree and "cycle" for one that is its own ancestor.
type LineageNode struct {
	Species string         `json:"species"`
	Path    string         `json:"path"`
	Depth   int            `json:"depth"`
	Recipe  Recipe         `json:"recipe"`
	Choice  int            `json:"choice"`
	Choices int            `json:"choices"`
	Sites   []string       `json:"sites,omitempty"`
	Ref     string         `json:"ref,omitempty"`
	Parents []*LineageNode `json:"parents,omitempty"`
}

// Lineage answers how a game's species can be obtained. Build it with BuildLineage.
type Lineage struct {
	db      *DB
	recipes map[string][]Recipe
}

// captureSites lists the names of locations whose encounters include a troop with the
// species, in the game's location order.
func (db *DB) captureSites(species string) []string {
	var sites []string
	for _, id := range db.Ordered("locations") {
		l := db.Locations[id]
		if l == nil {
			continue
		}
		for _, pick := range l.Encounters.Troops {
			t := db.Troops[pick.Troop]
			if t != nil && slices.Contains(t.Members, species) {
				name := l.Name
				if name == "" {
					name = id
				}
				if !slices.Contains(sites, name) {
					sites = append(sites, name)
				}
				break
			}
		}
	}
	return sites
}

// BuildLineage collects the usable recipes of every species. A recipe is usable when its parent
// species can be obtained, a male and a female are possible, and the parents can reach the
// minimum level. Recipes that only loop back to unobtainable species are left out.
func BuildLineage(db *DB) *Lineage {
	l := &Lineage{db: db, recipes: map[string][]Recipe{}}
	reach := db.BreedReach()
	levelOK := func(id string) bool {
		if db.Breeding == nil {
			return false
		}
		c := db.Classes[db.MonsterClass(id)]
		return c != nil && c.Max >= db.Breeding.MinLevel
	}
	maxLevel := func(id string) int { return db.Classes[db.MonsterClass(id)].Max }
	// uses reports whether making a species along its shortest path needs the child, so a
	// recipe through it would only loop back.
	var uses func(id, child string) bool
	uses = func(id, child string) bool {
		if id == child {
			return true
		}
		p := reach[id]
		return p.Steps > 0 && (uses(p.Parents[0], child) || uses(p.Parents[1], child))
	}
	ids := db.Ordered("enemies")
	for _, id := range ids {
		if e := db.Enemies[id]; e != nil && e.Recruit != nil && e.Recruit.Chance > 0 {
			l.recipes[id] = append(l.recipes[id], Recipe{Kind: RecipeCapture})
		}
	}
	if db.Breeding != nil {
		for _, p := range db.Breeding.Pairs {
			if len(p.Parents) != 2 || db.Enemies[p.Child] == nil || db.Enemies[p.Parents[0]] == nil || db.Enemies[p.Parents[1]] == nil {
				continue
			}
			a, b := p.Parents[0], p.Parents[1]
			pa, okA := reach[a]
			pb, okB := reach[b]
			if !okA || !okB || !levelOK(a) || !levelOK(b) || !db.sexCompatible(a, b) || uses(a, p.Child) || uses(b, p.Child) ||
				maxLevel(a) < p.MinLevel || maxLevel(b) < p.MinLevel || db.BreedResult(a, b, maxLevel(a), maxLevel(b), p.MinPlus, 0) != p.Child {
				continue
			}
			if a > b {
				a, b = b, a
			}
			l.recipes[p.Child] = append(l.recipes[p.Child], Recipe{Kind: RecipePair, Parents: [2]string{a, b}, Steps: max(pa.Steps, pb.Steps) + 1, MinLevel: p.MinLevel, MinPlus: p.MinPlus})
		}
		for _, f := range db.Breeding.Families {
			if len(f.Parents) != 2 {
				continue
			}
			best := map[string]Recipe{}
			for _, a := range ids {
				ea := db.Enemies[a]
				pa, okA := reach[a]
				if ea == nil || ea.Boss || ea.Family != f.Parents[0] || !okA || !levelOK(a) {
					continue
				}
				for _, b := range ids {
					eb := db.Enemies[b]
					pb, okB := reach[b]
					if eb == nil || eb.Boss || eb.Family != f.Parents[1] || !okB || !levelOK(b) || !db.sexCompatible(a, b) {
						continue
					}
					if slices.ContainsFunc(db.Breeding.Pairs, func(p BreedPair) bool { return samePair(p.Parents, a, b) }) {
						continue // a specific pair for these two is a pair recipe, not the family rule
					}
					child := db.BreedResult(a, b, maxLevel(a), maxLevel(b), 0, 0)
					if child == "" || !slices.Contains(f.Children, child) || uses(a, child) || uses(b, child) {
						continue
					}
					steps := max(pa.Steps, pb.Steps) + 1
					parents := [2]string{a, b}
					if parents[0] > parents[1] {
						parents[0], parents[1] = parents[1], parents[0]
					}
					families := [2]string{f.Parents[0], f.Parents[1]}
					if families[0] > families[1] {
						families[0], families[1] = families[1], families[0]
					}
					candidate := Recipe{Kind: RecipeFamily, Parents: parents, Families: families, Steps: steps}
					if old, ok := best[child]; !ok || steps < old.Steps || steps == old.Steps && cmp.Or(cmp.Compare(parents[0], old.Parents[0]), cmp.Compare(parents[1], old.Parents[1])) < 0 {
						best[child] = candidate
					}
				}
			}
			for child, recipe := range best {
				l.recipes[child] = append(l.recipes[child], recipe)
			}
		}
	}
	kind := map[string]int{RecipeCapture: 0, RecipePair: 1, RecipeFamily: 2}
	for id, rs := range l.recipes {
		slices.SortFunc(rs, func(a, b Recipe) int {
			return cmp.Or(cmp.Compare(a.Steps, b.Steps), cmp.Compare(kind[a.Kind], kind[b.Kind]),
				cmp.Compare(a.Parents[0], b.Parents[0]), cmp.Compare(a.Parents[1], b.Parents[1]))
		})
		l.recipes[id] = slices.CompactFunc(rs, func(a, b Recipe) bool { return a.Kind == b.Kind && a.Parents == b.Parents })
	}
	return l
}

// Recipes lists the ways to obtain a species, the shortest first.
func (l *Lineage) Recipes(species string) []Recipe { return slices.Clone(l.recipes[species]) }

// Tree builds the family tree of a species, or nil when it cannot be obtained. choice
// picks a recipe by position for a species; the default is the shortest recipe. A species
// is expanded the first time it appears in a depth-first walk, and later appearances are
// references, as is a species that is its own ancestor.
func (l *Lineage) Tree(species string, choice map[string]int) *LineageNode {
	if len(l.recipes[species]) == 0 {
		return nil
	}
	shown := map[string]bool{}
	var build func(id, path string, depth int, ancestors []string) *LineageNode
	build = func(id, path string, depth int, ancestors []string) *LineageNode {
		rs := l.recipes[id]
		pick := 0
		if n, ok := choice[id]; ok && n >= 0 && n < len(rs) {
			pick = n
		}
		n := &LineageNode{Species: id, Path: path, Depth: depth, Choice: pick, Choices: len(rs)}
		if len(rs) > 0 {
			n.Recipe = rs[pick]
		}
		switch {
		case slices.Contains(ancestors, id):
			n.Ref = "cycle"
			return n
		case shown[id]:
			n.Ref = "seen"
			return n
		}
		shown[id] = true
		switch n.Recipe.Kind {
		case RecipeCapture:
			n.Sites = l.db.captureSites(id)
		case RecipePair, RecipeFamily:
			next := append(slices.Clone(ancestors), id)
			for _, parent := range n.Recipe.Parents {
				n.Parents = append(n.Parents, build(parent, path+"/"+parent, depth+1, next))
			}
		}
		return n
	}
	return build(species, species, 0, nil)
}

// Walk visits a node and its parents depth first, parent A before parent B.
func (n *LineageNode) Walk(visit func(*LineageNode)) {
	visit(n)
	for _, p := range n.Parents {
		p.Walk(visit)
	}
}
