package play

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// The band of the novel buttons (ui.quick_menu): its height, the room between it and
// the window it does not share, and the room at its ends.
const (
	quickBarH = 42.0
	quickGap  = 8.0
	quickPad  = 12.0
	// novelBoxH is the height of the boxed novel window (ui.material or a narrowed one);
	// the v0.15 strip across the screen is novelH tall.
	novelBoxH = novelH - 42
	// novelSide is the room each side of the material window.
	novelSide = 104.0
)

// novelFrame is the layout of a novel message window and its buttons on one screen.
// Drawing and the clicks share it, so a button is where it shows.
type novelFrame struct {
	// win is the message window (the strip, the box or a call's captions), with the
	// band when the buttons stand inside it.
	win rectF
	// bar is the band of the buttons; buttons are their hit rectangles, in order.
	bar     rectF
	buttons []rectF
	place   string
	// strip is the v0.15 window across the screen, which carries the buttons itself.
	strip bool
	// band reports whether the band is drawn as its own surface.
	band bool
	// textL and textR bound the speaker's name and the lines.
	textL, textR float64
}

// novelBase is the novel window before its buttons are placed: where it stands (its
// bottom is where the window and the band may reach) and whether it is the strip.
func novelBase(ui *data.UIStyle, material bool, sw, sh float64) (rectF, bool) {
	x, w, bottom, h := 0.0, sw, sh, novelH
	strip := !material
	if material {
		x, w, bottom, h = novelSide, sw-2*novelSide, sh-quickGap, novelBoxH
	}
	if nw := ui.NovelWindow; nw != nil {
		if nw.Width > 0 {
			w = sw * nw.Width
			x = (sw - w) / 2
			if nw.X > 0 {
				x = sw * nw.X
			}
		}
		if nw.Bottom > 0 {
			bottom = sh - sh*nw.Bottom
		}
		if strip && (x > 0 || w < sw || bottom < sh) {
			strip, h = false, novelBoxH
		}
	}
	return rectF{x, bottom - h, w, h}, strip
}

// quickMenu lays the message window base and its buttons (label widths) out for
// ui.quick_menu place. Empty, the buttons stand in a row at the screen's lower right
// and a window that would run under them is raised above them. below puts them in a
// band under the window with its edges, the window raised by the band; inside puts
// them in the window's lower edge, the window taller by the band. A band too narrow
// for its buttons in one row takes them in more rows, taller by a row each.
func quickMenu(place string, base rectF, strip bool, sw, sh float64, widths []float64) novelFrame {
	f := novelFrame{win: base, place: place, strip: strip}
	bottom := base.y + base.h
	gap := quickGap
	if strip {
		gap = 0
	}
	f.buttons = make([]rectF, len(widths))
	switch place {
	case "below", "inside":
		rows := quickRows(widths, base.w-2*quickPad)
		h := quickBarH * float64(max(1, len(rows)))
		f.bar = rectF{base.x, bottom - h, base.w, h}
		if place == "below" {
			f.win.y = f.bar.y - gap - base.h
			f.band = true
		} else {
			f.win = rectF{base.x, bottom - base.h - h, base.w, base.h + h}
		}
		for r, row := range rows {
			spreadButtons(&f, row, widths, f.bar.y+quickBarH*float64(r))
		}
	default:
		x := sw - 40
		for i := len(widths) - 1; i >= 0; i-- {
			w := widths[i] + 28
			x -= w
			f.buttons[i] = rectF{x, sh - 40, w, 30}
			x -= 10
		}
		if len(widths) > 0 {
			f.bar = rectF{f.buttons[0].x - 28, sh - 46, sw - f.buttons[0].x + 16, quickBarH}
		}
		if !strip && len(widths) > 0 && f.win.rect().Overlaps(f.bar.rect()) {
			f.win.y = f.bar.y - gap - f.win.h
		}
	}
	f.textL, f.textR = f.win.x+26, f.win.x+f.win.w-26
	if strip {
		f.textL, f.textR = 130, sw-130
	}
	return f
}

// quickLeast is the least room each side of a label in a crowded band.
const quickLeast = 6.0

// quickRows splits the buttons (label widths) into rows that fit room, in order.
func quickRows(widths []float64, room float64) [][]int {
	var rows [][]int
	var row []int
	used := 0.0
	for i, w := range widths {
		if len(row) > 0 && used+w+2*quickLeast > room {
			rows, row, used = append(rows, row), nil, 0
		}
		row = append(row, i)
		used += w + 2*quickLeast
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// spreadButtons fills a row of the band at y with its buttons: equal cells when every
// label fits one, else cells as wide as their labels and an equal share of the room left.
func spreadButtons(f *novelFrame, row []int, widths []float64, y float64) {
	n := float64(len(row))
	tight, widest := 0.0, 0.0
	for _, i := range row {
		tight += widths[i] + 2*quickLeast
		widest = math.Max(widest, widths[i]+28)
	}
	room := f.bar.w - 2*quickPad
	x := f.bar.x + quickPad
	for _, i := range row {
		cell := room / n
		if widest > cell {
			cell = widths[i] + 2*quickLeast + (room-tight)/n
		}
		f.buttons[i] = rectF{x, y + 6, cell, 30}
		x += cell
	}
}

// buttonRect is a button's hit rectangle in whole pixels, as the row has always had.
func (r rectF) buttonRect() image.Rectangle {
	return image.Rect(int(r.x), int(r.y), int(r.x+r.w), int(r.y+r.h))
}

// chatCaption is where a call's (or a chat's) captions stand before the buttons.
func chatCaption(call bool, sw, sh float64) rectF {
	if call {
		return callGeometry(sw, sh).caption
	}
	return rectF{36, sh - 174, sw - 48 - min(340.0, (sw-48)*.30) - 36, 96}
}

// novelFrame lays the novel window of the line shown out with its buttons.
func (g *Game) novelFrame() novelFrame {
	sw, sh := g.screen()
	widths := g.novelButtonWidths()
	ui := &g.db.Game.UI
	if def := g.db.Game.Screens[g.talk.say.Ensemble]; g.talk.say.Ensemble != "" && def != nil && captionsOnStage(def.View) {
		return quickMenu(ui.QuickMenu, chatCaption(def.View.Call != nil, sw, sh), false, sw, sh, widths)
	}
	base, strip := novelBase(ui, g.hasMaterial(), sw, sh)
	if def := g.db.Game.Screens[g.talk.say.Ensemble]; g.talk.say.Ensemble != "" && def != nil && callOverlay(def.View) {
		base = overlayBase(base, sh)
	}
	return quickMenu(ui.QuickMenu, base, strip, sw, sh, widths)
}

// callOverlay reports whether a screen is a call whose lines show in the message
// window laid over it (call.captions: overlay).
func callOverlay(v data.PlanningView) bool {
	return v.Call != nil && v.Call.Captions == "overlay"
}

// overlayBase stretches the novel window base down to the screen's lower edge, its
// top kept, for a window laid over a full call: the window reads as docked on the
// call, not floating over its lower edge.
func overlayBase(base rectF, sh float64) rectF {
	base.h = max(base.h, sh-base.y)
	return base
}

// novelButtonWidths are the widths of the labels of the novel bar's buttons.
func (g *Game) novelButtonWidths() []float64 {
	buttons := g.novelButtons()
	widths := make([]float64, len(buttons))
	for i, b := range buttons {
		widths[i] = gfx.TextWidth(lang.T(b.label), sizeS)
	}
	return widths
}

// captionsOnStage reports whether a screen's lines show as captions over its call or
// chat, rather than in the novel message window (call.captions: window).
func captionsOnStage(v data.PlanningView) bool {
	return v.Chat != nil && (v.Call == nil || v.Call.Captions != "window" && v.Call.Captions != "overlay")
}

// callLayout is the call's geometry with its captions where the novel buttons leave
// them: raised or taller by the band, the self view above them. With call.captions
// window, the call is fitted above the novel message window and its buttons.
func (g *Game) callLayout(sw, sh float64, call *data.EnsembleCall) callLayout {
	ui := &g.db.Game.UI
	if call != nil && call.Layout == "full" {
		base, strip := novelBase(ui, g.hasMaterial(), sw, sh)
		f := quickMenu(ui.QuickMenu, overlayBase(base, sh), strip, sw, sh, g.novelButtonWidths())
		top := f.win.y
		if len(f.buttons) > 0 {
			top = min(top, f.bar.y)
		}
		return callGeometryFull(sw, sh, top, call.Others == "side")
	}
	if call != nil && call.Captions == "window" {
		base, strip := novelBase(ui, g.hasMaterial(), sw, sh)
		f := quickMenu(ui.QuickMenu, base, strip, sw, sh, g.novelButtonWidths())
		top := f.win.y
		if len(f.buttons) > 0 {
			top = min(top, f.bar.y)
		}
		return sideOthers(callGeometryAbove(sw, sh, top-quickGap), call)
	}
	l := callGeometry(sw, sh)
	if ui.QuickMenu == "" {
		return sideOthers(l, call)
	}
	f := quickMenu(ui.QuickMenu, l.caption, false, sw, sh, g.novelButtonWidths())
	l.self.y += f.win.y - l.caption.y
	l.caption = f.win
	return sideOthers(l, call)
}

// sideOthers gives the strip's room to the stage when the participants stand in the
// side column (call.others: side); the captions and the self view keep their place
// at the stage's lower edge.
func sideOthers(l callLayout, call *data.EnsembleCall) callLayout {
	if call == nil || call.Others != "side" || l.strip.h <= 0 {
		return l
	}
	grow := l.strip.y + l.strip.h - (l.main.y + l.main.h)
	l.main.h += grow
	l.caption.y += grow
	l.self.y += grow
	l.strip = rectF{}
	return l
}

// callGeometryFull lays a full call out (call.layout: full): the maximized browser
// over the whole screen, the stage on the left running to the screen's lower edge
// under the message window whose top is windowTop, the people on it framed above
// that top, and the side column on the right (the chat transcript, or the others
// with call.others: side) above the window with the controls in a row under it.
// Without side, the others stand in a strip at the foot of the visible stage.
func callGeometryFull(sw, sh, windowTop float64, side bool) callLayout {
	k := sh / 720
	var l callLayout
	l.full = true
	l.frameBottom = sh
	const top, edge = 76.0, 16.0
	sideW := min(360.0, max(260, sw*.25))
	bottom := max(top+120, windowTop-12*k)
	mainW := sw - 2*edge - 12 - sideW
	l.main = rectF{edge, top, mainW, max(0, sh-edge-top)}
	l.seen = bottom - top
	if !side {
		stripH := 64 * k
		l.strip = rectF{edge, bottom - stripH, mainW, stripH}
		l.seen = max(0, l.strip.y-10*k-top)
	}
	l.controlR = 17 * k
	l.controlsY = bottom - l.controlR
	l.chat = rectF{sw - edge - sideW, top, sideW, max(0, l.controlsY-l.controlR-10*k-top)}
	l.controlDX = min(64, (sideW-2*l.controlR-24)/5)
	l.controlsX = l.chat.x + (sideW-5*l.controlDX)/2
	l.captionSize, l.captionLines = 20*k, 2
	l.caption = rectF{edge + 12, top + l.seen, mainW - 24, 0}
	selfW := min(220.0, mainW*.22, max(0, l.seen-24*k)*16/9*.9)
	selfH := selfW * 9 / 16
	l.self = rectF{edge + mainW - selfW - 14, top + l.seen - selfH - 12*k, selfW, selfH}
	return l
}

// callGeometryAbove lays a call out above bottom, for a call whose lines show in the
// message window under it: the speaker view and a lower strip of participants on the
// left, the messages on the right with the controls in a row under them, and no
// captions. The browser chrome keeps its height; the rest scales with the screen.
func callGeometryAbove(sw, sh, bottom float64) callLayout {
	k := sh / 720
	kx := min(1.0, sw/1280)
	var l callLayout
	top := min(104.0, sh*104/720)
	l.frameBottom = bottom
	stripH := 64 * k
	stripY := bottom - 12*k - stripH
	mainW := sw - 404*kx
	mainH := max(0, stripY-10*k-top)
	l.main = rectF{36 * kx, top, mainW, mainH}
	l.strip = rectF{36 * kx, stripY, mainW, stripH}
	l.controlR = 17 * min(k, kx)
	l.controlsY = bottom - 12*k - l.controlR
	l.chat = rectF{sw - 352*kx, top, 316 * kx, max(0, l.controlsY-l.controlR-10*k-top)}
	l.controlDX = min(64, (316*kx-2*l.controlR-24*kx)/5)
	l.controlsX = l.chat.x + (316*kx-5*l.controlDX)/2
	l.caption = rectF{48 * kx, top + mainH, sw - 440*kx, 0}
	selfW := min(200.0, mainW*.22, max(0, mainH-24*k)*16/9*.9)
	selfH := selfW * 9 / 16
	l.self = rectF{36*kx + mainW - selfW - 14*kx, top + mainH - selfH - 12*k, selfW, selfH}
	return l
}

// novelTextWidth is the width the novel window's lines wrap at.
func novelTextWidth(gm *data.Game, sw float64) float64 {
	base, strip := novelBase(&gm.UI, gm.UI.Material != nil, sw, gfx.BaseHeight)
	f := quickMenu(gm.UI.QuickMenu, base, strip, sw, gfx.BaseHeight, nil)
	return f.textR - f.textL - 40
}

// drawQuickBand draws the surface under the buttons: none on the strip or on the
// screen as before, a hairline inside the window, else a band of its own.
func (g *Game) drawQuickBand(dst *ebiten.Image, f novelFrame, ensemble bool) {
	b := f.bar
	switch {
	case f.place == "inside":
		gfx.FillRoundRect(dst, float32(f.textL-10), float32(b.y), float32(f.textR-f.textL+20), 1, 0, withAlpha(g.th.border, 0x60))
	case f.place == "" && !g.hasMaterial():
	case f.strip:
		gfx.FillRoundRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), 0, g.th.window)
	case ensemble && g.hasMaterial():
		gfx.FillRoundRect(dst, float32(b.x), float32(b.y), float32(b.w), float32(b.h), 14, color.NRGBA{22, 29, 40, 235})
	default:
		g.surfaceWindow(dst, b.x, b.y, b.w, b.h, "message")
	}
}

// drawNovelChoice draws the novel window's choices on a panel of the choice role
// (ui.material.opacities.choice deepens it).
func (g *Game) drawNovelChoice(dst *ebiten.Image, l *list) {
	g.traceList(l)
	g.surfaceWindow(dst, l.x, l.y, l.w, l.height(), "choice")
	l.drawRows(g, dst, true)
}
