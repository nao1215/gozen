package testgame

import "strings"

// MonsterFiles is an original two-species collection fixture for contract tests.
// Mote is caught after victory; two raised Motes can breed a Sentinel.
func MonsterFiles() map[string]string {
	d := strings.Replace(Data, "  mote:\n", "  mote:\n    family: slime\n    rank: 1\n    monster_class: ranger\n    recruit: {chance: 1, level: 1, bonus_item: potion, bonus: 0}\n", 1)
	d = strings.Replace(d, "  boss:\n    name: Sentinel", "  boss:\n    family: slime\n    rank: 2\n    monster_class: ranger\n    name: Sentinel", 1)
	return map[string]string{
		"game.yaml":          "schema_version: 6\nid: monster-test\n" + Game,
		"data/main.yaml":     d,
		"data/monsters.yaml": "monster_families: {slime: Slime}\nbreeding:\n  min_level: 2\n  skill_slots: 1\n  pairs: [{parents: [mote, mote], child: boss}]\n  families: [{parents: [slime, slime], children: [mote, boss]}]\n",
	}
}
