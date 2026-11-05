package play

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// Chat messages (the message command) wait in the state's inbox. On a
// full-screen surface the oldest shows as a notification in the top-right
// corner: the speaker's small square face, the name and the first line, with
// the count of the others. Pressing it, or the inbox key, opens a bubble in the
// middle of the screen; closing the bubble drops the message and returns to the
// screen. Opening starts the message choice event on that screen.

// inboxToastRect is the notification's place below the header band.
func (g *Game) inboxToastRect() image.Rectangle {
	sw, _ := g.screen()
	return image.Rect(int(sw)-392, 104, int(sw)-16, 168)
}

// inboxSurface is the surface a notification shows on: a full-screen authored
// screen on top that is not running an action, with no line being spoken.
func (g *Game) inboxSurface() *authorScreen {
	if g.st == nil || len(g.st.Inbox) == 0 || g.talk.active || len(g.stack) == 0 {
		return nil
	}
	s, ok := g.stack[len(g.stack)-1].(*authorScreen)
	if !ok || s.running || s.popup != nil {
		return nil
	}
	if def := g.db.Game.Screens[s.name]; def == nil || !def.View.Fullscreen || def.View.Call != nil {
		return nil
	}
	if g.guideOn(s) || g.guide == nil && g.guideDef(s.name) != nil && g.guidesOn() {
		return nil // a screen guide owns the corner until it is finished
	}
	return s
}

// actorFacePath is the picture of an actor's default face.
func (g *Game) actorFacePath(who string) (string, string) {
	a, ok := g.db.Actors[who]
	if !ok {
		return "", who
	}
	face := a.Face
	if face == "" {
		face = "base"
	}
	if a.Faces == "" {
		return "", a.Name
	}
	return a.Faces + "/" + face + ".png", a.Name
}

func (g *Game) drawInboxToast(dst *ebiten.Image) {
	if g.inboxSurface() == nil {
		return
	}
	m := g.st.Inbox[0]
	r := g.inboxToastRect()
	x, y, w, h := float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy())
	light := g.lightPalette()
	if !g.materialSurface(dst, x, y, w, h, color.NRGBA{250, 252, 254, 240}, "card") {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, color.NRGBA{250, 252, 254, 240})
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, 1, color.NRGBA{255, 255, 255, 220})
	path, name := g.actorFacePath(m.Who)
	previous := g.faceSquare
	g.faceSquare = true
	g.planningFace(dst, path, x+8+24, y+h/2, 24)
	g.faceSquare = previous
	nameLeft := x + 66
	if m.Tag != "" {
		nameLeft += drawInboxTag(g, dst, m.Tag, nameLeft, y+7) + 6
	}
	g.drawPlanningText(dst, name, nameLeft, y+6, w-(nameLeft-x)-54, 16, 1, light.accent)
	first, _, _ := strings.Cut(m.Text, "\n")
	g.drawPlanningText(dst, first, x+66, y+30, w-80, 15, 1, light.text)
	if n := len(g.st.Inbox); n > 1 {
		label := "+" + strconv.Itoa(n-1)
		bw := gfx.TextWidth(label, 14) + 14
		gfx.FillRoundRect(dst, float32(x+w-bw-8), float32(y+6), float32(bw), 22, 11, light.accent)
		g.text(dst, label, x+w-bw-1, y+6, 14, color.White)
	}
}

// updateInboxToast opens the oldest message when its notification is pressed,
// the inbox key is pressed or a script picked it: the message leaves the inbox
// and its event runs on this screen under the bubble, as an entry action does,
// asking for the decision. It reports whether it opened one.
func (g *Game) updateInboxToast(s *authorScreen) bool {
	if g.inboxSurface() != s {
		s.inboxArmed = false
		return false
	}
	open := g.in.pressed[btnInbox] || g.in.click && g.in.mouse.In(g.inboxToastRect()) || s.inboxArmed && g.in.ok()
	s.inboxArmed = false
	if !open {
		return false
	}
	m := g.st.Inbox[0]
	g.st.Inbox = g.st.Inbox[1:]
	g.inboxShown = &m
	g.emit("message_open", traceLine{"who": m.Who, "event": m.Event, "left": len(g.st.Inbox)})
	s.destination, s.homed = "", false
	s.startEvent(g, m.Event, nil)
	return true
}

// drawInboxBubble shows the opened message in the upper middle of the screen,
// above the decision: the speaker's square crop, the name and the whole text.
func (g *Game) drawInboxBubble(dst *ebiten.Image) {
	m := g.inboxShown
	if m == nil {
		return
	}
	sw, _ := g.screen()
	w, h := min(760.0, sw-64), 186.0
	x, y := (sw-w)/2, 92.0
	light := g.lightPalette()
	if !g.materialSurface(dst, x, y, w, h, color.NRGBA{250, 252, 254, 245}, "card") {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 16, color.NRGBA{250, 252, 254, 245})
	}
	gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 16, 1, color.NRGBA{255, 255, 255, 230})
	path, name := g.actorFacePath(m.Who)
	crop := h - 32
	gfx.FillRoundRect(dst, float32(x+16), float32(y+16), float32(crop*.8), float32(crop), 10, color.NRGBA{236, 241, 247, 220})
	if path != "" {
		g.planningBust(dst, path, x+16, y+16, crop*.8, crop, .14, .42)
	}
	left := x + 32 + crop*.8
	nameLeft := left
	if m.Tag != "" {
		nameLeft += drawInboxTag(g, dst, m.Tag, nameLeft, y+22) + 8
	}
	g.drawPlanningText(dst, name, nameLeft, y+18, w-(nameLeft-x)-20, 20, 1, light.accent)
	g.drawPlanningText(dst, m.Text, left, y+52, w-(left-x)-24, 17, 5, light.text)
}

// drawInboxTag draws a message's tag (such as DM) as a small chip and returns
// its width.
func drawInboxTag(g *Game, dst *ebiten.Image, tag string, x, y float64) float64 {
	w := gfx.TextWidth(tag, 13) + 14
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), 20, 10, color.NRGBA{96, 112, 160, 230})
	g.text(dst, tag, x+7, y+1, 13, color.White)
	return w
}
