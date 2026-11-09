package play

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
)

// islandCells shares card geometry between rendering and input targets.
func islandCells(x, y, w, h float64, count int) []image.Rectangle {
	cols := 4
	if count > 8 {
		cols = 5
	}
	return planningCells(x, y, w, h, count, cols)
}

// residentGauge reads only the supplied public values. Missing or zero limits
// disable the bar; authored text can explain why a resident has no allowance.
func residentGauge(values map[string]any, f data.PlanningField) (value, limit float64, level int) {
	value, _ = expr.ToNumber(values[f.Key])
	limit = f.Maximum
	if f.MaximumKey != "" {
		limit, _ = expr.ToNumber(values[f.MaximumKey])
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		value = 0
	}
	if math.IsNaN(limit) || math.IsInf(limit, 0) || limit <= 0 {
		return value, 0, 0
	}
	if warning, ok := expr.ToNumber(values[f.WarningKey]); ok && value >= warning {
		level = 1
	}
	if f.CriticalRatio > 0 && value >= limit*f.CriticalRatio {
		level = 2
	}
	return
}

func residentFaceRect(r image.Rectangle, fields int) (x, y, radius float64) {
	bottom := float64(r.Max.Y) - 28 - float64(fields)*23
	top := float64(r.Min.Y) + 2
	radius = max(0, min(56, (bottom-top-8)/2))
	return float64(r.Min.X+r.Max.X) / 2, (top + bottom) / 2, radius
}

func (g *Game) drawResidentCard(dst *ebiten.Image, r image.Rectangle, name, portrait, badge string, values map[string]any, fields []data.PlanningField) {
	original := g.th.window
	defer func() { g.th.window = original }()
	r, g.th.window = g.controlMaterial(r)
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	g.surfaceWindow(dst, x, y, w, h, "card")
	cx, cy, radius := residentFaceRect(r, len(fields))
	g.planningFace(dst, portrait, cx, cy, radius)
	if badge != "" {
		g.drawPlanningText(dst, badge, x+10, y+6, w-20, 14, 1, g.th.accent)
	}
	yy := y + h - 28 - float64(len(fields))*23
	g.drawPlanningText(dst, name, x+12, yy, w-16, 19, 1, g.th.text)
	for j, f := range fields {
		value, limit, level := residentGauge(values, f)
		tint := g.th.accent
		if level == 1 {
			tint = g.th.accent
		}
		if level == 2 {
			tint = g.th.cursor
		}
		label := fmt.Sprint(values[f.Key])
		if f.TextKey != "" {
			label = fmt.Sprint(values[f.TextKey])
		}
		yy = y + h - 30 - float64(len(fields)-1-j)*23
		labelColor := g.th.dim
		if level > 0 {
			labelColor = tint
		}
		g.drawPlanningText(dst, f.Label+" "+label, x+12, yy, w-24, 14, 1, labelColor)
		if limit > 0 {
			rect(dst, x+12, yy+21, w-24, 4, g.th.shadow)
			rect(dst, x+12, yy+21, (w-24)*max(0, min(1, value/limit)), 4, tint)
			if warning, ok := expr.ToNumber(values[f.WarningKey]); ok && warning > 0 && warning < limit {
				rect(dst, x+12+(w-24)*warning/limit, yy+19, 1, 8, g.th.dim)
			}
		}
	}
}
