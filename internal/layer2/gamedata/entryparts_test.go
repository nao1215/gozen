package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

// loadBoard loads a workspace screen named board with the given entries.
func loadBoard(t *testing.T, version, entries string) error {
	t.Helper()
	src := "schema_version: " + version + "\nid: workshop\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {glaze: {name: Glaze, effort: 2}, fire: {name: Fire, effort: 1, after: [glaze]}}\nscreens:\n  board:\n    title: Board\n    kind: workspace\n    view: {fullscreen: true}\n    entries:\n" + entries
	_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}, "assets/kiln.png": {Data: []byte("x")}})
	return err
}

// A plate is only read and a button is only pressed; the checker keeps them apart.
func TestEntryStyleSeparatesReadingFromPressing(t *testing.T) {
	for _, tc := range []struct {
		name, entries, want string
	}{
		{"plate", "      - {label: Hours, text: 8h, style: plate, bounds: [0, 0, 200, 80]}\n", ""},
		{"button", "      - {label: Open, style: button, bounds: [0, 0, 120, 38], do: [{set: {var.n: 1}}]}\n", ""},
		{"plate that acts", "      - {label: Hours, style: plate, bounds: [0, 0, 200, 80], do: [{set: {var.n: 1}}]}\n", "plate is read only"},
		{"button that only reads", "      - {label: Hours, style: button, bounds: [0, 0, 200, 80]}\n", "a button needs do"},
		{"unknown style", "      - {label: Hours, style: banner, bounds: [0, 0, 200, 80]}\n", "style must be"},
		{"style without bounds", "      - {label: Hours, style: plate}\n", "a style requires bounds"},
	} {
		err := loadBoard(t, "10", tc.entries)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

// Ticket cards take interpolated text and pure fractions and need bounds.
func TestEntryTicketCard(t *testing.T) {
	ok := "      - label: '{activity.fire.name}'\n        bounds: [0, 0, 280, 150]\n        destination: board\n        ticket: {due: '2030-08-01', state: open, planned: '0.5', progress: 'activity.fire.progress/activity.fire.effort', behind: 'false', blocked: 'activity.glaze.progress < activity.glaze.effort', after: ['{activity.glaze.name}', '']}\n"
	if err := loadBoard(t, "10", ok); err != nil {
		t.Fatal(err)
	}
	if err := loadBoard(t, "9", ok); err == nil || !strings.Contains(err.Error(), "schema_version: 10") {
		t.Fatal("a ticket card loaded below schema 10", err)
	}
	for _, tc := range []struct{ entries, want string }{
		{"      - {label: Fire, ticket: {due: x}}\n", "a ticket card requires bounds"},
		{"      - {label: Fire, bounds: [0, 0, 280, 150], ticket: {planned: 'var.x +'}}\n", ""},
		{"      - {label: Fire, style: plate, bounds: [0, 0, 280, 150], ticket: {due: x}}\n", "a ticket card has no style"},
	} {
		err := loadBoard(t, "10", tc.entries)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: %v", tc.entries, err)
		}
	}
}

// A carousel is one per screen, its members are bounded actions sharing bounds,
// and a refused member needs a bounded action to refuse.
func TestEntryCarousel(t *testing.T) {
	member := func(label, bounds, extra string) string {
		return "      - {label: " + label + ", carousel: tools, picture: kiln.png, cost: 1 slot, bounds: " + bounds + ", do: [{set: {var.n: 1}}]" + extra + "}\n"
	}
	band := "[200, 500, 800, 190]"
	if err := loadBoard(t, "10", member("Fire", band, "")+member("Glaze", band, ", unavailable: 'var.n > 0', reason: Busy")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ entries, want string }{
		{member("Fire", band, "") + member("Glaze", "[200, 500, 800, 180]", ""), "shares the same bounds"},
		{member("Fire", band, "") + strings.Replace(member("Glaze", band, ""), "carousel: tools", "carousel: other", 1), "one carousel per screen"},
		{member("Fire", "[200, 500, 800, 100]", ""), "at least 120"},
		{"      - {label: Fire, carousel: tools, bounds: " + band + "}\n", "a carousel member is a bounded action"},
		{"      - {label: Hours, text: 8h, bounds: [0, 0, 200, 80], unavailable: 'true'}\n", "belong to a bounded action"},
		{"      - {label: Hours, bounds: [0, 0, 200, 80], cost: 1 slot}\n", "cost belongs to an action"},
	} {
		err := loadBoard(t, "10", tc.entries)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: %v", tc.entries, err)
		}
	}
}

func TestEntryPartsRejectIgnoredFieldsAndMissingBust(t *testing.T) {
	for _, extra := range []string{"dot: '#ff0000'", "meter: {progress: '0.5'}", "bust: {picture: kiln.png}", "picture: kiln.png"} {
		err := loadBoard(t, "10", "      - {label: Open, style: button, bounds: [0, 0, 200, 100], destination: board, "+extra+"}\n")
		if err == nil || !strings.Contains(err.Error(), "does not render") {
			t.Errorf("%s: %v", extra, err)
		}
	}
	err := loadBoard(t, "10", "      - {label: Portrait, style: plate, bounds: [0, 0, 200, 100], bust: {picture: missing.png}}\n")
	if err == nil || !strings.Contains(err.Error(), "no file assets/missing.png") {
		t.Fatal(err)
	}
}
