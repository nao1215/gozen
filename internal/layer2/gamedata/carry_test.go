package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestCarryCheck: carry needs schema 10 and game.yaml carry to name the values.
func TestCarryCheck(t *testing.T) {
	for _, tc := range []struct {
		name, head, want string
	}{
		{"ok", "schema_version: 10\nid: neutral\ncarry: [var.runs, flag.seen_*]\n", ""},
		{"no names", "schema_version: 10\nid: neutral\n", "carry needs game.yaml carry"},
		{"old schema", "schema_version: 9\nid: neutral\ncarry: [var.runs]\n", "schema_version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := testgame.TryLoad(map[string]string{"game.yaml": tc.head + testgame.Game, "data/z.yaml": "events: {end: {do: [{carry: true}]}}"})
			if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}
