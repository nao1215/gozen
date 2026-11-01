package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestEventArgumentDiagnostics(t *testing.T) {
	const header = "schema_version: 10\nid: bindings\nstart: {location: room}\n"
	const base = "locations: {room: {name: Room}}\nactors: {guide: {name: Guide}}\nevents:\n  shared:\n    params: {amount: number}\n    do: [{say: '{arg.amount}'}]\n  main:\n    do:\n    - "
	for _, tc := range []struct{ command, want string }{
		{"call: {event: shared, args: {amount: '2'}}", ""},
		{"call: shared", "missing argument"},
		{"call: {event: shared, args: {amount: true}}", "expected number"},
		{"call: {event: shared, args: {amount: 'nil'}}", "expected number"},
		{"call: {event: shared, args: {amount: '100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000 * 100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000'}}", "finite number"},
		{"call: {event: shared, args: {extra: 2}}", "unknown argument"},
		{"call: {event: shared, args: {amount: 'arg.amount'}}", "unknown name"},
		{"call: {event: shared, args: {amount: 'rand(5)'}}", "pure"},
		{"call: {event: missing}", "unknown event"},
		{"checkpoint: shared", "requires arguments"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/world.yaml": {Data: []byte(base + tc.command + "\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "data/world.yaml:") {
				t.Fatalf("want %s with source: %v", tc.want, err)
			}
		})
	}
	for _, tc := range []struct{ params, command, want string }{
		{"who: actor", "say: '{arg.whom}'", "no field"},
		{"who: invalid", "say: hello", "invalid parameter"},
		{"who: actor", "call: {event: target, args: {who: '(arg.who)'}}", "got actor"},
	} {
		story := "locations: {room: {name: Room}}\nevents:\n  target:\n    params: {who: entity}\n    do: [{say: hi}]\n  outer:\n    params: {" + tc.params + "}\n    do: [{" + tc.command + "}]\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/world.yaml": {Data: []byte(story)}})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %v", tc.want, err)
		}
	}
	for _, kind := range []string{"actor", "entity", "activity", "date", "bool", "string"} {
		for _, value := range []string{"'\"missing\"'", "3"} {
			story := strings.Replace(base, "amount: number", "amount: "+kind, 1) + "call: {event: shared, args: {amount: " + value + "}}\n"
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/world.yaml": {Data: []byte(story)}})
			if kind == "string" && value == "'\"missing\"'" {
				if err != nil {
					t.Fatal(err)
				}
				continue
			}
			if err == nil {
				t.Fatalf("accepted %s %s", kind, value)
			}
		}
	}
	// Even an argument-free mapping is a schema-10 form.
	_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(strings.Replace(header, "10", "9", 1))}, "data/world.yaml": {Data: []byte("locations: {room: {}}\nevents: {child: {do: [{say: hi}]}, main: {do: [{call: {event: child}}]}}\n")}})
	if err == nil || !strings.Contains(err.Error(), "schema_version: 10") {
		t.Fatalf("version gate: %v", err)
	}
}

func TestLiteralEntryPointsRejectRequiredEventParameters(t *testing.T) {
	const story = "locations: {room: {name: Room}}\nevents:\n  shared:\n    params: {amount: number}\n    do: [{say: '{arg.amount}'}]\n"
	const planningHeader = "planning:\n  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [day]}\n"
	for _, extra := range []string{
		"verbs: {greet: shared}\n",
		planningHeader + "  policies: {steady: [{when: true, event: shared}]}\n",
		planningHeader + "  schedule: [{id: bell, date: '2030-01-01', phase: day, event: shared}]\n",
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte("schema_version: 10\nid: bindings\nstart: {location: room}\n" + extra)}, "data/world.yaml": {Data: []byte(story)}})
		if err == nil || !strings.Contains(err.Error(), "requires arguments") {
			t.Fatalf("entry point accepted: %v", err)
		}
	}
}
