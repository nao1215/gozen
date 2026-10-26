package play

import (
	"fmt"
	"image/color"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The sortie screen (ADR 0030): who marches on a march with a clash shown as units or
// more than one general. On the left the generals who have joined, each with the face,
// the troops and the stats that count, those who marched this turn or are away grayed;
// on the right the slots of the units (the front two, then the back), each with its
// general and the tactic it fights by; at the bottom the troops marching against the
// region's garrison, and the buttons to march and to cancel. Up and down move over the
// generals and the buttons, OK puts a general in the first free slot or takes them out,
// left and right change the tactic of a general put in; a click does the same, and a
// click on a slot and then another swaps them. The first slot leads.

type sortieScene struct {
	r       event.SortieScene
	done    func()
	all     []string // the generals who have joined, in the written order
	picked  []string // in the slots, the leader first
	tactics map[string]string
	cursor  int // over all, then march, then cancel
	top     int // the first general's row shown (the list scrolls)
	swap    int // the slot clicked first for a swap (-1 none)
	closed  bool
}

func newSortie(g *Game, r event.SortieScene, done func()) *sortieScene {
	s := &sortieScene{r: r, done: done, tactics: map[string]string{}, swap: -1}
	for _, id := range g.db.Ordered("actors") {
		if _, ok := g.db.Game.Campaign.Generals[id]; ok && g.st.Actors[id] != nil {
			s.all = append(s.all, id)
		}
	}
	for _, id := range s.all {
		t := g.sortieTactics[id]
		if _, ok := g.db.Game.Campaign.Clash.DeclaredTactics()[t]; !ok {
			t = campaign.TacticOf(g.db, id, nil)
		}
		s.tactics[id] = t
	}
	if len(r.Ready) > 0 {
		s.picked = []string{r.Ready[0]}
		s.cursor = slices.Index(s.all, r.Ready[0])
		s.follow(g)
	}
	return s
}

func (s *sortieScene) overlay() bool { return false }

func (s *sortieScene) ready(id string) bool { return slices.Contains(s.r.Ready, id) }

// toggle puts a general in the first free slot, or takes them out.
func (s *sortieScene) toggle(g *Game, id string) {
	if i := slices.Index(s.picked, id); i >= 0 {
		s.picked = slices.Delete(s.picked, i, i+1)
		g.sfx("cancel")
		return
	}
	if !s.ready(id) || len(s.picked) >= s.r.Cap {
		g.sfx("buzzer")
		return
	}
	s.picked = append(s.picked, id)
	g.sfx("decide")
}

// turn changes a general's tactic by step.
func (s *sortieScene) turn(g *Game, id string, step int) {
	ids := g.db.Game.Campaign.Clash.TacticIDs()
	i := slices.Index(ids, s.tactics[id])
	s.tactics[id] = ids[(i+step+len(ids))%len(ids)]
	g.sfx("cursor")
}

func (s *sortieScene) close(g *Game, march bool) {
	if s.closed {
		return
	}
	s.closed = true
	g.pop()
	if march && len(s.picked) > 0 {
		tactics := map[string]string{}
		for _, id := range s.picked {
			tactics[id] = s.tactics[id]
			g.sortieTactics[id] = s.tactics[id]
		}
		var line []string
		for _, id := range s.picked {
			line = append(line, id+":"+tactics[id])
		}
		g.emit("sortie", traceLine{"order": s.r.Order, "target": s.r.Target, "generals": strings.Join(line, ",")})
		s.r.March(slices.Clone(s.picked), tactics)
	}
	s.done()
}

// The places of the screen (UI units). The generals' rows stand from 0.14 of the screen
// down to 0.86 (above the bottom line), as many as fit; the list scrolls past them.
func (s *sortieScene) rowBox(g *Game, i int) (x, y, w, h float64) {
	sw, sh := g.screen()
	h = sh * 0.105
	return sw * 0.04, sh*0.14 + float64(i-s.top)*(h+8), sw * 0.42, h
}

// rows is how many generals' rows fit.
func (s *sortieScene) rows(g *Game) int {
	_, sh := g.screen()
	h := sh * 0.105
	return max(1, int((sh*0.86-sh*0.14+8)/(h+8)))
}

// shownRows are the generals' rows drawn now: first to last, last not included.
func (s *sortieScene) shownRows(g *Game) (first, last int) {
	return s.top, min(len(s.all), s.top+s.rows(g))
}

// follow scrolls the list so the cursor's row shows (when it is on a general).
func (s *sortieScene) follow(g *Game) {
	if s.cursor >= len(s.all) {
		return
	}
	n := s.rows(g)
	if s.cursor < s.top {
		s.top = s.cursor
	}
	if s.cursor >= s.top+n {
		s.top = s.cursor - n + 1
	}
	s.top = max(0, min(s.top, len(s.all)-n))
}

func (s *sortieScene) slotBox(g *Game, k int) (x, y, w, h float64) {
	sw, sh := g.screen()
	front := min(campaign.Front, s.r.Cap)
	w, h = sw*0.22, sh*0.27
	col, row := k%front, k/front
	return sw*0.5 + float64(col)*(w+sw*0.02), sh*0.17 + float64(row)*(h+sh*0.08), w, h
}

func (s *sortieScene) buttonBox(g *Game, i int) (x, y, w, h float64) {
	sw, sh := g.screen()
	w, h = sw*0.13, sh*0.075
	return sw*0.66 + float64(i)*(w+sw*0.02), sh * 0.875, w, h
}

func inBox(px, py, x, y, w, h float64) bool { return px >= x && px < x+w && py >= y && py < y+h }

func (s *sortieScene) update(g *Game) {
	in := &g.in
	n := len(s.all) + 2
	switch {
	case in.cancel():
		s.close(g, false)
		return
	case in.up():
		s.cursor = (s.cursor - 1 + n) % n
		s.follow(g)
		g.sfx("cursor")
	case in.down():
		s.cursor = (s.cursor + 1) % n
		s.follow(g)
		g.sfx("cursor")
	case (in.left() || in.right()) && s.cursor < len(s.all) && slices.Contains(s.picked, s.all[s.cursor]):
		step := 1
		if in.left() {
			step = -1
		}
		s.turn(g, s.all[s.cursor], step)
	case in.ok():
		switch {
		case s.cursor < len(s.all):
			s.toggle(g, s.all[s.cursor])
		case s.cursor == len(s.all):
			if len(s.picked) == 0 {
				g.sfx("buzzer")
				return
			}
			g.sfx("decide")
			s.close(g, true)
		default:
			s.close(g, false)
		}
		return
	}
	if !in.click {
		return
	}
	mx, my := float64(in.mouse.X), float64(in.mouse.Y)
	first, last := s.shownRows(g)
	for i := first; i < last; i++ {
		id := s.all[i]
		if x, y, w, h := s.rowBox(g, i); inBox(mx, my, x, y, w, h) {
			s.cursor = i
			s.toggle(g, id)
			return
		}
	}
	for k := range s.r.Cap {
		x, y, w, h := s.slotBox(g, k)
		if !inBox(mx, my, x, y, w, h) || k >= len(s.picked) {
			continue
		}
		id := s.picked[k]
		switch {
		case my > y+h-58 && mx < x+w/3: // the tactic's arrows
			s.turn(g, id, -1)
		case my > y+h-58 && mx > x+w*2/3:
			s.turn(g, id, 1)
		case s.swap >= 0 && s.swap != k && s.swap < len(s.picked):
			s.picked[s.swap], s.picked[k] = s.picked[k], s.picked[s.swap]
			s.swap = -1
			g.sfx("decide")
		default:
			s.swap = k
			g.sfx("cursor")
		}
		return
	}
	for i := range 2 {
		if x, y, w, h := s.buttonBox(g, i); inBox(mx, my, x, y, w, h) {
			if i == 0 && len(s.picked) == 0 {
				g.sfx("buzzer")
				return
			}
			s.close(g, i == 0)
			return
		}
	}
}

// drawGeneralFace draws a general's round face.
func (g *Game) drawGeneralFace(dst *ebiten.Image, id string, cx, cy, size float64, alpha uint8) {
	a := g.db.Actors[id]
	if a == nil {
		return
	}
	gfx.FillCircle(dst, float32(cx), float32(cy), float32(size/2+3), withAlpha(g.th.accent, alpha), true)
	if img := g.dollFace(newDollSpec(g.db.FacePicture(a, a.Face, ""), a.Doll, 30)); img != nil {
		b := img.Bounds()
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		sc := size * g.ui / float64(b.Dx())
		op.GeoM.Scale(sc, sc)
		op.GeoM.Translate((cx-size/2)*g.ui, (cy-size/2)*g.ui)
		op.ColorScale.ScaleAlpha(float32(alpha) / 255)
		dst.DrawImage(img, op)
	}
}

// generalStats is a general's line of numbers: troops, might and attack.
func (s *sortieScene) generalStats(g *Game, id string) string {
	cs := g.st.Campaign()
	parts := []string{fmt.Sprintf(lang.T("兵 %d"), int(data.GeneralTroops(g.db, cs, id)))}
	if m := g.db.Game.Campaign.Generals[id].Might; m != 0 {
		parts = append(parts, fmt.Sprintf(lang.T("武 %g"), m))
	}
	if a := g.st.Actors[id]; a != nil {
		parts = append(parts, fmt.Sprintf(lang.T("攻 %d"), a.Stat("atk")))
	}
	return strings.Join(parts, "　")
}

// title is the sortie screen's title: campaign.sortie.title (the engine's words when
// empty) and the region marched on.
func (s *sortieScene) title(g *Game) string {
	region := data.RegionName(g.db, s.r.Target)
	if look := g.db.Game.Campaign.Sortie; look != nil && look.Title != "" {
		return data.Render(g.db.Tr(look.Title), g.viewEnv()) + "　" + region
	}
	return fmt.Sprintf(lang.T("出陣　%s"), region)
}

func (s *sortieScene) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	cs := g.st.Campaign()
	if look := g.db.Game.Campaign.Sortie; look != nil {
		shade := uint8(0xd8)
		if look.Background.File != "" {
			g.drawPicture(dst, look.Background, 0, 0, sw, sh, 1)
			shade = 0x90
		}
		gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0x10, 0x16, 0x1c, shade}, false)
		title := s.title(g)
		tw := gfx.TextWidth(title, sizeM) + 80
		g.window(dst, (sw-tw)/2, 18, tw, 64)
		gfx.DrawText(dst, title, sw/2, 18+16, sizeM, g.th.text)
	} else {
		gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0x10, 0x16, 0x1c, 0xd8}, false)
		gfx.DrawTextOutline(dst, s.title(g), sw/2, sh*0.035, sizeL*1.2, g.th.text)
	}
	// the generals, the rows that fit, with arrows when there are more
	first, last := s.shownRows(g)
	if first > 0 {
		x, y, w, _ := s.rowBox(g, first)
		gfx.DrawText(dst, "▲", x+w/2, y-sizeS, sizeS*0.8, g.th.cursor)
	}
	if last < len(s.all) {
		x, y, w, h := s.rowBox(g, last-1)
		gfx.DrawText(dst, "▼", x+w/2, y+h-4, sizeS*0.8, g.th.cursor)
	}
	for i := first; i < last; i++ {
		id := s.all[i]
		x, y, w, h := s.rowBox(g, i)
		g.window(dst, x, y, w, h)
		alpha := uint8(255)
		status := ""
		switch {
		case data.Out(cs, id) > 0:
			alpha, status = 110, fmt.Sprintf(lang.T("離脱 あと%d"), data.Out(cs, id))
		case !s.ready(id):
			alpha, status = 110, lang.T("行動済み")
		}
		if k := slices.Index(s.picked, id); k >= 0 {
			gfx.FillRoundRect(dst, float32(x+4), float32(y+4), float32(w-8), float32(h-8), 10, withAlpha(g.th.accent, 0x50))
			gfx.DrawTextRight(dst, s.slotName(k), x+w-20, y+12, sizeS, g.th.accent)
		}
		face := h - 20
		g.drawGeneralFace(dst, id, x+14+face/2, y+h/2, face, alpha)
		g.text(dst, campaign.GeneralLabelName(g.db, id), x+30+face, y+10, sizeM, withAlpha(g.th.text, alpha))
		line := s.generalStats(g, id)
		if status != "" {
			line = status
		}
		g.text(dst, line, x+30+face, y+h-12-sizeS*1.3, sizeS*0.9, withAlpha(g.th.text, alpha))
		if s.cursor == i {
			gfx.StrokeRoundRect(dst, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 10, 3, g.th.cursor)
		}
	}
	// the slots
	front := min(campaign.Front, s.r.Cap)
	for k := range s.r.Cap {
		x, y, w, h := s.slotBox(g, k)
		if k%front == 0 {
			gfx.DrawTextLeft(dst, s.slotName(k), x, y-sizeS*1.5, sizeS, g.th.text)
		}
		g.window(dst, x, y, w, h)
		if k == s.swap {
			gfx.StrokeRoundRect(dst, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 10, 3, g.th.cursor)
		}
		if k >= len(s.picked) {
			gfx.DrawText(dst, lang.T("（空き）"), x+w/2, y+h/2-sizeM*0.6, sizeM, g.th.dim)
			continue
		}
		id := s.picked[k]
		face := h * 0.36
		g.drawGeneralFace(dst, id, x+w/2, y+14+face/2, face, 255)
		name := campaign.GeneralLabelName(g.db, id)
		if k == 0 {
			name = fmt.Sprintf(lang.T("%s（大将）"), name)
		}
		gfx.DrawText(dst, name, x+w/2, y+20+face, sizeS, g.th.text)
		gfx.DrawText(dst, fmt.Sprintf(lang.T("兵 %d"), int(data.GeneralTroops(g.db, cs, id))), x+w/2, y+20+face+sizeS*1.4, sizeS*0.85, g.th.text)
		t := g.db.Game.Campaign.Clash.DeclaredTactics()[s.tactics[id]]
		label := s.tactics[id]
		if t != nil {
			label = lang.T(t.Label)
		}
		gfx.FillRoundRect(dst, float32(x+10), float32(y+h-54), float32(w-20), 44, 8, withAlpha(g.th.accent, 0x60))
		gfx.DrawText(dst, "◀　"+label+"　▶", x+w/2, y+h-50, sizeS*0.9, g.th.text)
	}
	// the troops and the buttons
	total := 0.0
	for _, id := range s.picked {
		total += data.GeneralTroops(g.db, cs, id)
	}
	_, by, _, bh := s.buttonBox(g, 0)
	g.window(dst, sw*0.04, by, sw*0.58, bh)
	g.text(dst, fmt.Sprintf(lang.T("出陣 %d　対　守り %d"), int(total), int(data.Garrison(g.db, cs, s.r.Target))), sw*0.04+24, by+(bh-sizeM*1.3)/2, sizeM, g.th.text)
	for i, label := range []string{lang.T("出陣する"), lang.T("やめる")} {
		x, y, w, h := s.buttonBox(g, i)
		g.window(dst, x, y, w, h)
		c := g.th.text
		if i == 0 && len(s.picked) == 0 {
			c = g.th.dim
		}
		gfx.DrawText(dst, label, x+w/2, y+(h-sizeM*1.3)/2, sizeM, c)
		if s.cursor == len(s.all)+i {
			gfx.StrokeRoundRect(dst, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 10, 3, g.th.cursor)
		}
	}
	g.text(dst, lang.T("決定：入れる・外す　左右：戦法　キャンセル：やめる"), 36, sh-36, sizeS*0.8, withAlpha(g.th.text, 200))
}

// slotName is the row of slot k: the front or the back.
func (s *sortieScene) slotName(k int) string {
	if k < min(campaign.Front, s.r.Cap) {
		return lang.T("前列")
	}
	return lang.T("後列")
}
