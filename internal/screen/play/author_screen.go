package play

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/base/expr"
	"github.com/nao1215/gozen/internal/rules/event"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// authorScreen presents an authored list without requiring a custom Go screen.
type authorScreen struct {
	destination string
	// homed is set once the running action asked for a home screen (goingHome).
	homed bool
	// carry is the entry picked up to be dropped, press the pointer pressed on
	// a carried, dropped or peeked entry, and peek the entry whose panel is open.
	carry *carried
	press *pointerPress
	peek  *peekOpen
	// ghostImg holds the dragged copy of a card between frames.
	ghostImg *ebiten.Image

	chart            workChart
	args             map[string]any
	timelineContext  *authorScreen
	timelineActivity string
	candidatePage    int
	calendarMonth    string
	// scroll is the residents' table scrolled by the wheel or a finger; the timelines
	// share its finger.
	scroll             rowScroll
	residentSort       int
	residentDescending bool
	residentPage       int
	timelineTop        int
	timelineFocus      int
	timelineAssignee   string
	timelineExpanded   string
	timelineZoom       bool
	target             string
	focus              int
	// ringAt is the carousel member in the middle; ringAnim eases a turn.
	ringAt   int
	ringAnim float64
	// keyed is set once the keys moved the focus: only then is the focus ring
	// of a button, ticket card or carousel drawn, so a mouse player never sees
	// the first control as if it were selected.
	keyed bool
	// inboxArmed is set by a script pick of the chat notification; OK opens it.
	inboxArmed   bool
	popup        *list
	popupIndices []int
	popupArmed   bool
	name         string
	title        string
	indices      []int
	running      bool
	parent       *event.Runner
	request      event.Request
	origin       string
	entries      []data.ScreenEntry
	rows         []event.ScreenRow
	list         *list
	done         func()
}

// rowScroll is a run of rows that scrolls: the first row shown and the finger dragging them.
type rowScroll struct {
	top  int
	drag swipe
}

// openAuthorScreen owns the depth check for both event and navigation opens.
func (g *Game) openAuthorScreen(name string, done func(), args map[string]any) {
	depth := 0
	for _, view := range g.stack {
		if _, ok := view.(*authorScreen); ok {
			depth++
		}
	}
	if depth >= event.MaxScreenDepth {
		g.events.Log(fmt.Errorf("author screens nested too deep: limit %d", event.MaxScreenDepth))
		if done != nil {
			done()
		}
		return
	}
	s := newAuthorScreen(g, name, done, args)
	g.push(s)
	if q, ok := g.req.(event.Open); !ok || q.Screen != name {
		g.screenTrace.blocked = true
	}
	g.recordScreen(s)
}

func newAuthorScreen(g *Game, name string, done func(), args map[string]any) *authorScreen {
	if len(args) == 0 {
		name = g.flowOpen(name)
	}
	s := &authorScreen{name: name, done: done, args: maps.Clone(args)}
	def := g.db.Game.Screens[name]
	var items []listItem
	if def != nil {
		s.title = data.Render(g.db.Tr(def.Title), s.env(g))
	}
	var report func(error)
	if g.events != nil {
		report = g.events.Log
	}
	build := func() {
		for index, entry := range event.ScreenEntries(g.db, name, s.env(g), report) {
			entry.Icon = data.Render(entry.Icon, entry.Env(s.env(g)))
			entry.Text = data.Render(g.db.Tr(entry.Text), entry.Env(s.env(g)))
			renderEntryParts(g, &entry.ScreenEntry, entry.Env(s.env(g)))
			s.indices = append(s.indices, index)
			s.entries = append(s.entries, entry.ScreenEntry)
			s.rows = append(s.rows, entry)
			items = append(items, listItem{label: entry.Label})
		}
	}
	// Building the entries only reads the state (ScreenEntries evaluates their
	// when, each and text), so the planning values are made once for it.
	if g.st != nil {
		g.st.Reading(build)
	} else {
		build()
	}
	s.warmPictures(g)
	sw, sh := g.screen()
	s.list = newList(items, 24, 88, sw*0.32, max(1, int((sh-140)/44)))
	if s.candidateList(g) {
		s.list.w, s.list.rowH, s.list.rows = sw*.56-36, 64, max(1, int((sh-140)/64))
	}
	return s
}

// warmPictures decodes the pictures the entries show (icons, busts, pictures)
// side by side before the first frame draws them: decoded one by one in the
// draw, a screen of standing pictures stalled for half a second when it opened.
func (s *authorScreen) warmPictures(g *Game) {
	var paths []string
	for _, e := range s.entries {
		paths = append(paths, e.Icon, e.Picture)
		if e.Bust != nil {
			paths = append(paths, e.Bust.Picture)
		}
	}
	paths = slices.DeleteFunc(paths, func(p string) bool {
		ext := strings.ToLower(filepath.Ext(p))
		return ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp"
	})
	if len(paths) > 1 {
		g.pics.warm(paths...)
	}
}

// env keeps frozen screen bindings separate from the opener and current state.
func (s *authorScreen) env(g *Game) expr.Env {
	return expr.Chain{expr.Map{"arg": expr.Map(s.args)}, g.st.Env()}
}

// Plain authored lists may bind entries to public planning portraits and fields.
// The existing list owns selection, scrolling, hit regions and confirmation.
func (s *authorScreen) candidateList(g *Game) bool {
	d := g.db.Game.Screens[s.name]
	return d != nil && d.Kind == "" && !d.View.Fullscreen && d.View.Portrait != "" && g.db.Game.Planning != nil
}

func (s *authorScreen) candidateInfo(g *Game, e data.ScreenEntry) (string, string) {
	v := g.db.Game.Screens[s.name].View
	values := g.st.EntityValues(e.Target.ID, false)
	portrait, ok := values[v.Portrait].(string)
	if !ok {
		portrait = ""
	}
	var fields []string
	for _, f := range v.Fields {
		if value := values[data.Render(f.Key, s.env(g))]; value != nil {
			fields = append(fields, data.Render(g.db.Tr(f.Label), s.env(g))+" "+fmt.Sprint(value))
		}
	}
	return portrait, strings.Join(fields, " · ")
}

func (s *authorScreen) drawCandidates(g *Game, dst *ebiten.Image) {
	l := s.list
	g.traceList(l)
	g.window(dst, l.x, l.y, l.w, l.height())
	for _, i := range l.shown() {
		r := l.rowRect(i)
		x, y := float64(r.Min.X), float64(r.Min.Y)
		tint := g.th.text
		if i == l.sel {
			tint = g.th.accent
			rect(dst, x+4, y+4, 3, l.rowH-8, tint)
		}
		portrait, summary := s.candidateInfo(g, s.entries[i])
		g.planningFace(dst, portrait, x+34, y+30, 22)
		g.drawPlanningText(dst, s.entries[i].Label, x+68, y+2, l.w-80, 18, 1, tint)
		g.drawPlanningText(dst, summary, x+68, y+30, l.w-80, 15, 1, g.th.dim)
	}
}

// A candidate screen opened from a grouped lane keeps the entire date axis
// visible beside a 360-logical-pixel pane. The parent remains input-inactive.
func (s *authorScreen) layoutTimelineCandidates(g *Game) {
	sw, sh := g.screen()
	s.list.x, s.list.y, s.list.w = sw-348, 280, 336
	s.list.rowH, s.list.rows = 64, max(1, int((sh-296)/64))
	s.list.scroll()
}

func (s *authorScreen) drawTimelineCandidates(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	parent := s.timelineContext
	fill(dst, color.NRGBA{30, 34, 40, 255})
	if g.db.Game.Screens[parent.name].View.Light {
		// The chart under the candidates keeps the light surface it came from.
		light := g.lightPalette()
		previous := g.th
		defer func() { g.th = previous }()
		g.th.window, g.th.text, g.th.dim, g.th.accent = light.window, light.text, light.dim, light.accent
		g.th.border, g.th.shadow = color.NRGBA{}, color.NRGBA{}
		fill(dst, light.paper)
	}
	parent.drawPlanningHeader(g, dst)
	cx, cy, cw, ch := g.surfaceChart()
	parent.drawGroupedTimeline(g, dst, cx, cy, cw-360, ch)
	g.window(dst, sw-360, 48, 360, sh-48)
	g.drawPlanningText(dst, s.title, sw-348, 60, 336, 18, 2, g.th.accent)
	if len(s.entries) > 0 {
		lines, pages := s.candidateTextPage()
		for i, line := range lines {
			g.text(dst, line, sw-348, 108+float64(i)*gfx.LineHeight(15), 15, g.th.text)
		}
		if pages > 1 {
			for i, r := range s.candidatePageButtons(g) {
				r, g.th.window = g.controlMaterial(r)
				g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
				label := "<"
				if i == 1 {
					label = ">"
				}
				g.text(dst, label, float64(r.Min.X)+12, float64(r.Min.Y)+2, 15, g.th.accent)
			}
			g.text(dst, fmt.Sprintf("%d / %d", s.candidatePage+1, pages), sw-270, 246, 15, g.th.dim)
		}
		s.drawCandidates(g, dst)
	}
	g.drawPlanningText(dst, data.Render(g.db.Tr(g.db.Game.Screens[s.name].View.Footer), s.env(g)), sw-348, sh-32, 336, 15, 1, g.th.dim)
	g.window(dst, 0, sh-108, sw-360, 108)
	for _, lane := range parent.timelineLanes(g) {
		if lane.item.Activity == s.timelineActivity {
			g.drawPlanningText(dst, timelineLaneDescription(g, lane), 24, sh-96, sw-408, 15, 4, g.th.text)
			break
		}
	}
}

// Keep full authored explanations readable without displacing the candidate list.
// Paging is view state only; it never selects or activates a candidate.
func (s *authorScreen) candidateTextPage() ([]string, int) {
	if len(s.entries) == 0 {
		return nil, 1
	}
	lines := gfx.Wrap(s.entries[s.list.sel].Text, 15, 336)
	pages := max(1, (len(lines)+5)/6)
	s.candidatePage = min(max(0, s.candidatePage), pages-1)
	start := s.candidatePage * 6
	return lines[start:min(start+6, len(lines))], pages
}

func (s *authorScreen) candidatePageButtons(g *Game) [2]image.Rectangle {
	sw, _ := g.screen()
	return [2]image.Rectangle{
		image.Rect(int(sw)-348, 244, int(sw)-300, 276),
		image.Rect(int(sw)-60, 244, int(sw)-12, 276),
	}
}

func (s *authorScreen) updateCandidatePage(g *Game) bool {
	if s.timelineContext == nil || g.in.cancel() {
		return false
	}
	_, pages := s.candidateTextPage()
	sw, _ := g.screen()
	delta := 0
	if g.in.pressedKey(btnLeft) {
		delta = -1
	}
	if g.in.pressedKey(btnRight) {
		delta = 1
	}
	if pages > 1 && g.in.inspect(g.in.click) {
		for i, r := range s.candidatePageButtons(g) {
			if g.in.mouse.In(r) {
				g.in.query(true)
				delta = 2*i - 1
			}
		}
	}
	if g.in.wheel != 0 && g.in.mouse.In(image.Rect(int(sw)-348, 108, int(sw)-12, 276)) {
		delta = 1
		if g.in.wheel > 0 {
			delta = -1
		}
	}
	if delta == 0 {
		return false
	}
	s.candidatePage = min(max(0, s.candidatePage+delta), pages-1)
	return true
}

func (s *authorScreen) modal() bool   { return !s.running }
func (s *authorScreen) overlay() bool { return s.running }

func (s *authorScreen) liveKeep(g *Game) scene { return s.refreshed(g) }

func (s *authorScreen) refreshed(g *Game) *authorScreen {
	n := newAuthorScreen(g, s.name, s.done, s.args)
	n.timelineContext, n.timelineActivity = s.timelineContext, s.timelineActivity
	if n.timelineContext != nil {
		n.layoutTimelineCandidates(g)
	}
	n.target = s.target
	n.destination = s.destination
	n.ghostImg = s.ghostImg
	n.keepCarry(g, s)
	n.focus = s.focus
	n.ringAt, n.ringAnim, n.keyed = s.ringAt, s.ringAnim, s.keyed
	n.keepRing(s.ringKey())
	n.calendarMonth = s.calendarMonth
	n.scroll.top = s.scroll.top
	n.residentSort, n.residentDescending, n.residentPage = s.residentSort, s.residentDescending, s.residentPage
	n.timelineTop = s.timelineTop
	n.timelineFocus = s.timelineFocus
	n.timelineAssignee = s.timelineAssignee
	n.timelineExpanded = s.timelineExpanded
	n.timelineZoom = s.timelineZoom
	n.list.sel = min(s.list.sel, max(0, len(n.entries)-1))
	retained := false
	if s.list.sel < len(s.indices) {
		for i, index := range n.indices {
			if index == s.indices[s.list.sel] && maps.Equal(n.rows[i].Args, s.rows[s.list.sel].Args) {
				n.list.sel, retained = i, true
				break
			}
		}
	}
	if n.timelineContext != nil && retained {
		n.candidatePage = s.candidatePage
		n.candidateTextPage()
	}
	n.running, n.origin = s.running, s.origin
	n.list.scroll()
	return n
}

// backBounds reserves a mouse-accessible close action for authored lists.
func (s *authorScreen) backBounds(g *Game) image.Rectangle {
	sw, _ := g.screen()
	return image.Rect(int(sw)-120, 24, int(sw)-36, 64)
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
		g.inboxShown = nil
		g.boundary()
		// The resumed opener already crossed an open-screen boundary. Its
		// runner must not inherit the completed child's replay snapshot: F5
		// would otherwise restore state from before the entry action.
		g.evCmds, g.evSnap, g.evRand = nil, nil, nil
		g.evName, g.evLines, g.evArrival = "", 0, false
		g.evAnswers, g.evHist, g.evOpened = nil, nil, nil
		if g.st.Location != s.origin {
			g.leave(s.done)
			return
		}
		if s.destination != "" {
			if s.returnTo(g, s.destination) {
				return
			}
			*s = *newAuthorScreen(g, s.destination, s.done, nil)
		} else if s.flowGivesWay(g) {
			*s = *newAuthorScreen(g, s.name, s.done, nil)
		} else {
			s.refresh(g)
		}
		g.recordScreen(s)
		return
	}
	if !g.guideOn(s) && (g.in.pressedKey(btnSave) || g.in.pressedKey(btnLoad)) {
		menu := newSaveMenu(g, g.in.pressedKey(btnSave), func() {})
		menu.inPlay = true
		g.push(menu)
		return
	}
	if g.db.Game.Screens[s.name].View.Fullscreen {
		// A step screen shown again (a screen over it closed) that would no
		// longer open as itself gives way to the step that opens.
		if !s.running && s.flowGivesWay(g) {
			*s = *newAuthorScreen(g, s.name, s.done, nil)
			g.recordScreen(s)
			return
		}
		if s.enterFlow(g) {
			return
		}
		if g.updateGuide(s) {
			return
		}
		name, running := s.name, s.running
		s.updateSurface(g)
		g.afterGuidedUpdate(s, name, running)
		return
	}
	if g.db.Game.Screens[s.name].Kind == "timeline" && len(s.entries) == 0 {
		_, sh := g.screen()
		s.scrollTimeline(g, sh-214)
	}
	if s.updateCandidatePage(g) {
		return
	}
	oldSelection := s.list.sel
	chose, cancel := s.list.update(g)
	if s.list.sel != oldSelection {
		s.candidatePage = 0
	}
	if cancel || (g.in.mouse.In(s.backBounds(g)) && g.in.clicked()) {
		g.recordScreenAnswer(-1)
		if s.leave(g) {
			return
		}
		g.leave(s.done)
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

func (s *authorScreen) activate(g *Game) bool { return s.activateWith(g, nil) }

// activateWith runs the selected entry with extra bindings added to its row's
// arguments (a drop's carried value).
func (s *authorScreen) activateWith(g *Game, extra map[string]any) bool {
	def := g.db.Game.Screens[s.name]
	if def == nil || s.list.sel >= len(s.indices) {
		s.refresh(g)
		return false
	}
	index := s.indices[s.list.sel]
	if index >= len(def.Entries) {
		s.refresh(g)
		return false
	}
	g.recordScreenAnswer(s.list.sel)
	entry := def.Entries[index]
	row := s.rows[s.list.sel]
	available, err := row.Available(g.db, entry, s.env(g))
	if err != nil || !available {
		if err != nil {
			g.screenTrace.blocked = true
			if g.events != nil && g.events.Log != nil {
				g.events.Log(err)
			}
		} else {
			g.emit("screen_refused", traceLine{"screen": s.name, "entry": index})
		}
		s.refresh(g)
		return true
	}
	if len(entry.Do) == 0 && entry.Destination == "" {
		return false
	}
	s.destination, s.homed = entry.Destination, false
	// Suspend the opener so entry commands run immediately and have their own
	// named save cursor. Closing the screen resumes the opener after its open.
	args := row.Args
	if len(extra) > 0 {
		args = maps.Clone(args)
		if args == nil {
			args = map[string]any{}
		}
		maps.Copy(args, extra)
	}
	s.startEvent(g, fmt.Sprintf("screen:%s:%d", s.name, index), args)
	g.emit("screen_action", traceLine{"screen": s.name, "entry": index})
	return true
}

// facesSquare reports whether this screen cuts faces as rounded squares: its
// view's face_shape, else ui.face_shape. A video call keeps round faces.
func (s *authorScreen) facesSquare(g *Game) bool {
	def := g.db.Game.Screens[s.name]
	if def == nil || def.View.Call != nil {
		return false
	}
	shape := def.View.FaceShape
	if shape == "" {
		shape = g.db.Game.UI.FaceShape
	}
	return shape == "square"
}

func (s *authorScreen) draw(g *Game, dst *ebiten.Image) {
	// Drawing only reads the state: the planning values its expressions read
	// are made once for the frame.
	if g.st != nil {
		g.st.Reading(func() { s.drawRead(g, dst) })
		return
	}
	s.drawRead(g, dst)
}

func (s *authorScreen) drawRead(g *Game, dst *ebiten.Image) {
	previous := g.faceSquare
	g.faceSquare = s.facesSquare(g)
	defer func() { g.faceSquare = previous }()
	if s.timelineContext != nil {
		s.drawTimelineCandidates(g, dst)
		return
	}
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
	if g.hasMaterial() {
		// With ui.material a plain list is the same glass desk as the full-screen
		// views: the wallpaper and light panels instead of a dark veil.
		light := g.lightPalette()
		previous := g.th
		defer func() { g.th = previous }()
		g.th.window, g.th.text, g.th.dim, g.th.accent = withAlpha(light.window, 0xd8), light.text, light.dim, light.accent
		g.th.border, g.th.shadow = color.NRGBA{}, color.NRGBA{}
		fill(dst, light.paper)
		if !g.drawMaterialWallpaper(dst) {
			g.prepareMaterial(dst, "")
		}
		g.prepareMaterial(dst, "")
	} else {
		fill(dst, color.NRGBA{0, 0, 0, 220})
	}
	g.drawLinesFit(dst, s.title, 36, 28, sw-230, sizeL, 1, g.th.accent)
	r := s.backBounds(g)
	tint := g.th.text
	if g.in.mouse.In(r) {
		tint = g.th.cursor
	}
	g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
	g.text(dst, "←", float64(r.Min.X)+28, float64(r.Min.Y)+4, sizeM, tint)
	if len(s.entries) > 0 {
		if s.candidateList(g) {
			s.drawCandidates(g, dst)
		} else {
			s.list.draw(g, dst, true)
		}
	}
	x, w := sw*0.36, sw*0.64-36
	if s.candidateList(g) {
		x, w = sw*.56, sw*.44-36
	}
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
	if def.View.Window != "" {
		return ids, islandCells(24, sh*292/720, sw*.72-36, sh*310/720, len(ids))
	}
	return ids, islandCells(12, sh*268/720, sw*.72-24, sh*334/720, len(ids))
}

// entryPictureHeight leaves two caption lines below a bounded entry picture.
func entryPictureHeight(r image.Rectangle) float64 {
	return max(0, float64(r.Dy()-12-entryCaptionHeight))
}

// entryCaptionHeight is the label and two text lines below a bounded picture.
const entryCaptionHeight = 82

func entryBounds(e data.ScreenEntry) image.Rectangle {
	if len(e.Bounds) != 4 {
		return image.Rectangle{}
	}
	return image.Rect(e.Bounds[0], e.Bounds[1], e.Bounds[0]+e.Bounds[2], e.Bounds[1]+e.Bounds[3])
}

func (s *authorScreen) openTarget(g *Game, id string, r image.Rectangle) bool {
	s.target = id
	s.popupArmed = false
	if g.db.Game.Screens[s.name].Kind == "entities" {
		return true
	}
	s.popupIndices = nil
	var items []listItem
	for i, e := range s.entries {
		if e.Target.ID == id {
			s.popupIndices = append(s.popupIndices, i)
			items = append(items, listItem{label: e.Label})
		}
	}
	if len(items) == 0 {
		s.target = ""
		s.popup = nil
		return false
	}
	sw, sh := g.screen()
	rows := max(1, min(6, len(items), int((sh-128)/44)))
	s.popup = newList(items, max(0, min(float64(r.Min.X), sw-380)), max(40, min(float64(r.Max.Y), sh-44*float64(rows)-32)), 360, rows)
	if g.db.Game.Screens[s.name].View.Fullscreen {
		// Anchor beside the selected seat and leave the authored sidebar clear.
		right := sw*.72 - 12
		s.popup.w = min(360, right-24)
		s.popup.x = float64(r.Max.X) + 8
		if s.popup.x+s.popup.w > right {
			s.popup.x = max(12, float64(r.Min.X)-s.popup.w-8)
		}
		s.popup.x = min(s.popup.x, right-s.popup.w)
		s.popup.y = max(88, min(float64(r.Min.Y)+40, sh-100-144-s.popup.height()))
	}
	s.popup.size = 22
	return true
}

// goingHome turns opening a home screen from the action of the screen on top
// into going to it once that action ends: the screen on top takes the home as
// its destination (returning to it when it lies under) instead of stacking a
// second copy over a screen whose action would then wait forever.
func (g *Game) goingHome(r event.Open) bool {
	def := g.db.Game.Screens[r.Screen]
	if def == nil || !def.Home || len(r.Args) > 0 || len(g.stack) < 2 {
		return false
	}
	top, ok := g.stack[len(g.stack)-1].(*authorScreen)
	if !ok || !top.running {
		return false
	}
	// The first home asked for wins (an ending's screen before the day's), and
	// the home's own action reopening it only refreshes it when the action ends.
	if !top.homed {
		top.homed = true
		top.destination = r.Screen
		if top.name == r.Screen {
			top.destination = ""
		}
	}
	return true
}

// smallTile is a portrait entry too low for the profile layout (face, then
// several lines): a roster tile of a face and a name.
func smallTile(r image.Rectangle) bool { return r.Dy() < 200 }

// returnTo closes this screen when the screen directly under it is the named
// parameterless screen, or is running the action that opened this one, so
// going "to" the opener returns to it instead of stacking a second copy and
// leaving the opener's action waiting. It reports whether it closed the screen.
func (s *authorScreen) returnTo(g *Game, name string) bool {
	if len(g.stack) < 2 {
		return false
	}
	under, ok := g.stack[len(g.stack)-2].(*authorScreen)
	// The opener directly under, or a screen whose action opened this one and
	// waits for it: closing hands control back, and the opener goes on. A
	// running opener gives control back only for itself or a home screen, so a
	// phone's card can lead to its profile while the home's action still waits.
	if !ok || !event.ReturnsTo(g.db, name, under.name, len(under.args), under.running) {
		return false
	}
	g.leave(s.done)
	switch {
	case under.running:
	case under.flowGivesWay(g):
		// A step screen under this one that would no longer open as itself
		// (a held step finished meanwhile) gives way to the step that opens.
		*under = *newAuthorScreen(g, under.name, under.done, nil)
	default:
		under.refresh(g)
	}
	return true
}

// overScreen reports whether another authored screen lies directly under this
// one: closing then returns to it, so back is not needed.
func (s *authorScreen) overScreen(g *Game) bool {
	if len(g.stack) < 2 {
		return false
	}
	_, ok := g.stack[len(g.stack)-2].(*authorScreen)
	return ok
}

// leave follows the screen's back or home instead of closing it. It reports
// whether the screen stays on the stack.
func (s *authorScreen) leave(g *Game) bool {
	def := g.db.Game.Screens[s.name]
	switch {
	case def == nil:
		return false
	case def.Home:
		g.push(newSysMenu(g))
		return true
	case def.Back != "" && g.db.Game.Screens[def.Back] != nil && !s.overScreen(g):
		*s = *newAuthorScreen(g, def.Back, s.done, nil)
		g.recordScreen(s)
		return true
	}
	return false
}

func (s *authorScreen) closeSurface(g *Game) {
	if s.leave(g) {
		return
	}
	g.leave(s.done)
}

func (s *authorScreen) updateSurface(g *Game) {
	if g.updateInboxToast(s) {
		return
	}
	if s.popup != nil {
		r := image.Rect(int(s.popup.x), int(s.popup.y), int(s.popup.x+s.popup.w), int(s.popup.y+s.popup.height()))
		if !g.in.mouse.In(r) && g.in.clicked() {
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
			g.in.query(s.activate(g))
		}
		return
	}
	if g.in.cancel() {
		if s.putDown() {
			return
		}
		s.closeSurface(g)
		return
	}
	// An idle surface has no input-dependent geometry to resolve. Keep tick,
	// script, event and animation processing in the normal Update path.
	if s.press == nil && s.carry == nil && s.peek == nil && s.ringAnim == 0 && !g.in.surfaceInput() {
		return
	}
	if s.updateFlowClick(g) {
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
	actions := s.surfaceActions()
	flow := s.flowStops(g)
	n := len(actions) + len(ids) + len(flow)
	if n == 0 {
		return
	}
	// An action can remove itself or another focus stop during the refresh.
	// Keep confirmation on a remaining control even before an arrow is pressed.
	s.focus = max(0, min(s.focus, n-1))
	if s.updateCarousel(g, actions) {
		return
	}
	step := func(d int) {
		for range n {
			s.focus = (s.focus + d + n) % n
			k := s.focus - len(actions) - len(ids)
			if !s.ringSkipped(s.focus, actions) && (k < 0 || flow[k].enabled) {
				return
			}
		}
	}
	if g.in.down() {
		step(1)
		s.keyed = true
	}
	if g.in.up() {
		step(-1)
		s.keyed = true
	}
	if s.updatePointer(g, actions, cells) {
		return
	}
	if g.in.inspect(g.in.click) {
		for i, r := range cells {
			if g.in.mouse.In(r) {
				s.focus = len(actions) + i
				g.in.query(s.openTarget(g, ids[i], r))
				return
			}
		}
		for j, i := range actions {
			if s.entries[i].Carousel == "" && !pointerPart(s.entries[i]) && g.in.mouse.In(entryBounds(s.entries[i])) {
				s.focus = j
				s.list.sel = i
				g.in.query(s.activate(g))
				return
			}
		}
	}
	g.in.receivePick(s)
	confirmable := true
	if k := s.focus - len(actions) - len(ids); k >= 0 && k < len(flow) {
		confirmable = flow[k].enabled
	}
	if confirmable && g.in.inspect(g.in.pressed[btnOK]) {
		if s.focus < len(actions) {
			if pointerPart(s.entries[actions[s.focus]]) {
				g.in.query(s.confirmPart(g, actions[s.focus]))
				return
			}
			s.list.sel = actions[s.focus]
			g.in.query(s.activate(g))
		} else if k := s.focus - len(actions) - len(ids); k >= 0 && k < len(flow) {
			g.in.query(s.goFlow(g, flow[k]))
		} else if s.focus-len(actions) < len(ids) {
			i := s.focus - len(actions)
			g.in.query(s.openTarget(g, ids[i], cells[i]))
		}
	}
}

func (s *authorScreen) drawSurface(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	def := g.db.Game.Screens[s.name]
	v := def.View
	light := g.lightPalette()
	if v.Light {
		previous := g.th
		defer func() { g.th = previous }()
		g.th.window, g.th.text, g.th.dim, g.th.accent = light.window, light.text, light.dim, light.accent
		g.th.border = color.NRGBA{}
		g.th.shadow = color.NRGBA{}
	}
	fill(dst, color.NRGBA{30, 34, 40, 255})
	if v.Light {
		fill(dst, light.paper)
	}
	if v.Background != "" {
		g.materialBackground(dst, v.Background)
	} else if !v.Light || !g.drawMaterialWallpaper(dst) {
		g.prepareMaterial(dst, "")
	}
	if def.Kind == "workspace" {
		g.drawPhaseWash(dst)
	}
	if def.Kind == "ensemble" && v.Call != nil {
		s.drawCall(g, dst)
		g.drawPlanningText(dst, data.Render(g.db.Tr(v.Footer), s.env(g)), 32, sh-36, sw-64, 16, 1, color.White)
		return
	}
	if v.Window != "" {
		g.desktopFrame(dst, sh-16, 38, color.NRGBA{})
		g.drawPlanningText(dst, data.Render(g.db.Tr(v.Window), s.env(g)), 36, 26, sw-200, 15, 1, g.chrome().title)
	}
	fx, fy, fw, fh := planningFrame(sw, sh)
	s.drawPlanning(g, dst, def.Kind, fx, fy, fw, fh)
	s.drawPlanningHeader(g, dst)
	g.prepareMaterial(dst, "")
	ring := s.ringMembers()
	switch {
	case v.Band == "none":
		// The author left out the bottom strip.
	case len(ring) > 0:
		// The carousel is the screen's band of actions; it draws its own.
	case v.Window != "":
		g.surfaceWindow(dst, 24, sh-102, sw-48, 78, "control")
	default:
		g.surfaceWindow(dst, 12, sh-102, sw-24, 94, "control")
	}
	footerWidth := sw * .70
	if tabs := s.tabBounds(g); len(tabs) > 0 {
		footerWidth = float64(tabs[0].Min.X) - 40
	}
	for _, e := range s.entries {
		r := entryBounds(e)
		if e.Target.ID == "" && !r.Empty() && float64(r.Min.Y) >= sh-108 {
			footerWidth = min(footerWidth, float64(r.Min.X)-40)
		}
	}
	footer := data.Render(g.db.Tr(v.Footer), s.env(g))
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
	ringFocused := false
	for entryIndex, e := range s.entries {
		if e.Target.ID != "" || len(e.Bounds) != 4 {
			continue
		}
		actionable := focusStop(e)
		focused := actionable && j == s.focus
		if g.controlFrame() != nil && actionable && e.Carousel == "" {
			r := entryBounds(e)
			reason := e.Reason
			enabled := !s.unavailable(g, entryIndex) && s.popup == nil
			if s.popup != nil {
				reason = "modal"
			}
			g.traceControl(e.Label, roundRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())), enabled, focused && s.keyed, reason)
		}
		switch {
		case e.Carousel != "" && actionable:
			ringFocused = ringFocused || focused
		case e.Ticket != nil:
			s.drawTicket(g, dst, entryIndex, focused && s.keyed)
		case e.Style == "button" && actionable:
			s.drawButton(g, dst, entryIndex, focused && s.keyed)
		default:
			s.drawPlainEntry(g, dst, entryIndex, focused)
		}
		if actionable {
			j++
		}
	}
	s.drawCarried(g, dst)
	s.drawTicketLinks(g, dst)
	if len(ring) > 0 {
		s.drawCarousel(g, dst, ringFocused && s.keyed)
	}
	_, cells := s.surfaceTargets(g)
	if i := s.focus - j; def.Kind != "entities" && i >= 0 && i < len(cells) {
		r := cells[i]
		rect(dst, float64(r.Min.X), float64(r.Max.Y-3), float64(r.Dx()), 3, g.th.accent)
	}
	if s.popup != nil {
		// A solid backing keeps underlying status text from showing through.
		rect(dst, s.popup.x, s.popup.y-40, s.popup.w, s.popup.height()+40, withAlpha(g.th.window, 255))
		g.window(dst, s.popup.x, s.popup.y-40, s.popup.w, 42)
		g.drawPlanningText(dst, g.db.Game.Planning.Entities[s.target].Name, s.popup.x+12, s.popup.y-30, s.popup.w-24, sizeS, 1, g.th.accent)
		s.popup.draw(g, dst, true)
		if s.popupArmed {
			e := s.entries[s.popupIndices[s.popup.sel]]
			y := s.popup.y + s.popup.height()
			rect(dst, s.popup.x, y, s.popup.w, 144, withAlpha(g.th.window, 255))
			g.window(dst, s.popup.x, y, s.popup.w, 144)
			g.drawPlanningText(dst, e.Text, s.popup.x+12, y+12, s.popup.w-24, 18, 5, g.th.text)
		}
	}
	s.drawTabs(g, dst)
	s.drawFlow(g, dst, s.focus-j-len(cells))
}

// drawPlainEntry draws an ordinary bounded card: a plate, a tone, a face, a
// picture, the label and the text. focused tints the label of a focus stop.
func (s *authorScreen) drawPlainEntry(g *Game, dst *ebiten.Image, entryIndex int, focused bool) {
	e := s.entries[entryIndex]
	r := entryBounds(e)
	originalWindow := g.th.window
	tones := g.tonePalette()
	if e.Tone == "note" {
		g.th.window = tones.note
	}
	if e.Tone == "primary" {
		g.th.window = tones.primary
	}
	bare := e.Bust != nil && e.Bust.Bare
	if e.Style != "plate" && !bare {
		r, g.th.window = g.controlMaterial(r)
	}
	if bare {
		// A bare standing picture: nothing under it but the screen.
	} else if e.Style == "plate" {
		g.drawPlate(dst, r)
	} else if r.Dy() > 32 {
		if g.db.Game.Screens[s.name].Kind == "workspace" {
			cardColor := color.NRGBA{245, 248, 250, 245}
			if r.Dy() > 400 {
				cardColor = color.NRGBA{224, 233, 242, 245}
			}
			if e.Tone == "primary" {
				cardColor = tones.primaryCard
			}
			if !g.materialSurface(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), cardColor, "card") {
				gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 8, cardColor)
				gfx.StrokeRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 8, 1, color.NRGBA{190, 201, 214, 255})
			}
		} else {
			g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		}
	}
	g.th.window = originalWindow
	// The dot and the sign are drawn last, over a bust or picture that may cover
	// the corner they mark (a full-width face card would otherwise hide them).
	defer func() {
		if e.Dot != "" {
			drawEntryDot(dst, r, e.Dot)
		}
		g.drawEntrySign(dst, r, e.Sign, e.Dot != "")
	}()
	tint := g.th.text
	if e.Style == "plate" {
		tint = g.th.dim
	}
	if focused {
		tint = g.th.accent
	}
	if e.Tone == "primary" && g.db.Game.Screens[s.name].Kind == "workspace" {
		tint = tones.primaryText
	}
	left := 10
	bottomIcon := e.Bust == nil && !e.Portrait && e.Icon != "" && r.Dx() < 300 && r.Dy() >= 160 && r.Dy() < 400
	if e.Bust != nil {
		left = s.drawEntryBust(g, dst, entryIndex, r)
	} else if e.Icon != "" {
		radius := min(36.0, float64(r.Dy()-12)/2)
		faceY := float64(r.Min.Y) + radius + 8
		faceX := float64(r.Min.X) + radius + 10
		if e.Portrait {
			radius = max(1, min(float64(r.Dx()-32)/2, float64(r.Dy()-160)/2))
			faceX = float64(r.Min.X+r.Max.X) / 2
			faceY = float64(r.Min.Y) + radius + 16
			if smallTile(r) {
				radius = max(1, min(float64(r.Dx()-12)/2, float64(r.Dy()-42)/2))
				faceY = float64(r.Min.Y) + radius + 4
			}
		}
		if bottomIcon {
			faceY = float64(r.Max.Y) - radius - 10
		}
		g.planningFace(dst, e.Icon, faceX, faceY, radius)
		if muted, err := e.Muted.Bool(s.rows[entryIndex].Env(s.env(g))); e.Muted.Set() && err == nil && muted {
			g.fillFace(dst, faceX, faceY, radius, color.NRGBA{166, 174, 181, 180})
			g.text(dst, "−", faceX+radius*.45, faceY+radius*.35, 18, g.th.text)
		}
		if !bottomIcon && !e.Portrait {
			left = int(radius*2) + 20
		}
	}
	textY := float64(r.Min.Y + 6)
	if e.Meter != nil {
		defer s.drawEntryMeter(g, dst, entryIndex, r, float64(r.Min.X+left))
	}
	if e.Bust != nil {
		textY += 6
		if e.Bust.Height > 0 {
			area, _, _ := bustLayout(r, e.Bust)
			textY = float64(area.Max.Y + 4)
		}
		if bare && (e.Bust.Height > 0 || e.Bust.Width >= 1) {
			// The picture is the whole entry; its words belong elsewhere.
			return
		}
	} else if e.Portrait && smallTile(r) {
		// A small portrait tile: the face fills the top, the label and one
		// short line sit under it, centered.
		face := min(float64(r.Dx()-12), float64(r.Dy()-42))
		label := g.th.text
		if focused {
			label = g.th.accent
		}
		g.drawPlanningTextCentered(dst, e.Label, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y)+face+6, float64(r.Dx()-6), 15, label)
		if e.Text != "" {
			g.drawPlanningTextCentered(dst, e.Text, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y)+face+23, float64(r.Dx()-6), 13, g.th.dim)
		}
		return
	}
	if e.Portrait && e.Bust == nil {
		textY += min(float64(r.Dx()-32), float64(r.Dy()-160)) + 22
	}
	if e.Picture != "" {
		// A bounded picture fills the card above its caption, cropped to cover.
		ph := entryPictureHeight(r)
		if e.PictureFit == "contain" && strings.TrimSpace(e.Label) == "" && strings.TrimSpace(e.Text) == "" {
			// A contained picture without words takes the whole entry.
			ph = float64(r.Dy() - 12)
		}
		if e.PictureFit == "contain" {
			g.drawPictureContain(dst, e.Picture, float64(r.Min.X+6), float64(r.Min.Y+6), float64(r.Dx()-12), ph, 1)
		} else {
			g.drawPicture(dst, data.Picture{File: e.Picture}, float64(r.Min.X+6), float64(r.Min.Y+6), float64(r.Dx()-12), ph, 1)
		}
		textY = float64(r.Min.Y+6) + ph + 6
	}
	g.drawPlanningText(dst, e.Label, float64(r.Min.X+left), textY, float64(r.Dx()-left-10), sizeS, 1, tint)
	if e.Text != "" && r.Dy() >= 48 {
		body := e.Text
		if bottomIcon {
			lines := strings.Split(body, "\n")
			g.drawPlanningText(dst, strings.TrimSpace(lines[len(lines)-1]), float64(r.Min.X+72), float64(r.Max.Y-44), float64(r.Dx()-82), 16, 1, g.th.text)
			body = strings.Join(lines[:len(lines)-1], "\n")
		}
		lines := max(1, (r.Dy()-46)/24)
		if e.Picture != "" {
			lines = max(1, int((float64(r.Max.Y)-textY-24)/24))
		}
		g.drawPlanningText(dst, body, float64(r.Min.X+left), textY+24, float64(r.Dx()-left-10), 16, lines, g.th.text)
	}
}

func (s *authorScreen) tabBounds(g *Game) []image.Rectangle {
	tabs := g.db.Game.Screens[s.name].View.Tabs
	sw, sh := g.screen()
	width := min(152.0, (sw-32)/float64(max(1, len(tabs))))
	bounds := make([]image.Rectangle, len(tabs))
	for i := range bounds {
		right, bottom := 16.0, 24.0
		if g.db.Game.Screens[s.name].View.Window != "" {
			right, bottom = 32, 32
		}
		x := int(sw - right - float64(len(tabs)-i)*width)
		bounds[i] = image.Rect(x, int(sh-bottom-48), x+int(width)-8, int(sh-bottom))
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
	if g.in.inspect(g.in.click) {
		for i, r := range s.tabBounds(g) {
			if g.in.mouse.In(r) {
				g.in.query(true)
				next = i
			}
		}
	}
	if next < 0 || next == selected {
		return false
	}
	// Keep the existing stack slot and opener; switching is not an event action.
	*s = *newAuthorScreen(g, tabs[next], s.done, nil)
	return true
}

func (s *authorScreen) drawTabs(g *Game, dst *ebiten.Image) {
	tabs := g.db.Game.Screens[s.name].View.Tabs
	for i, r := range s.tabBounds(g) {
		r, g.th.window = g.controlMaterial(r)
		g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		tint := g.th.text
		if tabs[i] == s.name {
			tint = g.th.accent
			rect(dst, float64(r.Min.X), float64(r.Max.Y-3), float64(r.Dx()), 3, tint)
		}
		label := data.Render(g.db.Tr(g.db.Game.Screens[tabs[i]].Title), s.env(g))
		g.drawPlanningText(dst, label, float64(r.Min.X+12), float64(r.Min.Y+12), float64(r.Dx()-24), 18, 1, tint)
	}
}

func (s *authorScreen) scrollTimeline(g *Game, height float64) {
	sw, _ := g.screen()
	s.timelineTop += s.scroll.drag.rows(&g.in, inBox(float64(g.in.mouse.X), float64(g.in.mouse.Y), 0, 108, sw, height), 56)
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
	if v.Window != "" {
		g.surfaceWindow(dst, 32, 60, sw-64, 36, "base")
		date := g.phaseDate(planningDate(st.Date, g.db.Game.Planning.Calendar.Phases[st.Phase], v))
		mark := g.drawPhaseMark(dst, 44, 66, 18)
		g.drawPlanningText(dst, date, 44+mark, 66, 240-mark, 18, 1, g.th.text)
		width := sw - 304
		if left, ok := s.flowBandLeft(g); ok {
			width = max(1, left-276)
		}
		shift := max(0, s.drawHeaderFace(g, dst, v, 44+mark+min(240-mark, gfx.TextWidth(date, 18))+14, 78, 28)+12-260)
		g.drawPlanningText(dst, data.Render(g.db.Tr(v.Header), s.env(g)), 260+shift, 66, width-shift, 18, 1, g.th.text)
		return
	}
	g.surfaceWindow(dst, 8, 4, sw-16, 44, "base")
	date := g.phaseDate(planningDate(st.Date, g.db.Game.Planning.Calendar.Phases[st.Phase], v))
	mark := g.drawPhaseMark(dst, 24, 12, sizeS)
	g.drawPlanningText(dst, date, 24+mark, 12, 240-mark, sizeS, 1, g.th.text)
	width := sw - 264
	if left, ok := s.flowBandLeft(g); ok {
		width = max(1, left-256)
	}
	shift := max(0, s.drawHeaderFace(g, dst, v, 24+mark+min(240-mark, gfx.TextWidth(date, sizeS))+14, 26, 32)+12-240)
	g.drawPlanningText(dst, data.Render(g.db.Tr(v.Header), s.env(g)), 240+shift, 12, width-shift, sizeS, 1, g.th.text)
}

// drawPictureContain draws the picture whole and centered inside x, y, w, h,
// keeping its aspect and its transparency (picture_fit: contain).
func (g *Game) drawPictureContain(dst *ebiten.Image, path string, x, y, w, h float64, alpha float32) {
	img := g.pics.get(path)
	if img == nil || w <= 0 || h <= 0 {
		return
	}
	gfx.DrawImageFit(dst, img, x, y, w, h, alpha)
}

// startEvent suspends the opener and gives the screen action its own save cursor.
func (s *authorScreen) startEvent(g *Game, name string, args map[string]any) {
	s.parent, s.request, s.origin, s.running = g.events, g.req, g.st.Location, true
	g.events, g.req = g.newEventRunner(), nil
	if err := g.events.StartBound(name, args); err != nil {
		g.events.Log(err)
	}
}
