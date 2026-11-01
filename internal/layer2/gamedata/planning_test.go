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
		{"  activities: {a: {effort: 2}}\n  rules: {move: {do: [{op: deadline, activity: missing, value: '\"2030-08-01\"'}]}}\n", "unknown activity"},
		{"  activities: {a: {effort: 2}}\n  rules: {move: {do: [{op: deadline, activity: a}]}}\n", "operation requires a value"},
		{"  activities: {a: {effort: 2}}\n  rules: {resize: {do: [{op: effort, activity: missing, value: 3}]}}\n", "unknown activity"},
		{"  activities: {a: {effort: 2}}\n  rules: {resize: {do: [{op: effort, activity: a}]}}\n", "operation requires a value"},
		{"  activities: {a: {effort: 2}}\n  rules: {exclude: {do: [{op: cancel, activity: missing, value: true}]}}\n", "unknown activity"},
		{"  activities: {a: {effort: 2}}\n  rules: {exclude: {do: [{op: cancel, activity: a}]}}\n", "operation requires a value"},
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

func TestObservationTargets(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\n  activities: {clay: {effort: 2}}\n"
	for _, tc := range []struct{ command, want string }{
		{"op: observe", ""},
		{"op: observe, entity: potter, tag: resident, where: 'false'", ""},
		{"op: observe, activity: clay", ""},
		{"op: observe, activity: clay, where: 'self.progress > 0'", ""},
		{"op: observe, activity: missing", "unknown activity"},
		{"op: observe, activity: clay, entity: potter", "cannot be combined"},
		{"op: observe, activity: clay, tag: resident", "cannot be combined"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nevents: {inspect: {do: [{plan: {" + tc.command + "}}]}}\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("missing diagnostic %q: %v", tc.want, err)
			}
		})
	}
}

func TestPlanningViewRejectsHiddenColumnsAndBindings(t *testing.T) {
	for _, view := range []string{"week_start: -1", "week_start: 7", "highlight: reserve", "highlight: unknown", "visible: reserve", "visible: unknown", "fields: [{key: reserve, label: Reserve}]", "portrait: reserve", "detail: reserve", "weekdays: [Sun]", "order: [missing]", "fields: [{key: name, maximum: 10, marks: [11]}]"} {
		src := "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  attributes: {reserve: {type: number, initial: 5, hidden: true}}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Board\n    kind: entities\n    view: {" + view + "}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if err == nil {
			t.Fatalf("accepted invalid view: %s", view)
		}
	}
}

func TestPlanningEnsembleHighlightObservation(t *testing.T) {
	const src = `schema_version: 9
id: workshop
start: {location: room}
planning:
  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}
  attributes: {reserve: {type: number, initial: 5, hidden: true}}
  entities:
    potter:
      observations: {attention: 'self.reserve < 3'}
screens:
  gathering:
    title: Gathering
    kind: ensemble
    view: {highlight: attention}
`
	db, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := db.Game.Screens["gathering"].View.Highlight; got != "attention" {
		t.Fatalf("highlight binding = %q", got)
	}
}

func TestPlanningTabs(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n"
	for _, tc := range []struct {
		views string
		valid bool
	}{
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [board, people]}}\n  people: {title: People, kind: entities, view: {fullscreen: true, tabs: [board, people]}}\n", true},
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [board, missing]}}\n", false},
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [board, board]}}\n", false},
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [board]}}\n", false},
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [board, people]}}\n  people: {title: People, kind: entities}\n", false},
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [board, people]}}\n  people: {title: People, kind: entities, view: {fullscreen: true, tabs: [people, board]}}\n", false},
		{"  board: {title: Board, kind: islands, view: {fullscreen: true, tabs: [people, dates]}}\n  people: {title: People, kind: entities, view: {fullscreen: true, tabs: [people, dates]}}\n  dates: {title: Dates, kind: calendar, view: {fullscreen: true, tabs: [people, dates]}}\n", false},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.views)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("tabs validity %v: %v", tc.valid, err)
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

func TestTimelineRowsAndMarkers(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {clay: {name: Clay, effort: 2}}\nscreens:\n  detail: {title: Detail, entries: [{label: Inspect}]}\n  board:\n    title: Board\n    kind: timeline\n    view: "
	for _, tc := range []struct {
		view  string
		valid bool
	}{
		{`{rows: [{label: Pottery, items: [{activity: clay, label: C, text: "Planned {activity.clay.effort} units", screen: detail}]}], markers: [{date: '2030-08-01', label: Fair, milestone: true}]}`, true},
		{`{rows: [{label: Pottery, text: "Checked {1+2}", items: [{activity: clay, sample: '0.4', sample_label: 'Checked today', sample_freshness: '0.5'}]}]}`, true},
		{`{rows: [{label: Pottery, items: [{activity: clay, text: "{missing(}"}]}]}`, false},
		{`{rows: [{label: Pottery, items: [{activity: clay, sample: 'rand()'}]}]}`, false},
		{`{rows: [{label: Pottery, items: [{activity: clay, sample_freshness: 'rand()'}]}]}`, false},
		{`{rows: [{label: Pottery, items: [{activity: unknown}]}]}`, false},
		{`{rows: [{label: Pottery, items: [{activity: clay}, {activity: clay}]}]}`, false},
		{`{rows: [{label: Pottery, items: [{activity: clay, screen: unknown}]}]}`, false},
		{`{rows: [{items: [{activity: clay}]}]}`, false},
		{`{markers: [{date: tomorrow, label: Fair}]}`, false},
		{`{markers: [{date: '2030-08-01', screen: detail}]}`, true},
		{`{markers: [{date: '2030-08-01'}]}`, false},
		{`{markers: [{date: '2030-08-01', label: Fair, screen: detail}]}`, true},
		{`{markers: [{date: '2030-08-01', label: Fair, screen: missing}]}`, false},
		{`{markers: [{date: '2030-08-01', label: Fair, screen: detail}, {date: '2030-08-01', label: Market, screen: board}]}`, false},
		{`{markers: [{date: '2030-08-01', label: Fair, screen: detail}, {date: '2030-08-01', label: Market, screen: detail}]}`, true},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.view + "\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.view, err)
		}
	}
}

func TestPlanningCandidateTargets(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  attributes: {icon: {type: string, initial: ''}}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Choose\n    view: {portrait: icon}\n    entries:\n"
	for _, target := range []string{"potter", "missing"} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + "      - {label: Potter, target: " + target + "}\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != (target == "potter") {
			t.Fatalf("target %s: %v", target, err)
		}
	}
}

func TestPlanningNumericTypes(t *testing.T) {
	const header = "schema_version: 9\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  ledgers: {hours: {retain: 2}}\n"
	for _, formula := range []string{"true", "false", "nil", "\"2\"", "true ? 2 : nil", "2", "int(true)"} {
		for _, op := range []string{"progress", "effort", "record", "observe"} {
			t.Run(op+"/"+formula, func(t *testing.T) {
				body := "  activities: {clay: {effort: 2}}\n  rules: {update: {do: [{op: " + op + ", activity: clay, value: '" + formula + "'}]}}\n"
				if op == "record" {
					body = strings.ReplaceAll(body, "activity: clay", "ledger: hours")
				}
				if op == "observe" {
					body = "  activities: {clay: {effort: 2, observation: '" + formula + "'}}\n"
				}
				_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + body)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
				if formula == "2" || formula == "int(true)" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "requires a number") || !regexp.MustCompile(`game\.yaml:\d+:\d+:`).MatchString(err.Error()) {
					t.Fatalf("missing numeric diagnostic with source: %v", err)
				}
			})
		}
	}
}
