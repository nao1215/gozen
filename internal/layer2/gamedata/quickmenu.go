package gamedata

import "github.com/nao1215/gozen/internal/diag"

// novelWindow checks ui.quick_menu and ui.novel_window (schema_version 10).
func (c *checker) novelWindow(u *UIStyle) {
	if u.QuickMenu != "" {
		c.needsVersion("game.yaml ui.quick_menu", "ui.quick_menu", SchemaVersion10)
		if u.QuickMenu != "below" && u.QuickMenu != "inside" {
			c.errorf(diag.OutOfRange, "game.yaml ui.quick_menu", "want below or inside, got %q", u.QuickMenu)
		}
	}
	if u.ChoicePosition != "" {
		c.needsVersion("game.yaml ui.choice_position", "ui.choice_position", SchemaVersion10)
		if u.ChoicePosition != "center" {
			c.errorf(diag.OutOfRange, "game.yaml ui.choice_position", "want center, got %q", u.ChoicePosition)
		}
	}
	if u.WheelUp != "" {
		c.needsVersion("game.yaml ui.wheel_up", "ui.wheel_up", SchemaVersion10)
		if u.WheelUp != "log" {
			c.errorf(diag.OutOfRange, "game.yaml ui.wheel_up", "want log, got %q", u.WheelUp)
		}
	}
	if u.Title != nil {
		c.needsVersion("game.yaml ui.title", "ui.title", SchemaVersion10)
	}
	if m := u.Material; m != nil {
		for role, o := range m.Opacities {
			if !materialRole(role) || o < 0 || o > 1 {
				c.errorf(diag.OutOfRange, "game.yaml ui.material.opacities", "want base, card, control, chart, message or choice with an opacity in [0,1], got %s: %g", role, o)
			}
		}
	}
	w := u.NovelWindow
	if w == nil {
		return
	}
	c.needsVersion("game.yaml ui.novel_window", "ui.novel_window", SchemaVersion10)
	if w.X < 0 || w.Width < 0 || w.Width > 1 || w.X+w.Width > 1 {
		c.errorf(diag.OutOfRange, "game.yaml ui.novel_window", "x and width are fractions of the screen's width, and x+width must stay within 1 (got x %g, width %g)", w.X, w.Width)
	}
	if w.X != 0 && w.Width == 0 {
		c.errorf(diag.Shape, "game.yaml ui.novel_window", "x needs a width")
	}
	if w.Width != 0 && w.Width < 0.3 {
		c.errorf(diag.OutOfRange, "game.yaml ui.novel_window.width", "want at least 0.3 of the screen's width for the lines and the buttons, got %g", w.Width)
	}
	if w.Bottom < 0 || w.Bottom > 0.4 {
		c.errorf(diag.OutOfRange, "game.yaml ui.novel_window.bottom", "want a fraction of the screen's height in [0, 0.4], got %g", w.Bottom)
	}
}

// materialRole reports whether role names a role of panel of ui.material.
func materialRole(role string) bool {
	switch role {
	case "base", "card", "control", "chart", "message", "choice":
		return true
	}
	return false
}
