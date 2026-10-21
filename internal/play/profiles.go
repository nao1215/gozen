package play

import (
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
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

// checkProfiles records the profiles the state now opens.
func (g *Game) checkProfiles() {
	env := g.st.Env()
	for _, id := range g.db.Order["profiles"] {
		if g.db.ProfileHolds(id, env) {
			g.unlock(profileKey(id))
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
	ids   []string
	sel   int
	top   int
	face  int
	env   expr.Env
	done  func()
	shown string
	// press is where a press began and last where it is now, for a swipe.
	press, last image.Point
	pressed     bool
}

func newProfiles(g *Game, done func()) *profileScreen {
	if g.st != nil && !g.replaying {
		g.checkProfiles()
	}
	s := &profileScreen{ids: g.db.Order["profiles"], env: g.profileEnv(), done: done}
	s.trace(g)
	return s
}

// modal draws the book over the lines and pictures of the scene it was opened from.
func (s *profileScreen) modal() bool { return true }

func (s *profileScreen) liveKeep(g *Game) scene {
	n := newProfiles(g, s.done)
	n.sel = min(s.sel, max(0, len(n.ids)-1))
	n.top = min(s.top, n.sel)
	return n
}

func (s *profileScreen) opened(g *Game, id string) bool {
	if g.opts.Development || g.opts.GalleryAll || g.unlocked[profileKey(id)] {
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

func (s *profileScreen) faceName(g *Game) string {
	faces := g.db.ProfileFaces(s.current())
	if len(faces) == 0 {
		return ""
	}
	return faces[min(s.face, len(faces)-1)]
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

func (s *profileScreen) rowRect(g *Game, i int) image.Rectangle {
	sw, _ := g.screen()
	y := profileTop + float64(i-s.top)*profileRow
	return image.Rect(20, int(y), int(sw*0.17), int(y+profileRow-6))
}

// arrows are the band's buttons to the previous and the next character.
func (s *profileScreen) arrows(g *Game) (prev, next image.Rectangle) {
	sw, _ := g.screen()
	cx := sw * 0.78
	return image.Rect(int(cx-230), 8, int(cx-180), 52), image.Rect(int(cx+180), 8, int(cx+230), 52)
}

// faceButtons are the buttons of the faces under the standing picture (none for one face).
func (s *profileScreen) faceButtons(g *Game, faces []string) []image.Rectangle {
	sw, sh := g.screen()
	if len(faces) < 2 {
		return nil
	}
	ws := make([]float64, len(faces))
	total := 0.0
	for i, f := range faces {
		ws[i] = gfx.TextWidth(f, sizeS) + 24
		total += ws[i] + 8
	}
	x := sw*0.36 - (total-8)/2
	out := make([]image.Rectangle, len(faces))
	for i := range faces {
		out[i] = image.Rect(int(x), int(sh-84), int(x+ws[i]), int(sh-54))
		x += ws[i] + 8
	}
	return out
}

func (s *profileScreen) pick(g *Game, i int) {
	if i == s.sel {
		return
	}
	s.sel, s.face = i, 0
	g.sfx("cursor")
}

func (s *profileScreen) update(g *Game) {
	n := len(s.ids)
	if g.in.cancel() {
		g.sfx("cancel")
		g.pop()
		if s.done != nil {
			s.done()
		}
		return
	}
	if n == 0 {
		return
	}
	id := s.current()
	faces := g.db.ProfileFaces(id)
	prev, next := s.arrows(g)
	swipe := s.swipe(g)
	// L and R of a pad are the buttons of the log and auto; A on a keyboard is both left
	// and auto, so auto counts only without left.
	back := g.in.left() || g.in.pressed[btnLog] || swipe > 0 || g.in.click && g.in.mouse.In(prev)
	fwd := g.in.right() || g.in.pressed[btnAuto] && !g.in.pressed[btnLeft] || swipe < 0 || g.in.click && g.in.mouse.In(next)
	switch {
	case back || g.in.up():
		s.pick(g, (s.sel+n-1)%n)
	case fwd || g.in.down():
		s.pick(g, (s.sel+1)%n)
	case g.in.ok() && len(faces) > 1 && s.opened(g, id):
		s.face = (s.face + 1) % len(faces)
		g.sfx("cursor")
	case g.in.click:
		for i := s.top; i < min(n, s.top+s.rows(g)); i++ {
			if g.in.mouse.In(s.rowRect(g, i)) {
				s.pick(g, i)
			}
		}
		if s.opened(g, id) {
			for i, r := range s.faceButtons(g, faces) {
				if g.in.mouse.In(r) && i != s.face {
					s.face = i
					g.sfx("cursor")
				}
			}
		}
	}
	if s.sel < s.top {
		s.top = s.sel
	}
	if r := s.rows(g); s.sel >= s.top+r {
		s.top = s.sel - r + 1
	}
	if p := s.puppet(g); p != nil {
		p.Motion.Step()
	}
	s.trace(g)
}

// swipe is +1 for a swipe to the right (the previous character), -1 for one to the
// left (the next), 0 for none: a press over the picture that moved more than 120 pixels
// sideways, and twice as far sideways as up or down.
func (s *profileScreen) swipe(g *Game) int {
	sw, _ := g.screen()
	if g.in.click && float64(g.in.mouse.X) > sw*0.17 && float64(g.in.mouse.X) < sw*0.55 {
		s.press, s.last, s.pressed = g.in.mouse, g.in.mouse, true
		return 0
	}
	if !s.pressed {
		return 0
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) || len(ebiten.AppendTouchIDs(nil)) > 0 {
		s.last = g.in.mouse
		return 0
	}
	s.pressed = false
	dx, dy := s.last.X-s.press.X, s.last.Y-s.press.Y
	if absInt(dx) < 120 || absInt(dx) < 2*absInt(dy) {
		return 0
	}
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
	sw, sh := g.screen()
	g.drawPicture(dst, g.db.Game.TitleBG, 0, 0, sw, sh, 1)
	fill(dst, withAlpha(colorBlack, 0xa8))
	gfx.FillRect(dst, 0, 0, float32(sw), profileBand, withAlpha(g.th.window, 0xe0), false)
	gfx.FillRect(dst, 0, profileBand-1, float32(sw), 2, withAlpha(g.th.border, 0xa0), false)
	gfx.DrawTextOutlineColor(dst, lang.T("キャラクター"), 20+sw*0.075, 12, sizeL, g.th.text, g.th.window, 1)
	for i := s.top; i < min(len(s.ids), s.top+s.rows(g)); i++ {
		s.drawRow(g, dst, i)
	}
	if s.top > 0 {
		gfx.DrawText(dst, "▲", 20+sw*0.075, profileTop-18, sizeS*0.8, g.th.cursor)
	}
	if s.top+s.rows(g) < len(s.ids) {
		gfx.DrawText(dst, "▼", 20+sw*0.075, profileTop+float64(s.rows(g))*profileRow-8, sizeS*0.8, g.th.cursor)
	}
	if id := s.current(); id != "" {
		s.drawBand(g, dst, id)
		s.drawFigure(g, dst, id)
		s.drawInfo(g, dst, id)
	}
	gfx.DrawTextOutlineColor(dst, lang.T("←→ / LR: キャラ   ↑↓: 一覧   Z: 表情   X: もどる"), sw/2, sh-44, sizeS*0.9, g.th.text, g.th.window, 1)
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
			g.drawPuppet(dst, p, cx, bottom, h, 1, false)
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
	faces := g.db.ProfileFaces(id)
	for i, r := range s.faceButtons(g, faces) {
		c, bg := g.th.text, withAlpha(g.th.window, 0xd8)
		if i == s.face {
			c, bg = g.th.cursor, withAlpha(g.th.accent, 0x90)
		}
		gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 8, bg)
		gfx.DrawText(dst, faces[i], float64(r.Min.X+r.Dx()/2), float64(r.Min.Y)+4, sizeS*0.9, c)
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
	if len(p.Fields) > 0 {
		s.rule(g, dst, x, y, w)
		y += 10
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
		s.rule(g, dst, x, y, w)
		y += 10
		y += s.lines(g, dst, strings.TrimSpace(data.Render(p.Text, s.env)), x, y, w, sizeS*0.9, bottom, g.th.text) + 8
	}
	if len(p.Relations) == 0 {
		return
	}
	s.rule(g, dst, x, y, w)
	y += 10
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
