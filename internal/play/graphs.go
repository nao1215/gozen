package play

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/headless/balance"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/text"
)

// 開発用のバランス表示（--dev の F7）。今いる場所で出会う敵の群れごとに、gozen sim と同じ計算
// （balance.SavedStart・Try・Add・StreakOne、-n 200 -seed 1 -turns 60）で、全回復した一行と
// 今の HP・MP・状態のままの一行（-as-is）の勝率、勝った後の残り HP の帯、今の HP から回復せずに
// 何連勝できるか（-as-is -streak 9）、Lv ごとの勝率（全回復）を描く。計算は Update の中で 1 フレーム
// graphBudget までに分け、ゲームの状態・乱数・DB を別の goroutine と共有しない。
const (
	graphTries  = 200
	graphSeed   = 1
	graphTurns  = 60
	graphLevels = 5 // 今の Lv の上に何 Lv 分の勝率を測るか
	graphStreak = 9 // 連勝の数の上限（gozen sim -streak）
	graphRows   = 6
	graphBudget = 3 * time.Millisecond
	graphCheck  = 15 // 場所・一行・データの変化を確かめる間隔（tick）
	// graphSettle は HP・MP・状態が変わってから、今の HP の勝率を測り直すまでの tick。
	// 回復や連戦で続けて変わる間は待ち、落ち着いてから 1 回だけ測る。
	graphSettle = 45
)

var graphColors = []color.NRGBA{
	{0x6c, 0xc4, 0xf0, 0xff},
	{0xf2, 0xb0, 0x5a, 0xff},
	{0x8e, 0xd0, 0x7c, 0xff},
	{0xf0, 0x7c, 0x9c, 0xff},
	{0xc4, 0x9c, 0xf0, 0xff},
	{0xe8, 0xe0, 0x70, 0xff},
}

type graphKind int

const (
	graphHealed graphKind = iota // 全回復（gozen sim -state）
	graphNow                     // 今の HP・MP・状態のまま（-as-is）
	graphRun                     // 今の HP から回復せずに続けて戦う（-as-is -streak）
)

type graphJob struct {
	troop string
	level int // 0 は今の一行のまま、それ以外はこの Lv に揃える（gozen sim -level）
	kind  graphKind
}

type graphResult struct {
	odds balance.Odds
	wins []int // graphRun の始まりごとの連勝の数
}

func (r *graphResult) tries() int { return r.odds.Battles + len(r.wins) }

type graphPanel struct {
	on      bool
	key     string // データ・場所・一行の Lv や装備（変われば全部を測り直す）
	db      *data.DB
	save    []byte // 全回復の勝率の始まり（key が変わった時の状態）
	now     []byte // 今の HP の勝率・連勝の始まり
	nowKey  string // HP・MP・状態・経験値（変われば落ち着いてから今の HP の分だけ測り直す）
	pendKey string
	pendT   int
	hurt    bool    // 誰かの HP・MP が減っているか状態異常がある
	hpRate  float64 // 一行の HP の割合（表示用）
	base    int     // 一行の今の Lv（いちばん高い人）
	here    []balance.Encounter
	fight   string // 戦闘中の群れ
	res     map[graphJob]*graphResult
	err     string
}

// wanted は測る物を、先に見せたい順に並べる（全回復の棒、今の HP の棒、連勝、Lv の線）。
func (p *graphPanel) wanted() []graphJob {
	rows := p.here[:min(len(p.here), graphRows)]
	var out []graphJob
	for _, e := range rows {
		out = append(out, graphJob{troop: e.Troop})
	}
	if p.hurt {
		for _, e := range rows {
			out = append(out, graphJob{troop: e.Troop, kind: graphNow})
		}
	}
	for _, e := range rows {
		out = append(out, graphJob{troop: e.Troop, kind: graphRun})
	}
	for _, e := range rows {
		for l := p.base + 1; l <= p.base+graphLevels; l++ {
			out = append(out, graphJob{troop: e.Troop, level: l})
		}
	}
	return out
}

// next は次に測る物。全部測り終えていれば ok は false。
func (p *graphPanel) next() (graphJob, bool) {
	for _, j := range p.wanted() {
		if r := p.res[j]; r == nil || r.tries() < graphTries {
			return j, true
		}
	}
	return graphJob{}, false
}

func (p *graphPanel) done() bool {
	_, ok := p.next()
	return !ok
}

// progress はここまでに測った始まりの数と全体の数。
func (p *graphPanel) progress() (n, total int) {
	for _, j := range p.wanted() {
		total += graphTries
		if r := p.res[j]; r != nil {
			n += min(graphTries, r.tries())
		}
	}
	return n, total
}

func (g *Game) updateGraphs() {
	p := &g.graphs
	if g.in.pressed[btnGraphs] {
		p.on = !p.on
	}
	if !p.on || g.fast != nil || g.st == nil || g.db == nil {
		return
	}
	if g.tick%graphCheck == 0 || p.key == "" {
		g.graphsCheck()
	}
	if p.err != "" {
		return
	}
	t0 := time.Now()
	for time.Since(t0) < graphBudget {
		j, ok := p.next()
		if !ok {
			return
		}
		r := p.res[j]
		if r == nil {
			r = &graphResult{odds: balance.Odds{Troop: j.troop}}
			p.res[j] = r
		}
		save, prep := p.save, balance.Prep{Level: j.level, Tree: true}
		if j.kind != graphHealed {
			save, prep.AsIs = p.now, true
		}
		st, err := balance.SavedStart(p.db, save, graphSeed, prep)(r.tries())
		if err != nil {
			p.err = err.Error()
			return
		}
		if j.kind == graphRun {
			r.wins = append(r.wins, balance.StreakOne(p.db, st, j.troop, graphTurns, graphStreak))
		} else {
			r.odds.Add(balance.Try(p.db, st, j.troop, graphTurns, nil))
		}
	}
}

// graphsCheck は場所・一行・データ（ホットリロードで替わる）が変わったら全部を測り直す。
// HP・MP・状態・経験値だけが変わったら、graphSettle の間変わらなくなってから（戦闘中は待つ）
// 今の HP の勝率と連勝だけを測り直す。戦闘の群れが増えただけなら、それまでの結果に足す。
func (g *Game) graphsCheck() {
	p := &g.graphs
	fight := ""
	if b := g.currentBattle(); b != nil {
		fight = b.troop
	}
	here := balance.Here(g.db, g.st)
	var party, cond []string
	base, hp, mhp, hurt := 0, 0, 0, false
	for _, a := range g.st.Members() {
		party = append(party, fmt.Sprintf("%s:%d:%v:%v:%d", a.ID, a.Level, a.Equip, a.Tree, a.SP))
		cond = append(cond, fmt.Sprintf("%s:%d:%d:%v:%d", a.ID, a.HP, a.MP, a.States, a.Exp))
		base = max(base, a.Level)
		hp, mhp = hp+max(0, a.HP), mhp+a.Stat("mhp")
		if a.HP < a.Stat("mhp") || a.MP < a.Stat("mmp") || len(a.States) > 0 {
			hurt = true
		}
	}
	var troops []string
	for _, e := range here {
		troops = append(troops, fmt.Sprintf("%s:%.3f", e.Troop, e.Share))
	}
	key := fmt.Sprintf("%p|%s|%s|%s", g.db, g.st.Location, strings.Join(party, ","), strings.Join(troops, ","))
	nowKey := strings.Join(cond, ",")
	setNow := func(save []byte) {
		p.now, p.nowKey, p.hurt = save, nowKey, hurt
		p.hpRate = float64(hp) / float64(max(1, mhp))
		p.pendKey = ""
		for j := range p.res {
			if j.kind != graphHealed {
				delete(p.res, j)
			}
		}
	}
	if key != p.key {
		save, err := g.st.Save()
		*p = graphPanel{on: true, key: key, db: g.db, save: save, base: base, here: here, res: map[graphJob]*graphResult{}}
		if err != nil {
			p.err = err.Error()
			return
		}
		setNow(save)
	} else if nowKey != p.nowKey {
		switch {
		case nowKey != p.pendKey:
			p.pendKey, p.pendT = nowKey, g.tick
		case fight == "" && g.tick-p.pendT >= graphSettle:
			save, err := g.st.Save()
			if err != nil {
				p.err = err.Error()
				return
			}
			setNow(save)
		}
	}
	if fight != p.fight {
		p.fight = fight
		if fight != "" && !p.has(fight) && g.db.Troops[fight] != nil {
			p.here = append([]balance.Encounter{{Troop: fight}}, p.here...)
		}
	}
}

func (p *graphPanel) has(troop string) bool {
	for _, e := range p.here {
		if e.Troop == troop {
			return true
		}
	}
	return false
}

// troopLabel は群れの名前（無ければ敵の名前と数）。
func troopLabel(db *data.DB, id string) string {
	t := db.Troops[id]
	if t == nil {
		return id
	}
	if t.Name != "" {
		return t.Name
	}
	var order []string
	count := map[string]int{}
	for _, m := range t.Members {
		n := m
		if e := db.Enemies[m]; e != nil && e.Name != "" {
			n = e.Name
		}
		if count[n] == 0 {
			order = append(order, n)
		}
		count[n]++
	}
	var parts []string
	for _, n := range order {
		if count[n] > 1 {
			n = fmt.Sprintf("%s×%d", n, count[n])
		}
		parts = append(parts, n)
	}
	return strings.Join(parts, " ")
}

func (g *Game) drawGraphs(screen *ebiten.Image) {
	p := &g.graphs
	if !p.on || !g.opts.Development {
		return
	}
	sw, sh := g.screen()
	const (
		w     = 530.0
		pad   = 14.0
		small = 14.0
		body  = 16.0
	)
	rowH := 32.0
	if p.hurt {
		rowH = 42
	}
	x, y := sw-w-12, 12.0
	rows := min(len(p.here), graphRows)
	chartH := 0.0
	if rows > 0 {
		chartH = 170
	}
	h := 88 + float64(max(1, rows))*rowH + chartH + 40
	h = min(h, sh-24)
	gfx.FillRoundRect(screen, float32(x), float32(y), float32(w), float32(h), 10, color.NRGBA{0x0c, 0x12, 0x18, 0xfa})
	gfx.StrokeRoundRect(screen, float32(x), float32(y), float32(w), float32(h), 10, 1.5, withAlpha(g.th.border, 0xa0))
	ink, dim := g.th.text, withAlpha(g.th.text, 0xa8)
	left := x + pad
	head := lang.T("バランス")
	if g.st != nil {
		if loc := g.db.Locations[g.st.Location]; loc != nil && loc.Name != "" {
			head += "  " + loc.Name
		}
	}
	gfx.DrawTextLeft(screen, gfx.Ellipsis(head, sizeS*0.85, w-2*pad-60), left, y+8, sizeS*0.85, ink)
	gfx.DrawTextRight(screen, "F7", x+w-pad, y+10, small, dim)
	sub := fmt.Sprintf(lang.T("%d戦・seed %d（gozen sim -state と同じ。今の HP は -as-is）"), graphTries, graphSeed)
	gfx.DrawTextLeft(screen, gfx.Ellipsis(sub, small, w-2*pad), left, y+36, small, dim)
	legend := lang.T("今は全快。棒は全回復の勝率")
	if p.hurt {
		legend = fmt.Sprintf(lang.T("上の段 全回復・下の段 今の HP（一行の HP %.0f%%）"), 100*p.hpRate)
	}
	gfx.DrawTextLeft(screen, gfx.Ellipsis(legend, small, w-2*pad), left, y+56, small, dim)
	top := y + 84
	switch {
	case g.st == nil:
		gfx.DrawTextLeft(screen, lang.T("ゲームを始めると測る"), left, top+4, body, ink)
		return
	case p.err != "":
		gfx.DrawTextLeft(screen, gfx.Ellipsis(lang.T("測れない: ")+p.err, body, w-2*pad), left, top+4, body, ink)
		return
	case rows == 0:
		gfx.DrawTextLeft(screen, lang.T("ここで出会う敵はいない"), left, top+4, body, ink)
		return
	}
	// 列: 名前 | 勝率の棒 | 勝った後の残り HP の帯 | 平均ターン | 今の HP からの連勝
	nameW, barX, barW, hpX, hpW := 152.0, left+154, 112.0, left+280, 96.0
	turnR, runR := hpX+hpW+54, x+w-pad
	gfx.DrawTextLeft(screen, lang.T("勝率"), barX, top-2, small, dim)
	gfx.DrawTextLeft(screen, lang.T("勝った後の HP"), hpX, top-2, small, dim)
	gfx.DrawTextRight(screen, lang.T("ターン"), turnR, top-2, small, dim)
	gfx.DrawTextRight(screen, lang.T("連勝"), runR, top-2, small, dim)
	top += 20
	// tier は 1 段の棒と帯。段が 2 つなら細く、文字を小さくする。
	tier := func(r *graphResult, ty, th, size float64, clr color.NRGBA) {
		gfx.FillRect(screen, float32(barX), float32(ty), float32(barW), float32(th), color.NRGBA{0xff, 0xff, 0xff, 0x22}, false)
		gfx.FillRect(screen, float32(hpX), float32(ty+th/2-1), float32(hpW), 2, color.NRGBA{0xff, 0xff, 0xff, 0x40}, false)
		ly := ty + (th-size*1.3)/2
		if r == nil || r.odds.Battles == 0 {
			gfx.DrawTextLeft(screen, "…", barX+4, ly, size, dim)
			return
		}
		o := r.odds
		gfx.FillRect(screen, float32(barX), float32(ty), float32(barW*o.WinRate()), float32(th), clr, false)
		label := fmt.Sprintf("%.0f%%", 100*o.WinRate())
		if o.Battles < graphTries {
			label = fmt.Sprintf("%d/%d", o.Battles, graphTries)
		}
		gfx.DrawTextRight(screen, label, barX+barW-1, ly+1, size, color.NRGBA{0, 0, 0, 0xc0})
		gfx.DrawTextRight(screen, label, barX+barW-2, ly, size, ink)
		if lo, mid, hi, ok := o.HPBand(); ok {
			gfx.FillRect(screen, float32(hpX+hpW*lo), float32(ty), float32(max(2, hpW*(hi-lo))), float32(th), withAlpha(clr, 0x90), false)
			gfx.FillRect(screen, float32(hpX+hpW*mid-1), float32(ty-2), 3, float32(th+4), ink, false)
		}
	}
	for i := range rows {
		e := p.here[i]
		ry := top + float64(i)*rowH
		clr := graphColors[i%len(graphColors)]
		if e.Troop == p.fight {
			gfx.FillRect(screen, float32(x+4), float32(ry-3), float32(w-8), float32(rowH-2), withAlpha(g.th.accent, 0x38), false)
		}
		mid := ry + (rowH-6)/2
		gfx.FillCircle(screen, float32(left+5), float32(mid), 5, clr, true)
		name := troopLabel(g.db, e.Troop)
		if e.Share > 0 {
			name = fmt.Sprintf("%s %.0f%%", name, 100*e.Share)
		}
		gfx.DrawTextLeft(screen, gfx.Ellipsis(name, body, nameW-14), left+14, mid-11, body, ink)
		healed := p.res[graphJob{troop: e.Troop}]
		if p.hurt {
			tier(healed, ry+1, 16, 13, clr)
			tier(p.res[graphJob{troop: e.Troop, kind: graphNow}], ry+20, 16, 13, shade(clr))
		} else {
			tier(healed, ry+4, 18, body, clr)
		}
		if healed != nil && healed.odds.Battles >= graphTries {
			gfx.DrawTextRight(screen, fmt.Sprintf("%.1f", healed.odds.AvgTurns()), turnR, mid-11, body, ink)
		}
		if r := p.res[graphJob{troop: e.Troop, kind: graphRun}]; r != nil && len(r.wins) >= graphTries {
			run := fmt.Sprintf("%d", balance.Percentile(r.wins, 0.5))
			if balance.Percentile(r.wins, 0.5) >= graphStreak {
				run = fmt.Sprintf("%d+", graphStreak)
			}
			gfx.DrawTextRight(screen, run, runR, mid-11, body, ink)
		}
	}
	// Lv と勝率の線（全回復）
	cy := top + float64(rows)*rowH + 8
	gfx.DrawTextLeft(screen, lang.T("Lv ごとの勝率（全回復）"), left, cy, small, dim)
	plotX, plotY, plotW, plotH := left+44, cy+24, w-2*pad-60, chartH-60
	for _, v := range []float64{0, 0.5, 1} {
		yy := plotY + plotH*(1-v)
		gfx.StrokeLine(screen, float32(plotX), float32(yy), float32(plotX+plotW), float32(yy), 1, color.NRGBA{0xff, 0xff, 0xff, 0x30}, false)
		gfx.DrawTextRight(screen, fmt.Sprintf("%.0f%%", 100*v), plotX-6, yy-9, small, dim)
	}
	px := func(k int) float64 { return plotX + plotW*float64(k)/float64(graphLevels) }
	for k := 0; k <= graphLevels; k++ {
		gfx.DrawText(screen, fmt.Sprintf("Lv%d", p.base+k), px(k), plotY+plotH+4, small, dim)
	}
	for i := range rows {
		e := p.here[i]
		clr := graphColors[i%len(graphColors)]
		var last [2]float64
		has := false
		for k := 0; k <= graphLevels; k++ {
			j := graphJob{troop: e.Troop}
			if k > 0 {
				j.level = p.base + k
			}
			r := p.res[j]
			if r == nil || r.odds.Battles < graphTries {
				break
			}
			pt := [2]float64{px(k), plotY + plotH*(1-r.odds.WinRate())}
			// 先の行ほど太く描き、重なった線も縁として見えるようにする。
			lw := 2.5 + 2.5*float64(rows-1-i)
			if has {
				gfx.StrokeLine(screen, float32(last[0]), float32(last[1]), float32(pt[0]), float32(pt[1]), float32(lw), clr, true)
			}
			gfx.FillCircle(screen, float32(pt[0]), float32(pt[1]), float32(lw/2+2), clr, true)
			last, has = pt, true
		}
	}
	n, total := p.progress()
	foot := fmt.Sprintf(lang.T("測定 %d/%d"), n, total)
	if n >= total {
		foot = lang.T("測り終えた。連勝は今の HP から回復せずに勝ち続けた数の中央値（-as-is -streak 9）")
	}
	gfx.DrawTextLeft(screen, gfx.Ellipsis(foot, small, w-2*pad), left, y+h-22, small, dim)
}

// shade は今の HP の段の色（群れの色を暗くして、全回復の段と見分ける）。
func shade(c color.NRGBA) color.NRGBA {
	dark := func(v uint8) uint8 { return v - v/5*2 }
	return color.NRGBA{dark(c.R), dark(c.G), dark(c.B), c.A}
}
