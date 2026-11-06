package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestRollbackSetting: game.yaml ui.rollback is 0 to 64 earlier events, at version 4.
func TestRollbackSetting(t *testing.T) {
	for _, c := range []struct{ head, want string }{
		{"schema_version: 4\nid: b\nui: {rollback: 8}\n", ""},
		{"schema_version: 4\nid: b\nui: {rollback: 0}\n", ""},
		{"schema_version: 4\nid: b\nui: {rollback: -1}\n", "ui.rollback: want 0 to 64"},
		{"schema_version: 4\nid: b\nui: {rollback: 65}\n", "ui.rollback: want 0 to 64"},
		{"schema_version: 3\nid: b\nui: {rollback: 8}\n", "ui.rollback needs schema_version: 4"},
		{"schema_version: 12\nid: b\n", "0〜11"},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": c.head + testgame.Game})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%q: %v", c.head, err)
		}
	}
}

// TestChoiceID: a choice's id needs version 4, is a name, is one per game, and chose.<id>
// needs a choice with that id.
func TestChoiceID(t *testing.T) {
	head := "schema_version: 4\nid: b\n"
	for _, c := range []struct{ head, events, want string }{
		{head, `{a: {do: [{choice: [{label: Yes}], id: dinner}, {if: "chose.dinner == \"0\"", then: [{gold: 1}]}]}}`, ""},
		{"schema_version: 3\nid: b\n", `{a: {do: [{choice: [{label: Yes}], id: dinner}]}}`, "the id of a choice needs schema_version: 4"},
		{"schema_version: 3\nid: b\n", `{a: {when: "chose.x == \"\"", do: [{gold: 1}]}}`, "the name chose needs schema_version: 4"},
		{head, `{a: {do: [{choice: [{label: Yes}], id: Dinner}]}}`, "want a name of lowercase"},
		{head, `{a: {do: [{choice: [{label: Yes}], id: d}]}, b: {do: [{choice: [{label: No}], id: d}]}}`, "choice id \"d\" is also at"},
		{head, `{a: {when: "chose.missing == \"\"", do: [{gold: 1}]}}`, "chose.missing: no choice has the id"},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": c.head + testgame.Game, "data/z.yaml": "events: " + c.events})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v", c.events, err)
		}
	}
}

func TestMaterialSettings(t *testing.T) {
	for _, c := range []struct {
		body  string
		valid bool
	}{
		{"{kind: mica, blur: 12, opacity: 0.6, surfaces: {card: acrylic, control: glass}, wallpaper: ['#e5e9ef', '#b6d4d5', '#d9ccbb']}", true},
		{"{kind: opaque}", true},
		{"{kind: glass, refraction: 32, wallpaper_strength: 1}", true},
		{"{kind: glass, refraction: -1}", false},
		{"{kind: glass, refraction: 33}", false},
		{"{kind: glass, wallpaper_strength: 1.01}", false},
		{"{kind: glass, wallpaper_strength: -0.01}", false},
		{"{kind: glass, blur: 65}", false},
		{"{kind: glass, opacity: -0.1}", false},
		{"{kind: glass, shadow: 1.1}", false},
		{"{kind: glass, surfaces: {unknown: mica}}", false},
		{"{kind: glass, surfaces: {card: unknown}}", false},
		{"{kind: glass, wallpaper: ['#ffffff']}", false},
		{"{kind: unknown}", false},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": "schema_version: 10\nid: material\nui: {material: " + c.body + "}\n" + testgame.Game})
		if (err == nil) != c.valid {
			t.Errorf("%s: %v", c.body, err)
		}
	}
}
