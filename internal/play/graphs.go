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

// 開発用のバランス表示（--dev の F7）。今いる場所で出会う敵の群れごとに、今の一行（全回復）で
// gozen sim と同じ計算（balance.SavedStart・Try・Add、-n 200 -seed 1 -turns 60）をして、
// 勝率・勝った後の残り HP の帯・Lv ごとの勝率を描く。計算は Update の中で 1 フレーム
// graphBudget までに分け、ゲームの状態・乱数・DB を別の goroutine と共有しない。
const (
	graphTries  = 200
	graphSeed   = 1
	graphTurns  = 60
	graphLevels = 5 // 今の Lv の上に何 Lv 分の勝率を測るか
	graphRows   = 6
	graphBudget = 3 * time.Millisecond
	graphCheck  = 15 // 場所・一行・データの変化を確かめる間隔（tick）
)

var graphColors = []color.NRGBA{
	{0x6c, 0xc4, 0xf0, 0xff},
	{0xf2, 0xb0, 0x5a, 0xff},
	{0x8e, 0xd0, 0x7c, 0xff},
	{0xf0, 0x7c, 0x9c, 0xff},
	{0xc4, 0x9c, 0xf0, 0xff},
	{0xe8, 0xe0, 0x70, 0xff},
}

type graphJob struct {
	troop string
	level int // 0 は今の一行のまま、それ以外はこの Lv に揃える（gozen sim -level）
}

type graphPanel struct {
	on    bool
	key   string
	db    *data.DB
	save  []byte
	base  int // 一行の今の Lv（いちばん高い人）
	here  []balance.Encounter
	fight string // 戦闘中の群れ
	jobs  []graphJob
	res   map[graphJob]*balance.Odds
	job   int
	err   string
	n     int // ここまでに戦った回数（表示の進み具合）
}

func (p *graphPanel) total() int { return len(p.jobs) * graphTries }

func (p *graphPanel) done() bool { return p.job >= len(p.jobs) }

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
	if p.err != "" || p.done() {
		return
	}
	t0 := time.Now()
	for !p.done() && time.Since(t0) < graphBudget {
		j := p.jobs[p.job]
		o := p.res[j]
		if o == nil {
			o = &balance.Odds{Troop: j.troop}
			p.res[j] = o
		}
		st, err := balance.SavedStart(p.db, p.save, graphSeed, balance.Prep{Level: j.level, Tree: true})(o.Battles)
		if err != nil {
			p.err = err.Error()
			return
		}
		o.Add(balance.Try(p.db, st, j.troop, graphTurns, nil))
		p.n++
		if o.Battles >= graphTries {
			p.job++
		}
	}
}

// graphsCheck は場所・一行・データ（ホットリロードで替わる）が変わったら測り直す。
// 戦闘の群れが増えただけなら、それまでの結果を残して足す。
func (g *Game) graphsCheck() {
	p := &g.graphs
	fight := ""
	if b := g.currentBattle(); b != nil {
		fight = b.troop
	}
	here := balance.Here(g.db, g.st)
	var party []string
	base := 0
	for _, a := range g.st.Members() {
		party = append(party, fmt.Sprintf("%s:%d:%d:%v:%v:%d", a.ID, a.Level, a.Exp, a.Equip, a.Tree, a.SP))
		base = max(base, a.Level)
	}
	var troops []string
	for _, e := range here {
		troops = append(troops, fmt.Sprintf("%s:%.3f", e.Troop, e.Share))
	}
	key := fmt.Sprintf("%p|%s|%s|%s", g.db, g.st.Location, strings.Join(party, ","), strings.Join(troops, ","))
	if key != p.key {
		save, err := g.st.Save()
		*p = graphPanel{on: true, key: key, db: g.db, save: save, base: base, here: here, res: map[graphJob]*balance.Odds{}}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.addJobs(here)
	}
	if fight != p.fight {
		p.fight = fight
		if fight != "" && !p.has(fight) && g.db.Troops[fight] != nil {
			e := balance.Encounter{Troop: fight}
			p.here = append([]balance.Encounter{e}, p.here...)
			p.addJobs([]balance.Encounter{e})
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

// addJobs は今の一行のままの勝率を先に、Lv を上げた勝率を後に並べる（棒が先に埋まる）。
func (p *graphPanel) addJobs(es []balance.Encounter) {
	if len(es) > graphRows {
		es = es[:graphRows]
	}
	var now, curve []graphJob
	for _, j := range p.jobs[p.job:] {
		if j.level == 0 {
			now = append(now, j)
		} else {
			curve = append(curve, j)
		}
	}
	for _, e := range es {
		now = append(now, graphJob{troop: e.Troop})
		for l := p.base + 1; l <= p.base+graphLevels; l++ {
			curve = append(curve, graphJob{troop: e.Troop, level: l})
		}
	}
	p.jobs = append(append(append([]graphJob{}, p.jobs[:p.job]...), now...), curve...)
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
		w     = 484.0
		pad   = 14.0
		small = 14.0
		body  = 16.0
		rowH  = 32.0
	)
	x, y := sw-w-12, 12.0
	rows := min(len(p.here), graphRows)
	chartH := 0.0
	if rows > 0 {
		chartH = 170
	}
	h := 66 + float64(max(1, rows))*rowH + chartH + 40
	h = min(h, sh-24)
	gfx.FillRoundRect(screen, float32(x), float32(y), float32(w), float32(h), 10, color.NRGBA{0x0c, 0x12, 0x18, 0xf2})
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
	sub := fmt.Sprintf(lang.T("%d戦・seed %d・全回復した今の一行（gozen sim -state と同じ）"), graphTries, graphSeed)
	gfx.DrawTextLeft(screen, gfx.Ellipsis(sub, small, w-2*pad), left, y+38, small, dim)
	top := y + 62
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
	// 列: 名前 | 勝率の棒 | 勝った後の残り HP の帯 | 平均ターン
	nameW, barX, barW, hpX, hpW := 146.0, left+152, 128.0, left+300, 112.0
	gfx.DrawTextLeft(screen, lang.T("勝率"), barX, top-2, small, dim)
	gfx.DrawTextLeft(screen, lang.T("勝った後の HP"), hpX, top-2, small, dim)
	gfx.DrawTextRight(screen, lang.T("ターン"), x+w-pad, top-2, small, dim)
	top += 20
	for i := range rows {
		e := p.here[i]
		ry := top + float64(i)*rowH
		clr := graphColors[i%len(graphColors)]
		if e.Troop == p.fight {
			gfx.FillRect(screen, float32(x+4), float32(ry-3), float32(w-8), float32(rowH-2), withAlpha(g.th.accent, 0x38), false)
		}
		gfx.FillCircle(screen, float32(left+5), float32(ry+12), 5, clr, true)
		name := troopLabel(g.db, e.Troop)
		if e.Share > 0 {
			name = fmt.Sprintf("%s %.0f%%", name, 100*e.Share)
		}
		gfx.DrawTextLeft(screen, gfx.Ellipsis(name, body, nameW-14), left+14, ry+1, body, ink)
		o := p.res[graphJob{troop: e.Troop}]
		// 勝率の棒
		gfx.FillRect(screen, float32(barX), float32(ry+4), float32(barW), 18, color.NRGBA{0xff, 0xff, 0xff, 0x22}, false)
		if o == nil || o.Battles == 0 {
			gfx.DrawTextLeft(screen, "…", barX+4, ry+1, body, dim)
			continue
		}
		rate := o.WinRate()
		gfx.FillRect(screen, float32(barX), float32(ry+4), float32(barW*rate), 18, clr, false)
		label := fmt.Sprintf("%.0f%%", 100*rate)
		if o.Battles < graphTries {
			label = fmt.Sprintf("%d/%d", o.Battles, graphTries)
		}
		gfx.DrawTextRight(screen, label, barX+barW-2, ry+1, body, ink)
		// 残り HP の帯（10%〜90% 点、中央の線）
		gfx.FillRect(screen, float32(hpX), float32(ry+12), float32(hpW), 2, color.NRGBA{0xff, 0xff, 0xff, 0x40}, false)
		if lo, mid, hi, ok := o.HPBand(); ok {
			gfx.FillRect(screen, float32(hpX+hpW*lo), float32(ry+4), float32(max(2, hpW*(hi-lo))), 18, withAlpha(clr, 0x90), false)
			gfx.FillRect(screen, float32(hpX+hpW*mid-1), float32(ry+1), 3, 24, ink, false)
		} else {
			gfx.DrawTextLeft(screen, "-", hpX+hpW/2, ry+1, body, dim)
		}
		gfx.DrawTextRight(screen, fmt.Sprintf("%.1f", o.AvgTurns()), x+w-pad, ry+1, body, ink)
	}
	// Lv と勝率の線
	cy := top + float64(rows)*rowH + 8
	gfx.DrawTextLeft(screen, lang.T("Lv ごとの勝率"), left, cy, small, dim)
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
			o := p.res[j]
			if o == nil || o.Battles < graphTries {
				break
			}
			pt := [2]float64{px(k), plotY + plotH*(1-o.WinRate())}
			// 先の行ほど太く描き、重なった線も縁として見えるようにする。
			lw := 2.5 + 2.5*float64(rows-1-i)
			if has {
				gfx.StrokeLine(screen, float32(last[0]), float32(last[1]), float32(pt[0]), float32(pt[1]), float32(lw), clr, true)
			}
			gfx.FillCircle(screen, float32(pt[0]), float32(pt[1]), float32(lw/2+2), clr, true)
			last, has = pt, true
		}
	}
	foot := fmt.Sprintf(lang.T("測定 %d/%d"), p.n, p.total())
	if p.done() {
		foot = lang.T("測り終えた（F9 のメモの .save を gozen sim -state に渡すと同じ数）")
	}
	gfx.DrawTextLeft(screen, gfx.Ellipsis(foot, small, w-2*pad), left, y+h-22, small, dim)
}
