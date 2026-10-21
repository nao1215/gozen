package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

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
		{"verb with capitals", game("verbs: [Bad]\n"), "want a name of lowercase letters, digits and _"},
		{"verb that is built in", game("verbs: [gold]\n"), "is a built-in command"},
		{"verb twice", game("verbs: [dance, dance]\n"), `"dance" is written twice`},
		{"verbs need version 2", map[string]string{"game.yaml": "schema_version: 1\nid: v\n" + testgame.Game + "verbs: [dance]\n"}, "verbs needs schema_version: 2"},
		{"screen with a space", game("screens: ['my screen']\n"), "want a name of lowercase letters"},
		{"screen that is built in", game("screens: [formation]\n"), "is a built-in screen"},
		{"screen twice", game("screens: [bag, bag]\n"), `"bag" is written twice`},
		{"set of a text that needs no quotes", data("events: {e: {do: [{set: {location: \"o'k\"}}]}}"), "a name without quotes"},
		{"set that is a number", data("events: {e: {do: [{set: 3}]}}"), "expected a map"},
		{"set facing that is not a direction", data("events: {e: {do: [{set: {pos.facing: sideways}}]}}"), "facing is north, east, south or west"},
		{"set roamers to something", data("events: {e: {do: [{set: {roamers: all}}]}}"), "roamers can be set only to none"},
		{"set party too large", data("events: {e: {do: [{set: {party: [scout, guide, scout, guide, scout, guide, scout, guide]}}]}}"), "party takes 1 to"},
		{"set party of a stranger", data("events: {e: {do: [{set: {party: [scout, ghost]}}]}}"), `unknown actor "ghost"`},
		{"set an actor's name", data("events: {e: {do: [{set: {actor.scout.name: Bob}}]}}"), "set can change actor.<id>."},
		{"set tactics to nonsense", data("events: {e: {do: [{set: {actor.scout.tactics: nonsense}}]}}"), "tactics is auto, manual or a preset"},
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
		"game.yaml": "schema_version: 3\nid: v\n" + testgame.Game + "verbs: [dance]\nscreens: [bag]\n",
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
