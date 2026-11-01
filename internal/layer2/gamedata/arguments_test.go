package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/expr"
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

func TestScreenArgumentDiagnostics(t *testing.T) {
	const game = "schema_version: 10\nid: screen-bindings\nstart: {location: room}\nscreens:\n  stock:\n    params: {amount: number}\n    title: 'Stock {arg.amount}'\n    entries: [{label: 'Take {arg.amount}', do: [{set: {var.total: 'var.total + arg.amount'}}]}]\n"
	const story = "locations: {room: {name: Room}}\nevents:\n  visit:\n    do:\n    - "
	for _, tc := range []struct{ command, want string }{
		{"open: {screen: stock, args: {amount: '2'}}", ""},
		{"open: stock", "missing argument"},
		{"open: {screen: stock, args: {amount: true}}", "expected number"},
		{"open: {screen: stock, args: {extra: 2}}", "unknown argument"},
		{"open: {screen: stock, args: {amount: 'rand(3)'}}", "pure"},
		{"open: {screen: stock, args: {amount: 'arg.amount'}}", "unknown name"},
		{"open: {screen: config, args: {amount: 1}}", "authored screen"},
		{"open: {screen: missing}", "not"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(game)}, "data/world.yaml": {Data: []byte(story + tc.command + "\n")}})
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "data/world.yaml:") {
				t.Fatalf("want %s with source: %v", tc.want, err)
			}
		})
	}
	for _, tc := range []struct{ old, new, want string }{
		{"schema_version: 10", "schema_version: 9", "schema_version: 10"},
		{"amount: number", "amount: invalid", "invalid parameter"},
		{"Stock {arg.amount}", "Stock {arg.missing}", "no field"},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(strings.Replace(game, tc.old, tc.new, 1))}, "data/world.yaml": {Data: []byte(story + "open: {screen: stock, args: {amount: 2}}\n")}})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("want %s: %v", tc.want, err)
		}
	}
}

func TestRepeatedCallValidationAndFrozenMembership(t *testing.T) {
	const header = "schema_version: 10\nid: repeat\nstart: {location: room}\n"
	const story = "locations: {room: {name: Room}}\nactors: {guide: {name: Guide}}\nevents:\n  visit:\n    params: {worker: actor}\n    do: [{say: hello}]\n  main:\n    do:\n    - call: {event: visit, each: "
	for _, tc := range []struct{ each, want string }{
		{"{param: worker, items: [guide]}", ""},
		{"{param: worker, items: []}", ""},
		{"{param: missing, items: [guide]}", "parameter"},
		{"{param: worker, items: [missing]}", "invalid actor"},
		{"{param: worker}", "exactly one"},
		{"{param: worker, in: entities, items: []}", "exactly one"},
		{"{param: worker, in: 'rand()'}", "pure"},
		{"{param: worker, in: true}", "list"},
	} {
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/story.yaml": {Data: []byte(story + tc.each + "}\n")}})
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Fatalf("%s: %v", tc.each, err)
		}
	}
	db, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(header)}, "data/story.yaml": {Data: []byte(story + "{param: worker, items: []}}\n")}})
	if err != nil {
		t.Fatal(err)
	}
	row := expr.Map{"id": "guide"}
	rows := []any{row}
	call := EventCall{Event: "visit", Each: &CallEach{Param: "worker", In: MustExpr("rows")}}
	commands, err := db.ExpandCall(call, expr.Map{"rows": rows})
	if err != nil || len(commands) != 1 {
		t.Fatalf("expand: %v %v", commands, err)
	}
	row["id"] = "missing"
	args, err := db.Bind(commands[0].Call, expr.Map{})
	if err != nil || args["worker"] != "guide" {
		t.Fatalf("membership not frozen: %v %v", args, err)
	}
	for _, value := range []any{true, []any{expr.Map{"id": "guide"}, expr.Map{"id": "missing"}}, []any{1.0}, make([]any, 4097)} {
		commands, err := db.ExpandCall(call, expr.Map{"rows": value})
		if err == nil || commands != nil {
			t.Fatalf("partial/invalid expansion: %v %v", commands, err)
		}
	}
	call.Each.In = MustExpr("rand()")
	draws := 0
	_, err = db.ExpandCall(call, expr.Map{"rand": expr.Func(func([]any) (any, error) { draws++; return 0.0, nil })})
	if err == nil || draws != 0 {
		t.Fatalf("impure selector evaluated: %d %v", draws, err)
	}
	call.Each = &CallEach{Param: "worker", Items: make([]string, 4096)}
	for i := range call.Each.Items {
		call.Each.Items[i] = "guide"
	}
	commands, err = db.ExpandCall(call, expr.Map{})
	if err != nil || len(commands) != 4096 {
		t.Fatalf("limit: %d %v", len(commands), err)
	}
	call.Each.Items = append(call.Each.Items, "guide")
	if _, err = db.ExpandCall(call, expr.Map{}); err == nil {
		t.Fatal("accepted oversized literal list")
	}

	db.Events["visit"].Params["worker"] = "string"
	call.Each.Items = []string{"line\nbreak", "quote\"slash\\", "tab\tvalue", "雪", "\x00"}
	commands, err = db.ExpandCall(call, expr.Map{})
	if err != nil {
		t.Fatal(err)
	}
	for i, command := range commands {
		args, err := db.Bind(command.Call, expr.Map{})
		if err != nil || args["worker"] != call.Each.Items[i] {
			t.Fatalf("string changed: %q -> %q (%v)", call.Each.Items[i], args["worker"], err)
		}
	}
}
