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

// 正例は登録表から作らず、各命令の実際に読める書き方を固定する。
var commandExamples = map[string]string{
	"say": `say: hi`, "choice": `choice: [{label: Yes, do: []}]`,
	"if": `if: true`, "set": `set: {flag.ok: true}`, "give": `give: {item: key}`,
	"take": `take: {item: key}`, "gold": `gold: 0`, "join": `join: guide`,
	"leave": `leave: guide`, "heal": `heal: all`, "hurt": `hurt: {who: all, amount: 1}`,
	"affect": `affect: {who: all, remove: bad}`, "battle": `battle: one`,
	"move": `move: deep`, "music": `music: ""`, "mood": `mood: calm`, "image": `image: ""`,
	"shop": `shop: store`, "call": `call: sub`, "wait": `wait: 1`, "notice": `notice: hi`,
	"sfx": `sfx: decide`, "exp": `exp: 0`, "learn": `learn: {actor: scout, skill: fire}`,
	"reset_tree": `reset_tree: all`, "effect": `effect: fade`, "end": `end: true`,
	"clear": `clear: true`, "save": `save: true`, "open": `open: skills`,
	"dive": `dive: deep`, "open_wall": `open_wall: S`,
	"stone_suggest": `stone_suggest: offer`, "stone_apply": `stone_apply: offer`,
	"stone_choose": `stone_choose: offer`, "stone_wearer": `stone_wearer: offer`,
	"stone_decline": `stone_decline: offer`, "equip_stones": `equip_stones: {actor: "'scout'", set: "'pair'"}`,
	"sp_suggest": `sp_suggest: offer`, "sp_apply": `sp_apply: offer`, "sp_decline": `sp_decline: offer`,
}

func TestCommandFieldMatrix(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": madeDungeon + "\nshops: {store: {goods: [potion]}}\n"})
	db.Order["events"] = append(db.Order["events"], "matrix")
	fields := []struct {
		name, owners string
		values       []string
	}{
		{"prompt", "choice,stone_choose,stone_wearer", []string{`"Pick"`, `""`}},
		{"then", "if", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"else", "if", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"on_win", "battle", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"on_lose", "battle", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"on_escape", "battle", []string{`[{gold: 1}]`, `[]`, `null`}},
		{"fighters", "battle", []string{`[scout]`, `[]`, `null`}},
		{"retry", "battle", []string{`true`, `false`, `null`}},
		{"at", "move,dive", []string{`S`, `""`}},
		{"facing", "move,dive", []string{`north`, `""`}},
		{"floor", "dive", []string{`1`, `0`}},
	}
	for f := range reflect.TypeFor[data.Command]().Fields() {
		role := f.Tag.Get("command")
		if role != "" && role != "verb" && role != "internal" && !slices.ContainsFunc(fields, func(field struct {
			name, owners string
			values       []string
		}) bool {
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
		if badField != "" {
			if err == nil || !strings.Contains(err.Error(), badField) || len(errs) == 0 || !strings.Contains(fmt.Sprint(errs), badField) {
				t.Fatalf("不正な付随項目 %s を拒否しない。Verb=%q, %v、Check=%v", badField, verb, err, errs)
			}
		} else if err != nil || verb != wantVerb || len(errs) > 0 {
			t.Fatalf("正例 %s が失敗。Verb=%q, %v、Check=%v", src, verb, err, errs)
		}
	}
	for verb, src := range commandExamples {
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

// 旧版・新版、イベント・場所の直接命令、入れ子でも明示したゼロ値を失わない。
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
