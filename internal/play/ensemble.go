package play

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// ensembleCells maximizes camera area while keeping tiles at 4:3, including
// the name strip. Incomplete rows are centered and do not stretch their faces.
func ensembleCells(x, y, w, h float64, count int) []image.Rectangle {
	if count <= 0 || w <= 0 || h <= 0 {
		return nil
	}
	best, cols := 0.0, 1
	for c := 1; c <= count; c++ {
		rows := (count + c - 1) / c
		width := min((w-float64(c-1)*12)/float64(c), (h-float64(rows-1)*12)/float64(rows)*4/3)
		if width > best {
			best, cols = width, c
		}
	}
	if best <= 0 {
		return nil
	}
	height := best * 3 / 4
	rows := (count + cols - 1) / cols
	top := y + (h-float64(rows)*height-float64(rows-1)*12)/2
	out := make([]image.Rectangle, count)
	for i := range out {
		row := i / cols
		n := min(cols, count-row*cols)
		left := x + (w-float64(n)*best-float64(n-1)*12)/2 + float64(i%cols)*(best+12)
		yy := top + float64(row)*(height+12)
		out[i] = image.Rect(int(math.Round(left)), int(math.Round(yy)), int(math.Round(left+best)), int(math.Round(yy+height)))
	}
	return out
}

type ensembleMessage struct {
	name, text string
	system     bool
}
type ensembleMember struct {
	id, name, reaction string
	camera             bool
}
type ensembleLog struct {
	key       string
	members   []ensembleMember
	messages  []ensembleMessage
	reactions map[string]string
}

func (g *Game) ensembleMembers(v data.PlanningView) []ensembleMember {
	var members []ensembleMember
	for _, id := range v.Guests {
		if actor := g.db.Actors[id]; actor != nil {
			members = append(members, ensembleMember{id: id, name: actor.Name, camera: true})
		}
	}
	for _, id := range g.visibleResidents(v) {
		entity := g.db.Game.Planning.Entities[id]
		values := g.st.EntityValues(id, false)
		reaction, ok := values[v.Chat.Reaction].(string)
		if !ok {
			reaction = ""
		}
		camera := values[v.Camera] == true
		if on, ok := g.callCamera[id]; ok {
			camera = on
		} else if on, ok := g.callCamera[entity.Actor]; ok {
			camera = on
		}
		members = append(members, ensembleMember{id: id, name: entity.Name, camera: camera, reaction: reaction})
	}
	return members
}

func newEnsembleLog(g *Game, name string) *ensembleLog {
	log := &ensembleLog{key: name + "/" + g.st.Planning.Date + "/" + strconv.Itoa(g.st.Planning.Phase), reactions: map[string]string{}}
	log.sync(g, g.db.Game.Screens[name].View)
	return log
}

func (l *ensembleLog) post(name, text string, system bool) {
	if strings.TrimSpace(text) == "" {
		return
	}
	l.messages = append(l.messages, ensembleMessage{name, text, system})
	if len(l.messages) > 100 {
		l.messages = append([]ensembleMessage(nil), l.messages[len(l.messages)-100:]...)
	}
}

func (l *ensembleLog) sync(g *Game, v data.PlanningView) { l.update(g.ensembleMembers(v), v.Chat) }

// update records only transitions. Drawing and repeated refreshes never append.
func (l *ensembleLog) update(members []ensembleMember, labels *data.EnsembleChat) {
	for _, old := range l.members {
		if !slices.ContainsFunc(members, func(m ensembleMember) bool { return m.id == old.id }) {
			l.post(old.name, labels.Left, true)
			delete(l.reactions, old.id)
		}
	}
	for _, member := range members {
		i := slices.IndexFunc(l.members, func(m ensembleMember) bool { return m.id == member.id })
		if i < 0 {
			l.post(member.name, labels.Joined, true)
		}
		if i < 0 && !member.camera || i >= 0 && l.members[i].camera != member.camera {
			label := labels.CameraOff
			if member.camera {
				label = labels.CameraOn
			}
			l.post(member.name, label, true)
		}
	}
	l.members = append([]ensembleMember(nil), members...)
}

func (l *ensembleLog) react(g *Game, speaker string) {
	for _, m := range l.members {
		entity, ok := g.db.Game.Planning.Entities[m.id]
		if !ok || entity.Actor == speaker || m.reaction == "" || l.reactions[m.id] == m.reaction {
			continue
		}
		l.post(m.name, m.reaction, false)
		l.reactions[m.id] = m.reaction
		return // one authored response per spoken line, in stable participant order
	}
}

func (l *ensembleLog) draw(g *Game, dst *ebiten.Image, labels *data.EnsembleChat, x, y, w, h float64) {
	ink := color.NRGBA{48, 52, 57, 255}
	dim := color.NRGBA{100, 106, 113, 255}
	if g.db.Game.UI.Material != nil {
		ink, dim = g.th.text, g.th.dim
		g.window(dst, x, y, w, h)
	} else {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 12, color.NRGBA{250, 251, 252, 255})
	}
	g.drawPlanningText(dst, labels.Title, x+18, y+18, w-36, 20, 1, ink)
	rect(dst, x+16, y+53, w-32, 1, color.NRGBA{220, 224, 228, 255})
	// Bottom-aligned newest entries: compute wrapped heights before selecting the
	// visible suffix. No partial row or overflowing text is drawn.
	heights := make([]float64, len(l.messages))
	first := len(heights)
	available := h - 80
	for i := len(heights) - 1; i >= 0; i-- {
		m := l.messages[i]
		text := m.text
		size := 16.0
		head := 28.0
		if m.system {
			text = m.name + " " + text
			size = 14
			head = 0
		}
		lines := len(gfx.Wrap(text, size, w-36))
		heights[i] = head + float64(min(lines, 4))*gfx.LineHeight(size) + 14
		if available < heights[i] {
			break
		}
		available -= heights[i]
		first = i
	}
	yy := y + 66
	for i := first; i < len(l.messages); i++ {
		m := l.messages[i]
		if m.system {
			g.drawPlanningText(dst, m.name+" "+m.text, x+18, yy, w-36, 14, 4, dim)
		} else {
			g.drawPlanningText(dst, m.name, x+18, yy, w-36, 15, 1, g.th.accent)
			g.drawPlanningText(dst, m.text, x+18, yy+28, w-36, 16, 4, ink)
		}
		yy += heights[i]
	}
}

// rectF is a logical rectangle of the call layout.
type rectF struct{ x, y, w, h float64 }

func (r rectF) rect() image.Rectangle {
	return image.Rect(int(math.Round(r.x)), int(math.Round(r.y)), int(math.Round(r.x+r.w)), int(math.Round(r.y+r.h)))
}

// callLayout is the geometry of a call for one logical screen size. Drawing,
// the captions and the line breaking share it. The browser chrome keeps its
// height; everything under it is laid out as a fraction of the screen height,
// so a 1280x720 screen keeps the original positions.
type callLayout struct {
	frameBottom          float64
	main, strip, chat    rectF
	self                 rectF
	controlsY, controlR  float64
	caption              rectF
	captionSize          float64
	captionLines         int
	controlsX, controlDX float64
	// full is a call over the whole screen (call.layout: full); seen is the height
	// of the stage above the message window, where its people are framed.
	full bool
	seen float64
	// controlsDY steps the controls down a column (a folded side column) instead of
	// across a row.
	controlsDY float64
}

// foldSide folds the side column of a call of two (call.others: side with at most one
// participant besides the self view): the
// stage, its strip and its captions widen to the column's right edge but for a slim
// column of the controls, stacked from the column's top; the self view follows the
// stage's right edge.
func foldSide(l callLayout) callLayout {
	right := l.chat.x + l.chat.w
	col := 2*l.controlR + 16
	grow := right - col - 12 - (l.main.x + l.main.w)
	l.main.w += grow
	l.caption.w += grow
	l.self.x += grow
	if l.strip.h > 0 {
		l.strip.w += grow
	}
	l.controlsX, l.controlDX = right-col/2, 0
	l.controlsY, l.controlsDY = l.chat.y+l.controlR+8, 2*l.controlR+10
	l.chat = rectF{}
	return l
}

func callGeometry(sw, sh float64) callLayout {
	k := sh / 720
	kx := min(1.0, sw/1280)
	var l callLayout
	l.frameBottom = sh - 56*k
	l.controlsY = l.frameBottom - 30*k
	l.controlR = 21 * min(k, kx)
	top := min(104.0, sh*104/720)
	stripH := 86 * k
	stripY := l.controlsY - 28*k - stripH
	mainW := sw - 404*kx
	mainH := max(0, stripY-12*k-top)
	l.main = rectF{36 * kx, top, mainW, mainH}
	l.strip = rectF{36 * kx, stripY, mainW, stripH}
	l.chat = rectF{sw - 352*kx, top, 316 * kx, max(0, stripY+stripH-top)}
	l.captionSize = 20 * k
	l.captionLines = 2
	capH := 96 * k
	l.caption = rectF{48 * kx, top + mainH - capH, sw - 440*kx, capH}
	// The self view sits in the stage's lower right, above the captions.
	selfW := min(240.0, mainW*.24, max(0, mainH-capH-12*k-14)*16/9*.9)
	selfH := selfW * 9 / 16
	l.self = rectF{36*kx + mainW - selfW - 14*kx, l.caption.y - selfH - 12*k, selfW, selfH}
	l.controlsX, l.controlDX = sw/2-160*kx, 64*kx
	return l
}

// callColors fills the call's palette from its authored colors.
func callColors(call *data.EnsembleCall) (body, tile, speaking, control, leave color.NRGBA) {
	body, tile, speaking = color.NRGBA{29, 32, 38, 255}, color.NRGBA{48, 53, 62, 255}, color.NRGBA{116, 210, 243, 255}
	control, leave = color.NRGBA{59, 65, 75, 255}, color.NRGBA{192, 55, 65, 255}
	if c := call.Colors; c != nil {
		body, tile, speaking = c.Body.Or(body), c.Tile.Or(tile), c.Speaking.Or(speaking)
		control, leave = c.Control.Or(control), c.Leave.Or(leave)
	}
	return
}

// memberActor is the actor who speaks for a call participant.
func (g *Game) memberActor(m ensembleMember) string {
	if e, ok := g.db.Game.Planning.Entities[m.id]; ok {
		return e.Actor
	}
	return m.id
}

// callStage picks the participant shown large: whoever speaks now, camera or
// not, then whoever spoke the last line of this call, then the first camera.
func (g *Game) callStage(name string, members []ensembleMember) int {
	speaker := ""
	if g.talk.say.Ensemble == name {
		// A narrated line of the call (no speaker) keeps whoever spoke last on
		// the stage instead of jumping back to the first camera.
		if g.callLast == nil {
			g.callLast = map[string]string{}
		}
		// The caller themself (a speaker who is not a participant, such as the
		// self view) keeps the stage too: their own view shows that they speak.
		for _, m := range members {
			if a := g.talk.say.Actor; a != "" && g.memberActor(m) == a {
				g.callLast[name] = a
			}
		}
		speaker = g.callLast[name]
	}
	if speaker != "" {
		for i, m := range members {
			if g.memberActor(m) == speaker {
				return i
			}
		}
	}
	for i, m := range members {
		if m.camera {
			return i
		}
	}
	return -1
}

// drawCall places a speaker view inside an authored desktop browser. Presentation
// time never consumes simulation randomness or changes observations.
func (s *authorScreen) drawCall(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	v := g.db.Game.Screens[s.name].View
	l := g.callLayout(sw, sh, v.Call)
	body, _, _, control, leave := callColors(v.Call)
	ch := g.chrome()
	if l.full {
		g.drawCallBrowserFull(dst, s.name, v.Call.Address, body)
	} else {
		g.drawMaterialWallpaper(dst)
		g.desktopFrame(dst, l.frameBottom, 74, body)
		gfx.FillRoundRect(dst, 34, 23, 290, 30, 8, ch.tab)
		g.drawPlanningText(dst, g.db.Game.Screens[s.name].Title, 48, 28, 264, 14, 1, ch.title)
		g.text(dst, ch.nav, 38, 59, 16, ch.text)
		gfx.FillRoundRect(dst, 126, 56, float32(sw-270), 26, 13, ch.field)
		g.text(dst, v.Call.Address, 146, 60, 14, ch.address)
	}
	all := g.ensembleMembers(v)
	members := make([]ensembleMember, 0, len(all))
	var self *ensembleMember
	for i := range all {
		if v.Call.Self != "" && all[i].id == v.Call.Self {
			self = &all[i]
			continue
		}
		members = append(members, all[i])
	}
	g.pics.warm(g.callPictures(v, all)...)
	callerSpeaks := g.callerSpeaking(s.name, v, all)
	// Who stands on the stage is known before anything is drawn, so that an empty side
	// list (call.others: side) folds and the stage widens first.
	sharing := g.callSharer(all) >= 0 && l.main.w > 0 && l.main.h > 0
	selfStage := !sharing && g.callSelfOnStage(s.name, v, all, self)
	active := -1
	if !selfStage && !sharing {
		active = g.callStage(s.name, members)
	}
	// Others remain in a single strip or the side list; the one on the stage leaves it.
	others := make([]ensembleMember, 0, len(members))
	for i, m := range members {
		if i != active {
			others = append(others, m)
		}
	}
	// A call of two folds the side list for good, not line by line: while the caller
	// takes the stage, the one participant stands in the small view instead.
	folded := v.Call.Others == "side" && len(members) <= 1
	if folded {
		l = foldSide(l)
	}
	// call_view.share: the shared screen takes the stage (call_share.go).
	sharing = g.drawCallShare(dst, v, all, l)
	if selfStage {
		// call.self_stage: the caller's own view takes the stage while they speak.
		g.callerSpeaks = true
		g.drawCallSelf(dst, v, self, l.main, l.seen, true)
		g.callerSpeaks = false
	}
	if active >= 0 {
		g.drawCallMember(dst, v, members[active], l.main.x, l.main.y, l.main.w, l.main.h, l.seen, true)
	}
	if l.strip.h > 0 {
		n := len(others)
		tileW := min(142.0*l.strip.h/86, (l.strip.w-float64(max(0, n-1))*8)/float64(max(1, n)))
		left := l.strip.x + (l.strip.w-(tileW+8)*float64(n)+8)/2
		for _, m := range others {
			g.drawCallMember(dst, v, m, left, l.strip.y, tileW, l.strip.h, 0, false)
			left += tileW + 8
		}
	}
	if folded && selfStage && len(others) == 1 {
		r := l.self
		gfx.FillRoundRect(dst, float32(r.x-2), float32(r.y-2), float32(r.w+4), float32(r.h+4), 8, color.NRGBA{0, 0, 0, 90})
		g.drawCallMember(dst, v, others[0], r.x, r.y, r.w, r.h, 0, false)
		others = nil
	}
	if v.Call.Self != "" && !selfStage && !sharing {
		g.callerSpeaks = callerSpeaks
		g.drawCallSelf(dst, v, self, l.self, 0, false)
		g.callerSpeaks = false
	}
	log := newEnsembleLog(g, s.name)
	if g.talk.active && g.talk.say.Ensemble == s.name && g.talk.meeting != nil {
		log = g.talk.meeting
	}
	switch {
	case v.Call.Others == "side" && !folded:
		g.drawCallOthers(dst, v, others, log, l.chat)
	case v.Call.Others == "side":
	case v.Chat != nil:
		log.draw(g, dst, v.Chat, l.chat.x, l.chat.y, l.chat.w, l.chat.h)
	}
	for i, c := range v.Call.Controls {
		cx, cy := l.controlsX+float64(i)*l.controlDX, l.controlsY+float64(i)*l.controlsDY
		tint := control
		if c.ControlLeave(i, len(v.Call.Controls)) {
			tint = leave
		}
		tint = c.Color.Or(tint)
		gfx.FillCircle(dst, float32(cx), float32(cy), float32(l.controlR), tint, true)
		drawCallControl(dst, c.ControlIcon(i), cx, cy, l.controlR/21)
	}
}

// callPictures are the pictures a call draws now: each participant's portrait and
// backdrop with the camera on (and the portrait's mouth and blink variants), the face
// with it off, and the self view's picture.
func (g *Game) callPictures(v data.PlanningView, all []ensembleMember) []string {
	var out []string
	add := func(path string) {
		if path == "" {
			return
		}
		out = append(out, path)
		if m := v.Call.Mouth[path]; m != "" {
			out = append(out, m)
		}
		if b := v.Call.Blink[path]; b != "" {
			out = append(out, b)
		}
	}
	for _, m := range all {
		values := g.st.EntityValues(m.id, false)
		text := func(key string) string {
			s, ok := values[key].(string)
			if !ok {
				return ""
			}
			return s
		}
		portrait, bg, icon := text(v.Portrait), text(v.Backdrop), text(v.Icon)
		if slices.Contains(v.Guests, m.id) {
			portrait, bg = "", v.GuestBackdrops[m.id]
			if a := g.db.Actors[m.id]; a != nil {
				portrait = g.db.FacePicture(a, a.Face, "")
			}
		}
		if m.camera {
			add(portrait)
			add(bg)
		}
		if !m.camera || v.Call.Others == "side" {
			if icon == "" {
				icon = portrait
			}
			add(icon)
		}
	}
	if g.callShare != nil {
		out = append(out, g.callShare.Picture)
	}
	if self := v.Call.Self; self != "" {
		if a := g.db.Actors[self]; a != nil {
			add(g.db.FacePicture(a, a.Face, ""))
		} else if _, ok := g.db.Game.Planning.Entities[self]; !ok {
			add(self)
		}
	}
	return out
}

// callerSpeaking reports whether the caller speaks the line shown: its speaker is in
// this call but not one of its participants (the self view stands for them).
func (g *Game) callerSpeaking(name string, v data.PlanningView, all []ensembleMember) bool {
	if v.Call.Self == "" || !g.talk.active || g.talk.say.Ensemble != name || g.talk.say.Actor == "" {
		return false
	}
	for _, m := range all {
		if g.memberActor(m) == g.talk.say.Actor {
			return false
		}
	}
	return true
}

// callSelfOnStage reports whether call.self_stage puts the self view on the stage now:
// the caller speaks, or self is a participant and speaks.
func (g *Game) callSelfOnStage(name string, v data.PlanningView, all []ensembleMember, self *ensembleMember) bool {
	if !v.Call.SelfStage {
		return false
	}
	if g.callerSpeaking(name, v, all) {
		return true
	}
	return self != nil && g.talk.active && g.talk.say.Ensemble == name && g.talk.say.Actor != "" && g.memberActor(*self) == g.talk.say.Actor
}

// drawCallSelf draws the player's own view: small over the speaker view, or
// large on the stage (call.self_stage). self is the participant when call.self
// names a visible one; otherwise call.self is an actor or a picture shown with
// its camera on.
func (g *Game) drawCallSelf(dst *ebiten.Image, v data.PlanningView, self *ensembleMember, r rectF, seen float64, large bool) {
	if r.w <= 0 || r.h <= 0 {
		return
	}
	if !large {
		gfx.FillRoundRect(dst, float32(r.x-2), float32(r.y-2), float32(r.w+4), float32(r.h+4), 8, color.NRGBA{0, 0, 0, 90})
	}
	label := data.Render(g.db.Tr(v.Call.SelfLabel), g.st.Env())
	if self != nil {
		m := *self
		if label != "" {
			m.name = label
		}
		g.drawCallMember(dst, v, m, r.x, r.y, r.w, r.h, seen, large)
		return
	}
	_, tile, _, _, _ := callColors(v.Call)
	rect(dst, r.x, r.y, r.w, r.h, tile)
	picture := v.Call.Self
	if a := g.db.Actors[v.Call.Self]; a != nil {
		picture = g.db.FacePicture(a, a.Face, "")
		if label == "" {
			label = a.Name
		}
	} else if e, ok := g.db.Game.Planning.Entities[v.Call.Self]; ok {
		// A resident who is not in the call now keeps an empty self view.
		picture = ""
		if label == "" {
			label = e.Name
		}
	}
	speaking := g.callerSpeaks || g.talk.active && g.talk.say.Actor != "" && g.talk.say.Actor == v.Call.Self
	g.callTorso(dst, picture, v.Call, v.Call.Self, speaking, r.x, r.y, r.w, r.h, seen)
	if speaking {
		_, _, ring, _, _ := callColors(v.Call)
		edge := 2.0
		if large {
			edge = 3
		}
		g.drawSpeakingEdge(dst, r.x, r.y, r.w, r.h, edge, ring)
	}
	// On the stage the name stays off while a line is shown, as a participant's does.
	if label != "" && (!large || !g.talk.active) {
		foot := r.y + seenOr(seen, r.h)
		rect(dst, r.x, foot-22, r.w, 22, color.NRGBA{18, 24, 31, 180})
		g.drawPlanningText(dst, label, r.x+6, foot-20, r.w-12, 13, 1, color.White)
	}
}

// seen is the height of the view not under the message window (zero or less, all of
// it): its person is framed in it, and its name stands at its foot.
func (g *Game) drawCallMember(dst *ebiten.Image, v data.PlanningView, m ensembleMember, x, y, w, h, seen float64, large bool) {
	values := g.st.EntityValues(m.id, false)
	actor := g.memberActor(m)
	portrait, ok := values[v.Portrait].(string)
	if !ok {
		portrait = ""
	}
	bg, ok := values[v.Backdrop].(string)
	if !ok {
		bg = ""
	}
	icon, ok := values[v.Icon].(string)
	if !ok {
		icon = ""
	}
	badge, ok := values[v.Badge].(string)
	if !ok {
		badge = ""
	}
	if slices.Contains(v.Guests, m.id) {
		if a := g.db.Actors[m.id]; a != nil {
			portrait = g.db.FacePicture(a, a.Face, "")
		}
		bg = v.GuestBackdrops[m.id]
	}
	_, tile, ring, _, _ := callColors(v.Call)
	speaking := g.talk.active && g.talk.say.Actor == actor
	pulse := math.Sin(float64(g.tick)/24) * math.Sin(float64(g.tick)/24)
	rect(dst, x, y, w, h, tile)
	if m.camera {
		if bg != "" {
			g.planningBackdrop(dst, bg, values[v.Mask] == true, x, y, w, h)
		}
		g.callTorso(dst, portrait, v.Call, m.id, speaking, x, y, w, h, seen)
	} else {
		// A camera-off participant shows a round face, larger on the stage.
		radius := min(28, h*.30)
		cy := y + (h-20)/2
		if large {
			// Centered above the captions, which cover the lower quarter of the stage.
			fh := seenOr(seen, h)
			radius = max(1, min(fh*.24, w*.18))
			cy = y + fh*.38
		}
		if speaking {
			// Rings breathe outward while the line is spoken.
			for i, alpha := range []uint8{150, 90} {
				grow := radius * (.10 + .10*float64(i) + .08*pulse)
				gfx.StrokeCircle(dst, float32(x+w/2), float32(cy), float32(radius+grow), float32(max(2, radius*.05)), withAlpha(ring, alpha), true)
			}
		}
		g.planningFace(dst, icon, x+w/2, cy, radius)
	}
	if speaking {
		edge := 2.0
		if large {
			edge = 3
		}
		g.drawSpeakingEdge(dst, x, y, w, h, edge, ring)
	}
	name := m.name
	if badge != "" && large {
		name += " · " + badge
	}
	if !large || !g.talk.active {
		foot := y + seenOr(seen, h)
		rect(dst, x, foot-22, w, 22, color.NRGBA{18, 24, 31, 180})
		g.drawPlanningText(dst, name, x+6, foot-20, w-12, 13, 1, color.White)
	}
	if large && speaking && g.talk.age < 12 {
		rect(dst, x, y, w, h, color.NRGBA{25, 30, 38, uint8(min(100, max(0, 100*(12-g.talk.age)/12)))})
	}
}

// drawSpeakingWave is the three-bar level mark in a speaker's top right corner.
// right and top are its outer corner; k scales it with the tile.
func (g *Game) drawSpeakingWave(dst *ebiten.Image, right, top, k float64, tint color.NRGBA) {
	size := 22 * k
	cx, cy := right-size/2, top+size/2
	gfx.FillCircle(dst, float32(cx), float32(cy), float32(size/2), tint, true)
	for i := range 3 {
		level := .35 + .65*math.Abs(math.Sin(float64(g.tick)/(5+float64(i)*1.7)+float64(i)))
		bh := size * .55 * level
		bx := cx + (float64(i)-1)*size*.22
		gfx.FillRoundRect(dst, float32(bx-size*.06), float32(cy-bh/2), float32(size*.12), float32(bh), float32(size*.06), color.White)
	}
}

// callMotion is deterministic and anchored at the detected face center.
func callMotion(id string, tick int, speaking bool) (dx, dy, angle float64, blink, mouth bool) {
	phase := 0.0
	for _, r := range id {
		phase += float64(r) * .173
	}
	t := float64(tick) / 60
	dx = math.Sin(t*.83+phase) * 1.4
	dy = math.Sin(t*1.7+phase) * 1.1
	angle = math.Sin(t*.7+phase) * .004
	if speaking {
		dy += math.Sin(t*5.1+phase) * 2.4
		angle += math.Sin(t*2.4+phase) * .006
	}
	blink = math.Mod(t+phase, 4.3) < .15
	mouth = speaking && tick%14 < 7
	return
}

// seen bounds the framing as in drawCallMember; the picture still runs to the
// view's lower edge, under the message window.
func (g *Game) callTorso(dst *ebiten.Image, path string, call *data.EnsembleCall, id string, speaking bool, x, y, w, h, seen float64) {
	if path == "" {
		return
	}
	dx, dy, angle, blink, mouth := callMotion(id, g.tick, speaking)
	if call.Still {
		dx, dy, angle = 0, 0, 0
	}
	variant := path
	if mouth && call.Mouth[path] != "" {
		variant = call.Mouth[path]
	}
	if blink && call.Blink[path] != "" {
		variant = call.Blink[path]
	}
	img := g.pics.get(variant)
	if img == missing {
		img = g.pics.get(path)
	}
	face := g.pics.head(path)
	g.pics.mu.Lock()
	if f, ok := g.pics.found[path]; ok && f.Box != nil && f.Size[0] > 0 && f.Size[1] > 0 {
		sx, sy := float64(img.Bounds().Dx())/float64(f.Size[0]), float64(img.Bounds().Dy())/float64(f.Size[1])
		face = image.Rect(int(float64(f.Box[0])*sx), int(float64(f.Box[1])*sy), int(float64(f.Box[2])*sx), int(float64(f.Box[3])*sy))
	}
	g.pics.mu.Unlock()
	fh := seenOr(seen, h)
	scale, left, top := portraitTransform(img.Bounds(), face, x, y+fh*.22, w, fh*.48)
	cx, cy := x+w/2, y+fh*.46
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(left-cx, top-cy)
	op.GeoM.Rotate(angle)
	op.GeoM.Translate(cx+dx*fh/400, cy+dy*fh/400)
	clip := image.Rect(int(x*gfx.Scale()), int(y*gfx.Scale()), int((x+w)*gfx.Scale()), int((y+h)*gfx.Scale())).Intersect(dst.Bounds())
	if !clip.Empty() {
		if target, ok := dst.SubImage(clip).(*ebiten.Image); ok {
			gfx.DrawImage(target, img, op)
		}
	}
}

// drawCallControl uses original geometric marks, independent of font glyph coverage.
// k scales the mark with its round button.
func drawCallControl(dst *ebiten.Image, icon string, x, y, k float64) {
	line := func(a, b, c, d float64) {
		gfx.StrokeLine(dst, float32(x+a*k), float32(y+b*k), float32(x+c*k), float32(y+d*k), float32(1.8*k), color.White, true)
	}
	switch icon {
	case "mic":
		gfx.FillRoundRect(dst, float32(x-3*k), float32(y-9*k), float32(6*k), float32(12*k), float32(3*k), color.White)
		line(-7, 0, -7, 4)
		line(-7, 4, 7, 4)
		line(7, 4, 7, 0)
		line(0, 4, 0, 10)
		line(-4, 10, 4, 10)
	case "camera":
		gfx.FillRoundRect(dst, float32(x-10*k), float32(y-6*k), float32(14*k), float32(12*k), float32(2*k), color.White)
		line(5, -3, 10, -6)
		line(10, -6, 10, 6)
		line(10, 6, 5, 3)
	case "share":
		line(-10, -7, 10, -7)
		line(10, -7, 10, 8)
		line(10, 8, -10, 8)
		line(-10, 8, -10, -7)
		line(0, 5, 0, -3)
		line(0, -3, -4, 1)
		line(0, -3, 4, 1)
	case "hand":
		for j := range 4 {
			line(float64(j*4-7), 4, float64(j*4-7), float64(-5-j%2*3))
		}
		line(-7, 4, -10, 0)
		line(-10, 0, -12, 2)
		line(-12, 2, -7, 10)
		line(-7, 10, 5, 10)
		line(5, 10, 5, 4)
	case "chat":
		line(-9, -7, 9, -7)
		line(9, -7, 9, 5)
		line(9, 5, -3, 5)
		line(-3, 5, -8, 10)
		line(-8, 10, -8, 5)
		line(-8, 5, -9, -7)
		line(-5, -2, 5, -2)
	case "leave":
		line(-10, 2, -5, -3)
		line(-5, -3, 5, -3)
		line(5, -3, 10, 2)
		line(-10, 2, -10, 6)
		line(10, 2, 10, 6)
	}
}

// drawSpeakingEdge is the glowing edge and the level mark of a view whose
// person is speaking.
func (g *Game) drawSpeakingEdge(dst *ebiten.Image, x, y, w, h, edge float64, ring color.NRGBA) {
	pulse := math.Sin(float64(g.tick)/24) * math.Sin(float64(g.tick)/24)
	c := withAlpha(ring, uint8(180+60*pulse))
	rect(dst, x, y, w, edge, c)
	rect(dst, x, y+h-edge, w, edge, c)
	rect(dst, x, y, edge, h, c)
	rect(dst, x+w-edge, y, edge, h, c)
	g.drawSpeakingWave(dst, x+w-10, y+10, min(1, h/86), ring)
}

// seenOr is the framing height of a view: seen when it is set and within h, else h.
func seenOr(seen, h float64) float64 {
	if seen > 0 && seen < h {
		return seen
	}
	return h
}

// drawCallBrowserFull draws the maximized browser of a full call (call.layout: full):
// the call's body over the whole screen and the tab and the address bar across its top.
func (g *Game) drawCallBrowserFull(dst *ebiten.Image, name, address string, body color.NRGBA) {
	sw, sh := g.screen()
	ch := g.chrome()
	rect(dst, 0, 0, sw, sh, body)
	rect(dst, 0, 0, sw, 64, ch.bar)
	rect(dst, 0, 64, sw, 1, withAlpha(ch.frame, 120))
	gfx.FillRoundRect(dst, 12, 7, 290, 26, 8, ch.tab)
	g.drawPlanningText(dst, g.db.Game.Screens[name].Title, 26, 11, 264, 14, 1, ch.title)
	if ch.buttons != "" {
		g.text(dst, ch.buttons, sw-110, 10, 16, ch.text)
	}
	g.text(dst, ch.nav, 18, 39, 16, ch.text)
	gfx.FillRoundRect(dst, 100, 37, float32(sw-200), 22, 11, ch.field)
	g.text(dst, address, 118, 39, 13, ch.address)
}

// callOthersGrid lays n people out in r below its heading: the fewest columns whose
// rows stay at least 52 pixels tall (at most three columns), each row at most 76.
func callOthersGrid(n int, r rectF) (cols int, rowH float64) {
	if n <= 0 || r.w <= 0 || r.h <= 0 {
		return 1, 0
	}
	for cols = 1; cols < 3; cols++ {
		rows := (n + cols - 1) / cols
		if r.h/float64(rows) >= 52 {
			break
		}
	}
	rows := (n + cols - 1) / cols
	return cols, min(76, r.h/float64(rows))
}

// drawCallOthers lists the participants not on the stage in the side column
// (call.others: side): a round face, the name, and under it the reaction the call's
// chat last posted for them. A join or a leave shows as a face coming or going.
func (g *Game) drawCallOthers(dst *ebiten.Image, v data.PlanningView, others []ensembleMember, log *ensembleLog, r rectF) {
	if r.w <= 0 || r.h <= 0 {
		return
	}
	ink := color.NRGBA{48, 52, 57, 255}
	dim := color.NRGBA{100, 106, 113, 255}
	line := color.NRGBA{220, 224, 228, 255}
	if g.db.Game.UI.Material != nil {
		ink, dim, line = g.th.text, g.th.dim, withAlpha(g.th.border, 0x60)
		g.window(dst, r.x, r.y, r.w, r.h)
	} else {
		gfx.FillRoundRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), 12, color.NRGBA{250, 251, 252, 255})
	}
	list := rectF{r.x + 10, r.y + 10, r.w - 20, r.h - 20}
	if title := data.Render(g.db.Tr(v.Call.OthersTitle), g.st.Env()); title != "" {
		g.drawPlanningText(dst, title, r.x+18, r.y+16, r.w-36, 18, 1, ink)
		rect(dst, r.x+16, r.y+48, r.w-32, 1, line)
		list = rectF{r.x + 10, r.y + 58, r.w - 20, r.h - 68}
	}
	cols, rowH := callOthersGrid(len(others), list)
	if rowH <= 0 {
		return
	}
	cellW := list.w / float64(cols)
	d := min(56, rowH-12)
	if cols > 1 {
		d = min(40, rowH-12) // a smaller face leaves the name its room in a narrow column
	}
	for i, m := range others {
		x := list.x + float64(i%cols)*cellW
		y := list.y + float64(i/cols)*rowH
		g.planningFace(dst, g.callFace(v, m), x+4+d/2, y+rowH/2, d/2)
		if g.talk.active && g.talk.say.Actor != "" && g.memberActor(m) == g.talk.say.Actor {
			// The speaker off the stage (a shared screen holds it) glows in the column.
			_, _, ring, _, _ := callColors(v.Call)
			gfx.StrokeCircle(dst, float32(x+4+d/2), float32(y+rowH/2), float32(d/2+3), 3, ring, true)
		}
		tx, tw := x+12+d, cellW-16-d
		react := log.reactions[m.id]
		size := 15.0
		if cols > 1 {
			size = 13
		}
		if react == "" || rowH < 44 {
			g.drawPlanningText(dst, m.name, tx, y+rowH/2-gfx.LineHeight(size)/2, tw, size, 1, ink)
			continue
		}
		rows := 1
		if rowH >= 64 {
			rows = 2
		}
		top := y + (rowH-gfx.LineHeight(size)-float64(rows)*gfx.LineHeight(12))/2
		g.drawPlanningText(dst, m.name, tx, top, tw, size, 1, ink)
		g.drawPlanningText(dst, react, tx, top+gfx.LineHeight(size), tw, 12, rows, dim)
	}
}

// callFace is the picture of a participant's round face in the side list: their icon,
// else their portrait (a guest's face picture), cut at the face.
func (g *Game) callFace(v data.PlanningView, m ensembleMember) string {
	if slices.Contains(v.Guests, m.id) {
		if a := g.db.Actors[m.id]; a != nil {
			return g.db.FacePicture(a, a.Face, "")
		}
		return ""
	}
	values := g.st.EntityValues(m.id, false)
	if icon, ok := values[v.Icon].(string); ok && icon != "" {
		return icon
	}
	if portrait, ok := values[v.Portrait].(string); ok {
		return portrait
	}
	return ""
}
