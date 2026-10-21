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
// front is routed. A strike lunges the striker's card toward the middle, draws a slash and
// dust on the card struck, shakes it red, shows the troops lost beside it and the
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
}

type clashPop struct {
	text         string
	x, y, tx, ty float64
	age, life    int
	c            color.NRGBA
	size         float64
}

type slash struct {
	x0, y0, x1, y1 float64
	age            int
}

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
	slashes []slash
	falls   []fallen
	over    bool
	age     int
	per     float64
	pics    [2]string // the soldiers' pictures
}

func newClashScene(g *Game, cl *campaign.Clash, done func()) *clashScene {
	c := g.db.Game.Campaign.Clash
	s := &clashScene{cl: cl, done: done, wait: 50, round: 1, per: 50, pics: [2]string{c.Soldiers["player"], c.Soldiers["foe"]}}
	if c.PerSoldier > 0 {
		s.per = float64(c.PerSoldier)
	}
	for side := range 2 {
		for _, u := range cl.Sides[side] {
			cu := &clashUnit{u: u, side: side, shown: u.Max, lag: u.Max, hp: float64(u.HP)}
			cu.soldiers = s.count(u.Max)
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
	return min(maxSoldiers, int(math.Ceil(troops/s.per)))
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
	return sh * 0.17, sh * 0.255
}

// blockWidth is a unit's block: its card and its ranks.
func blockWidth(g *Game) float64 {
	sw, _ := g.screen()
	return sw * 0.215
}

// slot is where unit i of a side has its card: the front units nearer the middle, the
// back ones nearer the edge, each in a lane; the player's on the left.
func (s *clashScene) slot(g *Game, side, i int) (float64, float64) {
	sw, sh := g.screen()
	k := s.standing(side, i)
	if s.isRouted(side, i) {
		k = i
	}
	cw, _ := cardSize(g)
	bw := blockWidth(g)
	back := k >= campaign.Front
	lane := float64(k % campaign.Front)
	x := sw*0.5 - sw*0.055 - cw // the card stands at the block's inner edge
	if back {
		x -= bw
	}
	if side == 1 {
		x = sw - x - cw
	}
	return x, sh*0.29 + lane*sh*0.3
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
	sl := s.slashes[:0]
	for _, x := range s.slashes {
		x.age++
		if x.age < 18 {
			sl = append(sl, x)
		}
	}
	s.slashes = sl
	fl := s.falls[:0]
	for _, f := range s.falls {
		f.age++
		if f.age < 40 {
			fl = append(fl, f)
		}
	}
	s.falls = fl
	if s.over {
		if in.ok() || in.click || (s.auto && s.wait <= 0) || g.replaying || g.skipping {
			g.pop()
			s.done()
			return
		}
		s.wait--
		return
	}
	if in.ok() || in.click {
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
	s.strike(g, h)
	s.next++
	s.wait = clashHitTicks
	if s.fast || g.skipping {
		s.wait = clashFastTicks
	}
}

// strike plays one strike.
func (s *clashScene) strike(g *Game, h campaign.Hit) {
	from, to := s.units[h.Side][h.At], s.units[h.Of][h.To]
	to.shown = math.Max(0, to.shown-float64(h.Loss))
	to.shake, to.glow, from.lunge = 18, 12, 14
	cw, ch := cardSize(g)
	if n := s.count(to.shown); n < to.soldiers { // the soldiers lost fall from the back
		for k := n; k < to.soldiers; k++ {
			x, y, sz := s.soldierAt(g, to, k)
			s.falls = append(s.falls, fallen{x: x, y: y, h: sz, flip: to.side == 1, pic: s.soldierPic(to), side: to.side})
		}
		to.soldiers = n
	}
	inner := func(u *clashUnit) float64 {
		if u.side == 0 {
			return u.x + cw
		}
		return u.x
	}
	s.slashes = append(s.slashes, slash{x0: inner(from), y0: from.y + ch*0.4, x1: to.x + cw/2, y1: to.y + ch*0.35})
	beside := func(u *clashUnit) float64 {
		if u.side == 0 {
			return u.x + cw + 60
		}
		return u.x - 60
	}
	s.pops = append(s.pops,
		clashPop{text: fmt.Sprintf("-%d", h.Loss), x: beside(to), y: to.y + ch*0.3, tx: beside(to), ty: to.y + ch*0.12, life: 70, c: color.NRGBA{0xff, 0x48, 0x38, 0xff}, size: sizeL * 1.8},
		clashPop{text: fmt.Sprintf(lang.T("%d人"), h.Attackers), x: beside(from), y: from.y + ch*0.7, tx: beside(from) + (beside(to)-beside(from))*0.3, ty: from.y + ch*0.62, life: 40, c: color.NRGBA{0xff, 0xf0, 0xb0, 0xff}, size: sizeM * 1.2},
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

// soldierPic is the picture of a unit's soldiers ("" draws a plain figure).
func (s *clashScene) soldierPic(u *clashUnit) string {
	if p := s.pics[u.side]; p != "" {
		return p
	}
	if u.side == 1 {
		return u.u.Picture
	}
	return ""
}

// soldierAt is where soldier k of a unit's ranks stands (its feet) and its height: the
// ranks fill the block behind the card, front rank first.
func (s *clashScene) soldierAt(g *Game, u *clashUnit, k int) (float64, float64, float64) {
	cw, ch := cardSize(g)
	room := blockWidth(g) - cw - 10
	col, row := k%soldierCols, k/soldierCols
	dx := room / soldierCols
	h := ch * 0.36
	x := u.x - 10 - dx*(float64(col)+0.5) - float64(row%2)*dx*0.4
	if u.side == 1 {
		x = u.x + cw + 10 + dx*(float64(col)+0.5) + float64(row%2)*dx*0.4
	}
	y := u.y + ch*0.32 + float64(row)*ch*0.19
	return x, y + h, h
}

func (s *clashScene) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0x5a, 0x6e, 0x48, 0xff}, false)
	if s.cl.Background != "" {
		g.drawPicture(dst, data.Picture{File: s.cl.Background}, 0, 0, sw, sh, 1)
	}
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), color.NRGBA{0, 0, 0, 40}, false)
	s.drawBands(g, dst)
	for side := range 2 { // the ranks (back units first), then the cards over them
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
		for _, u := range slices.Backward(s.units[side]) {
			s.drawCard(g, dst, u)
		}
	}
	for _, x := range s.slashes {
		a := uint8(255 * (1 - float64(x.age)/18))
		t := math.Min(1, float64(x.age)/6)
		mx, my := x.x0+(x.x1-x.x0)*t, x.y0+(x.y1-x.y0)*t
		gfx.StrokeLine(dst, float32(x.x0), float32(x.y0), float32(mx), float32(my), 5, withAlpha(color.NRGBA{0xff, 0xf8, 0xe0, 0xff}, a), true)
		for k := range 5 { // dust at the card struck
			ang := float64(k)*1.3 + float64(x.age)*0.1
			r := float32(10 + x.age*2)
			gfx.FillCircle(dst, float32(x.x1+math.Cos(ang)*float64(x.age)*3), float32(x.y1+40+math.Sin(ang)*float64(x.age)*1.5), r*0.6, withAlpha(color.NRGBA{0xd8, 0xc8, 0xa8, 0xff}, a/2), true)
		}
	}
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
	pic := s.soldierPic(u)
	alpha := float32(1 - u.gone*0.6)
	for k := u.soldiers - 1; k >= 0; k-- {
		x, y, h := s.soldierAt(g, u, k)
		s.drawSoldier(g, dst, pic, u.side, x, y, h, 0, alpha)
	}
	if u.soldiers == 0 {
		return
	}
	x, y, h := s.soldierAt(g, u, min(2, u.soldiers-1)) // the flag, in the front rank
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
			op.GeoM.Rotate(tilt)
			op.GeoM.Translate(x, y)
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

// drawBands draws the two sides' troops as one rope: the player's colour from the left,
// the foe's from the right, meeting where their troops stand against each other, with the
// numbers over each end.
func (s *clashScene) drawBands(g *Game, dst *ebiten.Image) {
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
	x, w, y, h := 60.0, sw-120, 46.0, 26.0
	share := 0.5
	if t := total[0] + total[1]; t > 0 {
		share = total[0] / t
	}
	gfx.FillRoundRect(dst, float32(x-3), float32(y-3), float32(w+6), float32(h+6), 8, color.NRGBA{0x10, 0x10, 0x14, 0xd0})
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 6, cols[1])
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(w*share), float32(h), 6, cols[0])
	gfx.FillRect(dst, float32(x+w*share-2), float32(y-4), 4, float32(h+8), plateInk, false)
	left, right := strconv.Itoa(int(total[0])), strconv.Itoa(int(total[1]))
	outlined(dst, left, x+gfx.TextWidth(left, sizeL)/2, 6, sizeL, plateInk, 255)
	outlined(dst, right, x+w-gfx.TextWidth(right, sizeL)/2, 6, sizeL, plateInk, 255)
	outlined(dst, data.RegionName(g.db, s.cl.Region), sw/2, 6, sizeL, g.th.accent, 255)
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
	alpha := float32(1 - cu.gone*0.5)
	g.window(dst, x, y, cw, ch)
	ph := ch * 0.56
	if cu.u.Picture != "" {
		if img := g.pics.get(cu.u.Picture); img != nil && !img.Bounds().Empty() {
			iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
			sub := img
			if cu.u.Actor != "" { // a standing picture: the face and the shoulders
				cut := int(math.Min(ih, iw*ph/(cw-16)))
				if c, ok := img.SubImage(image.Rect(0, 0, int(iw), cut)).(*ebiten.Image); ok {
					sub = c
				}
				ih = float64(sub.Bounds().Dy())
			}
			k := math.Min((cw-16)/iw, ph/ih)
			op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
			op.GeoM.Scale(k, k)
			op.GeoM.Translate(x+(cw-iw*k)/2, y+8+(ph-ih*k)/2)
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
	}
	if cu.glow > 0 {
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(cw), float32(ch), 16, color.NRGBA{0xff, 0x30, 0x20, uint8(min(255, 10*cu.glow))})
	}
	gfx.DrawText(dst, cu.u.Name, x+cw/2, y+ph+8, sizeS*0.9, withAlpha(g.th.text, uint8(255*alpha)))
	gx, gy, gw, gh := x+10, y+ph+36, cw-20, 14.0
	gfx.FillRoundRect(dst, float32(gx), float32(gy), float32(gw), float32(gh), 4, color.NRGBA{0x10, 0x10, 0x14, 0xe0})
	if cu.u.Max > 0 {
		gfx.FillRoundRect(dst, float32(gx), float32(gy), float32(gw*cu.lag/cu.u.Max), float32(gh), 4, color.NRGBA{0xd8, 0x30, 0x30, 0xff})
		c := color.NRGBA{0x50, 0xc8, 0x70, 0xff}
		if cu.side == 1 {
			c = color.NRGBA{0xe8, 0xb0, 0x40, 0xff}
		}
		gfx.FillRoundRect(dst, float32(gx), float32(gy), float32(gw*cu.shown/cu.u.Max), float32(gh), 4, c)
	}
	gfx.DrawText(dst, fmt.Sprintf("%d / %d", int(cu.shown), int(cu.u.Max)), x+cw/2, gy+gh+3, sizeS*0.8, withAlpha(g.th.text, uint8(255*alpha)))
	if cu.u.MHP > 0 {
		hy := gy + gh + 26
		gfx.FillRect(dst, float32(gx), float32(hy), float32(gw), 5, color.NRGBA{0x10, 0x10, 0x14, 0xe0}, false)
		gfx.FillRect(dst, float32(gx), float32(hy), float32(gw*cu.hp/float64(cu.u.MHP)), 5, color.NRGBA{0x70, 0xa8, 0xe8, 0xff}, false)
	}
	if cu.stamp != "" {
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
