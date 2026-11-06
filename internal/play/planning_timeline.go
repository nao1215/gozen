package play

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// timelineGroups preserves authored order and never drops unlisted activities.
func timelineGroups(p *data.Planning, v data.PlanningView) []data.TimelineRow {
	rows := slices.Clone(v.Rows)
	seen := map[string]bool{}
	for _, row := range rows {
		for _, item := range row.Items {
			seen[item.Activity] = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(p.Activities)) {
		if !seen[id] && p.Activities[id].Parent == "" {
			rows = append(rows, data.TimelineRow{Label: p.Activities[id].Name, Items: []data.TimelineItem{{Activity: id}}})
		}
	}
	return rows
}

type timelineLane struct {
	text                            string
	labelBounds                     image.Rectangle
	item                            data.TimelineItem
	bounds                          image.Rectangle
	left, right, reported, expected float64
	sample                          float64
	sampleLabel                     string
	sampleFreshness                 float64
	forecast                        float64
	forecastLabel                   string
	dimmed                          bool
}

type timelineResident struct {
	id, portrait string
}

type timelineGroup struct {
	key        string
	expanded   bool
	label      string
	text       string
	y, height  float64
	laneHeight float64
	lanes      []timelineLane
	residents  []timelineResident
}

func timelineLaneHeight(items []data.TimelineItem) float64 {
	for _, item := range items {
		if item.Text != "" {
			return 64
		}
	}
	return 44
}

// timelineRange is presentation-only: zoom shows three weeks around today,
// shifted at the authored boundaries so short calendars never gain extra days.
func (s *authorScreen) timelineRange(g *Game) (time.Time, time.Time, error) {
	p := g.db.Game.Planning
	start, err := time.Parse(time.DateOnly, p.Calendar.Start)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := time.Parse(time.DateOnly, p.Calendar.End)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !s.timelineZoom || planningDays(start, end) < 21 {
		return start, end, nil
	}
	today, err := time.Parse(time.DateOnly, g.st.Planning.Date)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	from := today.AddDate(0, 0, -10)
	if from.Before(start) {
		from = start
	}
	to := from.AddDate(0, 0, 20)
	if to.After(end) {
		to = end
		from = end.AddDate(0, 0, -20)
	}
	return from, to, nil
}

// All geometry is logical, shared by drawing, pointer input and keyboard focus.
func (s *authorScreen) groupedTimeline(g *Game, x, y, w, h float64) ([]timelineGroup, float64, float64) {
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	rows := timelineGroups(p, v)
	visible := g.visibleResidents(v)
	s.timelineAssignees(g)
	axisX, axisW := x+260, max(1, w-280)
	calendarStart, err := time.Parse(time.DateOnly, p.Calendar.Start)
	if err != nil {
		return nil, axisX, axisW
	}
	calendarEnd, err := time.Parse(time.DateOnly, p.Calendar.End)
	if err != nil {
		return nil, axisX, axisW
	}
	start, end, err := s.timelineRange(g)
	if err != nil {
		return nil, axisX, axisW
	}
	today, err := time.Parse(time.DateOnly, g.st.Planning.Date)
	if err != nil {
		return nil, axisX, axisW
	}
	days := planningDays(start, end) + 1
	rowHeight := func(row data.TimelineRow) float64 {
		if len(row.Items) > 0 && row.Items[0].Activity == s.timelineExpanded {
			return 40 + float64(len(row.Items))*timelineLaneHeight(row.Items)
		}
		return max(36, float64(len(row.Items))*10+4)
	}
	y += 56
	h -= 56
	// Keep readable minimums: larger authored tables scroll whole groups.
	s.timelineTop = min(s.timelineTop, max(0, len(rows)-1))
	var out []timelineGroup
	bottom := y + h
	env := g.st.Env()
	for _, row := range rows[s.timelineTop:] {
		rh := rowHeight(row)
		if y+rh > bottom {
			break
		}
		group := timelineGroup{label: row.Label, text: data.Render(g.db.Tr(row.Text), env), y: y, height: rh, laneHeight: timelineLaneHeight(row.Items)}
		if len(row.Items) > 0 {
			group.key = row.Items[0].Activity
			group.expanded = group.key == s.timelineExpanded
		}
		for j, item := range row.Items {
			d := p.Activities[item.Activity]
			a := g.st.Planning.Activities[item.Activity]
			if v.Portrait != "" && !a.Cancelled && slices.Contains(visible, a.Assignee) && !slices.ContainsFunc(group.residents, func(r timelineResident) bool { return r.id == a.Assignee }) {
				portrait, ok := g.st.EntityValues(a.Assignee, false)[v.Portrait].(string)
				if !ok {
					portrait = ""
				}
				group.residents = append(group.residents, timelineResident{id: a.Assignee, portrait: portrait})
			}
			from, to := calendarStart, calendarEnd
			if d.From != "" {
				date, err := time.Parse(time.DateOnly, d.From)
				if err == nil {
					from = date
				}
			}
			if deadline := g.st.ActivityDeadline(item.Activity); deadline != "" {
				date, err := time.Parse(time.DateOnly, deadline)
				if err == nil {
					to = date
				}
			}
			left := planningDays(start, from) / days
			right := max(left, (planningDays(start, to)+1)/days)
			reported := max(0, min(1, a.Observed/g.st.ActivityEffort(item.Activity)))
			expected := max(0, min(1, planningDays(from, today)/max(1, planningDays(from, to)+1)))
			sample := -1.0
			if value, err := item.Sample.Eval(env); err == nil {
				if number, ok := value.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 && number <= 1 {
					sample = number
				}
			}
			freshness := 1.0
			if item.SampleFreshness.Expr != nil {
				freshness = 0
				if value, err := item.SampleFreshness.Eval(env); err == nil {
					if number, ok := value.(float64); ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
						freshness = max(0, min(1, number))
					}
				}
			}
			forecast, forecastLabel := timelineForecast(from, today, calendarStart, calendarEnd, reported, a.Cancelled, data.Render(g.db.Tr(item.ForecastLabel), env))
			if forecast >= 0 {
				forecast = (planningDays(start, calendarStart) + forecast*(planningDays(calendarStart, calendarEnd)+1)) / days
			}
			ly := y + 3 + float64(j)*10
			if group.expanded {
				ly = y + 52 + float64(j)*group.laneHeight
			}
			labelBounds := image.Rectangle{}
			if group.expanded {
				labelBounds = image.Rect(int(x+24), int(ly-12), int(x+248), int(ly-12+group.laneHeight))
			}
			group.lanes = append(group.lanes, timelineLane{text: data.Render(g.db.Tr(item.Text), env), labelBounds: labelBounds, item: item, bounds: image.Rect(int(axisX+axisW*max(0, min(1, left))), int(ly), int(math.Ceil(axisX+axisW*max(0, min(1, right)))), int(ly+9)), left: left, right: right, reported: reported, expected: expected, forecast: forecast, forecastLabel: forecastLabel, dimmed: s.timelineAssignee != "" && a.Assignee != s.timelineAssignee, sample: sample, sampleFreshness: freshness, sampleLabel: data.Render(g.db.Tr(item.SampleLabel), env)})
		}
		out = append(out, group)
		y += rh
	}
	return out, axisX, axisW
}

// Blend between readable theme colors, never fade the historical mark away.
func timelineSampleTint(fresh, old color.NRGBA, freshness float64) color.NRGBA {
	mix := func(a, b uint8) uint8 { return uint8(math.Round(float64(b) + (float64(a)-float64(b))*freshness)) }
	return color.NRGBA{mix(fresh.R, old.R), mix(fresh.G, old.G), mix(fresh.B, old.B), mix(fresh.A, old.A)}
}

// timelineForecast uses elapsed calendar days and public reported work only.
// Zero reports, future starts and finished/excluded work have no defensible rate.
// The endpoint is clipped before date conversion, even for tiny positive rates.
func timelineForecast(from, today, start, end time.Time, reported float64, cancelled bool, label string) (float64, string) {
	elapsed := planningDays(from, today)
	if label == "" || cancelled || elapsed <= 0 || reported <= 0 || reported >= 1 || math.IsNaN(reported) || math.IsInf(reported, 0) {
		return -1, ""
	}
	finish := planningDays(start, from) + math.Ceil(elapsed/reported)
	days := planningDays(start, end) + 1
	if finish > days {
		return 1, label + " > " + end.Format(time.DateOnly)
	}
	if finish <= 0 {
		return -1, ""
	}
	return finish / days, label + " " + start.AddDate(0, 0, int(finish)-1).Format(time.DateOnly)
}

func (s *authorScreen) drawGroupedTimeline(g *Game, dst *ebiten.Image, x, y, w, h float64) {
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	groups, ax, aw := s.groupedTimeline(g, x, y, w, h)
	tc := timelinePalette(v.Light, g.th.text, g.th.dim, g.th.accent)
	if v.Light {
		// A frosted board behind the whole chart keeps it readable on the
		// wallpaper of a light surface.
		bottom := y + 56
		for _, group := range groups {
			bottom = group.y + group.height
		}
		g.drawPlate(dst, image.Rect(int(x+4), int(y-8), int(x+w-4), int(bottom+8)))
	}
	s.drawTimelineFilter(g, dst, x, y)
	start, end, err := s.timelineRange(g)
	if err != nil {
		return
	}
	days := planningDays(start, end) + 1
	dx := aw / days
	today, err := time.Parse(time.DateOnly, g.st.Planning.Date)
	if err != nil {
		return
	}
	bottom := y + 56
	for _, group := range groups {
		bottom = group.y + group.height
	}
	muted := tc.line
	red := tc.today
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		xx := ax + planningDays(start, d)*dx
		if slices.Contains(p.Calendar.WeekdaysOff, int(d.Weekday())) || slices.Contains(p.Calendar.Closed, d.Format(time.DateOnly)) {
			rect(dst, xx, y+23, dx, bottom-y-23, tc.holiday)
		}
		if d.Day() == 1 || d.Equal(start) {
			gfx.StrokeLine(dst, float32(xx), float32(y+20), float32(xx), float32(bottom), 1, muted, true)
		}
	}
	labelRight := ax
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		xx := ax + planningDays(start, d)*dx
		if d.Day() == 1 || d.Equal(start) {
			if xx+90 < x+w-100 {
				g.text(dst, d.Format("2006/1"), xx+2, y, 15, g.th.text)
			}
		}
		if (d.Weekday() == time.Monday || (s.timelineZoom && dx >= 40)) && xx+2 >= labelRight {
			g.text(dst, d.Format("2"), xx+2, y+23, 15, g.th.dim)
			labelRight = xx + 2 + gfx.TextWidth(d.Format("2"), 15) + 6
		}
	}
	// Annotation text follows the pointer's date in the footer; compact ticks
	// keep dense holidays from overwriting adjacent labels on the shared axis.
	for _, marker := range planningMarkers(p, v, start, end) {
		if marker.Label == "" && !marker.Milestone {
			continue
		}
		d, err := time.Parse(time.DateOnly, marker.Date)
		if err != nil {
			continue
		}
		if d.Before(start) || d.After(end) {
			continue
		}
		xx := ax + planningDays(start, d)*dx
		rect(dst, xx, y+46, max(2, dx), 5, g.th.accent)
		if marker.Milestone {
			for yy := y + 54; yy < bottom; yy += 8 {
				rect(dst, xx, yy, 1, min(4, bottom-yy), g.th.dim)
			}
		}
	}
	if s.timelineContext == nil {
		for i, r := range timelineZoomBounds(x, y, w) {
			g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
			tint := g.th.dim
			if (i == 0) == s.timelineZoom {
				tint = g.th.accent
			}
			g.text(dst, []string{"+", "−"}[i], float64(r.Min.X+8), float64(r.Min.Y), 18, tint)
		}
	}
	index := 0
	for _, group := range groups {
		nameWidth := 232.0
		if len(group.residents) > 0 {
			nameWidth -= float64(min(3, len(group.residents)))*28 + 8
		}
		indicator := "+ "
		if group.expanded {
			indicator = "− "
		}
		g.drawPlanningText(dst, indicator+group.label, x+16, group.y, nameWidth, 16, 1, g.th.text)
		for i, resident := range group.residents[:min(3, len(group.residents))] {
			cx := x + 248 - float64(min(3, len(group.residents))-i)*28 + 14
			if i == 2 && len(group.residents) > 3 {
				g.text(dst, fmt.Sprintf("+%d", len(group.residents)-2), cx-12, group.y, 15, g.th.text)
				continue
			}
			g.planningFace(dst, resident.portrait, cx, group.y+12, 12)
			if resident.id == s.timelineAssignee {
				gfx.StrokeCircle(dst, float32(cx), float32(group.y+12), 13, 1, g.th.accent, true)
			}
		}
		var labels []string
		for j, lane := range group.lanes {
			labels = append(labels, lane.item.Label)
			r := lane.bounds
			lx, ly, lw, lh := ax+aw*lane.left, float64(r.Min.Y), aw*(lane.right-lane.left), float64(r.Dy())
			a := g.st.Planning.Activities[lane.item.Activity]
			if group.expanded {
				top := group.y + 40 + float64(j)*group.laneHeight
				title, summary := timelineLaneSummary(g, lane)
				g.drawPlanningText(dst, title, x+24, top, 224, 16, 1, g.th.text)
				g.drawPlanningText(dst, summary, x+24, top+20, 224, 15, 1, g.th.dim)
				if lane.text != "" {
					g.drawPlanningText(dst, lane.text, x+24, top+40, 224, 15, 1, g.th.text)
				}
			}
			k := gfx.Scale()
			bars, ok := dst.SubImage(image.Rect(int(ax*k), int((ly-1)*k), int((ax+aw)*k), int((ly+lh+2)*k))).(*ebiten.Image)
			if !ok {
				continue
			}
			border := tc.border
			if index == s.timelineFocus {
				border = tc.focus
			}
			rect(bars, lx, ly, lw, lh, border)
			if index == s.timelineFocus && v.Light {
				rect(bars, lx-1, ly-1, lw+2, lh+2, border)
			}
			rect(bars, lx+1, ly+1, max(0, lw-2), lh-2, tc.inside)
			if a.Cancelled {
				for xx := max(ax, lx); xx < min(ax+aw, lx+lw); xx += 6 {
					gfx.StrokeLine(bars, float32(xx), float32(ly+lh-1), float32(min(xx+4, lx+lw)), float32(ly+1), tc.hatch, tc.cancel, true)
				}
			} else {
				if lane.reported < lane.expected && tc.lateFill {
					// What should have been reported by now and was not.
					rect(bars, lx+1+(lw-2)*lane.reported, ly+1, max(0, (lw-2)*(lane.expected-lane.reported)), lh-2, tc.late)
				}
				rect(bars, lx+1, ly+1, max(0, (lw-2)*lane.reported), lh-2, tc.reported)
				if lane.reported < lane.expected && !tc.lateFill {
					for xx := max(ax, lx+lw*lane.reported); xx < min(ax+aw, lx+lw*lane.expected); xx += 4 {
						gfx.StrokeLine(bars, float32(xx), float32(ly+lh-1), float32(min(xx+3, lx+lw*lane.expected)), float32(ly+1), 1, red, true)
					}
				}
				if lane.expected > 0 && lane.expected < 1 {
					rect(bars, lx+lw*lane.expected, ly-1, 1, lh+2, g.th.text)
				}
			}
			if lane.forecast >= 0 {
				// Dashes distinguish the extrapolation from the solid planned bar.
				fx := ax + aw*min(1, lane.forecast)
				tx := ax + planningDays(start, today)*dx
				for xx := max(ax, tx); xx < fx; xx += 6 {
					rect(bars, xx, ly+lh/2, min(3, fx-xx), 1, g.th.dim)
				}
				rect(bars, min(ax+aw-1, fx), ly, 1, lh, g.th.dim)
			}
			if lane.sample >= 0 && lw >= 5 {
				// A small downward wedge remains inside its lane at both endpoints.
				sx := lx + 2 + (lw-5)*lane.sample
				tint := timelineSampleTint(g.th.text, g.th.dim, lane.sampleFreshness)
				for step := range 3 {
					rect(bars, sx-float64(2-step), ly+float64(step), float64(5-2*step), 1, tint)
				}
			}
			if lane.dimmed {
				// Preserve the whole calendar and row positions; de-emphasize only
				// this lane, including its forecast, without hiding assignments.
				rect(bars, ax, ly, aw, lh, tc.veil)
			}
			index++
		}
		summary := strings.Join(labels, " / ")
		if group.text != "" {
			summary = group.text
		}
		g.drawPlanningText(dst, summary, x+16, group.y+18, nameWidth, 15, 1, g.th.dim)
		rect(dst, x+16, group.y+group.height-1, w-32, 1, muted)
	}
	if !today.Before(start) && !today.After(end) {
		tx := ax + planningDays(start, today)*dx
		rect(dst, tx, y+41, 2, max(0, bottom-y-41), red)
		if v.Light {
			label := lang.T("今日")
			g.text(dst, label, tx-gfx.TextWidth(label, 13)/2, y+44, 13, red)
		}
	}
}

// timelineColors are the colors of a grouped timeline: the dark ones of a
// plain surface, or the light ones of a view.light surface, where the planned
// span is filled pale blue, the report deep blue and the shortfall orange.
type timelineColors struct {
	holiday, line, inside, reported, late, cancel, veil, border, focus, today color.NRGBA
	hatch                                                                     float32
	lateFill                                                                  bool
}

func timelinePalette(light bool, text, dim, accent color.NRGBA) timelineColors {
	if !light {
		return timelineColors{
			holiday: color.NRGBA{48, 53, 62, 255}, line: color.NRGBA{62, 72, 85, 255}, inside: color.NRGBA{30, 34, 40, 255},
			reported: accent, cancel: color.NRGBA{62, 72, 85, 255}, veil: color.NRGBA{30, 34, 40, 185},
			border: dim, focus: text, today: color.NRGBA{240, 116, 108, 255}, hatch: 1,
		}
	}
	return timelineColors{
		holiday: color.NRGBA{222, 226, 232, 200}, line: color.NRGBA{196, 203, 212, 255}, inside: color.NRGBA{170, 202, 242, 255},
		reported: color.NRGBA{38, 98, 178, 255}, late: color.NRGBA{205, 92, 60, 255}, cancel: color.NRGBA{150, 158, 168, 255},
		veil: color.NRGBA{246, 248, 251, 200}, border: color.NRGBA{112, 146, 190, 255}, focus: accent,
		today: color.NRGBA{214, 64, 58, 255}, hatch: 1.4, lateFill: true,
	}
}

// The selector changes presentation only. A disappearing resident clears the
// filter rather than leaving the player with every lane dimmed.
func (s *authorScreen) timelineAssignees(g *Game) []string {
	v := g.db.Game.Screens[s.name].View
	if v.FilterLabel == "" {
		s.timelineAssignee = ""
		return nil
	}
	ids := append([]string{""}, g.visibleResidents(v)...)
	if !slices.Contains(ids, s.timelineAssignee) {
		s.timelineAssignee = ""
	}
	return ids
}

func timelineFilterBounds(x, y float64) [3]image.Rectangle {
	return [3]image.Rectangle{
		image.Rect(int(x+16), int(y+4), int(x+48), int(y+48)),
		image.Rect(int(x+48), int(y+4), int(x+216), int(y+48)),
		image.Rect(int(x+216), int(y+4), int(x+248), int(y+48)),
	}
}

func (s *authorScreen) drawTimelineFilter(g *Game, dst *ebiten.Image, x, y float64) {
	ids := s.timelineAssignees(g)
	if len(ids) == 0 {
		return
	}
	v := g.db.Game.Screens[s.name].View
	for _, r := range timelineFilterBounds(x, y) {
		g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
	}
	g.text(dst, "‹", x+24, y+16, 18, g.th.text)
	g.text(dst, "›", x+224, y+16, 18, g.th.text)
	label := g.db.Tr(v.FilterLabel)
	if s.timelineAssignee != "" {
		label = g.db.Game.Planning.Entities[s.timelineAssignee].Name
		path, ok := g.st.EntityValues(s.timelineAssignee, false)[v.Portrait].(string)
		if !ok {
			path = ""
		}
		g.planningFace(dst, path, x+68, y+26, 16)
	}
	g.drawPlanningText(dst, label, x+88, y+8, 122, 16, 1, g.th.text)
	g.text(dst, fmt.Sprintf("%d / %d", slices.Index(ids, s.timelineAssignee), len(ids)-1), x+88, y+28, 15, g.th.dim)
}

func (s *authorScreen) updateTimelineFilter(g *Game) bool {
	ids := s.timelineAssignees(g)
	if len(ids) == 0 || !g.in.click {
		return false
	}
	for i, r := range timelineFilterBounds(0, 108) {
		if !g.in.mouse.In(r) {
			continue
		}
		index := slices.Index(ids, s.timelineAssignee)
		switch i {
		case 0:
			index = (index + len(ids) - 1) % len(ids)
		case 1:
			index = 0 // The current portrait/name resets to the complete view.
		case 2:
			index = (index + 1) % len(ids)
		}
		s.timelineAssignee = ids[index]
		return true
	}
	return false
}

func (s *authorScreen) timelineLanes(g *Game) []timelineLane {
	sw, sh := g.screen()
	groups, _, _ := s.groupedTimeline(g, 0, 108, sw, sh-226)
	var lanes []timelineLane
	for _, group := range groups {
		lanes = append(lanes, group.lanes...)
	}
	return lanes
}

func (s *authorScreen) timelineDescription(g *Game) string {
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	sw, _ := g.screen()
	if date := s.timelinePointerDate(g); date != "" {
		labels := []string{planningDate(date, "", v)}
		d, err := time.Parse(time.DateOnly, date)
		if err != nil {
			return ""
		}
		for _, marker := range planningMarkers(p, v, d, d) {
			if marker.Date == date && marker.Label != "" {
				labels = append(labels, marker.Label)
			}
		}
		return strings.Join(labels, " · ")
	}
	if g.in.mouse.X >= 16 && g.in.mouse.X < 248 {
		_, sh := g.screen()
		groups, _, _ := s.groupedTimeline(g, 0, 108, sw, sh-226)
		for _, group := range groups {
			headerHeight := group.height
			if group.expanded {
				headerHeight = 36
			}
			if float64(g.in.mouse.Y) >= group.y && float64(g.in.mouse.Y) < group.y+headerHeight {
				names := []string{group.label}
				for _, resident := range group.residents {
					names = append(names, p.Entities[resident.id].Name)
				}
				return strings.Join(names, " · ") + "\n" + group.text
			}
		}
	}
	lanes := s.timelineLanes(g)
	if len(lanes) == 0 {
		return ""
	}
	s.timelineFocus = max(0, min(s.timelineFocus, len(lanes)-1))
	lane := lanes[s.timelineFocus]
	for _, candidate := range lanes {
		if g.in.mouse.In(candidate.labelBounds) || g.in.mouse.In(candidate.bounds) {
			lane = candidate
			break
		}
	}
	return timelineLaneDescription(g, lane)
}

func timelineLaneDescription(g *Game, lane timelineLane) string {
	p := g.db.Game.Planning
	d := p.Activities[lane.item.Activity]
	a := g.st.Planning.Activities[lane.item.Activity]
	// Only authored saved samples and reported observations enter the label.
	description := fmt.Sprintf("%s · %s · %.0f%%\n%s — %s", d.Name, p.Entities[a.Assignee].Name, 100*lane.reported, d.From, g.st.ActivityDeadline(lane.item.Activity))
	if lane.text != "" {
		description += " · " + lane.text
	}
	if lane.sample >= 0 {
		description += fmt.Sprintf(" · %s %.0f%%", lane.sampleLabel, 100*lane.sample)
	}
	if lane.forecastLabel != "" {
		description += " · " + lane.forecastLabel
	}
	return description
}

// Expanded labels use public reports and assignments, never live hidden work.
func timelineLaneSummary(g *Game, lane timelineLane) (string, string) {
	p := g.db.Game.Planning
	a := g.st.Planning.Activities[lane.item.Activity]
	return p.Activities[lane.item.Activity].Name,
		fmt.Sprintf("%s · %.0f%%", p.Entities[a.Assignee].Name, 100*lane.reported)
}

// A group header toggles its inline lanes. If it is near the footer, reveal it
// at the top rather than clipping details. Collapsing restores the overview.
func (s *authorScreen) toggleTimelineGroup(g *Game) bool {
	if !g.in.click || g.in.mouse.X < 16 || g.in.mouse.X >= 248 {
		return false
	}
	sw, sh := g.screen()
	groups, _, _ := s.groupedTimeline(g, 0, 108, sw, sh-226)
	for i, group := range groups {
		if float64(g.in.mouse.Y) < group.y || float64(g.in.mouse.Y) >= group.y+36 {
			continue
		}
		if group.expanded {
			s.timelineExpanded = ""
			s.timelineTop = 0
		} else {
			// Very large authored groups retain the compact representation.
			if 40+float64(len(group.lanes))*group.laneHeight > sh-282 {
				return true
			}
			s.timelineExpanded = group.key
			if group.y+40+float64(len(group.lanes))*group.laneHeight > sh-118 {
				s.timelineTop += i
			}
		}
		s.timelineFocus = 0
		if s.timelineExpanded != "" {
			visible, _, _ := s.groupedTimeline(g, 0, 108, sw, sh-226)
			for _, row := range visible {
				if row.expanded {
					break
				}
				s.timelineFocus += len(row.lanes)
			}
		}
		return true
	}
	return false
}

func timelineZoomBounds(x, y, w float64) [2]image.Rectangle {
	return [2]image.Rectangle{
		image.Rect(int(x+w-92), int(y), int(x+w-60), int(y+22)),
		image.Rect(int(x+w-56), int(y), int(x+w-24), int(y+22)),
	}
}

func (s *authorScreen) updateGroupedTimeline(g *Game) bool {
	if g.in.pressed[btnMapZoomIn] || g.in.pressed[btnMapZoomOut] {
		s.timelineZoom = g.in.pressed[btnMapZoomIn]
		return true
	}
	if g.in.click {
		w, _ := g.screen()
		for i, r := range timelineZoomBounds(0, 108, w) {
			if g.in.mouse.In(r) {
				s.timelineZoom = i == 0
				return true
			}
		}
	}
	if g.in.click && s.openCalendarDate(g, s.timelinePointerDate(g)) {
		return true
	}
	if s.updateTimelineFilter(g) || s.toggleTimelineGroup(g) {
		return true
	}
	lanes := s.timelineLanes(g)
	if len(lanes) == 0 {
		return false
	}
	open := func(lane timelineLane) {
		if lane.item.Screen != "" {
			args, err := g.db.BindScreen(data.ScreenCall{Screen: lane.item.Screen, Args: lane.item.Args}, s.env(g))
			if err != nil {
				g.events.Log(err)
				return
			}
			g.openAuthorScreen(lane.item.Screen, nil, args)
			if len(g.stack) > 0 {
				if child, ok := g.stack[len(g.stack)-1].(*authorScreen); ok && child != s && child.candidateList(g) {
					child.timelineContext, child.timelineActivity = s, lane.item.Activity
					child.layoutTimelineCandidates(g)
				}
			}
		}
	}
	if g.in.down() {
		if s.timelineFocus+1 < len(lanes) {
			s.timelineFocus++
		} else {
			rows := timelineGroups(g.db.Game.Planning, g.db.Game.Screens[s.name].View)
			sw, sh := g.screen()
			groups, _, _ := s.groupedTimeline(g, 0, 108, sw, sh-226)
			if s.timelineTop+len(groups) < len(rows) {
				s.timelineTop++
				s.timelineFocus = 0
			}
		}
		return true
	}
	if g.in.up() {
		if s.timelineFocus > 0 {
			s.timelineFocus--
		} else if s.timelineTop > 0 {
			s.timelineTop--
			s.timelineFocus = 0
		}
		return true
	}
	if g.in.click {
		for i, lane := range lanes {
			hit := g.in.mouse.In(lane.bounds) || g.in.mouse.In(lane.labelBounds)
			if hit {
				s.timelineFocus = i
				open(lane)
				return true
			}
		}
	}
	if g.in.ok() {
		open(lanes[min(s.timelineFocus, len(lanes)-1)])
		return true
	}
	return false
}
