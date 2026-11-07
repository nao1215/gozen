package play

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

type logLine struct {
	name, text string
	answer     bool
	// line is the line logged, or asked and pick the question and the option taken, so the
	// backlog shows them in the language the game shows now (ADR 0027).
	line  *event.Say
	asked *event.Choice
	pick  int
}

// shown is the entry's speaker and text in the language the game shows now.
func (l logLine) shown(db *data.DB) (name, text string) {
	switch {
	case l.line != nil:
		s := l.line.Reword(db)
		return s.Name, s.Text
	case l.asked != nil && l.pick >= 0 && l.pick < len(l.asked.Options):
		return "", "→ " + l.asked.Reword(db).Options[l.pick]
	}
	return l.name, l.text
}

func (g *Game) novel() bool { return g.db.Game.UI.Message == "novel" }

func (g *Game) remember(l logLine) {
	limit := g.db.Game.UI.Backlog
	if limit <= 0 && g.db.Game.SchemaVersion == data.LegacySchemaVersion {
		limit = 300
	}
	g.backlog = append(g.backlog, l)
	if len(g.backlog) > limit {
		g.backlog = g.backlog[len(g.backlog)-limit:]
	}
}

const (
	novelH    = 236.0
	novelSize = 28.0
)

type novelButton struct {
	label string
	b     button
}

var novelButtons = []novelButton{{"もどる", btnBack}, {"ログ", btnLog}, {"オート", btnAuto}, {"スキップ", btnSkip}, {"セーブ", btnSave}, {"ロード", btnLoad}, {"非表示", btnHide}, {"設定", btnConfig}}

// sceneSkipButton shows on the novel bar only while the running event can be skipped.
var sceneSkipButton = novelButton{"シーン送り", btnSceneSkip}

// bookButton opens the character book from the novel bar when game.yaml asks for it
// (ui.profiles); under --dev F6 opens the book without the button.
var bookButton = novelButton{"キャラクター", btnBook}

func (g *Game) novelButtons() []novelButton {
	out := novelButtons
	if g.hasProfiles() && g.db.Game.UI.Profiles {
		out = slices.Insert(slices.Clone(out), len(out)-1, bookButton)
	}
	if g.sceneSkippable() {
		return append([]novelButton{sceneSkipButton}, out...)
	}
	return out
}

// novelBarShown reports whether the buttons under the novel window show (and take clicks):
// not while a choice is shown with ui.hide_bar_on_choice.
func (g *Game) novelBarShown() bool {
	return !g.db.Game.UI.HideBarOnChoice || g.talk.choice == nil
}

func (g *Game) novelButtonRects() []image.Rectangle {
	f := g.novelFrame()
	out := make([]image.Rectangle, len(f.buttons))
	for i, r := range f.buttons {
		out[i] = r.buttonRect()
	}
	return out
}

func (g *Game) novelInput() bool {
	t := &g.talk
	if t.hidden {
		if g.in.ok() || g.in.cancel() || g.in.clicked() || g.in.pressedKey(btnHide) || g.in.esc || g.in.wheel != 0 {
			t.hidden = false
		}
		return true
	}
	press := map[button]bool{}
	for b := range numButtons {
		press[b] = g.in.pressed[b]
	}
	if g.in.click && g.novel() && t.active && g.novelBarShown() {
		buttons := g.novelButtons()
		for i, r := range g.novelButtonRects() {
			if g.in.mouse.In(r) {
				press[buttons[i].b] = true
				g.in.click = false
			}
		}
	}
	if g.in.wheel > 0 && (!t.active || t.choice == nil) {
		if g.db.Game.UI.WheelUp != "log" && g.canRollBack() {
			press[btnBack] = true
		} else {
			press[btnLog] = true
		}
	}
	switch {
	case len(g.backlog) > 0 && g.in.query(press[btnLog]):
		g.logView = &backlogView{}
		g.logView.top = g.logLastTop()
		return true
	case g.in.query(press[btnAuto]):
		g.autoRead = !g.autoRead
		return true
	case t.active && g.canToggleSkip() && g.in.query(press[btnSkip]):
		g.toggleSkip()
		return true
	case g.sceneSkippable() && g.in.query(press[btnSceneSkip]):
		g.sceneSkip()
		return true
	case g.canRollBack() && g.in.query(press[btnBack]):
		g.rollBack()
		return true
	case t.active && !g.replaying && g.in.query(press[btnSave]):
		m := newSaveMenu(g, true, func() {})
		if why := g.saveBlocked(); why != "" {
			g.sfx("buzzer")
			m.msg, m.msgT = why, 120
		}
		g.push(m)
		return true
	case t.active && !g.replaying && g.in.query(press[btnLoad]):
		m := newSaveMenu(g, false, func() {})
		m.inPlay = true
		g.push(m)
		return true
	case g.novel() && t.choice == nil && (g.in.query(press[btnHide]) || g.in.esc || g.in.rightClicked()):
		t.hidden = true
		return true
	case g.in.query(press[btnConfig]):
		g.push(newConfigScreen(g))
		return true
	case g.profilesInMenus() && !g.replaying && g.in.query(press[btnBook]):
		// the band's button (F6 itself is taken in Update)
		g.push(newProfiles(g, nil))
		return true
	}
	return false
}

func (g *Game) autoAdvance() bool {
	t := &g.talk
	if t.hidden || t.choice != nil || t.shown < t.total {
		return false
	}
	wait := float64(60+t.total*4) * g.cfg.autoScale()
	if g.battleAutoTalk() {
		return float64(t.full) > wait/g.autoTempo()
	}
	if t.voiceT > 0 && g.db.Game.UI.Voice.Waits() && t.age < t.voiceT+20 {
		return false // auto waits for the voice to end, and a breath after it
	}
	return g.autoRead && float64(t.full) > wait
}

func (t *talk) drawNovel(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	s := t.say
	if g.tr != nil && s.Face != "" {
		g.traceStandSpeaker(s.Face, g.stageBig(speakerKey(s), s.Face), speakerKey(s))
	}
	if k := g.stageAlpha(); k > 0 && s.Ensemble == "" {
		g.stageK = k
		t.drawNovelStage(g, dst)
		g.stageK = 1
	}
	frame := g.novelFrame()
	if def := g.db.Game.Screens[s.Ensemble]; s.Ensemble != "" && def != nil && def.Kind == "ensemble" {
		rect(dst, 0, 0, sw, sh, withAlpha(g.th.window, 255))
		screen := &authorScreen{name: s.Ensemble}
		height := frame.win.y - 40
		if def.View.Chat != nil {
			height = sh - 72
		}
		if def.View.Background != "" {
			g.materialBackground(dst, def.View.Background)
		} else {
			g.prepareMaterial(dst, "")
		}
		if def.View.Call != nil {
			screen.drawCall(g, dst)
		} else {
			screen.drawPlanning(g, dst, "ensemble", 24, 16, sw-48, height)
			g.drawPlanningText(dst, def.Title, 48, 28, sw-96, 22, 1, g.th.text)
		}
	}
	g.prepareMaterial(dst, "")
	if t.hidden {
		return
	}
	if t.bare {
		if t.choice != nil {
			g.drawNovelChoice(dst, t.choice)
		}
		return
	}
	if def := g.db.Game.Screens[s.Ensemble]; def != nil && captionsOnStage(def.View) {
		// Captions overlay only the video area; the transcript remains readable.
		c := frame.win
		cx, y, width, height := c.x, c.y, c.w, c.h
		size, k := t.size, 1.0
		if def.View.Call != nil {
			k = sh / 720
		}
		// A dark caption scrim protects text without tinting the camera itself.
		gfx.FillRoundRect(dst, float32(cx), float32(y), float32(width), float32(height), 10, color.NRGBA{15, 20, 26, 220})
		g.drawPlanningText(dst, s.Name, cx+16, y+8*k, width-32, 18*k, 1, g.th.nameTag)
		visible := ""
		left := t.shown
		for _, line := range t.pages[t.page] {
			runes := []rune(line)
			n := min(left, len(runes))
			visible += string(runes[:n])
			left -= n
		}
		g.drawPlanningText(dst, visible, cx+16, y+36*k, width-32, size, max(1, int((height-36*k)/gfx.LineHeight(size))), g.th.text)
	} else {
		w := frame.win
		y := w.y

		if frame.strip {
			// The v0.15 band: a soft fade into a full-width strip.
			for i := range 24 {
				a := uint8(float64(g.th.window.A) * float64(i) / 24)
				rect(dst, 0, y-24+float64(i), sw, 1, withAlpha(g.th.window, a))
			}
			gfx.FillRoundRect(dst, 0, float32(y), float32(sw), float32(w.h), 0, g.th.window)
		} else if def := g.db.Game.Screens[s.Ensemble]; def != nil && callOverlay(def.View) {
			// Over a full call the window is docked on the screen's lower edge: its
			// lower corners run off the screen, and call.window_opacity deepens it.
			if !g.materialSurfaceDeep(dst, w.x, y, w.w, w.h+24, g.th.window, "message", def.View.Call.WindowOpacity) {
				g.window(dst, w.x, y, w.w, w.h+24)
			}
		} else {
			g.surfaceWindow(dst, w.x, y, w.w, w.h, "message")
		}
		gfx.FillRoundRect(dst, float32(frame.textL-10), float32(y+58), float32(frame.textR-frame.textL+20), 1, 0, withAlpha(g.th.border, 0x80))
		if s.Name != "" {
			g.text(dst, s.Name, frame.textL, y+14, sizeL, g.th.nameTag)
		}
		left := t.shown
		size := novelSize
		if t.size > 0 {
			size = t.size
		}
		for i, ln := range t.pages[t.page] {
			r := []rune(ln)
			n := min(len(r), left)
			left -= n
			g.text(dst, string(r[:n]), frame.textL+20, y+74+float64(i)*size*1.5, size, g.th.text)
		}
		if t.shown >= t.total && t.choice == nil {
			bob := math.Sin(float64(g.tick)/6) * 4
			gfx.DrawText(dst, "▼", frame.textR-20, y+novelH-80+bob, sizeS, g.th.cursor)
		}

	}
	buttons := g.novelButtons()
	if g.novelBarShown() && len(frame.buttons) > 0 {
		g.drawQuickBand(dst, frame, s.Ensemble != "")
	}
	for i, r := range g.novelButtonRects() {
		if !g.novelBarShown() {
			break
		}
		c := g.th.dim
		if g.hasMaterial() {
			c = g.th.text
		}
		if buttons[i].b == btnAuto && g.autoRead || buttons[i].b == btnSkip && g.skipping {
			c = g.th.accent
		}
		if g.in.mouse.In(r) {
			c = g.th.cursor
		}
		if buttons[i].b == btnBack && !g.canRollBack() {
			c = g.th.disabled
		}
		// The label is centered on its hit rectangle (gfx.DrawText centers on x).
		gfx.DrawText(dst, lang.T(buttons[i].label), float64(r.Min.X)+float64(r.Dx())/2, float64(r.Min.Y)+2, sizeS, c)
	}
	if t.choice != nil {
		g.drawNovelChoice(dst, t.choice)
	}
}

func (t *talk) drawNovelStage(g *Game, dst *ebiten.Image) {
	_, sh := g.screen()
	s := t.say
	big := g.stageBig
	if s.Face != "" && !big(speakerKey(s), s.Face) {
		rise := math.Min(1, float64(t.age)/10)
		if t.still {
			rise = 1
		}
		ih := float64(g.pics.get(s.Face).Bounds().Dy())
		g.drawStanding(dst, s.Face, 140, sh-novelH-10, math.Min(ih, 260), float32(rise), false, nil)
		return
	}
	dim := func(v float32) *ebiten.ColorScale {
		if v == 0 {
			return nil
		}
		cs := &ebiten.ColorScale{}
		cs.Scale(v, v, v*1.1, 1)
		return cs
	}
	var figs []stageFigure
	if g.castOn {
		speaker := ""
		if s.Face != "" {
			speaker = speakerKey(s)
		}
		figs = g.castFigures(speaker)
	} else {
		figs = g.stageFigures(s.Face != "", big)
		for i := range figs {
			figs[i].x += g.figShakeX(figs[i].id)
		}
	}
	g.traceStandFigures(figs)
	for _, f := range figs {
		g.drawStage(dst, f.id, f.face, f.x, f.alpha, f.flip, dim(f.dim))
	}
}

type stageFigure struct {
	id    string
	face  string
	x     float64
	alpha float32
	flip  bool
	dim   float32

	leaving bool
}

func (g *Game) stageFigures(speaking bool, big func(id, face string) bool) []stageFigure {
	var out []stageFigure
	a, b := g.stage[0], g.stage[1]

	behind := g.cg.File != "" && !g.cg.Faces
	showA := big(a.id, a.face) && a.id != b.id && !behind
	showB := big(b.id, b.face) && !behind
	var xa, xb float64
	if showA {
		other := ""
		if showB {
			other = b.id
		}
		xa = g.sideOf(a.id, other)
	}
	if showB {
		other := ""
		if showA {
			other = a.id
		}
		xb = g.sideOf(b.id, other)
		if showA && xa == xb {
			xb = g.flipSide(b.id)
		}
	}
	sw, _ := g.screen()

	if k := 1 - float64(g.tick-g.stageLeftT)/12; k > 0 {
		for _, o := range g.stageLeft {
			if big(o.id, o.face) && o.id != a.id && o.id != b.id {
				v := float32(1)
				if o.dim {
					v = 0.75
				}
				out = append(out, stageFigure{o.id, o.face, sw * o.x, float32(k), o.flip, v, true})
			}
		}
	}
	if showA {
		v := float32(0.75)
		if speaking {
			v = 0.56
		}
		out = append(out, stageFigure{a.id, a.face, xa, 1, a.flip, v, false})
	}

	if o := g.stageOut; o.id != "" && !behind && big(o.id, o.face) && o.id != a.id && o.id != b.id {
		if k := 1 - float64(g.tick-g.stageOutT)/10; k > 0 {
			if x, ok := g.sides[o.id]; ok {
				out = append(out, stageFigure{o.id, o.face, sw * x, float32(k), o.flip, 0.56, true})
			}
		}
	}
	if showB {

		rise := float32(math.Min(1, float64(g.tick-b.entered)/10))
		v := float32(0)
		if !speaking {
			v = 0.75
		}
		out = append(out, stageFigure{b.id, b.face, xb, rise, b.flip, v, false})
	}
	return out
}

func (g *Game) stageAlpha() float32 {
	if g.cg.File != "" && !g.cg.Faces {
		if g.cgOld.File != "" && !g.cgOld.Faces && g.cgOldT > 0 {
			return 0
		}
		f := g.cg.FadeTicks()
		if f <= 0 || g.cgAge >= f {
			return 0
		}
		return 1 - float32(g.cgAge)/float32(f)
	}

	if (g.cg.File == "" || g.cg.Faces) && g.cgOld.File != "" && g.cgOldT > 0 && g.cgOut > 0 && !g.cgOld.Faces {
		return 1 - float32(g.cgOldT)/float32(g.cgOut)
	}
	return 1
}

// drawStage draws the figure id with the face picture face, or with its moving portrait
// in the picture's frame when the stage draws puppets (stagePuppet).
func (g *Game) drawStage(dst *ebiten.Image, id, face string, x float64, alpha float32, flip bool, cs *ebiten.ColorScale) {
	alpha *= g.stageK
	sw, sh := g.screen()
	var iw, ih float64
	p := g.stagePuppet(id, face)
	if p != nil {
		w, h := p.Rig().Size()
		iw, ih = float64(w), float64(h)
	} else {
		img := g.pics.get(face)
		iw, ih = float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	}
	stand := func(cx, bottom, h float64) {
		if p != nil {
			g.drawPuppet(dst, p, cx, bottom, h, alpha, flip, cs)
			return
		}
		g.drawStanding(dst, face, cx, bottom, h, alpha, flip, cs)
	}
	if f, ok := g.db.Game.UI.StageFraming[face]; ok {
		h := sh * f.Height / f.Head[2]
		if k := g.db.Game.UI.StageScale[face]; k > 0 {
			h *= k
		}
		headX := f.Head[0]
		if flip {
			headX = 1 - headX
		}
		// drawStanding anchors the canvas center/bottom. Translate those so
		// the authored head, including mirrored heads, stays at the requested
		// screen position. Large canvases deliberately clip at the viewport.
		cx := x + (0.5-headX)*iw*h/ih
		bottom := sh*(f.Top+g.db.Game.UI.StageDrop[face]) + (1-f.Head[1])*h
		stand(cx, bottom, h)
		return
	}
	bottom, h := sh+sh*0.85, sh*1.85-headRoom
	if iw/ih > 0.6 {
		h = math.Min(sh-novelH+60-headRoom, sw*0.42*ih/iw)
		bottom = headRoom + h
	}
	if k := g.db.Game.UI.StageScale[face]; k > 0 && k != 1 {
		top := bottom - h
		h *= k
		bottom = top + h
	}
	if d := g.db.Game.UI.StageDrop[face]; d != 0 {
		bottom += sh * d
	}
	w := iw * h / ih
	if w < sw-20 {
		x = math.Max(w/2+10, math.Min(sw-w/2-10, x))
	} else {
		x = sw / 2
	}
	stand(x, bottom, h)
}

// stageBig reports whether the figure id with the face picture face stands on the novel
// stage: a moving portrait that has the face does, whatever its size, without loading
// the picture.
func (g *Game) stageBig(id, face string) bool {
	if p := g.stagePuppet(id, face); p != nil {
		return true
	}
	return g.stagePicture(face)
}

// Explicit framing also opts small source pictures into the novel stage.
func (g *Game) stagePicture(face string) bool {
	if face == "" {
		return false
	}
	_, framed := g.db.Game.UI.StageFraming[face]
	return framed || g.pics.get(face).Bounds().Dy() >= 600
}

type backlogView struct{ top int }

func (g *Game) logEntryH(i int) float64 {
	sw, _ := g.screen()
	_, text := g.backlog[i].shown(g.db)
	return float64(max(1, len(gfx.Wrap(text, sizeM, sw-380))))*sizeM*1.4 + 30
}

func (g *Game) logLastTop() int {
	_, sh := g.screen()
	room := sh - 120
	i := len(g.backlog)
	// A line taller than the viewport must still be shown. Drawing already
	// permits the first entry to exceed the available height.
	if i > 0 {
		i--
		room -= g.logEntryH(i)
	}
	for i > 0 && room-g.logEntryH(i-1) >= 0 {
		room -= g.logEntryH(i - 1)
		i--
	}
	return i
}

func (v *backlogView) update(g *Game) {
	last := g.logLastTop()
	switch {
	case g.in.cancel() || g.in.pressedKey(btnLog) || g.in.ok() || g.in.clicked():
		g.logView = nil
	case g.in.up():
		v.top = max(0, v.top-1)
	case g.in.down():
		if v.top >= last {
			g.logView = nil
			return
		}
		v.top++
	}
}

func (v *backlogView) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	gfx.FillRoundRect(dst, 0, 0, float32(sw), float32(sh), 0, withAlpha(g.th.window, 0xf0))
	g.text(dst, lang.T("バックログ"), 60, 30, sizeL, g.th.accent)
	gfx.DrawTextRight(dst, lang.T("↑↓ ホイール: スクロール　X / クリック: もどる"), sw-60, 38, sizeS, g.th.dim)
	y := 100.0
	for i := v.top; i < len(g.backlog); i++ {
		l := g.backlog[i]
		l.name, l.text = l.shown(g.db)
		c := g.th.text
		if l.answer {
			c = g.th.accent
		}

		lines := gfx.Wrap(l.text, sizeM, sw-380)
		h := float64(max(1, len(lines)))*sizeM*1.4 + 30
		if y+h > sh-20 && i > v.top {
			break
		}
		if l.name != "" {
			ns := sizeM
			if nw := gfx.TextWidth(l.name, ns); nw > 184 {
				ns *= 184 / nw
			}
			g.text(dst, l.name, 80, y+(sizeM-ns)/2, ns, g.th.nameTag)
		}
		for j, ln := range lines {
			g.text(dst, ln, 280, y+float64(j)*sizeM*1.4, sizeM, c)
		}
		y += h
	}
}

func (g *Game) sideOf(id, other string) float64 {
	const left, right = 0.3, 0.7
	sw, _ := g.screen()
	if g.sides == nil {
		g.sides = map[string]float64{}
	}
	if x, ok := g.sides[id]; ok {
		return sw * x
	}
	x := left
	if o, ok := g.sides[other]; ok && o == left {
		x = right
	} else if !ok {
		for _, v := range g.sides {
			if v == left {
				x = right
			}
		}
	}
	g.sides[id] = x
	return sw * x
}

func (g *Game) flipSide(id string) float64 {
	sw, _ := g.screen()
	x := 0.3
	if g.sides[id] == 0.3 {
		x = 0.7
	}
	g.sides[id] = x
	return sw * x
}
