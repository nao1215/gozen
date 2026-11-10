package play

import (
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/base/expr"
	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/model/rpg"
	"github.com/nao1215/gozen/internal/rules/event"
	"github.com/nao1215/gozen/internal/screen/gfx"
	"github.com/nao1215/gozen/internal/screen/puppet"
)

// The character book (ADR 0031). A profile opens when its when holds and stays open, in
// the gallery's record of unlocks. Under --dev every profile is open.

func (g *Game) hasProfiles() bool { return g.db != nil && len(g.db.Profiles) > 0 }

// profilesInMenus reports whether the menus (title, field menu, novel bar) offer the
// book: when game.yaml asks for it (ui.profiles), and always under --dev.
func (g *Game) profilesInMenus() bool {
	return g.hasProfiles() && (g.db.Game.UI.Profiles || g.opts.Development)
}

func profileKey(id string) string { return "profile:" + id }

// profileFaceKey records a face of a profile with a when of its own.
func profileFaceKey(id, face string) string { return "profile:" + id + "/face:" + face }

// profileTitle is the book's name in the menus and on its band.
func (g *Game) profileTitle() string {
	if b := g.db.Game.UI.ProfileBook; b != nil && b.Title != "" {
		return b.Title
	}
	return lang.T("キャラクター")
}

// bookAll reports whether every profile and face is open: under --dev (unless
// --book-as-played) and with --gallery-all.
func (g *Game) bookAll() bool {
	return g.opts.Development && !g.opts.BookAsPlayed || g.opts.GalleryAll
}

// profileGrid reports whether the book is laid out as a grid of face cards.
func (g *Game) profileGrid() bool {
	b := g.db.Game.UI.ProfileBook
	return b != nil && b.Layout == "grid"
}

// bookKey opens the book with F6 over whatever is on the screen (the book itself closes
// with F6 in its update), when the menus offer it.
func (g *Game) bookKey() {
	if !g.in.inspect(g.in.pressed[btnBook]) || !g.profilesInMenus() || g.memo != nil || g.replaying || g.panelModal() || len(g.stack) == 0 {
		return
	}
	if _, open := g.stack[len(g.stack)-1].(*profileScreen); open {
		return
	}
	g.in.query(true)
	g.in.pressed[btnBook] = false
	g.sfx("decide")
	g.push(newProfiles(g, nil))
}

// checkProfiles records the profiles the state now opens.
func (g *Game) checkProfiles() {
	env := g.st.Env()
	for _, id := range g.db.Order["profiles"] {
		if !g.db.ProfileHolds(id, env) {
			continue
		}
		g.unlock(profileKey(id))
		for i, face := range g.db.ProfileFaces(id) {
			if g.db.ProfileFaceLocked(id, i) && g.db.ProfileFaceHolds(id, i, env) {
				g.unlock(profileFaceKey(id, face))
			}
		}
	}
}

// meetLine records the profiles and faces a line opens: a profile with met when its
// actor speaks, a face with seen when the line shows it.
func (g *Game) meetLine(r event.Say) {
	if !g.hasProfiles() || g.replaying {
		return
	}
	for id, faces := range g.db.ProfilesMet(r.Actor, r.Face) {
		if g.db.Profiles[id].Met {
			g.unlock(profileKey(id))
		}
		for _, f := range faces {
			g.unlock(profileFaceKey(id, f))
		}
	}
}

// profileEnv is the state the book fills its {expressions} from: the game's, or a new
// game's at the title.
func (g *Game) profileEnv() expr.Env {
	if g.st != nil {
		return g.st.Env()
	}
	return rpg.New(g.db, g.rng()).Env()
}

type profileScreen struct {
	ids []string
	sel int
	// list is the list layout's rows: the rows shown and the pointer over them (the
	// screen draws the rows itself, and sel is the cursor of all three layouts).
	list   *list
	scroll float64 // the grid's scroll, in the screen's units
	// drag is the finger that scrolls the grid, and free tells that it scrolled it last:
	// the grid then stays until the selection moves.
	drag       swipe
	free       bool
	face       int
	env        expr.Env
	done       func()
	shown      string
	chart      bool
	zoom       float64
	camX, camY float64
	chartTick  int
	focus      int // selected profile during a chart transition; -1 means the full chart
	focusFrame int // 0..chartFocusFrames, also counts down when returning
	focusBack  bool
	chartPage  bool // the detail page was opened from a focused chart
	// press is where a press began and last where it is now, for a swipe.
	press, last image.Point
	pressed     bool
}

func newProfiles(g *Game, done func()) *profileScreen {
	if g.st != nil && !g.replaying {
		g.checkProfiles()
	}
	s := &profileScreen{ids: g.db.Order["profiles"], env: g.profileEnv(), done: done, focus: -1}
	s.zoom = 1
	s.trace(g)
	return s
}

// modal draws the book over the lines and pictures of the scene it was opened from.
func (s *profileScreen) modal() bool { return true }

func (s *profileScreen) liveKeep(g *Game) scene {
	n := newProfiles(g, s.done)
	n.sel = min(s.sel, max(0, len(n.ids)-1))
	n.rowList(g).top = min(s.rowList(g).top, n.sel)
	n.chart, n.zoom, n.camX, n.camY = s.chart, s.zoom, s.camX, s.camY
	n.focus, n.focusFrame = s.focus, s.focusFrame
	n.focusBack = s.focusBack
	n.chartPage = s.chartPage
	return n
}

func (s *profileScreen) opened(g *Game, id string) bool {
	if g.bookAll() || g.unlocked[profileKey(id)] {
		return true
	}
	return g.st != nil && !g.replaying && g.db.ProfileHolds(id, g.st.Env())
}

func (s *profileScreen) current() string {
	if s.sel < 0 || s.sel >= len(s.ids) {
		return ""
	}
	return s.ids[s.sel]
}

// faceOpen reports whether face i of profile id is offered: a face without a when of
// its own, or one unlocked now or in an earlier play (always under --dev).
func (s *profileScreen) faceOpen(g *Game, id string, i int) bool {
	if !g.db.ProfileFaceLocked(id, i) || g.bookAll() {
		return true
	}
	faces := g.db.ProfileFaces(id)
	if i < 0 || i >= len(faces) {
		return false
	}
	if g.unlocked[profileFaceKey(id, faces[i])] {
		return true
	}
	return g.st != nil && !g.replaying && g.db.ProfileFaceHolds(id, i, g.st.Env())
}

// shownFaces are the indexes of the faces of profile id the screen offers.
func (s *profileScreen) shownFaces(g *Game, id string) []int {
	var out []int
	for i := range g.db.ProfileFaces(id) {
		if s.faceOpen(g, id, i) {
			out = append(out, i)
		}
	}
	return out
}

// faceIndex is the index among the profile's faces of the face shown (-1 for none).
func (s *profileScreen) faceIndex(g *Game) int {
	shown := s.shownFaces(g, s.current())
	if len(shown) == 0 {
		return -1
	}
	return shown[min(s.face, len(shown)-1)]
}

func (s *profileScreen) faceName(g *Game) string {
	i := s.faceIndex(g)
	if i < 0 {
		return ""
	}
	return g.db.ProfileFaces(s.current())[i]
}

func (s *profileScreen) trace(g *Game) {
	id := s.current()
	if id == "" {
		return
	}
	face := s.faceName(g)
	key := id + "/" + face
	if key == s.shown {
		return
	}
	s.shown = key
	g.emit("profiles", traceLine{"profile": id, "face": face, "unlocked": s.opened(g, id)})
}

// The layout, in the screen's units (1280x720 on a 1920x1080 game): a band on top with
// the title and the switch between characters, the list on the left, the standing
// picture in the middle with its faces under it, and the profile on the right.

const (
	profileBand = 60.0 // the height of the top band
	profileTop  = 76.0 // where the list and the profile begin
	profileRow  = 62.0 // the height of a row of the list
)

func (s *profileScreen) rows(g *Game) int {
	_, sh := g.screen()
	return max(1, int((sh-profileTop-50)/profileRow))
}

// rowList is the list of the list layout, fitted to the screen and set on the profile
// selected.
func (s *profileScreen) rowList(g *Game) *list {
	if s.list == nil || len(s.list.items) != len(s.ids) {
		items := make([]listItem, len(s.ids))
		for i, id := range s.ids {
			items[i].label = id
		}
		s.list = &list{items: items, x: 20, y: profileTop - 12, rowH: profileRow, size: sizeS, press: true}
	}
	sw, _ := g.screen()
	l := s.list
	l.w, l.rows = sw*0.17-20, s.rows(g)
	if l.sel != s.sel {
		l.sel, l.free = s.sel, false
	}
	l.scroll()
	return l
}

func (s *profileScreen) rowRect(g *Game, i int) image.Rectangle {
	r := s.rowList(g).rowRect(i)
	r.Max.Y -= 6
	return r
}

// arrows are the band's buttons to the previous and the next character.
func (s *profileScreen) arrows(g *Game) (prev, next image.Rectangle) {
	sw, _ := g.screen()
	cx := sw * 0.78
	return image.Rect(int(cx-230), 8, int(cx-180), 52), image.Rect(int(cx+180), 8, int(cx+230), 52)
}

// faceLabels are the labels of the faces offered for profile id.
func (s *profileScreen) faceLabels(g *Game, id string) []string {
	var out []string
	for _, i := range s.shownFaces(g, id) {
		out = append(out, g.db.ProfileFaceLabel(id, i))
	}
	return out
}

// faceButtons are the buttons of the faces under the standing picture, centered on
// cx with their top at y (none for one face).
func (s *profileScreen) faceButtonsAt(labels []string, cx, y, h float64) []image.Rectangle {
	if len(labels) < 2 {
		return nil
	}
	ws := make([]float64, len(labels))
	total := 0.0
	for i, f := range labels {
		ws[i] = gfx.TextWidth(f, sizeS) + 24
		total += ws[i] + 8
	}
	x := cx - (total-8)/2
	out := make([]image.Rectangle, len(labels))
	for i := range labels {
		out[i] = image.Rect(int(x), int(y), int(x+ws[i]), int(y+h))
		x += ws[i] + 8
	}
	return out
}

// faceButtons are the buttons of the faces labeled labels in the list layout.
func (s *profileScreen) faceButtons(g *Game, labels []string) []image.Rectangle {
	sw, sh := g.screen()
	return s.faceButtonsAt(labels, sw*0.36, sh-84, 30)
}

// listFaceButtons are the face buttons of profile id in the list layout.
func (s *profileScreen) listFaceButtons(g *Game, id string) []image.Rectangle {
	return s.faceButtons(g, s.faceLabels(g, id))
}

func (s *profileScreen) pick(g *Game, i int) bool {
	if i == s.sel {
		return false
	}
	s.sel, s.face = i, 0
	g.sfx("cursor")
	return true
}

// chartButton is where the button that turns to the relation chart and back
// stands, on every page of the book.
func (s *profileScreen) chartButton(g *Game) image.Rectangle {
	sw, sh := g.screen()
	return image.Rect(int(sw)-176, int(sh)-112, int(sw)-16, int(sh)-72)
}

// drawChartButton names the page the button turns to.
func (s *profileScreen) drawChartButton(g *Game, dst *ebiten.Image) {
	label := lang.T("相関図")
	if s.chart {
		label = lang.T("プロフィール")
	}
	g.drawTapButton(dst, s.chartButton(g), label, sizeS, true)
}

func (s *profileScreen) update(g *Game) {
	if g.tapped(s.chartButton(g)) {
		s.chart = !s.chart
		g.sfx("cursor")
		return // the tap turned the page; the page it shows takes the next one
	}
	if g.in.pressedKey(btnNext) {
		s.chart = !s.chart
		g.sfx("cursor")
	}
	if s.chart {
		s.updateChart(g)
		return
	}
	if g.profileGrid() {
		s.updateGrid(g)
		return
	}
	n := len(s.ids)
	if g.in.cancel() || g.in.pressedKey(btnBook) {
		if s.chartPage && !g.in.inspect(g.in.pressed[btnBook]) {
			s.chart, s.focusBack, s.chartPage = true, true, false
			g.sfx("cancel")
			return
		}
		g.sfx("cancel")
		g.leave(s.done)
		return
	}
	if n == 0 {
		return
	}
	swipe := s.swipe(g)
	// The list moves the cursor up and down (keys, wheel, a click on a row) and scrolls
	// with a finger or its arrows.
	l := s.rowList(g)
	l.browse(g)
	if l.sel != s.sel {
		s.sel, s.face = l.sel, 0
	}
	id := s.current()
	faces := s.shownFaces(g, id)
	prev, next := s.arrows(g)
	// L and R of a pad are the buttons of the log and auto; A on a keyboard is both left
	// and auto, so auto counts only without left.
	back := g.in.inspect(g.in.pressed[btnLeft]) || g.in.inspect(g.in.pressed[btnLog]) || swipe > 0 || g.in.mouse.In(prev) && g.in.inspect(g.in.click)
	fwd := g.in.inspect(g.in.pressed[btnRight]) || !g.in.pressed[btnLeft] && g.in.inspect(g.in.pressed[btnAuto]) || swipe < 0 || g.in.mouse.In(next) && g.in.inspect(g.in.click)
	switch {
	case back:
		g.in.query(s.pick(g, (s.sel+n-1)%n))
	case fwd:
		g.in.query(s.pick(g, (s.sel+1)%n))
	case len(faces) > 1 && s.opened(g, id) && g.in.ok():
		s.face = (s.face + 1) % len(faces)
		g.sfx("cursor")
	case g.in.inspect(g.in.click):
		if s.opened(g, id) {
			for i, r := range s.listFaceButtons(g, id) {
				if g.in.mouse.In(r) && i != s.face {
					g.in.query(true)
					s.face = i
					g.sfx("cursor")
				}
			}
		}
	}
	if p := s.puppet(g); p != nil {
		p.Motion.Step()
	}
	s.trace(g)
}

// swipe is +1 for a swipe to the right (the previous character), -1 for one to the
// left (the next), 0 for none: a press over the picture (a finger or the mouse's button)
// let go more than 120 units to the side, and twice as far sideways as up or down. The
// pointer is read from the game's input, so a script and a recorded run swipe as well; the
// lift of a finger that swiped is no click.
func (s *profileScreen) swipe(g *Game) int {
	in := &g.in
	sw, _ := g.screen()
	if in.inspect(in.pdown) {
		s.press, s.last = in.mouse, in.mouse
		s.pressed = float64(in.mouse.X) > sw*0.17 && float64(in.mouse.X) < sw*0.55
		return 0
	}
	if !s.pressed {
		return 0
	}
	if in.inspect(in.pheld) || in.pup {
		s.last = in.mouse // the pointer may move on the tick it lets go
	}
	if in.pheld {
		return 0
	}
	s.pressed = false
	dx, dy := s.last.X-s.press.X, s.last.Y-s.press.Y
	if absInt(dx) < 120 || absInt(dx) < 2*absInt(dy) {
		return 0
	}
	in.query(true)
	in.click = false
	if dx > 0 {
		return 1
	}
	return -1
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// profilePicture is the standing picture of profile id with face (a file under assets/).
func (g *Game) profilePicture(id, face string) string {
	p := g.db.Profiles[id]
	if p.Picture != "" {
		return p.Picture
	}
	if e := g.db.Enemies[p.Enemy]; e != nil {
		return e.Image.File
	}
	if a := g.db.Actors[g.db.ProfileActor(id)]; a != nil && a.Faces != "" && face != "" {
		return a.Faces + "/" + face + ".png"
	}
	return ""
}

// puppet is the moving portrait of the profile shown, set to its face (nil for none).
func (s *profileScreen) puppet(g *Game) *puppet.Puppet {
	id := s.current()
	if id == "" {
		return nil
	}
	p, i := g.talkPuppet(event.Say{Actor: g.db.ProfileActor(id), Face: s.faceName(g)})
	if p != nil {
		p.Motion.SetFace(i)
	}
	return p
}

func (s *profileScreen) draw(g *Game, dst *ebiten.Image) {
	defer s.drawChartButton(g, dst)
	if s.chart {
		s.drawChart(g, dst)
		return
	}
	if g.profileGrid() {
		s.drawGrid(g, dst)
		return
	}
	sw, sh := g.screen()
	g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0xa8))
	gfx.FillRect(dst, 0, 0, float32(sw), profileBand, withAlpha(g.th.window, 0xe0), false)
	gfx.FillRect(dst, 0, profileBand-1, float32(sw), 2, withAlpha(g.th.border, 0xa0), false)
	gfx.DrawTextOutlineColor(dst, g.profileTitle(), 20+sw*0.075, 12, sizeL, g.th.text, g.th.window, 1)
	l := s.rowList(g)
	for _, i := range l.shown() {
		s.drawRow(g, dst, i)
	}
	l.drawMarks(g, dst)
	if id := s.current(); id != "" {
		s.drawBand(g, dst, id)
		s.drawFigure(g, dst, id)
		s.drawInfo(g, dst, id)
	}
	gfx.DrawTextOutlineColor(dst, lang.T("←→ / LR: キャラ   ↑↓: 一覧   Z: 表情   Tab: 相関図   X / F6: もどる"), sw/2, sh-44, sizeS*0.9, g.th.text, g.th.window, 1)
}

// drawBand draws the switch between characters on the top band: ◀ name n / N ▶.
func (s *profileScreen) drawBand(g *Game, dst *ebiten.Image, id string) {
	sw, _ := g.screen()
	prev, next := s.arrows(g)
	for _, r := range []image.Rectangle{prev, next} {
		gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 12, withAlpha(g.th.accent, 0x50))
	}
	gfx.DrawText(dst, "◀", float64(prev.Min.X+prev.Dx()/2), float64(prev.Min.Y)+8, sizeM, g.th.cursor)
	gfx.DrawText(dst, "▶", float64(next.Min.X+next.Dx()/2), float64(next.Min.Y)+8, sizeM, g.th.cursor)
	name := lang.T("？？？")
	if s.opened(g, id) {
		name = g.db.ProfileName(id)
	}
	label := name + "    " + itoa(s.sel+1) + " / " + itoa(len(s.ids))
	ls, sz := gfx.FitLines(label, sizeM, 330, 1)
	if len(ls) > 0 {
		gfx.DrawTextOutlineColor(dst, ls[0], sw*0.78, 16, sz, g.th.text, g.th.window, 1)
	}
}

func (s *profileScreen) drawRow(g *Game, dst *ebiten.Image, i int) {
	id := s.ids[i]
	r := s.rowRect(g, i)
	x, y, w, h := float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy())
	bg := withAlpha(g.th.window, 0xc0)
	if i == s.sel {
		bg = withAlpha(g.th.accent, 0x70)
	}
	gfx.FillRoundRect(dst, x, y, w, h, 8, bg)
	if i == s.sel {
		gfx.FillRect(dst, x, y+6, 4, h-12, g.th.cursor, false)
	}
	open := s.opened(g, id)
	icon := float64(h) - 8
	if faces := g.db.ProfileFaces(id); len(faces) > 0 {
		if pic := g.profilePicture(id, faces[0]); pic != "" {
			s.drawIcon(g, dst, pic, float64(x)+8, float64(y)+4, icon, !open)
		}
	}
	name := lang.T("？？？")
	c := g.th.dim
	if open {
		name, c = g.db.ProfileName(id), g.th.text
	}
	if i == s.sel {
		c = g.th.cursor
	}
	ls, sz := gfx.FitLines(name, sizeS, float64(w)-icon-26, 1)
	if len(ls) > 0 {
		g.text(dst, ls[0], float64(x)+icon+18, float64(y)+float64(h)/2-sz*0.7, sz, c)
	}
}

// drawIcon draws the face cut from a standing picture, or its shadow.
func (s *profileScreen) drawIcon(g *Game, dst *ebiten.Image, pic string, x, y, size float64, shadow bool) {
	if !shadow {
		g.drawFace(dst, pic, x, y, size, 1, false)
		return
	}
	img := g.pics.get(pic)
	r := g.pics.head(pic)
	sub, ok := img.SubImage(r).(*ebiten.Image)
	if !ok || sub.Bounds().Empty() {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(size/float64(r.Dx()), size/float64(r.Dy()))
	op.GeoM.Translate(x, y)
	darken(&op.ColorScale)
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(size), float32(size), 10, color.NRGBA{0xff, 0xff, 0xff, 24})
	gfx.DrawImage(dst, sub, op)
}

// darken turns a picture into its shadow.
func darken(cs *ebiten.ColorScale) { cs.Scale(0.025, 0.02, 0.035, 0.95) }

func (s *profileScreen) drawFigure(g *Game, dst *ebiten.Image, id string) {
	sw, sh := g.screen()
	open := s.opened(g, id)
	cx, bottom, h := sw*0.36, sh-92, sh-92-profileTop
	if p := s.puppet(g); p != nil {
		if open {
			g.drawPuppet(dst, p, cx, bottom, h, 1, false, nil)
		}
		// A locked puppet shows no shadow (its parts are drawn apart); the name says ???.
	} else if pic := g.profilePicture(id, s.faceName(g)); pic != "" {
		cs := &ebiten.ColorScale{}
		if !open {
			darken(cs)
		}
		g.drawStanding(dst, pic, cx, bottom, h, 1, false, cs)
	}
	if !open {
		return
	}
	labels := s.faceLabels(g, id)
	for i, r := range s.listFaceButtons(g, id) {
		c, bg := g.th.text, withAlpha(g.th.window, 0xd8)
		if i == s.face {
			c, bg = g.th.cursor, withAlpha(g.th.accent, 0x90)
		}
		gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 8, bg)
		gfx.DrawText(dst, labels[i], float64(r.Min.X+r.Dx()/2), float64(r.Min.Y)+4, sizeS*0.9, c)
	}
}

// rule draws a thin line across the profile panel.
func (s *profileScreen) rule(g *Game, dst *ebiten.Image, x, y, w float64) {
	gfx.FillRect(dst, float32(x), float32(y), float32(w), 2, withAlpha(g.th.border, 0x80), false)
}

// lines draws s wrapped to w from y, as many lines as fit above bottom (the last one
// ending in an ellipsis when some do not), and returns the height drawn.
func (s *profileScreen) lines(g *Game, dst *ebiten.Image, str string, x, y, w, size, bottom float64, c color.Color) float64 {
	lh := gfx.LineHeight(size)
	ls := gfx.Wrap(str, size, w)
	n := min(len(ls), max(0, int((bottom-y)/lh)))
	for i := range n {
		ln := ls[i]
		if i == n-1 && n < len(ls) {
			ln = strings.TrimRight(ln, " ") + "…"
		}
		g.text(dst, ln, x, y+float64(i)*lh, size, c)
	}
	return float64(n) * lh
}

func (s *profileScreen) drawInfo(g *Game, dst *ebiten.Image, id string) {
	sw, sh := g.screen()
	x, y, w, h := sw*0.55, profileTop, sw*0.45-20, sh-profileTop-50
	g.window(dst, x, y, w, h)
	s.drawProfileText(g, dst, id, x, y, w, h)
}

// drawProfileText draws the name, title, fields, introduction and relations of
// profile id inside the board at x, y, w, h.
func (s *profileScreen) drawProfileText(g *Game, dst *ebiten.Image, id string, x, y, w, h float64) {
	bottom := y + h - 8
	x, w = x+24, w-48
	y += 16
	if !s.opened(g, id) {
		g.text(dst, lang.T("？？？"), x, y, sizeL*1.3, g.th.dim)
		return
	}
	p := g.db.Profiles[id]
	g.text(dst, g.db.ProfileName(id), x, y, sizeL*1.2, g.th.text)
	y += gfx.LineHeight(sizeL * 1.2)
	if p.Title != "" {
		y += s.lines(g, dst, data.Render(p.Title, s.env), x, y, w, sizeS, bottom, g.th.accent)
	}
	y += 8
	heading := func(label string) {
		g.text(dst, label, x, y, sizeS*0.85, g.th.accent)
		y += gfx.LineHeight(sizeS*0.85) + 2
		s.rule(g, dst, x, y-3, w)
		y += 6
	}
	if len(p.Fields) > 0 {
		heading(lang.T("プロフィール"))
		labelW := 0.0
		for _, f := range p.Fields {
			labelW = max(labelW, gfx.TextWidth(f.Label, sizeS*0.9))
		}
		labelW = min(labelW+20, w*0.35)
		for _, f := range p.Fields {
			if y+gfx.LineHeight(sizeS*0.9) > bottom {
				return
			}
			g.text(dst, f.Label, x, y, sizeS*0.9, g.th.dim)
			y += s.lines(g, dst, data.Render(f.Value, s.env), x+labelW, y, w-labelW, sizeS*0.9, bottom, g.th.text) + 2
		}
		y += 8
	}
	if p.Text != "" {
		heading(lang.T("紹介"))
		y += s.lines(g, dst, strings.TrimSpace(data.Render(p.Text, s.env)), x, y, w, sizeS*0.9, bottom, g.th.text) + 8
	}
	if len(p.Relations) == 0 {
		return
	}
	heading(lang.T("関係"))
	for _, r := range p.Relations {
		if y+gfx.LineHeight(sizeS*0.9) > bottom {
			return
		}
		who := lang.T("あなた")
		if r.Who != "you" {
			who = g.db.ProfileName(r.Who)
			if _, ok := g.db.Profiles[r.Who]; !ok {
				if a := g.db.Actors[r.Who]; a != nil {
					who = a.Name
				}
			} else if !s.opened(g, r.Who) {
				who = lang.T("？？？")
			}
		}
		head := who
		if calls := data.Render(r.Calls, s.env); calls != "" && calls != who && who != lang.T("？？？") {
			head += "  " + lang.T("「") + calls + lang.T("」")
		}
		y += s.lines(g, dst, head, x, y, w, sizeS*0.9, bottom, g.th.cursor)
		if r.Text != "" {
			y += s.lines(g, dst, data.Render(r.Text, s.env), x+16, y, w-16, sizeS*0.8, bottom, g.th.text)
		}
		y += 4
	}
}
