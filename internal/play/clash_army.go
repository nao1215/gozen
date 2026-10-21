package play

import (
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// A clash shown with armies (clash.armies): each unit is a crowd of soldiers the engine
// lines up in staggered rows from the poses of its kind: the rows behind are a little
// smaller and higher, every other row half a step aside, the front row drawn last, the
// poses scattered. Under it a gauge of the troops (the side's colour, a red band that
// shrinks after a loss) and their number, and the leader's card. The soldiers lost fall
// from the back rows. clash.layout "front" stands the card in front of the army, at the
// edge facing the foe; "below" puts a smaller card under the army. The pictures face right;
// the foe side is turned to face left.

const (
	armyCols = 8
	armyRows = 4
	armyBarH = 18.0
	armyNumW = 84.0 // the troops written beside the gauge
)

type box struct{ x, y, w, h float64 }

// armyGeo is the size of a unit's block (UI units).
type armyGeo struct {
	bw           float64 // the block's width
	aw, ah       float64 // the army's picture box
	cw, ch       float64 // the card
	height       float64 // the whole block
	lane0, laneΔ float64 // the top of the first lane and the step to the next
}

func (s *clashScene) geo(g *Game) armyGeo {
	sw, sh := g.screen()
	bw := (sw*0.445 - sw*0.02) / 2
	if s.layout == "below" {
		ah := sh * 0.215
		ch := sh * 0.085
		return armyGeo{bw: bw, aw: bw, ah: ah, cw: bw, ch: ch, height: ah + 6 + armyBarH + 8 + ch, lane0: sh * 0.17, laneΔ: sh * 0.34}
	}
	cw, ch := sh*0.105, sh*0.17
	return armyGeo{bw: bw, aw: bw - cw - 10, ah: ch, cw: cw, ch: ch, height: ch + 8 + armyBarH, lane0: sh * 0.27, laneΔ: sh * 0.3}
}

// armySlot is where unit i of a side has its block: the front units nearer the middle, the
// back ones nearer the edge, each in a lane; the player's on the left.
func (s *clashScene) armySlot(g *Game, side, i int) (float64, float64) {
	sw, _ := g.screen()
	k := s.standing(side, i)
	if s.isRouted(side, i) {
		k = i
	}
	m := s.geo(g)
	front := max(1, s.cl.Front)
	lane := float64(k%front) + float64(2-front)*0.5 // a duel stands in the middle
	x := sw*0.445 - m.bw
	if k >= front {
		x -= m.bw
	}
	if side == 1 {
		x = sw - x - m.bw
	}
	return x, m.lane0 + lane*m.laneΔ
}

// armyParts are the army's box, the gauge and the card of a unit where it stands now. The
// side 1 block is the mirror of the side 0's.
func (s *clashScene) armyParts(g *Game, u *clashUnit) (army, gauge, card box) {
	m := s.geo(g)
	mirror := func(off, w float64) float64 {
		if u.side == 1 {
			return u.x + m.bw - off - w
		}
		return u.x + off
	}
	cardOff, cardY := m.bw-m.cw, u.y
	if s.layout == "below" {
		cardOff, cardY = 0, u.y+m.ah+6+armyBarH+8
	}
	army = box{mirror(0, m.aw), u.y, m.aw, m.ah}
	barW := m.aw - armyNumW
	barOff := 0.0
	if u.side == 1 {
		barOff = armyNumW
	}
	gauge = box{army.x + barOff, u.y + m.ah + 6, barW, armyBarH}
	card = box{mirror(cardOff, m.cw), cardY, m.cw, m.ch}
	return army, gauge, card
}

// armyFocus is where a unit's pops and skill bands belong: its picture's middle, top and
// width (the card's, with no armies).
func (s *clashScene) focus(g *Game, u *clashUnit) (cx, top, w float64) {
	if !s.army {
		cw, _ := cardSize(g)
		return u.x + cw/2, u.y, cw
	}
	a, _, _ := s.armyParts(g, u)
	return a.x + a.w/2, a.y, a.w * 0.8
}

// midY is the height at which a strike lands on a unit.
func (s *clashScene) midY(g *Game, u *clashUnit) float64 {
	if !s.army {
		_, ch := cardSize(g)
		return u.y + ch*0.6
	}
	m := s.geo(g)
	return u.y + m.ah*0.55
}

// cap is the most soldiers in a unit's crowd.
func (s *clashScene) cap() int {
	if s.army {
		return armyCols * armyRows
	}
	return maxSoldiers
}

// poses are the pictures of a unit's soldiers.
func (s *clashScene) poses(u *clashUnit) []string {
	return s.armies[u.kind]
}

// armySoldierAt is where soldier k of a unit's crowd stands (the middle of its feet) and
// its height: the rows start at the front and go back, each smaller and higher.
func (s *clashScene) armySoldierAt(g *Game, u *clashUnit, k int) (float64, float64, float64) {
	b, _, _ := s.armyParts(g, u)
	row := k / armyCols
	col := (k % armyCols * 3) % armyCols // the places filled in a scattered order
	hf := b.h * 0.6
	w := hf * 0.45
	dx := (b.w - w) / (armyCols - 0.5)
	x := w/2 + dx*float64(col) + float64(row%2)*dx*0.5
	if u.side == 1 {
		x = b.w - x
	}
	return b.x + x, b.y + b.h - float64(row)*b.h*0.11, hf * math.Pow(0.9, float64(row))
}

// drawArmyUnit draws a unit: its soldiers, the gauge and number, the card.
func (s *clashScene) drawArmyUnit(g *Game, dst *ebiten.Image, cu *clashUnit) {
	army, gauge, card := s.armyParts(g, cu)
	dx := 0.0
	if cu.shake > 0 {
		dx += math.Sin(float64(cu.shake)*1.7) * 9 * float64(cu.shake) / 18
	}
	if cu.lunge > 0 { // a step toward the middle and back
		d := math.Sin(float64(14-cu.lunge)/14*math.Pi) * 36
		if cu.side == 1 {
			d = -d
		}
		dx += d
	}
	alpha := float32(1 - cu.gone*0.6)
	part := float32(1) // the last soldier shows the troops short of a whole one, fainter
	if cu.soldiers > 0 && cu.soldiers < s.cap() {
		if rem := cu.shown - float64(cu.soldiers-1)*s.per; rem < s.per {
			part = float32(0.3 + 0.7*rem/s.per)
		}
	}
	if cu.glow > 0 { // struck: red
		s.red = float32(cu.glow) / 20
	}
	for k := cu.soldiers - 1; k >= 0; k-- {
		x, y, h := s.armySoldierAt(g, cu, k)
		a := alpha
		if k == cu.soldiers-1 {
			a *= part
		}
		s.drawSoldier(g, dst, s.soldierPic(cu, k), cu.side, x+dx, y, h, 0, a)
	}
	s.red = 0
	if cu.stamp != "" {
		drawStamp(dst, cu.stamp, army.x+army.w/2, army.y+army.h*0.45)
	}
	s.drawGauge(g, dst, cu, gauge)
	card.x += dx
	s.drawCardIn(g, dst, cu, card, s.layout == "below")
}

// drawGauge draws the troops: a bar in the side's colour with the red band of what was
// just lost shrinking after it, and the number beside it.
func (s *clashScene) drawGauge(g *Game, dst *ebiten.Image, cu *clashUnit, r box) {
	col := flagFill(factionColor(g, g.db.Game.Campaign.Player))
	if cu.side == 1 {
		col = flagFill(factionColor(g, s.cl.Owner))
		if s.cl.Owner == "" {
			col = color.NRGBA{0xc8, 0x50, 0x40, 0xff}
		}
	}
	rate := func(v float64) float64 {
		if cu.u.Max <= 0 {
			return 0
		}
		return math.Max(0, math.Min(1, v/cu.u.Max))
	}
	gfx.FillRoundRect(dst, float32(r.x), float32(r.y), float32(r.w), float32(r.h), float32(r.h/2), color.NRGBA{0, 0, 0, 0xb0})
	fill := func(share float64, c color.NRGBA) {
		if share <= 0 {
			return
		}
		w := math.Max(r.h-4, (r.w-4)*share)
		x := r.x + 2
		if cu.side == 1 { // the gauge empties toward the middle
			x = r.x + r.w - 2 - w
		}
		gfx.FillRoundRect(dst, float32(x), float32(r.y+2), float32(w), float32(r.h-4), float32(r.h/2-2), c)
	}
	fill(rate(cu.lag), color.NRGBA{0xd8, 0x30, 0x30, 0xff})
	fill(rate(cu.shown), col)
	n := strconv.Itoa(int(math.Ceil(cu.shown)))
	nx := r.x + r.w + armyNumW/2
	if cu.side == 1 {
		nx = r.x - armyNumW/2
	}
	outlined(dst, n, nx, r.y+r.h/2-sizeM*0.62, sizeM, plateInk, uint8(255*(1-cu.gone*0.4)))
}

// drawStamp draws the red stamp on a routed unit.
func drawStamp(dst *ebiten.Image, text string, cx, cy float64) {
	sw := gfx.TextWidth(text, sizeL*1.4) + 28
	h := sizeL*1.4 + 16
	bx, by := cx-sw/2, cy-h/2
	gfx.FillRoundRect(dst, float32(bx), float32(by), float32(sw), float32(h), 6, color.NRGBA{0xff, 0xf6, 0xe8, 0xe8})
	gfx.StrokeRoundRect(dst, float32(bx+3), float32(by+3), float32(sw-6), float32(h-6), 4, 3, color.NRGBA{0xc8, 0x20, 0x20, 0xff})
	gfx.DrawText(dst, text, cx, by+6, sizeL*1.4, color.NRGBA{0xc8, 0x20, 0x20, 0xff})
}
