package play

import (
	"cmp"
	"fmt"
	"image"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// tableResidents sorts only displayed observations. Equal values retain the
// authored order, missing values stay last, and zero restores the authored order.
func (s *authorScreen) tableResidents(g *Game) []string {
	v := g.db.Game.Screens[s.name].View
	ids := g.visibleResidents(v)
	if s.residentSort == 0 || s.residentSort > len(v.Fields)+1 {
		return ids
	}
	values := make(map[string]any, len(ids))
	for _, id := range ids {
		if s.residentSort == 1 {
			values[id] = g.db.Game.Planning.Entities[id].Name
		} else {
			values[id] = g.st.EntityValues(id, false)[v.Fields[s.residentSort-2].Key]
		}
	}
	slices.SortStableFunc(ids, func(a, b string) int {
		av, bv := values[a], values[b]
		if av == nil && bv == nil {
			return 0
		}
		if av == nil {
			return 1
		}
		if bv == nil {
			return -1
		}
		n, nok := expr.ToNumber(av)
		m, mok := expr.ToNumber(bv)
		order := strings.Compare(fmt.Sprint(av), fmt.Sprint(bv))
		if nok && mok {
			order = cmp.Compare(n, m)
		} else if nok != mok {
			// A mixed-type observation must still define a transitive order.
			order = 1
			if nok {
				order = -1
			}
		}
		if s.residentDescending {
			return -order
		}
		return order
	})
	return ids
}

func (s *authorScreen) entityTableHeaders(g *Game) []image.Rectangle {
	sw, _ := g.screen()
	fields := g.db.Game.Screens[s.name].View.Fields
	headers := []image.Rectangle{image.Rect(16, 54, 202, 88)}
	width := (sw - 230) / float64(max(1, len(fields)))
	for i := range fields {
		headers = append(headers, image.Rect(int(210+float64(i)*width), 54, int(210+float64(i+1)*width), 88))
	}
	return headers
}

// Both detail columns page together, so neither records nor assignments vanish
// below a fixed-height row. These are public reports, not live hidden progress.
func (s *authorScreen) entityDetailPage(g *Game, w, h float64) ([]string, []string, int) {
	v := g.db.Game.Screens[s.name].View
	var tasks []string
	for _, id := range slices.Sorted(maps.Keys(g.st.Planning.Activities)) {
		a := g.st.Planning.Activities[id]
		if a.Cancelled || a.Assignee != s.target || a.Completed != "" {
			continue
		}
		text := fmt.Sprintf("%s  %.0f%%", g.db.Game.Planning.Activities[id].Name, 100*a.Observed/g.st.ActivityEffort(id))
		tasks = append(tasks, gfx.Wrap(text, 16, w*.30-32)...)
	}
	value := g.st.EntityValues(s.target, false)[v.Detail]
	text := ""
	if value != nil {
		text = fmt.Sprint(value)
	}
	notes := gfx.Wrap(text, 16, w*.70-24)
	count := max(1, int((h-68)/gfx.LineHeight(16)))
	pages := max(1, (max(len(tasks), len(notes))+count-1)/count)
	s.residentPage = max(0, min(s.residentPage, pages-1))
	start := s.residentPage * count
	return tasks[min(start, len(tasks)):min(start+count, len(tasks))], notes[min(start, len(notes)):min(start+count, len(notes))], pages
}

func entityDetailButtons(r image.Rectangle) [2]image.Rectangle {
	return [2]image.Rectangle{
		image.Rect(r.Min.X+16, r.Max.Y-32, r.Min.X+64, r.Max.Y),
		image.Rect(r.Max.X-64, r.Max.Y-32, r.Max.X-16, r.Max.Y),
	}
}

// entityTableLayout is shared by drawing and hit testing. Expanded details take
// their own space below a row; scrolling never shrinks the text or column widths.
func (s *authorScreen) entityTableLayout(g *Game) ([]string, []image.Rectangle, image.Rectangle) {
	v := g.db.Game.Screens[s.name].View
	all := s.tableResidents(g)
	sw, sh := g.screen()
	s.residentTop = min(s.residentTop, max(0, len(all)-1))
	var ids []string
	var cells []image.Rectangle
	var detail image.Rectangle
	y, bottom := 90, int(sh)-112
	for _, id := range all[s.residentTop:] {
		if y+56 > bottom {
			break
		}
		ids = append(ids, id)
		cells = append(cells, image.Rect(16, y, int(sw)-16, y+56))
		y += 56
		if id == s.target && v.Detail != "" {
			detail = image.Rect(16, y, int(sw)-16, min(y+180, bottom))
			y += 180
		}
	}
	return ids, cells, detail
}

func (s *authorScreen) updateEntityTable(g *Game) {
	v := g.db.Game.Screens[s.name].View
	all := s.tableResidents(g)
	if g.in.click {
		for i, r := range s.entityTableHeaders(g) {
			if !g.in.mouse.In(r) {
				continue
			}
			if s.residentSort != i+1 {
				s.residentSort, s.residentDescending = i+1, true
			} else if s.residentDescending {
				s.residentDescending = false
			} else {
				s.residentSort = 0
			}
			s.residentTop, s.focus = 0, 0
			s.target, s.residentPage = "", 0
			return
		}
	}
	_, _, detail := s.entityTableLayout(g)
	if !detail.Empty() && g.in.mouse.In(detail) {
		_, _, pages := s.entityDetailPage(g, float64(detail.Dx()), float64(detail.Dy()))
		delta := 0
		if g.in.wheel < 0 {
			delta = 1
		} else if g.in.wheel > 0 {
			delta = -1
		}
		if g.in.click && pages > 1 {
			for i, r := range entityDetailButtons(detail) {
				if g.in.mouse.In(r) {
					delta = 2*i - 1
				}
			}
		}
		if delta != 0 {
			s.residentPage = max(0, min(s.residentPage+delta, pages-1))
			return
		}
	}
	var actions []int
	for i, e := range s.entries {
		if e.Target.ID == "" && len(e.Bounds) == 4 {
			actions = append(actions, i)
		}
	}
	count := len(all) + len(actions)
	if count == 0 {
		s.target = ""
		return
	}
	if !slices.Contains(all, s.target) {
		s.target = ""
	}
	if g.in.wheel != 0 {
		step := 1
		if g.in.wheel > 0 {
			step = -1
		}
		s.residentTop = max(0, min(len(all)-1, s.residentTop+step))
	}
	s.focus = min(s.focus, count-1)
	moved := g.in.wheel == 0 && (g.in.down() || g.in.up())
	if g.in.wheel == 0 && g.in.down() {
		s.focus = (s.focus + 1) % count
	}
	if g.in.wheel == 0 && g.in.up() {
		s.focus = (s.focus + count - 1) % count
	}
	ids, cells, _ := s.entityTableLayout(g)
	rowFocus := s.focus - len(actions)
	if moved && rowFocus >= 0 && !slices.Contains(ids, all[rowFocus]) {
		s.residentTop = rowFocus
	}
	chosen := ""
	if g.in.ok() {
		if rowFocus < 0 {
			s.list.sel = actions[s.focus]
			s.activate(g)
			return
		}
		chosen = all[rowFocus]
	}
	if g.in.click {
		for j, i := range actions {
			if g.in.mouse.In(entryBounds(s.entries[i])) {
				s.focus = j
				s.list.sel = i
				s.activate(g)
				return
			}
		}
		for i, r := range cells {
			if g.in.mouse.In(r) {
				chosen = ids[i]
				s.focus = len(actions) + slices.Index(all, chosen)
				break
			}
		}
	}
	if chosen != "" && v.Detail != "" {
		s.residentPage = 0
		if s.target == chosen {
			s.target = ""
		} else {
			s.target = chosen
			// Reveal the whole detail even when opening the last visible row.
			_, rows, detail := s.entityTableLayout(g)
			if detail.Dy() < 180 || len(rows) == 0 {
				s.residentTop = slices.Index(all, chosen)
			}
		}
	}
}

func (s *authorScreen) drawEntityTable(g *Game, dst *ebiten.Image) {
	v := g.db.Game.Screens[s.name].View
	sw, _ := g.screen()
	ids, cells, detail := s.entityTableLayout(g)
	start, width := 210.0, (sw-230)/float64(max(1, len(v.Fields)))
	for i, r := range s.entityTableHeaders(g) {
		label := s.title
		if i > 0 {
			label = v.Fields[i-1].Label
		}
		tint := g.th.dim
		if s.residentSort == i+1 {
			tint = g.th.accent
			marker := "^"
			if s.residentDescending {
				marker = "v"
			}
			g.text(dst, marker, float64(r.Max.X)-16, 62, 15, tint)
		}
		g.drawPlanningText(dst, label, float64(r.Min.X)+4, 62, float64(r.Dx())-24, 15, 1, tint)
	}
	all := s.tableResidents(g)
	for i, id := range ids {
		r := cells[i]
		y := float64(r.Min.Y)
		values := g.st.EntityValues(id, false)
		portrait, ok := values[v.Portrait].(string)
		if !ok {
			portrait = ""
		}
		g.planningFace(dst, portrait, 42, y+28, 24)
		tint := g.th.text
		if id == s.target {
			tint = g.th.accent
		}
		g.drawPlanningText(dst, g.db.Game.Planning.Entities[id].Name, 78, y+16, 124, 18, 1, tint)
		for j, f := range v.Fields {
			x := start + float64(j)*width
			value := ""
			if values[f.Key] != nil {
				value = fmt.Sprint(values[f.Key])
			}
			g.drawPlanningText(dst, value, x, y+4, width-8, 16, 1, g.th.text)
			if f.Maximum > 0 {
				n, _ := expr.ToNumber(values[f.Key])
				bar(dst, x, y+28, width-10, 6, n/f.Maximum, g.th.accent)
				for _, m := range f.Marks {
					mx := x + (width-10)*m/f.Maximum
					gfx.StrokeLine(dst, float32(mx), float32(y+25), float32(mx), float32(y+36), 1, g.th.dim, true)
					g.drawPlanningText(dst, strconv.FormatFloat(m, 'f', 0, 64), min(mx-8, x+width-34), y+36, 32, 15, 1, g.th.dim)
				}
			}
		}
		actionCount := 0
		for _, e := range s.entries {
			if e.Target.ID == "" && len(e.Bounds) == 4 {
				actionCount++
			}
		}
		if slices.Index(all, id)+actionCount == s.focus {
			rect(dst, 16, y+54, sw-32, 2, g.th.accent)
		}
	}
	if !detail.Empty() {
		s.drawEntityDetail(g, dst, float64(detail.Min.X), float64(detail.Min.Y), float64(detail.Dx()), float64(detail.Dy()))
	}
}

// planningMarkers derives public schedule labels in calendar order. It does not
// run events or inspect hidden state; authored markers keep their original order.
