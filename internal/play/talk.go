package play

import (
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

type talk struct {
	meeting *ensembleLog
	say     event.Say
	pages   [][]string
	page    int
	shown   int
	total   int
	choice  *list
	active  bool
	age     int
	full    int
	hidden  bool

	still  bool
	closed int
	bare   bool
	size   float64
	read   bool
	// keys are the keys of the choice's options (event.Choice.Keys).
	keys []uint64
	// voiceT is how many ticks the line's voice lasts (auto waits for it).
	voiceT int
	// asked is the question while choice is open, to ask it again in another language.
	asked event.Choice
	// held are the options of a muted choice (event.Choice.Muted) while choice
	// is its unmute button, and muted the ticks left before the player is taken
	// to keep quiet.
	held  *list
	muted int
	// mic is where the call drew its mic control while muted (micR 0 for
	// nowhere), so a click on it unmutes as the button does.
	mic  phonePoint
	micR float64
}

const (
	talkLines = 3
	talkSize  = 30.0
)

func (g *Game) talkBox() (x, y, w, h float64) {
	sw, sh := g.screen()
	if g.novel() {
		return 0, sh - novelH, sw, novelH
	}
	return 40, sh - 250, sw - 80, 230
}

func (t *talk) open(g *Game, s event.Say) {
	if s.Ensemble == "" {
		// A line outside a call ends the call's presentation-only camera changes.
		g.callCamera = nil
		g.callShare = nil
	}
	meeting := t.meeting
	if s.Ensemble == "" {
		meeting = nil
	} else if def := g.db.Game.Screens[s.Ensemble]; def != nil && def.View.Chat != nil {
		key := s.Ensemble + "/" + g.st.Planning.Date + "/" + strconv.Itoa(g.st.Planning.Phase)
		if meeting == nil || meeting.key != key {
			meeting = newEnsembleLog(g, s.Ensemble)
		}
		meeting.sync(g, def.View)
		// A reaction is authored public text, posted once per changed value.
		meeting.react(g, s.Actor)
	}
	still := g.tick-t.closed <= 1 && s.Face != "" && t.say.Face != "" && speakerKey(s) == speakerKey(t.say) && s.Right == t.say.Right
	*t = talk{meeting: meeting, say: s, active: true, still: still, read: g.isRead(s)}
	g.lineVoice(t, s, g.in.fast && (t.read || g.cfg.SkipAll))
	g.remember(logLine{name: s.Name, text: s.Text, line: &s})
	t.layout(g)
	if s.Face != "" {
		if p := g.stagePuppet(speakerKey(s), s.Face); p == nil {
			g.pics.get(s.Face) // a moving portrait on the stage does not need the picture
		}
		if g.castOn {
			g.castSays(s)
		} else {
			g.onStage(s)
		}
		g.openPuppet(s, still)
	}
}

type stageActor struct {
	id, face string
	flip     bool
	entered  int
	named    bool
}

func (g *Game) onStage(s event.Say) {
	k := speakerKey(s)
	switch {
	case k == g.stage[1].id, s.Face == g.stage[1].face && !g.stage[1].named:

		if old := g.stage[1].id; old != k {
			if x, ok := g.sides[old]; ok {
				g.sides[k] = x
				delete(g.sides, old)
			}
		}
		g.stage[1].id, g.stage[1].named = k, true
		g.stage[1].face, g.stage[1].flip = s.Face, s.Right
	case k == g.stage[0].id:
		a := g.stage[0]
		a.face, a.flip = s.Face, s.Right
		g.stage = [2]stageActor{g.stage[1], a}
	default:
		if g.stage[0].id != "" && g.stage[0].id != g.stage[1].id {
			g.stageOut, g.stageOutT = g.stage[0], g.tick
		}
		g.stage = [2]stageActor{g.stage[1], {id: k, face: s.Face, flip: s.Right, entered: g.tick, named: true}}
	}
}

func speakerKey(s event.Say) string {
	switch {
	case s.Actor != "":
		return "actor:" + s.Actor
	case s.Name != "":
		return "name:" + s.Name
	}
	return "face:" + s.Face
}

func (t *talk) setPage(i int) {
	t.page, t.shown, t.total, t.full = i, 0, 0, 0
	for _, ln := range t.pages[i] {
		t.total += len([]rune(ln))
	}
	if t.say.Instant {
		t.shown = t.total
	}
}

// layout breaks the line into the window's pages and shows the first.
func (t *talk) layout(g *Game) {
	t.pages, t.size = MessagePages(&g.db.Game, t.say.Text)
	if def := g.db.Game.Screens[t.say.Ensemble]; g.novel() && def != nil && captionsOnStage(def.View) {
		sw, sh := g.screen()
		width := sw - 48 - min(340.0, (sw-48)*.30) - 68
		size, rows := t.size, 2
		if def.View.Call != nil {
			l := callGeometry(sw, sh)
			width, rows = max(1, l.caption.w-32), max(1, int((l.caption.h-36*sh/720)/gfx.LineHeight(size)))
		}
		lines := gfx.Wrap(t.say.Text, size, width)
		t.pages = nil
		for len(lines) > 0 {
			n := min(rows, len(lines))
			t.pages = append(t.pages, lines[:n])
			lines = lines[n:]
		}
	}
	if len(t.pages) == 0 {
		t.pages = [][]string{{""}}
	}
	t.setPage(0)
}

func (t *talk) ask(g *Game, c event.Choice) { t.question(g, c, false) }

// question opens a choice; again asks the open one again (in another language) without
// logging or voicing its prompt a second time.
func (t *talk) question(g *Game, c event.Choice, again bool) {
	switch {
	case c.Prompt != "" && again:
		*t = talk{say: c.Question(), active: true, still: true}
		t.layout(g)
	case c.Prompt != "":
		t.open(g, c.Question())
	case t.say.Text != "" && len(t.pages) > 0 && (again || g.tick-t.closed <= 3):

		prev := *t
		*t = talk{say: prev.say, pages: prev.pages, size: prev.size, active: true, still: true}
		t.setPage(prev.page)
	default:
		// A choice right after a line of a call keeps the call on the screen, as the
		// line's own text would (the window is bare, the call stays behind it).
		ensemble := ""
		if g.tick-t.closed <= 3 {
			ensemble = t.say.Ensemble
		}
		*t = talk{active: true, bare: true, pages: [][]string{{""}}, meeting: t.meeting}
		t.say.Ensemble = ensemble
	}
	if c.Recruit != nil {
		t.say.Name = recruitName(c.Recruit)
	}
	t.shown = t.total
	t.keys = c.Keys
	t.asked = c
	var items []listItem
	for i, o := range c.Options {
		it := listItem{label: o}
		if i < len(c.Keys) && g.read[c.Keys[i]] {
			it.color = g.th.accent // taken before (ADR 0026)
		}
		items = append(items, it)
	}
	bx, by, bw, bh := g.talkBox()
	w := choiceWidth(&g.db.Game, c.Options)
	l := newList(items, bx+bw-w, 0, w, 6)
	l.y = by - l.height() - 16

	if !t.bare && len(t.pages) > 1 {
		text := c.Prompt
		if text == "" {
			for _, ln := range t.pages[t.page] {
				text += ln
			}
		}
		lines, sz, _ := PromptLines(&g.db.Game, text, c.Options)
		t.pages, t.size = [][]string{lines}, sz
		t.setPage(0)
		t.shown = t.total
	}
	if g.novel() {

		l.rowH = 40
		l.x = bw - 40 - w
		f := g.novelFrame()
		if f.place != "" {
			l.x = f.win.x + f.win.w - 40 - w
		}
		l.y = by + bh - 48 - l.height()
		if f.place != "" || !f.strip {
			l.y = f.bar.y - 2 - l.height()
		}

		switch {
		case g.db.Game.UI.ChoicePosition == "center":
			// In the middle of the screen, never over the window.
			sw, sh := g.screen()
			l.x = (sw - w) / 2
			l.y = min((sh-l.height())/2, f.win.y-16-l.height())
		case !t.bare && len(t.pages) > 0 && novelTextReaches(t.pages[t.page], t.size, l.x-30-(f.textL+20-150)):
			l.y = f.win.y - 32 - l.height()
		}
	}
	t.choice = l
	if c.Muted > 0 && !again {
		t.mute(int(math.Ceil(c.Muted * 60)))
	}
}

// mute holds the options behind an unmute button, in the last row of where
// they stand, for ticks.
func (t *talk) mute(ticks int) {
	l := t.choice
	gate := newList([]listItem{{label: lang.T("ミュートを解除")}}, l.x, 0, l.w, 1)
	gate.rowH, gate.size = l.rowH, l.size
	gate.y = l.y + l.height() - gate.height()
	t.held, t.choice, t.muted, t.micR = l, gate, max(1, ticks), 0
}

// answers is how many answers the open choice takes: its options, and
// staying quiet while it is muted.
func (t *talk) answers() int {
	if t.held != nil {
		return len(t.held.items) + 1
	}
	return len(t.choice.items)
}

// reword shows the open line or question again in the language the game shows now
// (ADR 0027): its text, its speaker's name and the options, keeping the page, the
// letters already shown and the option selected.
func (t *talk) reword(g *Game) {
	if !t.active {
		return
	}
	old := speakerKey(t.say)
	t.say = t.say.Reword(g.db)
	if k := speakerKey(t.say); k != old {
		g.renameOnStage(old, k)
	}
	if t.choice != nil {
		muted := 0
		if t.held != nil {
			muted = t.muted
		}
		sel, top := t.choice.sel, t.choice.top
		if !t.bare {
			// A question without a prompt stays under the last page of the line before it.
			t.layout(g)
			t.setPage(len(t.pages) - 1)
		}
		t.question(g, t.asked.Reword(g.db), true)
		if muted > 0 {
			t.mute(muted)
		} else {
			t.choice.sel, t.choice.top = sel, top
		}
		return
	}
	if t.bare {
		return
	}
	page, shown, full := t.page, t.shown, t.shown >= t.total
	t.layout(g)
	t.setPage(min(page, len(t.pages)-1))
	t.shown = min(shown, t.total)
	if full {
		t.shown = t.total
	}
}

// renameOnStage gives a speaker who stands on the stage the key of their name in another
// language, so their next line does not bring them on again.
func (g *Game) renameOnStage(old, k string) {
	for i := range g.stage {
		if g.stage[i].id == old {
			g.stage[i].id = k
		}
	}
	if x, ok := g.sides[old]; ok {
		g.sides[k] = x
		delete(g.sides, old)
	}
}

func novelTextReaches(lines []string, size, right float64) bool {
	if size <= 0 {
		size = novelSize
	}
	for _, ln := range lines {
		if 150+gfx.TextWidth(ln, size) > right {
			return true
		}
	}
	return false
}

func (t *talk) update(g *Game) (done bool, answer int) {
	t.age++
	g.stepPuppet(t)
	if g.recruit != nil && g.recruit.leave == 0 {
		minAge := 24
		if t.choice != nil {
			minAge = 54
		}
		if g.tick-g.recruit.start < minAge {
			return false, -1
		}
	}
	if g.novelInput() {
		return false, -1
	}
	if t.choice != nil {
		if t.held != nil {
			t.muted--
			if t.muted <= 0 {
				t.active, t.closed = false, g.tick
				g.remember(logLine{text: "→ " + lang.T("ミュートのまま"), answer: true})
				quiet := len(t.held.items)
				// The call stays up after the choice: its mic is no longer muted.
				t.held, t.micR = nil, 0
				return true, quiet
			}
			chose, _ := t.choice.update(g)
			if chose || g.in.click && t.micR > 0 && t.mic.Dist(g.in.mouse) <= t.micR {
				t.choice, t.held = t.held, nil
			}
			return false, -1
		}
		chose, _ := t.choice.update(g)
		if chose {
			t.active, t.closed = false, g.tick
			if k := t.choice.sel; k < len(t.keys) && !g.live.quiet {
				g.markKey(t.keys[k])
			}
			asked := t.asked
			g.remember(logLine{text: "→ " + t.choice.items[t.choice.sel].label, answer: true, asked: &asked, pick: t.choice.sel})
			return true, t.choice.sel
		}
		return false, -1
	}

	if g.skipping && (g.in.ok() || g.in.click) {
		g.stopSkip("input")
		return false, -1
	}
	fast := (g.in.fast || g.skipping) && (t.read || g.cfg.SkipAll || (g.rep != nil && g.rep.version == 1) || t.say.Text == "")
	if t.shown < t.total {
		t.shown += g.cfg.textStep()
		if fast {
			t.shown = t.total
		}
		if g.in.advance() && t.age > 4 {
			t.shown = t.total
		}
		return false, -1
	}
	t.full++
	if g.in.advance() || fast && t.age%4 == 0 || (!g.in.fast || fast) && g.autoAdvance() {
		if t.page+1 < len(t.pages) {
			t.setPage(t.page + 1)
			return false, -1
		}
		t.active, t.closed = false, g.tick
		if !g.live.quiet && (g.fast == nil || !g.fast.page) {
			g.markRead(t.say)
		}
		return true, -1
	}
	return false, -1
}

func (t *talk) draw(g *Game, dst *ebiten.Image) {
	if !t.active {
		return
	}
	if g.recruit != nil && g.recruit.leave == 0 {
		minAge := 24
		if t.choice != nil {
			minAge = 54
		}
		if g.tick-g.recruit.start < minAge {
			return
		}
	}
	if g.novel() {
		t.drawNovel(g, dst)
		return
	}
	if t.bare {
		if t.choice != nil {
			t.choice.draw(g, dst, true)
		}
		return
	}
	x, y, w, h := g.talkBox()
	sw, sh := g.screen()
	s := t.say
	if s.Ensemble != "" {
		if def := g.db.Game.Screens[s.Ensemble]; def != nil && def.Kind == "ensemble" {
			screen := &authorScreen{name: s.Ensemble}
			screen.drawPlanning(g, dst, "ensemble", 24, 16, sw-48, y-56)
			if def.View.Fullscreen {
				screen.drawPlanningHeader(g, dst)
			}
		}
	}
	if s.Ensemble == "" && s.Face != "" && (g.cg.File == "" || g.cg.Faces) && g.recruit == nil {
		img := g.pics.get(s.Face)
		ih := float64(img.Bounds().Dy())
		cx := x + 230
		if s.Right {
			cx = sw - 260
		}
		rise := math.Min(1, float64(t.age)/10)
		if t.still {
			rise = 1
		}
		bottom, tall := sh+120, sh*1.02
		if tp := g.db.Game.UI.TalkPortrait; tp != (data.TalkPortrait{}) {
			if tp.X > 0 {
				cx = sw * tp.X
				if s.Right {
					cx = sw * (1 - tp.X)
				}
			}
			if tp.Bottom > 0 {
				bottom = sh * tp.Bottom
			}
			if tp.Height > 0 {
				tall = sh * tp.Height
			}
		}
		if p, _ := g.talkPuppet(s); p != nil {
			// A moving portrait stands like a tall picture, whatever its size.
			g.drawPuppet(dst, p, cx, bottom, tall, float32(rise), s.Right, nil)
		} else if ih >= 600 {
			g.drawStanding(dst, s.Face, cx, bottom, tall, float32(rise), s.Right, nil)
		} else {
			g.drawStanding(dst, s.Face, cx, y+10, math.Min(ih, 280), float32(rise), false, nil)
		}
	}
	g.prepareMaterial(dst, "")
	g.window(dst, x, y, w, h)
	if s.Name != "" {
		nw := gfx.TextWidth(s.Name, sizeM) + 48
		nx := x + 28
		if s.Right && s.Face != "" {
			nx = x + w - nw - 28
		}
		if g.hasMaterial() {
			g.window(dst, nx, y-24, nw, 46)
		} else {
			gfx.FillRoundRect(dst, float32(nx), float32(y-24), float32(nw), 46, 14, g.th.nameTag)
		}
		g.text(dst, s.Name, nx+24, y-17, sizeM, g.th.text)
	}
	left := t.shown
	size := talkSize
	if t.size > 0 {
		size = t.size
	}
	for i, ln := range t.pages[t.page] {
		r := []rune(ln)
		n := min(len(r), left)
		left -= n
		g.text(dst, string(r[:n]), x+50, y+40+float64(i)*size*1.45, size, g.th.text)
	}
	if t.shown >= t.total && t.choice == nil {
		bob := math.Sin(float64(g.tick)/6) * 4
		gfx.DrawText(dst, "▼", x+w-50, y+h-48+bob, sizeS, g.th.cursor)
	}
	if t.choice != nil {
		t.choice.draw(g, dst, true)
	}
}
