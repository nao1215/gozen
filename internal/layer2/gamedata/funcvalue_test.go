package gamedata_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestFunctionAsValueWarning: an expression that uses a function's name as a value (floor + 1
// in a tile's commands, rand > 1) is warned of with GZN0114, since evaluating it fails; a call,
// and floor where it is the floor's number (a generated floor's count), are not.
func TestFunctionAsValueWarning(t *testing.T) {
	db := testgame.Load(t, map[string]string{
		"data/z.yaml": `events: {e: {do: [
{gold: "floor + 1"},
{if: "rand > 1", then: [{gold: 1}]},
{gold: "floor(gold / 2) + rand(3)"},
{if: "chance(0.5)", then: [{gold: 1}]}
]}}
locations:
  depths:
    name: Depths
    dungeon:
      generate: {width: 11, height: 11, rooms: 2, start: S, size: "floor <= 2 ? 9 : 11", place: [{char: T, count: "floor < 5 ? 2 : 1"}]}
      tiles:
        S: {name: Up}
        T: {name: Chest, solid: "true", do: [{gold: "floor + 1"}]}
`,
	})
	w := errors.Join(db.SceneWarnings()...)
	if w == nil {
		t.Fatal("no warnings")
	}
	text := w.Error()
	for _, want := range []string{`events.e.do[0].gold: "floor + 1" uses the function floor as a value`, `"rand > 1" uses the function rand as a value`, `tiles.T.do[0].gold: "floor + 1" uses the function floor`} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	if !strings.Contains(text, diag.FunctionValue.String()) {
		t.Errorf("not coded %s:\n%s", diag.FunctionValue, text)
	}
	for _, not := range []string{"floor(gold", "chance(0.5)", "floor < 5", "floor <= 2"} {
		if strings.Contains(text, not) {
			t.Errorf("warned of %s:\n%s", not, text)
		}
	}
}
