package play

import (
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// Preview expressions can read the usual state, including rand/chance, without
// advancing gameplay RNG when the player hovers, pans or redraws the panel.
func (s *atlasScene) previewState(g *Game) *rpg.State {
	if s.location(g).Map.SelectionPanel == nil {
		return g.st
	}
	state := *g.st
	source := rand.PCG{}
	if g.pcg != nil {
		source = *g.pcg
	}
	state.Rand = rand.New(&source) //nolint:gosec // Isolated gameplay preview, not security.
	return &state
}

func (s *atlasScene) panelVisible(g *Game) bool {
	return s.location(g).Map.SelectionPanel != nil && !s.panelHidden && s.sel >= 0 && s.sel < len(s.choices) && !s.walking() && !g.busy()
}

func (s *atlasScene) selectionPanelBox(g *Game) plateBox {
	rect := s.location(g).Map.SelectionPanel.Rect
	if len(rect) != 4 {
		rect = []float64{.66, .1, .32, .65}
	}
	w, h := g.screen()
	return plateBox{x: rect[0] * w, y: rect[1] * h, w: rect[2] * w, h: rect[3] * h}
}

func (s *atlasScene) selectionAdvice(g *Game) (heading, text, portrait string) {
	if s.sel < 0 || s.sel >= len(s.choices) {
		return
	}
	a := s.location(g).Map
	p := a.Points[s.choices[s.sel].Point]
	env := s.previewState(g).Env()
	heading = s.labelOf(g, s.choices[s.sel].Point)
	if heading == "" {
		heading = s.choices[s.sel].Point
	}
	portrait = a.SelectionPanel.Portrait
	for _, advice := range p.Advice {
		if advice.When.Set() {
			ok, err := advice.When.Bool(env)
			if err != nil || !ok {
				continue
			}
		}
		if advice.Heading != "" {
			heading = data.Render(advice.Heading, env)
		}
		text = data.Render(advice.Text, env)
		if advice.Portrait != "" {
			portrait = advice.Portrait
		}
		break
	}
	return
}

func (s *atlasScene) focusPoint(id string) {
	for i, c := range s.choices {
		if c.Point == id {
			s.sel = i
			s.panelHidden = false
			return
		}
	}
}

func (s *atlasScene) panelButtons(g *Game) [2]plateBox {
	b := s.selectionPanelBox(g)
	width := (b.w - 48) / 2
	return [2]plateBox{{x: b.x + 16, y: b.y + b.h - 54, w: width, h: 38}, {x: b.x + 32 + width, y: b.y + b.h - 54, w: width, h: 38}}
}

func inPlate(b plateBox, x, y float64) bool {
	return x >= b.x && x <= b.x+b.w && y >= b.y && y <= b.y+b.h
}

func (s *atlasScene) updateSelectionPanel(g *Game) bool {
	if s.location(g).Map.SelectionPanel == nil {
		return false
	}
	in := &g.in
	x, y := float64(in.mouse.X), float64(in.mouse.Y)
	if s.panelVisible(g) {
		if in.cancel() {
			s.panelHidden = true
			return true
		}
		if inPlate(s.selectionPanelBox(g), x, y) {
			if in.click {
				buttons := s.panelButtons(g)
				switch {
				case inPlate(buttons[0], x, y):
					s.goTo(g, s.choices[s.sel])
				case inPlate(buttons[1], x, y):
					s.panelHidden = true
				}
				return true
			}
			// The fixed panel obscures map targets under it.
			if in.moved {
				return false
			}
		} else if in.moved {
			if c, ok := s.pointAt(g, x, y); ok && c.Point != s.choices[s.sel].Point {
				s.focusPoint(c.Point)
			}
		}
	} else if in.moved {
		if c, ok := s.pointAt(g, x, y); ok {
			s.focusPoint(c.Point)
		}
	}
	if in.pressed[btnNext] && s.location(g).Map.Camera == nil && len(s.choices) > 0 {
		s.sel = (s.sel + 1) % len(s.choices)
		s.panelHidden = false
		return true
	}
	// Confirm after Back first shows the selected advice again.
	if s.panelHidden && in.ok() {
		s.panelHidden = false
		return true
	}
	return false
}

func (s *atlasScene) drawSelectionPanel(g *Game, dst *ebiten.Image) {
	if !s.panelVisible(g) {
		return
	}
	b := s.selectionPanelBox(g)
	gfx.FillRoundRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), 12, withAlpha(g.th.window, 255))
	gfx.StrokeRoundRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), 12, 2, g.th.border)
	heading, text, portrait := s.selectionAdvice(g)
	x, y, w := b.x+18, b.y+18, b.w-36
	face := math.Min(96, math.Min(w*.3, b.h*.25))
	if portrait != "" {
		g.drawFace(dst, portrait, x, y, face, 1, false)
		g.drawLinesFit(dst, heading, x+face+12, y, w-face-12, sizeM, 2, g.th.text)
		y += face + 16
	} else {
		g.drawLinesFit(dst, heading, x, y, w, sizeM, 2, g.th.text)
		y += gfx.LineHeight(sizeM)*2 + 12
	}
	lines := max(1, int((b.y+b.h-70-y)/gfx.LineHeight(sizeS)))
	g.drawLinesFit(dst, text, x, y, w, sizeS, lines, g.th.text)
	labels := [2]string{lang.T("決定"), lang.T("もどる")}
	for i, button := range s.panelButtons(g) {
		gfx.FillRoundRect(dst, float32(button.x), float32(button.y), float32(button.w), float32(button.h), 8, withAlpha(g.th.accent, 90))
		gfx.DrawText(dst, labels[i], button.x+button.w/2, button.y+7, sizeS*.8, g.th.text)
	}
}
