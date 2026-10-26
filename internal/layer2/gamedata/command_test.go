package gamedata_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// Positive cases are not built from the registry; they pin down how each command can actually be
// written.
var commandExamples = map[string]string{
	"capture": "capture: mote",
	"respond": "respond: {who: scout, text: Hello}",
	"say":     `say: hi`, "choice": `choice: [{label: Yes, do: []}]`,
	"if": `if: true`, "set": `set: {flag.ok: true}`, "give": `give: {item: key}`,
	"take": `take: {item: key}`, "gold": `gold: 0`, "join": `join: guide`,
	"leave": `leave: guide`, "heal": `heal: all`, "hurt": `hurt: {who: all, amount: 1}`,
	"reserve": `reserve: {in: monster_2, out: monster_1}`, "release": `release: monster_2`,
	"breed":  `breed: {parents: [monster_1, monster_2], skills: [fire]}`,
	"affect": `affect: {who: all, remove: bad}`, "battle": `battle: one`,
	"move": `move: deep`, "music": `music: ""`, "mood": `mood: calm`, "image": `image: ""`,
	"shop": `shop: store`, "call": `call: sub`, "wait": `wait: 1`, "notice": `notice: hi`,
	"sfx": `sfx: decide`, "exp": `exp: 0`, "learn": `learn: {actor: scout, skill: fire}`,
	"reset_tree": `reset_tree: all`, "reset_actor": `reset_actor: scout`, "effect": `effect: fade`, "stand": `stand: none`, "end": `end: true`,
	"clear": `clear: true`, "save": `save: true`, "open": `open: skills`,
	"dive": `dive: deep`, "open_wall": `open_wall: S`,
	"stone_suggest": `stone_suggest: offer`, "stone_apply": `stone_apply: offer`,
	"stone_choose": `stone_choose: offer`, "stone_wearer": `stone_wearer: offer`,
	"stone_decline": `stone_decline: offer`, "equip_stones": `equip_stones: {actor: "'scout'", set: "'pair'"}`,
	"sp_suggest": `sp_suggest: offer`, "sp_apply": `sp_apply: offer`, "sp_decline": `sp_decline: offer`,
	"pick": `pick: Rest`, "turn": `turn: left`, "step": `step: 2`, "walk": `walk: S`,
	"use": `use: potion`, "equip": `equip: {actor: scout, slot: weapon}`,
	"buy": `buy: {shop: store, item: potion, count: 2}`, "sell": `sell: {shop: store, item: potion}`,
	"act":   `act: {actor: scout, skill: attack}`,
	"order": `order: attack`, "end_turn": `end_turn: true`, "game_over": `game_over: true`,
}

func TestCommandFieldMatrix(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": madeDungeon + "\nshops: {store: {goods: [potion]}}\n"})
	db.Order["events"] = append(db.Order["events"], "matrix")
	fields := []struct {
		name, owners string
		values       []string
	}{
		{"prompt", "choice,stone_choose,stone_wearer", []string{`"Pick"`, `""`}},
		{"id", "choice", []string{`""`}}, // a named choice needs schema_version 4 (TestChoiceID)
		{"then", "if", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"else", "if", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"on_win", "battle", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"on_lose", "battle", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"on_escape", "battle", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"fighters", "battle", []string{`[scout]`, `[]`, `null`}},
		{"retry", "battle", []string{`true`, `false`, `null`}},
		{"recover", "battle", []string{`true`, `false`, `null`}},
		{"at", "move,dive", []string{`S`, `""`}},
		{"facing", "move,dive", []string{`north`, `""`}},
		{"floor", "dive", []string{`1`, `0`}},
		{"target", "use", []string{`scout`, `""`}},
		{"level", "reset_actor", []string{`2`}},
	}
	for f := range reflect.TypeFor[data.Command]().Fields() {
		role := f.Tag.Get("command")
		if role != "" && role != "verb" && role != "internal" && !slices.ContainsFunc(fields, func(field struct {
			name, owners string
			values       []string
		},
		) bool {
			return field.name == f.Tag.Get("yaml")
		}) {
			t.Errorf("%s の付随項目の検査例が無い", f.Name)
		}
	}
	check := func(t *testing.T, src, wantVerb, badField string) {
		t.Helper()
		var cmd data.Command
		if err := yaml.UnmarshalWithOptions([]byte("{"+src+"}"), &cmd, yaml.Strict()); err != nil {
			t.Fatal(err)
		}
		verb, err := cmd.Verb()
		db.Events["matrix"] = &data.Event{Do: []data.Command{cmd}}
		errs := db.Check()
		campaign := wantVerb == "order" || wantVerb == "end_turn" || wantVerb == "game_over"
		if slices.Contains(data.NewVerbs, wantVerb) || wantVerb == "stand" || campaign { // a spec version 2 (stand: 4, a campaign's: 5) command yields only a version error in older versions
			var rest []error
			for _, e := range errs {
				if !strings.Contains(e.Error(), "needs schema_version: 2") && !strings.Contains(e.Error(), "stand needs schema_version: 4") && !strings.Contains(e.Error(), "needs schema_version: 5") {
					rest = append(rest, e)
				}
			}
			if len(errs) == len(rest) && badField == "" {
				t.Fatalf("%s を版 1 以前で受け入れた", src)
			}
			errs = rest
		}
		if badField != "" {
			if err == nil || !strings.Contains(err.Error(), badField) || len(errs) == 0 || !strings.Contains(fmt.Sprint(errs), badField) {
				t.Fatalf("不正な付随項目 %s を拒否しない。Verb=%q, %v、Check=%v", badField, verb, err, errs)
			}
		} else if err != nil || verb != wantVerb || len(errs) > 0 {
			t.Fatalf("正例 %s が失敗。Verb=%q, %v、Check=%v", src, verb, err, errs)
		}
	}
	for verb, src := range commandExamples {
		if verb == "capture" || verb == "respond" || verb == "reserve" || verb == "release" || verb == "breed" {
			continue
		} // covered with schema version 6 in monster_test.go
		t.Run(verb, func(t *testing.T) {
			check(t, src, verb, "")
			for _, field := range fields {
				for _, value := range field.values {
					t.Run(field.name+"="+value, func(t *testing.T) {
						bad := field.name
						if slices.Contains(strings.Split(field.owners, ","), verb) {
							bad = ""
						}
						check(t, src+", "+field.name+": "+value, verb, bad)
					})
				}
			}
		})
	}
}

// Old and new versions, event and place direct commands, and nesting all keep an explicitly
// written zero value.
func TestCommandFieldPresence(t *testing.T) {
	for _, version := range []int{0, 1} {
		for _, direct := range []bool{false, true} {
			for _, branch := range []string{"%s", "{if: true, then: [%s]}", "{choice: [{label: Yes, do: [%s]}]}", "{battle: one, on_win: [%s]}"} {
				for _, field := range []string{"fighters: []", "retry: false", "floor: 0", `at: ""`, `prompt: ""`, "on_escape: []", "save: false", "wait: 0"} {
					t.Run(fmt.Sprintf("版%d/直接%t/%s/%s", version, direct, branch, field), func(t *testing.T) {
						src := "events: {invalid: {do: [" + fmt.Sprintf(branch, "{say: hi, "+field+"}") + "]}}"
						files := map[string]string{"game.yaml": fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + testgame.Game}
						if direct {
							files["data/z.yaml"] = "locations: {direct: {on_enter: [" + fmt.Sprintf(branch, "{say: hi, "+field+"}") + "]}}"
						} else {
							files["data/z.yaml"] = src
						}
						_, err := testgame.TryLoad(files)
						if err == nil || !strings.Contains(err.Error(), strings.Split(field, ":")[0]) {
							t.Fatalf("明示した項目を見失った。%v", err)
						}
					})
				}
			}
		}
	}
}

func TestCommandExamplesCoverRegistry(t *testing.T) {
	for f := range reflect.TypeFor[data.Command]().Fields() {
		if f.Tag.Get("command") == "verb" {
			if _, ok := commandExamples[f.Tag.Get("yaml")]; !ok {
				t.Errorf("%s の正例が無い", f.Name)
			}
		}
	}
}

func TestCommandFieldValidation(t *testing.T) {
	for _, src := range []string{
		`{say: hi, done: tile}`, `{say: hi, unknown: false}`,
		`{stone_choose: offer, prompt: "{gld}"}`, `{stone_wearer: offer, prompt: "{gld}"}`,
		`{end: false}`, `{clear: false}`, `{save: false}`, `{join: ""}`,
	} {
		t.Run(src, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/z.yaml": "events: {invalid: {do: [" + src + "]}}"})
			if err == nil {
				t.Fatal("不正な命令を受け入れた")
			}
		})
	}
	cmd := data.Command{Done: "tile"}
	if verb, err := cmd.Verb(); err != nil || verb != "done" {
		t.Fatalf("内部の done が動かない。%s, %v", verb, err)
	}
}

func TestCaptureCommandAdmissionValidation(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	for _, species := range []string{"mote", "boss", "missing"} {
		ds := data.CheckScript(db, "- capture: "+species+"\n", false)
		if species == "mote" && len(ds) > 0 || species != "mote" && len(ds) == 0 {
			t.Errorf("capture %s: %v", species, ds)
		}
	}
	files := testgame.MonsterFiles()
	files["game.yaml"] = strings.Replace(files["game.yaml"], "schema_version: 6", "schema_version: 5", 1)
	files["data/capture.yaml"] = "events: {admit: {do: [{capture: mote}]}}\n"
	if _, err := testgame.TryLoad(files); err == nil || !strings.Contains(err.Error(), "events.admit.do[0].capture") {
		t.Fatalf("capture before schema version 6: %v", err)
	}
}
