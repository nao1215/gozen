package gamedata_test

import (
	"slices"
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
		{"schema_version: 5\nid: b\n", "0〜4"},
	} {
		_, err := testgame.TryLoad(map[string]string{"game.yaml": c.head + testgame.Game})
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%q: %v", c.head, err)
		}
	}
}

// TestStateUses: the index of names finds where each flag and var is written and read,
// into the branches of if and choice, the when of events and places, and {expressions} in lines.
func TestStateUses(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/novel.yaml": `
events:
  dinner:
    when: "!flag.ate"
    do:
      - say: "Trust is {var.trust}."
      - choice:
          - label: Soup
            do: [{set: {flag.ate: true, var.trust: "var.trust + 1"}}]
          - label: Bread
            when: "item.key > 0"
            do: [{if: "flag.ghost", then: [{give: {item: potion}}]}]
      - call: sub
`})
	uses := db.StateUses()
	for _, c := range []struct {
		name       string
		sets, read []string
	}{
		{"flag.ate", []string{"events.dinner.do[1].choice[0].do[0].set"}, []string{"events.dinner.when"}},
		{"var.trust", []string{"events.dinner.do[1].choice[0].do[0].set"}, []string{"events.dinner.do[0].say", "events.dinner.do[1].choice[0].do[0].set.var.trust"}},
		{"item.key", nil, []string{"events.dinner.do[1].choice[1].when", "locations.camp.exits[1].when"}},
		{"flag.ghost", nil, []string{"events.dinner.do[1].choice[1].do[0].if", "locations.camp.people[1].when"}},
		{"item.potion", []string{"events.dinner.do[1].choice[1].do[0].then[0].give"}, nil},
		{"ran.sub", []string{"events.dinner.do[2].call"}, nil},
	} {
		u := uses[c.name]
		if u == nil {
			t.Errorf("%s: not indexed", c.name)
			continue
		}
		if !slices.Equal(u.Sets, c.sets) || !slices.Equal(u.Reads, c.read) {
			t.Errorf("%s: sets %q reads %q, want %q and %q", c.name, u.Sets, u.Reads, c.sets, c.read)
		}
	}
	if u := uses["flag.saw_two"]; u != nil {
		t.Errorf("a troop's event is outside the index for now: %+v", u)
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
