package play

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"strconv"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
)

func (g *Game) openScreen(name string, done func()) {
	if g.db.Game.Screens[name] != nil && g.st != nil {
		g.push(newAuthorScreen(g, name, done))
		return
	}
	if g.st != nil && len(g.st.Party) == 0 && slices.Contains([]string{"status", "skill", "equip", "formation", "skills", "tactics", "fusion"}, name) {
		done()
		return
	}
	switch name {
	case "tactics":
		if g.currentBattle() != nil || g.st == nil {
			done()
			return
		}
		g.push(newTacticsScreen(g, done))
	case "formation":
		g.push(newFormation(g, done))
	case "skills":
		g.push(newTreeView(g, done))
	case "fusion":
		g.push(newFusion(g, done))
	case "profiles":
		g.push(newProfiles(g, done))
	case "bestiary":
		g.push(newBestiary(g, done))
	case "ranch":
		g.push(newRanch(g, done))
	case "streams":
		if g.db.Streams == nil || g.st == nil {
			done()
			return
		}
		g.push(newStreamLobby(g, done))

	case "status", "item", "skill", "equip":
		g.push(newDirectMenu(g, name, done))
	case "save":
		g.push(newSaveMenu(g, true, done))
	case "load":
		if !g.anySave() {
			done()
			return
		}
		sm := newSaveMenu(g, false, done)
		sm.inPlay = true
		g.push(sm)
	case "config":
		c := newConfigScreen(g)
		c.done = done
		g.push(c)
	case "title":
		c := newConfirm(lang.T("タイトルに もどりますか？（セーブして いない ところは 消えます）"), func() {
			g.st = nil
			g.replace(newTitle(g))
		})
		c.onNo = done
		g.push(c)
	default:
		done()
	}
}

func (g *Game) standingOf(a *rpg.Actor) string {
	d := a.Def()
	if d.Faces == "" {
		return ""
	}
	return d.Faces + "/" + a.Face() + ".png"
}

type formation struct {
	ids  []string
	sel  int
	msg  string
	msgT int
	done func()
}

func newFormation(g *Game, done func()) *formation {
	f := &formation{done: done}
	for _, id := range g.db.Order["actors"] {
		if g.st.Actors[id] != nil {
			f.ids = append(f.ids, id)
		}
	}
	return f
}

func (f *formation) overlay() bool { return true }

func (f *formation) update(g *Game) {
	if f.msgT > 0 {
		f.msgT--
	}
	n := len(f.ids)
	switch {
	case g.in.cancel():
		if len(g.st.Party) == 0 {
			f.msg, f.msgT = lang.T("だれか ひとりは つれていこう。"), 90
			return
		}
		g.sfx("cancel")
		g.pop()
		f.done()
	case g.in.up():
		f.sel = (f.sel - 1 + n) % n
		g.sfx("cursor")
	case g.in.down():
		f.sel = (f.sel + 1) % n
		g.sfx("cursor")
	case g.in.left() || g.in.right():
		a := g.st.Actors[f.ids[f.sel]]
		a.Back = !a.Back
		g.sfx("cursor")
	case g.in.ok():
		id := f.ids[f.sel]
		if slices.Contains(g.st.Party, id) {
			if g.st.Leave(id) {
				g.sfx("cancel")
			} else {
				g.sfx("buzzer") // the last member stays
			}
		} else if g.st.Join(id) {
			g.sfx("decide")
		} else {
			f.msg, f.msgT = fmt.Sprintf(lang.T("ダンジョンに いけるのは %dにん までだ。"), g.db.Game.MaxParty), 90
			g.sfx("buzzer")
		}
	}
}

func (f *formation) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, color.NRGBA{0, 0, 0, 0x90})
	g.window(dst, 30, 30, 560, sh-60)
	g.text(dst, fmt.Sprintf(lang.T("へんせい　（%d / %d にん）"), len(g.st.Party), g.db.Game.MaxParty), 56, 50, sizeM, g.th.accent)
	g.text(dst, lang.T("Z: つれていく／はずす　←→: 前列／後列　X: おわる"), 56, sh-80, sizeS, g.th.dim)
	for i, id := range f.ids {
		a := g.st.Actors[id]
		y := 100 + float64(i)*52
		if i == f.sel {
			rect(dst, 44, y-6, 532, 48, withAlpha(g.th.cursor, 0x60))
		}
		in := slices.Index(g.st.Party, id)
		mark := "　"
		if in >= 0 {
			mark = strconv.Itoa(in + 1)
		}
		g.text(dst, mark, 60, y, sizeM, g.th.accent)
		g.text(dst, a.Name(), 100, y, sizeM, g.th.text)
		g.text(dst, fmt.Sprintf("%s%d", g.db.Game.Terms.Level, a.Level), 270, y+4, sizeS, g.th.dim)
		row := lang.T("前列")
		rc := color.NRGBA{0xff, 0xc0, 0x80, 0xff}
		if a.Back {
			row, rc = lang.T("後列"), color.NRGBA{0x90, 0xc8, 0xff, 0xff}
		}
		g.text(dst, row, 350, y+4, sizeS, rc)
		g.text(dst, a.Class().Name, 430, y+4, sizeS, g.th.dim)
	}

	a := g.st.Actors[f.ids[f.sel]]
	if p := g.standingOf(a); p != "" {
		g.drawStanding(dst, p, sw*0.72, sh, sh*0.95, 1, false, nil)
	}
	g.window(dst, sw-430, sh-200, 400, 170)
	g.text(dst, a.Name()+"　"+a.Class().Name, sw-410, sh-186, sizeM, g.th.text)
	g.text(dst, fmt.Sprintf("HP %d/%d　MP %d/%d", a.HP, a.Stat("mhp"), a.MP, a.Stat("mmp")), sw-410, sh-146, sizeS, g.th.text)
	g.text(dst, fmt.Sprintf(lang.T("こうげき %d　ぼうぎょ %d　まりょく %d"), a.Stat("atk"), a.Stat("def"), a.Stat("mat")), sw-410, sh-114, sizeS, g.th.dim)
	g.text(dst, fmt.Sprintf(lang.T("すばやさ %d　SP %d"), a.Stat("agi"), a.SP), sw-410, sh-82, sizeS, g.th.dim)
	if f.msgT > 0 {
		g.window(dst, sw/2-300, sh/2-40, 600, 80)
		gfx.DrawText(dst, f.msg, sw/2, sh/2-14, sizeM, g.th.text)
	}
}

type treeView struct {
	ids    []string
	member int
	sel    int
	msg    string
	msgT   int
	done   func()
}

func newTreeView(g *Game, done func()) *treeView {
	t := &treeView{done: done}

	seen := map[string]bool{}
	var order []string
	for _, a := range g.st.Members() {
		order = append(order, a.ID)
	}
	order = append(order, g.db.Order["actors"]...)
	for _, id := range order {
		if a := g.st.Actors[id]; a != nil && !seen[id] && len(a.Class().Tree) > 0 {
			seen[id] = true
			t.ids = append(t.ids, id)
		}
	}
	return t
}

func (t *treeView) overlay() bool { return true }

func (t *treeView) actor(g *Game) *rpg.Actor { return g.st.Actors[t.ids[t.member]] }

func (t *treeView) update(g *Game) {
	if t.msgT > 0 {
		t.msgT--
	}
	if len(t.ids) == 0 || g.in.cancel() {
		g.sfx("cancel")
		g.pop()
		t.done()
		return
	}
	a := t.actor(g)
	nodes := a.Class().Tree

	cur := nodes[t.sel]
	move := func(dc int) {
		best, bd := -1, 1<<30
		for i, n := range nodes {
			if n.Col != cur.Col+dc {
				continue
			}
			if d := abs(n.Row - cur.Row); d < bd {
				best, bd = i, d
			}
		}
		if best >= 0 {
			t.sel = best
			g.sfx("cursor")
		}
	}
	vert := func(dr int) {
		best, bd := -1, 1<<30
		for i, n := range nodes {
			if n.Col != cur.Col || (n.Row-cur.Row)*dr <= 0 {
				continue
			}
			if d := abs(n.Row - cur.Row); d < bd {
				best, bd = i, d
			}
		}
		if best >= 0 {
			t.sel = best
			g.sfx("cursor")
		}
	}

	click, raise := false, false
	if g.in.click {
		g.in.click = false
		for i, n := range nodes {
			if g.in.mouse.In(treeNodeRect(n.Col, n.Row)) {
				click = true
				if i == t.sel {
					raise = true
				} else {
					t.sel = i
					g.sfx("cursor")
				}
			}
		}
		if !click && g.in.mouse.Y < 110 {
			t.member = (t.member + 1) % len(t.ids)
			t.sel, t.msgT = 0, 0
			g.sfx("cursor")
			return
		}
		if !raise {
			return
		}
	}
	switch {
	case raise:
		t.raise(g, a, cur)
	case g.in.left():
		move(-1)
	case g.in.right():
		move(1)
	case g.in.up():
		vert(-1)
	case g.in.down():
		vert(1)
	case g.in.pressed[btnMenu] || g.in.pressed[btnNext]:
		if g.in.tab() < 0 {
			t.member = (t.member + len(t.ids) - 1) % len(t.ids)
		} else {
			t.member = (t.member + 1) % len(t.ids)
		}
		t.sel, t.msgT = 0, 0
		g.sfx("cursor")
	case g.in.ok():
		t.raise(g, a, cur)
	}
}

func (t *treeView) raise(g *Game, a *rpg.Actor, cur gamedata.TreeNode) {
	if a.Raise(cur.ID) {
		g.sfx("decide")
		t.msg, t.msgT = fmt.Sprintf(lang.T("%s が Lv%d に なった！"), t.nodeName(g, cur.ID), a.Tree[cur.ID]), 60
	} else {
		g.sfx("buzzer")
		switch {
		case a.Tree[cur.ID] >= cur.Max:
			t.msg = lang.T("これいじょう あげられない。")
		case !a.NodeReached(cur.ID):
			t.msg = lang.T("まだ とれない。（さきに ひつような スキルを あげよう）")
		default:
			t.msg = lang.T("SPが たりない。")
		}
		t.msgT = 90
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (t *treeView) nodeName(g *Game, id string) string { return t.actor(g).NodeName(id) }

func (t *treeView) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, color.NRGBA{0x08, 0x0c, 0x18, 0xe8})
	if len(t.ids) == 0 {
		return
	}
	a := t.actor(g)
	if p := g.standingOf(a); p != "" {
		cs := &ebiten.ColorScale{}
		cs.ScaleAlpha(0.3)
		g.drawStanding(dst, p, sw*0.8, sh, sh-headRoom, 1, false, cs)
	}
	g.text(dst, fmt.Sprintf(lang.T("スキルツリー　%s（%s）"), a.Name(), a.Class().Name), 40, 28, sizeL, g.th.text)
	perLv := max(1, a.Class().SP)
	if g.db.Game.SchemaVersion >= gamedata.SchemaVersion1 {
		perLv = a.Class().SP
	}
	if a.SP > 0 {
		g.text(dst, fmt.Sprintf(lang.T("のこり SP %d"), a.SP), 40, 74, sizeM, g.th.accent)
		g.text(dst, lang.T("＋の 付いた スキルを あげられる"), 220, 80, sizeS*0.85, treeCanColor)
	} else {
		g.text(dst, lang.T("のこり SP 0"), 40, 74, sizeM, treeNoSPColor)
		g.text(dst, fmt.Sprintf(lang.T("SP が ない。レベルが 上がると +%d"), perLv), 220, 80, sizeS*0.85, treeNoSPColor)
	}
	g.text(dst, lang.T("Z: あげる　矢印: えらぶ　C / Tab: つぎの なかま　X: もどる　（マウス: クリックで えらび、もう一度で あげる。上の 名前で つぎの なかま）"), 40, sh-50, sizeS*0.8, g.th.dim)
	nodes := a.Class().Tree
	const x0, y0, cw, rh, bw, bh = 48.0, 140.0, 300.0, 104.0, 230.0, 72.0
	const gx, gy = (cw - bw) / 2, (rh - bh) / 2
	pos := func(col, row int) (float64, float64) { return x0 + float64(col)*cw, y0 + float64(row)*rh }

	for _, n := range nodes {
		x, y := pos(n.Col, n.Row)
		for _, r := range n.Requires {
			p := a.Node(r.Node)
			if p == nil {
				continue
			}
			px, py := pos(p.Col, p.Row)
			c := color.NRGBA{0x50, 0x5c, 0x78, 0xff}
			if a.Tree[r.Node] >= r.Level {
				c = color.NRGBA{0x8f, 0xc8, 0xf0, 0xff}
			}
			ay, by := py+bh/2, y+bh/2
			pts := [][2]float64{{px + bw, ay}}
			if n.Col-p.Col <= 1 {
				mx := px + bw + gx
				pts = append(pts, [2]float64{mx, ay}, [2]float64{mx, by})
			} else {
				lane := y - gy
				if n.Row < p.Row {
					lane = y + bh + gy
				}
				pts = append(pts, [2]float64{px + bw + gx, ay}, [2]float64{px + bw + gx, lane}, [2]float64{x - gx, lane}, [2]float64{x - gx, by})
			}
			pts = append(pts, [2]float64{x, by})
			for k := 1; k < len(pts); k++ {
				gfx.StrokeLine(dst, float32(pts[k-1][0]), float32(pts[k-1][1]), float32(pts[k][0]), float32(pts[k][1]), 3, c, true)
			}
		}
	}
	for i, n := range nodes {
		x, y := pos(n.Col, n.Row)
		lv := a.Tree[n.ID]
		reached := a.NodeReached(n.ID)
		can := a.NodeOpen(n.ID)
		bg := color.NRGBA{0x1e, 0x26, 0x3a, 0xf4}
		border := color.NRGBA{0x5a, 0x68, 0x88, 0xff}
		tc := g.th.text
		switch {
		case lv >= n.Max:
			bg, border = color.NRGBA{0x3a, 0x34, 0x20, 0xf4}, color.NRGBA{0xe0, 0xc0, 0x60, 0xff}
		case can:
			bg, border = color.NRGBA{0x1e, 0x40, 0x3a, 0xf4}, treeCanColor
		case lv > 0:
			bg = color.NRGBA{0x26, 0x40, 0x68, 0xf4}
		case !reached:
			bg, border, tc = color.NRGBA{0x14, 0x16, 0x1e, 0xf4}, color.NRGBA{0x38, 0x3c, 0x4a, 0xff}, g.th.dim
		}
		gfx.FillRoundRect(dst, float32(x+3), float32(y+4), bw, bh, 12, color.NRGBA{0, 0, 0, 0x70})
		gfx.FillRoundRect(dst, float32(x), float32(y), bw, bh, 12, bg)
		w := float32(2)
		if i == t.sel {
			border, w = g.th.cursor, 4
		}
		gfx.StrokeRoundRect(dst, float32(x), float32(y), bw, bh, 12, w, border)
		g.text(dst, t.nodeName(g, n.ID), x+14, y+8, sizeS, tc)
		state := fmt.Sprintf("Lv %d / %d", lv, n.Max)
		switch {
		case lv >= n.Max:
			state = "MAX"
		case !reached:
			state = lang.T("未解放")
		}
		g.text(dst, state, x+14, y+36, sizeS*0.8, tc)
		if n.Skill == "" {
			gfx.DrawTextRight(dst, lang.T("パッシブ"), x+bw-14, y+36, sizeS*0.75, color.NRGBA{0xc0, 0xa0, 0xff, 0xff})
		}
		if can {
			gfx.FillRoundRect(dst, float32(x+bw-34), float32(y+8), 24, 24, 12, treeCanColor)
			gfx.DrawText(dst, "+", x+bw-22, y+6, sizeS*0.9, colorBlack)
		}

		bar(dst, x+14, y+bh-12, bw-28, 5, float64(lv)/float64(max(1, n.Max)), border)
	}

	cur := nodes[t.sel]
	desc := cur.Desc
	if desc == "" {
		if s := g.db.Skills[cur.Skill]; s != nil {
			desc = s.Desc
		}
	}
	if s := g.db.Skills[cur.Skill]; s != nil && s.MP > 0 {
		desc += fmt.Sprintf(lang.T("（MP %d）"), s.MP)
	}
	var note string
	nc := treeCanColor
	switch {
	case a.Tree[cur.ID] >= cur.Max:
		note, nc = lang.T("これ以上 あげられない"), color.NRGBA{0xe0, 0xc0, 0x60, 0xff}
	case !a.NodeReached(cur.ID):
		note, nc = lang.T("必要")+":", treeNoSPColor
		for _, r := range cur.Requires {
			note += fmt.Sprintf(lang.T(" %s Lv%d（いま Lv%d）"), t.nodeName(g, r.Node), r.Level, a.Tree[r.Node])
		}
	case a.SP <= 0:
		note, nc = fmt.Sprintf(lang.T("SP が たりない（レベルが 上がると +%d）"), perLv), treeNoSPColor
	default:
		note = lang.T("Z で あげる（SP 1）")
	}
	t.drawPassives(g, dst, a, sw, sh)
	g.window(dst, 40, sh-186, sw-80, 116)
	g.text(dst, t.nodeName(g, cur.ID), 64, sh-174, sizeM, g.th.accent)
	gfx.DrawTextRight(dst, note, sw-64, sh-170, sizeS*0.9, nc)
	g.drawLinesFit(dst, desc, 64, sh-136, sw-140, sizeS, 2, g.th.text)
	if t.msgT > 0 {
		g.window(dst, sw/2-320, sh/2-40, 640, 80)
		gfx.DrawText(dst, t.msg, sw/2, sh/2-14, sizeM, g.th.text)
	}
}

func (t *treeView) drawPassives(g *Game, dst *ebiten.Image, a *rpg.Actor, sw, sh float64) {
	var ps []gamedata.Passive
	for _, p := range g.db.Game.Battle.Passives {
		if p.Actor == a.ID && g.db.States[p.State] != nil {
			ps = append(ps, p)
		}
	}
	if len(ps) == 0 {
		return
	}
	const y0, h = 436.0, 84.0
	y := min(y0, sh-186-12-h*float64(len(ps)))
	env := g.st.Env()
	for _, p := range ps {
		st := g.db.States[p.State]
		open := true
		if p.When.Set() {
			ok, err := p.When.Bool(env)
			open = err == nil && ok
		}
		g.window(dst, 40, y, sw-80, h-8)
		g.text(dst, lang.T("隠しスキル"), 64, y+10, sizeS*0.8, color.NRGBA{0xc0, 0xa0, 0xff, 0xff})
		if !open {
			g.text(dst, lang.T("？？？"), 64, y+34, sizeM, g.th.dim)
			g.text(dst, lang.T("まだ 目覚めて いない"), 260, y+40, sizeS*0.85, g.th.dim)
		} else {
			g.text(dst, st.Name, 64, y+34, sizeM, g.th.accent)
			if p.Level.Set() {
				if n, err := p.Level.Number(env); err == nil {
					gfx.DrawTextRight(dst, fmt.Sprintf(lang.T("段 %d"), int(n)), sw-64, y+12, sizeS*0.9, g.th.accent)
				}
			}
			g.drawLinesFit(dst, st.Desc, 260, y+38, sw-340, sizeS*0.85, 1, g.th.text)
		}
		y += h
	}
}

var (
	treeCanColor  = color.NRGBA{0x7c, 0xe0, 0xa8, 0xff}
	treeNoSPColor = color.NRGBA{0xff, 0xa0, 0x70, 0xff}
)

type fusion struct {
	list *list
	idx  []int
	msg  string
	msgT int
	done func()
}

func newFusion(g *Game, done func()) *fusion {
	f := &fusion{done: done}
	f.refresh(g)
	return f
}

func (f *fusion) refresh(g *Game) {
	var items []listItem
	f.idx = nil
	cur := g.db.Game.Terms.Currency
	for i, fu := range g.db.Fusions {
		have := true
		need := map[string]int{}
		for _, s := range fu.From {
			need[s]++
		}
		label := ""
		for j, s := range fu.From {
			if j > 0 {
				label += " + "
			}
			label += g.db.Items[s].Name
		}
		for s, n := range need {
			if g.st.Items[s] < n {
				have = false
			}
		}
		items = append(items, listItem{
			label: label + " → " + g.db.Items[fu.To].Name, right: fmt.Sprintf("%d%s", fu.Price, cur),
			disabled: !have || g.st.Gold < fu.Price,
		})
		f.idx = append(f.idx, i)
	}
	items = append(items, listItem{label: lang.T("やめる")})
	sel := 0
	if f.list != nil {
		sel = f.list.sel
	}
	_, sh := g.screen()
	f.list = newList(items, 36, 110, 760, int((sh-300)/44))
	f.list.sel = min(sel, len(items)-1)
	f.list.scroll()
}

func (f *fusion) overlay() bool { return true }

func (f *fusion) update(g *Game) {
	if f.msgT > 0 {
		f.msgT--
	}
	chose, cancel := f.list.update(g)
	if cancel || (chose && f.list.sel == len(f.idx)) {
		g.sfx("cancel")
		g.pop()
		f.done()
		return
	}
	if !chose {
		if g.in.ok() && f.list.sel < len(f.idx) && f.list.items[f.list.sel].disabled {
			f.msg, f.msgT = lang.T("ざいりょうか おかねが たりない。"), 90
		}
		return
	}
	fu := g.db.Fusions[f.idx[f.list.sel]]
	for _, s := range fu.From {
		g.st.AddItem(s, -1)
	}
	g.st.Gold -= fu.Price
	g.st.AddItem(fu.To, 1)
	g.sfx("decide")
	f.msg, f.msgT = g.db.Items[fu.To].Name+lang.T("が できた！"), 90
	f.refresh(g)
}

func (f *fusion) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, color.NRGBA{0, 0, 0, 0x90})
	g.window(dst, 24, 30, 790, sh-60)
	g.text(dst, fmt.Sprintf(lang.T("ごうせい　　もちきん %d%s"), g.st.Gold, g.db.Game.Terms.Currency), 50, 52, sizeM, g.th.accent)
	f.list.draw(g, dst, true)
	if f.list.sel < len(f.idx) {
		fu := g.db.Fusions[f.idx[f.list.sel]]
		it := g.db.Items[fu.To]
		g.window(dst, 840, 110, sw-870, 260)
		g.text(dst, it.Name, 864, 126, sizeM, g.th.text)

		y := 170 + g.drawLines(dst, it.Desc, 864, 170, sw-920, sizeS, g.th.dim) + 8
		need := map[string]int{}
		for _, s := range fu.From {
			need[s]++
		}
		for _, s := range fu.From {
			if need[s] == 0 {
				continue
			}
			g.text(dst, fmt.Sprintf(lang.T("%s　必要 %d / 所持 %d"), g.db.Items[s].Name, need[s], g.st.Items[s]), 864, y, sizeS, g.th.text)
			need[s] = 0
			y += 34
		}
	}
	if f.msgT > 0 {
		g.window(dst, sw/2-300, sh/2-40, 600, 80)
		gfx.DrawText(dst, f.msg, sw/2, sh/2-14, sizeM, g.th.text)
	}
}

func treeNodeRect(col, row int) image.Rectangle {
	const x0, y0, cw, rh, bw, bh = 48.0, 140.0, 300.0, 104.0, 230.0, 72.0
	x, y := x0+float64(col)*cw, y0+float64(row)*rh
	return image.Rect(int(x), int(y), int(x+bw), int(y+bh))
}
