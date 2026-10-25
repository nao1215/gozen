package gamedata_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestCheckPositionLevel(t *testing.T) {
	for _, condition := range []string{"pos.level == 0", "pos.level == 1", "pos.level + 1 > 0"} {
		t.Run(condition, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{
				"data/level-probe.yaml": fmt.Sprintf("events: {level_probe: {do: [{if: %q, then: [{say: Upstairs.}]}]}}", condition),
			})
			if err != nil {
				t.Fatalf("numeric position level must validate: %v", err)
			}
		})
	}
	for _, tc := range []struct{ command, want string }{
		{`if: "pos.unknown == 0", then: []`, `pos has no field "unknown"`},
		{`set: {pos.level: 1}`, `"pos.level": set changes`},
		{`set: {pos.unknown: 1}`, `"pos.unknown": set changes`},
	} {
		t.Run(tc.command, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{
				"data/level-probe.yaml": "events: {level_probe: {do: [{" + tc.command + "}]}}",
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

// Place of a generated dungeon (tests add content and use it). %s takes the dungeon's body.
const madeDungeon = `
locations:
  deep:
    name: Deep
    dungeon:
      generate:
        width: 11
        height: 11
        rooms: 2
        start: S
        place: [{char: ">", far: true}]
      tiles:
        S: {name: Up}
        ">": {name: Down, solid: "true", do: [{dive: deep, floor: "var.floor + 1"}]}
`

// TestCheckMore verifies that the checks added to check report mistakes with their location.
func TestCheckMore(t *testing.T) {
	game := func(extra string) string { return testgame.Game + extra }
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"回想の頁の題名", map[string]string{"game.yaml": game(`gallery: {scenes: [{title: A, event: sub, flag: a, pages: [{line: 1}]}]}`)}, `pages[0].title: 頁の題名が必要`},
		{"回想の頁の行", map[string]string{"game.yaml": game(`gallery: {scenes: [{title: A, event: sub, flag: a, pages: [{title: P, line: 0}]}]}`)}, `pages[0].line: 頁の行番号は１以上`},
		{"回想の頁の答え", map[string]string{"game.yaml": game(`gallery: {scenes: [{title: A, event: sub, flag: a, pages: [{title: P, line: 1, answers: [-1]}]}]}`)}, `pages[0].answers[0]: 頁の答えは０以上`},
		{"回想の頁の絵", map[string]string{"game.yaml": game(`gallery: {scenes: [{title: A, event: sub, flag: a, pages: [{title: P, line: 1, picture: missing.png}]}]}`)}, `pages[0].picture: no file`},
		{"回想の重複ID", map[string]string{"game.yaml": game(`gallery: {scenes: [{id: same, title: A, event: sub, flag: a}, {id: same, title: B, event: sub, flag: b}]}`)}, `gallery.scenes[1].id: 回想のキー`},
		{"回想の不正ID", map[string]string{"game.yaml": game(`gallery: {scenes: [{id: "bad/id", title: A, event: sub, flag: a}]}`)}, `gallery.scenes[0].id: 回想 ID`},
		{"旧キーだけでは移行しない", map[string]string{"game.yaml": game(`gallery: {scenes: [{legacy_keys: ["scene:a"], title: A, event: sub, flag: a}]}`)}, `旧キーの移行には回想 ID が必要`},
		{"別回想への旧キー重複", map[string]string{"game.yaml": game(`gallery: {scenes: [{id: first, legacy_keys: ["scene:old"], title: A, event: sub, flag: a}, {id: second, legacy_keys: ["scene:old"], title: B, event: sub, flag: b}]}`)}, `gallery.scenes[1].legacy_keys[0]: 回想のキー`},
		{"旧YAMLとのキー衝突", map[string]string{"game.yaml": game(`gallery: {scenes: [{title: A, event: sub, flag: old}, {id: second, legacy_keys: ["scene:old"], title: B, event: sub, flag: b}]}`)}, `gallery.scenes[1].legacy_keys[0]: 回想のキー`},
		{"空の旧キー", map[string]string{"game.yaml": game(`gallery: {scenes: [{id: first, legacy_keys: ["scene:"], title: A, event: sub, flag: a}]}`)}, `旧キーは scene:`},
		{"CGを回想へ移さない", map[string]string{"game.yaml": game(`gallery: {scenes: [{id: first, legacy_keys: ["cg:a"], title: A, event: sub, flag: a}]}`)}, `旧キーは scene:`},
		{"tile: bad slide", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `S: {name: Up}`, `S: {name: Up, slide: up-ish}`, 1)}, `tiles.S.slide: want north, east, south, west or facing`},
		{"tile: solid slide", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `S: {name: Up}`, `S: {name: Up, slide: north, solid: "true"}`, 1)}, `a slide is a floor one walks onto`},
		{"tile: bad spin", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `S: {name: Up}`, `S: {name: Up, spin: twice}`, 1)}, `tiles.S.spin: want right, left, back or random`},
		{"item: state_resist of no state", map[string]string{"data/z.yaml": `items: {charm: {name: C, kind: hat, slot: armor, state_resist: {nope: 0}}}`}, `items.charm.state_resist: unknown state "nope"`},
		{"item: a kind no class wears", map[string]string{"data/z.yaml": `items: {tome: {name: T, kind: book, slot: weapon, stats: {mat: 5}}}`}, `items.tome.kind: no class can wear a "book"`},
		{"actor: starts in gear the class cannot wear", map[string]string{"data/z.yaml": `actors: {third: {name: Third, class: ranger, equip: {armor: hat}}}`}, `actors.third.equip.armor: hat is a "hat", which a ranger cannot wear`},
		{"group: wearing no item", map[string]string{"data/z.yaml": `events: {e: {do: [{if: "party.wearing_nope > 0", then: []}]}}`}, `unknown item "nope"`},
		{"level_scale: no cap", map[string]string{"data/z.yaml": `troops: {b: {members: [mote], level_scale: {level: 5, rate: 0.05}}}`}, `troops.b.level_scale.max: 0 (want more than 0, up to 0.5)`},
		{"level_scale: cap too high", map[string]string{"data/z.yaml": `troops: {b: {members: [mote], level_scale: {level: 5, rate: 0.05, max: 2}}}`}, `troops.b.level_scale.max: 2 (want`},
		{"level_scale: unknown stat", map[string]string{"data/z.yaml": `troops: {b: {members: [mote], level_scale: {level: 5, rate: 0.05, max: 0.2, stats: [power]}}}`}, `troops.b.level_scale.stats: unknown stat "power"`},
		{"actor: no puppet file", map[string]string{"data/z.yaml": `actors: {third: {name: Third, class: ranger, puppet: p/none.yaml}}`}, `actors.third.puppet: no file assets/p/none.yaml`},
		{"actor: bad puppet", map[string]string{"data/z.yaml": `actors: {third: {name: Third, class: ranger, puppet: p/rig.yaml}}`, "assets/p/rig.yaml": "base: b.jpg\n"}, `actors.third.puppet: p/rig.yaml: base: picture "b.jpg" must be a relative PNG`},
		{"actor: puppet picture missing", map[string]string{"data/z.yaml": `actors: {third: {name: Third, class: ranger, puppet: p/rig.yaml}}`, "assets/p/rig.yaml": "base: base.png\n"}, `actors.third.puppet: no file assets/p/base.png`},
		{"say: a face the puppet does not have", map[string]string{"data/z.yaml": "actors: {third: {name: Third, class: ranger, faces: p, puppet: p/rig.yaml}}\nevents: {e: {do: [{say: {who: third, face: grin, text: Hi}}]}}", "assets/p/rig.yaml": "base: base.png\nfaces: {normal: {}}\n", "assets/p/base.png": "x", "assets/p/grin.png": "x"}, `third has no face "grin" (p/rig.yaml)`},
		{"actor: puppet without the face", map[string]string{"data/z.yaml": `actors: {third: {name: Third, class: ranger, puppet: p/rig.yaml}}`, "assets/p/rig.yaml": "base: base.png\nfaces: {smile: {}}\n", "assets/p/base.png": "x"}, `p/rig.yaml has no face "normal"`},
		// 1. stone sets and synthesis
		{"セットの系列IDが不正", map[string]string{"data/z.yaml": `stone_sets: {s: {family: "bad id", name: S, stones: [hat], stats: {atk: 1}}}`}, `stone_sets.s.family:`},
		{"stone set: unknown stone", map[string]string{"data/z.yaml": `stone_sets: {s: {name: S, stones: [nope], stats: {atk: 1}}}`}, `stone_sets.s.stones: unknown item "nope"`},
		{"stone set: not equipment", map[string]string{"data/z.yaml": `stone_sets: {s: {name: S, stones: [key], stats: {atk: 1}}}`}, `key is not equipment`},
		{"stone set: unknown skill", map[string]string{"data/z.yaml": `stone_sets: {s: {name: S, stones: [hat], skill: nope}}`}, `stone_sets.s.skill: unknown skill "nope"`},
		{"stone set: unknown stat", map[string]string{"data/z.yaml": `stone_sets: {s: {name: S, stones: [hat], stats: {power: 1}}}`}, `stone_sets.s.stats: unknown stat "power"`},
		{"stone set: no stones", map[string]string{"data/z.yaml": `stone_sets: {s: {name: S, stats: {atk: 1}}}`}, `stones is empty`},
		{"fusion: unknown from", map[string]string{"data/z.yaml": `fusions: [{from: [hat, nope], to: blade, price: 1}]`}, `fusions[0].from: unknown item "nope"`},
		{"fusion: unknown to", map[string]string{"data/z.yaml": `fusions: [{from: [hat, hat], to: nope, price: 1}]`}, `fusions[0].to: unknown item "nope"`},
		{"fusion: negative price", map[string]string{"data/z.yaml": `fusions: [{from: [hat, hat], to: blade, price: -5}]`}, `fusions[0].price: want 0 or more`},
		// 2. skill tree
		{"tree: unknown skill", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{id: a, skill: nope}]}}`}, `classes.c2.tree[0].skill: unknown skill "nope"`},
		{"tree: duplicate id", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{skill: fire}, {id: fire, stats: {atk: 1}}]}}`}, `the node id "fire" is used twice`},
		{"tree: unknown required node", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{skill: fire, requires: [{node: nope}]}]}}`}, `unknown node "nope"`},
		{"tree: required level over max", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{skill: fire, max: 3}, {skill: heal, requires: [{node: fire, level: 4}]}]}}`}, `want 1 to 3`},
		{"tree: circle", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{skill: fire, requires: [{node: heal}]}, {skill: heal, requires: [{node: fire}]}]}}`}, `go round in a circle`},
		{"tree: requires itself", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{skill: fire, requires: [{node: fire}]}]}}`}, `cannot require itself`},
		{"tree: node without id or skill", map[string]string{"data/z.yaml": `classes: {c2: {name: C, stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level", tree: [{stats: {atk: 1}}]}}`}, `a node needs id (or skill)`},
		// 3. a generated dungeon also goes through the shared tile checks and the placement-count
		// expressions
		{"made dungeon: bad look", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up, look: roof}", 1)}, `deep.dungeon.tiles.S.look: want floor, wall or door`},
		{"made dungeon: bad trigger", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up, trigger: touch}", 1)}, `tiles.S.trigger: want step, look or bump`},
		{"made dungeon: bad facing", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up, facing: up-left}", 1)}, `tiles.S.facing`},
		{"made dungeon: bad else", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up, else: Z}", 1)}, `tiles.S.else: "Z" is neither`},
		{"made dungeon: bad after", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up, once: true, after: Z}", 1)}, `tiles.S.after: "Z" is neither`},
		{"made dungeon: missing picture", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", "S: {name: Up, picture: nope.png}", 1)}, `tiles.S.picture: no picture assets/nope.png`},
		{"made dungeon: bad when", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", `S: {name: Up, when: "flagg.x"}`, 1)}, `tiles.S.when: "flagg.x" uses the unknown name "flagg"`},
		{"made dungeon: bad do", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "S: {name: Up}", `S: {name: Up, do: [{give: {item: nope}}]}`, 1)}, `tiles.S.do[0].give: unknown item "nope"`},
		{"made dungeon: bad count", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `far: true}`, `count: "flor + 1"}`, 1)}, `generate.place[0].count: "flor + 1" uses the unknown name "flor"`},
		{"made dungeon: at outside", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `far: true}`, `at: [11, 0]}`, 1)}, `generate.place[0].at: [11 0] is outside the map`},
		{"made dungeon: even size", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "width: 11", "width: 12", 1)}, `width and height are odd numbers`},
		{"made dungeon: huge width", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "width: 11", "width: 9223372036854775807", 1)}, `width and height must be at most 255 cells`},
		{"made dungeon: negative height", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "height: 11", "height: -1", 1)}, `width and height are 9 or more`},
		{"made dungeon: too many rooms", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "rooms: 2", "rooms: 1025", 1)}, `want at most 1024 rooms`},
		{"hand map: too wide", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "      generate:\n        width: 11\n        height: 11\n        rooms: 2\n        start: S\n        place: [{char: \">\", far: true}]", "      map: \""+strings.Repeat("#", 256)+"\"", 1)}, `map width and height must be at most 255 cells`},
		{"made dungeon: in a wall but not solid", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `far: true}`, `far: true}, {char: S, wall: true}`, 1)}, `generate.place[1].wall: a tile put in a wall cannot be walked into`},
		{"made dungeon: in a wall at", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `far: true}`, `wall: true, at: [3, 3]}`, 1)}, `generate.place[0].wall: wall is put by far or steps`},
		{"made dungeon: a swap to no picture", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "      generate:", "      pictures: {a.png: none.png}\n      generate:", 1)}, `dungeon.pictures.a.png: no picture`},
		{"made dungeon: flat with no picture", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `S: {name: Up}`, `S: {name: Up, flat: true}`, 1)}, `tiles.S.flat: a picture laid on the floor`},
		{"made dungeon: darkness", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, "      generate:", "      darkness: 1.5\n      generate:", 1)}, `want 0 to 1`},
		// 4. open and dive
		{"open: unknown screen", map[string]string{"data/z.yaml": `events: {e: {do: [{open: inventory}]}}`}, `want one of formation, skills, fusion, status, item, skill, equip, save, load, config, title, tactics, profiles, bestiary, ranch (or a screen declared in game.yaml screens), not "inventory"`},
		{"dive: not a made dungeon", map[string]string{"data/z.yaml": `events: {e: {do: [{dive: vault}]}}`}, `vault is not a made dungeon`},
		{"dive: unknown location", map[string]string{"data/z.yaml": `events: {e: {do: [{dive: nope}]}}`}, `unknown location "nope"`},
		{"dive: bad floor", map[string]string{"data/z.yaml": strings.Replace(madeDungeon, `floor: "var.floor + 1"`, `floor: "vr.floor + 1"`, 1)}, `.floor: "vr.floor + 1" uses the unknown name "vr"`},
		{"say: instead by no actor", map[string]string{"data/z.yaml": `events: {e: {do: [{say: {who: scout, text: hi, instead: [{who: Bob, text: yo}]}}]}}`}, `do[0].say.instead[0].who: "Bob" is not an actor`},
		{"say: instead in instead", map[string]string{"data/z.yaml": `events: {e: {do: [{say: {who: scout, text: hi, instead: [{who: guide, text: yo, instead: [{who: scout, text: no}]}]}}]}}`}, `instead in instead`},
		{"floor without dive", map[string]string{"data/z.yaml": `events: {e: {do: [{say: hi, floor: "1"}]}}`}, `floor belongs to dive, not say`},
		// 5. game_over.event
		{"game over event", map[string]string{"game.yaml": game("game_over: {event: nope}\n")}, `game.yaml game_over.event: unknown event "nope"`},
		// gallery (files diffs, recollection when, seen, set)
		{"gallery cg: file and files", map[string]string{"game.yaml": game("gallery: {cgs: [{file: x.png, files: [x.png], title: X}]}\n")}, `gallery.cgs[0]: file or files`},
		{"gallery cg: files match nothing", map[string]string{"game.yaml": game("gallery: {cgs: [{files: [cg/none.*.jpg], title: X}]}\n")}, `gallery.cgs[0].files: no picture matches`},
		{"gallery scene: nothing opens it", map[string]string{"game.yaml": game("gallery: {scenes: [{title: X, event: " + `e` + "}]}\n"), "data/z.yaml": "events: {e: {do: []}}"}, `needs a title and a flag, when or seen`},
		{"gallery scene: bad set", map[string]string{"game.yaml": game("gallery: {scenes: [{title: X, event: e, flag: f, set: {mood: 1}}]}\n"), "data/z.yaml": "events: {e: {do: []}}"}, `"mood": set changes flag.<name>, var.<name>, gold`},
		{"gallery scene: new set needs version 2", map[string]string{"game.yaml": game("gallery: {scenes: [{title: X, event: e, flag: f, set: {gold: 1}}]}\n"), "data/z.yaml": "events: {e: {do: []}}"}, `set gold needs schema_version: 2`},
		{"gallery scene: bad when", map[string]string{"game.yaml": game(`gallery: {scenes: [{title: X, event: e, when: "flagg.x"}]}` + "\n"), "data/z.yaml": "events: {e: {do: []}}"}, `gallery.scenes[0].when: "flagg.x" uses the unknown name`},
		{"gallery music pictures: unknown song", map[string]string{"game.yaml": game("gallery: {music_pictures: {nosong: [cg/none.jpg]}}\n")}, `gallery.music_pictures.nosong: unknown song "nosong"`},
		{"gallery music pictures: missing picture", map[string]string{"game.yaml": game("gallery: {music_pictures: {nosong: [cg/none.jpg]}}\n")}, `gallery.music_pictures.nosong[0]: no file assets/cg/none.jpg`},
		{"gallery scene: seen missing", map[string]string{"game.yaml": game("gallery: {scenes: [{title: X, event: e, seen: cg/none.jpg}]}\n"), "data/z.yaml": "events: {e: {do: []}}"}, `gallery.scenes[0].seen: no file assets/cg/none.jpg`},
		// 6. fields of fixed-type items
		{"group field typo", map[string]string{"data/z.yaml": `events: {e: {when: "party.min_hp_rtae < 0.5", do: []}}`}, `"party.min_hp_rtae < 0.5": party: a party member has no field "hp_rtae"`},
		{"aggregate of a non-number", map[string]string{"data/z.yaml": `events: {e: {when: "party.min_class > 0", do: []}}`}, `class is not a number`},
		{"member field typo", map[string]string{"data/z.yaml": `events: {e: {when: "party.members[0].hpp > 0", do: []}}`}, `party.members[]: a party member has no field "hpp"`},
		{"battler field typo", map[string]string{"data/z.yaml": `ai: {a: [{use: attack, when: "self.hp_rat < 0.5"}]}`}, `self: a battler has no field "hp_rat"`},
		{"field only member in battle", map[string]string{"data/z.yaml": `ai: {a: [{use: attack, when: "self.exp > 0"}]}`}, `a battler has no field "exp"`},
		{"unknown state", map[string]string{"data/z.yaml": `ai: {a: [{use: attack, when: "foes.with_poisn > 0"}]}`}, `unknown state "poisn"`},
		{"unknown state in states", map[string]string{"data/z.yaml": `skills: {s: {name: S, effects: [{damage: "1", when: "b.states.slep"}]}}`}, `b.states: unknown state "slep"`},
		{"unknown element", map[string]string{"data/z.yaml": `ai: {a: [{use: attack, target: {where: "t.resist_wind > 1"}}]}`}, `unknown element "wind"`},
		{"unknown item", map[string]string{"data/z.yaml": `events: {e: {when: "item.potoin > 0", do: []}}`}, `item: unknown item "potoin"`},
		{"unknown actor", map[string]string{"data/z.yaml": `events: {e: {when: "actor.hreo.in_party", do: []}}`}, `actor: unknown actor "hreo"`},
		{"field of a number", map[string]string{"data/z.yaml": `events: {e: {when: "gold.amount > 0", do: []}}`}, `gold: a number has no fields`},
		{"pos field typo", map[string]string{"data/z.yaml": `events: {e: {when: "pos.z > 0", do: []}}`}, `pos has no field "z"`},
		// 7. start.party
		{"start party empty", map[string]string{"game.yaml": strings.Replace(testgame.Game, "party: [scout]", "party: []", 1)}, `start.party is empty`},
		{"start party twice", map[string]string{"game.yaml": strings.Replace(testgame.Game, "party: [scout]", "party: [scout, scout]", 1)}, `scout is written twice`},
		{"start party over max", map[string]string{"game.yaml": strings.Replace(testgame.Game, "party: [scout]", "party: [scout, guide]", 1) + "max_party: 1\n"}, `more than max_party (1)`},
		// 8. text with embedded expressions
		{"person name", map[string]string{"data/z.yaml": `locations: {x: {name: X, people: [{name: "{nme}", talk: []}]}}`}, `locations.x.people[0].name: "nme" uses the unknown name "nme"`},
		{"action label", map[string]string{"data/z.yaml": `locations: {x: {name: X, actions: [{label: "Rest ({membres}G)", do: []}]}}`}, `locations.x.actions[0].label`},
		{"person new", map[string]string{"data/z.yaml": `locations: {x: {name: X, people: [{name: P, new: "!flg.x", talk: []}]}}`}, `locations.x.people[0].new`},
		{"action new", map[string]string{"data/z.yaml": `locations: {x: {name: X, actions: [{label: A, new: "self.hp > 0", do: []}]}}`}, `locations.x.actions[0].new`},
		{"exit label", map[string]string{"data/z.yaml": `locations: {x: {name: X, exits: [{to: camp, label: "{party.sizee}"}]}}`}, `locations.x.exits[0].label`},
		{"location desc", map[string]string{"data/z.yaml": `locations: {x: {name: X, desc: "B{vr.deepest}F"}}`}, `locations.x.desc`},
		{"choice label", map[string]string{"data/z.yaml": `events: {e: {do: [{choice: [{label: "{gld}G", do: []}]}]}}`}, `choice[0].label`},
		// 9. face of a speaker who is not a party member
		{"say face of a non-member", map[string]string{"data/z.yaml": `events: {e: {do: [{say: {who: Stranger, face: chars/nope.png, text: hi}}]}}`}, `events.e.do[0].say.face: no file assets/chars/nope.png`},
		// 10. affect's add allows only existing states (bad allows only remove)
		{"affect add bad", map[string]string{"data/z.yaml": `events: {e: {do: [{affect: {who: all, add: bad}}]}}`}, `events.e.do[0].affect.add: unknown state "bad"`},
		{"affect remove unknown", map[string]string{"data/z.yaml": `events: {e: {do: [{affect: {who: all, remove: nope}}]}}`}, `affect.remove: unknown state (or bad) "nope"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testgame.TryLoad(tt.files)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
}

// TestCheckAccepts verifies that correct writing is not reported as an error (preventing false
// positives).
func TestCheckAccepts(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{
		"game.yaml": testgame.Game + `
gallery:
  scenes:
    - {title: 旧一, event: sub, flag: shared}
    - {title: 旧二, event: sub, flag: shared}
    - {id: first, legacy_keys: ["scene:old/一", "scene:old/二"], title: 同名, event: sub, flag: a}
    - {id: second, title: 同名, event: sub, flag: b}
`,
		"assets/chars/stranger.png": "x",
		"assets/p/rig.yaml":         "base: base.png\nface: normal\nfaces: {normal: {}, smile: {}}\neyes: {closed: eyes.png}\n",
		"assets/p/normal.png":       "x",
		"assets/p/base.png":         "x",
		"assets/p/eyes.png":         "x",
		"data/z.yaml": madeDungeon + `
actors:
  fourth: {name: Fourth, class: ranger, faces: p, puppet: p/rig.yaml}
stone_sets:
  s: {name: S, stones: [hat, blade], skill: fire, stats: {atk: 1}}
fusions:
  - {from: [hat, hat], to: blade, price: 0}
classes:
  c2:
    name: C
    stats: {mhp: "1", mmp: "1", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}
    exp_for: "level"
    tree:
      - {skill: fire, max: 3}
      - {id: art, name: Art, stats: {atk: 2}}
      - {skill: heal, requires: [{node: fire, level: 3}, {node: art}]}
ai:
  a:
    - use: attack
      when: "self.hp_rate < 0.5 && allies.min_hp_rate < 0.3 && foes.with_poison == 0 && party.members[0].guarding == false"
      target: {where: "t.resist_fire > 1 && t.states.sleep == 0 && t.lv >= 1", pick: max, by: "t.hp"}
    - {use: attack, when: "boss.boss && enemies.avg_agi > party.sum_level && turn % 2 == 0 && phase == 'act'"}
    - {use: attack, when: "has(self.states, 'poison') && max(foes.max_resist_fire, 1) > 0 && self.fell_with.poison == 0"}
events:
  e:
    when: "party.min_hp_rate < 0.5 && party.avg_level > 1 && party.members[var.i].exp >= 0 && actor.scout.in_party && actor.guide.equip.weapon == ''"
    do:
      - say: {who: Stranger, face: chars/stranger.png, text: "{party.members[0].name} has {gold}G"}
      - say: {who: fourth, face: smile, text: "A puppet's face, not a picture in faces."}
      - say: {who: scout, text: "Go!", instead: [{who: guide, text: "Scout is down. Go!"}]}
      - choice: [{label: "{item.potion} potions", do: []}]
      - if: "flag.anything || var.anything > 0 || flag['x'] || pos.facing == 'north' && pos.x > 0"
        then: [{open: formation}, {open: skills}, {open: fusion}]
      - dive: deep
        floor: "var.gate + 1"
      - affect: {who: all, add: poison}
      - affect: {who: scout, remove: bad}
      - if: "pages('cg/x') >= 2"
        then: [{wait: 1}]
  f:
    when: "rand() < 0.5 && chance(0.5) && steps > 0 && members > 0 && location == 'camp'"
    do: [{set: {var.x: "floor(var.x / 2)"}}]
`,
	})
	if err != nil {
		t.Fatalf("a correct game was reported: %v", err)
	}
}

// TestDarkness verifies that an unspecified darkness (default 0.5) is distinguished from 0.
func TestDarkness(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
locations:
  lit:
    name: Lit
    dungeon:
      darkness: 0
      map: |
        ###
        #S#
        ###
      tiles: {S: {start: true}}
      looks:
        - {when: "true", darkness: 0}
        - {when: "true"}
  plain:
    name: Plain
    dungeon:
      map: |
        ###
        #S#
        ###
      tiles: {S: {start: true}}
`})
	lit, plain := db.Locations["lit"].Dungeon, db.Locations["plain"].Dungeon
	if lit.Dark != 0 {
		t.Errorf("darkness: 0 became %v", lit.Dark)
	}
	if plain.Dark != 0.5 {
		t.Errorf("no darkness should be 0.5, got %v", plain.Dark)
	}
	if lk := lit.Looks[0]; !lk.HasDark() || lk.Dark != 0 {
		t.Errorf("a look's darkness: 0 is lost: %+v", lk)
	}
	if lit.Looks[1].HasDark() {
		t.Error("a look without darkness says it has one")
	}
	lit.SetMap("###\n#S#\n###") // the written 0 stays even after a rebuild
	if lit.Dark != 0 {
		t.Errorf("darkness: 0 became %v after SetMap", lit.Dark)
	}
}
