package gamedata_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
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
		"data/main.yaml": {Data: []byte(v2Data + "events:\n" + events)},
	})
}

// Checks of the spec version 2 vocabulary (ADR 0021): the error messages, and that they cannot be used in version 1.
func TestVersion2Checks(t *testing.T) {
	for _, tc := range []struct{ name, head, events, want string }{
		{"命令の打ち間違い", v2Head, "  e: {do: [{sya: hi}]}\n", `do[0].sya: unknown field "sya" (did you mean "say"?)`},
		{"版 1 でも打ち間違い", strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1), "  e: {do: [{wiat: 1}]}\n", `unknown field "wiat" (did you mean "wait"?)`},
		{"未宣言の独自の命令", v2Head, "  e: {do: [{roll_dice: 6}]}\n", `unknown field "roll_dice" (a command of the game's own is declared in game.yaml verbs)`},
		{"宣言した名前の打ち間違い", v2Head + "verbs: [roll_dice]\n", "  e: {do: [{rol_dice: 6}]}\n", `did you mean "roll_dice"?`},
		{"独自の命令と既存の命令", v2Head + "verbs: [roll]\n", "  e: {do: [{roll: 6, say: hi}]}\n", `unknown field "roll"`},
		{"場所の名前", v2Head, "  e: {do: [{set: {location: nowhere}}]}\n", `unknown location "nowhere"`},
		{"仲間の名前", v2Head, "  e: {do: [{set: {party: [scout, ghost]}}]}\n", `unknown actor "ghost"`},
		{"作戦の名前", v2Head, "  e: {do: [{set: {actor.scout.tactics: bold}}]}\n", "tactics is auto, manual or a preset"},
		{"宣言の名前", v2Head + "verbs: [say]\n", "  e: {do: []}\n", "built-in command"},
		{"画面の名前", v2Head + "screens: [status]\n", "  e: {do: []}\n", "built-in screen"},
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
		{"版 1 の宣言", strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1) + "verbs: [roll]\n", "  e: {do: []}\n", "verbs needs schema_version: 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadV2(tc.head, tc.events)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	db, err := loadV2(v2Head+"verbs: [roll]\nscreens: [album]\n", `  e: {do: [{roll: {sides: 6}}, {open: album}, {use: tonic, target: scout}, {equip: {actor: scout, item: blade}}, {pick: Rest}, {set: {gold: "gold + 1", party: [scout], location: camp, item.tonic: 2, actor.scout.tactics: auto}}, {if: "len(menu) > 0 && result == '' && !over && !cleared && len(repairs) == 0 && cell(0, 0) == nil && len(map) == 0 && actor.scout.tactics.mode == 'auto'"}]}
`)
	if err != nil {
		t.Fatal(err)
	}
	if name, arg := data.CustomVerb(&db.Events["e"].Do[0]); name != "roll" || arg == nil {
		t.Fatalf("custom verb: %q %v", name, arg)
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
	if _, err := loadV2(strings.Replace(v2Head, "schema_version: 2", "schema_version: 1", 1)+"verbs: [roll]\n", "  e: {do: []}\n"); err == nil {
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
