package play

import (
	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// A full-screen view's header can begin with a face: a small square crop of an
// interpolated picture, with an optional colored ring, so a state such as the
// player's fatigue reads from the face instead of a word.

// drawHeaderFace draws the header face of view v with its left edge at x on a
// band whose middle is cy, right after the date, and returns its right edge (x
// when there is none). The date is short, so the face fits before the header
// text and the step band whatever their length.
func (s *authorScreen) drawHeaderFace(g *Game, dst *ebiten.Image, v data.PlanningView, x, cy, size float64) float64 {
	if v.HeaderFace == "" {
		return x
	}
	env := s.env(g)
	path := data.Render(v.HeaderFace, env)
	if path == "" {
		return x
	}
	r := size / 2
	previous := g.faceSquare
	g.faceSquare = true
	g.planningFace(dst, path, x+r, cy, r)
	g.faceSquare = previous
	if tint, ok := parseDotColor(data.Render(v.HeaderTint, env)); ok {
		gfx.StrokeRoundRect(dst, float32(x-1.5), float32(cy-r-1.5), float32(size+3), float32(size+3), float32(r*.45), 2.5, tint)
	}
	return x + size
}
