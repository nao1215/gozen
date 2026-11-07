package play

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

func planningMarkers(p *data.Planning, v data.PlanningView, start, end time.Time) []data.TimelineMarker {
	markers := slices.Clone(v.Markers)
	// Unnamed reservations contribute counts in calendarCells, not markers.
	if !slices.ContainsFunc(p.Schedule, func(r data.Reservation) bool { return r.Label != "" }) {
		return markers
	}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format(time.DateOnly)
		closed := slices.Contains(p.Calendar.WeekdaysOff, int(d.Weekday())) || slices.Contains(p.Calendar.Closed, date)
		for _, r := range p.Schedule {
			if r.Label == "" || !reservationVisible(r, d, closed) {
				continue
			}
			markers = append(markers, data.TimelineMarker{Date: date, Label: r.Label})
		}
	}
	return markers
}

// calendarCells shares hit regions with drawing, including six-week months.
type calendarCell struct {
	date   string
	bounds image.Rectangle
	closed bool
	labels []string
}

func calendarCells(p *data.Planning, v data.PlanningView, month string, x, y, w, h float64) []calendarCell {
	start, err := time.Parse(time.DateOnly, month)
	if err != nil {
		return nil
	}
	start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	cw, ch := (w-40)/7, (h-30)/6
	var cells []calendarCell
	markers := planningMarkers(p, v, start, start.AddDate(0, 1, -1))
	for d := start; d.Month() == start.Month(); d = d.AddDate(0, 0, 1) {
		n := (int(start.Weekday())-v.WeekStart+7)%7 + d.Day() - 1
		xx, yy := x+20+float64(n%7)*cw, y+30+float64(n/7)*ch
		date := d.Format(time.DateOnly)
		cell := calendarCell{date: date, bounds: image.Rect(int(xx), int(yy), int(xx+cw-6), int(yy+ch-2)), closed: slices.Contains(p.Calendar.WeekdaysOff, int(d.Weekday())) || slices.Contains(p.Calendar.Closed, date)}
		for _, marker := range markers {
			if marker.Date == date && marker.Label != "" && !slices.Contains(cell.labels, marker.Label) {
				cell.labels = append(cell.labels, marker.Label)
			}
		}
		// Authored markers supply player-facing names. Preserve the old count
		// for games that have not authored names; never expose internal event IDs.
		if len(cell.labels) == 0 {
			count := 0
			for _, r := range p.Schedule {
				if !reservationVisible(r, d, cell.closed) {
					continue
				}
				count++
			}
			if count > 0 {
				cell.labels = append(cell.labels, strconv.Itoa(count)+" "+planningEventLabel(v))
			}
		}
		cells = append(cells, cell)
	}
	return cells
}

func (s *authorScreen) calendarDate(g *Game) string {
	if s.calendarMonth != "" {
		return s.calendarMonth
	}
	return g.st.Planning.Date
}

func calendarButtons(w float64) []image.Rectangle {
	return []image.Rectangle{image.Rect(int(w)-180, 58, int(w)-108, 100), image.Rect(int(w)-92, 58, int(w)-20, 100)}
}

func (s *authorScreen) updateCalendar(g *Game) bool {
	delta := 0
	if g.in.left() {
		delta = -1
	} else if g.in.right() {
		delta = 1
	}
	w, _ := g.screen()
	if g.in.inspect(g.in.click) {
		for i, r := range calendarButtons(w) {
			if g.in.mouse.In(r) {
				g.in.query(true)
				delta = i*2 - 1
			}
		}
	}
	if delta == 0 {
		if g.in.inspect(g.in.click) {
			_, h := g.screen()
			v := g.db.Game.Screens[s.name].View
			for _, cell := range calendarCells(g.db.Game.Planning, v, s.calendarDate(g), 0, 108, w, h-226) {
				if g.in.mouse.In(cell.bounds) {
					return g.in.query(s.openCalendarDate(g, cell.date))
				}
			}
		}
		return false
	}
	d, err := time.Parse(time.DateOnly, s.calendarDate(g))
	if err != nil {
		return true
	}
	d = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, delta, 0)
	p := g.db.Game.Planning
	month := d.Format("2006-01")
	if month >= p.Calendar.Start[:7] && month <= p.Calendar.End[:7] {
		s.calendarMonth = d.Format(time.DateOnly)
	}
	return true
}

func (s *authorScreen) drawCalendar(g *Game, dst *ebiten.Image, x, y, w, h float64) {
	v := g.db.Game.Screens[s.name].View
	month := s.calendarDate(g)
	if len(month) < 7 {
		return
	}
	if v.Fullscreen {
		g.text(dst, month[:7], x+24, 66, 22, g.th.text)
		for i, r := range calendarButtons(w) {
			g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
			g.text(dst, []string{"<", ">"}[i], float64(r.Min.X+28), float64(r.Min.Y+9), 22, g.th.text)
		}
	}
	weekdays := v.Weekdays
	if len(weekdays) != 7 {
		weekdays = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	}
	for col := range 7 {
		g.text(dst, weekdays[(col+v.WeekStart)%7], x+20+float64(col)*(w-40)/7, y, 16, g.th.dim)
	}
	for _, cell := range calendarCells(g.db.Game.Planning, v, month, x, y, w, h) {
		r := cell.bounds
		xx, yy := float64(r.Min.X), float64(r.Min.Y)
		if cell.closed {
			closed := color.NRGBA{77, 46, 50, 255}
			if g.hasMaterial() {
				// On glass a day off is a light veil, not an opaque block.
				closed = withAlpha(g.th.dim, 46)
			}
			rect(dst, xx, yy, float64(r.Dx()), float64(r.Dy()), closed)
		}
		tint := g.th.text
		if cell.date == g.st.Planning.Date {
			tint = g.th.accent
			rect(dst, xx, yy, 3, float64(r.Dy()), tint)
		}
		gfx.StrokeLine(dst, float32(xx), float32(yy), float32(r.Max.X), float32(yy), 1, tint, true)
		g.text(dst, strings.TrimLeft(cell.date[8:], "0"), xx+6, yy+2, 18, tint)
		limit := min(3, max(0, (r.Dy()-24)/16))
		for i := range min(limit, len(cell.labels)) {
			label := g.db.Tr(cell.labels[i])
			if i == limit-1 && len(cell.labels) > limit {
				label = fmt.Sprintf("+%d", len(cell.labels)-i)
			}
			g.drawPlanningText(dst, label, xx+6, yy+24+float64(i)*16, float64(r.Dx()-12), 15, 1, g.th.text)
		}
	}
}

func (s *authorScreen) calendarDescription(g *Game) string {
	w, h := g.screen()
	v := g.db.Game.Screens[s.name].View
	for _, cell := range calendarCells(g.db.Game.Planning, v, s.calendarDate(g), 0, 108, w, h-226) {
		if g.in.mouse.In(cell.bounds) {
			labels := []string{planningDate(cell.date, "", v)}
			for _, label := range cell.labels {
				labels = append(labels, g.db.Tr(label))
			}
			return strings.Join(labels, " · ")
		}
	}
	return ""
}

// Date links open authored screens without advancing or rewriting planning time.
func (s *authorScreen) openCalendarDate(g *Game, date string) bool {
	if date == "" {
		return false
	}
	for _, marker := range g.db.Game.Screens[s.name].View.Markers {
		if marker.Date == date && marker.Screen != "" {
			args, err := g.db.BindScreen(data.ScreenCall{Screen: marker.Screen, Args: marker.Args}, s.env(g))
			if err != nil {
				g.events.Log(err)
				return true
			}
			g.openAuthorScreen(marker.Screen, nil, args)
			return true
		}
	}
	v := g.db.Game.Screens[s.name].View
	p := g.db.Game.Planning
	d, err := time.Parse(time.DateOnly, date)
	if v.DateLink == nil || err != nil || date < p.Calendar.Start || date > p.Calendar.End {
		return false
	}
	start, err := time.Parse(time.DateOnly, p.Calendar.Start)
	if err != nil {
		return false
	}
	labels := []string{}
	for _, marker := range planningMarkers(p, v, d, d) {
		if marker.Date == date && marker.Label != "" {
			labels = append(labels, marker.Label)
		}
	}
	selected := expr.Map{
		"date": date, "offset": planningDays(start, d), "weekday": float64(d.Weekday()),
		"closed":  slices.Contains(p.Calendar.WeekdaysOff, int(d.Weekday())) || slices.Contains(p.Calendar.Closed, date),
		"summary": strings.Join(labels, " / "),
	}
	args, err := g.db.BindScreen(*v.DateLink, expr.Chain{expr.Map{"selected": selected}, s.env(g)})
	if err != nil {
		g.events.Log(err)
		return true
	}
	g.openAuthorScreen(v.DateLink.Screen, nil, args)
	return true
}

func (s *authorScreen) timelinePointerDate(g *Game) string {
	w, _ := g.screen()
	for _, r := range timelineZoomBounds(0, 108, w) {
		if g.in.mouse.In(r) {
			return ""
		}
	}
	if g.in.mouse.Y < 108 || g.in.mouse.Y >= 164 || g.in.mouse.X < 260 || float64(g.in.mouse.X) >= w-20 {
		return ""
	}
	start, end, err := s.timelineRange(g)
	if err != nil {
		return ""
	}
	days := planningDays(start, end) + 1
	return start.AddDate(0, 0, int((float64(g.in.mouse.X)-260)/(w-280)*days)).Format(time.DateOnly)
}

// reservationVisible is shared by named annotations and fallback counts only;
// hidden closed-day annotations do not change event dispatch.
func reservationVisible(r data.Reservation, d time.Time, closed bool) bool {
	return (!r.HideClosed || !closed) && (r.Date == "" || r.Date == d.Format(time.DateOnly)) && (len(r.Weekdays) == 0 || slices.Contains(r.Weekdays, int(d.Weekday())))
}
