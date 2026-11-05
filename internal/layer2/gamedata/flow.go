package gamedata

import (
	"fmt"
	"maps"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

// MaxFlowSteps bounds the step band of a flow.
const MaxFlowSteps = 10

// FlowOf names the flow a screen is a step of and its step number (-1, "" when none).
func (db *DB) FlowOf(screen string) (string, int) {
	for _, name := range slices.Sorted(maps.Keys(db.Game.Flows)) {
		if f := db.Game.Flows[name]; f != nil {
			for i, st := range f.Steps {
				if st.Screen == screen {
					return name, i
				}
			}
		}
	}
	return "", -1
}

// flows checks game.yaml's flows: each step a parameterless full-screen
// surface in at most one flow, a pure restart value and a pure skip condition.
func (c *checker) flows() {
	g := &c.db.Game
	owner := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(g.Flows)) {
		w := "game.yaml.flows." + name
		c.needsVersion(w, "flows", SchemaVersion10)
		if !declaredName.MatchString(name) {
			c.errorf(diag.NotAWord, w, "want a name of lowercase letters, digits and _, not %q", name)
		}
		f := g.Flows[name]
		if f == nil || len(f.Steps) < 2 || len(f.Steps) > MaxFlowSteps {
			c.errorf(diag.Shape, w+".steps", "a flow has 2 to %d steps", MaxFlowSteps)
			continue
		}
		for i, st := range f.Steps {
			sw := fmt.Sprintf("%s.steps[%d]", w, i)
			if st.Label == "" {
				c.errorf(diag.Shape, sw+".label", "a step needs a label")
			}
			c.text(sw+".label", st.Label, FieldNames)
			screen := g.Screens[st.Screen]
			switch {
			case screen == nil:
				c.errorf(diag.UnknownReference, sw+".screen", "unknown screen %q", st.Screen)
			case len(screen.Params) > 0 || !screen.View.Fullscreen:
				c.errorf(diag.Shape, sw+".screen", "a step is a parameterless full-screen surface")
			case owner[st.Screen] != "":
				c.errorf(diag.Shape, sw+".screen", "screen %q is already a step of flow %q", st.Screen, owner[st.Screen])
			}
			if st.Enter != "" {
				if c.db.Events[st.Enter] == nil {
					c.errorf(diag.UnknownReference, sw+".enter", "unknown event %q", st.Enter)
				} else {
					c.ref(sw+".enter", "event", st.Enter, true)
				}
			}
			c.expr(sw+".done", st.Done, FieldNames)
			c.planningPure(sw+".done", st.Done)
			if st.Done.Set() {
				if t := c.resultType(st.Done, FieldNames); t != expr.UnknownType && t != expr.BoolType {
					c.errorf(diag.Expression, sw+".done", "must be true or false")
				}
			}
			c.expr(sw+".when", st.When, FieldNames)
			c.planningPure(sw+".when", st.When)
			if st.When.Set() {
				if t := c.resultType(st.When, FieldNames); t != expr.UnknownType && t != expr.BoolType {
					c.errorf(diag.Expression, sw+".when", "must be true or false")
				}
				if i == 0 {
					c.errorf(diag.Shape, sw+".when", "the first step is always in the flow")
				}
			}
			if st.Hold && (!st.Done.Set() || i == len(f.Steps)-1) {
				c.errorf(diag.Shape, sw+".hold", "a held step needs done and is not the last step")
			}
			if st.Screen != "" && owner[st.Screen] == "" {
				owner[st.Screen] = name
			}
		}
		c.expr(w+".restart", f.Restart, FieldNames)
		c.planningPure(w+".restart", f.Restart)
		c.expr(w+".skip", f.Skip, FieldNames)
		c.planningPure(w+".skip", f.Skip)
		if f.Skip.Set() {
			if t := c.resultType(f.Skip, FieldNames); t != expr.UnknownType && t != expr.BoolType {
				c.errorf(diag.Expression, w+".skip", "must be true or false")
			}
		}
		c.text(w+".next", f.Next, FieldNames)
	}
}
