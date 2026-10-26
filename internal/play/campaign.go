package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nao1215/gozen/internal/layer1/geom"

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
		s.dirty, s.asking = true, true
		return
	}
	opts = append(opts, data.ChoiceOption{Label: lang.T("やめる")})
	g.events.Start([]data.Command{{Prompt: prompt, Choice: opts}})
	s.dirty, s.asking = true, true
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
	if dst == nil {
		return
	}
	cs := g.st.Campaign()
	owner := data.RegionOwner(g.db, cs, p.id)
	c := factionColor(g, owner)
	pole := float32(h)
	fx, fy := float32(x), float32(y)
	gfx.FillCircle(dst, fx, fy, pole*0.12, withAlpha(colorShadowNRGBA, 110), true)
	gfx.StrokeLine(dst, fx, fy, fx, fy-pole, 3, withAlpha(colorShadowNRGBA, 230), true)
	w, fh := pole*0.62, pole*0.42
	gfx.FillRoundRect(dst, fx+1, fy-pole+2, w+3, fh+3, 4, withAlpha(colorShadowNRGBA, 160))
	gfx.FillRoundRect(dst, fx+2, fy-pole, w, fh, 3, flagFill(c))
	gfx.StrokeRoundRect(dst, fx+2, fy-pole, w, fh, 3, 2.5, plateInk) // a light edge: no colour sinks into the map
	if f := g.db.Factions[owner]; f != nil && f.Capital == p.id {
		gfx.StrokeCircle(dst, fx, fy, pole*0.24, 3, withAlpha(c, 255), true)
	}
}

// drawAreas fills each region's area (regions.<id>.area) in its owner's colour, faint
// for a free one, and draws its border; the region chosen is brighter, its border lit.
func (s *atlasScene) drawAreas(g *Game, dst *ebiten.Image, toScreen func(geom.Pt) (float32, float32), sel string) {
	u := float32(g.ui)
	cs := g.st.Campaign()
	for _, id := range data.RegionIDs(g.db) {
		r := g.db.Regions[id]
		if len(r.Area) < 3 {
			continue
		}
		var path vector.Path
		for i, p := range r.Area.Poly() {
			x, y := toScreen(p)
			if i == 0 {
				path.MoveTo(x*u, y*u)
			} else {
				path.LineTo(x*u, y*u)
			}
		}
		path.Close()
		owner := data.RegionOwner(g.db, cs, id)
		// a faint wash of the owner's colour so the picture shows through; no wash for a free region
		fill := withAlpha(flagFill(factionColor(g, owner)), 0x38)
		if owner == "" {
			fill.A = 0
		}
		if id == sel {
			fill.A += 0x28
			if owner == "" {
				fill = color.NRGBA{0xf4, 0xf0, 0xe4, 0x28}
			}
		}
		if fill.A > 0 {
			op := &vector.DrawPathOptions{AntiAlias: true}
			op.ColorScale.ScaleWithColor(fill)
			vector.FillPath(dst, &path, nil, op)
		}
		always := s.location(g).Map.RegionBorders == "always"
		if !always && id != sel {
			continue
		}
		line := &vector.DrawPathOptions{AntiAlias: true}
		width := float32(3)
		if id == sel {
			line.ColorScale.ScaleWithColor(g.th.cursor)
			width = 5
		} else {
			line.ColorScale.ScaleWithColor(withAlpha(plateInk, 210))
		}
		vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: width * u, LineJoin: vector.LineJoinRound}, line)
	}
}

// drawLandmark stands a region's landmark (regions.<id>.landmark) on its point (x, y),
// mapW the map picture's width on the screen: the picture with its foot on the point, the
// owner's flag at its right, darker and reddened with a little smoke while the region was
// fought over this turn or the last. It returns the box it covers (for the plates).
func (s *atlasScene) drawLandmark(g *Game, dst *ebiten.Image, p mapPoint, x, y, mapW float64) (plateBox, bool) {
	r := g.db.Regions[p.id]
	img := g.pics.get(r.Landmark)
	if img == nil || img.Bounds().Empty() {
		return plateBox{}, false
	}
	scale := r.LandmarkScale
	if scale <= 0 {
		scale = 0.12
	}
	b := img.Bounds()
	w := mapW * scale
	h := w * float64(b.Dy()) / float64(b.Dx())
	box := plateBox{x: x - w/2, y: y - h*0.85, w: w*0.92 + h*0.3, h: h}
	if dst == nil {
		return box, true
	}
	cs := g.st.Campaign()
	turn := float64(data.Turn(g.db, cs))
	f := data.Fought(cs, p.id)
	ravaged := f > 0 && f >= turn-1
	ground := 1.0
	if r.LandmarkGround != nil {
		ground = *r.LandmarkGround
	}
	if ground > 0 {
		img = s.settled(g, p.id, r.Landmark, img, x, y, mapW, ground)
		drawFootShadow(dst, x+w*0.04, y+h*0.15-w*0.02, w, ground)
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(w/float64(b.Dx()), h/float64(b.Dy()))
	op.GeoM.Translate(x-w/2, y-h*0.85)
	if ravaged {
		op.ColorScale.Scale(0.78, 0.6, 0.55, 1)
	}
	gfx.DrawImage(dst, img, op)
	if ravaged { // smoke rising from the roofs
		for i := range 3 {
			t := float64(g.tick+i*40) / 120
			ph := t - math.Floor(t)
			sx := x - w*0.2 + float64(i)*w*0.2 + math.Sin(t*3)*6
			sy := y - h*0.6 - ph*h*0.5
			gfx.FillCircle(dst, float32(sx), float32(sy), float32(8+ph*14), color.NRGBA{0x50, 0x48, 0x44, uint8(150 * (1 - ph))}, true)
		}
	}
	s.drawRegionMarker(g, dst, p, x+w*0.36, y+h*0.08, math.Max(36, h*0.45))
	return box, true
}

// flagFill is a flag's colour, lightened when it is dark (a navy or a gray would sink into
// a painted map).
func flagFill(c color.NRGBA) color.NRGBA {
	lum := (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
	if lum >= 0.45 {
		return c
	}
	k := (0.45 - lum) / 0.45 * 0.6 // toward white, more for darker
	mix := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*k) }
	return color.NRGBA{mix(c.R), mix(c.G), mix(c.B), c.A}
}

// regionLabel is a region's plate when the map gives none: its name and garrison.
func regionLabel(g *Game, id string) string {
	return fmt.Sprintf("%s  %d", data.RegionName(g.db, id), int(data.Garrison(g.db, g.st.Campaign(), id)))
}

// campaignBand is the band's text: the turn (campaign.heading, else the calendar's words),
// the player's faction, the actions left, and the last turn when the turns are counted by
// the engine's words.
func campaignBand(g *Game) string {
	c := g.db.Game.Campaign
	cs := g.st.Campaign()
	turn := data.Turn(g.db, cs)
	when := fmt.Sprintf(lang.T("第%dターン"), turn)
	if c.Calendar == "week" {
		when = fmt.Sprintf(lang.T("%d年目 第%d週"), (turn-1)/52+1, (turn-1)%52+1)
	}
	if c.Heading != "" {
		when = data.Render(g.db.Tr(c.Heading), g.st.Env())
	}
	band := when + "　" + campaign.FactionName(g.db, c.Player)
	if c.Count != "orders" {
		band += fmt.Sprintf(lang.T("　行動 %d/%d"), int(data.ActionsLeft(g.db, cs)), int(data.TurnActions(g.db, cs)))
	}
	if c.Turns > 0 && c.Heading == "" {
		band = fmt.Sprintf(lang.T("%s　（全 %d ターン）"), band, c.Turns)
	}
	return band
}

// drawCampaign draws the band, the meters, the window of the region chosen and the
// generals over a campaign's map.
func (s *atlasScene) drawCampaign(g *Game, dst *ebiten.Image, sel string) {
	sw, sh := g.screen()
	c := g.db.Game.Campaign
	// the band: the turn, the player's faction, the actions
	band := campaignBand(g)
	bw := gfx.TextWidth(band, sizeM) + 120
	bx := (sw - bw) / 2
	g.window(dst, bx, 18, bw, 64)
	gfx.FillRoundRect(dst, float32(bx+22), float32(18+22), 20, 20, 4, factionColor(g, c.Player))
	g.text(dst, band, bx+58, 18+16, sizeM, g.th.text)
	// the meters at the top right, the region chosen at the top left
	g.drawMeters(dst, g.shownMeters("", false), sw-296, 96, 260)
	if sel != "" && g.db.Regions[sel] != nil {
		s.drawSelectionRing(g, dst, sel)
		s.drawRegionWindow(g, dst, sel)
	}
	// the generals who have joined, with their troops
	s.drawGenerals(g, dst, sw, sh)
}

// regionLines are the lines of a region's window under its name: owner, garrison and values.
func regionLines(g *Game, id string) []string {
	cs := g.st.Campaign()
	owner := data.RegionOwner(g.db, cs, id)
	lines := []string{
		fmt.Sprintf(lang.T("持ち主　%s"), campaign.FactionName(g.db, owner)),
		fmt.Sprintf(lang.T("兵　%d"), int(data.Garrison(g.db, cs, id))),
	}
	for _, v := range data.ValueNames(g.db) {
		// a value only some regions hold is not shown as a zero on the others
		if !data.HasRegionValue(g.db, cs, id, v) {
			continue
		}
		label := v
		if cp := g.db.Game.Campaign; cp != nil && cp.Labels[v] != "" {
			label = cp.Labels[v]
		}
		lines = append(lines, fmt.Sprintf("%s　%s", label, expr.Format(data.RegionValue(g.db, cs, id, v))))
	}
	return lines
}

// regionWindowBox is where the window of the region id goes: at the top left, unless the
// region's own point lies under it; then at the right under the meters, else at the bottom
// left, whichever covers the point least. The window never hides the region it tells.
func (s *atlasScene) regionWindowBox(g *Game, id string) plateBox {
	sw, sh := g.screen()
	w, h := 300.0, 70+float64(len(regionLines(g, id)))*gfx.LineHeight(sizeS)
	i := s.pointIndex(id)
	if i < 0 {
		return plateBox{x: 36, y: 96, w: w, h: h}
	}
	px, py := s.screen(g, s.points[i].at)
	// the point with its flag and plate around it
	mark := plateBox{x: px - 80, y: py - 110, w: 160, h: 160}
	top := 96.0
	if mh := g.metersHeight(); mh > 0 {
		top += mh + 16
	}
	tries := []plateBox{
		{x: 36, y: 96, w: w, h: h},
		{x: sw - 36 - w, y: top, w: w, h: h},
		{x: 36, y: sh - 150 - h, w: w, h: h},
	}
	// the other windows over the map: the generals' and the camera's buttons
	var others []plateBox
	gen, hasGen := s.generalsRect(g)
	if hasGen {
		others = append(others, gen)
	}
	others = append(others, s.cameraButtons(g)...)
	best, least, found := tries[0], math.Inf(1), false
	for _, t := range tries {
		t.y = math.Max(4, math.Min(sh-4-t.h, t.y))
		if slices.ContainsFunc(others, func(o plateBox) bool { return t.overlap(o) > 0 }) {
			continue
		}
		if o := t.overlap(mark); o < least {
			best, least, found = t, o, true
		}
		if least == 0 {
			break
		}
	}
	if !found && hasGen { // none fits: the first, kept above the generals
		best.y = math.Max(4, math.Min(best.y, gen.y-8-best.h))
	}
	return best
}

// drawSelectionRing marks the region chosen on the map: a ring in the cursor's colour round
// its point, breathing slowly, so the window's region is the one the eye finds.
func (s *atlasScene) drawSelectionRing(g *Game, dst *ebiten.Image, id string) {
	i := s.pointIndex(id)
	if i < 0 || !s.cameraPointVisible(g, s.points[i].at) {
		return
	}
	px, py := s.screen(g, s.points[i].at)
	_, sh := g.screen()
	r := float32(markerHeight * sh * 0.55 * (1 + 0.08*math.Sin(float64(g.tick)/12)))
	gfx.StrokeCircle(dst, float32(px), float32(py), r+2, 6, withAlpha(colorShadowNRGBA, 150), true)
	gfx.StrokeCircle(dst, float32(px), float32(py), r, 4, g.th.cursor, true)
}

// drawRegionWindow is the window of a region: name, owner, garrison and values.
func (s *atlasScene) drawRegionWindow(g *Game, dst *ebiten.Image, id string) {
	b := s.regionWindowBox(g, id)
	x, y := b.x, b.y
	owner := data.RegionOwner(g.db, g.st.Campaign(), id)
	g.window(dst, x, y, b.w, b.h)
	gfx.FillRoundRect(dst, float32(x+22), float32(y+24), 18, 18, 4, factionColor(g, owner))
	g.text(dst, data.RegionName(g.db, id), x+50, y+16, sizeM, g.th.accent)
	ly := y + 56
	for _, l := range regionLines(g, id) {
		g.text(dst, l, x+24, ly, sizeS, g.th.text)
		ly += gfx.LineHeight(sizeS)
	}
}

// generalIDs are the generals who have joined, in the actors' order.
func generalIDs(g *Game) []string {
	var ids []string
	for _, id := range g.db.Ordered("actors") {
		if _, ok := g.db.Game.Campaign.Generals[id]; ok && g.st.Actors[id] != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// generalsRect is the generals' window on the map (false while none has joined).
func (s *atlasScene) generalsRect(g *Game) (plateBox, bool) {
	n := len(generalIDs(g))
	if n == 0 {
		return plateBox{}, false
	}
	sw, sh := g.screen()
	x, y, w, cell := s.generalsBox(g, n, sw, sh)
	return plateBox{x: x, y: y, w: w, h: cell + 30}, true
}

// generalsBox is the window of n generals at the bottom right: its left edge, top and
// width, and each one's cell. The cells narrow (132 at most, 72 at least) so the window
// keeps right of the map's camera buttons; too many for that, it stands over them.
func (s *atlasScene) generalsBox(g *Game, n int, sw, sh float64) (x, y, w, cell float64) {
	cell = 132.0
	left, bottom := 36.0, sh-42
	var buttons plateBox
	if bs := s.cameraButtons(g); len(bs) > 0 {
		buttons = bs[len(bs)-1]
		left = buttons.x + buttons.w + 16
	}
	room := sw - 36 - left - 24
	if cell*float64(n) > room {
		cell = room / float64(n)
	}
	if cell < 72 && buttons.w > 0 { // over the buttons and their hint, across the screen
		cell = math.Min(132, (sw-72-24)/float64(n))
		bottom = buttons.y - 44
	}
	cell = math.Max(48, cell)
	w = cell*float64(n) + 24
	return sw - w - 36, bottom - cell - 30, w, cell
}

// drawGenerals stands the generals who have joined in a row at the bottom right: each
// face with its troops under it, dim when it has marched this turn.
func (s *atlasScene) drawGenerals(g *Game, dst *ebiten.Image, sw, sh float64) {
	cs := g.st.Campaign()
	ids := generalIDs(g)
	if len(ids) == 0 {
		return
	}
	x, y, w, cell := s.generalsBox(g, len(ids), sw, sh)
	face := cell * 92 / 132
	g.window(dst, x, y, w, cell+30)
	for i, id := range ids {
		a := g.db.Actors[id]
		cx := x + 12 + cell*float64(i) + cell/2
		alpha := uint8(255)
		if data.Acted(g.db, cs, id) || data.Out(cs, id) > 0 {
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
		if out := data.Out(cs, id); out > 0 {
			troops = fmt.Sprintf(lang.T("離脱 あと%d"), out)
		}
		gfx.DrawText(dst, troops, cx, y+24+face, sizeS*0.85*math.Min(1, cell/120), withAlpha(g.th.text, alpha))
	}
}

// shadowBlob is a soft round shadow (opaque at the centre, clear at the rim), stretched
// into an ellipse under a landmark's foot.
var shadowBlob = func() *ebiten.Image {
	const n = 64
	pix := make([]byte, n*n*4)
	for yy := range n {
		for xx := range n {
			dx, dy := (float64(xx)+0.5-n/2)/(n/2), (float64(yy)+0.5-n/2)/(n/2)
			d := math.Sqrt(dx*dx + dy*dy)
			a := 1 - d
			if a < 0 {
				a = 0
			}
			a = a * a * (3 - 2*a) // smoothstep
			i := (yy*n + xx) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = 0, 0, 0, byte(a*255)
		}
	}
	img := ebiten.NewImage(n, n)
	img.WritePixels(pix)
	return img
}()

// drawFootShadow lays a soft ellipse shadow of a landmark w wide centred on (cx, cy), the
// map's light coming from the upper left so it leans a little to the lower right.
func drawFootShadow(dst *ebiten.Image, cx, cy, w, ground float64) {
	sw, sh := w*1.05, w*0.26
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(sw/64, sh/64)
	op.GeoM.Translate(cx-sw/2, cy-sh/2)
	op.ColorScale.ScaleAlpha(float32(0.55 * ground))
	gfx.DrawImage(dst, shadowBlob, op)
}

// settled returns the landmark's picture drawn into its place: its bottom edge faded out,
// its colours drawn toward the average colour of the terrain around (x, y), and its
// saturation eased by ground (0 to 1). It reads the map's pixels once and keeps the result.
func (s *atlasScene) settled(g *Game, id, file string, img *ebiten.Image, x, y, mapW, ground float64) *ebiten.Image {
	key := id + "|" + file + fmt.Sprintf("|%g", ground)
	if e, ok := s.ground[key]; ok {
		return e
	}
	if s.ground == nil {
		s.ground = map[string]*ebiten.Image{}
	}
	b := img.Bounds()
	pix := make([]byte, 4*b.Dx()*b.Dy())
	img.ReadPixels(pix)
	tr, tg, tb := 0.5, 0.5, 0.5
	if m := g.pics.get(s.pic); m != nil && m.Bounds().Dx() > 0 {
		// the map's pixels under the landmark: sample the ring of ground around its point
		scale := float64(m.Bounds().Dx()) / mapW
		cx, cy, rad := int(x*scale), int(y*scale), int(mapW*0.05*scale)
		r := image.Rect(cx-rad, cy-rad, cx+rad, cy+rad).Intersect(m.Bounds())
		if !r.Empty() {
			mp := make([]byte, 4*r.Dx()*r.Dy())
			if sub, ok := m.SubImage(r).(*ebiten.Image); ok {
				sub.ReadPixels(mp)
			} else {
				mp = nil
			}
			var sr, sg, sb float64
			for i := 0; i+3 < len(mp); i += 4 {
				sr += float64(mp[i])
				sg += float64(mp[i+1])
				sb += float64(mp[i+2])
			}
			n := float64(len(mp) / 4)
			tr, tg, tb = sr/n/255, sg/n/255, sb/n/255
		}
	}
	tl := 0.299*tr + 0.587*tg + 0.114*tb
	if tl < 0.05 {
		tl = 0.05
	}
	k := 0.4 * ground
	// the tint keeps the ground's hue; the brightness eases toward it, never below 0.8
	bright := math.Max(0.8, 1+(tl/0.5-1)*k)
	fade := int(float64(b.Dy()) * 0.07)
	for yy := range b.Dy() {
		fa := 1.0
		if d := b.Dy() - 1 - yy; d < fade {
			fa = float64(d) / float64(fade)
			fa = fa * fa * (3 - 2*fa)
			fa = 0.15 + 0.85*fa
		}
		for xx := range b.Dx() {
			i := (yy*b.Dx() + xx) * 4
			a := float64(pix[i+3]) / 255
			if a == 0 {
				continue
			}
			// the picture is premultiplied: work on straight colour
			r, gg, bb := float64(pix[i])/255/a, float64(pix[i+1])/255/a, float64(pix[i+2])/255/a
			l := 0.299*r + 0.587*gg + 0.114*bb
			sat := 1 - 0.35*ground // eased saturation
			r, gg, bb = l+(r-l)*sat, l+(gg-l)*sat, l+(bb-l)*sat
			r = r*(1-k) + r*(tr/tl)*k
			gg = gg*(1-k) + gg*(tg/tl)*k
			bb = bb*(1-k) + bb*(tb/tl)*k
			a *= fa
			pix[i] = clampByte(r * bright * a)
			pix[i+1] = clampByte(gg * bright * a)
			pix[i+2] = clampByte(bb * bright * a)
			pix[i+3] = clampByte(a)
		}
	}
	out := ebiten.NewImage(b.Dx(), b.Dy())
	out.WritePixels(pix)
	s.ground[key] = out
	return out
}

func clampByte(v float64) byte {
	return byte(math.Max(0, math.Min(1, v)) * 255)
}

// drawReach shows, for the region chosen, the neighbours it can march on (those held by
// another side): a thin, translucent arrow to each point, and a faint outline round each
// area. Nothing is drawn while no region is chosen.
func (s *atlasScene) drawReach(g *Game, dst *ebiten.Image, toScreen func(geom.Pt) (float32, float32), sel string) {
	r := g.db.Regions[sel]
	if r == nil {
		return
	}
	cs := g.st.Campaign()
	at := func(id string) (geom.Pt, bool) {
		if i := s.pointIndex(id); i >= 0 {
			return s.points[i].at, true
		}
		return geom.Pt{}, false
	}
	from, ok := at(sel)
	if !ok {
		return
	}
	u := float32(g.ui)
	owner := data.RegionOwner(g.db, cs, sel)
	for _, n := range r.Next {
		nr := g.db.Regions[n]
		if nr == nil || data.RegionOwner(g.db, cs, n) == owner {
			continue
		}
		to, ok := at(n)
		if !ok {
			continue
		}
		if len(nr.Area) >= 3 {
			var path vector.Path
			for i, p := range nr.Area.Poly() {
				x, y := toScreen(p)
				if i == 0 {
					path.MoveTo(x*u, y*u)
				} else {
					path.LineTo(x*u, y*u)
				}
			}
			path.Close()
			line := &vector.DrawPathOptions{AntiAlias: true}
			line.ColorScale.ScaleWithColor(withAlpha(plateInk, 120))
			vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: 2 * u, LineJoin: vector.LineJoinRound}, line)
		}
		x0, y0 := toScreen(from)
		x1, y1 := toScreen(to)
		dx, dy := float64(x1-x0), float64(y1-y0)
		d := math.Hypot(dx, dy)
		if d < 1 {
			continue
		}
		dx, dy = dx/d, dy/d
		// stop short of both points, so the arrow does not sit on the landmarks
		gap := math.Min(d*0.2, 40)
		ax, ay := float32(float64(x0)+dx*gap), float32(float64(y0)+dy*gap)
		bx, by := float32(float64(x1)-dx*gap), float32(float64(y1)-dy*gap)
		c := withAlpha(plateInk, 150)
		gfx.StrokeLine(dst, ax, ay, bx, by, 2, c, true)
		head := float64(14)
		for _, side := range []float64{0.45, -0.45} {
			c0, s0 := math.Cos(side), math.Sin(side)
			hx := -(dx*c0 - dy*s0) * head
			hy := -(dx*s0 + dy*c0) * head
			gfx.StrokeLine(dst, bx, by, bx+float32(hx), by+float32(hy), 2, c, true)
		}
	}
}
