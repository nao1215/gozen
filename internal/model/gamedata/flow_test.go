package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func loadFlow(t *testing.T, version, flows string) error {
	t.Helper()
	board := func(name string, extra string) string {
		return "  " + name + ":\n    title: B\n    kind: workspace\n    view: {fullscreen: true}\n" + extra + "    entries: [{label: Hours, style: plate, bounds: [0, 0, 200, 80]}]\n"
	}
	src := "schema_version: " + version + "\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {glaze: {name: Glaze, effort: 2}}\nscreens:\n" +
		board("one", "") + board("two", "") + board("three", "") +
		"  plain:\n    title: P\n    entries: [{label: Hours}]\n" + "flows:\n" + flows
	_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
	return err
}

// A flow's steps are parameterless full-screen surfaces, each in one flow, with
// a pure restart value and a true-or-false skip.
func TestFlows(t *testing.T) {
	ok := "  day:\n    steps: [{label: One, screen: one}, {label: Two, screen: two, hold: true, done: 'var.n > 1', when: 'var.n < 5'}, {label: Three, screen: three}]\n    restart: 'planning.date'\n    skip: 'var.n > 0'\n    next: 'Go on'\n"
	if err := loadFlow(t, "10", ok); err != nil {
		t.Fatal(err)
	}
	if err := loadFlow(t, "9", ok); err == nil || !strings.Contains(err.Error(), "schema_version: 10") {
		t.Fatal("a flow loaded below schema 10", err)
	}
	for _, tc := range []struct{ name, flows, want string }{
		{"one step", "  day:\n    steps: [{label: One, screen: one}]\n", "2 to 10 steps"},
		{"unknown screen", "  day:\n    steps: [{label: One, screen: one}, {label: X, screen: nowhere}]\n", "unknown screen"},
		{"not a surface", "  day:\n    steps: [{label: One, screen: one}, {label: P, screen: plain}]\n", "full-screen surface"},
		{"two flows", "  day:\n    steps: [{label: One, screen: one}, {label: Two, screen: two}]\n  night:\n    steps: [{label: Two, screen: two}, {label: Three, screen: three}]\n", "already a step"},
		{"skip not a truth", "  day:\n    steps: [{label: One, screen: one}, {label: Two, screen: two}]\n    skip: '1 + 1'\n", "must be true or false"},
		{"unknown enter event", "  day:\n    steps: [{label: One, screen: one, enter: nowhere}, {label: Two, screen: two}]\n", "unknown event"},
		{"no label", "  day:\n    steps: [{screen: one}, {label: Two, screen: two}]\n", "a step needs a label"},
		{"hold without done", "  day:\n    steps: [{label: One, screen: one, hold: true}, {label: Two, screen: two}]\n", "held step needs done"},
		{"hold on the last step", "  day:\n    steps: [{label: One, screen: one}, {label: Two, screen: two, hold: true, done: 'var.n > 0'}]\n", "held step needs done"},
		{"when not a truth", "  day:\n    steps: [{label: One, screen: one}, {label: Two, screen: two, when: '1 + 1'}, {label: Three, screen: three}]\n", "must be true or false"},
		{"when on the first step", "  day:\n    steps: [{label: One, screen: one, when: 'var.n > 0'}, {label: Two, screen: two}]\n", "always in the flow"},
	} {
		err := loadFlow(t, "10", tc.flows)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}
