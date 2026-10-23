package gamedata_test

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestDefinitionsOfTheWrongShape: a scalar where a map belongs, a list where a scalar belongs and the like are reported at their place,
// not turned into zero values.
func TestDefinitionsOfTheWrongShape(t *testing.T) {
	cases := []struct {
		name, y, want string
	}{
		{"a list of items", "items: [a, b]", "expected a map"},
		{"an item that is a list", "items: {a: [1, 2]}", "expected"},
		{"a name that is a map", "items: {a: {name: {x: 1}}}", "a scalar"},
		{"a name that is a list", "items: {a: {name: [1, 2]}}", "a scalar"},
		{"skills of a class as a map", "classes: {c3: {skills: {x: 1}}}", "expected a list"},
		{"a condition that is a list", "events: {e: {when: [1], do: []}}", "a scalar"},
		{"a condition that is a map", "events: {e: {when: {a: 1}, do: []}}", "a scalar"},
		{"do that is a map", "events: {e: {do: {say: hi}}}", "expected a list"},
		{"an event that is a scalar", "events: {e: 3}", "expected"},
		{"a number given a list", "enemies: {m: {name: M, image: x.png, stats: {mhp: [1]}}}", "a scalar"},
		{"stats that is a list", "enemies: {m: {name: M, image: x.png, stats: [1]}}", "expected a map"},
		{"a color that is a list", "locations: {a1: {name: A, dungeon: {map: '###', floor: [1, 2]}}}", "a scalar"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/z.yaml": c.y})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want an error with %q, got %v", c.want, err)
			}
		})
	}
}

func TestDefinitionFilesOfTheWrongKind(t *testing.T) {
	for name, body := range map[string]string{
		"a list at the root":      "- a\n- b\n",
		"a scalar at the root":    "just text\n",
		"a number at the root":    "42\n",
		"broken flow":             "items: {a: [\n",
		"a tab":                   "items:\n\ta: 1\n",
		"a duplicated key":        "items: {a: {name: A}, a: {name: B}}\n",
		"a key that is a list":    "? [a, b]\n: 1\n",
		"a document separator":    "items: {}\n---\nitems: {}\n",
		"an unknown tag":          "items: !wat {}\n",
		"an alias with no anchor": "items: *nothing\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"data/z.yaml": body})
			if err == nil {
				t.Log("accepted")
				return
			}
			if !strings.Contains(err.Error(), "data/z.yaml") {
				t.Errorf("the diagnostic must name the file: %v", err)
			}
		})
	}
	// an empty file and one with only comments are allowed
	for _, body := range []string{"", "# nothing\n", "---\n", "{}"} {
		if _, err := testgame.TryLoad(map[string]string{"data/z.yaml": body}); err != nil {
			t.Errorf("%q: %v", body, err)
		}
	}
}

func TestAnchorsAndTagsAreAccepted(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/z.yaml": `
items:
  base: &base {name: Base, price: 1}
  copy: *base
`})
	if db.Items["copy"].Name != "Base" || db.Items["copy"].Price != 1 || db.Items["base"].Name != "Base" {
		t.Errorf("anchors: %+v %+v", db.Items["copy"], db.Items["base"])
	}
}

// TestMapMistakes: mistakes in a map of points and roads.
func TestMapMistakes(t *testing.T) {
	for _, c := range []struct{ old, new, msg string }{
		{"picture: maps/town.png", "picture: \"\"", "a map needs a picture"},
		{"        out: {at: [2, 34], to: world, label: Leave town}", "        out: {at: [2, 34], to: town}", "leads into another location"},
		{"        out: {at: [2, 34], to: world, label: Leave town}", "        out: {at: [2, 34], to: world, at_point: ghost}", "ghost"},
		{"        out: {at: [2, 34], to: world, label: Leave town}", "        out: {at: [2, 34], to: shop, at_point: ghost}", "at_point"},
		{"        fork: {at: [32, 15]}", "        fork: {at: [32, 15], at_point: out}", "at_point is for a point with to"},
		{"        fork: {at: [32, 15]}", "        fork: {at: [32, 15], once: true}", "once is for a point with do"},
		{"        square: {at: [32, 30], to: plaza}", "        square: {at: [32, 30], to: plaza, start: true}\n        far: {at: [1, 1], start: true}", "have start"},
		{"        - {between: [square, fork]}", "        - {between: [square]}", "a road is between two points"},
		{"        - {between: [square, fork]}", "        - {between: [square, square]}", "two different points"},
		{"        - {between: [square, fork]}", "        - {between: [square, fork], steps: -1}", "want steps in [0, 10000]"},
	} {
		src, files := scenesWith(c.old, c.new)
		if src == testgame.ScenesData {
			t.Fatalf("%q was not found in the fixture", c.old)
		}
		_, err := data.Load(testgame.ScenesFS(files))
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s -> %s: want an error with %q, got %v", c.old, c.new, c.msg, err)
		}
	}
}
