package gamedata

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

// MinCarouselHeight leaves room for the boxes, the name and one detail line.
const MinCarouselHeight = 120

// entryParts checks the reading and pressing parts of a bounded entry: a plate
// never acts, a button always does, a ticket card and a carousel need bounds.
func (c *checker) entryParts(ew string, e ScreenEntry) {
	// Specialized layouts must not silently discard authored card parts.
	if e.Style == "button" || e.Ticket != nil || e.Carousel != "" {
		for _, part := range []struct {
			name string
			set  bool
		}{
			{"dot", e.Dot != ""},
			{"sign", e.Sign != ""},
			{"meter", e.Meter != nil},
			{"bust", e.Bust != nil && e.Carousel == ""},
			{"picture", e.Picture != "" && e.Carousel == ""},
			{"icon", e.Icon != "" && e.Ticket == nil},
		} {
			if part.set {
				c.errorf(diag.Shape, ew+"."+part.name, "this layout does not render %s; use an ordinary bounded entry", part.name)
			}
		}
	}
	actionable := len(e.Do) > 0 || e.Destination != ""
	bounded := len(e.Bounds) == 4
	if e.Style != "" && e.Style != "plate" && e.Style != "button" {
		c.errorf(diag.Shape, ew+".style", "style must be plate, button or empty")
	}
	if e.Style != "" && !bounded {
		c.errorf(diag.Shape, ew+".style", "a style requires bounds")
	}
	if e.Style == "plate" && actionable {
		c.errorf(diag.Shape, ew+".style", "a plate is read only: remove do and destination, or use button")
	}
	if e.Style == "button" && !actionable {
		c.errorf(diag.Shape, ew+".style", "a button needs do or a destination; use plate for text that is only read")
	}
	if t := e.Ticket; t != nil {
		if !bounded {
			c.errorf(diag.Shape, ew+".ticket", "a ticket card requires bounds")
		}
		if e.Style != "" || e.Carousel != "" {
			c.errorf(diag.Shape, ew+".ticket", "a ticket card has no style and is not a carousel member")
		}
		c.text(ew+".ticket.due", t.Due, FieldNames)
		c.text(ew+".ticket.state", t.State, FieldNames)
		for i, name := range t.After {
			c.text(fmt.Sprintf("%s.ticket.after[%d]", ew, i), name, FieldNames)
		}
		if len(t.After) > 8 {
			c.errorf(diag.OutOfRange, ew+".ticket.after", "at most 8 prerequisite names")
		}
		for name, x := range map[string]Expr{"planned": t.Planned, "progress": t.Progress, "behind": t.Behind, "blocked": t.Blocked} {
			c.expr(ew+".ticket."+name, x, FieldNames)
			c.planningPure(ew+".ticket."+name, x)
		}
	}
	if e.Carousel != "" {
		if b := e.Bust; b != nil && (b.Width != 0 || b.Height != 0 || b.Bare) {
			c.errorf(diag.Shape, ew+".bust", "carousel busts use the box size; remove width, height and bare")
		}
		if !varName.MatchString(e.Carousel) {
			c.errorf(diag.Shape, ew+".carousel", "a carousel name is a plain identifier")
		}
		if !bounded || !actionable || e.Target.set() {
			c.errorf(diag.Shape, ew+".carousel", "a carousel member is a bounded action with do or a destination")
		}
		if e.Style != "" {
			c.errorf(diag.Shape, ew+".carousel", "a carousel member has no style")
		}
		if bounded && e.Bounds[3] < MinCarouselHeight {
			c.errorf(diag.OutOfRange, ew+".bounds", "a carousel needs a height of at least %d", MinCarouselHeight)
		}
	}
	if (e.Unavailable.Set() || e.Reason != "") && (!bounded || !actionable) {
		c.errorf(diag.Shape, ew+".unavailable", "unavailable and reason belong to a bounded action")
	}
	if e.Cost != "" && !actionable {
		c.errorf(diag.Shape, ew+".cost", "cost belongs to an action")
	}
	c.text(ew+".cost", e.Cost, FieldNames)
	c.text(ew+".reason", e.Reason, FieldNames)
	c.expr(ew+".unavailable", e.Unavailable, FieldNames)
	c.planningPure(ew+".unavailable", e.Unavailable)
}

// carousels keeps one rotating selector per screen whose members share bounds.
func (c *checker) carousels(w string, screen *AuthorScreen) {
	name := ""
	var bounds []int
	for i, e := range screen.Entries {
		if e.Carousel == "" {
			continue
		}
		ew := fmt.Sprintf("%s.entries[%d].carousel", w, i)
		if name == "" {
			name, bounds = e.Carousel, e.Bounds
			if screen.Kind != "workspace" || !screen.View.Fullscreen {
				c.errorf(diag.Shape, ew, "a carousel requires a full-screen workspace")
			}
			continue
		}
		if e.Carousel != name {
			c.errorf(diag.Shape, ew, "one carousel per screen: %q is already %q", e.Carousel, name)
		}
		if !slices.Equal(e.Bounds, bounds) {
			c.errorf(diag.Shape, w+fmt.Sprintf(".entries[%d].bounds", i), "every member of carousel %q shares the same bounds", name)
		}
	}
}

// entryDrag checks the carried and dropped parts of a bounded entry and the
// peek panel. c.params holds the row's bindings; the drop's guard and hint are
// checked with the carried value bound too.
func (c *checker) entryDrag(ew string, screen *AuthorScreen, i int) {
	e := screen.Entries[i]
	if e.Sign != "" {
		if len(e.Bounds) != 4 {
			c.errorf(diag.Shape, ew+".sign", "a sign needs a bounded entry")
		}
		if !strings.Contains(e.Sign, "{") {
			c.file(ew+".sign", e.Sign)
		}
		c.text(ew+".sign", e.Sign, FieldNames)
	}
	if b := e.Bust; b != nil {
		switch {
		case len(e.Bounds) != 4:
			c.errorf(diag.Shape, ew+".bust", "a bust needs a bounded entry")
		case b.Picture == "":
			c.errorf(diag.Shape, ew+".bust.picture", "a bust needs a picture")
		case b.Width < 0 || b.Width > 1 || b.Height < 0 || b.Height > 1 || b.Top < 0 || b.Top >= 1 || b.Face < 0 || b.Face >= 1:
			c.errorf(diag.OutOfRange, ew+".bust", "width and height are fractions up to 1, top and face below 1")
		}
		if b.Fit != "" && b.Fit != "contain" {
			c.errorf(diag.OutOfRange, ew+".bust.fit", "want contain, got %q", b.Fit)
		}
		if b.Picture != "" && !strings.Contains(b.Picture, "{") {
			c.file(ew+".bust.picture", b.Picture)
		}
		c.text(ew+".bust.picture", b.Picture, FieldNames)
	}
	if m := e.Meter; m != nil {
		if len(e.Bounds) != 4 || !m.Progress.Set() {
			c.errorf(diag.Shape, ew+".meter", "a meter needs a bounded entry and progress")
		}
		for _, part := range []struct {
			name string
			x    Expr
		}{{"progress", m.Progress}, {"planned", m.Planned}, {"behind", m.Behind}} {
			c.expr(ew+".meter."+part.name, part.x, FieldNames)
			c.planningPure(ew+".meter."+part.name, part.x)
		}
	}
	if e.Dot != "" {
		if len(e.Bounds) != 4 {
			c.errorf(diag.Shape, ew+".dot", "a status dot needs a bounded entry")
		}
		if !strings.Contains(e.Dot, "{") && !dotColor.MatchString(e.Dot) {
			c.errorf(diag.Shape, ew+".dot", "a status dot is a #rrggbb color")
		}
		c.text(ew+".dot", e.Dot, FieldNames)
	}
	if e.Drag == "" && e.Drop == "" && !e.Accepts.Set() && e.Hint == "" && e.Peek == "" {
		return
	}
	bounded := len(e.Bounds) == 4
	surface := screen.View.Fullscreen
	if e.Drag != "" {
		switch {
		case !bounded || !surface:
			c.errorf(diag.Shape, ew+".drag", "a carried entry needs bounds on a full-screen surface")
		case len(e.Do) > 0 || e.Destination != "" || e.Carousel != "" || e.Target.set() || e.Drop != "" || e.Peek != "":
			c.errorf(diag.Shape, ew+".drag", "a carried entry has no do, destination, carousel, target, drop or peek")
		}
		c.text(ew+".drag", e.Drag, FieldNames)
	}
	if e.Drop != "" {
		taken := screen.Params[e.Drop] != "" || e.Each != nil && e.Each.Param == e.Drop
		switch {
		case !varName.MatchString(e.Drop) || taken:
			c.errorf(diag.Shape, ew+".drop", "name a new parameter for the carried value without shadowing screen or each arguments")
		case !bounded || !surface:
			c.errorf(diag.Shape, ew+".drop", "a drop place needs bounds on a full-screen surface")
		case len(e.Do) == 0:
			c.errorf(diag.Shape, ew+".drop", "a drop place needs do: it runs when something is dropped on it")
		case e.Destination != "" || e.Carousel != "" || e.Target.set() || e.Peek != "":
			c.errorf(diag.Shape, ew+".drop", "a drop place has no destination, carousel, target or peek")
		}
	}
	if (e.Accepts.Set() || e.Hint != "") && e.Drop == "" {
		c.errorf(diag.Shape, ew+".accepts", "accepts and hint belong to a drop place")
	}
	if e.Peek != "" {
		switch {
		case !bounded || !surface:
			c.errorf(diag.Shape, ew+".peek", "a peek needs bounds on a full-screen surface")
		case len(e.Do) > 0 || e.Destination != "" || e.Carousel != "" || e.Target.set():
			c.errorf(diag.Shape, ew+".peek", "a peek only opens text: remove do, destination, carousel and target")
		}
		c.text(ew+".peek", e.Peek, FieldNames)
	}
	if e.Drop == "" {
		return
	}
	row := c.params
	c.params = screen.entryParams(i)
	c.expr(ew+".accepts", e.Accepts, FieldNames)
	c.planningPure(ew+".accepts", e.Accepts)
	if e.Accepts.Set() {
		if t := c.resultType(e.Accepts, FieldNames); t != expr.UnknownType && t != expr.BoolType {
			c.errorf(diag.Expression, ew+".accepts", "must be true or false")
		}
	}
	c.text(ew+".hint", e.Hint, FieldNames)
	c.params = row
}

// dotColor is the literal form of a status dot color.
var dotColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
