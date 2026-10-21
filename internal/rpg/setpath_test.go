package rpg_test

import (
	"testing"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestSetPathKeepsOnlySavableVariables: a variable holds a number, text, true/false or nothing. A function
// (set: {var.x: rand}) or an object is refused, because it would make the state impossible to save.
func TestSetPathKeepsOnlySavableVariables(t *testing.T) {
	st := testgame.State(testgame.Load(t, nil), 1)
	for name, v := range map[string]any{
		"a built-in function": expr.Func(func([]any) (any, error) { return nil, nil }),
		"an object":           expr.Map{"a": 1.0},
		"a list":              []any{1.0},
	} {
		if err := st.SetPath("var.x", v); err == nil {
			t.Errorf("%s was accepted into a variable", name)
		}
	}
	if _, ok := st.Vars["x"]; ok {
		t.Error("a refused value left a variable behind")
	}
	for name, v := range map[string]any{"number": 2.5, "text": "hi", "bool": true, "nil": nil, "int": 3} {
		if err := st.SetPath("var."+name, v); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := st.Save(); err != nil {
		t.Fatalf("a state of plain variables must be saved: %v", err)
	}
}
