package play

import (
	"image"
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
// (ui.profiles); under --dev the P key opens the book without the button.
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

func (g *Game) novelButtonRects() []image.Rectangle {
	sw, sh := g.screen()
	var out []image.Rectangle
	x := sw - 40
	for _, btn := range slices.Backward(g.novelButtons()) {
		w := gfx.TextWidth(lang.T(btn.label), sizeS) + 28
		x -= w
		out = append([]image.Rectangle{image.Rect(int(x), int(sh-40), int(x+w), int(sh-10))}, out...)
		x -= 10
	}
	return out
}

func (g *Game) novelInput() bool {
	t := &g.talk
	if t.hidden {
		if g.in.ok() || g.in.cancel() || g.in.click || g.in.pressed[btnHide] || g.in.esc || g.in.wheel != 0 {
			t.hidden = false
		}
		return true
	}
	press := map[button]bool{}
	for b := range numButtons {
		press[b] = g.in.pressed[b]
	}
	if g.in.click && g.novel() && t.active {
		buttons := g.novelButtons()
		for i, r := range g.novelButtonRects() {
			if g.in.mouse.In(r) {
				press[buttons[i].b] = true
				g.in.click = false
			}
		}
	}
	if g.in.wheel > 0 && (!t.active || t.choice == nil) {
		if g.canRollBack() {
			press[btnBack] = true
		} else {
			press[btnLog] = true
		}
	}
	switch {
	case press[btnLog]:
		if len(g.backlog) > 0 {
			g.logView = &backlogView{}
			g.logView.top = g.logLastTop()
		}
		return true
	case press[btnAuto]:
		g.autoRead = !g.autoRead
		return true
	case press[btnSkip] && t.active:
		g.toggleSkip()
		return true
	case press[btnSceneSkip] && t.active:
		g.sceneSkip()
		return true
	case press[btnBack] && t.active:
		g.rollBack()
		return true
	case press[btnSave] && t.active && !g.replaying:
		g.push(newSaveMenu(g, true, func() {}))
		return true
	case press[btnLoad] && t.active && !g.replaying:
		m := newSaveMenu(g, false, func() {})
		m.inPlay = true
		g.push(m)
		return true
	case (press[btnHide] || g.in.esc || g.in.rclick) && g.novel() && t.choice == nil:
		t.hidden = true
		return true
	case press[btnConfig]:
		g.push(newConfigScreen(g))
		return true
	case press[btnBook] && g.profilesInMenus() && !g.replaying:
		g.push(newProfiles(g, nil))
		return true
	}
	return false
}

func (g *Game) autoAdvance() bool {
	t := &g.talk
	if t.choice != nil || t.shown < t.total {
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
		g.traceStandSpeaker(s.Face, g.pics.get(s.Face).Bounds().Dy() >= 600, speakerKey(s))
	}
	if k := g.stageAlpha(); k > 0 {
		g.stageK = k
		t.drawNovelStage(g, dst)
		g.stageK = 1
	}
	if t.hidden {
		return
	}
	if t.bare {
		if t.choice != nil {
			t.choice.draw(g, dst, true)
		}
		return
	}
	y := sh - novelH

	for i := range 24 {
		a := uint8(float64(g.th.window.A) * float64(i) / 24)
		rect(dst, 0, y-24+float64(i), sw, 1, withAlpha(g.th.window, a))
	}
	gfx.FillRoundRect(dst, 0, float32(y), float32(sw), float32(novelH), 0, g.th.window)
	gfx.FillRoundRect(dst, 120, float32(y+58), float32(sw-240), 1, 0, withAlpha(g.th.border, 0x80))
	if s.Name != "" {
		g.text(dst, s.Name, 130, y+14, sizeL, g.th.nameTag)
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
		g.text(dst, string(r[:n]), 150, y+74+float64(i)*size*1.5, size, g.th.text)
	}
	if t.shown >= t.total && t.choice == nil {
		bob := math.Sin(float64(g.tick)/6) * 4
		gfx.DrawText(dst, "▼", sw-150, y+novelH-80+bob, sizeS, g.th.cursor)
	}
	buttons := g.novelButtons()
	for i, r := range g.novelButtonRects() {
		c := g.th.dim
		if buttons[i].b == btnAuto && g.autoRead || buttons[i].b == btnSkip && g.skipping {
			c = g.th.accent
		}
		if g.in.mouse.In(r) {
			c = g.th.cursor
		}
		gfx.DrawText(dst, lang.T(buttons[i].label), float64(r.Min.X+r.Dx()/2), float64(r.Min.Y)+2, sizeS, c)
	}
	if t.choice != nil {
		t.choice.draw(g, dst, true)
	}
}

func (t *talk) drawNovelStage(g *Game, dst *ebiten.Image) {
	_, sh := g.screen()
	s := t.say
	big := func(face string) bool { return face != "" && g.pics.get(face).Bounds().Dy() >= 600 }
	if s.Face != "" && !big(s.Face) {
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
		g.drawStage(dst, f.face, f.x, f.alpha, f.flip, dim(f.dim))
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

func (g *Game) stageFigures(speaking bool, big func(string) bool) []stageFigure {
	var out []stageFigure
	a, b := g.stage[0], g.stage[1]

	behind := g.cg.File != "" && !g.cg.Faces
	showA := big(a.face) && a.id != b.id && !behind
	showB := big(b.face) && !behind
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
			if big(o.face) && o.id != a.id && o.id != b.id {
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

	if o := g.stageOut; o.id != "" && !behind && big(o.face) && o.id != a.id && o.id != b.id {
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

func (g *Game) drawStage(dst *ebiten.Image, face string, x float64, alpha float32, flip bool, cs *ebiten.ColorScale) {
	alpha *= g.stageK
	sw, sh := g.screen()
	img := g.pics.get(face)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
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
	g.drawStanding(dst, face, x, bottom, h, alpha, flip, cs)
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
	for i > 0 && room-g.logEntryH(i-1) >= 0 {
		room -= g.logEntryH(i - 1)
		i--
	}
	return i
}

func (v *backlogView) update(g *Game) {
	last := g.logLastTop()
	switch {
	case g.in.cancel() || g.in.pressed[btnLog] || g.in.ok() || g.in.click:
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
