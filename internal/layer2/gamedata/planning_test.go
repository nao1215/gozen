package gamedata

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestPlanningDiagnostics(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n"
	source := regexp.MustCompile(`game\.yaml:\d+:\d+:`)
	for _, tc := range []struct{ body, want string }{
		{"  activity_attributes: {progress: {type: number, initial: 0}}\n", "reserved attribute"},
		{"  activity_attributes: {observed: {type: number, initial: 0}}\n", "reserved attribute"},
		{"  activity_attributes: {moisture: {type: number, initial: false}}\n", "expected a finite number"},
		{"  activities: {a: {effort: 2}}\n  activity_attributes: {moisture: {type: number, initial: 0}}\n  rules: {water: {do: [{op: set, activity: a, tag: crop, attribute: moisture, value: 1}]}}\n", "cannot be combined"},
		{"  activities: {a: {effort: 2}}\n  rules: {water: {do: [{op: set, activity: a, attribute: missing, value: 1}]}}\n", "unknown attribute"},
		{"  entities: {potter: {data: {words: [null]}}}\n", "homogeneous finite scalar lists"},
		{"  entities: {potter: {data: {words: [[hello]]}}}\n", "homogeneous finite scalar lists"},
		{"  entities: {potter: {data: {words: null}}}\n", "finite scalars or scalar lists"},
		{"  entities: {potter: {data: {words: [hello, 3]}}}\n", "homogeneous finite scalar lists"},
		{"  entities: {potter: {data: {words: {nested: true}}}}\n", "finite scalars or scalar lists"},
		{"  entities: {potter: {data: {limit: .inf}}}\n", "finite scalars or scalar lists"},
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
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\n  activities: {clay: {effort: 2}}\n"
	for _, tc := range []struct{ command, want string }{
		{"op: observe", ""},
		{"op: observe, entity: potter, tag: resident, where: 'false'", ""},
		{"op: observe, activity: clay", ""},
		{"op: observe, activity: clay, where: 'sum(activities, it.observed) == 0'", ""},
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
	for _, view := range []string{`fields: [{key: name, maximum_key: reserve}]`, `fields: [{key: name, warning_key: reserve}]`, `fields: [{key: name, text_key: reserve}]`, `fields: [{key: name, critical_ratio: 2}]`, `burnup: {planned: rand()}`, "week_start: -1", "week_start: 7", "highlight: reserve", "highlight: unknown", "visible: reserve", "visible: unknown", "fields: [{key: reserve, label: Reserve}]", "portrait: reserve", "detail: reserve", "weekdays: [Sun]", "order: [missing]", "fields: [{key: name, maximum: 10, marks: [11]}]"} {
		src := "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  attributes: {reserve: {type: number, initial: 5, hidden: true}}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Board\n    kind: entities\n    view: {" + view + "}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if err == nil {
			t.Fatalf("accepted invalid view: %s", view)
		}
	}
}

func TestPlanningEnsembleHighlightObservation(t *testing.T) {
	const src = `schema_version: 10
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
    view: {highlight: attention, guests: [visitor]}
`
	db, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("actors: {visitor: {name: Visitor}}\nlocations: {room: {name: Workshop}}\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(db.Game.Planning.Entities) != 1 || db.Game.Screens["gathering"].View.Guests[0] != "visitor" {
		t.Fatal("guest became a simulation entity")
	}
	if got := db.Game.Screens["gathering"].View.Highlight; got != "attention" {
		t.Fatalf("highlight binding = %q", got)
	}
}

func TestPlanningTabs(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n"
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
		src := "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens: {gathering: {title: Gathering, kind: " + kind + "}}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nevents: {gather: {do: [{say: {text: Welcome, ensemble: gathering}}]}}\n")}})
		if (err == nil) != (kind == "ensemble") {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

func TestPlanningSurfaceEntryDiagnostics(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Board\n    kind: islands\n    view: {fullscreen: true}\n    entries:\n"
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
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {clay: {name: Clay, effort: 2}}\nscreens:\n  detail: {title: Detail, entries: [{label: Inspect}]}\n  board:\n    title: Board\n    kind: timeline\n    view: "
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
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  attributes: {icon: {type: string, initial: ''}}\n  entities: {potter: {name: Potter}}\nscreens:\n  board:\n    title: Choose\n    view: {portrait: icon}\n    entries:\n"
	for _, target := range []string{"potter", "missing"} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + "      - {label: Potter, target: " + target + "}\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != (target == "potter") {
			t.Fatalf("target %s: %v", target, err)
		}
	}
}

func TestPlanningNumericTypes(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  ledgers: {hours: {retain: 2}}\n"
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

func TestPlanningRuleArgumentNames(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {clay: {effort: 2}}\n"
	for _, tc := range []struct{ name, params, field, formula, want string }{
		{"declared", "[amount]", "value", "arg.amount", ""},
		{"literal index", "[amount]", "value", "arg[\"amount\"]", ""},
		{"dynamic index", "[amount]", "value", "arg[var.key]", ""},
		{"typo", "[amount]", "value", "arg.amonut", "has no field \"amonut\""},
		{"index typo", "[amount]", "value", "arg[\"amonut\"]", "has no field \"amonut\""},
		{"predicate typo", "[amount]", "where", "arg.amonut > 0", "has no field \"amonut\""},
		{"no parameters", "[]", "value", "arg.amount", "has no field \"amount\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := header + "  rules:\n    work:\n      params: " + tc.params + "\n      do:\n      - {op: progress, activity: clay, value: 1, " + tc.field + ": '" + tc.formula + "'}\n"
			if tc.field == "where" {
				body = strings.Replace(body, "op: progress, activity: clay, value: 1", "op: observe, activity: clay", 1)
			}
			if tc.field == "value" {
				body = strings.Replace(body, "value: 1, ", "", 1)
			}
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(body)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "game.yaml:11:") {
				t.Fatalf("missing located argument diagnostic %q: %v", tc.want, err)
			}
		})
	}
}

func TestPlanningArgumentScope(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {clay: {effort: 2}}\n  rules:\n    work: {params: [amount], do: [{op: progress, activity: clay, value: 'arg.amount'}]}\n    inspect: {do: [{op: observe, activity: clay}]}\n"
	for _, tc := range []struct{ command, want string }{
		{"op: rule, rule: work, args: {amount: '2'}", ""},
		{"op: rule, rule: work, args: {amount: 'arg.amount'}", "unknown name \"arg\""},
		{"op: progress, activity: clay, value: 'arg.amount'", "unknown name \"arg\""},
		{"op: rule, rule: work, args: {amonut: '2'}", "missing parameter \"amount\""},
		{"op: rule, rule: inspect", ""},
	} {
		t.Run(tc.command, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nevents: {work: {do: [{plan: {" + tc.command + "}}]}}\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "data/world.yaml:2:") {
				t.Fatalf("missing scoped diagnostic %q: %v", tc.want, err)
			}
		})
	}
}

func TestPlanningExpressionTargetDiagnostics(t *testing.T) {
	const header = `schema_version: 10
id: targets
start: {location: room}
planning:
  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [day]}
  entities: {potter: {name: Potter}}
  activities: {prepare: {effort: 2}}
`
	for _, tc := range []struct{ target, want string }{
		{`potter`, ""},
		{`{expr: arg.resident}`, ""},
		{`{expr: '"potter"'}`, ""},
		{`{expr: arg.task}`, "expected entity, got activity"},
		{`{expr: arg.missing}`, "no field"},
		{`{expr: self.id}`, "unknown name"},
		{`{expr: true}`, "expected entity"},
		{`{expr: '"missing"'}`, "invalid entity"},
		{`{expr: '""'}`, "invalid entity"},
		{`{expr: 'rand(2)'}`, "pure"},
		{`{}`, "requires expr"},
		{`{expr: arg.resident, typo: 1}`, "unknown field"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			story := "locations: {room: {name: Workshop}}\nevents:\n  assign:\n    params: {task: activity, resident: entity}\n    do:\n    - plan: {op: assign, activity: {expr: arg.task}, entity: " + tc.target + "}\n"
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/story.yaml": {Data: []byte(story)}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "data/story.yaml:") {
				t.Fatalf("want %q and source: %v", tc.want, err)
			}
		})
	}
	for _, command := range []string{
		`op: advance, entity: {expr: '"potter"'}`,
		`op: observe, activity: {expr: '"prepare"'}, entity: potter`,
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/story.yaml": {Data: []byte("locations: {room: {name: Workshop}}\nevents: {bad: {do: [{plan: {" + command + "}}]}}")}})
		if err == nil {
			t.Fatalf("accepted conflicting or unused target: %s", command)
		}
	}
	_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(strings.Replace(header, "version: 10", "version: 9", 1))}, "data/story.yaml": {Data: []byte(`locations: {room: {name: Workshop}}
events: {bad: {do: [{plan: {op: assign, activity: {expr: '"prepare"'}, entity: potter}}]}}
`)}})
	if err == nil || !strings.Contains(err.Error(), "10") {
		t.Fatalf("accepted old schema: %v", err)
	}
}

func TestCalendarDestinationArguments(t *testing.T) {
	const header = `schema_version: 10
id: workshop
start: {location: room}
planning:
  calendar: {start: '2030-08-01', end: '2030-08-03', phases: [morning]}
screens:
  detail:
    params: {day: date}
    title: 'Day {arg.day}'
    entries: [{label: Read}]
  calendar:
    title: Calendar
    kind: calendar
    view:
      markers:
`
	for _, tc := range []struct {
		markers string
		valid   bool
	}{
		{`      - {date: '2030-08-02', screen: detail, args: {day: '"2030-08-02"'}}`, true},
		{`      - {date: '2030-08-02', screen: detail}`, false},
		{`      - {date: '2030-08-02', screen: detail, args: {day: '2'}}`, false},
		{`      - {date: '2030-08-02', screen: detail, args: {day: '"bad"'}}`, false},
		{`      - {date: '2030-08-02', screen: detail, args: {day: 'rand(2)'}}`, false},
		{`      - {date: '2030-08-02', label: Fair, args: {day: '"2030-08-02"'}}`, false},
		{`      - {date: '2030-08-02', screen: detail, args: {day: '"2030-08-02"'}}
      - {date: '2030-08-02', screen: detail, args: {day: '"2030-08-03"'}}`, false},
		{`      - {date: '2030-08-02', screen: detail, args: {day: '"2030-08-02"'}}
      - {date: '2030-08-02', screen: detail, args: {day: '"2030-08-02"'}}`, true},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.markers + "\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("markers %s: %v", tc.markers, err)
		}
	}
}

func TestTimelineDestinationArguments(t *testing.T) {
	const header = `schema_version: 10
id: workshop
start: {location: room}
planning:
  calendar: {start: '2030-08-01', end: '2030-08-03', phases: [morning]}
  entities: {maker: {name: Maker}}
  attributes: {load: {type: number, initial: 1}}
  activities: {kiln: {name: Kiln, effort: 2}}
screens:
  detail:
    params: {task: activity, field: string}
    title: 'Choose for {arg.task}'
    view: {fields: [{key: '{arg.field}', label: 'Work {arg.task}'}]}
    entries: [{label: Read}]
  tasks:
    title: Tasks
    kind: timeline
    view:
      rows:
      - label: Pottery
        items:
`
	for _, tc := range []struct {
		item  string
		valid bool
	}{
		{`{activity: kiln, screen: detail, args: {task: '"kiln"', field: '"load"'}}`, true},
		{`{activity: kiln, screen: detail}`, false},
		{`{activity: kiln, screen: detail, args: {task: '1', field: '"load"'}}`, false},
		{`{activity: kiln, screen: detail, args: {task: '"missing"', field: '"load"'}}`, false},
		{`{activity: kiln, screen: detail, args: {task: 'rand(2)', field: '"load"'}}`, false},
		{`{activity: kiln, args: {task: '"kiln"'}}`, false},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + "        - " + tc.item + "\n")}, "data/story.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.item, err)
		}
	}
	for _, invalid := range []string{
		strings.Replace(header, "schema_version: 10", "schema_version: 9", 1),
		strings.Replace(header, "{arg.field}", "{arg.missing}", 1),
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(invalid + `        - {activity: kiln, screen: detail, args: {task: '"kiln"', field: '"load"'}}` + "\n")}, "data/story.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if err == nil || !strings.Contains(err.Error(), "fields[0].key") {
			t.Fatalf("missing field-key diagnostic: %v", err)
		}
	}
}

func TestCollectionFieldDiagnostics(t *testing.T) {
	for _, source := range []string{`sum(activities, it.efforrt)`, `sum(filter(activities, it.cancelled), it.efforrt)`} {
		game := "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [day]}\n"
		story := "locations: {room: {name: Room}}\nevents:\n  total:\n    do:\n    - set: {var.total: '" + source + "'}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(game)}, "data/world.yaml": {Data: []byte(story)}})
		if err == nil || !strings.Contains(err.Error(), "efforrt") {
			t.Fatalf("missing collection field diagnostic: %v", err)
		}
	}
}

func TestSelectedDateDestinationValidation(t *testing.T) {
	const header = `schema_version: 10
id: workshop
start: {location: room}
planning:
  calendar: {start: '2030-08-01', end: '2030-08-03', phases: [morning]}
screens:
  detail: {params: {day: date}, title: 'Day {arg.day}', entries: [{label: Read}]}
  calendar:
    title: Calendar
    kind: calendar
    view:
      date_link: `
	for _, tc := range []struct {
		link  string
		valid bool
	}{
		{`{screen: detail, args: {day: selected.date}}`, true},
		{`{screen: missing, args: {day: selected.date}}`, false},
		{`{screen: detail, args: {day: selected.offset}}`, false},
		{`{screen: detail, args: {day: selected.missing}}`, false},
		{`{screen: detail, args: {day: 'rand(2)'}}`, false},
		{`{screen: detail}`, false},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.link + "\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.link, err)
		}
	}
}

func TestMaterialDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		material string
		valid    bool
	}{
		{"{kind: glass, blur: 14, opacity: 0.78, border: 0.4, shadow: 0.6}", true},
		{"{kind: opaque, blur: 0, opacity: 0, border: 0, shadow: 0}", true},
		{"{kind: glass, opacity: .nan}", false},
		{"{kind: glass, blur: .inf}", false},
		{"{kind: plastic}", false},
		{"{kind: glass, surfaces: {chart: mica, card: glass}}", true},
		{"{kind: glass, surfaces: {chart: plastic}}", false},
		{"{kind: glass, surfaces: {unknown: mica}}", false},
		{"{kind: glass, blur: -1}", false},
		{"{kind: glass, blur: 65}", false},
		{"{kind: glass, opacity: 1.1}", false},
		{"{kind: glass, border: -0.1}", false},
		{"{kind: glass, shadow: 2}", false},
	} {
		source := "schema_version: 10\nid: studio\nstart: {location: room}\nui:\n  material: " + tc.material + "\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(source)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Studio}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.material, err)
		}
	}
}

func TestDesktopCallSettings(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n  call:\n    title: Workshop call\n    kind: ensemble\n    view:\n"
	for _, tc := range []struct{ body, want string }{
		{"      call: {address: call.example.test/workshop, mouth: {base.png: mouth.png}, blink: {base.png: blink.png}}\n      chat: {title: Messages}\n", ""},
		{"      call: {}\n", "requires chat"},
		{"      call: {controls: [a,b,c,d,e,f,g]}\n      chat: {}\n", "at most six"},
		{"      call: {layout: full, captions: overlay, others: side, others_title: People, window_opacity: 0.8}\n      chat: {}\n", ""},
		{"      call: {others: side, captions: window}\n      chat: {}\n", ""},
		{"      call: {layout: full}\n      chat: {}\n", "go together"},
		{"      call: {captions: overlay}\n      chat: {}\n", "go together"},
		{"      call: {layout: wide, captions: overlay}\n      chat: {}\n", "want full"},
		{"      call: {others: grid}\n      chat: {}\n", "want side"},
		{"      call: {captions: below}\n      chat: {}\n", "want window or overlay"},
		{"      call: {layout: full, captions: overlay, window_opacity: 1.5}\n      chat: {}\n", "want 0 to 1"},
		{"      call: {window_opacity: 0.5}\n      chat: {}\n", "needs captions: overlay"},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.body)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Fatalf("%s: %v", tc.body, err)
		}
	}
}

// TestCallSelfBackdrop: call.self_backdrop puts a picture behind a self view that is a
// picture or a guest actor; it needs self and the file, and an entity's self view takes
// the entity's own backdrop.
func TestCallSelfBackdrop(t *testing.T) {
	const header = "schema_version: 11\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  entities: {potter: {name: Potter}}\nscreens:\n  call:\n    title: Workshop call\n    kind: ensemble\n    view:\n      chat: {}\n"
	for _, tc := range []struct{ call, want string }{
		{"{self: me.png, self_stage: true, self_backdrop: room.png}", ""},
		{"{self_backdrop: room.png}", "needs self"},
		{"{self: me.png, self_backdrop: nowhere.png}", "no file assets/nowhere.png"},
		{"{self: potter, self_backdrop: room.png}", "is an entity"},
	} {
		_, err := Load(fstest.MapFS{
			"game.yaml":       {Data: []byte(header + "      call: " + tc.call + "\n")},
			"data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")},
			"assets/me.png":   {Data: []byte("x")},
			"assets/room.png": {Data: []byte("x")},
		})
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Fatalf("%s: %v", tc.call, err)
		}
	}
}

// TestCallEnsembleNamesACall: call.ensemble takes an ensemble screen only.
func TestCallEnsembleNamesACall(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n  call: {title: Call, kind: ensemble, view: {chat: {}, call: {}}}\n  board: {title: Board, kind: entities}\n"
	for _, tc := range []struct{ target, want string }{{"call", ""}, {"board", "expected an ensemble screen"}, {"nowhere", "expected an ensemble screen"}} {
		world := "locations: {room: {name: Workshop}}\nevents:\n  shared: {do: [{say: Hello.}]}\n  meet: {do: [{call: {event: shared, ensemble: " + tc.target + "}}]}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/world.yaml": {Data: []byte(world)}})
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Fatalf("%s: %v", tc.target, err)
		}
	}
}

func TestDesktopToolWindowSettings(t *testing.T) {
	for _, tc := range []struct {
		kind, view string
		valid      bool
	}{
		{"islands", "{fullscreen: true, window: Workshop}", true},
		{"islands", "{window: Workshop}", false},
		{"ensemble", "{fullscreen: true, window: Workshop}", false},
	} {
		source := "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n  board:\n    title: Workshop\n    kind: " + tc.kind + "\n    view: " + tc.view + "\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(source)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Studio}}\n")}})
		if (err == nil) != tc.valid {
			t.Fatalf("%s %s: %v", tc.kind, tc.view, err)
		}
	}
}

// Workspace progress strips reuse public planning values and reject malformed bounds.
func TestWorkspaceProgressBounds(t *testing.T) {
	const header = "schema_version: 10\nid: studio\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n  board:\n    title: Studio\n    kind: workspace\n    view:\n      fullscreen: true\n      burnup: {compact: true, bounds: "
	for _, tc := range []struct {
		bounds string
		valid  bool
	}{
		{"[0, 0, 120, 48]", true},
		{"[10, 20, 500, 70]", true},
		{"[0, 0, 119, 48]", false},
		{"[0, 0, 120, 47]", false},
		{"[-1, 0, 120, 48]", false},
		{"[0, 0, 120]", false},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.bounds + "}\n    entries: [{label: Potter, bounds: [0, 0, 250, 350], portrait: true}]\n")}, "data/main.yaml": {Data: []byte("locations: {room: {name: Studio}}\n")}})
		if (err == nil) != tc.valid {
			t.Errorf("%s: %v", tc.bounds, err)
		}
	}
}

// Bounded entry pictures are schema 10 vocabulary and need room for a caption.
func TestBoundedEntryPictureVersionAndHeight(t *testing.T) {
	for _, tc := range []struct {
		version, height string
		valid           bool
	}{{"10", "300", true}, {"9", "300", false}, {"10", "100", false}} {
		src := "schema_version: " + tc.version + "\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\nscreens:\n  board:\n    title: Board\n    kind: workspace\n    view: {fullscreen: true}\n    entries:\n      - {label: Visitor, picture: visitor.png, bounds: [1, 2, 200, " + tc.height + "]}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}, "assets/visitor.png": {Data: []byte("x")}})
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestPlanningCalendarBoundaries(t *testing.T) {
	const calendar = "start: '2032-02-28', end: '2032-03-01', phases: [morning]"
	for _, tc := range []struct{ name, calendar, want string }{
		{"leap day", calendar + ", closed: ['2032-02-29'], weekdays_off: [0, 6]", ""},
		{"same day", "start: '2032-02-29', end: '2032-02-29', phases: [morning]", ""},
		{"invalid leap day", "start: '2031-02-29', end: '2031-03-01', phases: [morning]", "expected an ISO date"},
		{"backwards", "start: '2032-03-01', end: '2032-02-28', phases: [morning]", "end precedes start"},
		{"missing phase", "start: '2032-02-28', end: '2032-03-01', phases: []", "at least one phase"},
		{"duplicate phase", "start: '2032-02-28', end: '2032-03-01', phases: [morning, morning]", "duplicate phase"},
		{"invalid closure", calendar + ", closed: ['2031-02-29']", "expected an ISO date"},
		{"weekday below", calendar + ", weekdays_off: [-1]", "weekday must be"},
		{"weekday above", calendar + ", weekdays_off: [7]", "weekday must be"},
		{"unknown look", calendar + ", looks: {noon: {}}", "a look names a phase"},
		{"closed look", calendar + ", looks: {closed: {color: '#ccddee', wash: 0.1}}", ""},
		{"wash without color", calendar + ", looks: {morning: {wash: 0.1}}", "a wash needs a color"},
		{"nan wash", calendar + ", looks: {morning: {wash: .nan}}", "wash must be"},
		{"negative wash", calendar + ", looks: {morning: {wash: -0.1}}", "wash must be"},
		{"excess wash", calendar + ", looks: {morning: {color: '#ccddee', wash: 1}}", "wash must be"},
		{"long clock", calendar + ", looks: {morning: {time: '1234567890123'}}", "at most 12 characters"},
		{"clock boundary", calendar + ", looks: {morning: {time: '123456789012'}}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {" + tc.calendar + "}\n"
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPlanningActivityGraphBoundaries(t *testing.T) {
	const header = "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-08-01', end: '2030-08-31', phases: [morning]}\n  entities: {potter: {name: Potter}}\n  activities: "
	for _, tc := range []struct{ name, activities, want string }{
		{"zero effort", "{a: {effort: 0}}", "finite and positive"},
		{"negative effort", "{a: {effort: -1}}", "finite and positive"},
		{"nonfinite effort", "{a: {effort: .inf}}", "finite and positive"},
		{"unknown predecessor", "{a: {effort: 1, after: [missing]}}", "unknown activity"},
		{"self dependency", "{a: {effort: 1, after: [a]}}", "cyclic dependency"},
		{"shared predecessor", "{a: {effort: 1}, b: {effort: 2, after: [a]}, c: {effort: 3, after: [a, b]}}", ""},
		{"ratio below", "{a: {effort: 1, dependency_ratio: -0.01}}", "ratio must be"},
		{"ratio above", "{a: {effort: 1, dependency_ratio: 1.01}}", "ratio must be"},
		{"ratio zero", "{a: {effort: 1, dependency_ratio: 0}}", ""},
		{"ratio one", "{a: {effort: 1, dependency_ratio: 1}}", ""},
		{"invalid date", "{a: {effort: 1, from: '2030-02-29'}}", "expected an ISO date"},
		{"backwards dates", "{a: {effort: 1, from: '2030-08-02', to: '2030-08-01'}}", "end precedes start"},
		{"missing parent", "{a: {effort: 1, parent: missing}}", "root activity"},
		{"self parent", "{a: {effort: 1, parent: a}}", "root activity"},
		{"one child", "{a: {effort: 2}, b: {effort: 2, parent: a}}", "at least two"},
		{"lost effort", "{a: {effort: 3}, b: {effort: 1, parent: a}, c: {effort: 1, parent: a}}", "conserve parent effort"},
		{"assigned child", "{a: {effort: 2}, b: {effort: 1, parent: a, assignee: potter}, c: {effort: 1, parent: a}}", "must be unassigned"},
		{"valid split", "{a: {effort: 3, assignee: potter}, b: {effort: 1, parent: a}, c: {effort: 2, parent: a}}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header + tc.activities + "\n")}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestPlanningCallOpacityBoundaries(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{"0", true}, {"1", true}, {"0.5", true}, {"-0.1", false}, {"1.01", false}, {".nan", false}, {".inf", false}, {"-.inf", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			src := "schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-08-01', end: '2030-08-31', phases: [morning]}\nscreens:\n  call: {title: Call, kind: ensemble, view: {chat: {}, call: {layout: full, captions: overlay, window_opacity: " + tc.value + "}}}\n"
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "window_opacity") {
				t.Fatalf("invalid opacity accepted or misdiagnosed: %v", err)
			}
		})
	}
}

// Large authored schedules should not require comparing every task with every
// other task merely to validate split effort.
func BenchmarkPlanningActivities(b *testing.B) {
	for _, count := range []int{100, 1000} {
		var src strings.Builder
		src.WriteString("schema_version: 10\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-08-01', end: '2030-08-31', phases: [morning]}\n  activities:\n")
		for i := range count {
			fmt.Fprintf(&src, "    task%d: {effort: 3}\n    left%d: {effort: 1, parent: task%d}\n    right%d: {effort: 2, parent: task%d}\n", i, i, i, i, i)
		}
		fsys := fstest.MapFS{"game.yaml": {Data: []byte(src.String())}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}}
		b.Run(fmt.Sprintf("load/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Load(fsys); err != nil {
					b.Fatal(err)
				}
			}
		})
		db, err := Load(fsys)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("validate/%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				c := checker{db: db}
				c.planning()
				if len(c.errs) != 0 {
					b.Fatal(c.errs)
				}
			}
		})
	}
}
