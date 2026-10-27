package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

type shop struct {
	id    string
	def   *data.Shop
	top   *list
	goods *list
	ids   []string
	mode  string
	count int
	pick  string
	msg   string
	msgT  int
	done  func()
}

func newShop(g *Game, id string, done func()) *shop {
	s := &shop{id: id, def: g.db.Shops[id], done: done}
	items := []listItem{{label: lang.T("かう")}, {label: lang.T("うる"), disabled: s.def.Sell != nil && !*s.def.Sell}, {label: lang.T("でる")}}
	s.top = newList(items, 36, 36, 250, 3)
	s.msg = s.def.Greeting
	return s
}

func (s *shop) overlay() bool { return true }

func (s *shop) open(g *Game) {
	var items []listItem
	s.ids = nil
	cur := g.db.Game.Terms.Currency
	if s.mode == "buy" {
		for _, id := range s.def.Goods {
			it := g.db.Items[id]
			row := listItem{label: it.Name, right: fmt.Sprintf("%d%s", it.Price, cur), disabled: it.Price > g.st.Gold}
			row.note, row.noteColor = gearNote(g, gearCompare(g, id))
			items = append(items, row)
			s.ids = append(s.ids, id)
		}
	} else {
		for _, id := range g.st.Bag() {
			it := g.db.Items[id]
			if it.Kind == "key" || it.Price <= 0 {
				continue
			}
			items = append(items, listItem{label: fmt.Sprintf("%s ×%d", it.Name, g.st.Items[id]), right: fmt.Sprintf("%d%s", it.Price/2, cur)})
			s.ids = append(s.ids, id)
		}
	}
	sel := 0
	if s.goods != nil {
		sel = min(s.goods.sel, max(0, len(items)-1))
	}
	_, sh := g.screen()
	s.goods = newList(items, 300, 36, s.goodsW(g), int((sh-260)/44))
	s.goods.sel = sel
	s.goods.scroll()
}

func (s *shop) update(g *Game) {
	if s.msgT > 0 {
		s.msgT--
	}
	if s.pick != "" {
		s.updateCount(g)
		return
	}
	if s.mode == "" {
		chose, cancel := s.top.update(g)
		if cancel || (chose && s.top.sel == 2) {
			g.pop()
			s.done()
			return
		}
		if chose {
			s.mode = []string{"buy", "sell"}[s.top.sel]
			s.goods = nil
			s.open(g)
		}
		return
	}
	chose, cancel := s.goods.update(g)
	if cancel {
		s.mode = ""
		return
	}
	if chose && len(s.ids) > 0 {
		s.pick, s.count = s.ids[s.goods.sel], 1
	}
}

func (s *shop) price(g *Game) int {
	p := g.db.Items[s.pick].Price
	if s.mode == "sell" {
		p /= 2
	}
	return p
}

func (s *shop) updateCount(g *Game) {
	var most int
	if s.mode == "buy" {
		most = min(99, g.st.Gold/max(1, s.price(g)))
	} else {
		most = g.st.Items[s.pick]
	}
	old := s.count

	minus, plus, box := s.countRects(g)
	click := g.in.click && g.in.mouse.In(box)
	if g.in.up() || g.in.right() || (click && g.in.mouse.In(plus)) {
		s.count = min(most, s.count+1)
	}
	if g.in.down() || g.in.left() || (click && g.in.mouse.In(minus)) {
		s.count = max(1, s.count-1)
	}
	if click && (g.in.mouse.In(plus) || g.in.mouse.In(minus)) {
		click = false
	}
	if s.count != old {
		g.sfx("cursor")
	}
	if g.in.cancel() {
		s.pick = ""
		g.sfx("cancel")
		return
	}
	if g.in.ok() || click {
		name := g.db.Items[s.pick].Name
		if s.mode == "buy" {
			g.st.Buy(s.id, s.pick, s.count)
			s.msg = fmt.Sprintf(lang.T("%sを %dこ かった！"), name, s.count)
		} else {
			g.st.Sell(s.id, s.pick, s.count)
			s.msg = fmt.Sprintf(lang.T("%sを %dこ うった！"), name, s.count)
		}
		g.sfx("item")
		s.msgT = 90
		s.pick = ""
		s.open(g)
	}
}

func (s *shop) countLine(g *Game) string {
	return fmt.Sprintf(lang.T("◀  %d こ  ▶     %d %s"), s.count, s.count*s.price(g), g.db.Game.Terms.Currency)
}

func (s *shop) countRects(g *Game) (minus, plus, box image.Rectangle) {
	cx, cw, sh := s.countBox(g)
	line := s.countLine(g)
	x0 := cx - gfx.TextWidth(line, sizeL)/2
	y0, y1 := int(sh/2-24), int(sh/2+30)
	at := func(prefix string) image.Rectangle {
		before, _, ok := strings.Cut(line, prefix)
		if !ok {
			return image.Rectangle{}
		}
		x := x0 + gfx.TextWidth(before, sizeL)
		return image.Rect(int(x-18), y0, int(x+gfx.TextWidth(prefix, sizeL)+18), y1)
	}
	return at("◀"), at("▶"), image.Rect(int(cx-cw/2), int(sh/2-80), int(cx+cw/2), int(sh/2+80))
}

func (s *shop) countBox(g *Game) (cx, w, sh float64) {
	_, sh = g.screen()
	gw := s.goodsW(g)
	return 300 + gw/2, math.Min(520, gw), sh
}

func (s *shop) keeperAt(g *Game) placed {
	p := &data.Person{Image: s.def.Keeper}
	if loc := g.db.Locations[g.st.Location]; loc != nil {
		f := &field{}
		people := f.people(g)
		for i, q := range people {
			if q.Image == s.def.Keeper {
				return f.place(g, people)[i]
			}
		}
	}
	return (&field{}).place(g, []*data.Person{p})[0]
}

func (s *shop) right(g *Game) float64 {
	sw, _ := g.screen()
	if s.def.Keeper == "" {
		return sw - 36
	}
	img := g.pics.get(s.def.Keeper)
	q := s.keeperAt(g)
	w := float64(img.Bounds().Dx()) * q.h / float64(max(1, img.Bounds().Dy()))
	return q.x - w/2 - 12
}

func (s *shop) goodsW(g *Game) float64 { return math.Max(420, math.Min(600, s.right(g)-300)) }

func (s *shop) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, withAlpha(colorBlack, 0x70))
	if s.def.Keeper != "" {
		q := s.keeperAt(g)
		g.drawStanding(dst, s.def.Keeper, q.x, q.bottom, q.h, 1, false, nil)
	}
	s.top.draw(g, dst, s.mode == "")
	g.window(dst, 36, 200, 250, 110)
	g.text(dst, s.def.Name, 56, 214, sizeS*0.85, g.th.border)
	cur := g.db.Game.Terms.Currency
	gfx.DrawTextRight(dst, fmt.Sprintf("%d %s", g.st.Gold, cur), 266, 256, sizeM, g.th.text)
	if s.goods != nil && s.mode != "" {
		s.goods.draw(g, dst, s.pick == "")
		if len(s.ids) > 0 {
			id := s.ids[s.goods.sel]
			it := g.db.Items[id]
			desc := it.Desc
			if it.Equipment() {
				desc = equipDesc(g, it) + " " + desc

				if s.mode == "buy" {
					s.drawCompare(g, dst, gearCompare(g, id))
				}
			}
			gw := s.goodsW(g)
			g.window(dst, 300, sh-206, gw, 90)
			g.drawLinesFit(dst, fmt.Sprintf(lang.T("%s  もっている数 %d"), desc, g.st.ItemCount(id, true)), 324, sh-194, gw-40, sizeS*0.9, 3, g.th.text)
		}
	}
	if s.pick != "" {
		cx, cw, _ := s.countBox(g)
		g.window(dst, cx-cw/2, sh/2-80, cw, 160)
		gfx.DrawText(dst, g.db.Items[s.pick].Name, cx, sh/2-60, sizeM, g.th.text)
		gfx.DrawText(dst, s.countLine(g), cx, sh/2-14, sizeL, g.th.cursor)
		gfx.DrawText(dst, lang.T("Z: けってい  X: やめる"), cx, sh/2+36, sizeS*0.85, g.th.border)
	}
	if s.msg != "" {

		w := math.Min(sw-72, s.right(g)-36)
		g.window(dst, 36, sh-110, w, 84)
		g.drawLinesFit(dst, s.msg, 60, sh-98, w-48, sizeM, 2, g.th.text)
	}
}

type gearCmp struct {
	who   string
	now   string
	worn  bool
	diffs []statDiff
	sum   int
}

type statDiff struct {
	stat string
	d    int
}

func gearCompare(g *Game, id string) []gearCmp {
	it := g.db.Items[id]
	if it == nil || !it.Equipment() {
		return nil
	}
	var out []gearCmp
	for _, a := range g.st.Members() {
		if !a.CanEquip(id) {
			continue
		}

		slot, empty, first := "", "", ""
		for _, sl := range g.db.Game.Slots {
			if !it.Fits(sl.ID) {
				continue
			}
			if first == "" {
				first = sl.ID
			}
			if a.Equip[sl.ID] == id && slot == "" {
				slot = sl.ID
			}
			if a.Equip[sl.ID] == "" && empty == "" {
				empty = sl.ID
			}
		}
		if slot == "" {
			slot = empty
		}
		if slot == "" {
			slot = first
		}
		if slot == "" {
			continue
		}
		c := gearCmp{who: a.Name()}
		cur := a.Equip[slot]
		if cur == id {
			c.worn = true
		} else {
			if ci := g.db.Items[cur]; ci != nil {
				c.now = ci.Name
			}
			for _, st := range data.StatNames {
				if d := a.StatWith(st, slot, id) - a.Stat(st); d != 0 {
					c.diffs = append(c.diffs, statDiff{st, d})
					c.sum += d
				}
			}
		}
		out = append(out, c)
	}
	return out
}

var (
	upColor   = color.NRGBA{0x7c, 0xe5, 0x8e, 0xff}
	downColor = color.NRGBA{0xff, 0x6b, 0x6b, 0xff}
)

func gearNote(g *Game, cs []gearCmp) (string, color.Color) {
	if len(cs) == 0 {
		return "", nil
	}
	up, worn, down := false, false, true
	for _, c := range cs {
		if c.worn {
			worn = true
			down = false
			continue
		}
		if c.sum > 0 {
			up = true
		}
		if c.sum >= 0 {
			down = false
		}
	}
	switch {
	case up:
		return "↑", upColor
	case worn:
		return lang.T("装備中"), g.th.border
	case down:
		return "↓", downColor
	}
	return "＝", g.th.dim
}

func (s *shop) drawCompare(g *Game, dst *ebiten.Image, cs []gearCmp) {
	if len(cs) == 0 {
		return
	}
	_, sh := g.screen()
	const x, y, w = 36.0, 322.0, 250.0
	bottom := sh - 122
	type seg struct {
		s string
		c color.Color
	}
	type line struct {
		segs  []seg
		right seg
		head  bool
	}
	size := sizeS * 0.78
	gap := gfx.TextWidth(" ", size)
	var lines []line
	for _, c := range cs {
		head := line{segs: []seg{{c.who, g.th.text}}, head: true}
		switch {
		case c.worn:
			head.right = seg{lang.T("装備中"), g.th.cursor}
		case c.now != "":
			head.right = seg{lang.T("いま ") + c.now, g.th.border}
		}
		lines = append(lines, head)
		if c.worn {
			continue
		}
		if len(c.diffs) == 0 {
			lines = append(lines, line{segs: []seg{{lang.T("変わらない"), g.th.dim}}})
			continue
		}
		cur := line{}
		used := 0.0
		for _, d := range c.diffs {
			t, col := fmt.Sprintf("%s%+d↑", lang.T(data.StatLabels[d.stat]), d.d), color.Color(upColor)
			if d.d < 0 {
				t, col = fmt.Sprintf("%s%+d↓", lang.T(data.StatLabels[d.stat]), d.d), downColor
			}
			tw := gfx.TextWidth(t, size)
			if used > 0 && used+gap+tw > w-36 {
				lines = append(lines, cur)
				cur, used = line{}, 0
			}
			if used > 0 {
				used += gap
			}
			cur.segs = append(cur.segs, seg{t, col})
			used += tw
		}
		lines = append(lines, cur)
	}
	lh := size * 1.45
	h := 24 + lh*float64(len(lines))
	if h > bottom-y {
		lh = (bottom - y - 24) / float64(len(lines))
		size = math.Min(size, lh/1.3)
		h = bottom - y
	}
	g.window(dst, x, y, w, h)
	yy := y + 12
	for _, l := range lines {
		xx := x + 18
		if !l.head {
			xx += 14
		}
		for _, sg := range l.segs {
			g.text(dst, sg.s, xx, yy, size, sg.c)
			xx += gfx.TextWidth(sg.s, size) + gap
		}
		if l.right.s != "" {
			rs := size * 0.9
			txt := gfx.Ellipsis(l.right.s, rs, w-36-(xx-x-18))
			gfx.DrawTextRight(dst, txt, x+w-18, yy+(size-rs)*0.5, rs, l.right.c)
		}
		yy += lh
	}
}

type saveMenu struct {
	saving bool
	inPlay bool
	slots  *list
	done   func()
	msg    string
	msgT   int
}

func newSaveMenu(g *Game, saving bool, done func()) *saveMenu {
	s := &saveMenu{saving: saving, done: done}
	s.refresh(g)
	return s
}

func (s *saveMenu) overlay() bool { return true }

func (s *saveMenu) refresh(g *Game) {
	var items []listItem
	n := saveSlots
	if !s.saving {
		n++
	}
	for i := range n {
		label, ok := g.slotLabel(i)
		items = append(items, listItem{label: label, disabled: !s.saving && !ok})
	}
	sw, sh := g.screen()
	sel := 0
	if s.slots != nil {
		sel = s.slots.sel
	} else if !s.saving {
		sel, _ = g.newestSave()
	}
	s.slots = newList(items, sw/2-400, sh/2-110, 800, n)
	s.slots.rowH = 60
	s.slots.sel = sel
}

func (s *saveMenu) update(g *Game) {
	if s.msgT > 0 {
		s.msgT--
		if s.msgT == 0 || g.in.advance() {
			s.msgT = 0
			g.pop()
			s.done()
		}
		return
	}
	chose, cancel := s.slots.update(g)
	if cancel {
		g.pop()
		s.done()
		return
	}
	if !chose {
		return
	}
	i := s.slots.sel
	if s.saving {
		if g.storeHas(g.slotPath(i)) {
			g.push(newConfirm(fmt.Sprintf(lang.T("スロット%dに 上書きしますか？"), i+1), func() { s.write(g, i) }))
			return
		}
		s.write(g, i)
		return
	}
	st, err := g.load(i)
	if err != nil {
		s.msg = lang.T("ロードできなかった: ") + err.Error()
		s.msgT = 90
		return
	}
	if s.inPlay {
		g.push(newConfirm(fmt.Sprintf(lang.T("スロット%dを ロードしますか？（セーブしていない 進み具合は 消えます）"), i+1), func() { g.loadSavedGame(st) }))
		return
	}
	g.loadSavedGame(st)
}

func (s *saveMenu) write(g *Game, i int) {
	if err := g.save(i); err != nil {
		s.msg = lang.T("セーブできなかった: ") + err.Error()
	} else {
		s.msg = fmt.Sprintf(lang.T("スロット%dに セーブしました。"), i+1)
		g.sfx("item")
	}
	s.refresh(g)
	s.msgT = 80
}

func (s *saveMenu) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0x90))
	title := lang.T("どこに セーブしますか？")
	if !s.saving {
		title = lang.T("どの きろくから はじめますか？")
	}
	gfx.DrawTextOutlineColor(dst, title, sw/2, s.slots.y-70, sizeL, g.th.text, g.th.window, 1)
	s.slots.draw(g, dst, s.msgT == 0)
	if s.msgT > 0 {
		g.window(dst, sw/2-300, s.slots.y+s.slots.height()+20, 600, 64)
		gfx.DrawText(dst, s.msg, sw/2, s.slots.y+s.slots.height()+36, sizeM, g.th.text)
	}
}
