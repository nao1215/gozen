package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

// A carried entry and a drop place are checked apart: the carried value is in
// scope only in the drop's action, guard and hint.
func TestEntryDragAndDrop(t *testing.T) {
	source := "      - {label: Bowl, drag: bowl, bounds: [0, 0, 120, 80]}\n"
	place := func(extra string) string {
		return "      - {label: Shelf, drop: item, bounds: [200, 0, 120, 80], do: [{set: {var.got: 'arg.item'}}]" + extra + "}\n"
	}
	if err := loadBoard(t, "10", source+place(", accepts: 'arg.item != \"cup\"', hint: 'Put {arg.item} here'")); err != nil {
		t.Fatal(err)
	}
	if err := loadBoard(t, "9", source+place("")); err == nil || !strings.Contains(err.Error(), "schema_version: 10") {
		t.Fatal("drag and drop loaded below schema 10", err)
	}
	for _, tc := range []struct{ name, entries, want string }{
		{"drag with do", "      - {label: Bowl, drag: bowl, bounds: [0, 0, 120, 80], do: [{set: {var.n: 1}}]}\n", "a carried entry has no do"},
		{"drag without bounds", "      - {label: Bowl, drag: bowl}\n", "a carried entry needs bounds"},
		{"drop without do", "      - {label: Shelf, drop: item, bounds: [0, 0, 120, 80]}\n", "a drop place needs do"},
		{"drop with destination", "      - {label: Shelf, drop: item, bounds: [0, 0, 120, 80], destination: board, do: [{set: {var.n: 1}}]}\n", "a drop place has no destination"},
		{"drop that shadows each", "      - {label: Shelf, drop: id, each: {param: id, items: [a]}, bounds: [0, 0, 120, 80], do: [{set: {var.n: 1}}]}\n", "without shadowing"},
		{"drop that is not a name", "      - {label: Shelf, drop: 'a-b', bounds: [0, 0, 120, 80], do: [{set: {var.n: 1}}]}\n", "without shadowing"},
		{"label sees the carried value", "      - {label: 'Shelf {arg.item}', drop: item, bounds: [0, 0, 120, 80], do: [{set: {var.n: 1}}]}\n", "arg.item"},
		{"accepts without drop", "      - {label: Shelf, accepts: 'true', bounds: [0, 0, 120, 80], do: [{set: {var.n: 1}}]}\n", "accepts and hint belong to a drop place"},
		{"accepts not a truth", source + place(", accepts: '1 + 2'"), "must be true or false"},
		{"peek with do", "      - {label: Day, peek: Busy, bounds: [0, 0, 120, 80], do: [{set: {var.n: 1}}]}\n", "a peek only opens text"},
	} {
		err := loadBoard(t, "10", tc.entries)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if err := loadBoard(t, "10", "      - {label: Day, style: plate, peek: 'Busy {planning.date}', bounds: [0, 0, 120, 80]}\n"); err != nil {
		t.Fatal("a plate with a peek did not load", err)
	}
}

// A status dot is a bounded entry's small colored sign: a literal must be
// #rrggbb, an interpolated one is checked as text.
func TestEntryDot(t *testing.T) {
	if err := loadBoard(t, "10", "      - {label: Ann, style: plate, dot: '#7fc8a9', bounds: [0, 0, 120, 80]}\n"); err != nil {
		t.Fatal(err)
	}
	if err := loadBoard(t, "10", "      - {label: Ann, style: plate, dot: '{var.n > 1 ? \"#7fc8a9\" : \"#e8a07a\"}', bounds: [0, 0, 120, 80]}\n"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, entries, want string }{
		{"not a color", "      - {label: Ann, dot: green, bounds: [0, 0, 120, 80]}\n", "#rrggbb"},
		{"no bounds", "      - {label: Ann, dot: '#7fc8a9'}\n", "a status dot needs a bounded entry"},
	} {
		if err := loadBoard(t, "10", tc.entries); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
	if err := loadBoard(t, "9", "      - {label: Ann, dot: '#7fc8a9', bounds: [0, 0, 120, 80]}\n"); err == nil {
		t.Fatal("a status dot loaded below schema 10")
	}
}

// A bust is a bounded crop of a picture; a meter needs progress.
func TestEntryBustAndMeter(t *testing.T) {
	ok := "      - {label: Ann, style: plate, bust: {picture: 'kiln.png', width: .4, top: .1, face: .3}, meter: {progress: '.5', planned: '.6', behind: 'true'}, bounds: [0, 0, 300, 160]}\n"
	if err := loadBoard(t, "10", ok); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, entries, want string }{
		{"bust without picture", "      - {label: Ann, bust: {width: .4}, bounds: [0, 0, 300, 160]}\n", "a bust needs a picture"},
		{"bust too wide", "      - {label: Ann, bust: {picture: a.png, width: 1.5}, bounds: [0, 0, 300, 160]}\n", "fractions up to 1"},
		{"bust without bounds", "      - {label: Ann, bust: {picture: a.png}}\n", "a bust needs a bounded entry"},
		{"meter without progress", "      - {label: Ann, meter: {planned: '.5'}, bounds: [0, 0, 300, 160]}\n", "a meter needs a bounded entry and progress"},
	} {
		if err := loadBoard(t, "10", tc.entries); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestFaceShape(t *testing.T) {
	load := func(view, ui string) error {
		src := "schema_version: 10\nid: workshop\nstart: {location: room}\n" + ui + "planning:\n  calendar: {start: '2030-07-31', end: '2030-08-02', phases: [morning]}\n  activities: {glaze: {name: Glaze, effort: 2}}\nscreens:\n  board:\n    title: Board\n    kind: workspace\n    view: {fullscreen: true" + view + "}\n    entries:\n      - {label: Ann, style: plate, bounds: [0, 0, 300, 160]}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Workshop}}\n")}})
		return err
	}
	if err := load(", face_shape: square", "ui: {face_shape: round}\n"); err != nil {
		t.Fatal(err)
	}
	if err := load(", face_shape: oval", ""); err == nil || !strings.Contains(err.Error(), "face_shape") {
		t.Fatal("an unknown view face_shape passed", err)
	}
	if err := load("", "ui: {face_shape: oval}\n"); err == nil || !strings.Contains(err.Error(), "face_shape") {
		t.Fatal("an unknown ui face_shape passed", err)
	}
}

func TestMessageCommandCheck(t *testing.T) {
	load := func(cmd string) error {
		src := "schema_version: 10\nid: workshop\nstart: {location: room}\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(src)}, "data/world.yaml": {Data: []byte("actors: {ann: {name: Ann}}\nlocations: {room: {name: Workshop}}\nevents:\n  ask:\n    do:\n    - choice: [{label: Yes, do: []}]\n  ping:\n    do:\n    - " + cmd + "\n")}})
		return err
	}
	if err := load("message: {who: ann, text: Hello, event: ask}"); err != nil {
		t.Fatal(err)
	}
	if err := load("message: {who: bob, text: Hello, event: ask}"); err == nil || !strings.Contains(err.Error(), "not an actor") {
		t.Fatal("an unknown speaker passed", err)
	}
	if err := load("message: {who: ann, event: ask}"); err == nil || !strings.Contains(err.Error(), "needs text") {
		t.Fatal("an empty message passed", err)
	}
	if err := load("message: {who: ann, text: Hello}"); err == nil || !strings.Contains(err.Error(), "asks for the decision") {
		t.Fatal("a message without its decision passed", err)
	}
}

func TestEntrySign(t *testing.T) {
	if err := loadBoard(t, "10", "      - {label: Ann, style: plate, sign: kiln.png, bounds: [0, 0, 120, 80]}\n"); err != nil {
		t.Fatal(err)
	}
	if err := loadBoard(t, "10", "      - {label: Ann, style: plate, sign: missing.png, bounds: [0, 0, 120, 80]}\n"); err == nil {
		t.Fatal("a missing sign picture passed")
	}
	if err := loadBoard(t, "10", "      - {label: Ann, sign: kiln.png}\n"); err == nil || !strings.Contains(err.Error(), "a sign needs a bounded entry") {
		t.Fatal("a sign without bounds passed", err)
	}
}
