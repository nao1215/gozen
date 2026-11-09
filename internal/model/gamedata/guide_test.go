package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func loadGuide(t *testing.T, version, guide, entry, view string) error {
	t.Helper()
	board := func(name, extra string) string {
		return "  " + name + ":\n    title: B\n    kind: workspace\n    view: {fullscreen: true" + view + "}\n" + extra +
			"    entries:\n    - {label: Hours, style: plate, bounds: [0, 0, 200, 80]}\n    - {label: Go, style: button, bounds: [0, 100, 200, 48], do: [{set: {var.n: 1}}]" + entry + "}\n"
	}
	src := "schema_version: " + version + "\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {glaze: {name: Glaze, effort: 2}}\nscreens:\n" +
		board("one", guide) + board("two", "") +
		"  plain:\n    title: P\n    entries: [{label: Hours}]\n" +
		"flows:\n  day:\n    steps: [{label: One, screen: one}, {label: Two, screen: two}]\n"
	_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("actors: {ann: {name: Ann}}\nlocations: {room: {name: Workshop}}\n")}})
	return err
}

// A guide's lines have a speaker and text; focus names the screen's marks or
// next; until needs focus; the conditions are true or false.
func TestScreenGuides(t *testing.T) {
	ok := "    guide:\n      when: 'var.n == 0'\n      steps:\n      - {who: ann, text: Look first}\n      - {who: ann, text: Press it, focus: [go], until: 'var.n > 0'}\n      - {who: ann, text: On, focus: [next]}\n"
	if err := loadGuide(t, "10", ok, ", mark: go", ", header_face: x.png, header_tint: '#ffaa00'"); err == nil || !strings.Contains(err.Error(), "no file assets/x.png") {
		t.Fatal("a missing header face passed", err)
	}
	if err := loadGuide(t, "10", ok, ", mark: go", ", header_face: '{var.face}', header_tint: '#ffaa00'"); err != nil {
		t.Fatal(err)
	}
	if err := loadGuide(t, "9", ok, ", mark: go", ""); err == nil || !strings.Contains(err.Error(), "schema_version: 10") {
		t.Fatal("a guide loaded below schema 10", err)
	}
	for _, tc := range []struct{ name, guide, entry, view, want string }{
		{"unknown mark", ok, "", "", `no entry of screen "one" has mark "go"`},
		{"no speaker", "    guide:\n      steps: [{text: Hi}]\n", "", "", "needs a speaker"},
		{"unknown speaker", "    guide:\n      steps: [{who: bob, text: Hi}]\n", "", "", `unknown actor "bob"`},
		{"no text", "    guide:\n      steps: [{who: ann}]\n", "", "", "needs text"},
		{"no steps", "    guide: {steps: []}\n", "", "", "1 to 8 steps"},
		{"until without focus", "    guide:\n      steps: [{who: ann, text: Hi, until: 'var.n > 0'}]\n", "", "", "until ends a step with focus"},
		{"when not a truth", "    guide:\n      when: '1 + 1'\n      steps: [{who: ann, text: Hi}]\n", "", "", "must be true or false"},
		{"mark named next", "", ", mark: next", "", "a mark is a plain identifier"},
		{"bad tint", "", "", ", header_tint: orange", "a header tint is a #rrggbb color"},
	} {
		err := loadGuide(t, "10", tc.guide, tc.entry, tc.view)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}
