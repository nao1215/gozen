package play

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
)

type menu struct {
	tabs   *list
	mode   string
	member int
	picker bool
	items  *list
	ids    []string

	slot  int
	slots *list
	cands *list

	using    string
	targetOn bool
	tsel     int
	msgs     []string
	msgT     int

	msgSoft bool

	cmds []string

	direct string
	done   func()
}

func newDirectMenu(g *Game, mode string, done func()) *menu {
	m := newMenu(g)
	m.direct, m.done = mode, done
	switch mode {
	case "status":
		m.mode, m.member = "status", 0
	case "item":
		m.mode = "item"
		m.openItems(g)
	case "skill", "equip":
		m.picker, m.mode = true, mode
	}
	return m
}

func newMenu(g *Game) *menu {
	var cmds []string
	var head []listItem
	for _, id := range g.st.Bag() {
		if it := g.db.Items[id]; it.Command && eventItemUsable(g, it) {
			cmds = append(cmds, id)
			head = append(head, listItem{label: it.Name, id: id})
		}
	}
	items := []listItem{{label: lang.T("ステータス")}, {label: lang.T("どうぐ")}, {label: lang.T("スキル")}, {label: lang.T("そうび")}, {label: lang.T("セーブ")}, {label: lang.T("ロード")}, {label: lang.T("せってい")}, {label: lang.T("タイトルへ")}, {label: lang.T("とじる")}}
	if loc := g.db.Locations[g.st.Location]; loc == nil || !loc.Safe {
		items[4].disabled = true
		items[4].right = "×"
	}
	if !g.anySave() {
		items[5].disabled = true
	}
	items = append(head, items...)
	m := &menu{tabs: newList(items, 36, 36, 250, len(items)), cmds: cmds}
	return m
}

func eventItemUsable(g *Game, it *data.Item) bool {
	if it.Event == "" || g.db.Events[it.Event] == nil {
		return false
	}
	if !it.EventOK.Set() {
		return true
	}
	ok, err := it.EventOK.Bool(g.st.Env())
	return err == nil && ok
}

func (m *menu) overlay() bool { return true }

func (m *menu) liveKeep(g *Game) scene {
	n := newMenu(g)
	sel := m.tabs.sel - len(m.cmds) + len(n.cmds)
	if sel < 0 || sel >= len(n.tabs.items) {
		sel = m.tabs.sel
	}
	n.tabs.sel = clampInt(sel, 0, len(n.tabs.items)-1)
	if m.mode == "status" {
		n.mode, n.member = "status", clampInt(m.member, 0, max(0, len(n.members(g))-1))
	}
	return n
}

func (m *menu) members(g *Game) []*rpg.Actor { return g.st.Members() }

func (m *menu) update(g *Game) {
	if m.msgT > 0 && m.msgSoft {
		m.msgT--
	} else if m.msgT > 0 {
		m.msgT--
		if m.msgT < 60 && (g.in.advance() || g.in.cancel()) {
			m.msgT = 0
		}
		return
	}
	if m.targetOn {
		m.updateTarget(g)
		return
	}
	if m.direct != "" && m.mode == "" && !m.picker {
		g.pop()
		if m.done != nil {
			m.done()
		}
		return
	}
	switch {
	case m.picker:
		m.updatePicker(g)
	case m.mode == "item":
		m.updateItems(g)
	case m.mode == "skill":
		m.updateSkills(g)
	case m.mode == "equip":
		m.updateEquip(g)
	case m.mode == "status":
		ms := m.members(g)
		if g.in.left() || g.in.up() || g.in.tab() < 0 {
			m.member = (m.member - 1 + len(ms)) % len(ms)
			g.sfx("cursor")
		}
		if g.in.right() || g.in.down() || g.in.tab() > 0 {
			m.member = (m.member + 1) % len(ms)
			g.sfx("cursor")
		}
		if g.in.cancel() || g.in.ok() || g.in.click {
			g.sfx("cancel")
			m.mode = ""
		}
	default:
		chose, cancel := m.tabs.update(g)
		if cancel {
			g.pop()
			return
		}
		if !chose {
			return
		}
		if m.tabs.sel < len(m.cmds) {
			it := g.db.Items[m.cmds[m.tabs.sel]]
			if !eventItemUsable(g, it) {
				g.sfx("buzzer")
				return
			}
			if it.Consumed() {
				g.st.AddItem(m.cmds[m.tabs.sel], -1)
			}
			g.pop()
			g.events.Start(g.db.Events[it.Event].Do)
			return
		}
		switch m.tabs.sel - len(m.cmds) {
		case 0:
			m.mode, m.member = "status", 0
		case 1:
			m.mode = "item"
			m.openItems(g)
		case 2, 3:
			m.picker = true
			m.mode = []string{"", "", "skill", "equip"}[m.tabs.sel-len(m.cmds)]
		case 4:
			g.push(newSaveMenu(g, true, func() { m.tabs.items[len(m.cmds)+5].disabled = !g.anySave() }))
		case 5:
			sm := newSaveMenu(g, false, func() {})
			sm.inPlay = true
			g.push(sm)
		case 6:
			g.push(newConfigScreen(g))
		case 7:
			g.push(newConfirm(lang.T("タイトルに もどりますか？（セーブして いない ところは 消えます）"), func() {
				g.st = nil
				g.replace(newTitle(g))
			}))
		case 8:
			g.pop()
		}
	}
}

func (m *menu) updatePicker(g *Game) {
	ms := m.members(g)
	if g.in.cancel() {
		g.sfx("cancel")
		m.picker, m.mode = false, ""
		return
	}
	if g.in.up() || g.in.left() {
		m.member = (m.member - 1 + len(ms)) % len(ms)
		g.sfx("cursor")
	}
	if g.in.down() || g.in.right() {
		m.member = (m.member + 1) % len(ms)
		g.sfx("cursor")
	}
	for i := range ms {
		if g.in.mouse.In(m.cardRect(g, i)) && (g.in.moved || g.in.click) {
			m.member = i
		}
	}
	if g.in.ok() || g.in.click {
		g.sfx("decide")
		m.picker = false
		if m.mode == "skill" {
			m.openSkills(g)
		} else {
			m.openEquip(g)
		}
	}
}

func (m *menu) openItems(g *Game) {
	var items []listItem
	m.ids = nil
	for _, id := range g.st.Bag() {
		it := g.db.Items[id]
		if it.Command {
			continue
		}
		usable := battle.Usable(g.db, "", id)
		if it.Event != "" && g.db.Events[it.Event] != nil {
			usable = eventItemUsable(g, it)
		}
		label := it.Name
		if it.Equipment() {
			label = lang.T("［装備］") + label
		}
		items = append(items, listItem{label: label, right: fmt.Sprintf("×%d", g.st.Items[id]), disabled: !usable, id: id})
		m.ids = append(m.ids, id)
	}
	sel := 0
	if m.items != nil {
		sel = min(m.items.sel, max(0, len(items)-1))
	}
	_, sh := g.screen()
	m.items = newList(items, 300, 36, menuListW, int((sh-200)/44))
	m.items.sel = sel
	m.items.scroll()
}

func (m *menu) updateItems(g *Game) {
	chose, cancel := m.items.update(g)
	if cancel {
		m.mode = ""
		return
	}
	if !chose || len(m.ids) == 0 {
		return
	}
	id := m.ids[m.items.sel]
	if it := g.db.Items[id]; it.Event != "" && g.db.Events[it.Event] != nil {
		if it.EventOK.Set() {
			if ok, err := it.EventOK.Bool(g.st.Env()); err != nil || !ok {
				g.sfx("buzzer")
				m.msgs, m.msgT, m.msgSoft = []string{lang.T("いまは つかえない。")}, 90, false
				return
			}
		}
		if it.Consumed() {
			g.st.AddItem(id, -1)
		}
		g.pop()
		g.events.Start(g.db.Events[it.Event].Do)
		return
	}
	m.use(g, "item:"+id, "", id)
}

func (m *menu) use(g *Game, key, skill, item string) {
	m.using = key
	if battle.NeedsTarget(g.db, skill, item) {
		m.targetOn, m.tsel = true, 0
		for i, a := range m.members(g) {
			if battle.CanTarget(g.db, skill, item, a) {
				m.tsel = i
				break
			}
		}
		return
	}
	m.apply(g, nil)
}

func (m *menu) updateTarget(g *Game) {
	ms := m.members(g)
	if g.in.cancel() {
		g.sfx("cancel")
		m.targetOn = false
		return
	}
	if g.in.up() || g.in.left() {
		m.tsel = (m.tsel - 1 + len(ms)) % len(ms)
		g.sfx("cursor")
	}
	if g.in.down() || g.in.right() {
		m.tsel = (m.tsel + 1) % len(ms)
		g.sfx("cursor")
	}
	for i := range ms {
		if g.in.mouse.In(m.cardRect(g, i)) && (g.in.moved || g.in.click) {
			m.tsel = i
		}
	}
	if g.in.ok() || g.in.click {
		kind, id, _ := strings.Cut(m.using, ":")
		skill, item := id, ""
		if kind == "item" {
			skill, item = "", id
		}
		if !battle.CanTarget(g.db, skill, item, ms[m.tsel]) {
			g.sfx("buzzer")
			return
		}
		m.apply(g, ms[m.tsel])
	}
}

func (m *menu) apply(g *Game, target *rpg.Actor) {
	kind, id, _ := strings.Cut(m.using, ":")
	user := m.members(g)[m.member]
	var msgs []string
	var err error
	if kind == "item" {
		msgs, err = battle.UseInMenu(g.db, g.st, user, "", id, target)
	} else {
		msgs, err = battle.UseInMenu(g.db, g.st, user, id, "", target)
	}
	if err != nil {
		g.sfx("buzzer")
		m.msgs, m.msgT, m.msgSoft = []string{err.Error()}, 90, false
		return
	}
	g.sfx("heal")
	m.msgs, m.msgT, m.msgSoft = msgs, 100, true
	if kind == "item" {
		m.openItems(g)
		if g.st.Items[id] <= 0 {
			m.targetOn = false
		}
	} else {
		m.openSkills(g)
		if user.MP < g.db.Skills[id].MP {
			m.targetOn = false
		}
	}
}

func (m *menu) openSkills(g *Game) {
	a := m.members(g)[m.member]
	var items []listItem
	m.ids = nil
	for _, id := range a.Skills() {
		sk := g.db.Skills[id]
		usable := battle.Usable(g.db, id, "") && a.MP >= sk.MP && a.Alive()
		items = append(items, listItem{label: sk.Name, right: fmt.Sprintf("%s %d", g.db.Game.Terms.MP, sk.MP), disabled: !usable})
		m.ids = append(m.ids, id)
	}
	sel := 0
	if m.items != nil && m.mode == "skill" {
		sel = min(m.items.sel, max(0, len(items)-1))
	}
	_, sh := g.screen()
	m.items = newList(items, 300, 36, menuListW, int((sh-200)/44))
	m.items.sel = sel
}

func (m *menu) updateSkills(g *Game) {
	if g.in.tab() != 0 {
		ms := m.members(g)
		d := 1
		if g.in.tab() < 0 {
			d = len(ms) - 1
		}
		m.member = (m.member + d) % len(ms)
		g.sfx("cursor")
		m.items.sel = 0
		m.openSkills(g)
		return
	}
	chose, cancel := m.items.update(g)
	if cancel {
		m.picker = true
		return
	}
	if !chose || len(m.ids) == 0 {
		return
	}
	id := m.ids[m.items.sel]
	m.use(g, "skill:"+id, id, "")
}

func (m *menu) openEquip(g *Game) {
	a := m.members(g)[m.member]
	var items []listItem
	for _, s := range g.db.Game.Slots {
		name := "―"
		if it := g.db.Items[a.Equip[s.ID]]; it != nil {
			name = it.Name
		}
		items = append(items, listItem{label: s.Name + lang.T("：") + name})
	}

	items = append(items, listItem{label: lang.T("★ さいきょうそうび（ぜんいん）")})
	m.slots = newList(items, 300, 36, menuListW, len(items))
	m.slots.sel = m.slot
	m.cands = nil
}

func (m *menu) candidates(g *Game) {
	a := m.members(g)[m.member]
	slot := g.db.Game.Slots[m.slot].ID
	items := []listItem{{label: lang.T("（はずす）"), id: ""}}
	for _, id := range g.st.Bag() {
		it := g.db.Items[id]
		if it.Fits(slot) && a.CanEquip(id) {
			items = append(items, listItem{label: it.Name, right: fmt.Sprintf("×%d", g.st.Items[id]), id: id})
		}
	}

	_, sh := g.screen()
	y := 36 + m.slots.height() + 16
	rows := int((sh - 24 - compareH() - 12 - y - 24) / 44)
	m.cands = newList(items, 300, y, menuListW, max(1, min(5, rows)))
	m.cands.sel = 0
	for i, it := range items {
		if it.id != "" && it.id == a.Equip[slot] {
			m.cands.sel = i
		}
	}
	m.cands.scroll()
}

func (m *menu) updateEquip(g *Game) {
	if m.cands != nil {
		if g.in.tab() != 0 {
			ms := m.members(g)
			d := 1
			if g.in.tab() < 0 {
				d = len(ms) - 1
			}
			m.member = (m.member + d) % len(ms)
			g.sfx("cursor")
			m.openEquip(g)
			m.candidates(g)
			return
		}
		chose, cancel := m.cands.update(g)
		if cancel {
			m.cands = nil
			return
		}
		if chose {
			a := m.members(g)[m.member]
			slot := g.db.Game.Slots[m.slot].ID
			if err := g.st.Equip(a, slot, m.cands.items[m.cands.sel].id); err != nil {
				g.sfx("buzzer")
				return
			}
			g.sfx("item")
			m.openEquip(g)
		}
		return
	}
	chose, cancel := m.slots.update(g)
	m.slot = m.slots.sel
	if cancel {
		m.picker = true
		return
	}
	if g.in.left() || g.in.right() || g.in.tab() != 0 {
		ms := m.members(g)
		d := 1
		if g.in.left() || g.in.tab() < 0 {
			d = len(ms) - 1
		}
		m.member = (m.member + d) % len(ms)
		g.sfx("cursor")
		m.openEquip(g)
		return
	}
	if chose {
		if m.slot >= len(g.db.Game.Slots) {
			if g.equipBestAll() == 0 {
				m.msgs, m.msgT, m.msgSoft = []string{lang.T("いまの そうびが いちばん 強い。")}, 70, false
				g.sfx("cancel")
			} else {
				m.msgs, m.msgT, m.msgSoft = []string{lang.T("ぜんいんを さいきょうそうびに した。")}, 70, false
			}
			m.openEquip(g)
			return
		}
		m.candidates(g)
	}
}

const menuListW = 420

func (m *menu) cardRect(g *Game, i int) image.Rectangle {
	sw, _ := g.screen()
	const w, h, gap = 250, 120, 8
	x := int(sw) - 36 - 2*w - gap + (i%2)*(w+gap)
	y := 36 + (i/2)*(h+gap)
	return image.Rect(x, y, x+w, y+h)
}

func (m *menu) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, withAlpha(colorBlack, 0x80))
	if m.direct == "" {
		m.tabs.draw(g, dst, m.mode == "" && !m.picker)
	}

	t := g.db.Game.Terms
	g.window(dst, 36, sh-110, 250, 74)
	gfx.DrawTextRight(dst, fmt.Sprintf("%d %s", g.st.Gold, t.Currency), 260, sh-90, sizeM, g.th.text)
	ms := m.members(g)
	switch m.mode {
	case "status":
		m.drawStatus(g, dst, ms[m.member])
		return
	case "item", "skill":
		if m.items != nil && !m.picker {
			m.items.draw(g, dst, !m.targetOn)
			if len(m.ids) > 0 {
				id := m.ids[m.items.sel]
				var desc string
				if m.mode == "item" {
					desc = g.db.Items[id].Desc
					if it := g.db.Items[id]; it.Equipment() {
						desc = equipDesc(g, it) + "  " + desc
					}
				} else {
					desc = g.db.Skills[id].Desc
				}

				ls, sz := gfx.FitLines(desc, sizeS*0.9, menuListW+100, 3)
				h := 74 + float64(max(0, len(ls)-2))*gfx.LineHeight(sz)
				g.window(dst, 300, sh-36-h, menuListW+140, h)
				for i, ln := range ls {
					g.text(dst, ln, 324, sh-22-h+float64(i)*gfx.LineHeight(sz), sz, g.th.text)
				}
			}
		}
	case "equip":
		if !m.picker && m.slots != nil {
			m.slots.draw(g, dst, m.cands == nil)
			if m.cands != nil {
				m.cands.draw(g, dst, true)
				m.drawCompare(g, dst, ms[m.member])
			}
		}
	}
	if len(g.stack) > 0 && g.stack[len(g.stack)-1] != scene(m) {
		return
	}
	for i, a := range ms {
		r := m.cardRect(g, i)
		active := (m.picker && i == m.member) || (m.targetOn && i == m.tsel) || (!m.picker && (m.mode == "skill" || m.mode == "equip") && i == m.member)
		g.drawActorCard(dst, a, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), active, "")
	}

	last := m.cardRect(g, max(0, len(ms)-1))
	hx, hy := float64(m.cardRect(g, 0).Min.X+m.cardRect(g, 1).Max.X)/2, float64(last.Max.Y)+24
	switch {
	case m.targetOn:
		gfx.DrawTextOutlineColor(dst, lang.T("だれに つかう？"), hx, hy, sizeM, g.th.text, g.th.window, 1)
	case m.picker:
		gfx.DrawTextOutlineColor(dst, lang.T("だれの？"), hx, hy, sizeM, g.th.text, g.th.window, 1)
	case m.mode == "skill" && m.items != nil:
		gfx.DrawTextOutlineColor(dst, lang.T("Tab / Shift+Tab でメンバーを かえる"), hx, hy, sizeS*0.9, g.th.text, g.th.window, 1)
	case m.mode == "equip" && m.cands != nil:
		gfx.DrawTextOutlineColor(dst, lang.T("Tab / Shift+Tab でメンバーを かえる"), hx, hy, sizeS*0.9, g.th.text, g.th.window, 1)
	case m.mode == "equip" && m.slots != nil:
		gfx.DrawTextOutlineColor(dst, lang.T("← → / Tab でメンバーを かえる"), hx, hy, sizeS*0.9, g.th.text, g.th.window, 1)
	}
	if m.msgT > 0 {
		h := float64(len(m.msgs))*36 + 40
		g.window(dst, sw/2-360, sh/2-h/2, 720, h)
		for i, s := range m.msgs {
			gfx.DrawText(dst, s, sw/2, sh/2-h/2+20+float64(i)*36, sizeM, g.th.text)
		}
	}
}

func equipDesc(_ *Game, it *data.Item) string {
	s := ""
	for _, st := range data.StatNames {
		if v := it.Stats[st]; v != 0 {
			s += fmt.Sprintf("%s%+d ", lang.T(data.StatLabels[st]), v)
		}
	}
	return s
}

const compareRowH = 34

func compareH() float64 { return float64((len(data.StatNames)+1)/2)*compareRowH + 22 }

func (m *menu) drawCompare(g *Game, dst *ebiten.Image, a *rpg.Actor) {
	slot := g.db.Game.Slots[m.slot].ID
	id := m.cands.items[m.cands.sel].id
	x, y := m.cands.x, m.cands.y+m.cands.height()+12
	g.window(dst, x, y, m.cands.w, compareH())
	for i, st := range data.StatNames {
		now, then := a.Stat(st), a.StatWith(st, slot, id)
		c := g.th.text
		if then > now {
			c = hpColor(1)
		} else if then < now {
			c = hpColor(0)
		}
		xx := x + 20 + float64(i%2)*(m.cands.w/2-10)
		yy := y + 14 + float64(i/2)*compareRowH
		g.text(dst, lang.T(data.StatLabels[st]), xx, yy, sizeS*0.85, g.th.border)
		gfx.DrawTextRight(dst, fmt.Sprintf("%d → %d", now, then), xx+m.cands.w/2-30, yy, sizeS*0.85, c)
	}
}

func (m *menu) drawStatus(g *Game, dst *ebiten.Image, a *rpg.Actor) {
	sw, sh := g.screen()
	d := a.Def()
	if d.Faces != "" {
		g.drawStanding(dst, g.faceOf(a.ID, a.Face()), sw-200, sh-8, sh-24, 1, false, nil)
	}
	x, y, w := 300.0, 36.0, 680.0
	g.window(dst, x, y, w, sh-72)
	t := g.db.Game.Terms
	g.text(dst, a.Name(), x+30, y+20, sizeXL*0.8, d.Color.Or(g.th.text))
	g.text(dst, fmt.Sprintf("%s  %s %d", a.Class().Name, t.Level, a.Level), x+30, y+74, sizeM, g.th.border)
	g.text(dst, fmt.Sprintf("%s %d / %d", t.HP, a.HP, a.Stat("mhp")), x+30, y+114, sizeM, g.th.text)
	g.text(dst, fmt.Sprintf("%s %d / %d", t.MP, a.MP, a.Stat("mmp")), x+30, y+150, sizeM, g.th.text)
	next := a.ExpNext()
	g.text(dst, fmt.Sprintf(lang.T("%s %d   つぎのレベルまで %d"), t.Exp, a.Exp, next), x+30, y+192, sizeS*0.9, g.th.text)

	for i, st := range data.StatNames[2:] {
		yy := y + 232 + float64(i)*34
		g.text(dst, lang.T(data.StatLabels[st]), x+30, yy, sizeS*0.9, g.th.border)
		gfx.DrawTextRight(dst, strconv.Itoa(a.Stat(st)), x+300, yy, sizeS*0.9, g.th.text)
	}
	yy := y + 114
	for _, s := range g.db.Game.Slots {
		name := "―"
		if it := g.db.Items[a.Equip[s.ID]]; it != nil {
			name = it.Name
		}
		g.text(dst, s.Name, x+350, yy, sizeS*0.9, g.th.border)
		g.text(dst, name, x+470, yy, sizeS*0.9, g.th.text)
		yy += 34
	}

	yy += 10
	for _, mt := range g.shownMeters(a.ID, true) {
		g.drawMeterRow(dst, mt, x+350, yy, w-380)
		yy += 34
	}
	for _, sid := range g.db.Ordered("states") {
		st := g.db.States[sid]
		if a.States[sid] == 0 || st.Desc == "" {
			continue
		}
		g.text(dst, st.Name, x+350, yy, sizeS*0.8, st.Color.Or(g.th.accent))
		yy += 26
		lines := gfx.Wrap(st.Desc, sizeS*0.72, w-380)
		g.drawLines(dst, st.Desc, x+350, yy, w-380, sizeS*0.72, withAlpha(g.th.text, 0xd0))
		yy += float64(len(lines))*gfx.LineHeight(sizeS*0.72) + 8
	}
	if d.Profile != "" {
		py := y + 232 + float64(len(data.StatNames)-2)*34 + 12
		g.drawLines(dst, d.Profile, x+30, py, w-60, sizeS*0.85, withAlpha(g.th.text, 0xd0))
	}
	gfx.DrawTextOutlineColor(dst, lang.T("← → / Tab: メンバー   X: もどる"), x+w/2, sh-70, sizeS*0.85, g.th.text, g.th.window, 1)
}

type confirm struct {
	msg  string
	yes  func()
	onNo func()
	age  int
	no   bool
}

func newConfirm(msg string, yes func()) *confirm { return &confirm{msg: msg, yes: yes} }

func (c *confirm) overlay() bool { return true }

func (c *confirm) layout(g *Game) (lines []string, size, y, h float64, yes, no image.Rectangle) {
	sw, sh := g.screen()
	lines, size = gfx.FitLines(c.msg, sizeM, 640, 2)
	h = 104 + float64(len(lines))*gfx.LineHeight(size)
	y = sh/2 - h/2
	by := int(y + h - 66)
	yes = image.Rect(int(sw/2-170), by, int(sw/2-20), by+44)
	no = image.Rect(int(sw/2+20), by, int(sw/2+170), by+44)
	return
}

func (c *confirm) update(g *Game) {
	c.age++
	if c.age < 6 {
		return
	}
	_, _, _, _, yes, no := c.layout(g)
	if g.in.moved {
		if g.in.mouse.In(yes) {
			c.no = false
		} else if g.in.mouse.In(no) {
			c.no = true
		}
	}
	if g.in.left() || g.in.right() {
		c.no = !c.no
		g.sfx("cursor")
	}
	accept, refuse := false, g.in.cancel()
	if g.in.ok() {
		accept, refuse = !c.no, c.no
	}
	if g.in.click {
		accept, refuse = g.in.mouse.In(yes), g.in.mouse.In(no)
	}
	switch {
	case accept:
		g.sfx("decide")
		g.pop()
		c.yes()
	case refuse:
		g.sfx("cancel")
		g.pop()
		if c.onNo != nil {
			c.onNo()
		}
	}
}

func (c *confirm) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0x70))
	ls, sz, y, h, yes, no := c.layout(g)
	g.window(dst, sw/2-360, y, 720, h)
	for i, ln := range ls {
		gfx.DrawText(dst, ln, sw/2, y+20+float64(i)*gfx.LineHeight(sz), sz, g.th.text)
	}
	for i, r := range []image.Rectangle{yes, no} {
		label := lang.T("はい")
		if i == 1 {
			label = lang.T("いいえ")
		}
		on := (i == 1) == c.no
		if on {
			gfx.FillRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 10, withAlpha(g.th.accent, 0x70))
		}
		gfx.StrokeRoundRect(dst, float32(r.Min.X), float32(r.Min.Y), float32(r.Dx()), float32(r.Dy()), 10, 2, g.th.border)
		c2 := g.th.dim
		if on {
			c2 = g.th.text
		}
		gfx.DrawText(dst, label, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y)+(float64(r.Dy())-sizeM*1.3)/2, sizeM, c2)
	}
}
