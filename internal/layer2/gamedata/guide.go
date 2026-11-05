package gamedata

import (
	"fmt"
	"slices"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

// MaxGuideSteps bounds a screen guide; a longer one is a manual, not a guide.
const MaxGuideSteps = 8

// GuideNext is the focus name of the flow's next action.
const GuideNext = "next"

// GuideKey is the record a finished guide leaves beside the gallery's
// unlocks, so it never shows again in a later play.
func GuideKey(screen string) string { return "guide:" + screen }

// guide checks a screen's first-visit guide: a full-screen surface, one to
// eight lines with a speaker and text, focus names that are marks of the
// screen's bounded entries (or next on a flow step with a next action), and
// pure conditions.
func (c *checker) guide(w string, name string, screen *AuthorScreen) {
	marks := map[string]bool{}
	for i, e := range screen.Entries {
		if e.Mark == "" {
			continue
		}
		ew := fmt.Sprintf("%s.entries[%d].mark", w, i)
		if !varName.MatchString(e.Mark) || e.Mark == GuideNext {
			c.errorf(diag.Shape, ew, "a mark is a plain identifier other than %s", GuideNext)
		}
		if len(e.Bounds) != 4 {
			c.errorf(diag.Shape, ew, "a mark names a bounded entry")
		}
		marks[e.Mark] = true
	}
	gd := screen.Guide
	if gd == nil {
		return
	}
	gw := w + ".guide"
	c.needsVersion(gw, "screen guides", SchemaVersion10)
	if !screen.View.Fullscreen {
		c.errorf(diag.Shape, gw, "a guide belongs to a full-screen surface")
	}
	if len(gd.Steps) == 0 || len(gd.Steps) > MaxGuideSteps {
		c.errorf(diag.Shape, gw+".steps", "a guide has 1 to %d steps", MaxGuideSteps)
	}
	c.expr(gw+".when", gd.When, FieldNames)
	c.planningPure(gw+".when", gd.When)
	c.boolean(gw+".when", gd.When)
	flow, step := c.db.FlowOf(name)
	hasNext := flow != "" && step < len(c.db.Game.Flows[flow].Steps)-1 && !c.db.Game.Flows[flow].Steps[step].Hold
	for i, st := range gd.Steps {
		sw := fmt.Sprintf("%s.steps[%d]", gw, i)
		if st.Who == "" {
			c.errorf(diag.Shape, sw+".who", "a guide line needs a speaker")
		} else {
			_, ok := c.db.Actors[st.Who]
			c.ref(sw+".who", "actor", st.Who, ok)
		}
		if st.Text == "" {
			c.errorf(diag.Shape, sw+".text", "a guide line needs text")
		}
		c.text(sw+".text", st.Text, FieldNames)
		for k, f := range st.Focus {
			fw := fmt.Sprintf("%s.focus[%d]", sw, k)
			switch {
			case f == GuideNext && !hasNext:
				c.errorf(diag.Shape, fw, "next is the next action of a flow step that is not the last")
			case f != GuideNext && !marks[f]:
				c.errorf(diag.UnknownReference, fw, "no entry of screen %q has mark %q", name, f)
			case slices.Contains(st.Focus[:k], f):
				c.errorf(diag.Shape, fw, "focus %q is named twice", f)
			}
		}
		if st.Until.Set() && len(st.Focus) == 0 {
			c.errorf(diag.Shape, sw+".until", "until ends a step with focus; a line without focus is pressed")
		}
		c.expr(sw+".until", st.Until, FieldNames)
		c.planningPure(sw+".until", st.Until)
		c.boolean(sw+".until", st.Until)
	}
}

// boolean reports a set condition whose result is not true or false.
func (c *checker) boolean(where string, x Expr) {
	if !x.Set() {
		return
	}
	if t := c.resultType(x, FieldNames); t != expr.UnknownType && t != expr.BoolType {
		c.errorf(diag.Expression, where, "must be true or false")
	}
}
