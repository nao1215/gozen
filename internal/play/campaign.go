package play

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// A campaign's map (ADR 0030): the campaign's place shown as a map whose points named
// like regions are the regions. Each region stands a mark in its owner's colour with its
// garrison on its plate; a band over the map tells the turn, the player's faction and the
// actions; a window tells the region chosen, and the generals stand in a row with their
// troops. Choosing a region lists the orders that can be given it (not a walk: the token
// is not drawn), and the menu's list still has every order and "End turn".

// campaignMap reports whether the map is the campaign's place.
func (s *atlasScene) campaignMap(g *Game) bool {
	c := g.db.Game.Campaign
	return c != nil && c.Location == s.loc
}

// regionPoint reports whether the point is a region of the campaign's map.
func (s *atlasScene) regionPoint(g *Game, id string) bool {
	return s.campaignMap(g) && g.db.Regions[id] != nil
}

// campaignChoices are the regions of the map to choose, whatever the roads.
func (s *atlasScene) campaignChoices(g *Game) {
	s.choices = s.choices[:0]
	for i := range s.points {
		p := &s.points[i]
		p.choice = -1
		if p.shown == 0 || g.db.Regions[p.id] == nil {
			continue
		}
		p.choice = len(s.choices)
		s.choices = append(s.choices, atlas.Choice{Point: p.id})
	}
	if !s.warChosen { // at first, the first region the player can march on
		s.warChosen = true
		cs := g.st.Campaign()
		for i, c := range s.choices {
			if data.RegionOwner(g.db, cs, c.Point) != g.db.Game.Campaign.Player && data.Borders(g.db, cs, c.Point, g.db.Game.Campaign.Player) {
				s.sel = i
				break
			}
		}
	}
	s.sel = min(s.sel, max(0, len(s.choices)-1))
}

// regionOrders asks what to order the region: the orders whose target it can be now,
// then Cancel; an order taken asks for its general and gives it.
func (s *atlasScene) regionOrders(g *Game, id string) {
	g.sfx("decide")
	c := g.db.Game.Campaign
	var opts []data.ChoiceOption
	for _, oid := range c.OrderIDs() {
		o := c.Orders[oid]
		if o == nil || o.Target == "" || campaign.Open(g.db, g.st, o) != "" {
			continue
		}
		for _, t := range campaign.Targets(g.db, g.st, o) {
			if t == id {
				label := o.Label
				if label == "" {
					label = oid
				}
				opts = append(opts, data.ChoiceOption{Label: label, Do: []data.Command{{Order: &data.OrderCmd{ID: oid, Target: id}}}})
			}
		}
	}
	prompt := campaign.TargetLabel(g.db, g.st, id)
	if len(opts) == 0 {
		g.events.Start([]data.Command{{Say: &data.Say{Text: prompt + "\n" + lang.T("ここに出せる命令はない")}}})
		s.dirty = true
		return
	}
	opts = append(opts, data.ChoiceOption{Label: lang.T("やめる")})
	g.events.Start([]data.Command{{Prompt: prompt, Choice: opts}})
	s.dirty = true
	g.emit("region", traceLine{"location": s.loc, "region": id})
}

// factionColor is the colour of the faction holding the region (gray for none).
func factionColor(g *Game, faction string) color.NRGBA {
	if f := g.db.Factions[faction]; f != nil && f.Color.Valid {
		return f.Color.NRGBA
	}
	return color.NRGBA{0x9a, 0x9a, 0x9e, 0xff}
}

// drawRegionMarker draws a region's mark standing on (x, y), h tall: a flag in its owner's
// colour, a ring when it is a capital.
func (s *atlasScene) drawRegionMarker(g *Game, dst *ebiten.Image, p mapPoint, x, y, h float64) {
	cs := g.st.Campaign()
	owner := data.RegionOwner(g.db, cs, p.id)
	c := factionColor(g, owner)
	pole := float32(h)
	fx, fy := float32(x), float32(y)
	gfx.FillCircle(dst, fx, fy, pole*0.12, withAlpha(colorShadowNRGBA, 110), true)
	gfx.StrokeLine(dst, fx, fy, fx, fy-pole, 3, withAlpha(colorShadowNRGBA, 230), true)
	w, fh := pole*0.62, pole*0.42
	gfx.FillRoundRect(dst, fx+1, fy-pole-1, w+2, fh+2, 4, withAlpha(colorShadowNRGBA, 220))
	gfx.FillRoundRect(dst, fx+2, fy-pole, w, fh, 3, c)
	if f := g.db.Factions[owner]; f != nil && f.Capital == p.id {
		gfx.StrokeCircle(dst, fx, fy, pole*0.24, 3, withAlpha(c, 255), true)
	}
}

// regionLabel is a region's plate when the map gives none: its name and garrison.
func regionLabel(g *Game, id string) string {
	return fmt.Sprintf("%s  %d", data.RegionName(g.db, id), int(data.Garrison(g.db, g.st.Campaign(), id)))
}

// drawCampaign draws the band, the meters, the window of the region chosen and the
// generals over a campaign's map.
func (s *atlasScene) drawCampaign(g *Game, dst *ebiten.Image, sel string) {
	sw, sh := g.screen()
	c := g.db.Game.Campaign
	cs := g.st.Campaign()
	// the band: the turn, the player's faction, the actions
	band := fmt.Sprintf(lang.T("第%dターン　%s　行動 %d/%d"), data.Turn(g.db, cs), campaign.FactionName(g.db, c.Player), int(data.ActionsLeft(g.db, cs)), int(data.TurnActions(g.db, cs)))
	if c.Turns > 0 {
		band = fmt.Sprintf(lang.T("%s　（全 %d ターン）"), band, c.Turns)
	}
	bw := gfx.TextWidth(band, sizeM) + 120
	bx := (sw - bw) / 2
	g.window(dst, bx, 18, bw, 64)
	gfx.FillRoundRect(dst, float32(bx+22), float32(18+22), 20, 20, 4, factionColor(g, c.Player))
	g.text(dst, band, bx+58, 18+16, sizeM, g.th.text)
	// the meters at the top right, the region chosen at the top left
	g.drawMeters(dst, g.shownMeters("", false), sw-296, 96, 260)
	if sel != "" && g.db.Regions[sel] != nil {
		s.drawRegionWindow(g, dst, sel, 36, 96)
	}
	// the generals who have joined, with their troops
	s.drawGenerals(g, dst, sw, sh)
}

// drawRegionWindow is the window of a region: name, owner, garrison and values.
func (s *atlasScene) drawRegionWindow(g *Game, dst *ebiten.Image, id string, x, y float64) {
	cs := g.st.Campaign()
	owner := data.RegionOwner(g.db, cs, id)
	lines := []string{
		fmt.Sprintf(lang.T("持ち主　%s"), campaign.FactionName(g.db, owner)),
		fmt.Sprintf(lang.T("兵　%d"), int(data.Garrison(g.db, cs, id))),
	}
	for _, v := range data.ValueNames(g.db) {
		lines = append(lines, fmt.Sprintf("%s　%s", v, expr.Format(data.RegionValue(g.db, cs, id, v))))
	}
	h := 70 + float64(len(lines))*gfx.LineHeight(sizeS)
	g.window(dst, x, y, 300, h)
	gfx.FillRoundRect(dst, float32(x+22), float32(y+24), 18, 18, 4, factionColor(g, owner))
	g.text(dst, data.RegionName(g.db, id), x+50, y+16, sizeM, g.th.accent)
	ly := y + 56
	for _, l := range lines {
		g.text(dst, l, x+24, ly, sizeS, g.th.text)
		ly += gfx.LineHeight(sizeS)
	}
}

// drawGenerals stands the generals who have joined in a row at the bottom right: each
// face with its troops under it, dim when it has marched this turn.
func (s *atlasScene) drawGenerals(g *Game, dst *ebiten.Image, sw, sh float64) {
	cs := g.st.Campaign()
	var ids []string
	for _, id := range g.db.Ordered("actors") {
		if _, ok := g.db.Game.Campaign.Generals[id]; ok && g.st.Actors[id] != nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	const cell, face = 132.0, 92.0
	w := cell*float64(len(ids)) + 24
	x, y := sw-w-36, sh-cell-72
	g.window(dst, x, y, w, cell+30)
	for i, id := range ids {
		a := g.db.Actors[id]
		cx := x + 12 + cell*float64(i) + cell/2
		alpha := uint8(255)
		if data.Acted(g.db, cs, id) {
			alpha = 120
		}
		gfx.FillCircle(dst, float32(cx), float32(y+16+face/2), float32(face/2+3), withAlpha(g.th.accent, alpha), true)
		if img := g.dollFace(newDollSpec(g.db.FacePicture(a, a.Face, ""), a.Doll, 30)); img != nil {
			b := img.Bounds()
			op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
			sc := face * g.ui / float64(b.Dx())
			op.GeoM.Scale(sc, sc)
			op.GeoM.Translate((cx-face/2)*g.ui, (y+16)*g.ui)
			op.ColorScale.ScaleAlpha(float32(alpha) / 255)
			dst.DrawImage(img, op)
		}
		troops := fmt.Sprintf(lang.T("兵 %d"), int(data.GeneralTroops(g.db, cs, id)))
		gfx.DrawText(dst, troops, cx, y+24+face, sizeS*0.85, withAlpha(g.th.text, alpha))
	}
}
