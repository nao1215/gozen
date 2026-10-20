package play

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/world"
)

type field struct {
	quiet   bool
	menu    *list
	entries []world.Entry
	dirty   bool
	age     int
	busy    bool
}

func newField(g *Game) *field {
	f := &field{dirty: true, quiet: true}
	g.enterAt = g.st.Location
	return f
}

func (f *field) loc(g *Game) *data.Location { return g.db.Locations[g.st.Location] }

func (f *field) arrive(g *Game) {
	f.dirty = true
	f.age = 0
	if f.menu != nil {
		f.menu.sel = 0
	}
	loc := f.loc(g)
	var pics []string
	for _, e := range loc.Exits {
		if l := g.db.Locations[e.To]; l != nil {
			pics = append(pics, l.Background.File)
		}
	}
	g.pics.preload(pics...)
}

func (f *field) rebuild(g *Game) {
	f.entries = world.Entries(g.db, g.st)
	var items []listItem
	for _, e := range f.entries {
		it := listItem{label: e.Label}
		switch e.Kind {
		case world.Do, world.Search:
		case world.Talk:
			it.color = g.th.cursor
		case world.Exit:
			it.color = g.th.border
		case world.Menu:
			it.right = "X"
		}
		items = append(items, it)
	}
	sel := 0
	if f.menu != nil {
		sel = min(f.menu.sel, len(items)-1)
	}
	_, sh := g.screen()
	top := 28 + f.headH(g) + 16
	f.menu = newList(items, 36, top, 470, int((sh-top-150)/44))
	f.menu.sel = sel
	f.menu.scroll()
	f.dirty = false
}

func (f *field) update(g *Game) {
	f.age++
	if g.pump() {
		f.busy = true
		return
	}
	if f.busy {
		f.busy = false
		f.dirty = true
	}
	if g.enterAt != "" {
		loc := g.enterAt
		g.enterAt = ""
		if f.enter(g, loc) {
			return
		}
	}
	if f.auto(g) {
		return
	}
	if f.dirty {
		f.rebuild(g)
	}
	if loc := g.db.Locations[g.st.Location]; loc != nil && loc.NoMenu && (g.in.rclick || g.in.pressed[btnMenu] && !g.in.held[btnOK]) {

		g.in.rclick = false
		g.sfx("decide")
		g.push(newSysMenu(g))
		return
	}
	if g.in.pressed[btnMenu] && !g.in.held[btnOK] {
		g.sfx("decide")
		g.push(newMenu(g))
		return
	}
	chose, _ := f.menu.update(g)
	if !chose {
		return
	}
	e := f.entries[f.menu.sel]
	if e.Kind == world.Menu {
		g.push(newMenu(g))
		return
	}
	if e.Kind == world.Exit && e.Exit.To != "" {
		g.sfx("step")
	}
	if e.Kind == world.Talk && e.Person != nil {
		f.standTalker(g, e.Person)
	}
	g.events.Start(e.Commands(g.db, g.st))
	f.dirty = true
}

func (f *field) standTalker(g *Game, p *data.Person) {
	if p.Image == "" || g.pics.get(p.Image).Bounds().Dy() < 600 {
		return
	}
	sw, _ := g.screen()
	x := 0.7
	if cx, ok := f.personX(g, p); ok && cx < sw/2 {
		x = 0.3
	}
	id := "face:" + p.Image
	g.stage = [2]stageActor{{}, {id: id, face: p.Image, entered: g.tick - 10}}
	g.stageOut = stageActor{}
	g.sides = map[string]float64{id: x}
	g.stageGen++
}

func (f *field) personX(g *Game, p *data.Person) (float64, bool) {
	sw, _ := g.screen()
	_ = sw
	people := f.people(g)
	for i, q := range people {
		if q == p {
			return f.place(g, people)[i].head, true
		}
	}
	return 0, false
}

type placed struct {
	x, bottom, h, head float64
	big                bool
}

func (f *field) place(g *Game, people []*data.Person) []placed {
	sw, sh := g.screen()
	n := float64(len(people))
	gap := math.Min(260, sw*0.5/n)
	h := sh*1.18 - headRoom
	out := make([]placed, len(people))
	top := float64(headRoom)
	for i, p := range people {
		x := sw*0.70 + (float64(i)-(n-1)/2)*gap
		img := g.pics.get(p.Image)
		iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		if ih < 600 {
			out[i] = placed{x: x, bottom: sh * 0.78, h: math.Min(ih, 300), head: x}
			continue
		}
		s := h / ih
		r := g.pics.foundHead(p.Image)
		off := (float64(r.Min.X+r.Max.X)/2 - iw/2) * s
		out[i] = placed{x: x - off, h: h, head: x, big: true, bottom: -float64(r.Min.Y) * s}
		if half := float64(r.Dx()) * s / 2; x+half > sw-goldBoxW-36 {
			top = goldBoxY + goldBoxH + 20
			if mh := g.metersHeight(); mh > 0 {
				top += mh + 8
			}
		}
	}
	for i, person := range people {
		if out[i].big {
			out[i].bottom += top + h
			if d := g.db.Game.UI.StageDrop[person.Image]; d != 0 {
				out[i].bottom += sh * d
			}
		}
	}
	return out
}

const (
	goldBoxW   = 240.0
	fieldHeadW = 470.0
	goldBoxY   = 28.0
	goldBoxH   = 84.0
)

func (f *field) people(g *Game) []*data.Person {
	loc := f.loc(g)
	env := g.st.Env()
	var people []*data.Person
	for i := range loc.People {
		if ok, err := loc.People[i].When.Bool(env); err == nil && ok && loc.People[i].Image != "" {
			people = append(people, &loc.People[i])
		}
	}
	return people
}

func (f *field) enter(g *Game, loc string) bool {
	if g.st.Location != loc {
		return false
	}
	cmds := world.Arrive(g.db, g.st, !f.quiet)
	f.quiet = false
	g.events.Start(cmds)
	return len(cmds) > 0
}

func (f *field) auto(g *Game) bool {
	cmds := world.Auto(g.db, g.st)
	g.events.Start(cmds)
	return len(cmds) > 0
}

func (f *field) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	loc := f.loc(g)
	g.drawPicture(dst, loc.Background, 0, 0, sw, sh, 1)
	people := f.people(g)
	var focus *data.Person
	if f.menu != nil && f.menu.sel < len(f.entries) && !g.talk.active {
		focus = f.entries[f.menu.sel].Person
	}
	if len(g.stack) > 1 || g.talk.active || g.events != nil && g.events.Running() {

		people = nil
	}
	places := f.place(g, people)
	for i, p := range people {

		cs := &ebiten.ColorScale{}
		if focus != nil && focus != p {
			cs.Scale(0.7, 0.7, 0.75, 1)
		}
		if g.talk.active {
			cs.Scale(0.6, 0.6, 0.65, 1)
		}
		q := places[i]
		g.drawStanding(dst, p.Image, q.x, q.bottom, q.h, 1, false, cs)
	}
	if g.talk.active || g.cg.File != "" || len(g.stack) > 1 {
		return
	}

	a := float32(math.Min(1, float64(f.age)/20))
	g.window(dst, 36, 28, fieldHeadW, f.headH(g))
	area, name, desc := f.head(g)
	if area != "" {
		g.text(dst, area, 64, 44, sizeS, g.th.border)
	}
	g.text(dst, name, 64, 72, sizeL, g.th.text)
	if desc != "" {
		g.drawLines(dst, desc, 64, 120, 420, sizeS*0.85, withAlpha(g.th.text, uint8(220*a)))
	}

	t := g.db.Game.Terms
	sec := g.st.Frames / 60
	info := fmt.Sprintf("%d %s", g.st.Gold, t.Currency)
	g.window(dst, sw-goldBoxW-36, goldBoxY, goldBoxW, goldBoxH)
	gfx.DrawTextRight(dst, info, sw-60, 40, sizeM, g.th.text)
	gfx.DrawTextRight(dst, fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60), sw-60, 72, sizeS*0.8, g.th.border)
	g.drawMeters(dst, g.shownMeters("", false), sw-goldBoxW-36, goldBoxY+goldBoxH+8, goldBoxW)
	if f.menu != nil {
		f.menu.draw(g, dst, !g.busy())
	}
	g.drawPartyHUD(dst, sh-140)
}

func (f *field) head(g *Game) (area, name, desc string) {
	loc := f.loc(g)
	env := g.st.Env()
	return data.Render(loc.Area, env), data.Render(loc.Name, env), data.Render(loc.Desc, env)
}

func (f *field) headH(g *Game) float64 {
	_, _, desc := f.head(g)
	if desc == "" {
		return 92
	}
	lines := gfx.Wrap(desc, sizeS*0.85, 420)
	return 92 + float64(len(lines))*gfx.LineHeight(sizeS*0.85) + 14
}

func (g *Game) drawPartyHUD(dst *ebiten.Image, y float64) {
	sw, _ := g.screen()
	ms := g.st.Members()
	w := math.Min(300, (sw-72)/float64(max(1, len(ms))))
	x := sw - 36 - w*float64(len(ms))
	for i, a := range ms {
		g.drawActorCard(dst, a, x+float64(i)*w, y, w-12, 120, false, "")
	}
	for i, a := range ms {
		g.drawCardBubbles(dst, a.ID, a.States, i, x+float64(i)*w, y, w-12, 120)
	}
}

func (g *Game) drawActorCard(dst *ebiten.Image, a *rpg.Actor, x, y, w, h float64, active bool, face string) {
	g.window(dst, x, y, w, h)
	if active {
		gfx.StrokeRoundRect(dst, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 14, 4, g.th.accent)
	}
	if face == "" {
		face = a.Face()
	}
	g.drawFace(dst, g.faceOf(a.ID, face), x+12, y+12, h-24, 1, !a.Alive())
	tx := x + h
	tw := w - h - 14
	t := g.db.Game.Terms
	lv := fmt.Sprintf("%s%d", t.Level, a.Level)
	ns := sizeM * 0.9
	if room := tw - gfx.TextWidth(lv, sizeS*0.8) - 8; gfx.TextWidth(a.Name(), ns) > room {
		ns *= room / gfx.TextWidth(a.Name(), ns)
	}
	g.text(dst, a.Name(), tx, y+10+(sizeM*0.9-ns)/2, ns, g.th.text)
	gfx.DrawTextRight(dst, lv, x+w-14, y+14, sizeS*0.8, g.th.border)
	mhp, mmp := a.Stat("mhp"), a.Stat("mmp")
	hr := float64(a.HP) / float64(max(1, mhp))
	g.text(dst, fmt.Sprintf("%s %d/%d", t.HP, a.HP, mhp), tx, y+44, sizeS*0.8, g.th.text)
	bar(dst, tx, y+68, tw, 9, hr, hpColor(hr))
	g.text(dst, fmt.Sprintf("%s %d/%d", t.MP, a.MP, mmp), tx, y+80, sizeS*0.8, g.th.text)
	bar(dst, tx, y+104, tw, 7, float64(a.MP)/float64(max(1, mmp)), mpColor)

	g.drawTagRows(dst, g.stateTagRows(a.States, h-24, 3), x+12, y+h-30, -26)
}

func (g *Game) drawStateBadges(dst *ebiten.Image, states map[string]int, x, y float64) {
	g.drawStateTags(dst, states, x, y, 520)
}

func (g *Game) drawStateTags(dst *ebiten.Image, states map[string]int, x, y, maxW float64) {
	g.drawTagRows(dst, g.stateTagRows(states, maxW, 0), x, y, -26)
}

func (g *Game) stateTagRows(states map[string]int, maxW float64, maxRows int) [][]string {
	const size = sizeS * 0.7
	var rows [][]string
	var row []string
	used := 0.0
	n := 0

	var order []string
	for _, bad := range []bool{true, false} {
		for _, s := range g.db.Ordered("states") {
			if d := g.db.States[s]; d != nil && d.Bad == bad {
				order = append(order, s)
			}
		}
	}
	for _, s := range order {
		if states[s] <= 0 {
			continue
		}
		n++
		w := gfx.TextWidth(g.db.States[s].Name, size) + 14
		if len(row) > 0 && used+w > maxW {
			rows = append(rows, row)
			row, used = nil, 0
		}
		row = append(row, s)
		used += w + 4
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	if maxRows > 0 && len(rows) > maxRows {
		shown := 0
		for _, r := range rows[:maxRows] {
			shown += len(r)
		}
		rows = rows[:maxRows]
		last := rows[maxRows-1]
		rows[maxRows-1] = append(last[:len(last)-1:len(last)-1], fmt.Sprintf("+%d", n-shown+1))
	}
	return rows
}

func (g *Game) drawTagRows(dst *ebiten.Image, rows [][]string, x, y, dy float64) {
	const size = sizeS * 0.7
	for _, row := range rows {
		xx := x
		for _, id := range row {
			name, bg := id, g.th.border
			if st := g.db.States[id]; st != nil {
				name, bg = st.Name, st.Color.Or(g.th.accent)
			}
			w := gfx.TextWidth(name, size) + 14
			gfx.FillRoundRect(dst, float32(xx), float32(y), float32(w), 24, 8, bg)
			gfx.DrawText(dst, name, xx+w/2, y+2, size, inkOn(bg))
			xx += w + 4
		}
		y += dy
	}
}

func inkOn(c color.NRGBA) color.Color {
	if 0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B) < 140 {
		return colorWhite
	}
	return colorBlack
}

func (g *Game) stateTagsWidth(states map[string]int) float64 {
	w := 0.0
	for _, s := range g.db.Ordered("states") {
		if states[s] > 0 {
			w += gfx.TextWidth(g.db.States[s].Name, sizeS*0.7) + 18
		}
	}
	return w
}
