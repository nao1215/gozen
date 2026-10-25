package gamedata_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

const v2Head = "schema_version: 2\nid: v2\nstart: {location: camp, party: [scout]}\nequip_slots: [{id: weapon, name: Weapon}]\n"

const v2Data = `classes: {walker: {stats: {mhp: "9", mmp: "0", atk: "1", def: "1", mat: "1", mdf: "1", agi: "1", luk: "1"}, exp_for: "level * 9"}}
actors: {scout: {class: walker}}
items:
  tonic: {use: {scope: one_ally, where: [menu], effects: [{heal: "5"}]}}
  rock: {}
  blade: {slot: weapon}
skills: {attack: {}, guard: {}}
locations: {camp: {}}
`

func loadV2(head, events string) (*data.DB, error) {
	return data.Load(fstest.MapFS{
		"game.yaml":      {Data: []byte(head)},
		"data/main.yaml": {Data: []byte(v2Data + "events:\n  sub: {do: [{gold: 1}]}\n" + events)},
	})
}

// Checks of the spec version 2 vocabulary (ADR 0021): the error messages, and that they cannot be used in version 1.
func TestVersion2Checks(t *testing.T) {
	for _, tc := range []struct{ name, head, events, want string }{
		{"命令の打ち間違い", v2Head, "  e: {do: [{sya: hi}]}\n", `do[0].sya: unknown field "sya" (did you mean "say"?)`},
		{"版 1 でも打ち間違い", strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1), "  e: {do: [{wiat: 1}]}\n", `unknown field "wiat" (did you mean "wait"?)`},
		{"未宣言の独自の命令", v2Head, "  e: {do: [{roll_dice: 6}]}\n", `unknown field "roll_dice" (a command of the game's own is declared in game.yaml verbs)`},
		{"宣言した名前の打ち間違い", v2Head + "verbs: {roll_dice: sub}\n", "  e: {do: [{rol_dice: 6}]}\n", `did you mean "roll_dice"?`},
		{"独自の命令と既存の命令", v2Head + "verbs: {roll: sub}\n", "  e: {do: [{roll: 6, say: hi}]}\n", `unknown field "roll"`},
		{"場所の名前", v2Head, "  e: {do: [{set: {location: nowhere}}]}\n", `unknown location "nowhere"`},
		{"仲間の名前", v2Head, "  e: {do: [{set: {party: [scout, ghost]}}]}\n", `unknown actor "ghost"`},
		{"作戦の名前", v2Head, "  e: {do: [{set: {actor.scout.tactics: bold}}]}\n", "tactics is auto, manual or a preset"},
		{"宣言の名前", v2Head + "verbs: {say: sub}\n", "  e: {do: []}\n", "built-in command"},
		{"画面の名前", v2Head + "screens: {status: {title: Notes, entries: [{label: Entry, text: Welcome}]}}\n", "  e: {do: []}\n", "built-in screen"},
		{"未宣言の画面", v2Head, "  e: {do: [{open: album}]}\n", `not "album"`},
		{"向き", v2Head, "  e: {do: [{turn: up}]}\n", "want left, right or back"},
		{"歩数", v2Head, "  e: {do: [{step: -1}]}\n", "1 or more"},
		{"迷宮に無い文字", v2Head, "  e: {do: [{walk: Q}]}\n", "not a tile of any dungeon"},
		{"メニューで使えない道具", v2Head, "  e: {do: [{use: rock}]}\n", "cannot be used from the menu"},
		{"装備でない道具", v2Head, "  e: {do: [{equip: {actor: scout, item: rock}}]}\n", "not equipment"},
		{"無い枠", v2Head, "  e: {do: [{equip: {actor: scout, slot: head}}]}\n", "not an equip slot"},
		{"set の左辺", v2Head, "  e: {do: [{set: {mood: 1}}]}\n", `"mood": set changes`},
		{"仲間の値", v2Head, "  e: {do: [{set: {actor.scout.atk: 1}}]}\n", "set can change actor.<id>.hp"},
		{"無い道具の個数", v2Head, "  e: {do: [{set: {item.none: 1}}]}\n", "item"},
		{"版 1 の命令", strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1), "  e: {do: [{pick: Rest}]}\n", "pick needs schema_version: 2"},
		{"版 1 の set", strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1), "  e: {do: [{set: {gold: 1}}]}\n", "set gold needs schema_version: 2"},
		{"版 1 の宣言", strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1) + "verbs: {roll: sub}\n", "  e: {do: []}\n", "verbs needs schema_version: 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadV2(tc.head, tc.events)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	db, err := loadV2(v2Head+"verbs: {roll: sub}\nscreens: {album: {title: Notes, entries: [{label: Entry, text: Welcome}]}}\n", `  e: {do: [{roll: true}, {open: album}, {use: tonic, target: scout}, {equip: {actor: scout, item: blade}}, {pick: Rest}, {set: {gold: "gold + 1", party: [scout], location: camp, item.tonic: 2, actor.scout.tactics: auto}}, {if: "len(menu) > 0 && result == '' && !over && !cleared && len(repairs) == 0 && cell(0, 0) == nil && len(map) == 0 && actor.scout.tactics.mode == 'auto'"}]}
`)
	if err != nil {
		t.Fatal(err)
	}
	if db.Events["e"].Do[0].Call != "sub" {
		t.Fatal("custom command was not resolved")
	}
}

// set on abilities and levels is a warning in production data (author's decision 5).
func TestVersion2DevSetWarning(t *testing.T) {
	db, err := loadV2(v2Head, "  e: {do: [{set: {actor.scout.level: 5, gold: 1}}]}\n")
	if err != nil {
		t.Fatal(err)
	}
	w := db.SetWarnings()
	if len(w) != 1 || !strings.Contains(w[0].Error(), "actor.scout.level") || !strings.Contains(w[0].Error(), "development mode") {
		t.Fatalf("want one warning for the level, got %v", w)
	}
}

// Diagnose turns a located error into a Diagnostic and keeps the original error traceable with errors.Is.
func TestDiagnose(t *testing.T) {
	_, err := loadV2(v2Head, "  e: {do: [{battle: wolves}]}\n")
	ds := data.Diagnose(err, "error")
	if len(ds) != 1 || ds[0].File != "data/main.yaml" || ds[0].Line == 0 || ds[0].Col == 0 || ds[0].Path != "events.e.do[0].battle" || ds[0].Severity != "error" || !strings.Contains(ds[0].Message, "wolves") {
		t.Fatalf("diagnostic: %+v", ds)
	}
	if ds.Error() != err.Error() {
		t.Fatalf("the text changed:\n%s\n%s", ds.Error(), err)
	}
	_, err = data.Load(fstest.MapFS{})
	if ds := data.Diagnose(err, "error"); !errors.Is(ds, fs.ErrNotExist) || ds[0].File != "" {
		t.Fatalf("a missing file: %+v", ds)
	}
	if _, err := loadV2(strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1)+"verbs: {roll: sub}\n", "  e: {do: []}\n"); err == nil {
		t.Fatal("verbs accepted in version 1")
	} else if ds := data.Diagnose(err, "error"); ds[0].Path != "verbs" {
		t.Fatalf("game.yaml paths lose the file name: %+v", ds)
	}
}

// A state.strict declaration covers only flag and var; spec version 2 left-hand sides (gold, actor and so on) do not need it.
func TestVersion2SetInStrictGame(t *testing.T) {
	if _, err := loadV2(v2Head+"state: {strict: true}\n", "  e: {do: [{set: {gold: 1, actor.scout.tactics: auto, location: camp}}]}\n"); err != nil {
		t.Fatal(err)
	}
}

const vocabShop = `
shops:
  smith: {goods: [potion, blade]}
  closed: {goods: [potion], sell: false}
`

// TestVocabularyMistakes: mistakes in the version 2 vocabulary (declared verbs and screens, set, pick, act, equip, buy, sell).
func TestVocabularyMistakes(t *testing.T) {
	v2 := "schema_version: 2\nid: v\n" + testgame.Game
	game := func(extra string) map[string]string { return map[string]string{"game.yaml": v2 + extra} }
	both := func(extra, yaml string) map[string]string {
		return map[string]string{"game.yaml": v2 + extra, "data/z.yaml": yaml + vocabShop}
	}
	data := func(y string) map[string]string {
		return map[string]string{"game.yaml": v2, "data/z.yaml": y + vocabShop}
	}
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"verb with capitals", game("verbs: {Bad: sub}\n"), "want a name of lowercase letters, digits and _"},
		{"verb that is built in", game("verbs: {gold: sub}\n"), "is a built-in command"},
		{"verb twice", game("verbs: {dance: sub, dance: sub}\n"), `is defined twice`},
		{"verbs need version 2", map[string]string{"game.yaml": "schema_version: 1\nid: v\n" + testgame.Game + "verbs: {dance: sub}\n"}, "verbs needs schema_version: 2"},
		{"empty screen", game("screens: {journal: null}\n"), "expected a definition map (not null)"},
		{"screen with a space", game("screens: {'my screen': {title: Notes, entries: [{label: Entry, text: Welcome}]}}\n"), "want a name of lowercase letters"},
		{"screen that is built in", game("screens: {formation: {title: Notes, entries: [{label: Entry, text: Welcome}]}}\n"), "is a built-in screen"},
		{"screen twice", game("screens: {bag: {title: Notes, entries: [{label: Entry, text: Welcome}]}, bag: {title: Notes, entries: [{label: Entry, text: Welcome}]}}\n"), `is defined twice`},
		{"set of a quoted unknown location", data("events: {e: {do: [{set: {location: \"o'k\"}}]}}"), `unknown location "o'k"`},
		{"set that is a number", data("events: {e: {do: [{set: 3}]}}"), "expected a map"},
		{"set facing that is not a direction", data("events: {e: {do: [{set: {pos.facing: sideways}}]}}"), "facing is north, east, south or west"},
		{"set roamers to something", data("events: {e: {do: [{set: {roamers: all}}]}}"), "roamers can be set only to none"},
		{"set party too large", data("events: {e: {do: [{set: {party: [scout, guide, scout, guide, scout, guide, scout, guide]}}]}}"), "party takes 1 to"},
		{"set party of a stranger", data("events: {e: {do: [{set: {party: [scout, ghost]}}]}}"), `unknown actor "ghost"`},
		{"set an actor's name", data("events: {e: {do: [{set: {actor.scout.name: Bob}}]}}"), "set can change actor.<id>."},
		{"set tactics to nonsense", data("events: {e: {do: [{set: {actor.scout.tactics: nonsense}}]}}"), "tactics is auto, manual or a preset"},
		{"set tactics to an unknown preset", data("events: {e: {do: [{set: {actor.scout.tactics: {mode: preset, preset: ghost}}}]}}"), `tactics: no preset "ghost" in battle.tactic_presets`},
		{"set tactics to an unknown mode", data("events: {e: {do: [{set: {actor.scout.tactics: {mode: bold}}}]}}"), `tactics: mode is auto, manual or preset, not "bold"`},
		{"set tactics to a preset without its name", data("events: {e: {do: [{set: {actor.scout.tactics: {mode: preset}}}]}}"), "tactics: mode preset needs preset"},
		{"set tactics of broken rows", data("events: {e: {do: [{set: {actor.scout.tactics: {mode: manual, rows: 3}}}]}}"), "tactics: "},
		{"set an item that does not exist", data("events: {e: {do: [{set: {item.ghost: 1}}]}}"), `unknown item "ghost"`},
		{"set spot needs version 3", data("events: {e: {do: [{set: {spot: gate}}]}}"), "set spot needs schema_version: 3"},
		{"pick without a label", data("events: {e: {do: [{pick: ' '}]}}"), "pick needs the label"},
		{"act without skill or item", data("events: {e: {do: [{act: {actor: scout}}]}}"), "act takes a skill or an item"},
		{"act with both", data("events: {e: {do: [{act: {actor: scout, skill: attack, item: potion}}]}}"), "act takes a skill or an item"},
		{"act with an unknown item", data("events: {e: {do: [{act: {actor: scout, item: ghost}}]}}"), `unknown item "ghost"`},
		{"act at a stranger", data("events: {e: {do: [{act: {actor: scout, skill: attack, target: ghost}}]}}"), "is neither an actor nor an enemy"},
		{"equip an item into the wrong slot", data("events: {e: {do: [{equip: {actor: scout, item: hat, slot: weapon}}]}}"), "does not go in weapon"},
		{"buy a negative count", data("events: {e: {do: [{buy: {shop: smith, item: potion, count: -1}}]}}"), "count is 1 or more"},
		{"buy what the shop does not sell", data("events: {e: {do: [{buy: {shop: smith, item: hat}}]}}"), "does not sell hat"},
		{"sell a key", data("events: {e: {do: [{sell: {shop: smith, item: key}}]}}"), "cannot be sold"},
		{"sell at a shop that does not buy", data("events: {e: {do: [{sell: {shop: closed, item: potion}}]}}"), "cannot be sold at closed"},
		{"a verb nobody declared", data("events: {e: {do: [{dance: true}]}}"), "dance"},
	}
	_ = both
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := testgame.TryLoad(tt.files)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
}

// TestDeclaredVerbsAndSetAreAccepted: the same vocabulary written correctly loads.
func TestDeclaredVerbsAndSetAreAccepted(t *testing.T) {
	_, err := testgame.TryLoad(map[string]string{
		"game.yaml": "schema_version: 3\nid: v\n" + testgame.Game + "verbs: {dance: sub}\nscreens: {bag: {title: Notes, entries: [{label: Entry, text: Welcome}]}}\n",
		"data/z.yaml": vocabShop + `
events:
  e:
    do:
      - set:
          location: camp
          pos.facing: north
          party: [scout]
          actor.scout.tactics: auto
          item.potion: 3
          roamers: none
          gold: "gold + 1"
      - pick: Field
      - act: {actor: scout, skill: attack, target: mote}
      - equip: {actor: scout, item: stick}
      - buy: {shop: smith, item: potion, count: 2}
      - sell: {shop: smith, item: potion}
      - open: bag
      - dance: true
`,
	})
	if err != nil {
		t.Fatalf("correct vocabulary was refused: %v", err)
	}
}
