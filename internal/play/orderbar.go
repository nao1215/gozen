package play

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
)

// The row of icons (campaign.order_view: icons): the orders a region chosen on the
// campaign's map can be given, as square buttons in a row over the bottom of the map, a
// little apart between groups. Left and right (or the pointer) choose, OK gives the order
// (what it asks next, such as its general, is asked as before), Cancel closes the row.
// The end of the turn is not in it: campaign.end_turn_button stands it on the map.

// barItem is one button of the row: an order given the region.
type barItem struct {
	label, note, icon, group string
	do                       []data.Command
}

// orderBar is the row of the orders of one region, over the map.
type orderBar struct {
	scene  *atlasScene
	region string
	prompt string
	items  []barItem
	sel    int
}

func (b *orderBar) overlay() bool { return true }

// regionOrderItems are the orders the region id can be given now, in the written order.
func regionOrderItems(g *Game, id string) []barItem {
	c := g.db.Game.Campaign
	var out []barItem
	for _, oid := range c.OrderIDs() {
		o := c.Orders[oid]
		if o == nil || o.Target == "" || campaign.Open(g.db, g.st, o) != "" {
			continue
		}
		for _, t := range campaign.Targets(g.db, g.st, o) {
			if t != id {
				continue
			}
			it := barItem{label: o.Label, icon: o.Icon, group: o.Group, do: []data.Command{{Order: &data.OrderCmd{ID: oid, Target: id}}}}
			if it.label == "" {
				it.label = oid
			}
			if c.Count != "orders" && o.CostOf() != 1 {
				it.note = fmt.Sprintf(lang.T("行動 %d"), o.CostOf())
			}
			out = append(out, it)
		}
	}
	return out
}

// barGap is the room between two buttons, wider between groups.
func (b *orderBar) gap(i int, size float64) float64 {
	if i > 0 && b.items[i].group != b.items[i-1].group {
		return size * 0.35
	}
	return size * 0.12
}

// layout places the row: the window and each button (UI units), centred over the bottom of
// the map's safe part.
func (b *orderBar) layout(g *Game) (win plateBox, buttons []plateBox) {
	sw, sh := g.screen()
	size := sh * 96 / 1080
	n := float64(len(b.items))
	total := func(s float64) float64 {
		w := s * n
		for i := range b.items {
			if i > 0 {
				w += b.gap(i, s)
			}
		}
		return w
	}
	if w := total(size); w > sw-120 {
		size *= (sw - 120) / w
	}
	bottom := sh - 24
	if a := b.scene.location(g).Map.SafeArea; len(a) == 4 {
		bottom = math.Min(bottom, sh*(1-a[2])-8)
	}
	labelH := gfx.LineHeight(sizeS) + 12
	w := math.Max(total(size)+48, 520)
	h := size + labelH + 30
	win = plateBox{x: (sw - w) / 2, y: bottom - h, w: w, h: h}
	for _, c := range b.scene.cameraButtons(g) { // over the camera's buttons, not on them
		if win.overlap(c) > 0 {
			win.y = math.Min(win.y, c.y-10-h)
		}
	}
	x := (sw - total(size)) / 2
	for i := range b.items {
		if i > 0 {
			x += b.gap(i, size)
		}
		buttons = append(buttons, plateBox{x: x, y: win.y + labelH + 12, w: size, h: size})
		x += size
	}
	return win, buttons
}

func (b *orderBar) update(g *Game) {
	in := &g.in
	n := len(b.items)
	if in.cancel() {
		g.sfx("cancel")
		g.pop()
		return
	}
	old := b.sel
	if in.left() {
		b.sel = (b.sel - 1 + n) % n
	}
	if in.right() {
		b.sel = (b.sel + 1) % n
	}
	win, buttons := b.layout(g)
	hit := -1
	x, y := float64(in.mouse.X), float64(in.mouse.Y)
	for i, r := range buttons {
		if inPlate(r, x, y) {
			hit = i
		}
	}
	if hit >= 0 && (in.moved || in.click) {
		b.sel = hit
	}
	if b.sel != old {
		g.sfx("cursor")
	}
	if in.click && hit < 0 && !inPlate(win, x, y) { // a click off the row closes it
		g.sfx("cancel")
		g.pop()
		return
	}
	if in.ok() || in.click && hit >= 0 {
		g.sfx("decide")
		g.pop()
		g.events.Start(b.items[b.sel].do)
		b.scene.dirty, b.scene.asking = true, true
	}
}

func (b *orderBar) draw(g *Game, dst *ebiten.Image) {
	win, buttons := b.layout(g)
	g.window(dst, win.x, win.y, win.w, win.h)
	it := b.items[b.sel]
	g.text(dst, gfx.Ellipsis(b.prompt, sizeS*0.85, win.w/2-40), win.x+24, win.y+12, sizeS*0.85, g.th.dim)
	label := it.label
	if it.note != "" {
		label += "　" + it.note
	}
	gfx.DrawTextRight(dst, gfx.Ellipsis(label, sizeS, win.w/2), win.x+win.w-24, win.y+10, sizeS, g.th.accent)
	for i, r := range buttons {
		item := b.items[i]
		gfx.FillRoundRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), 10, withAlpha(colorShadowNRGBA, 0xc0))
		if img := g.pics.get(item.icon); item.icon != "" && img != nil && !img.Bounds().Empty() {
			gfx.DrawImageFit(dst, img, r.x+4, r.y+4, r.w-8, r.h-8, 1)
		} else { // no picture: the label in the square
			lines, size := gfx.FitLines(item.label, sizeS*0.8, r.w-10, 3)
			ty := r.y + (r.h-float64(len(lines))*gfx.LineHeight(size))/2
			for k, ln := range lines {
				gfx.DrawText(dst, ln, r.x+r.w/2, ty+float64(k)*gfx.LineHeight(size), size, g.th.text)
			}
		}
		edge, width := withAlpha(g.th.border, 0xc0), float32(2)
		if i == b.sel {
			pulse := uint8(0xc0 + 0x3f*math.Sin(float64(g.tick)/8))
			edge, width = withAlpha(g.th.cursor, pulse), 4
		}
		gfx.StrokeRoundRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), 10, width, edge)
	}
}

// barOpen reports whether the row of icons is open over this map.
func (s *atlasScene) barOpen(g *Game) bool {
	b, ok := g.stack[len(g.stack)-1].(*orderBar)
	return ok && b.scene == s
}

// endTurnAction is the campaign's end of the turn among the actions of its place.
func endTurnAction(g *Game) *data.Action {
	l := g.db.Locations[g.db.Game.Campaign.Location]
	if l == nil {
		return nil
	}
	for i := range l.Actions {
		if l.Actions[i].ID == "end_turn" {
			return &l.Actions[i]
		}
	}
	return nil
}

// endTurnRect is the end of the turn's button (campaign.end_turn_button): at the bottom
// right, over the generals' window.
func (s *atlasScene) endTurnRect(g *Game) (plateBox, bool) {
	c := g.db.Game.Campaign
	if c == nil || c.EndTurnButton == nil || !s.campaignMap(g) || endTurnAction(g) == nil {
		return plateBox{}, false
	}
	sw, sh := g.screen()
	label := data.Render(endTurnAction(g).Label, g.st.Env())
	w := gfx.TextWidth(label, sizeM) + 70
	if c.EndTurnButton.Icon != "" {
		w += 58 // the picture, 48 wide, and a gap
	}
	bottom := sh - 42
	if gen, ok := s.generalsRect(g); ok {
		bottom = gen.y - 10
	}
	return plateBox{x: sw - 36 - w, y: bottom - 64, w: w, h: 64}, true
}

// drawEndTurn draws the end of the turn's button, lit when no actions are left.
func (s *atlasScene) drawEndTurn(g *Game, dst *ebiten.Image) {
	r, ok := s.endTurnRect(g)
	if !ok {
		return
	}
	g.window(dst, r.x, r.y, r.w, r.h)
	x := r.x + 22
	if pic := g.db.Game.Campaign.EndTurnButton.Icon; pic != "" {
		if img := g.pics.get(pic); img != nil && !img.Bounds().Empty() {
			gfx.DrawImageFit(dst, img, x, r.y+8, 48, 48, 1)
		}
		x += 58
	}
	g.text(dst, data.Render(endTurnAction(g).Label, g.st.Env()), x, r.y+14, sizeM, g.th.text)
	gfx.DrawTextRight(dst, "E", r.x+r.w-14, r.y+r.h-24, sizeS*0.7, g.th.dim)
	cs := g.st.Campaign()
	if g.db.Game.Campaign.Count != "orders" && data.ActionsLeft(g.db, cs) <= 0 {
		pulse := uint8(0x80 + 0x7f*math.Sin(float64(g.tick)/10))
		gfx.StrokeRoundRect(dst, float32(r.x+2), float32(r.y+2), float32(r.w-4), float32(r.h-4), 14, 4, withAlpha(g.th.cursor, pulse))
	}
}

// updateEndTurn ends the turn when its button is clicked or its key pressed.
func (s *atlasScene) updateEndTurn(g *Game) bool {
	r, ok := s.endTurnRect(g)
	if !ok {
		return false
	}
	in := &g.in
	if !in.pressed[btnEndTurn] && !(in.click && inPlate(r, float64(in.mouse.X), float64(in.mouse.Y))) {
		return false
	}
	g.sfx("decide")
	g.events.Start(world.Entry{Kind: world.Do, Action: endTurnAction(g)}.Commands(g.db, g.st))
	s.dirty = true
	return true
}
