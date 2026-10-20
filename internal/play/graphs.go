package play

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/headless/balance"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// Developer balance display (F7 under --dev). For each enemy troop met at the current location, it computes
// the same way as gozen sim battle (balance.SavedStart, Try, Add, StreakOne; -n 200 --seed 1 --turns 60) and draws
// the win rate for a fully healed party row and for the party row as-is (--as-is), the band of remaining HP
// after a win, how many fights in a row can be won from the current HP without healing (--as-is --streak 9),
// and the win rate per level (fully healed). The computation is split inside Update to at most graphBudget per
// frame, and game state, randomness and the DB are not shared with other goroutines.
const (
	graphTries  = 200
	graphSeed   = 1
	graphTurns  = 60
	graphLevels = 5 // how many levels above the current level to measure the win rate for
	graphStreak = 9 // upper limit of the win streak count (gozen sim battle --streak)
	graphRows   = 6
	graphBudget = 3 * time.Millisecond
	graphCheck  = 15 // interval (ticks) for checking changes of location, party row and data
	// graphSettle is the number of ticks after HP, MP or states change before the current-HP win rate is re-measured.
	// While they keep changing through healing or consecutive fights it waits, then measures once after things settle.
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
	graphHealed graphKind = iota // fully healed (gozen sim battle --state)
	graphNow                     // current HP, MP and states as-is (--as-is)
	graphRun                     // keep fighting from the current HP without healing (--as-is --streak)
)

type graphJob struct {
	troop string
	level int // 0 keeps the current party row; otherwise align to this level (gozen sim battle --level)
	kind  graphKind
}

type graphResult struct {
	odds balance.Odds
	wins []int // win streak count for each start of graphRun
}

func (r *graphResult) tries() int { return r.odds.Battles + len(r.wins) }

type graphPanel struct {
	on      bool
	key     string // data, location, and the party row's level and equipment (re-measure everything if it changes)
	db      *data.DB
	save    []byte // start of the fully healed win rate (state when key changed)
	now     []byte // start of the current-HP win rate and streak
	nowKey  string // HP, MP, states and experience (if it changes, re-measure only the current-HP part after settling)
	pendKey string
	pendT   int
	hurt    bool    // someone's HP or MP is reduced or has a status ailment
	hpRate  float64 // HP ratio of the party row (for display)
	base    int     // current level of the party row (the highest member)
	here    []balance.Encounter
	fight   string // troop in battle
	res     map[graphJob]*graphResult
	err     string
}

// wanted orders what to measure by what to show first (fully healed bar, current-HP bar, streak, level line).
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

// next is the next thing to measure; ok is false if everything has been measured.
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

// progress is the number of starts measured so far and the total.
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

// graphsCheck re-measures everything when the location, party row or data (replaced by hot reload) changes.
// If only HP, MP, states or experience change, it re-measures only the current-HP win rate and streak after
// they stay unchanged for graphSettle (waiting during battle). If a troop merely increased, it adds to the earlier results.
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

// troopLabel is the troop's name (or the enemy names and counts if it has none).
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
	sub := fmt.Sprintf(lang.T("%d戦・seed %d（gozen sim battle --state と同じ。今の HP は --as-is）"), graphTries, graphSeed)
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
	// Columns: name | win rate bar | band of HP remaining after a win | average turns | streak from the current HP
	nameW, barX, barW, hpX, hpW := 152.0, left+154, 112.0, left+280, 96.0
	turnR, runR := hpX+hpW+54, x+w-pad
	gfx.DrawTextLeft(screen, lang.T("勝率"), barX, top-2, small, dim)
	gfx.DrawTextLeft(screen, lang.T("勝った後の HP"), hpX, top-2, small, dim)
	gfx.DrawTextRight(screen, lang.T("ターン"), turnR, top-2, small, dim)
	gfx.DrawTextRight(screen, lang.T("連勝"), runR, top-2, small, dim)
	top += 20
	// tier is one tier of bar and band. With two tiers they are thinner and the text is smaller.
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
			run := strconv.Itoa(balance.Percentile(r.wins, 0.5))
			if balance.Percentile(r.wins, 0.5) >= graphStreak {
				run = fmt.Sprintf("%d+", graphStreak)
			}
			gfx.DrawTextRight(screen, run, runR, mid-11, body, ink)
		}
	}
	// Line of level vs. win rate (fully healed)
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
			// Draw later rows thicker so that overlapping lines still show as outlines.
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
		foot = lang.T("測り終えた。連勝は今の HP から回復せずに勝ち続けた数の中央値（--as-is --streak 9）")
	}
	gfx.DrawTextLeft(screen, gfx.Ellipsis(foot, small, w-2*pad), left, y+h-22, small, dim)
}

// shade is the color of the current-HP tier (the troop's color darkened to tell it apart from the fully healed tier).
func shade(c color.NRGBA) color.NRGBA {
	dark := func(v uint8) uint8 { return v - v/5*2 }
	return color.NRGBA{dark(c.R), dark(c.G), dark(c.B), c.A}
}
