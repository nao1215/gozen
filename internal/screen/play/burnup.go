package play

import (
	"fmt"
	"image/color"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

type workPoint struct {
	date                     string
	total, planned, observed float64
	position                 float64
	known                    bool
}
type workChart struct {
	state   *rpg.PlanningState
	points  []workPoint
	maximum float64
}

func (s *authorScreen) workChart(g *Game, v *data.BurnupView) workChart {
	// Planning rules replace their state atomically; refresh rebuilds the screen on reload.
	if s.chart.state == g.st.Planning {
		return s.chart
	}
	chart := workChart{state: g.st.Planning, maximum: 1}
	p := g.db.Game.Planning
	start, e1 := time.Parse(time.DateOnly, p.Calendar.Start)
	end, e2 := time.Parse(time.DateOnly, p.Calendar.End)
	if e1 != nil || e2 != nil {
		return chart
	}
	total := 0.0
	for _, id := range slices.Sorted(maps.Keys(g.st.Planning.Activities)) {
		a := g.st.Planning.Activities[id]
		if g.db.Game.Planning.Activities[id].Parent != "" {
			continue
		}
		if !a.Cancelled {
			total += g.st.ActivityEffort(id)
		}
	}
	// Long calendars sample the plan while retaining every recorded observation,
	// today's marker and both endpoints. Positions remain proportional to dates.
	days := int((end.Unix() - start.Unix()) / 86400)
	if days < 0 {
		return chart
	}
	offsets := map[int]bool{0: true, days: true}
	for offset := 0; offset < days; offset += max(1, (days+719)/720) {
		offsets[offset] = true
	}
	for date := range g.st.Planning.WorkHistory {
		if d, err := time.Parse(time.DateOnly, date); err == nil {
			offset := int((d.Unix() - start.Unix()) / 86400)
			if offset >= 0 && offset <= days {
				offsets[offset] = true
			}
		}
	}
	if d, err := time.Parse(time.DateOnly, g.st.Planning.Date); err == nil {
		offset := int((d.Unix() - start.Unix()) / 86400)
		if offset >= 0 && offset <= days {
			offsets[offset] = true
		}
	}
	ordered := make([]int, 0, len(offsets))
	for offset := range offsets {
		ordered = append(ordered, offset)
	}
	slices.Sort(ordered)
	for _, offset := range ordered {
		ds := start.AddDate(0, 0, offset).Format(time.DateOnly)
		point := workPoint{date: ds, total: total, planned: g.st.PlannedWork(ds), position: float64(offset) / float64(max(1, days))}
		if v.Planned.Set() {
			val, err := v.Planned.Eval(expr.Chain{expr.Map{"selected": expr.Map{"date": ds}}, s.env(g)})
			if number, ok := expr.ToNumber(val); err == nil && ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
				point.planned = max(0, number)
			}
		}
		if sample, ok := g.st.Planning.WorkHistory[ds]; ok && ds <= g.st.Planning.Date {
			point.total, point.observed, point.known = sample.Total, sample.Observed, true
		}
		chart.maximum = max(chart.maximum, point.total, point.planned, point.observed)
		chart.points = append(chart.points, point)
	}
	s.chart = chart
	return chart
}

func (s *authorScreen) drawBurnup(g *Game, dst *ebiten.Image, x, y, w, h float64) {
	v := g.db.Game.Screens[s.name].View.Burnup
	chart := s.workChart(g, v)
	if len(chart.points) == 0 {
		return
	}
	blue, teal, grey, today, grid := burnupColors(v.Colors)
	g.surfaceWindow(dst, x, y, w, h, "chart")
	if v.Compact {
		g.drawPlanningText(dst, v.Title, x+12, y+8, w*.28, 16, 1, g.th.text)
		g.drawPlanningText(dst, v.PlannedLabel+" / "+v.Observed+" · "+v.Unit, x+12, y+30, w*.28, 14, 1, g.th.dim)
		left, top, pw, ph := x+w*.32, y+10, w*.65, h-20
		for i := 1; i < len(chart.points); i++ {
			a, b := chart.points[i-1], chart.points[i]
			line := func(av, bv float64, tint color.NRGBA) {
				gfx.StrokeLine(dst, float32(left+pw*a.position), float32(top+ph*(1-av/chart.maximum)), float32(left+pw*b.position), float32(top+ph*(1-bv/chart.maximum)), 2, tint, true)
			}
			line(a.planned, b.planned, blue)
			if a.known && b.known {
				line(a.observed, b.observed, teal)
			}
		}
		return
	}
	g.drawPlanningText(dst, v.Title, x+14, y+7, w*.36, 18, 1, g.th.text)
	for i, item := range []struct {
		label string
		tint  color.NRGBA
	}{{v.Total, grey}, {v.PlannedLabel, blue}, {v.Observed, teal}} {
		xx := x + w*.38 + float64(i)*w*.20
		rect(dst, xx, y+18, 18, 3, item.tint)
		g.drawPlanningText(dst, item.label, xx+24, y+7, w*.19-24, 14, 1, g.th.text)
	}
	left, top, pw, ph := x+48, y+65, w-70, h-93
	px := func(i int) float64 { return left + pw*chart.points[i].position }
	py := func(value float64) float64 { return top + ph*(1-max(0, min(1, value/chart.maximum))) }
	line := func(x1, y1, x2, y2, width float64, c color.NRGBA) {
		gfx.StrokeLine(dst, float32(x1), float32(y1), float32(x2), float32(y2), float32(width), c, true)
	}
	for _, value := range []float64{0, chart.maximum / 4, chart.maximum / 2, chart.maximum * 3 / 4, chart.maximum} {
		line(left, py(value), left+pw, py(value), 1, grid)
		g.text(dst, fmt.Sprintf("%.0f", value), x+8, py(value)-8, 14, g.th.dim)
	}
	for i, p := range chart.points {
		if i > 0 {
			prev := chart.points[i-1]
			if i%2 == 0 {
				line(px(i-1), py(prev.total), px(i), py(p.total), 1.5, grey)
			}
			line(px(i-1), py(prev.planned), px(i), py(p.planned), 2, blue)
			if prev.known && p.known {
				line(px(i-1), py(prev.observed), px(i), py(p.observed), 3, teal)
			}
		}
		if p.date == g.st.Planning.Date {
			observed := "—"
			if p.known {
				observed = fmt.Sprintf("%.0f%s", p.observed, v.Unit)
			}
			g.text(dst, fmt.Sprintf("%s  %s %.0f%s / %s %s", v.Today, v.PlannedLabel, p.planned, v.Unit, v.Observed, observed), x+14, y+33, 15, g.th.text)
			line(px(i), top-3, px(i), top+ph, 1.5, today)
			g.text(dst, v.Today, min(left+pw-70, max(left, px(i)+5)), top+ph/2-10, 14, g.th.dim)
		}
		if p.known {
			gfx.FillCircle(dst, float32(px(i)), float32(py(p.observed)), 2.5, teal, true)
		}
	}
	start, startErr := time.Parse(time.DateOnly, chart.points[0].date)
	end, endErr := time.Parse(time.DateOnly, chart.points[len(chart.points)-1].date)
	if startErr != nil || endErr != nil {
		return
	}
	g.text(dst, start.Format("1/2"), left, y+h-23, 14, g.th.dim)
	deadline := v.Deadline + " " + end.Format("1/2")
	g.text(dst, deadline, left+pw-gfx.TextWidth(deadline, 14), y+h-23, 14, g.th.text)
}

// burnupColors resolves view.burnup.colors over the built-in chart colors.
func burnupColors(c *data.BurnupColors) (planned, observed, total, today, grid color.NRGBA) {
	planned, observed, total = color.NRGBA{55, 112, 190, 255}, color.NRGBA{15, 128, 111, 255}, color.NRGBA{115, 124, 134, 255}
	today, grid = color.NRGBA{180, 98, 46, 255}, color.NRGBA{225, 229, 232, 255}
	if c != nil {
		planned, observed, total = c.Planned.Or(planned), c.Observed.Or(observed), c.Total.Or(total)
		today, grid = c.Today.Or(today), c.Grid.Or(grid)
	}
	return
}
