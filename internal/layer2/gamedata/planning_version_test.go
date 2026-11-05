package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestPlanningVocabularyNeedsSchema10 keeps the vocabulary added after v0.15
// behind schema 10: a game that says 9 must be loadable by v0.15.
func TestPlanningVocabularyNeedsSchema10(t *testing.T) {
	const calendar = "planning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\n"
	world := "locations: {room: {name: Workshop}}\n"
	for _, tc := range []struct {
		name, game, world, want string
	}{
		{"restore_checkpoint", "", world + "events: {resume: {do: [{restore_checkpoint: true}]}}\n", "restore_checkpoint needs schema_version: 10"},
		{"checkpoint", "", world + "events: {resume: {do: [{checkpoint: resume}, {restore_checkpoint: true}]}}\n", "checkpoint needs schema_version: 10"},
		{"planning", calendar, world, "planning needs schema_version: 10"},
		{"material", "ui:\n  material: {kind: opaque}\n", world, "ui.material needs schema_version: 10"},
		{"chrome", "ui:\n  chrome: {bar: '#202020', buttons: ''}\n", world, "ui.chrome needs schema_version: 10"},
		{"tones", "ui:\n  tones: {note: '#202020'}\n", world, "ui.tones needs schema_version: 10"},
		{"light", "ui:\n  light: {paper: '#202020'}\n", world, "ui.light needs schema_version: 10"},
		{"kind", calendar + "screens: {board: {title: Board, kind: entities}}\n", world, "planning views needs schema_version: 10"},
		{"view", calendar + "screens: {board: {title: Board, kind: islands, view: {fullscreen: true}, entries: [{label: Kiln}]}}\n", world, "screen views needs schema_version: 10"},
		{"entry", "screens: {menu: {title: Menu, entries: [{label: Kiln, tone: primary}]}}\n", world, "workspace entries needs schema_version: 10"},
		{"instant", "", world + "events: {talk: {do: [{say: {text: Hello, instant: true}}]}}\n", "instant lines needs schema_version: 10"},
		{"ensemble", calendar + "screens: {meet: {title: Meet, kind: ensemble}}\n", world + "events: {talk: {do: [{say: {text: Hello, ensemble: meet}}]}}\n", "ensemble lines needs schema_version: 10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			load := func(version string) error {
				src := "schema_version: " + version + "\nid: workshop\nstart: {location: room}\n" + tc.game
				_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte(tc.world)}})
				return err
			}
			if err := load("9"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("schema 9 accepted %s: %v", tc.name, err)
			}
			if err := load("10"); err != nil {
				t.Fatalf("schema 10 refused %s: %v", tc.name, err)
			}
		})
	}
}

// TestCallControlsAndSelf reads call controls as bare labels and as maps,
// rejects an unknown icon, and checks the self view's reference.
func TestCallControlsAndSelf(t *testing.T) {
	const head = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\n"
	call := func(body string) error {
		src := head + "screens:\n  meet:\n    title: Meet\n    kind: ensemble\n    view:\n      chat: {title: Messages}\n      call: " + body + "\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nactors: {visitor: {name: Visitor}}\n")}})
		return err
	}
	for _, ok := range []string{
		"{controls: [Mic, Camera, Leave]}",
		"{controls: [Mic, {label: Out, icon: leave, color: '#c03040'}], self: potter, self_label: You}",
		"{self: visitor, colors: {speaking: '#ffcc00', leave: '#aa0000'}}",
	} {
		if err := call(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for body, want := range map[string]string{
		"{controls: [{label: Mic, icon: trumpet}]}": "want one of mic",
		"{controls: [{label: Mic, sound: on}]}":     "sound",
		"{self: missing.png}":                       "missing.png",
	} {
		if err := call(body); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", body, want, err)
		}
	}
}

func TestPlanningViewRejectsUnusedKindFields(t *testing.T) {
	for _, view := range []string{"{chat: {title: Messages}}", "{filter_label: Person}", "{burnup: {title: Work}}"} {
		src := "schema_version: 10\nid: studio\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-08-01', end: '2030-08-03', phases: [morning]}\nscreens:\n  board: {title: Board, kind: entities, view: " + view + "}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Room}}\n")}})
		if err == nil || !strings.Contains(err.Error(), "require") {
			t.Errorf("unused view accepted %s: %v", view, err)
		}
	}
}
