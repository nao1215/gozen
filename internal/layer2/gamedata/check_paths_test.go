package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestCheckRejectsMistakes: every mistake in a game is reported with the place that holds it, and loading fails.
// Each case changes one thing in the neutral fixture game.
func TestCheckRejectsMistakes(t *testing.T) {
	game := func(extra string) map[string]string { return map[string]string{"game.yaml": testgame.Game + extra} }
	data := func(y string) map[string]string {
		y = strings.ReplaceAll(y, "@C", "stats: {mhp: '1', mmp: '1', atk: '1', def: '1', mat: '1', mdf: '1', agi: '1', luk: '1'}, exp_for: level")
		y = strings.ReplaceAll(y, "@E", "stats: {mhp: 1, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}")
		return map[string]string{"data/z.yaml": y}
	}
	noLoc := strings.Replace(testgame.Game, "location: camp, ", "", 1)
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"start location missing", map[string]string{"game.yaml": noLoc}, "start.location is missing"},
		{"unknown language", game("language: xx\n"), `"xx" is not a built-in language`},
		{"voice needs version 4", game("ui: {voice: {cut: true}}\n"), "ui.voice needs schema_version: 4"},
		{"rollback needs version 4", game("ui: {rollback: 3}\n"), "ui.rollback needs schema_version: 4"},
		{"balloon needs version 3", game("ui: {balloon: 0.05}\n"), "ui.balloon needs schema_version: 3"},
		{"talk portrait needs version 3", game("ui: {talk_portrait: {x: 0.5}}\n"), "ui.talk_portrait needs schema_version: 3"},
		{"title text needs version 3", game("title_text: {size: 4}\n"), "title_text needs schema_version: 3"},
		{"gallery picture without a title", game("gallery: {cgs: [{file: x.png, flag: a}]}\n"), "a picture of the gallery needs a title"},
		{"gallery scene with a missing picture", game("gallery: {scenes: [{title: A, event: sub, flag: a, seen: gone.png}]}\n"), "no file assets/gone.png"},
		{"gallery music pictures without pictures", map[string]string{"game.yaml": testgame.Game + "gallery: {music_pictures: {theme: []}}\n", "data/z.yaml": "music: {songs: {theme: {file: x.png}}}"}, "a song needs one picture or more"},
		{"gallery music picture empty", game("gallery: {music_pictures: {theme: ['']}}\n"), "an empty picture"},
		{"meter without a value", game("meters: [{label: Hunger}]\n"), "a meter needs value"},
		{"meter of an unknown actor", game("meters: [{value: '1', actor: nobody}]\n"), `unknown actor "nobody"`},
		{"card bubble of an unknown actor", game("card_bubbles: [{actor: nobody, when: 'true'}]\n"), `unknown actor "nobody"`},
		{"card bubble of an unknown state", game("card_bubbles: [{state: nostate}]\n"), `unknown state "nostate"`},
		{"card bubble negative", game("card_bubbles: [{size: -1}]\n"), "must not be negative"},
		{"aside without a speaker", game("asides: [{call: sub}]\n"), "an aside needs a speaker"},
		{"aside of an unknown actor", game("asides: [{actor: nobody, call: sub}]\n"), `unknown actor "nobody"`},
		{"aside with a bad face pattern", game("asides: [{face: '[', call: sub}]\n"), "bad pattern"},
		{"aside with a bad skip pattern", game("asides: [{name: X, skip: ['['], call: sub}]\n"), "bad pattern"},
		{"aside calling nothing", game("asides: [{name: X, call: nope}]\n"), `unknown event "nope"`},
		{"memo kind without a label", game("memo: {kinds: [{id: a}]}\n"), "a kind of note needs an id of its own and a label"},
		{"memo kind twice", game("memo: {kinds: [{id: a, label: A}, {id: a, label: B}]}\n"), "a kind of note needs an id of its own and a label"},
		{"memo var without a value", game("memo: {vars: [{name: a}]}\n"), "a number kept with the notes needs a name and a value"},
		{"cg review tag twice", game("cg_review: {tags: [{id: a, label: A, ok: true}, {id: a, label: B}]}\n"), "a tag needs an id of its own and a label"},
		{"cg review without an ok tag", game("cg_review: {tags: [{id: a, label: A}]}\n"), "one tag (and only one) is ok: true"},
		{"cg review group without files", game("cg_review: {groups: [{label: G}]}\n"), "a group needs a label and files"},
		{"cg review group with a bad pattern", game("cg_review: {groups: [{label: G, files: ['[']}]}\n"), "bad pattern"},
		{"cg review extra with a bad pattern", game("cg_review: {extra: ['[']}\n"), "bad pattern"},
		{"item with an unknown stat", data("items: {ring: {name: R, stats: {power: 1}}}"), `unknown stat "power"`},
		{"item with an unknown slot", data("items: {ring: {name: R, kind: sword, slot: finger}}"), `unknown slot "finger"`},
		{"command item without an event", data("items: {ring: {name: R, command: true}}"), "command needs an event"},
		{"item state of nothing", data("items: {ring: {name: R, kind: sword, slot: weapon, states: [{state: nope}]}}"), `unknown state "nope"`},
		{"state resist on a plain item", data("items: {rock: {name: R, state_resist: {sleep: 0.5}}}"), "state_resist is for equipment"},
		{"state restrict unknown", data("states: {dizzy: {name: D, restrict: fly}}"), "want skip or confuse"},
		{"state restrict chance without skip", data("states: {dizzy: {name: D, restrict: confuse, restrict_chance: 0.5}}"), "want 0 < chance < 1 with restrict: skip"},
		{"state unknown stat", data("states: {dizzy: {name: D, stats: {power: 1}}}"), `unknown stat "power"`},
		{"state resist unknown state", data("states: {dizzy: {name: D, state_resist: {nope: 0.5}}}"), `unknown state "nope"`},
		{"class unknown stat", data("classes: {c3: {name: C, stats: {power: '1', mhp: '1', mmp: '1', atk: '1', def: '1', mat: '1', mdf: '1', agi: '1', luk: '1'}, exp_for: level}}"), `unknown stat "power"`},
		{"class unknown skill", data("classes: {c3: {name: C, @C, skills: [{level: 1, skill: nope}]}}"), `unknown skill "nope"`},
		{"actor equips into an unknown slot", data("actors: {x: {name: X, class: ranger, equip: {finger: hat}}}"), `unknown slot "finger"`},
		{"actor equips an item of another slot", data("actors: {x: {name: X, class: ranger, equip: {weapon: hat}}}"), `hat goes in "armor"`},
		{"actor face box of two numbers", data("actors: {x: {name: X, class: ranger, face_box: [1, 2]}}"), "want [x, y, size]"},
		{"actor cutin box of two numbers", data("actors: {x: {name: X, class: ranger, cutin_box: [1, 2]}}"), "want [x, y, size]"},
		{"actor face shade over 1", data("actors: {x: {name: X, class: ranger, faces: p, face_shade: 2}}"), "face_shade: want 0 to 1"},
		{"actor face shade top under 0", data("actors: {x: {name: X, class: ranger, faces: p, face_shade_top: -1}}"), "face_shade_top: want 0 to 1"},
		{"actor face shade without faces", data("actors: {x: {name: X, class: ranger, face_shade: 0.5}}"), "needs faces"},
		{"actor reaction face missing", data("actors: {x: {name: X, class: ranger, faces: p, reactions: {hurt: ouch}}}"), "no face assets/p/ouch.png"},
		{"actor face rule face missing", data("actors: {x: {name: X, class: ranger, faces: p, face_rules: [{when: 'true', face: gone}]}}"), "no face assets/p/gone.png"},
		{"skill unknown fx", data("skills: {z: {name: Z, scope: one_enemy, fx: sparkle, effects: [{damage: '1'}]}}"), `unknown fx "sparkle"`},
		{"skill bad where", data("skills: {z: {name: Z, scope: one_enemy, where: [sky], effects: [{damage: '1'}]}}"), "want battle or menu"},
		{"skill unknown scope", data("skills: {z: {name: Z, scope: everywhere, effects: [{damage: '1'}]}}"), `unknown scope "everywhere"`},
		{"effect turns without add_state", data("skills: {z: {name: Z, scope: one_enemy, effects: [{damage: '1', turns: '2'}]}}"), "turns goes with add_state"},
		{"effect stat unknown", data("skills: {z: {name: Z, scope: one_enemy, effects: [{damage: '1', stat: sp}]}}"), "want hp or mp"},
		{"effect remove state unknown", data("skills: {z: {name: Z, scope: one_ally, effects: [{remove_state: nope}]}}"), `unknown state "nope"`},
		{"effect recoil over 1", data("skills: {z: {name: Z, scope: one_enemy, effects: [{damage: '1', recoil: 2}]}}"), "recoil is a part of the damage"},
		{"effect with two actions", data("skills: {z: {name: Z, scope: one_enemy, effects: [{damage: '1', heal: '1'}]}}"), "an effect needs exactly one of"},
		{"effect with no action", data("skills: {z: {name: Z, scope: one_enemy, effects: [{}]}}"), "an effect needs exactly one of"},
		{"effect summon unknown", data("skills: {z: {name: Z, scope: none, effects: [{summon: nope}]}}"), `unknown enemy "nope"`},
		{"effect unknown element", data("skills: {z: {name: Z, scope: one_enemy, effects: [{damage: '1', element: wind}]}}"), `unknown element "wind"`},
		{"effect variance out of range", data("skills: {z: {name: Z, scope: one_enemy, effects: [{damage: '1', variance: 2}]}}"), "variance"},
		{"class ai unknown", data("classes: {c3: {name: C, @C, ai: nope}}"), `unknown ai "nope"`},
		{"rule with both skill and item", data("ai: {a: [{use: attack, item: potion}]}"), "a rule needs exactly one of use"},
		{"rule with neither skill nor item", data("ai: {a: [{when: 'true'}]}"), "a rule needs exactly one of use"},
		{"rule with an item without use", data("ai: {a: [{item: key}]}"), "use"},
		{"rule target preset unknown", data("ai: {a: [{use: attack, target: sideways}]}"), `unknown target "sideways"`},
		{"rule target side unknown", data("ai: {a: [{use: attack, target: {side: up}}]}"), "want foes or allies"},
		{"rule target pick unknown", data("ai: {a: [{use: attack, target: {pick: best}}]}"), "want min, max, random or first"},
		{"rule target pick without by", data("ai: {a: [{use: attack, target: {pick: min}}]}"), "pick min needs by"},
		{"enemy face rule unknown face", data("enemies: {e1: {name: E, image: x.png, @E, faces: {a: x.png}, face_rules: [{when: 'true', face: b}]}}"), `unknown face "b"`},
		{"enemy hard negative", data("enemies: {e1: {name: E, image: x.png, @E, hard: -1}}"), "hard: want 0 or more"},
		{"enemy resists an unknown element", data("enemies: {e1: {name: E, image: x.png, @E, resist: {wind: 2}}}"), `unknown element "wind"`},
		{"enemy resists an unknown state", data("enemies: {e1: {name: E, image: x.png, @E, state_resist: {nope: 0.5}}}"), `unknown state "nope"`},
		{"enemy face missing file", data("enemies: {e1: {name: E, image: x.png, @E, faces: {a: gone.png}}}"), "no file assets/gone.png"},
		{"troop level scale level zero", data("troops: {b: {members: [mote], level_scale: {level: 0, rate: 0.05, max: 0.2}}}"), "level_scale.level"},
		{"troop level scale rate over cap", data("troops: {b: {members: [mote], level_scale: {level: 5, rate: 0.5, max: 0.2}}}"), "level_scale.rate"},
		{"location resume chain", data("locations: {a1: {name: A, resume_to: a2}, a2: {name: B, resume_to: camp}}"), "has resume_to too"},
		{"location resume unknown", data("locations: {a1: {name: A, resume_to: nope}}"), `unknown location "nope"`},
		{"two people with one id", data("locations: {a1: {name: A, people: [{id: p, name: P}, {id: p, name: Q}]}}"), `duplicate entry id "p"`},
		{"people and actions share ids", data("locations: {a1: {name: A, people: [{id: p, name: P}], actions: [{id: p, label: L, do: []}]}}"), `duplicate entry id "p"`},
		{"exit with nowhere to go", data("locations: {a1: {name: A, exits: [{label: L}]}}"), "an exit needs to or do"},
		{"event trigger unknown", data("events: {ev: {trigger: sometimes, do: []}}"), "want enter or auto"},
		{"event location unknown", data("events: {ev: {trigger: enter, location: nope, do: []}}"), `unknown location "nope"`},
		{"shop spent not a var", data("shops: {s: {goods: [potion], spent: gold}}"), "want var.<name>"},
		{"shop unknown goods", data("shops: {s: {goods: [nope]}}"), `unknown item "nope"`},
		{"music song without bpm", data("music: {songs: {t: {dir: music, moods: {calm: a}}}}"), "bpm is missing"},
		{"music track with moods", data("music: {songs: {t: {file: x.png, moods: {calm: a}}}}"), "a track (file) has no moods"},
		{"music rule of an unknown context", data("music: {rules: {sky: [{mood: calm}]}}"), `unknown context "sky"`},
		{"music rule without anything to play", data("music: {rules: {field: [{when: 'true'}]}}"), "a rule needs a mood or a song to play"},
		{"music rule of an unknown song", data("music: {rules: {field: [{play: nope}]}}"), `unknown song "nope"`},
		{"hurt without an amount", data("events: {e: {do: [{hurt: {who: all}}]}}"), "hurt needs an amount"},
		{"affect without add or remove", data("events: {e: {do: [{affect: {who: all}}]}}"), "affect needs add or remove"},
		{"hurt an unknown actor", data("events: {e: {do: [{hurt: {who: nobody, amount: '1'}}]}}"), `unknown actor (or all) "nobody"`},
		{"heal an unknown actor", data("events: {e: {do: [{heal: nobody}]}}"), `unknown actor (or all) "nobody"`},
		{"reset tree of an unknown actor", data("events: {e: {do: [{reset_tree: nobody}]}}"), `unknown actor "nobody"`},
		{"learn for an unknown actor", data("events: {e: {do: [{learn: {actor: nobody, skill: fire}}]}}"), `unknown actor "nobody"`},
		{"learn an unknown skill", data("events: {e: {do: [{learn: {actor: scout, skill: nope}}]}}"), `unknown skill "nope"`},
		{"sp suggest with a bad name", data("events: {e: {do: [{sp_suggest: 'a b'}]}}"), "want a name for var.<name>"},
		{"stone suggest with a bad name", data("events: {e: {do: [{stone_suggest: 'a-b'}]}}"), "want a name for var.<name>"},
		{"equip stones without actor", data("events: {e: {do: [{equip_stones: {set: '1'}}]}}"), "actor and set are needed"},
		{"open profiles with none", data("events: {e: {do: [{open: profiles}]}}"), "open: profiles needs profiles"},
		{"open wall of no tile", data("events: {e: {do: [{open_wall: 'Q'}]}}"), "is not a tile of any dungeon"},
		{"open wall of two characters", data("events: {e: {do: [{open_wall: 'QQ'}]}}"), "is not a tile of any dungeon"},
		{"image with file and files", data("events: {e: {do: [{image: {file: x.png, files: [x.png]}}]}}"), "file or files, not both"},
		{"image files matching nothing", data("events: {e: {do: [{image: {files: ['cg/none*.png']}}]}}"), "no picture matches"},
		{"image fit unknown", data("events: {e: {do: [{image: {file: x.png, fit: stretch}}]}}"), "fit is contain or cover"},
		{"image fade negative", data("events: {e: {do: [{image: {file: x.png, fade: -1}}]}}"), "fade is seconds"},
		{"image auto without files", data("events: {e: {do: [{image: {file: x.png, auto: 2}}]}}"), "auto is seconds a picture"},
		{"image page without files", data("events: {e: {do: [{image: {file: x.png, page: 2}}]}}"), "page is 1 or more, with files"},
		{"image pick without files", data("events: {e: {do: [{image: {file: x.png, pick: '1'}}]}}"), "pick is with files"},
		{"image look unknown", data("events: {e: {do: [{image: {file: x.png, look: blur}}]}}"), "look is dream"},
		{"image haze without a look", data("events: {e: {do: [{image: {file: x.png, haze: '#aabbcc'}}]}}"),"haze is with look: dream"},
		{"image over missing", data("events: {e: {do: [{image: {file: x.png, over: {file: gone.png}}}]}}"), "no picture assets/gone.png"},
		{"battle unknown troop and fighter", data("events: {e: {do: [{battle: nope, fighters: [nobody]}]}}"), `unknown troop "nope"`},
		{"call unknown event", data("events: {e: {do: [{call: nope}]}}"), `unknown event "nope"`},
		{"shop command unknown", data("events: {e: {do: [{shop: nope}]}}"), `unknown shop "nope"`},
		{"join unknown actor", data("events: {e: {do: [{join: nobody}]}}"), `unknown actor "nobody"`},
		{"text with a broken expression", data("locations: {a1: {name: A, desc: 'go {1 +}'}}"), "in the text"},
		{"actor tree of a missing node", data("classes: {c3: {name: C, @C, sp: 1, start_sp: 1, tree: [{skill: fire}]}}\nactors: {x: {name: X, class: c3, tree: {nope: 1}}}"), `no node "nope"`},
		{"actor tree level too high", data("classes: {c3: {name: C, @C, sp: 1, start_sp: 5, tree: [{skill: fire, max: 2}]}}\nactors: {x: {name: X, class: c3, tree: {fire: 3}}}"), "level 3 (want 1..2)"},
		{"actor tree of a node that needs others", data("classes: {c3: {name: C, @C, sp: 1, start_sp: 5, tree: [{skill: fire}, {skill: heal, requires: [{node: fire}]}]}}\nactors: {x: {name: X, class: c3, tree: {heal: 1}}}"), "the node needs others first"},
		{"actor tree more than the SP", data("classes: {c3: {name: C, @C, sp: 0, start_sp: 1, tree: [{skill: fire, max: 3}]}}\nactors: {x: {name: X, class: c3, tree: {fire: 3}}}"), "more levels than the start's SP"},
		{"tree node with neither skill nor stats", data("classes: {c3: {name: C, @C, tree: [{id: a}]}}"), "a node needs skill or stats"},
		{"tree node with an unknown stat", data("classes: {c3: {name: C, @C, tree: [{id: a, stats: {power: 1}}]}}"), `unknown stat "power"`},
		{"tree requirement without a node", data("classes: {c3: {name: C, @C, tree: [{skill: fire}, {skill: heal, requires: [{level: 1}]}]}}"), "node is missing"},
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
