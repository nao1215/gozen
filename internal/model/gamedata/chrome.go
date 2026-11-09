package gamedata

import (
	"slices"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"

	"github.com/nao1215/gozen/internal/base/diag"
)

// CallControl is a decorative button. Bare labels have no icon or exit styling.
type CallControl struct {
	Label string `yaml:"label"`
	// Icon names mic, camera, share, hand, chat, leave or none. Empty draws no icon.
	Icon string `yaml:"icon"`
	// Color fills the round button. Unset uses the call's control color, or the
	// leave color for the leave icon.
	Color Color `yaml:"color"`
}

// UnmarshalYAML reads a control written as its label or as a map.
func (c *CallControl) UnmarshalYAML(n ast.Node) error {
	if x, ok := n.(*ast.StringNode); ok {
		*c = CallControl{Label: x.Value}
		return nil
	}
	type plain CallControl
	return yaml.NodeToValue(n, (*plain)(c), yaml.Strict())
}

// ControlIcon is the icon the control at index i draws.
func (c CallControl) ControlIcon(_ int) string {
	if c.Icon != "" {
		return c.Icon
	}
	return "none"
}

// ControlLeave reports explicit exit decoration; labels have no implied meaning.
func (c CallControl) ControlLeave(_, _ int) bool { return c.Icon == "leave" }

// CallColors recolors the call surface. Unset colors keep the built-in look.
type CallColors struct {
	// Body is the browser body behind the call.
	Body Color `yaml:"body"`
	// Tile is a participant's empty camera tile.
	Tile Color `yaml:"tile"`
	// Speaking is the ring and wave drawn around the participant who speaks.
	Speaking Color `yaml:"speaking"`
	// Control and Leave fill the round control buttons.
	Control Color `yaml:"control"`
	Leave   Color `yaml:"leave"`
}

// BurnupColors recolors a burnup chart. Unset colors keep the built-in look.
type BurnupColors struct {
	Planned  Color `yaml:"planned"`
	Observed Color `yaml:"observed"`
	Total    Color `yaml:"total"`
	Today    Color `yaml:"today"`
	Grid     Color `yaml:"grid"`
}

// ChromeStyle recolors the title bar and browser decoration drawn around
// windowed planning surfaces and calls. The buttons are decorative.
type ChromeStyle struct {
	Bar   Color `yaml:"bar"`
	Text  Color `yaml:"text"`
	Field Color `yaml:"field"`
	Frame Color `yaml:"frame"`
	// Buttons are the window's decorative buttons, as text. Empty hides them;
	// omitted keeps the minimize, maximize and close marks.
	Buttons *string `yaml:"buttons"`
	// Nav is the browser's back and forward marks, as text. Empty hides them.
	Nav *string `yaml:"nav"`
}

// ToneStyle is the fill of the authored tones of workspace entries.
type ToneStyle struct {
	Note        Color `yaml:"note"`
	Primary     Color `yaml:"primary"`
	PrimaryCard Color `yaml:"primary_card"`
	PrimaryText Color `yaml:"primary_text"`
}

// LightStyle is the quiet paper palette of view.light surfaces.
type LightStyle struct {
	Paper  Color `yaml:"paper"`
	Window Color `yaml:"window"`
	Text   Color `yaml:"text"`
	Dim    Color `yaml:"dim"`
	Accent Color `yaml:"accent"`
}

func (c *checker) callControls(w string, call *EnsembleCall) {
	for _, ctl := range call.Controls {
		if ctl.Icon != "" && !slices.Contains([]string{"mic", "camera", "share", "hand", "chat", "leave", "none"}, ctl.Icon) {
			c.errorf(diag.OutOfRange, w+".icon", "want one of mic, camera, share, hand, chat, leave or none, not %q", ctl.Icon)
		}
	}
}

// callSelf reports whether a call's self names a participant rather than a picture.
func (c *checker) callSelf(self string) bool {
	if c.db.Actors[self] != nil {
		return true
	}
	return c.isEntity(self)
}

// isEntity reports whether name is a planning entity.
func (c *checker) isEntity(name string) bool {
	p := c.db.Game.Planning
	if p == nil {
		return false
	}
	_, ok := p.Entities[name]
	return ok
}
