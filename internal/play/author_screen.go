package play

import (
	"image"
	"image/color"
	"slices"

	"github.com/nao1215/gozen/internal/event"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// authorScreen presents an authored list without requiring a custom Go screen.
type authorScreen struct {
	calendarMonth string
	residentTop   int
	timelineTop   int
	timelineFocus int
	target        string
	focus         int
	popup         *list
	popupIndices  []int
	popupArmed    bool
	name          string
	title         string
	indices       []int
	running       bool
	parent        *event.Runner
	request       event.Request
	origin        string
	entries       []data.ScreenEntry
	list          *list
	done          func()
}

func newAuthorScreen(g *Game, name string, done func()) *authorScreen {
	s := &authorScreen{name: name, done: done}
	def := g.db.Game.Screens[name]
	var items []listItem
	if def != nil {
		s.title = data.Render(g.db.Tr(def.Title), g.st.Env())
		for i, entry := range def.Entries {
			visible, err := entry.When.Bool(g.st.Env())
			if err != nil || !visible {
				continue
			}
			entry.Label = data.Render(g.db.Tr(entry.Label), g.st.Env())
			entry.Text = data.Render(g.db.Tr(entry.Text), g.st.Env())
			s.entries = append(s.entries, entry)
			s.indices = append(s.indices, i)
			items = append(items, listItem{label: entry.Label})
		}
	}
	sw, sh := g.screen()
	s.list = newList(items, 24, 88, sw*0.32, max(1, int((sh-140)/44)))
	return s
}

func (s *authorScreen) modal() bool   { return !s.running }
func (s *authorScreen) overlay() bool { return s.running }

func (s *authorScreen) liveKeep(g *Game) scene { return s.refreshed(g) }

func (s *authorScreen) refreshed(g *Game) *authorScreen {
	n := newAuthorScreen(g, s.name, s.done)
	n.target = s.target
	n.focus = s.focus
	n.calendarMonth = s.calendarMonth
	n.residentTop = s.residentTop
	n.timelineTop = s.timelineTop
	n.timelineFocus = s.timelineFocus
	n.list.sel = min(s.list.sel, max(0, len(n.entries)-1))
	if s.list.sel < len(s.indices) {
		for i, index := range n.indices {
			if index == s.indices[s.list.sel] {
				n.list.sel = i
				break
			}
		}
	}
	n.running, n.origin = s.running, s.origin
	n.list.scroll()
	return n
}

func (s *authorScreen) update(g *Game) {
	if s.running {
		if g.pump() {
			return
		}
		if s.parent != nil {
			g.events, g.req = s.parent, s.request
		}
		s.parent, s.request, s.running = nil, nil, false
		g.boundary()
		// The resumed opener already crossed an open-screen boundary. Its
		// runner must not inherit the completed child's replay snapshot: F5
		// would otherwise restore state from before the entry action.
		g.evCmds, g.evSnap, g.evRand = nil, nil, nil
		g.evName, g.evLines, g.evArrival = "", 0, false
		g.evAnswers, g.evHist = nil, nil
		if g.st.Location != s.origin {
			g.pop()
			if s.done != nil {
				s.done()
			}
			return
		}
		s.refresh(g)
		return
	}
	if g.db.Game.Screens[s.name].View.Fullscreen {
		s.updateSurface(g)
		return
	}
	if g.db.Game.Screens[s.name].Kind == "timeline" && len(s.entries) == 0 {
		_, sh := g.screen()
		s.scrollTimeline(g, sh-214)
	}
	chose, cancel := s.list.update(g)
	if cancel {
		g.pop()
		if s.done != nil {
			s.done()
		}
		return
	}
	if chose {
		s.activate(g)
	}
}

func (s *authorScreen) refresh(g *Game) {
	n := s.refreshed(g)
	*s = *n
}

func (s *authorScreen) activate(g *Game) {
	def := g.db.Game.Screens[s.name]
	if def == nil || s.list.sel >= len(s.indices) {
		s.refresh(g)
		return
	}
	index := s.indices[s.list.sel]
	if index >= len(def.Entries) {
		s.refresh(g)
		return
	}
	entry := def.Entries[index]
	visible, err := entry.When.Bool(g.st.Env())
	if err != nil || !visible {
		s.refresh(g)
		return
	}
	if len(entry.Do) == 0 {
		return
	}
	// Suspend the opener so entry commands run immediately and have their own
	// named save cursor. Closing the screen resumes the opener after its open.
	s.parent, s.request, s.origin, s.running = g.events, g.req, g.st.Location, true
	g.events, g.req = g.newEventRunner(), nil
	g.events.Start(entry.Do)
	g.emit("screen_action", traceLine{"screen": s.name, "entry": index})
}

func (s *authorScreen) draw(g *Game, dst *ebiten.Image) {
	if s.running {
		if g.db.Game.Screens[s.name].View.Fullscreen {
			s.drawSurface(g, dst)
		}
		return
	}
	if g.db.Game.Screens[s.name].View.Fullscreen {
		s.drawSurface(g, dst)
		return
	}
	sw, sh := g.screen()
	fill(dst, color.NRGBA{0, 0, 0, 220})
	g.drawLinesFit(dst, s.title, 36, 28, sw-72, sizeL, 1, g.th.accent)
	if len(s.entries) > 0 {
		s.list.draw(g, dst, true)
	}
	x, w := sw*0.36, sw*0.64-36
	if def := g.db.Game.Screens[s.name]; def != nil && def.Kind != "" {
		if len(s.entries) == 0 {
			x, w = 24, sw-48
		}
		s.drawPlanning(g, dst, def.Kind, x, 88, w, sh-120)
		return
	}
	g.window(dst, x, 88, w, sh-120)
	if len(s.entries) == 0 {
		g.text(dst, "No entries yet", x+20, 108, sizeM, g.th.dim)
		return
	}
	e := s.entries[s.list.sel]
	y := float64(108)
	if e.Picture != "" {
		g.drawPicture(dst, data.Picture{File: e.Picture}, x+20, y, w-40, (sh-160)*0.55, 1)
		y += (sh-160)*0.55 + 16
	}
	g.drawLinesFit(dst, e.Text, x+20, y, w-40, sizeM, max(1, int((sh-y-52)/32)), g.th.text)
}

// surfaceTargets shares the exact resident layout with drawing.
func (s *authorScreen) surfaceTargets(g *Game) ([]string, []image.Rectangle) {
	def := g.db.Game.Screens[s.name]
	if def.Kind != "islands" && (def.Kind != "entities" || def.View.Detail == "") {
		return nil, nil
	}
	sw, sh := g.screen()
	ids := g.visibleResidents(def.View)
	if def.Kind == "entities" {
		ids, cells, _ := s.entityTableLayout(g)
		return ids, cells
	}
	return ids, planningCells(12, 48+60+116, sw*.72-24, sh-156-70-116, len(ids), 4)
}

func entryBounds(e data.ScreenEntry) image.Rectangle {
	if len(e.Bounds) != 4 {
		return image.Rectangle{}
	}
	return image.Rect(e.Bounds[0], e.Bounds[1], e.Bounds[0]+e.Bounds[2], e.Bounds[1]+e.Bounds[3])
}

func (s *authorScreen) openTarget(g *Game, id string, r image.Rectangle) {
	s.target = id
	s.popupArmed = false
	if g.db.Game.Screens[s.name].Kind == "entities" {
		return
	}
	s.popupIndices = nil
	var items []listItem
	for i, e := range s.entries {
		if e.Target == id {
			s.popupIndices = append(s.popupIndices, i)
			items = append(items, listItem{label: e.Label})
		}
	}
	if len(items) == 0 {
		s.target = ""
		s.popup = nil
		return
	}
	sw, sh := g.screen()
	rows := max(1, min(6, len(items), int((sh-128)/44)))
	s.popup = newList(items, max(0, min(float64(r.Min.X), sw-380)), max(40, min(float64(r.Max.Y), sh-44*float64(rows)-32)), 360, rows)
	if g.db.Game.Screens[s.name].View.Fullscreen {
		// Use the auxiliary column rather than covering another resident.
		s.popup.x, s.popup.y, s.popup.w = sw*.72+8, 148, sw*.28-24
	}
	s.popup.size = 22
}

func (s *authorScreen) closeSurface(g *Game) {
	g.pop()
	if s.done != nil {
		s.done()
	}
}

func (s *authorScreen) updateSurface(g *Game) {
	if s.popup != nil {
		r := image.Rect(int(s.popup.x), int(s.popup.y), int(s.popup.x+s.popup.w), int(s.popup.y+s.popup.height()))
		if g.in.click && !g.in.mouse.In(r) {
			s.popup = nil
			s.target = ""
			return
		}
		previous := s.popup.sel
		chose, cancel := s.popup.update(g)
		if previous != s.popup.sel {
			s.popupArmed = false
		}
		if cancel {
			s.popup = nil
			s.target = ""
			return
		}
		if chose && len(s.popupIndices) > 0 {
			s.list.sel = s.popupIndices[s.popup.sel]
			// Authored explanatory text opts a target action into preview.
			if s.entries[s.list.sel].Text != "" && !s.popupArmed {
				s.popupArmed = true
				return
			}
			s.popup = nil
			s.activate(g)
		}
		return
	}
	if g.in.cancel() {
		s.closeSurface(g)
		return
	}
	if s.updateTabs(g) {
		return
	}
	if g.db.Game.Screens[s.name].Kind == "calendar" && s.updateCalendar(g) {
		return
	}
	if g.db.Game.Screens[s.name].Kind == "entities" {
		s.updateEntityTable(g)
		return
	}
	if g.db.Game.Screens[s.name].Kind == "timeline" && len(g.db.Game.Screens[s.name].View.Rows) > 0 {
		if s.updateGroupedTimeline(g) {
			return
		}
	} else if g.db.Game.Screens[s.name].Kind == "timeline" {
		_, sh := g.screen()
		s.scrollTimeline(g, sh-250)
	}
	ids, cells := s.surfaceTargets(g)
	var actions []int
	for i, e := range s.entries {
		if e.Target == "" && len(e.Bounds) == 4 {
			actions = append(actions, i)
		}
	}
	n := len(actions) + len(ids)
	if n == 0 {
		return
	}
	if g.in.down() {
		s.focus = (s.focus + 1) % n
	}
	if g.in.up() {
		s.focus = (s.focus + n - 1) % n
	}
	if g.in.click {
		for i, r := range cells {
			if g.in.mouse.In(r) {
				s.focus = len(actions) + i
				s.openTarget(g, ids[i], r)
				return
			}
		}
		for j, i := range actions {
			if g.in.mouse.In(entryBounds(s.entries[i])) {
				s.focus = j
				s.list.sel = i
				s.activate(g)
				return
			}
		}
	}
	if g.in.ok() {
		if s.focus < len(actions) {
			s.list.sel = actions[s.focus]
			s.activate(g)
		} else {
			i := s.focus - len(actions)
			s.openTarget(g, ids[i], cells[i])
		}
	}
}

func (s *authorScreen) drawSurface(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	def := g.db.Game.Screens[s.name]
	v := def.View
	fill(dst, color.NRGBA{30, 34, 40, 255})
	s.drawPlanning(g, dst, def.Kind, 0, 48, sw, sh-156)
	s.drawPlanningHeader(g, dst)
	g.window(dst, 0, sh-108, sw, 108)
	footerWidth := sw * .70
	if tabs := s.tabBounds(g); len(tabs) > 0 {
		footerWidth = float64(tabs[0].Min.X) - 40
	}
	for _, e := range s.entries {
		r := entryBounds(e)
		if e.Target == "" && !r.Empty() && float64(r.Min.Y) >= sh-108 {
			footerWidth = min(footerWidth, float64(r.Min.X)-40)
		}
	}
	footer := data.Render(g.db.Tr(v.Footer), g.st.Env())
	if def.Kind == "timeline" && len(v.Rows) > 0 {
		footer = s.timelineDescription(g)
	}
	if def.Kind == "calendar" {
		if description := s.calendarDescription(g); description != "" {
			footer = description
		}
	}
	if g.noticeSurface() == s && g.noticesShown() && len(g.notices.list) > 0 {
		footer = g.notices.list[0].text
	}
	g.drawPlanningText(dst, footer, 24, sh-88, max(1, footerWidth), sizeS, 2, g.th.text)
	j := 0
	for _, e := range s.entries {
		if e.Target != "" || len(e.Bounds) != 4 {
			continue
		}
		r := entryBounds(e)
		g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		tint := g.th.text
		if j == s.focus {
			tint = g.th.accent
		}
		g.drawPlanningText(dst, e.Label, float64(r.Min.X+10), float64(r.Min.Y+10), float64(r.Dx()-20), sizeS, 1, tint)
		if e.Text != "" && r.Dy() >= 72 {
			g.drawPlanningText(dst, e.Text, float64(r.Min.X+10), float64(r.Min.Y+38), float64(r.Dx()-20), 16, max(1, (r.Dy()-46)/24), g.th.text)
		}
		j++
	}
	_, cells := s.surfaceTargets(g)
	if i := s.focus - j; def.Kind != "entities" && i >= 0 && i < len(cells) {
		r := cells[i]
		rect(dst, float64(r.Min.X), float64(r.Max.Y-3), float64(r.Dx()), 3, g.th.accent)
	}
	if s.popup != nil {
		g.window(dst, s.popup.x, s.popup.y-40, s.popup.w, 42)
		g.drawPlanningText(dst, g.db.Game.Planning.Entities[s.target].Name, s.popup.x+12, s.popup.y-30, s.popup.w-24, sizeS, 1, g.th.accent)
		s.popup.draw(g, dst, true)
		if s.popupArmed {
			e := s.entries[s.popupIndices[s.popup.sel]]
			y := s.popup.y + s.popup.height()
			g.window(dst, s.popup.x, y, s.popup.w, 144)
			g.drawPlanningText(dst, e.Text, s.popup.x+12, y+12, s.popup.w-24, 18, 5, g.th.text)
		}
	}
	s.drawTabs(g, dst)
}

func (s *authorScreen) tabBounds(g *Game) []image.Rectangle {
	tabs := g.db.Game.Screens[s.name].View.Tabs
	sw, sh := g.screen()
	width := min(152.0, (sw-32)/float64(max(1, len(tabs))))
	bounds := make([]image.Rectangle, len(tabs))
	for i := range bounds {
		x := int(sw - 16 - float64(len(tabs)-i)*width)
		bounds[i] = image.Rect(x, int(sh-72), x+int(width)-8, int(sh-24))
	}
	return bounds
}

func (s *authorScreen) updateTabs(g *Game) bool {
	tabs := g.db.Game.Screens[s.name].View.Tabs
	if len(tabs) == 0 {
		return false
	}
	selected := slices.Index(tabs, s.name)
	next := selected
	if g.in.left() {
		next = (selected + len(tabs) - 1) % len(tabs)
	} else if g.in.right() {
		next = (selected + 1) % len(tabs)
	}
	if g.in.click {
		for i, r := range s.tabBounds(g) {
			if g.in.mouse.In(r) {
				next = i
			}
		}
	}
	if next < 0 || next == selected {
		return false
	}
	// Keep the existing stack slot and opener; switching is not an event action.
	*s = *newAuthorScreen(g, tabs[next], s.done)
	return true
}

func (s *authorScreen) drawTabs(g *Game, dst *ebiten.Image) {
	tabs := g.db.Game.Screens[s.name].View.Tabs
	for i, r := range s.tabBounds(g) {
		g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		tint := g.th.text
		if tabs[i] == s.name {
			tint = g.th.accent
			rect(dst, float64(r.Min.X), float64(r.Max.Y-3), float64(r.Dx()), 3, tint)
		}
		label := data.Render(g.db.Tr(g.db.Game.Screens[tabs[i]].Title), g.st.Env())
		g.drawPlanningText(dst, label, float64(r.Min.X+12), float64(r.Min.Y+12), float64(r.Dx()-24), 18, 1, tint)
	}
}

func (s *authorScreen) scrollTimeline(g *Game, height float64) {
	if g.in.down() {
		s.timelineTop++
	}
	if g.in.up() {
		s.timelineTop--
	}
	s.timelineTop = max(0, min(s.timelineTop, len(g.st.Planning.Activities)-timelineRows(height)))
}

// drawPlanningHeader keeps authored status text shared by surfaces and dialogue.
// Clip each column by ellipsis, never by shrinking below the normal label size.
func (s *authorScreen) drawPlanningHeader(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	v := g.db.Game.Screens[s.name].View
	st := g.st.Planning
	g.window(dst, 0, 0, sw, 48)
	g.drawPlanningText(dst, planningDate(st.Date, g.db.Game.Planning.Calendar.Phases[st.Phase], v), 24, 12, 200, sizeS, 1, g.th.text)
	g.drawPlanningText(dst, data.Render(g.db.Tr(v.Header), g.st.Env()), 240, 12, sw-264, sizeS, 1, g.th.text)
}
