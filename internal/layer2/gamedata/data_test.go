package gamedata_test

import (
	"strings"
	"testing"
	"testing/fstest"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestLoad(t *testing.T) {
	db := testgame.Load(t, nil)
	if got := db.Ordered("items"); strings.Join(got, ",") != "potion,stick,blade,hat,key" {
		t.Errorf("items in order = %v", got)
	}
	if db.Game.Battle.Attack != "attack" || db.Game.MaxParty != 4 || len(db.Game.Slots) != 3 {
		t.Errorf("defaults not filled: %+v", db.Game)
	}
	if db.Skills["attack"].Hits != 1 || db.Skills["attack"].Where[0] != "battle" {
		t.Error("skill defaults not filled")
	}
}

// TestCheckReports checks that each mistake in the data is reported with where it is.
func TestCheckReports(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"unknown item", `locations: {x: {name: X, actions: [{label: a, do: [{give: {item: nope}}]}]}}`, `unknown item "nope"`},
		{"unknown name in an expression", `locations: {x: {name: X, actions: [{label: a, when: "flagg.x", do: []}]}}`, `unknown name "flagg"`},
		{"two verbs", `events: {e: {do: [{say: hi, gold: "1"}]}}`, `several verbs`},
		{"no verb", `events: {e: {do: [{}]}}`, `without a verb`},
		{"then without if", `events: {e: {do: [{say: hi, then: []}]}}`, `then belongs to if`},
		{"bad expression", `events: {e: {do: [{if: "1 +", then: []}]}}`, `unexpected end`},
		{"unknown field", `events: {e: {doo: []}}`, `unknown field "doo"`},
		{"duplicate id", `items: {potion: {name: P}}`, `items.potion is defined twice`},
		{"unknown troop", `locations: {x: {name: X, encounters: {troops: [{troop: nope}]}}}`, `unknown troop "nope"`},
		{"bad scope", `skills: {s: {name: S, scope: around}}`, `unknown scope "around"`},
		{"effect with two kinds", `skills: {s: {name: S, effects: [{damage: "1", heal: "1"}]}}`, `exactly one of`},
		{"rule with a bad target", `ai: {a: [{use: attack, target: nobody}]}`, `unknown target "nobody"`},
		{"self in a field condition", `events: {e: {when: "self.hp > 0", do: []}}`, `unknown name "self"`},
		{"missing picture", `enemies: {e: {name: E, image: nope.png, stats: {mhp: 1, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}`, `no picture assets/nope.png`},
		{"missing event picture", `events: {e: {do: [{image: cg/nope.jpg}]}}`, `no picture assets/cg/nope.jpg`},
		{"bad picture fit", `events: {e: {do: [{image: {file: x.png, fit: zoom}}]}}`, `fit is contain or cover`},
		{"text interpolation", `events: {e: {do: [{say: "you have {gld} gold"}]}}`, `unknown name "gld"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"game.yaml":      {Data: []byte(testgame.Game)},
				"data/main.yaml": {Data: []byte(testgame.Data)},
				"data/z.yaml":    {Data: []byte(tt.yaml)},
				"assets/x.png":   {Data: []byte("x")},
			}
			_, err := data.Load(fsys)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want an error with %q, got %v", tt.want, err)
			}
		})
	}
}

func TestRender(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 1)
	got := data.Render("{party.members[0].name} has {gold}G {{ok}} {nope}", st.Env())
	if want := "Scout has 100G {ok}} {nope}"; got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

func TestShorthands(t *testing.T) {
	db := testgame.Load(t, map[string]string{"assets/m/calm.ogg": "x", "data/z.yaml": `
music:
  songs:
    field: {dir: m, bpm: 100, moods: {calm: calm}}
events:
  e:
    do:
      - say: plain
      - say: {who: scout, face: happy, text: hi}
      - music: field/calm
      - image: x.png
      - image: {file: x.png, fit: cover, tint: "#ff8080", fade: 2, faces: true}
      - image: ""
`, "assets/x.png": "x"})
	cmds := db.Events["e"].Do
	if cmds[0].Say.Text != "plain" || cmds[1].Say.Who != "scout" {
		t.Errorf("say shorthand: %+v %+v", cmds[0].Say, cmds[1].Say)
	}
	if m := cmds[2].Music; m.Song != "field" || m.Mood != "calm" {
		t.Errorf("music shorthand: %+v", m)
	}
	if c := cmds[3].Image; c.File != "x.png" || c.FadeTicks() != 15 {
		t.Errorf("image shorthand: %+v", c)
	}
	if c := cmds[4].Image; c.Fit != "cover" || !c.Tint.Valid || c.FadeTicks() != 120 || !c.Faces {
		t.Errorf("image map: %+v", c)
	}
	if c := cmds[5].Image; c == nil || c.File != "" {
		t.Errorf("image \"\" hides: %+v", c)
	}
}

func TestPageBase(t *testing.T) {
	for in, want := range map[string]string{
		"cg/x.jpg": "cg/x.jpg", "cg/x.002.jpg": "cg/x.jpg", "cg/a.b.jpg": "cg/a.b.jpg", "bg/inn.jpg": "bg/inn.jpg", "": "",
	} {
		if got := data.PageBase(in); got != want {
			t.Errorf("PageBase(%q) = %q, want %q", in, got, want)
		}
	}
}
