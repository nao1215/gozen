package gamedata_test

import (
	"slices"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

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
	if u := uses["flag.saw_two"]; u == nil || !slices.Equal(u.Sets, []string{"troops.boss.events[0].do[1].set"}) {
		t.Errorf("a troop's event: %+v", u)
	}
}

func TestMapAdviceStateUses(t *testing.T) {
	db := testgame.LoadScenes(t, nil)
	db.Locations["town"].Map.Points["square"].Advice = []data.MapAdvice{{When: data.MustExpr("flag.ready"), Heading: "Stock {item.potion}", Text: "Count {var.stock}"}}
	uses := db.StateUses()
	for name, suffix := range map[string]string{"flag.ready": "when", "item.potion": "heading", "var.stock": "text"} {
		want := "locations.town.map.points.square.advice[0]." + suffix
		if u := uses[name]; u == nil || !slices.Contains(u.Reads, want) {
			t.Fatalf("%s missing %s", name, want)
		}
	}
}
