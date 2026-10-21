package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The clash screen (ADR 0030, fight: clash_view): it plays back a march the rules have
// already decided, as two armies facing each other on the terrain's picture. Each unit is
// a block in a lane: its leader's card at the edge facing the middle (the face, the name,
// a gauge of troops that keeps a red band where it was and shrinks after it, the troops
// now and at the start, the leader's HP) and its soldiers in ranks behind, one for every
// per_soldier troops, under a flag of the side's colour. The first two units of a side
// stand in front, nearer the middle; the rest behind, nearer the edge, and step up as the
// front is routed. A strike lunges the striker's card toward the middle, bursts dust and sparks
// where the armies meet, shakes the card struck red, shows the troops lost beside it and the
// soldiers who went setting off from the striker; the soldiers lost fall from the back of
// the ranks. A routed unit turns gray under a stamp (routed or caught). A rope of the two
// sides' troops runs over the field, the rounds under it. At the end the field darkens and
// a table tells each side's troops sent and left, the loss and its rate, the foes caught
// and the region. OK plays faster (or ends the table); A plays on by itself.

const (
	clashHitTicks  = 44 // a strike, at normal speed
	clashFastTicks = 10
	maxSoldiers    = 24 // the most soldiers in a unit's ranks
	soldierCols    = 6
	soldierRows    = 4
)

type fallen struct {
	x, y, h float64
	age     int
	flip    bool
	pic     string
	side    int
}

type clashUnit struct {
	u           campaign.Unit
	side        int
	shown, lag  float64 // the troops the gauge shows, and its red band
	hp          float64
	x, y        float64 // the block's place now (its card's top-left, UI units)
	shake, glow int
	lunge       int
	gone        float64 // 0 standing, to 1 grayed
	stamp       string  // routed or caught, drawn on the card
	soldiers    int     // the soldiers standing in the ranks
	kind        string  // with armies, the kind of army
}

type clashPop struct {
	text         string
	x, y, tx, ty float64
	age, life    int
	c            color.NRGBA
	size         float64
}

// impact is a burst of dust and sparks where the armies meet. It waits (age below 0) for
// the step forward to land and is gone in 0.3 s.
type impact struct {
	x, y float64
	age  int
}

const (
	impactTicks = 18
	impactDelay = 7
)

type clashScene struct {
	cl      *campaign.Clash
	done    func()
	units   [2][]*clashUnit
	next    int
	wait    int
	round   int
	fast    bool
	auto    bool
	pops    []clashPop
	impacts []impact
	falls   []fallen
	over    bool
	age     int
	per     float64
	pics    [2]string // the soldiers' pictures
	bands   []skillBand
	cut     *clashCut
	// skillShown is the strike (counted from 1) whose skill was last shown.
	skillShown int
	// army is set when clash.armies are written: each unit is shown as a picture of an
	// army that thins as its troops fall, not as soldiers one by one.
	army   bool
	armies map[string][]string
	red    float32 // how red the soldiers being drawn are (a unit struck)
	layout string  // front or below
}

// clashCut is a skill's cut-in: the user's picture across the screen with its name.
type clashCut struct {
	u    *clashUnit
	name string
	pic  string
	t    int
}

const clashCutTicks = 56

// skillBand is a skill's name shown across its unit's card.
type skillBand struct {
	text string
	side int
	at   int
	age  int
}

func newClashScene(g *Game, cl *campaign.Clash, done func()) *clashScene {
	c := g.db.Game.Campaign.Clash
	s := &clashScene{cl: cl, done: done, wait: 50, round: 1, per: 50, pics: [2]string{c.Soldiers["player"], c.Soldiers["foe"]}}
	if c.PerSoldier > 0 {
		s.per = float64(c.PerSoldier)
	}
	s.army, s.layout, s.armies = len(c.Armies) > 0, c.Layout, c.Armies
	for side := range 2 {
		for _, u := range cl.Sides[side] {
			cu := &clashUnit{u: u, side: side, shown: u.Max, lag: u.Max, hp: float64(u.HP)}
			cu.soldiers = s.count(u.Max)
			if s.army {
				cu.kind = u.Army
				if _, ok := c.Armies[cu.kind]; !ok {
					cu.kind = c.FirstArmy()
				}
				cu.soldiers = s.count(u.Max)
			}
			s.units[side] = append(s.units[side], cu)
		}
	}
	for side := range 2 {
		for i, cu := range s.units[side] {
			cu.x, cu.y = s.slot(g, side, i)
		}
	}
	return s
}

// count is how many soldiers stand for troops.
func (s *clashScene) count(troops float64) int {
	return min(s.cap(), int(math.Ceil(troops/s.per)))
}

func (s *clashScene) overlay() bool { return false }

// standing is the place in the line of unit i among the units still standing.
func (s *clashScene) standing(side, i int) int {
	n := 0
	for j := range i {
		if !s.isRouted(side, j) {
			n++
		}
	}
	return n
}

func (s *clashScene) isRouted(side, i int) bool {
	for _, h := range s.cl.Hits[:s.next] {
		if h.Of == side && h.To == i && h.Routed {
			return true
		}
	}
	return false
}

// cardSize is a leader's card (UI units).
func cardSize(g *Game) (float64, float64) {
	_, sh := g.screen()
	return sh * 0.13, sh * 0.175
}

// blockWidth is a unit's block: its card and its ranks.
func blockWidth(g *Game) float64 {
	sw, _ := g.screen()
	return sw * 0.27
}

// slot is where unit i of a side has its card: the front units nearer the middle, the
// back ones nearer the edge, each in a lane; the player's on the left.
func (s *clashScene) slot(g *Game, side, i int) (float64, float64) {
	if s.army {
		return s.armySlot(g, side, i)
	}
	sw, sh := g.screen()
	k := s.standing(side, i)
	if s.isRouted(side, i) {
		k = i
	}
	cw, _ := cardSize(g)
	bw := blockWidth(g)
	front := max(1, s.cl.Front)
	back := k >= front
	lane := float64(k%front) + float64(campaign.Front-front)*0.5 // a duel stands in the middle
	x := sw*0.5 - sw*0.055 - cw                                  // the card stands at the block's inner edge
	if back {
		x -= bw
	}
	if side == 1 {
		x = sw - x - cw
	}
	return x, sh*0.3 + lane*sh*0.32
}

func (s *clashScene) update(g *Game) {
	s.age++
	in := &g.in
	if in.pressed[btnAuto] {
		s.auto = !s.auto
	}
	for side := range 2 {
		for i, cu := range s.units[side] {
			tx, ty := s.slot(g, side, i)
			cu.x += (tx - cu.x) * 0.12
			cu.y += (ty - cu.y) * 0.12
			if cu.lag > cu.shown {
				cu.lag = math.Max(cu.shown, cu.lag-cu.u.Max*0.012)
			}
			cu.shake, cu.glow, cu.lunge = max(0, cu.shake-1), max(0, cu.glow-1), max(0, cu.lunge-1)
			if s.isRouted(side, i) && cu.gone < 1 {
				cu.gone = math.Min(1, cu.gone+0.025)
			}
		}
	}
	live := s.pops[:0]
	for _, p := range s.pops {
		p.age++
		if p.age < p.life {
			live = append(live, p)
		}
	}
	s.pops = live
	im := s.impacts[:0]
	for _, x := range s.impacts {
		x.age++
		if x.age < impactTicks {
			im = append(im, x)
		}
	}
	s.impacts = im
	fl := s.falls[:0]
	for _, f := range s.falls {
		f.age++
		if f.age < 40 {
			fl = append(fl, f)
		}
	}
	s.falls = fl
	bs := s.bands[:0]
	for _, b := range s.bands {
		b.age++
		if b.age < 80 {
			bs = append(bs, b)
		}
	}
	s.bands = bs
	if s.cut != nil {
		s.cut.t++
		if s.cut.t >= clashCutTicks || ((in.ok() || in.click) && s.cut.t > 10) || g.skipping {
			s.cut = nil
		}
		return
	}
	if s.over {
		if in.ok() || in.click || (s.auto && s.wait <= 0) || g.replaying || g.skipping {
			g.pop()
			s.done()
			return
		}
		s.wait--
		return
	}
	if (in.ok() || in.click) && s.age > 10 { // not the press that began the march
		s.fast = true
	}
	s.wait--
	if s.wait > 0 {
		return
	}
	if s.next >= len(s.cl.Hits) {
		s.over, s.wait = true, 150
		g.sfx("decide")
		return
	}
	h := s.cl.Hits[s.next]
	s.round = h.Round
	first := h.Skill != "" && (s.next == 0 || s.cl.Hits[s.next-1].Skill != h.Skill || s.cl.Hits[s.next-1].At != h.At || s.cl.Hits[s.next-1].Side != h.Side || s.cl.Hits[s.next-1].Round != h.Round)
	if first && s.skillShown != s.next+1 {
		s.skillShown = s.next + 1
		s.useSkill(g, h)
		if s.cut != nil { // the strike comes after the cut-in
			return
		}
	}
	s.strike(g, h)
	s.next++
	s.wait = clashHitTicks
	if first {
		s.wait += 30
	}
	if s.fast || g.skipping {
		s.wait = clashFastTicks
	}
}

// useSkill shows a skill's name across its unit's card and the troops it gave back.
func (s *clashScene) useSkill(g *Game, h campaign.Hit) {
	label := h.Skill
	if sk := g.db.Game.Campaign.Clash.Skills[h.Skill]; sk != nil {
		label = sk.Label
	}
	s.bands = append(s.bands, skillBand{text: label, side: h.Side, at: h.At})
	if sk := g.db.Game.Campaign.Clash.Skills[h.Skill]; sk != nil && sk.Cutin && !s.fast && !g.skipping {
		u := s.units[h.Side][h.At]
		pic := u.u.Picture
		if a := g.db.Actors[u.u.Actor]; a != nil {
			if a.Cutin != "" {
				pic = a.Cutin
			} else {
				pic = g.db.FacePicture(a, a.Face, "")
			}
		}
		s.cut = &clashCut{u: u, name: label, pic: pic}
		g.sfx("cutin")
	}
	if h.Heal > 0 {
		u := s.units[h.Side][h.At]
		u.shown = math.Min(u.u.Max, u.shown+float64(h.Heal))
		u.lag = math.Max(u.lag, u.shown)
		u.soldiers = s.count(u.shown)
		x, y := s.beside(g, u)
		s.pops = append(s.pops, clashPop{text: fmt.Sprintf("+%d", h.Heal), x: x, y: y + 6, tx: x, ty: y - 10, life: 70, c: color.NRGBA{0x70, 0xe8, 0x90, 0xff}, size: sizeL * 1.5})
	}
	g.sfx("decide")
	g.emit("clash_skill", traceLine{"round": h.Round, "side": h.Side, "at": h.At, "skill": h.Skill, "heal": h.Heal})
}

// drawBands draws the skills' names across the cards: a band that slides in and fades.
func (s *clashScene) drawSkillBands(g *Game, dst *ebiten.Image) {
	for _, b := range s.bands {
		u := s.units[b.side][b.at]
		cx, top, cw := s.focus(g, u)
		_, ch := cardSize(g)
		if s.army {
			ch = s.geo(g).ah
		}
		a := 1.0
		if b.age > 60 {
			a = float64(80-b.age) / 20
		}
		in := math.Min(1, float64(b.age)/8)
		w := (cw + 80) * in
		x := cx - w/2
		y := top + ch*0.42
		gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(sizeL*1.4), color.NRGBA{0x18, 0x10, 0x08, uint8(220 * a)}, false)
		gfx.FillRect(dst, float32(x), float32(y), float32(w), 3, color.NRGBA{0xff, 0xd0, 0x60, uint8(255 * a)}, false)
		gfx.FillRect(dst, float32(x), float32(y+sizeL*1.4-3), float32(w), 3, color.NRGBA{0xff, 0xd0, 0x60, uint8(255 * a)}, false)
		if in >= 1 {
			gfx.DrawTextOutlineColor(dst, b.text, cx, y+sizeL*0.15, sizeL, color.NRGBA{0xff, 0xf0, 0xc0, 0xff}, color.NRGBA{0x40, 0x20, 0x00, 0xff}, float32(a))
		}
	}
}

// strike plays one strike.
func (s *clashScene) strike(g *Game, h campaign.Hit) {
	from, to := s.units[h.Side][h.At], s.units[h.Of][h.To]
	to.shown = math.Max(0, to.shown-float64(h.Loss))
	to.shake, to.glow, from.lunge = 18, 12, 14
	if n := s.count(to.shown); n < to.soldiers { // the soldiers lost fall from the back
		for k := n; k < to.soldiers; k++ {
			x, y, sz := s.soldierAt(g, to, k)
			s.falls = append(s.falls, fallen{x: x, y: y, h: sz, flip: to.side == 1, pic: s.soldierPic(to, k), side: to.side})
		}
		to.soldiers = n
	}
	// the burst stands on the line where the armies meet, at the unit struck
	mid, _ := g.screen()
	s.impacts = append(s.impacts, impact{x: mid / 2, y: s.midY(g, to), age: -impactDelay})
	tx, ty := s.beside(g, to)
	lift := 0.0 // a loss shown while another still shows over the same card goes above it
	for _, p := range s.pops {
		if p.x == tx && p.text != "" && p.text[0] == '-' && p.age < p.life/2 {
			lift += sizeL * 1.8
		}
	}
	fx, ftop, _ := s.focus(g, from)
	s.pops = append(s.pops,
		clashPop{text: fmt.Sprintf("-%d", h.Loss), x: tx, y: ty - lift, tx: tx, ty: ty - sizeL*0.8 - lift, life: 70, c: color.NRGBA{0xff, 0x48, 0x38, 0xff}, size: sizeL * 1.8},
		clashPop{text: fmt.Sprintf(lang.T("%d人"), h.Attackers), x: fx, y: ftop + sizeM*0.2, tx: fx, ty: ftop, life: 40, c: color.NRGBA{0xff, 0xf0, 0xb0, 0xff}, size: sizeM * 1.2},
	)
	if h.Routed {
		to.stamp = lang.T("壊滅")
		if h.Caught {
			to.stamp = lang.T("捕縛")
		}
	}
	g.sfx("hit")
	g.emit("clash_hit", traceLine{"round": h.Round, "side": h.Side, "to": h.To, "loss": h.Loss, "routed": h.Routed})
}

// beside is where a number about a unit appears: beside its card, or over its army.
func (s *clashScene) beside(g *Game, u *clashUnit) (float64, float64) {
	if s.army {
		cx, top, _ := s.focus(g, u)
		return cx, top + s.geo(g).ah*0.3
	}
	cw, ch := cardSize(g)
	if u.side == 0 {
		return u.x + cw + 60, u.y + ch*0.3
	}
	return u.x - 60, u.y + ch*0.3
}

// soldierPic is the picture of a unit's soldiers ("" draws a plain figure).
func (s *clashScene) soldierPic(u *clashUnit, k int) string {
	if s.army {
		if poses := s.poses(u); len(poses) > 0 {
			return poses[(k*5+k/3)%len(poses)] // the poses scattered, the same for a soldier each time
		}
	}
	if p := s.pics[u.side]; p != "" {
		return p
	}
	if u.side == 1 {
		return u.u.Picture
	}
	return ""
}

// soldierAt is where soldier k of a unit's ranks stands (its feet) and its height: the
// ranks fill the block behind the card, the front rank (nearest the viewer, lowest) first,
// each rank from the card outward; the last ones stand at the back, top.
func (s *clashScene) soldierAt(g *Game, u *clashUnit, k int) (float64, float64, float64) {
	if s.army {
		return s.armySoldierAt(g, u, k)
	}
	_, sh := g.screen()
	cw, _ := cardSize(g)
	room := blockWidth(g) - cw - 16
	col, row := k%soldierCols, k/soldierCols
	dx := room / soldierCols
	h := sh * 0.1
	x := u.x - 16 - dx*(float64(col)+0.5) - float64(row%2)*dx*0.35
	if u.side == 1 {
		x = u.x + cw + 16 + dx*(float64(col)+0.5) + float64(row%2)*dx*0.35
	}
	feet := u.y + float64(soldierRows-1-row)*sh*0.042
	return x, feet + h*0.6, h
}

// troopsAbove is where a unit's troops are written: over the middle of its ranks.
func (s *clashScene) troopsAbove(g *Game, u *clashUnit) (float64, float64) {
	cw, _ := cardSize(g)
	room := blockWidth(g) - cw - 16
	x := u.x - 16 - room/2
	if u.side == 1 {
		x = u.x + cw + 16 + room/2
	}
	rows := max(1, (u.soldiers+soldierCols-1)/soldierCols)
	_, top, h := s.soldierAt(g, u, soldierCols*(rows-1))
	return x, top - h - sizeM*1.3
}

func (s *clashScene) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0x5a, 0x6e, 0x48, 0xff}, false)
	if s.cl.Background != "" {
		g.drawPicture(dst, data.Picture{File: s.cl.Background}, 0, 0, sw, sh, 1)
	}
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0, 0, 0, 40}, false)
	s.drawBands(g, dst)
	if s.army {
		for side := range 2 {
			for _, u := range slices.Backward(s.units[side]) {
				s.drawArmyUnit(g, dst, u)
			}
		}
	}
	for side := range 2 { // the ranks (back units first), then the cards over them
		if s.army {
			break
		}
		for _, u := range slices.Backward(s.units[side]) {
			s.drawRanks(g, dst, u)
		}
	}
	for _, f := range s.falls {
		t := float64(f.age) / 40
		tilt := -t * 1.2
		if f.flip {
			tilt = -tilt
		}
		s.drawSoldier(g, dst, f.pic, f.side, f.x, f.y, f.h, tilt, float32(1-t))
	}
	for side := range 2 {
		if s.army {
			break
		}
		for _, u := range slices.Backward(s.units[side]) {
			s.drawCard(g, dst, u)
		}
	}
	for _, x := range s.impacts {
		drawImpact(dst, x)
	}
	s.drawSkillBands(g, dst)
	for _, p := range s.pops {
		t := math.Min(1, float64(p.age)/float64(min(p.life, 30)))
		x, y := p.x+(p.tx-p.x)*t, p.y+(p.ty-p.y)*t
		a := uint8(255)
		if p.age > p.life*2/3 {
			a = uint8(255 * float64(p.life-p.age) / float64(p.life/3))
		}
		outlined(dst, p.text, x, y, p.size, withAlpha(p.c, a), a)
	}
	r := fmt.Sprintf(lang.T("第%dラウンド / %d"), s.round, s.cl.MaxRounds)
	g.window(dst, sw/2-150, sh-86, 300, 58)
	gfx.DrawText(dst, r, sw/2, sh-74, sizeM, g.th.text)
	g.text(dst, s.hint(), 36, sh-40, sizeS*0.8, withAlpha(g.th.text, 200))
	if s.cut != nil {
		s.drawCut(g, dst)
	}
	if s.over {
		gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0, 0, 0, 150}, false)
		s.drawResult(g, dst)
	}
}

// outlined draws text centered on x with a dark edge, readable over any picture.
func outlined(dst *ebiten.Image, text string, x, y, size float64, c color.NRGBA, a uint8) {
	edge := withAlpha(color.NRGBA{0x10, 0x0c, 0x0c, 0xff}, a)
	for _, d := range [][2]float64{{-3, 0}, {3, 0}, {0, -3}, {0, 3}, {-2, -2}, {2, 2}, {-2, 2}, {2, -2}} {
		gfx.DrawText(dst, text, x+d[0], y+d[1], size, edge)
	}
	gfx.DrawText(dst, text, x, y, size, c)
}

func (s *clashScene) hint() string {
	h := lang.T("決定：早送り　A：自動")
	if s.auto {
		h += lang.T("（自動）")
	}
	return h
}

// drawRanks draws a unit's soldiers behind its card, its flag among them.
func (s *clashScene) drawRanks(g *Game, dst *ebiten.Image, u *clashUnit) {
	alpha := float32(1 - u.gone*0.6)
	part := float32(1) // the last soldier shows the troops short of a whole one, fainter
	if u.soldiers > 0 && u.soldiers < maxSoldiers {
		if rem := u.shown - float64(u.soldiers-1)*s.per; rem < s.per {
			part = float32(0.3 + 0.7*rem/s.per)
		}
	}
	for k := u.soldiers - 1; k >= 0; k-- {
		x, y, h := s.soldierAt(g, u, k)
		a := alpha
		if k == u.soldiers-1 {
			a *= part
		}
		s.drawSoldier(g, dst, s.soldierPic(u, k), u.side, x, y, h, 0, a)
	}
	if u.gone < 1 {
		tx, ty := s.troopsAbove(g, u)
		outlined(dst, strconv.Itoa(int(u.shown)), tx, ty, sizeM, plateInk, uint8(255*alpha))
	}
	if u.soldiers == 0 {
		return
	}
	x, y, h := s.soldierAt(g, u, soldierCols-1) // the flag, at the outer end of the front rank
	col := factionColor(g, g.db.Game.Campaign.Player)
	if u.side == 1 {
		col = factionColor(g, s.cl.Owner)
		if s.cl.Owner == "" {
			col = color.NRGBA{0xc8, 0x50, 0x40, 0xff}
		}
	}
	a := uint8(255 * alpha)
	top := y - h*1.9
	gfx.StrokeLine(dst, float32(x), float32(y-h*0.2), float32(x), float32(top), 3, withAlpha(color.NRGBA{0x40, 0x30, 0x20, 0xff}, a), true)
	fw := float32(h * 0.7)
	fx := float32(x)
	if u.side == 1 {
		fx -= fw
	}
	gfx.FillRect(dst, fx, float32(top), fw, float32(h*0.42), withAlpha(flagFill(col), a), false)
	gfx.StrokeRoundRect(dst, fx, float32(top), fw, float32(h*0.42), 2, 2, withAlpha(plateInk, a))
}

// drawImpact draws the burst where a strike lands: a puff of dust opening out and a few
// sparks, restrained and gone in 0.3 s.
func drawImpact(dst *ebiten.Image, im impact) {
	if im.age < 0 {
		return
	}
	t := float64(im.age) / impactTicks
	e := 1 - (1-t)*(1-t)
	for k := range 9 {
		ang := float64(k)*0.7 + 0.4
		d := 6 + e*64
		x := im.x + math.Cos(ang)*d
		y := im.y + math.Sin(ang)*d*0.55 - t*22
		gfx.FillCircle(dst, float32(x), float32(y), float32(12+e*16), color.NRGBA{0xd2, 0xc0, 0x9c, uint8(170 * (1 - t))}, true)
	}
	if t < 0.55 {
		for k := range 8 {
			ang := float64(k)*0.785 + 0.3
			r0, r1 := 8+e*58, 18+e*70
			gfx.StrokeLine(dst, float32(im.x+math.Cos(ang)*r0), float32(im.y+math.Sin(ang)*r0*0.8), float32(im.x+math.Cos(ang)*r1), float32(im.y+math.Sin(ang)*r1*0.8), float32(3*(1-t)), color.NRGBA{0xff, 0xe8, 0xb4, uint8(230 * (1 - t/0.55))}, true)
		}
	}
	if t < 0.3 {
		gfx.FillCircle(dst, float32(im.x), float32(im.y), float32(10+e*30), color.NRGBA{0xff, 0xf4, 0xd8, uint8(150 * (1 - t/0.3))}, true)
	}
}

// drawSoldier draws one soldier standing on (x, y), h tall, tilted by tilt radians: the
// picture, or a plain figure in the side's colour.
func (s *clashScene) drawSoldier(g *Game, dst *ebiten.Image, pic string, side int, x, y, h, tilt float64, alpha float32) {
	if pic != "" {
		if img := g.pics.get(pic); img != nil && !img.Bounds().Empty() {
			b := img.Bounds()
			k := h / float64(b.Dy())
			op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
			op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy()))
			op.GeoM.Scale(k, k)
			if side == 1 { // the pictures face right; the foe is turned to face left
				op.GeoM.Scale(-1, 1)
			}
			op.GeoM.Rotate(tilt)
			op.GeoM.Translate(x, y)
			if s.red > 0 {
				op.ColorScale.Scale(1, 1-s.red, 1-s.red, 1)
			}
			op.ColorScale.ScaleAlpha(alpha)
			gfx.DrawImage(dst, img, op)
			return
		}
	}
	c := flagFill(factionColor(g, g.db.Game.Campaign.Player))
	if side == 1 {
		c = color.NRGBA{0xb0, 0x40, 0x34, 0xff}
	}
	a := uint8(255 * alpha)
	dx := math.Sin(tilt) * h * 0.5
	gfx.FillRoundRect(dst, float32(x-h*0.14+dx), float32(y-h*0.62), float32(h*0.28), float32(h*0.55), 4, withAlpha(c, a))
	gfx.FillCircle(dst, float32(x+dx*1.4), float32(y-h*0.76), float32(h*0.13), withAlpha(color.NRGBA{0xf0, 0xd8, 0xc0, 0xff}, a), true)
	sx := float32(h * 0.3)
	if side == 1 {
		sx = -sx
	}
	gfx.StrokeLine(dst, float32(x+dx), float32(y-h*0.2), float32(x+dx)+sx, float32(y-h*1.0), 2, withAlpha(color.NRGBA{0x60, 0x50, 0x40, 0xff}, a), true)
}

// drawBands draws the two sides' troops as one rope in a frame: the player's colour from
// the left, the foe's from the right, each lit from above, meeting at a small diamond
// where their troops stand against each other; a small flag and the troops outside each
// end, the region's name on a plate over the middle. clash.rope: false leaves it out.
func (s *clashScene) drawBands(g *Game, dst *ebiten.Image) {
	if r := g.db.Game.Campaign.Clash.Rope; r != nil && !*r {
		return
	}
	sw, _ := g.screen()
	var total [2]float64
	for side := range 2 {
		for _, cu := range s.units[side] {
			total[side] += cu.shown
		}
	}
	cols := [2]color.NRGBA{flagFill(factionColor(g, g.db.Game.Campaign.Player)), flagFill(factionColor(g, s.cl.Owner))}
	if s.cl.Owner == "" {
		cols[1] = color.NRGBA{0xc8, 0x50, 0x40, 0xff}
	}
	x, w, y, h := sw*0.16, sw*0.68, 70.0, 14.0
	share := 0.5
	if t := total[0] + total[1]; t > 0 {
		share = total[0] / t
	}
	// the frame: dark wood with a lighter rim, the groove shadowed inside
	gfx.FillRoundRect(dst, float32(x-7), float32(y-7), float32(w+14), float32(h+14), 5, color.NRGBA{0x3a, 0x2a, 0x1c, 0xf0})
	gfx.StrokeRoundRect(dst, float32(x-7), float32(y-7), float32(w+14), float32(h+14), 5, 2, color.NRGBA{0xb8, 0x96, 0x60, 0xff})
	gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(h), color.NRGBA{0x10, 0x0c, 0x0a, 0xff}, false)
	shade := func(c color.NRGBA, k float64) color.NRGBA {
		return color.NRGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), 0xff}
	}
	light := func(c color.NRGBA) color.NRGBA {
		return color.NRGBA{uint8(min(255, int(c.R)+60)), uint8(min(255, int(c.G)+60)), uint8(min(255, int(c.B)+60)), 0xff}
	}
	part := func(c color.NRGBA, px, pw float64) {
		gfx.FillRect(dst, float32(px), float32(y), float32(pw), float32(h*0.5), c, false)
		gfx.FillRect(dst, float32(px), float32(y+h*0.5), float32(pw), float32(h*0.5), shade(c, 0.68), false)
		gfx.FillRect(dst, float32(px), float32(y+2), float32(pw), 1.5, light(c), false)
	}
	part(cols[0], x, w*share)
	part(cols[1], x+w*share, w*(1-share))
	gfx.FillRect(dst, float32(x), float32(y), float32(w), 2, color.NRGBA{0, 0, 0, 0x80}, false) // the groove's shadow
	mx, my := x+w*share, y+h/2
	for i := range 9 { // the diamond where they meet
		d := 10 - math.Abs(float64(i-4))*2.5
		gfx.FillRect(dst, float32(mx-d), float32(my-10+float64(i)*2.5), float32(2*d), 2.5, color.NRGBA{0xff, 0xec, 0xb0, 0xff}, false)
	}
	gfx.StrokeCircle(dst, float32(mx), float32(my), 12, 1.5, color.NRGBA{0x3a, 0x2a, 0x1c, 0xff}, true)
	// the ends: a small flag and the troops
	for side := range 2 {
		fx := x - 30
		if side == 1 {
			fx = x + w + 30
		}
		gfx.StrokeLine(dst, float32(fx), float32(y-14), float32(fx), float32(y+h+8), 2, color.NRGBA{0x3a, 0x2a, 0x1c, 0xff}, true)
		fw := float32(20.0)
		px := float32(fx)
		if side == 1 {
			px -= fw
		}
		gfx.FillRect(dst, px, float32(y-14), fw, 13, cols[side], false)
		n := strconv.Itoa(int(total[side]))
		tx := fx - 16 - gfx.TextWidth(n, sizeL)/2
		if side == 1 {
			tx = fx + 16 + gfx.TextWidth(n, sizeL)/2
		}
		outlined(dst, n, tx, y+h/2-sizeL*0.7, sizeL, plateInk, 255)
	}
	name := data.RegionName(g.db, s.cl.Region)
	nw := gfx.TextWidth(name, sizeM) + 56
	g.window(dst, sw/2-nw/2, 12, nw, sizeM*1.5)
	gfx.DrawText(dst, "◆", sw/2-nw/2+16, 12+sizeM*0.22, sizeS*0.7, color.NRGBA{0xd8, 0xb8, 0x78, 0xff})
	gfx.DrawText(dst, "◆", sw/2+nw/2-16, 12+sizeM*0.22, sizeS*0.7, color.NRGBA{0xd8, 0xb8, 0x78, 0xff})
	gfx.DrawText(dst, name, sw/2, 12+sizeM*0.12, sizeM, g.th.text)
}

// drawCard draws a leader's card: the face, the name, the gauge of troops and the HP.
func (s *clashScene) drawCard(g *Game, dst *ebiten.Image, cu *clashUnit) {
	cw, ch := cardSize(g)
	x, y := cu.x, cu.y
	if cu.shake > 0 {
		x += math.Sin(float64(cu.shake)*1.7) * 9 * float64(cu.shake) / 18
	}
	if cu.lunge > 0 { // a step toward the middle and back
		d := math.Sin(float64(14-cu.lunge)/14*math.Pi) * 36
		if cu.side == 1 {
			d = -d
		}
		x += d
	}
	s.drawCardIn(g, dst, cu, box{x, y, cw, ch}, false)
}

// drawFace draws a leader's picture in a box: a standing picture is cut to the face and
// the shoulders.
func (s *clashScene) drawFace(g *Game, dst *ebiten.Image, cu *clashUnit, x, y, w, h float64, alpha float32) {
	if cu.u.Picture == "" {
		return
	}
	img := g.pics.get(cu.u.Picture)
	if img == nil || img.Bounds().Empty() {
		return
	}
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	sub := img
	if cu.u.Actor != "" {
		cut := int(math.Min(ih, iw*h/w))
		if c, ok := img.SubImage(image.Rect(0, 0, int(iw), cut)).(*ebiten.Image); ok {
			sub = c
		}
		ih = float64(sub.Bounds().Dy())
	}
	k := math.Min(w/iw, h/ih)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(x+(w-iw*k)/2, y+(h-ih*k)/2)
	if cu.gone > 0 {
		op.ColorScale.Scale(0.45, 0.45, 0.45, 1)
	}
	if cu.glow > 0 { // struck: red
		f := 1 - float32(cu.glow)/20
		op.ColorScale.Scale(1, f, f, 1)
	}
	op.ColorScale.ScaleAlpha(alpha)
	gfx.DrawImage(dst, sub, op)
}

// drawCardIn draws a leader's card in a box. A compact card is a row: the face at the
// left, the name and the HP beside it.
func (s *clashScene) drawCardIn(g *Game, dst *ebiten.Image, cu *clashUnit, r box, compact bool) {
	x, y, cw, ch := r.x, r.y, r.w, r.h
	alpha := float32(1 - cu.gone*0.5)
	g.window(dst, x, y, cw, ch)
	if compact {
		fs := ch - 14
		s.drawFace(g, dst, cu, x+7, y+7, fs, fs, alpha)
		if cu.glow > 0 {
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 16, color.NRGBA{0xff, 0x30, 0x20, uint8(min(255, 10*cu.glow))})
		}
		tx := x + fs + 20
		g.text(dst, gfx.Ellipsis(cu.u.Name, sizeS, cw-fs-34), tx, y+ch*0.14, sizeS, withAlpha(g.th.text, uint8(255*alpha)))
		if cu.u.MHP > 0 {
			hr := cu.hp / float64(cu.u.MHP)
			bar(dst, tx, y+ch-24, cw-fs-40, 8, hr, withAlpha(hpColor(hr), uint8(255*alpha)))
		}
		return
	}
	ph := ch - 52
	s.drawFace(g, dst, cu, x+8, y+8, cw-16, ph, alpha)
	if cu.glow > 0 {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 16, color.NRGBA{0xff, 0x30, 0x20, uint8(min(255, 10*cu.glow))})
	}
	gfx.DrawText(dst, gfx.Ellipsis(cu.u.Name, sizeS*0.85, cw-12), x+cw/2, y+ph+6, sizeS*0.85, withAlpha(g.th.text, uint8(255*alpha)))
	if cu.u.MHP > 0 { // the leader's HP, a thin line
		r := cu.hp / float64(cu.u.MHP)
		bar(dst, x+12, y+ch-14, cw-24, 7, r, withAlpha(hpColor(r), uint8(255*alpha)))
	}
	if cu.stamp != "" && !s.army {
		sw := gfx.TextWidth(cu.stamp, sizeL*1.1) + 20
		bx, by := x+(cw-sw)/2, y+ph*0.4
		gfx.FillRoundRect(dst, float32(bx), float32(by), float32(sw), float32(sizeL*1.1+14), 6, color.NRGBA{0xff, 0xf6, 0xe8, 0xe8})
		gfx.StrokeRoundRect(dst, float32(bx+3), float32(by+3), float32(sw-6), float32(sizeL*1.1+8), 4, 3, color.NRGBA{0xc8, 0x20, 0x20, 0xff})
		gfx.DrawText(dst, cu.stamp, x+cw/2, by+5, sizeL*1.1, color.NRGBA{0xc8, 0x20, 0x20, 0xff})
	}
}

// drawResult draws the table of how the clash went.
func (s *clashScene) drawResult(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	w, h := sw*0.6, sh*0.52
	x, y := (sw-w)/2, (sh-h)/2
	g.window(dst, x, y, w, h)
	head := map[string]string{"won": lang.T("勝利"), "lost": lang.T("敗北"), "draw": lang.T("引き分け")}[s.cl.Result]
	gfx.DrawText(dst, head, sw/2, y+20, sizeL*1.3, g.th.accent)
	names := [2]string{campaign.FactionName(g.db, g.db.Game.Campaign.Player), campaign.FactionName(g.db, s.cl.Owner)}
	if s.cl.Owner == "" {
		names[1] = data.RegionName(g.db, s.cl.Region)
	}
	ry := y + 90
	for side := range 2 {
		var sent, left float64
		for _, u := range s.cl.Sides[side] {
			sent += u.Max
			left += u.Troops
		}
		loss := sent - left
		rate := 0.0
		if sent > 0 {
			rate = loss / sent
		}
		g.text(dst, names[side], x+40, ry, sizeM, g.th.text)
		g.text(dst, fmt.Sprintf(lang.T("出陣 %d → 残り %d（損害 -%d）"), int(sent), int(left), int(loss)), x+40+w*0.22, ry, sizeM, g.th.text)
		bx, bw := x+40+w*0.22, w*0.5
		gfx.FillRect(dst, float32(bx), float32(ry+38), float32(bw), 10, color.NRGBA{0x10, 0x10, 0x14, 0xe0}, false)
		gfx.FillRect(dst, float32(bx), float32(ry+38), float32(bw*rate), 10, color.NRGBA{0xd8, 0x30, 0x30, 0xff}, false)
		g.text(dst, fmt.Sprintf("%d%%", int(math.Round(rate*100))), bx+bw+12, ry+28, sizeS, g.th.text)
		ry += 86
	}
	var caught []string
	for _, u := range s.cl.Sides[1] {
		if u.Caught {
			caught = append(caught, u.Name)
		}
	}
	if len(caught) > 0 {
		g.text(dst, fmt.Sprintf(lang.T("捕縛：%s"), joinNames(caught)), x+40, ry, sizeM, g.th.text)
		ry += 40
	}
	if s.cl.Result == "won" {
		g.text(dst, fmt.Sprintf(lang.T("%sを取った"), data.RegionName(g.db, s.cl.Region)), x+40, ry, sizeM, g.th.accent)
	}
}

func joinNames(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += lang.T("、")
		}
		out += n
	}
	return out
}

// drawCut draws a skill's cut-in: the screen darkens, a slanted band slides open across
// it in the side's colour with the user's picture sliding in from its side and the
// skill's name, and closes.
func (s *clashScene) drawCut(g *Game, dst *ebiten.Image) {
	c := s.cut
	sw, sh := g.screen()
	open := math.Min(1, float64(c.t)/8)
	if c.t > clashCutTicks-10 {
		open = math.Max(0, float64(clashCutTicks-c.t)/10)
	}
	open = 1 - (1-open)*(1-open)
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0, 0, 0, uint8(110 * open)}, false)
	bandH := 260 * open
	if bandH < 2 {
		return
	}
	cy := sh * 0.46
	col := factionColor(g, g.db.Game.Campaign.Player)
	if c.u.side == 1 {
		col = color.NRGBA{0xc8, 0x50, 0x40, 0xff}
		if s.cl.Owner != "" {
			col = factionColor(g, s.cl.Owner)
		}
	}
	gfx.FillRect(dst, 0, float32(cy-bandH/2-6), float32(sw), float32(bandH+12), withAlpha(flagFill(col), 0xe0), false)
	gfx.FillRect(dst, 0, float32(cy-bandH/2), float32(sw), float32(bandH), withAlpha(g.th.window, 0xf4), false)
	for i := range 12 { // speed lines
		y := cy - bandH/2 + math.Mod(float64(i)*37.3, bandH)
		x := math.Mod(float64(i)*211.7+float64(c.t)*42, sw+300) - 150
		if c.u.side == 1 {
			x = sw - x
		}
		gfx.StrokeLine(dst, float32(x), float32(y), float32(x+160), float32(y), 2, withAlpha(flagFill(col), 0x70), true)
	}
	slide := math.Min(1, float64(c.t)/12)
	if c.pic != "" {
		if img := g.pics.get(c.pic); img != nil && !img.Bounds().Empty() {
			b := img.Bounds()
			cut := min(b.Dy(), int(float64(b.Dx())*0.9)) // the head and shoulders
			sub, ok := img.SubImage(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+cut)).(*ebiten.Image)
			if ok {
				k := bandH / float64(cut)
				w := float64(b.Dx()) * k
				x := sw*0.18 - w/2 - (1-slide)*sw*0.3
				if c.u.side == 1 {
					x = sw*0.82 - w/2 + (1-slide)*sw*0.3
				}
				op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
				op.GeoM.Scale(k, k)
				op.GeoM.Translate(x, cy-bandH/2)
				gfx.DrawImage(dst, sub, op)
			}
		}
	}
	tx := sw * 0.58
	if c.u.side == 1 {
		tx = sw * 0.42
	}
	gfx.DrawTextOutlineColor(dst, c.u.u.Name, tx, cy-sizeL*1.6, sizeM, g.th.text, g.th.window, float32(open))
	gfx.DrawTextOutlineColor(dst, c.name, tx, cy-sizeL*0.5, sizeL*1.8, color.NRGBA{0xff, 0xf0, 0xc0, 0xff}, color.NRGBA{0x40, 0x20, 0x00, 0xff}, float32(open))
}
