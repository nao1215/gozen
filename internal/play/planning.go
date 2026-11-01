package play

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// nativeView returns an inner render target. Ebitengine fits this target in the
// window, supplies the letterbox and transforms pointer coordinates exactly once.
func nativeView(w, h, dsf float64) gameView {
	if w <= 0 || h <= 0 || dsf <= 0 {
		return gameView{}
	}
	k := min(w/1280, h/720) * dsf
	return gameView{w: max(1, int(math.Round(1280*k))), h: max(1, int(math.Round(720*k))), ui: k}
}

func (s *authorScreen) drawPlanning(g *Game, dst *ebiten.Image, kind string, x, y, w, h float64) {
	st, p := g.st.Planning, g.db.Game.Planning
	if st == nil || p == nil {
		return
	}
	v := g.db.Game.Screens[s.name].View
	if !v.Fullscreen {
		g.window(dst, x, y, w, h)
		g.text(dst, planningDate(st.Date, p.Calendar.Phases[st.Phase], v), x+20, y+18, sizeM, g.th.accent)
	}
	y += 60
	h -= 70
	switch kind {
	case "timeline":
		if len(v.Rows) > 0 {
			s.drawGroupedTimeline(g, dst, x, y, w, h)
			return
		}
		start, err := time.Parse(time.DateOnly, p.Calendar.Start)
		if err != nil {
			return
		}
		end, err := time.Parse(time.DateOnly, p.Calendar.End)
		if err != nil {
			return
		}
		for id, a := range p.Activities {
			if d, err := time.Parse(time.DateOnly, a.From); err == nil && d.Before(start) {
				start = d
			}
			if d, err := time.Parse(time.DateOnly, g.st.ActivityDeadline(id)); err == nil && d.After(end) {
				end = d
			}
		}
		days := end.Sub(start).Hours()/24 + 1
		axisX, axisW := x+w*.48, w*.46
		g.text(dst, start.Format("1/2")+" — "+end.Format("1/2"), axisX, y, 15, g.th.dim)
		y += 24
		h -= 24
		ids := slices.DeleteFunc(slices.Sorted(maps.Keys(st.Activities)), func(id string) bool { return st.Activities[id].Cancelled })
		slices.SortFunc(ids, func(a, b string) int {
			if order := strings.Compare(p.Activities[a].From, p.Activities[b].From); order != 0 {
				return order
			}
			return strings.Compare(a, b)
		})
		rows := timelineRows(h)
		s.timelineTop = max(0, min(s.timelineTop, len(ids)-rows))
		last := min(len(ids), s.timelineTop+rows)
		if len(ids) > rows {
			g.text(dst, fmt.Sprintf("%d–%d / %d  ↑ ↓", s.timelineTop+1, last, len(ids)), x+20, y-24, 15, g.th.dim)
		}
		rowH := 56.0
		for i, id := range ids[s.timelineTop:last] {
			a := st.Activities[id]
			d := p.Activities[id]
			yy := y + float64(i)*rowH
			g.drawPlanningText(dst, d.Name, x+20, yy, w*.45, sizeS, 1, g.th.text)
			g.text(dst, p.Entities[a.Assignee].Name, x+20, yy+24, 16, g.th.dim)
			from, to := start, end
			if d.From != "" {
				if date, err := time.Parse(time.DateOnly, d.From); err == nil {
					from = date
				}
			}
			if deadline := g.st.ActivityDeadline(id); deadline != "" {
				if date, err := time.Parse(time.DateOnly, deadline); err == nil {
					to = date
				}
			}
			left := max(0, min(1, from.Sub(start).Hours()/24/days))
			right := max(left, min(1, (to.Sub(start).Hours()/24+1)/days))
			gfx.StrokeLine(dst, float32(axisX), float32(yy+14), float32(axisX+axisW), float32(yy+14), 1, g.th.dim, true)
			if right > left {
				rect(dst, axisX+axisW*left, yy+6, axisW*(right-left), 10, color.NRGBA{160, 190, 235, 255})
				actual := max(0, min(1, a.Observed/g.st.ActivityEffort(id)))
				tint := g.th.accent
				today, err := time.Parse(time.DateOnly, st.Date)
				if err != nil {
					return
				}
				expected := max(0, min(1, (today.Sub(from).Hours()/24)/max(1, to.Sub(from).Hours()/24+1)))
				if actual < expected {
					tint = color.NRGBA{220, 65, 65, 255}
				}
				// The unfilled shortfall remains visible even at zero progress.
				if actual < expected {
					rect(dst, axisX+axisW*(left+(right-left)*actual), yy+18, axisW*(right-left)*(expected-actual), 8, tint)
				}
				rect(dst, axisX+axisW*left, yy+18, axisW*(right-left)*actual, 8, g.th.accent)
			}
			g.text(dst, fmt.Sprintf("%.0f%%", 100*a.Observed/g.st.ActivityEffort(id)), x+w*.28, yy+28, 16, g.th.text)
		}
		today, err := time.Parse(time.DateOnly, st.Date)
		if err != nil {
			return
		}
		tx := axisX + axisW*max(0, min(1, today.Sub(start).Hours()/24/days))
		gfx.StrokeLine(dst, float32(tx), float32(y), float32(tx), float32(y+h), 2, color.NRGBA{220, 65, 65, 255}, true)
	case "calendar":
		start, err := time.Parse(time.DateOnly, st.Date)
		if err != nil {
			return
		}
		start = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		cw, ch := (w-40)/7, (h-30)/6
		weekdays := v.Weekdays
		if len(weekdays) != 7 {
			weekdays = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
		}
		for col, label := range weekdays {
			g.text(dst, label, x+20+float64(col)*cw, y, 16, g.th.dim)
		}
		for d := start; d.Month() == start.Month(); d = d.AddDate(0, 0, 1) {
			n := int(start.Weekday()) + d.Day() - 1
			xx, yy := x+20+float64(n%7)*cw, y+30+float64(n/7)*ch
			clr := g.th.dim
			if d.Format(time.DateOnly) == st.Date {
				clr = g.th.accent
			}
			gfx.StrokeLine(dst, float32(xx), float32(yy), float32(xx+cw-6), float32(yy), 1, clr, true)
			g.text(dst, strconv.Itoa(d.Day()), xx+4, yy+5, 18, clr)
			count := 0
			for _, r := range p.Schedule {
				if r.Date != "" && r.Date != d.Format(time.DateOnly) || len(r.Weekdays) > 0 && !slices.Contains(r.Weekdays, int(d.Weekday())) {
					continue
				}
				count++
			}
			if count > 0 {
				g.text(dst, strconv.Itoa(count)+" "+planningEventLabel(v), xx+4, yy+29, 15, g.th.text)
			}
		}
	case "entities", "ensemble", "islands":
		s.drawResidents(g, dst, kind, x, y, w, h)
	}
}

// planningCells reserves gaps and never lets one tile overlap another.
func planningCells(x, y, w, h float64, count, cols int) []image.Rectangle {
	if count <= 0 || cols <= 0 {
		return nil
	}
	rows := (count + cols - 1) / cols
	cw, ch := w/float64(cols), h/float64(rows)
	out := make([]image.Rectangle, count)
	for i := range out {
		out[i] = image.Rect(int(x+float64(i%cols)*cw+6), int(y+float64(i/cols)*ch+6), int(x+float64(i%cols+1)*cw-6), int(y+float64(i/cols+1)*ch-6))
	}
	return out
}

// visibleResidents uses observations, never hidden rule attributes.
func (g *Game) visibleResidents(v data.PlanningView) []string {
	ids := v.Order
	if len(ids) == 0 {
		ids = slices.Sorted(maps.Keys(g.db.Game.Planning.Entities))
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if v.Visible == "" || g.st.EntityValues(id, false)[v.Visible] == true {
			out = append(out, id)
		}
	}
	return out
}

func (s *authorScreen) drawResidents(g *Game, dst *ebiten.Image, kind string, x, y, w, h float64) {
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	ids := g.visibleResidents(v)
	if s.target != "" && !slices.Contains(ids, s.target) {
		s.target = ""
	}
	cols := 1
	if kind == "ensemble" || kind == "islands" {
		cols = 4
	}
	if kind == "islands" {
		rect(dst, x+12, y, w-24, h, color.NRGBA{216, 210, 196, 255})
		// Wall boards use the same observed state as the detailed views.
		rect(dst, x+24, y+8, w*.46, 86, color.NRGBA{248, 248, 244, 255})
		total, done := 0.0, 0.0
		for id, a := range g.st.Planning.Activities {
			total += g.st.ActivityEffort(id)
			done += a.Observed
		}
		if total > 0 {
			bar(dst, x+36, y+42, w*.42, 12, done/total, g.th.accent)
		}
		g.text(dst, g.db.Game.Screens[s.name].Title, x+36, y+14, 18, color.Black)
		rect(dst, x+w*.52, y+8, w*.25, 86, color.NRGBA{248, 248, 244, 255})
		g.text(dst, planningDate(g.st.Planning.Date, "", v), x+w*.53, y+16, 16, color.Black)
		date, err := time.Parse(time.DateOnly, g.st.Planning.Date)
		if err != nil {
			return
		}
		monday := date.AddDate(0, 0, -(int(date.Weekday())+6)%7)
		for i := range 7 {
			d := monday.AddDate(0, 0, i)
			cx := x + w*.53 + float64(i)*w*.032
			tint := color.NRGBA{225, 229, 234, 255}
			if d.Equal(date) {
				tint = color.NRGBA{145, 183, 236, 255}
			}
			rect(dst, cx, y+45, w*.029, 34, tint)
			g.text(dst, strconv.Itoa(d.Day()), cx+2, y+53, 15, color.Black)
		}
		rect(dst, x+w*.81, y+4, w*.15, 100, color.NRGBA{151, 118, 81, 255})
		g.drawPlanningText(dst, v.Door, x+w*.82, y+20, w*.13, 16, 2, color.White)
		y += 116
		h -= 116
		if v.Fullscreen {
			rect(dst, x+w*.75, y+h*.20, w*.23, h*.72, color.NRGBA{174, 145, 105, 255})
			rect(dst, x+w*.77, y+h*.24, w*.19, h*.50, color.NRGBA{53, 58, 60, 255})
			w *= .72
		}
	}
	if kind == "entities" && v.Detail != "" {
		s.drawEntityDetail(g, dst, x+w*.64, y, w*.36, h)
		w *= .64
	}
	for i, r := range planningCells(x+12, y, w-24, h, len(ids), cols) {
		id := ids[i]
		values := g.st.EntityValues(id, false)
		xx, yy, cw, ch := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
		value := func(key string) string {
			if key == "" || values[key] == nil {
				return ""
			}
			return fmt.Sprint(values[key])
		}
		portrait, badge := value(v.Portrait), value(v.Badge)
		switch kind {
		case "islands":
			rect(dst, xx, yy+ch-78, cw, 76, color.NRGBA{174, 145, 105, 255})
			if badge != "" {
				g.drawPlanningText(dst, badge, xx+8, yy+8, cw-16, 18, 1, color.Black)
			} else {
				g.planningFace(dst, portrait, xx+cw/2, yy+ch*.20, min(26, ch*.18))
			}
			g.drawPlanningText(dst, p.Entities[id].Name, xx+4, yy+ch-72, cw-8, 18, 1, color.Black)
			for j, f := range v.Fields {
				g.drawPlanningText(dst, f.Label+" "+value(f.Key), xx+4, yy+ch-48+float64(j)*20, cw-8, 15, 1, color.Black)
			}
		case "ensemble":
			rect(dst, xx, yy, cw, ch, color.NRGBA{53, 58, 60, 255})
			if values[v.Camera] == true {
				bg := value(v.Backdrop)
				if bg != "" && g.pics.get(bg) != missing {
					g.planningBackdrop(dst, bg, values[v.Mask] == true, xx, yy, cw, ch-28)
				} else {
					rect(dst, xx, yy, cw, ch-28, color.NRGBA{183, 184, 180, 255})
					rect(dst, xx+cw*.62, yy+12, cw*.30, ch*.35, color.NRGBA{196, 216, 231, 255})
					if values[v.Mask] == true { // Geometric fallback is coarse too.
						for row := range 4 {
							for col := range 6 {
								rect(dst, xx+float64(col)*cw/6, yy+float64(row)*(ch-28)/4, cw/6, (ch-28)/4, color.NRGBA{uint8(154 + col*6), uint8(160 + row*9), 170, 255})
							}
						}
					}
				}
				if portrait != "" && g.pics.get(portrait) != missing {
					g.planningTorso(dst, portrait, xx+cw*.16, yy, cw*.68, ch-28)
				} else {
					g.planningFace(dst, "", xx+cw/2, yy+ch*.38, min(30, ch*.23))
					rect(dst, xx+cw*.26, yy+ch*.58, cw*.48, ch*.20, color.NRGBA{55, 69, 93, 255})
				}
			} else {
				g.planningFace(dst, value(v.Icon), xx+cw/2, yy+ch*.43, min(32, ch*.26))
			}
			if len(v.Fields) > 0 {
				count := min(2, len(v.Fields))
				summaryH := min(80.0, ch-28)
				fieldH := summaryH / float64(count)
				rect(dst, xx, yy+ch-28-summaryH, cw, summaryH, color.NRGBA{30, 34, 40, 245})
				for j, f := range v.Fields[:count] {
					g.drawPlanningText(dst, f.Label+value(f.Key), xx+8, yy+ch-24-summaryH+float64(j)*fieldH, cw-16, 15, max(1, int((fieldH-8)/gfx.LineHeight(15))), color.White)
				}
			}
			g.drawPlanningText(dst, p.Entities[id].Name, xx+8, yy+ch-24, cw-16, 16, 1, color.White)
			if g.talk.active && g.talk.say.Actor != "" && p.Entities[id].Actor == g.talk.say.Actor {
				rect(dst, xx, yy, cw, 3, g.th.accent)
				rect(dst, xx, yy+ch-3, cw, 3, g.th.accent)
				rect(dst, xx, yy, 3, ch, g.th.accent)
				rect(dst, xx+cw-3, yy, 3, ch, g.th.accent)
			}
		default:
			g.planningFace(dst, portrait, xx+24, yy+ch/2, min(22, ch*.40))
			g.drawPlanningText(dst, p.Entities[id].Name, xx+58, yy+8, cw*.24, 16, 1, g.th.text)
			fw := (cw * .66) / float64(max(1, len(v.Fields)))
			for j, f := range v.Fields {
				fx := xx + cw*.34 + float64(j)*fw
				g.drawPlanningText(dst, f.Label+" "+value(f.Key), fx, yy+4, fw-12, 16, 1, g.th.text)
				if f.Maximum > 0 {
					n, _ := expr.ToNumber(values[f.Key])
					bar(dst, fx, yy+26, fw-16, 8, n/f.Maximum, g.th.accent)
					for _, m := range f.Marks {
						mx := fx + (fw-16)*m/f.Maximum
						gfx.StrokeLine(dst, float32(mx), float32(yy+23), float32(mx), float32(yy+35), 1, g.th.dim, true)
						g.text(dst, strconv.FormatFloat(m, 'f', 0, 64), mx-8, yy+36, 15, g.th.dim)
					}
				}
			}
		}
	}
}

func (g *Game) planningFace(dst *ebiten.Image, path string, cx, cy, r float64) {
	gfx.FillCircle(dst, float32(cx), float32(cy), float32(r), color.NRGBA{238, 232, 219, 255}, true)
	if path != "" && g.pics.get(path) != missing {
		// Face crop is centered; circular clipping keeps transparent portraits within a badge.
		img := g.pics.get(path)
		head := g.pics.head(path)
		scale := 2 * r / float64(max(1, head.Dx()))
		vertices := make([]ebiten.Vertex, 34)
		for i := range vertices {
			angle := float64(i-1) * 2 * math.Pi / 32
			dx, dy := math.Cos(angle)*r, math.Sin(angle)*r
			if i == 0 {
				dx, dy = 0, 0
			}
			vertices[i] = ebiten.Vertex{DstX: float32((cx + dx) * gfx.Scale()), DstY: float32((cy + dy) * gfx.Scale()), SrcX: float32(float64(head.Min.X+head.Dx()/2) + dx/scale), SrcY: float32(float64(head.Min.Y+head.Dy()/2) + dy/scale), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
		}
		indices := make([]uint16, 0, 96)
		for i := 1; i <= 32; i++ {
			indices = append(indices, 0, uint16(i), uint16(i+1))
		}
		dst.DrawTriangles(vertices, indices, img, &ebiten.DrawTrianglesOptions{Filter: ebiten.FilterLinear})
		return
	}
	gfx.FillCircle(dst, float32(cx), float32(cy), float32(r*.77), color.NRGBA{235, 190, 148, 255}, true)
	for _, dx := range []float64{-.25, .25} {
		gfx.FillCircle(dst, float32(cx+dx*r), float32(cy-r*.12), 2, color.Black, true)
	}
	gfx.StrokeLine(dst, float32(cx-r*.20), float32(cy+r*.30), float32(cx+r*.20), float32(cy+r*.30), 2, color.Black, true)
}

func (g *Game) planningTorso(dst *ebiten.Image, path string, x, y, w, h float64) {
	img := g.pics.get(path)
	bounds := img.Bounds()
	bounds.Max.Y = bounds.Min.Y + int(float64(bounds.Dy())*.58)
	sub, ok := img.SubImage(bounds).(*ebiten.Image)
	if !ok {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	scale := min(w/float64(bounds.Dx()), h/float64(bounds.Dy()))
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x+(w-float64(bounds.Dx())*scale)/2, y+h-float64(bounds.Dy())*scale)
	gfx.DrawImage(dst, sub, op)
}

func (g *Game) planningBackdrop(dst *ebiten.Image, path string, masked bool, x, y, w, h float64) {
	if !masked {
		g.drawPicture(dst, data.Picture{File: path}, x, y, w, h, 1)
		return
	}
	img := g.pics.blurred(path)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Scale(w/float64(img.Bounds().Dx()), h/float64(img.Bounds().Dy()))
	op.GeoM.Translate(x, y)
	gfx.DrawImage(dst, img, op)
}

func planningEventLabel(v data.PlanningView) string {
	if v.EventLabel != "" {
		return v.EventLabel
	}
	return "events"
}

func planningDate(date, phase string, v data.PlanningView) string {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return date
	}
	layout := v.DateFormat
	if layout == "" {
		layout = time.DateOnly
	}
	label := d.Format(layout)
	if len(v.Weekdays) == 7 {
		label = strings.ReplaceAll(label, "{weekday}", v.Weekdays[d.Weekday()])
	}
	if translated := v.Phases[phase]; translated != "" {
		phase = translated
	}
	if phase == "" {
		return label
	}
	return label + " " + phase
}

// The detail binding is an explicitly observed value, never a hidden attribute.
func (s *authorScreen) drawEntityDetail(g *Game, dst *ebiten.Image, x, y, w, h float64) {
	if s.target == "" {
		return
	}
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	g.window(dst, x, y, w-12, h)
	g.text(dst, p.Entities[s.target].Name, x+16, y+16, sizeM, g.th.accent)
	yy := y + 54
	for _, id := range slices.Sorted(maps.Keys(g.st.Planning.Activities)) {
		a := g.st.Planning.Activities[id]
		if a.Cancelled || a.Assignee != s.target || a.Completed != "" {
			continue
		}
		d := p.Activities[id]
		g.drawPlanningText(dst, fmt.Sprintf("%s  %.0f%%", d.Name, 100*a.Observed/g.st.ActivityEffort(id)), x+16, yy, w-44, 16, 1, g.th.text)
		yy += 24
	}
	g.drawPlanningText(dst, fmt.Sprint(g.st.EntityValues(s.target, false)[v.Detail]), x+16, yy+16, w-44, 16, max(1, int((y+h-yy-24)/24)), g.th.text)
}

// timelineRows keeps task names and quantities on distinct, readable rows.
func timelineRows(height float64) int {
	return max(1, int(height/56))
}

// Planning tables truncate overflow instead of shrinking below the readable size.
func (g *Game) drawPlanningText(dst *ebiten.Image, text string, x, y, width, size float64, limit int, tint color.Color) {
	size = max(15, size)
	lines := gfx.Wrap(text, size, width)
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = gfx.Ellipsis(lines[limit-1]+"…", size, width)
	}
	for i, line := range lines {
		g.text(dst, line, x, y+float64(i)*gfx.LineHeight(size), size, tint)
	}
}

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
		if !seen[id] {
			rows = append(rows, data.TimelineRow{Label: p.Activities[id].Name, Items: []data.TimelineItem{{Activity: id}}})
		}
	}
	return rows
}

type timelineLane struct {
	item                            data.TimelineItem
	bounds                          image.Rectangle
	left, right, reported, expected float64
}

type timelineGroup struct {
	label     string
	y, height float64
	lanes     []timelineLane
}

// Grouped axes stay fixed to the authored calendar, even after deadline changes.
// All geometry is logical, shared by drawing, pointer input and keyboard focus.
func (s *authorScreen) groupedTimeline(g *Game, x, y, w, h float64) ([]timelineGroup, float64, float64) {
	p := g.db.Game.Planning
	rows := timelineGroups(p, g.db.Game.Screens[s.name].View)
	axisX, axisW := x+260, max(1, w-280)
	start, err := time.Parse(time.DateOnly, p.Calendar.Start)
	if err != nil {
		return nil, axisX, axisW
	}
	end, err := time.Parse(time.DateOnly, p.Calendar.End)
	if err != nil {
		return nil, axisX, axisW
	}
	today, err := time.Parse(time.DateOnly, g.st.Planning.Date)
	if err != nil {
		return nil, axisX, axisW
	}
	days := end.Sub(start).Hours()/24 + 1
	rowHeight := func(row data.TimelineRow) float64 { return max(36, float64(len(row.Items))*10+4) }
	y += 56
	h -= 56
	// Keep readable minimums: larger authored tables scroll whole groups.
	s.timelineTop = min(s.timelineTop, max(0, len(rows)-1))
	var out []timelineGroup
	bottom := y + h
	for _, row := range rows[s.timelineTop:] {
		rh := rowHeight(row)
		if y+rh > bottom {
			break
		}
		group := timelineGroup{label: row.Label, y: y, height: rh}
		for j, item := range row.Items {
			d := p.Activities[item.Activity]
			a := g.st.Planning.Activities[item.Activity]
			from, to := start, end
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
			left := max(0, min(1, from.Sub(start).Hours()/24/days))
			right := max(left, min(1, (to.Sub(start).Hours()/24+1)/days))
			reported := max(0, min(1, a.Observed/g.st.ActivityEffort(item.Activity)))
			expected := max(0, min(1, today.Sub(from).Hours()/24/max(1, to.Sub(from).Hours()/24+1)))
			ly := y + 3 + float64(j)*10
			group.lanes = append(group.lanes, timelineLane{item: item, bounds: image.Rect(int(axisX+axisW*left), int(ly), int(math.Ceil(axisX+axisW*right)), int(ly+9)), left: left, right: right, reported: reported, expected: expected})
		}
		out = append(out, group)
		y += rh
	}
	return out, axisX, axisW
}

func (s *authorScreen) drawGroupedTimeline(g *Game, dst *ebiten.Image, x, y, w, h float64) {
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	groups, ax, aw := s.groupedTimeline(g, x, y, w, h)
	start, err := time.Parse(time.DateOnly, p.Calendar.Start)
	if err != nil {
		return
	}
	end, err := time.Parse(time.DateOnly, p.Calendar.End)
	if err != nil {
		return
	}
	days := end.Sub(start).Hours()/24 + 1
	dx := aw / days
	bottom := y + 56
	for _, group := range groups {
		bottom = group.y + group.height
	}
	muted := color.NRGBA{62, 72, 85, 255}
	red := color.NRGBA{240, 116, 108, 255}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		xx := ax + d.Sub(start).Hours()/24*dx
		if slices.Contains(p.Calendar.WeekdaysOff, int(d.Weekday())) || slices.Contains(p.Calendar.Closed, d.Format(time.DateOnly)) {
			rect(dst, xx, y+23, dx, bottom-y-23, color.NRGBA{48, 53, 62, 255})
		}
		if d.Day() == 1 || d.Equal(start) {
			g.text(dst, d.Format("2006/1"), xx+2, y, 15, g.th.text)
			gfx.StrokeLine(dst, float32(xx), float32(y+20), float32(xx), float32(bottom), 1, muted, true)
		}
		if d.Weekday() == time.Monday {
			g.text(dst, d.Format("2"), xx+2, y+23, 15, g.th.dim)
		}
	}
	// Annotation text follows the pointer's date in the footer; compact ticks
	// keep dense holidays from overwriting adjacent labels on the shared axis.
	for _, marker := range v.Markers {
		d, err := time.Parse(time.DateOnly, marker.Date)
		if err != nil {
			continue
		}
		if d.Before(start) || d.After(end) {
			continue
		}
		xx := ax + d.Sub(start).Hours()/24*dx
		rect(dst, xx, y+46, max(2, dx), 5, g.th.accent)
		if marker.Milestone {
			for yy := y + 54; yy < bottom; yy += 8 {
				rect(dst, xx, yy, 1, min(4, bottom-yy), g.th.dim)
			}
		}
	}
	index := 0
	for _, group := range groups {
		g.drawPlanningText(dst, group.label, x+16, group.y, 232, 16, 1, g.th.text)
		var labels []string
		for _, lane := range group.lanes {
			labels = append(labels, lane.item.Label)
			r := lane.bounds
			lx, ly, lw, lh := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
			a := g.st.Planning.Activities[lane.item.Activity]
			border := g.th.dim
			if index == s.timelineFocus {
				border = g.th.text
			}
			rect(dst, lx, ly, lw, lh, border)
			rect(dst, lx+1, ly+1, max(0, lw-2), lh-2, color.NRGBA{30, 34, 40, 255})
			if a.Cancelled {
				for xx := lx; xx < lx+lw; xx += 6 {
					gfx.StrokeLine(dst, float32(xx), float32(ly+lh-1), float32(min(xx+4, lx+lw)), float32(ly+1), 1, muted, true)
				}
			} else {
				rect(dst, lx+1, ly+1, max(0, (lw-2)*lane.reported), lh-2, g.th.accent)
				if lane.reported < lane.expected {
					for xx := lx + lw*lane.reported; xx < lx+lw*lane.expected; xx += 4 {
						gfx.StrokeLine(dst, float32(xx), float32(ly+lh-1), float32(min(xx+3, lx+lw*lane.expected)), float32(ly+1), 1, red, true)
					}
				}
				if lane.expected > 0 && lane.expected < 1 {
					rect(dst, lx+lw*lane.expected, ly-1, 1, lh+2, g.th.text)
				}
			}
			index++
		}
		g.drawPlanningText(dst, strings.Join(labels, " / "), x+16, group.y+18, 232, 15, 1, g.th.dim)
		rect(dst, x+16, group.y+group.height-1, w-32, 1, muted)
	}
	today, err := time.Parse(time.DateOnly, g.st.Planning.Date)
	if err != nil {
		return
	}
	if !today.Before(start) && !today.After(end) {
		tx := ax + today.Sub(start).Hours()/24*dx
		rect(dst, tx, y+41, 2, max(0, bottom-y-41), red)
	}
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
	if g.in.mouse.Y >= 108 && g.in.mouse.Y < 164 && g.in.mouse.X >= 260 && float64(g.in.mouse.X) < sw-20 {
		start, err := time.Parse(time.DateOnly, p.Calendar.Start)
		if err != nil {
			return ""
		}
		end, err := time.Parse(time.DateOnly, p.Calendar.End)
		if err != nil {
			return ""
		}
		days := end.Sub(start).Hours()/24 + 1
		d := start.AddDate(0, 0, int((float64(g.in.mouse.X)-260)/(sw-280)*days))
		labels := []string{planningDate(d.Format(time.DateOnly), "", v)}
		for _, marker := range v.Markers {
			if marker.Date == d.Format(time.DateOnly) {
				labels = append(labels, marker.Label)
			}
		}
		return strings.Join(labels, " · ")
	}
	lanes := s.timelineLanes(g)
	if len(lanes) == 0 {
		return ""
	}
	s.timelineFocus = max(0, min(s.timelineFocus, len(lanes)-1))
	lane := lanes[s.timelineFocus]
	d := p.Activities[lane.item.Activity]
	a := g.st.Planning.Activities[lane.item.Activity]
	// Only the reported observation enters the label. Progress remains hidden.
	return fmt.Sprintf("%s · %s · %.0f%%\n%s — %s", d.Name, p.Entities[a.Assignee].Name, 100*lane.reported, d.From, g.st.ActivityDeadline(lane.item.Activity))
}

func (s *authorScreen) updateGroupedTimeline(g *Game) bool {
	lanes := s.timelineLanes(g)
	if len(lanes) == 0 {
		return false
	}
	open := func(lane timelineLane) {
		if lane.item.Screen != "" {
			g.openScreen(lane.item.Screen, nil)
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
			if g.in.mouse.In(lane.bounds) {
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
