package gamedata

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestScheduleDependencyValidation(t *testing.T) {
	for _, tc := range []struct{ schedule, want string }{
		{"[{id: a, phase: morning, event: scene}, {id: b, phase: morning, event: scene, after: [a]}]", ""},
		{"[{id: a, phase: morning, event: scene, after: [missing]}]", "unknown reservation"},
		{"[{id: a, phase: morning, event: scene, after: [b]}, {id: b, phase: morning, event: scene, after: [a]}]", "cycle"},
		{"[{id: a, phase: morning, event: scene, date: '2030-08-02'}, {id: b, phase: morning, event: scene, date: '2030-08-01', after: [a]}]", "cannot precede"},
	} {
		game := "schema_version: 10\nid: studio\nstart: {location: room}\nplanning:\n  calendar: {start: '2030-08-01', end: '2030-08-03', phases: [morning]}\n  schedule: " + tc.schedule + "\n"
		_, err := Load(fstest.MapFS{"game.yaml": {Data: []byte(game)}, "data/world.yaml": {Data: []byte("locations: {room: {name: Room}}\nevents: {scene: {do: [{say: Hello}]}}\n")}})
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: %v", tc.want, err)
		}
	}
}
