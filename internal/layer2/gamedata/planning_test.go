package gamedata

import (
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPlanningDiagnostics(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n"
	source := regexp.MustCompile(`game\.yaml:\d+:\d+:`)
	for _, tc := range []struct{ body, want string }{
		{"  attributes: {vigor: {type: number, initial: true}}\n", "expected a finite number"},
		{"  attributes: {vigor: {type: number, initial: 7, max: 5}}\n", "outside declared range"},
		{"  attributes: {vigor: {type: number, initial: 7}}\n  entities: {potter: {attributes: {unknown: 4}}}\n", "unknown attribute"},
		{"  activities: {a: {effort: 2, after: [b]}, b: {effort: 3, after: [a]}}\n", "cyclic dependency"},
		{"  activities: {a: {effort: 2, assignee: missing}}\n", "unknown entity"},
		{"  schedule: [{id: bell, phase: noon, event: missing}]\n", "unknown phase"},
		{"  rules: {cycle: {do: [{op: rule, rule: cycle}]}}\n", "nested rule"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.body)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
			if err == nil || !strings.Contains(err.Error(), tc.want) || !source.MatchString(err.Error()) {
				t.Fatalf("missing diagnostic and source line: %v", err)
			}
		})
	}
}

func TestPlanningViewRejectsHiddenColumnsAndBindings(t *testing.T) {
	for _, view := range []string{"fields: [{key: reserve, label: Reserve}]", "portrait: reserve", "weekdays: [Sun]", "order: [missing]", "fields: [{key: name, maximum: 10, marks: [11]}]"} {
		src := "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  attributes: {reserve: {type: number, initial: 5, hidden: true}}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Board\n    kind: entities\n    view: {" + view + "}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if err == nil {
			t.Fatalf("accepted invalid view: %s", view)
		}
	}
}

func TestSayRequiresEnsembleScreen(t *testing.T) {
	for _, kind := range []string{"ensemble", "entities"} {
		src := "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens: {gathering: {title: Gathering, kind: " + kind + "}}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nevents: {gather: {do: [{say: {text: Welcome, ensemble: gathering}}]}}\n")}})
		if (err == nil) != (kind == "ensemble") {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

func TestPlanningSurfaceEntryDiagnostics(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Board\n    kind: islands\n    view: {fullscreen: true}\n    entries:\n"
	for _, entry := range []string{
		"{label: Work, bounds: [1, 2, 3]}",
		"{label: Work, bounds: [1, 2, 0, 4]}",
		"{label: Work, target: missing}",
		"{label: Work, target: potter, bounds: [1, 2, 3, 4]}",
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + "      - " + entry + "\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if err == nil {
			t.Fatalf("invalid surface entry accepted: %s", entry)
		}
	}
}
