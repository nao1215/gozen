package gamedata

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/base/diag"
)

func unusedGame(t *testing.T, game, data string) *DB {
	t.Helper()
	db, err := LoadDir(fstest.MapFS{
		"game.yaml":       {Data: []byte("schema_version: 11\nid: unused_test\n" + game)},
		"data/world.yaml": {Data: []byte(data)},
		"assets/mote.png": {Data: []byte("synthetic image sentinel")},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func unusedPaths(db *DB) []string {
	var paths []string
	for _, d := range Diagnose(errors.Join(db.DefinitionWarnings()...), "warning") {
		paths = append(paths, d.Path)
	}
	slices.Sort(paths)
	return paths
}

func TestUnusedDefinitionsOneWarningPerDefinition(t *testing.T) {
	db := unusedGame(t, "start: {event: intro, location: camp, party: [walker]}\nscreens: {spare: {title: Spare, entries: [{label: Wait, do: [{say: Hello}]}]}}\n", `
events:
  intro: [{say: Welcome}]
  leftover: [{say: Unused}]
locations: {camp: {}, unused_room: {}}
items: {unused_token: {name: Token}}
troops: {unused_group: {members: [mote]}}
enemies: {mote: {name: Mote, image: mote.png, stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}}}
shops: {unused_shop: {goods: []}}
ai: {unused_ai: [{use: attack}]}
skills: {attack: {}, guard: {}}
actors: {walker: {class: ranger}}
classes: {ranger: {stats: {mhp: 9, mmp: 0, atk: 1, def: 1, mat: 1, mdf: 1, agi: 1, luk: 1}, exp_for: level * 9}}
`)
	want := []string{"ai.unused_ai", "events.leftover", "screens.spare", "items.unused_token", "locations.unused_room", "shops.unused_shop", "troops.unused_group"}
	slices.Sort(want)
	if got := unusedPaths(db); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, d := range Diagnose(errors.Join(db.DefinitionWarnings()...), "warning") {
		if d.Code != diag.UnusedDefinition.String() || d.File == "" || d.Line == 0 || d.Col == 0 {
			t.Fatalf("unpositioned or uncoded warning: %+v", d)
		}
	}
	if errs := db.Check(); len(errs) > 0 {
		t.Fatal(errs)
	}
	if got := unusedPaths(db); !slices.Equal(got, want) {
		t.Fatalf("Check accumulated or lost warnings: %v", got)
	}
}

func TestUnusedDefinitionsResolveCallsEntriesAndExpressions(t *testing.T) {
	db := unusedGame(t, `start: {event: intro, location: camp}
game_over: {event: finish}
verbs: {greet: alias_target}
screens:
  menu:
    title: Menu
    back: other
    entries:
      - {label: Next, destination: destination, do: [{call: entry_target}]}
  other: {title: Other, entries: [{label: Done, do: [{say: Done}]}]}
  destination: {title: Destination, entries: [{label: Done, do: [{say: Done}]}]}
`, `
events:
  intro:
    - call: {event: repeated, each: {param: name, items: [one, two]}}
    - open: menu
    - if: "item['token'] > 0"
      then: [{say: Found}]
    - if: "location == 'looked_at'"
      then: [{say: There}]
  repeated: {params: {name: string}, do: [{call: nested}]}
  nested: [{say: Nested}]
  entry_target: [{say: Entry}]
  alias_target: [{say: Alias}]
  finish: [{say: Done}]
  triggered: {trigger: auto, when: 'false', do: [{say: Triggered}]}
locations: {camp: {}, looked_at: {}}
items: {token: {name: Token}}
`)
	if got := unusedPaths(db); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestUnusedDefinitionsDynamicItemLookupIsConservative(t *testing.T) {
	for _, expression := range []string{"item[var.which] > 0", "len(item) > 0"} {
		db := unusedGame(t, "start: {event: intro, location: camp}\n", "locations: {camp: {}}\nitems: {a: {name: A}, b: {name: B}}\nevents: {intro: {do: [{if: \""+expression+"\", then: [{say: Found}]}]}}\n")
		if got := unusedPaths(db); len(got) > 0 {
			t.Fatalf("%s: %v", expression, got)
		}
	}
}

func TestUnusedDefinitionsDoNotTreatProseAsReferences(t *testing.T) {
	db := unusedGame(t, "start: {event: intro, location: camp}\n", "locations: {camp: {}}\nevents: {intro: [{say: leftover}], leftover: [{say: Hello}]}\n")
	if got := unusedPaths(db); !slices.Equal(got, []string{"events.leftover"}) {
		t.Fatal(got)
	}
	if !strings.Contains(db.DefinitionWarnings()[0].Error(), "leftover") {
		t.Fatal(db.DefinitionWarnings())
	}
}

func TestUnusedDefinitionsFlowAndEnsembleEntryPoints(t *testing.T) {
	db := unusedGame(t, `start: {location: camp, event: intro}
planning:
  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}
  activities: {glaze: {name: Glaze, effort: 2}}
screens:
  first: {title: First, kind: workspace, view: {fullscreen: true}}
  second: {title: Second, kind: workspace, view: {fullscreen: true}}
  conversation: {title: Conversation, kind: ensemble}
  narration: {title: Narration, kind: ensemble}
  spare: {title: Spare, kind: ensemble}
flows:
  day:
    steps: [{label: First, screen: first, enter: enter_first}, {label: Second, screen: second}]
`, `locations: {camp: {}}
events:
  intro: [{call: {event: shared, ensemble: conversation}}]
  shared: [{say: {text: Welcome, ensemble: narration}}]
  enter_first: [{say: Entered}]
`)
	if got := unusedPaths(db); !slices.Equal(got, []string{"screens.spare"}) {
		t.Fatal(got)
	}
}

func TestUnusedDefinitionsLiteralWildcardDoesNotHideOtherItems(t *testing.T) {
	db := unusedGame(t, "start: {location: camp, event: intro}\n", `locations: {camp: {}}
items: {'*': {name: Star}, unused: {name: Unused}}
events: {intro: [{if: "item['*'] > 0", then: [{say: Found}]}]}
`)
	if got := unusedPaths(db); !slices.Equal(got, []string{"items.unused"}) {
		t.Fatal(got)
	}
}

func TestUnusedDefinitionsRetainGlobalPhaseFormatting(t *testing.T) {
	db := unusedGame(t, `start: {location: camp}
planning:
  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}
screens:
  labels: {title: Labels, kind: calendar, view: {phases: {morning: Morning}}}
  dates: {title: Dates, kind: calendar, view: {date_format: '{year}-{month}-{day}'}}
`, "locations: {camp: {}}\n")
	if got := unusedPaths(db); len(got) != 0 {
		t.Fatal(got)
	}
}

// TestUnusedDefinitionsBannerOpensEvent: the event a phone banner opens when
// clicked has a consumer.
func TestUnusedDefinitionsBannerOpensEvent(t *testing.T) {
	db := unusedGame(t, "start: {event: intro, location: camp}\n", "locations: {camp: {}}\nevents: {intro: [{phone: {app: banner, notices: [{title: Nora, text: Ship it?, event: reply}]}}], reply: [{say: Yes}]}\n")
	if got := unusedPaths(db); len(got) > 0 {
		t.Fatal(got)
	}
}
