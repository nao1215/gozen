package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

// TestDungeonNumbersAreFiniteAndBounded: a darkness, a loop rate and a tile size
// written as .nan or .inf are refused, and so is a depth above MaxDungeonDepth.
func TestDungeonNumbersAreFiniteAndBounded(t *testing.T) {
	for _, c := range []struct{ name, old, repl string }{
		{"darkness nan", "      generate:", "      darkness: .nan\n      generate:"},
		{"look darkness nan", "      generate:", "      looks: [{when: \"true\", darkness: .nan}]\n      generate:"},
		{"loops nan", "rooms: 2", "rooms: 2\n        loops: .nan"},
		{"tile size nan", "S: {name: Up}", "S: {name: Up, size: .nan}"},
		{"tile size inf", "S: {name: Up}", "S: {name: Up, size: .inf}"},
		{"depth huge", "      generate:", "      depth: 2000000000\n      generate:"},
	} {
		src := strings.Replace(madeDungeon, c.old, c.repl, 1)
		if src == madeDungeon {
			t.Fatalf("%s: fixture not changed", c.name)
		}
		if _, err := testgame.TryLoad(map[string]string{"data/z.yaml": src}); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
}
