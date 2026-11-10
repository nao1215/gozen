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

	"github.com/nao1215/gozen/internal/base/expr"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
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
	x, y, w, h = planningChart(x, y, w, h)
	if v.Fullscreen && g.hasMaterial() && (kind == "calendar" || kind == "timeline") {
		// A full-screen calendar or timeline over a picture stands on one glass card,
		// as the workspace entries do, so its lines stay readable.
		g.surfaceWindow(dst, x+12, y-52, w-24, h+60, "card")
	}
	switch kind {
	case "workspace":
		if v.Burnup != nil && len(v.Burnup.Bounds) == 4 {
			b := v.Burnup.Bounds
			s.drawBurnup(g, dst, float64(b[0]), float64(b[1]), float64(b[2]), float64(b[3]))
		}
		return
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
		days := planningDays(start, end) + 1
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
			left := max(0, min(1, planningDays(start, from)/days))
			right := max(left, min(1, (planningDays(start, to)+1)/days))
			gfx.StrokeLine(dst, float32(axisX), float32(yy+14), float32(axisX+axisW), float32(yy+14), 1, g.th.dim, true)
			if right > left {
				rect(dst, axisX+axisW*left, yy+6, axisW*(right-left), 10, color.NRGBA{160, 190, 235, 255})
				actual := max(0, min(1, a.Observed/g.st.ActivityEffort(id)))
				tint := g.th.accent
				today, err := time.Parse(time.DateOnly, st.Date)
				if err != nil {
					return
				}
				expected := max(0, min(1, planningDays(from, today)/max(1, planningDays(from, to)+1)))
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
		tx := axisX + axisW*max(0, min(1, planningDays(start, today)/days))
		gfx.StrokeLine(dst, float32(tx), float32(y), float32(tx), float32(y+h), 2, color.NRGBA{220, 65, 65, 255}, true)
	case "calendar":
		s.drawCalendar(g, dst, x, y, w, h)
	case "entities", "ensemble", "islands":
		s.drawResidents(g, dst, kind, x, y, w, h)
	}
}

// planningFrame is the part of the screen a full-screen planning view owns:
// under the title strip and above the band of actions.
func planningFrame(sw, sh float64) (x, y, w, h float64) { return 0, 48, sw, sh - 156 }

// planningChart is the part of a planning frame under its date line, where the
// calendar, the timeline and the residents are laid out.
func planningChart(x, y, w, h float64) (cx, cy, cw, ch float64) { return x, y + 60, w, h - 70 }

// surfaceChart is the chart of a full-screen planning view. drawSurface draws
// into it and every pointer test reads it, so the two cannot drift apart.
func (g *Game) surfaceChart() (x, y, w, h float64) {
	return planningChart(planningFrame(g.screen()))
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
		if v.Visible == "" || g.st.EntityValue(id, v.Visible) == true {
			out = append(out, id)
		}
	}
	return out
}

// ensembleHeights reserves a separate summary strip below the camera image.
func ensembleHeights(height float64, fields int) (face, summary float64) {
	content := max(0, height-28)
	if fields > 0 {
		summary = min(content, max(float64(min(2, fields))*24, min(80, content/3)))
	}
	return content - summary, summary
}

func (s *authorScreen) drawResidents(g *Game, dst *ebiten.Image, kind string, x, y, w, h float64) {
	p := g.db.Game.Planning
	v := g.db.Game.Screens[s.name].View
	if kind == "entities" && v.Fullscreen {
		s.drawEntityTable(g, dst)
		return
	}
	ids := g.visibleResidents(v)
	if kind == "ensemble" {
		ids = append(append([]string{}, v.Guests...), ids...)
	}
	if s.target != "" && !slices.Contains(ids, s.target) {
		s.target = ""
	}
	cols := 1
	if kind == "ensemble" || kind == "islands" {
		cols = 4
	}

	if kind == "ensemble" && len(ids) > 8 {
		cols = max(1, min(4, int(math.Ceil(math.Sqrt(float64(len(ids)))))))
	}
	if kind == "islands" {
		if v.Burnup != nil {
			if v.Window != "" {
				s.drawBurnup(g, dst, x+32, 104, w*.72-44, 180)
			} else {
				s.drawBurnup(g, dst, x+24, 58, w*.72-36, 198)
			}
		}
		y, h = 268, 334
		if v.Fullscreen {
			w *= .72
		}
	}

	if kind == "entities" && v.Detail != "" {
		s.drawEntityDetail(g, dst, x+w*.64, y, w*.36, h)
		w *= .64
	}
	cells := planningCells(x+12, y, w-24, h, len(ids), cols)
	if kind == "islands" {
		_, cells = s.surfaceTargets(g)
	}
	if kind == "ensemble" && v.Chat != nil {
		chatW := min(340.0, w*.30)
		log := newEnsembleLog(g, s.name)
		if g.talk.active && g.talk.say.Ensemble == s.name && g.talk.meeting != nil {
			log = g.talk.meeting
		}
		log.draw(g, dst, v.Chat, x+w-chatW, y, chatW, h)
		gridH := h
		if g.talk.active && g.talk.say.Ensemble == s.name {
			gridH -= 116
		}
		cells = ensembleCells(x+12, y, w-chatW-36, gridH, len(ids))
	}
	for i, r := range cells {
		id := ids[i]
		values := g.st.EntityValues(id, false)
		name, actor := "", ""
		if entity, ok := p.Entities[id]; ok {
			name, actor = entity.Name, entity.Actor
		}
		if kind == "ensemble" && slices.Contains(v.Guests, id) {
			if guest := g.db.Actors[id]; guest != nil {
				name, actor = guest.Name, id
				values = map[string]any{v.Camera: true, v.Portrait: g.db.FacePicture(guest, guest.Face, "")}
				for _, resident := range g.visibleResidents(v) {
					if backdrop, ok := g.st.EntityValues(resident, false)[v.Backdrop].(string); ok && backdrop != "" {
						values[v.Backdrop] = backdrop
						break
					}
				}
			}
			if bg := v.GuestBackdrops[id]; bg != "" {
				values[v.Backdrop] = bg
			}
		}
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
			g.drawResidentCard(dst, r, name, portrait, badge, values, v.Fields)

		case "ensemble":
			fields := v.Fields
			if v.Chat != nil {
				fields = nil
			}
			faceH, summaryH := ensembleHeights(ch, len(fields))
			rect(dst, xx, yy, cw, ch, color.NRGBA{53, 58, 60, 255})
			if values[v.Camera] == true {
				bg := value(v.Backdrop)
				if bg != "" && g.pics.get(bg) != missing {
					g.planningBackdrop(dst, bg, values[v.Mask] == true, xx, yy, cw, faceH)
				} else {
					rect(dst, xx, yy, cw, faceH, color.NRGBA{183, 184, 180, 255})
					rect(dst, xx+cw*.62, yy+12, cw*.30, ch*.35, color.NRGBA{196, 216, 231, 255})
					if values[v.Mask] == true { // Geometric fallback is coarse too.
						for row := range 4 {
							for col := range 6 {
								rect(dst, xx+float64(col)*cw/6, yy+float64(row)*faceH/4, cw/6, faceH/4, color.NRGBA{uint8(154 + col*6), uint8(160 + row*9), 170, 255})
							}
						}
					}
				}
				if portrait != "" && g.pics.get(portrait) != missing {
					g.planningTorso(dst, portrait, xx, yy, cw, faceH)
				} else {
					g.planningFace(dst, "", xx+cw/2, yy+faceH*.38, min(30, faceH*.23))
					rect(dst, xx+cw*.26, yy+faceH*.58, cw*.48, faceH*.20, color.NRGBA{55, 69, 93, 255})
				}
			} else {
				g.planningFace(dst, value(v.Icon), xx+cw/2, yy+faceH*.5, min(52, faceH*.34))
			}
			if len(fields) > 0 {
				count := min(2, len(fields))
				fieldH := summaryH / float64(count)
				rect(dst, xx, yy+ch-28-summaryH, cw, summaryH, color.NRGBA{30, 34, 40, 245})
				for j, f := range fields[:count] {
					g.drawPlanningText(dst, f.Label+value(f.Key), xx+8, yy+ch-24-summaryH+float64(j)*fieldH, cw-16, 15, max(1, int((fieldH-8)/gfx.LineHeight(15))), color.White)
				}
			}
			label := name
			if v.Chat != nil && badge != "" {
				label += " · " + badge
			}
			g.drawPlanningText(dst, label, xx+8, yy+ch-24, cw-16, 16, 1, color.White)
			if badge != "" && v.Chat == nil {
				// Badges use only the explicitly observed binding, like seat cards.
				rect(dst, xx, yy, cw, 28, g.th.accent)
				g.drawPlanningText(dst, badge, xx+8, yy+4, cw-16, 15, 1, color.Black)
			}
			if values[v.Highlight] == true || (g.talk.active && g.talk.say.Actor != "" && actor == g.talk.say.Actor) {
				rect(dst, xx, yy, cw, 3, g.th.accent)
				rect(dst, xx, yy+ch-3, cw, 3, g.th.accent)
				rect(dst, xx, yy, 3, ch, g.th.accent)
				rect(dst, xx+cw-3, yy, 3, ch, g.th.accent)
			}
		default:
			g.planningFace(dst, portrait, xx+24, yy+ch/2, min(22, ch*.40))
			g.drawPlanningText(dst, name, xx+58, yy+8, cw*.24, 16, 1, g.th.text)
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

// faceOutline is the badge outline of a face of half size r around the
// center: a circle, or a rounded square while faces are square.
func (g *Game) faceOutline(r float64) [][2]float64 {
	var pts [][2]float64
	if !g.faceSquare {
		for i := range 33 {
			angle := float64(i) * 2 * math.Pi / 32
			pts = append(pts, [2]float64{math.Cos(angle) * r, math.Sin(angle) * r})
		}
		return pts
	}
	cr := r * .22
	for k, c := range [4][2]float64{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
		for i := range 9 {
			angle := float64(k)*math.Pi/2 + float64(i)*math.Pi/16
			pts = append(pts, [2]float64{c[0]*(r-cr) + math.Cos(angle)*cr, c[1]*(r-cr) + math.Sin(angle)*cr})
		}
	}
	return append(pts, pts[0])
}

// fillFace fills the face badge (a circle or a rounded square) with c.
func (g *Game) fillFace(dst *ebiten.Image, cx, cy, r float64, c color.NRGBA) {
	if !g.faceSquare {
		gfx.FillCircle(dst, float32(cx), float32(cy), float32(r), c, true)
		return
	}
	gfx.FillRoundRect(dst, float32(cx-r), float32(cy-r), float32(2*r), float32(2*r), float32(r*.22), c)
}

func (g *Game) planningFace(dst *ebiten.Image, path string, cx, cy, r float64) {
	// The backing shows around a transparent portrait: warm paper on the
	// classic look, a cool neutral with ui.material, matching its glass.
	backing := color.NRGBA{238, 232, 219, 255}
	if g.hasMaterial() {
		backing = color.NRGBA{226, 232, 240, 255}
	}
	g.fillFace(dst, cx, cy, r, backing)
	if path != "" && g.pics.get(path) != missing {
		// Face crop is centered; the badge's outline clips transparent portraits.
		img := g.pics.get(path)
		head := g.pics.head(path)
		if g.faceSquare {
			// A square cut shows a little hair and neck around the face box.
			grow := head.Dx() * 3 / 20
			head = head.Inset(-grow)
		}
		scale := 2 * r / float64(max(1, head.Dx()))
		outline := g.faceOutline(r)
		vertices := make([]ebiten.Vertex, len(outline)+1)
		for i := range vertices {
			dx, dy := 0.0, 0.0
			if i > 0 {
				dx, dy = outline[i-1][0], outline[i-1][1]
			}
			vertices[i] = ebiten.Vertex{DstX: float32((cx + dx) * gfx.Scale()), DstY: float32((cy + dy) * gfx.Scale()), SrcX: float32(float64(head.Min.X+head.Dx()/2) + dx/scale), SrcY: float32(float64(head.Min.Y+head.Dy()/2) + dy/scale), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
		}
		indices := make([]uint16, 0, 3*len(outline))
		for i := 1; i < len(outline); i++ {
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

// faceOr is face, or the upper middle quarter of a picture whose face is unknown.
func faceOr(bounds, face image.Rectangle) image.Rectangle {
	if face.Empty() || !face.In(bounds) {
		return image.Rect(bounds.Min.X+bounds.Dx()/4, bounds.Min.Y, bounds.Min.X+3*bounds.Dx()/4, bounds.Min.Y+max(1, bounds.Dy()/4))
	}
	return face
}

func portraitTransform(bounds, face image.Rectangle, x, y, w, faceHeight float64) (scale, left, top float64) {
	face = faceOr(bounds, face)
	scale = faceHeight / float64(face.Dy())
	left = x + w/2 - float64(face.Min.X+face.Dx()/2)*scale
	top = y - float64(face.Min.Y)*scale
	return
}

func (g *Game) planningTorso(dst *ebiten.Image, path string, x, y, w, h float64) {
	g.planningBust(dst, path, x, y, w, h, .28, .42)
}

// planningBust draws a standing picture clipped to x, y, w, h with its face box
// faceTop down and faceHeight of the height (both fractions of h).
func (g *Game) planningBust(dst *ebiten.Image, path string, x, y, w, h, faceTop, faceHeight float64) {
	img := g.pics.get(path)
	face := g.pics.portraitFace(path, img.Bounds().Size())
	scale, left, top := portraitTransform(img.Bounds(), face, x, y+h*faceTop, w, h*faceHeight)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(left, top)
	clip := image.Rect(int(x*gfx.Scale()), int(y*gfx.Scale()), int((x+w)*gfx.Scale()), int((y+h)*gfx.Scale())).Intersect(dst.Bounds())
	if !clip.Empty() {
		if target, ok := dst.SubImage(clip).(*ebiten.Image); ok {
			gfx.DrawImage(target, img, op)
		}
	}
}

func (g *Game) planningBackdrop(dst *ebiten.Image, path string, masked bool, x, y, w, h float64) {
	if !masked {
		g.drawPicture(dst, data.Picture{File: path}, x, y, w, h, 1)
		return
	}
	img := g.pics.blurred(path)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
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

// planningDays measures UTC calendar dates across the full four-digit year range.
// Sub would saturate at the roughly 292-year limit of time.Duration.
func planningDays(from, to time.Time) float64 {
	return float64(to.Unix()-from.Unix()) / 86400
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
	textWidth := w - 44
	inline := v.Fullscreen && g.db.Game.Screens[s.name].Kind == "entities"
	if inline {
		tasks, notes, pages := s.entityDetailPage(g, w, h)
		g.drawPlanningText(dst, p.Entities[s.target].Name, x+16, y+8, w*.30-32, 18, 1, g.th.accent)
		for i, line := range tasks {
			g.text(dst, line, x+16, y+36+float64(i)*gfx.LineHeight(16), 16, g.th.text)
		}
		for i, line := range notes {
			g.text(dst, line, x+w*.30, y+12+float64(i)*gfx.LineHeight(16), 16, g.th.text)
		}
		if pages > 1 {
			r := image.Rect(int(x), int(y), int(x+w), int(y+h))
			for i, button := range entityDetailButtons(r) {
				g.window(dst, float64(button.Min.X), float64(button.Min.Y), float64(button.Dx()), float64(button.Dy()))
				label := "<"
				if i == 1 {
					label = ">"
				}
				g.text(dst, label, float64(button.Min.X)+16, float64(button.Min.Y)+4, 15, g.th.accent)
			}
			g.text(dst, fmt.Sprintf("%d / %d", s.residentPage+1, pages), x+w/2-30, y+h-28, 15, g.th.dim)
		}
		return
	}
	g.drawPlanningText(dst, p.Entities[s.target].Name, x+16, y+12, textWidth, 18, 1, g.th.accent)
	yy := y + 40
	for _, id := range slices.Sorted(maps.Keys(g.st.Planning.Activities)) {
		a := g.st.Planning.Activities[id]
		if a.Cancelled || a.Assignee != s.target || a.Completed != "" {
			continue
		}
		if yy+48 > y+h {
			break
		}
		d := p.Activities[id]
		g.drawPlanningText(dst, fmt.Sprintf("%s  %.0f%%", d.Name, 100*a.Observed/g.st.ActivityEffort(id)), x+16, yy, textWidth, 16, 1, g.th.text)
		yy += 24
	}
	if value := g.st.EntityValues(s.target, false)[v.Detail]; value != nil {
		g.drawPlanningText(dst, fmt.Sprint(value), x+16, yy+16, w-44, 16, max(1, int((y+h-yy-24)/24)), g.th.text)
	}
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

// drawPlanningTextCentered draws one line centered on x, shortened to width.
func (g *Game) drawPlanningTextCentered(dst *ebiten.Image, text string, x, y, width, size float64, tint color.Color) {
	line, _, _ := strings.Cut(text, "\n")
	line = gfx.Ellipsis(line, size, width)
	g.text(dst, line, x-gfx.TextWidth(line, size)/2, y, size, tint)
}
