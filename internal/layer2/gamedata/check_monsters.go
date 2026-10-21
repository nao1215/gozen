package gamedata

import (
	"fmt"
	"math"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
)

func (c *checker) monsters() {
	db := c.db
	startErrors, sexErrors := len(c.errs), 0
	used := db.Breeding != nil || len(db.MonsterFamilies) != 0
	for _, e := range db.Enemies {
		used = used || e.Family != "" || e.Rank != 0 || e.Recruit != nil || e.Sex != "" || e.SexRatio != nil
	}
	if !used {
		return
	}
	c.needsVersion("game.yaml schema_version", "monster collection", SchemaVersion6)
	if len(db.MonsterFamilies) == 0 {
		c.errorf(diag.MonsterRules, "monster_families", "define monster_families before assigning enemy families")
	}
	if db.Game.MaxParty > 4 {
		c.errorf(diag.MonsterRules, "game.yaml max_party", "monster collection supports at most four active party members")
	}
	for id, e := range db.Enemies {
		w := "enemies." + id
		if e.Sex != "" && e.Sex != "any" && e.Sex != "male" && e.Sex != "female" {
			c.errorf(diag.MonsterRules, w+".sex", "sex must be any, male or female")
			sexErrors++
		}
		if e.SexRatio != nil && (math.IsNaN(*e.SexRatio) || *e.SexRatio < 0 || *e.SexRatio > 1 || e.Sex != "" && e.Sex != "any") {
			c.errorf(diag.MonsterRules, w+".sex_ratio", "sex_ratio must be between zero and one and is only valid for sex: any")
			sexErrors++
		}
		if e.Boss && e.Family == "" && e.Rank == 0 {
			continue
		}
		if e.Family == "" || db.MonsterFamilies[e.Family] == "" {
			c.errorf(diag.MonsterRules, w+".family", "unknown or missing monster family %q", e.Family)
		}
		if e.Rank < 1 {
			c.errorf(diag.MonsterRules, w+".rank", "rank must be positive")
		}
		if !e.Boss && db.Classes[db.MonsterClass(id)] == nil {
			c.errorf(diag.MonsterRules, w+".monster_class", "unknown or missing monster class %q", db.MonsterClass(id))
		}
		if r := e.Recruit; r != nil {
			if e.Boss {
				c.errorf(diag.MonsterRules, w+".recruit", "a boss cannot be recruited")
			}
			if r.Chance <= 0 || r.Chance > 1 {
				c.errorf(diag.MonsterRules, w+".recruit.chance", "chance must be greater than zero and at most one")
			}
			if r.Class != "" && db.Classes[r.Class] == nil {
				c.errorf(diag.MonsterRules, w+".recruit.class", "unknown monster class %q", r.Class)
			}
			if r.Level < 0 {
				c.errorf(diag.MonsterRules, w+".recruit.level", "level cannot be negative")
			}
			if r.Bonus < 0 {
				c.errorf(diag.MonsterRules, w+".recruit.bonus", "bonus cannot be negative")
			}
			if r.BonusItem != "" && db.Items[r.BonusItem] == nil {
				c.errorf(diag.MonsterRules, w+".recruit.bonus_item", "unknown item %q", r.BonusItem)
			}
			if r.BonusSkill != "" && db.Skills[r.BonusSkill] == nil {
				c.errorf(diag.MonsterRules, w+".recruit.bonus_skill", "unknown skill %q", r.BonusSkill)
			}
		}
	}
	if db.Breeding == nil {
		if len(c.errs) == startErrors+sexErrors {
			c.monsterReachability()
		}
		return
	}
	r := db.Breeding
	if r.MinLevel < 1 || r.SkillSlots < 0 {
		c.errorf(diag.MonsterRules, "breeding", "min_level must be positive and skill_slots cannot be negative")
	}
	seenPairs := map[string]string{}
	for i, p := range r.Pairs {
		w := fmt.Sprintf("breeding.pairs[%d]", i)
		if len(p.Parents) != 2 {
			c.errorf(diag.MonsterRules, w+".parents", "parents needs two enemy IDs")
			continue
		}
		if p.Parents[0] > p.Parents[1] {
			p.Parents[0], p.Parents[1] = p.Parents[1], p.Parents[0]
		}
		key := strings.Join(p.Parents, "/")
		if prev, ok := seenPairs[key]; ok {
			c.errorf(diag.MonsterRules, w, "pair %s is also defined at %s", key, prev)
		}
		seenPairs[key] = w
		for _, id := range append(append([]string{}, p.Parents...), p.Child) {
			if db.Enemies[id] == nil {
				c.errorf(diag.MonsterRules, w, "unknown monster %q", id)
			} else if db.Enemies[id].Boss {
				c.errorf(diag.MonsterRules, w, "boss %q cannot breed", id)
			}
		}
		if db.Enemies[p.Parents[0]] != nil && db.Enemies[p.Parents[1]] != nil && !db.sexCompatible(p.Parents[0], p.Parents[1]) {
			c.errorf(diag.MonsterRules, w+".parents", "pair cannot breed: no male and female combination")
			sexErrors++
		}
		if p.Child == p.Parents[0] || p.Child == p.Parents[1] {
			c.errorf(diag.MonsterRules, w, "a direct parent-to-self recipe does not advance the collection")
		}
		if p.MinLevel < 0 || p.MinPlus < 0 {
			c.errorf(diag.MonsterRules, w, "min_level and min_plus cannot be negative")
		}
	}
	seenFamilies := map[string]string{}
	for i, f := range r.Families {
		w := fmt.Sprintf("breeding.families[%d]", i)
		if len(f.Parents) != 2 {
			c.errorf(diag.MonsterRules, w+".parents", "parents needs two family IDs")
			continue
		}
		if f.Parents[0] > f.Parents[1] {
			f.Parents[0], f.Parents[1] = f.Parents[1], f.Parents[0]
		}
		key := strings.Join(f.Parents, "/")
		if prev, ok := seenFamilies[key]; ok {
			c.errorf(diag.MonsterRules, w, "family pair %s is also defined at %s", key, prev)
		}
		seenFamilies[key] = w
		for _, id := range f.Parents {
			if db.MonsterFamilies[id] == "" {
				c.errorf(diag.MonsterRules, w, "unknown family %q", id)
			}
		}
		if db.MonsterFamilies[f.Parents[0]] != "" && db.MonsterFamilies[f.Parents[1]] != "" && !db.familySexCompatible(f.Parents[0], f.Parents[1]) {
			c.errorf(diag.MonsterRules, w+".parents", "family rule cannot breed: no male and female combination")
			sexErrors++
		}
		if len(f.Children) == 0 {
			c.errorf(diag.MonsterRules, w+".children", "children cannot be empty")
		}
		for _, id := range f.Children {
			if db.Enemies[id] == nil {
				c.errorf(diag.MonsterRules, w, "unknown monster %q", id)
			} else if db.Enemies[id].Boss {
				c.errorf(diag.MonsterRules, w, "boss %q cannot be a child", id)
			}
		}
	}
	if len(c.errs) != startErrors+sexErrors {
		return
	}
	c.monsterReachability()
}

func (c *checker) monsterReachability() {
	db := c.db
	paths := db.BreedReach()
	for id, e := range db.Enemies {
		if e.Boss {
			continue
		}
		if _, ok := paths[id]; !ok {
			c.warnf(diag.UnobtainableMonster, "enemies."+id, "monster %q has no path from capturable species", id)
		}
	}
}
