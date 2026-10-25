package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/layer5/doll"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/stage"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
)

// A map (ADR 0022): one picture with the roads painted in it, and points the party goes
// between. The token stands on a point; choosing another point (arrow keys and OK, a click
// or a tap, the list) walks the token along the roads one at a time, each road's steps,
// battles and do running when the token reaches its end (the rules' cross and reach
// commands, in the order atlas.Go puts them). The screen draws a small marker on each
// point it shows and a quiet name plate; it draws the roads' lines only when the map asks
// (draw_roads), for the picture is meant to have them.

const (
	tokenSpeed   = 0.12  // of the picture's width a second
	tokenHeight  = 0.095 // the face's, of the screen's height, when the map does not say
	dollHeight   = 0.17  // the doll's (token: doll)
	markerHeight = 0.06  // of the screen's height, when the map does not say
	defaultLabel = 0.03  // the name plates' text, of the screen's height
	pointHit     = 40.0  // UI units around a point a click or a tap takes it (a finger)
)

// mapPoint is what the screen knows of a point, found when the state changes.
type mapPoint struct {
	id     string
	at     geom.Pt
	shown  int // 0 hidden, 1 a mark only, 2 dim, 3 visited (or the token's)
	open   bool
	label  string
	marker string
	choice int // its index in choices, -1 when it is not one
}

// mapRoad is what the screen knows of a road.
type mapRoad struct {
	line   geom.Polyline
	shown  bool
	usable bool
}

type atlasScene struct {
	loc     string
	pic     string
	age     int
	banner  int
	sel     int
	choices []atlas.Choice
	points  []mapPoint
	roads   []mapRoad
	here    string
	dirty   bool     // the state may have changed: find the points again when idle
	leader  dollSpec // the token's doll when the map has no token picture
	spot    string   // the spot last written to the trace
	boxes   []plateBox
	targets []pointTarget
	ground  map[string]*ebiten.Image // landmarks settled onto their terrain, by region
	// warChosen: a campaign's map has chosen its first region
	warChosen bool

	// the walk along the roads: the commands still to run, and the road being walked
	queue  []data.Command
	path   geom.Polyline
	walked float64
	at     geom.Pt // the token, in the picture's pixels
	dir    geom.Pt
	cross  *data.Command // run when the token reaches the road's end
	phase  float64
}

func newAtlasScene(g *Game) *atlasScene {
	s := &atlasScene{}
	s.arrive(g)
	return s
}

func (s *atlasScene) location(g *Game) *data.Location { return g.db.Locations[s.loc] }

// arrive sets the map up for the party's location, the token on its point.
func (s *atlasScene) arrive(g *Game) {
	s.loc = g.st.Location
	l := s.location(g)
	s.pic = l.Map.Picture
	s.age, s.banner, s.queue, s.path, s.cross = 0, 240, nil, nil, nil
	s.leader = g.leaderDoll()
	s.leader = newDollSpec(s.leader.picture, s.leader.doll, 30)
	s.dir = geom.Pt{Y: 1}
	atlas.Visit(g.db, g.st, s.loc, atlas.Here(g.db, g.st)) // standing there is having been there
	s.warChosen = false
	s.refresh(g)
	if !s.campaignMap(g) {
		s.sel = s.firstChoice()
	}
	if p := l.Map.Points[s.here]; p != nil {
		s.at = p.At.Pt()
	}
}

// firstChoice is the first choice that is not the point the token stands on.
func (s *atlasScene) firstChoice() int {
	for i, c := range s.choices {
		if c.Point != s.here {
			return i
		}
	}
	return 0
}

// refresh finds the choices and what each point and road shows (after the state changed):
// the hidden points are neither drawn nor chosen.
func (s *atlasScene) refresh(g *Game) {
	a := s.location(g).Map
	s.here = atlas.Here(g.db, g.st)
	if s.here != s.spot {
		s.spot = s.here
		g.emit("spot", traceLine{"location": s.loc, "spot": s.here})
	}
	ids := a.PointIDs()
	all := atlas.Choices(g.db, g.st)
	reach := map[string]bool{}
	for _, c := range all {
		reach[c.Point] = true
	}
	s.points = s.points[:0]
	index := map[string]int{}
	for pointIndex, id := range ids {
		p := a.Points[id]
		mp := mapPoint{id: id, at: p.At.Pt(), shown: s.shownOf(g, id, pointIndex), open: atlas.Open(g.st, p), label: s.labelOf(g, id), marker: p.Marker, choice: -1}
		if mp.shown == 0 && reach[id] { // hidden until the roads reach it
			mp.shown = 1
		}
		index[id] = len(s.points)
		s.points = append(s.points, mp)
	}
	s.choices = s.choices[:0]
	for _, c := range all {
		if s.points[index[c.Point]].shown == 0 {
			continue
		}
		s.points[index[c.Point]].choice = len(s.choices)
		s.choices = append(s.choices, c)
	}
	if s.campaignMap(g) {
		s.campaignChoices(g)
	} else {
		s.sel = min(s.sel, max(0, len(s.choices)-1))
	}
	s.roads = s.roads[:0]
	for i := range a.Roads {
		r := &a.Roads[i]
		mr := mapRoad{}
		if len(r.Between) == 2 && a.Points[r.Between[0]] != nil && a.Points[r.Between[1]] != nil {
			mr.line = roadLine(a, r, r.Between[0])
			mr.shown = s.points[index[r.Between[0]]].shown > 0 && s.points[index[r.Between[1]]].shown > 0
			mr.usable = atlas.Usable(g.st, a, r)
		}
		s.roads = append(s.roads, mr)
	}
	if p := a.Points[s.here]; p != nil && s.path == nil {
		s.at = p.At.Pt()
	}
	s.dirty = false
}

// fit is how the picture lies on the screen (pixels): the scale and the offset.
func (s *atlasScene) fit(g *Game) (k, ox, oy float64, ok bool) {
	img := g.pics.get(s.pic)
	if img == nil {
		return 0, 0, 0, false
	}
	pw, ph := g.pixels()
	b := img.Bounds()
	k = math.Min(pw/float64(b.Dx()), ph/float64(b.Dy()))
	return k, (pw - float64(b.Dx())*k) / 2, (ph - float64(b.Dy())*k) / 2, true
}

// screen is where a point of the picture shows, in UI units.
func (s *atlasScene) screen(g *Game, p geom.Pt) (float64, float64) {
	k, ox, oy, _ := s.fit(g)
	return (ox + p.X*k) / g.ui, (oy + p.Y*k) / g.ui
}

// walking reports whether the token is on its way (a road, or commands still to run).
func (s *atlasScene) walking() bool { return s.path != nil || len(s.queue) > 0 }

func (s *atlasScene) update(g *Game) {
	s.age++
	s.banner = max(0, s.banner-1)
	if s.path != nil {
		s.walk(g)
		return
	}
	if g.pump() {
		s.dirty = true
		return
	}
	if g.enterAt != "" {
		loc := g.enterAt
		g.enterAt = ""
		if g.st.Location == loc {
			cmds := g.startArrival(true)
			s.dirty = true
			if len(cmds) > 0 {
				return
			}
		}
	}
	if cmds := world.Auto(g.db, g.st); cmds != nil {
		g.events.Start(cmds)
		s.dirty = true
		return
	}
	if len(s.queue) > 0 { // the next road, or the arrival
		c := s.queue[0]
		s.queue = s.queue[1:]
		s.dirty = true
		if c.Cross != "" && s.startRoad(g, c) {
			return
		}
		g.events.Start([]data.Command{c})
		return
	}
	if s.dirty || s.age%60 == 0 { // the state changed (or a while passed): look again
		s.refresh(g)
		if s.sel < len(s.choices) && s.choices[s.sel].Point == s.here && !s.campaignMap(g) {
			s.sel = s.firstChoice()
		}
	}
	in := &g.in
	if in.openList() {
		g.sfx("decide")
		g.push(newEntryListOf(g, s.take, s.listed))
		return
	}
	if in.click {
		if c, ok := s.pointAt(g, float64(in.mouse.X), float64(in.mouse.Y)); ok {
			s.goTo(g, c)
		}
		return
	}
	if len(s.choices) == 0 {
		return
	}
	var want geom.Pt
	switch {
	case in.up():
		want = geom.Pt{Y: -1}
	case in.down():
		want = geom.Pt{Y: 1}
	case in.left():
		want = geom.Pt{X: -1}
	case in.right():
		want = geom.Pt{X: 1}
	}
	if want != (geom.Pt{}) {
		s.selectToward(g, want)
		return
	}
	if in.ok() {
		s.goTo(g, s.choices[s.sel])
	}
}

// listed reports whether an entry goes in the list (a hidden point does not).
func (s *atlasScene) listed(e world.Entry) bool {
	if e.Choice == nil {
		return true
	}
	for _, p := range s.points {
		if p.id == e.Choice.Point {
			return p.shown > 0
		}
	}
	return true
}

// selectToward selects the choice whose way out of the token's point is nearest the way
// (another than the one selected when the way is the same).
func (s *atlasScene) selectToward(g *Game, want geom.Pt) {
	best, bd := s.sel, 0.3
	for i, c := range s.choices {
		d := s.firstStep(g, c).Sub(s.at)
		if i == s.sel || d.Len() == 0 {
			continue
		}
		if dot := (d.X*want.X + d.Y*want.Y) / d.Len(); dot > bd {
			best, bd = i, dot
		}
	}
	if best != s.sel {
		g.sfx("cursor")
		s.sel = best
	}
}

// firstStep is where the way to a choice first heads (its first road's first bend).
func (s *atlasScene) firstStep(g *Game, c atlas.Choice) geom.Pt {
	a := s.location(g).Map
	if len(c.Roads) == 0 {
		return a.Points[c.Point].At.Pt()
	}
	line := roadLine(a, &a.Roads[c.Roads[0]], s.here)
	if len(line) > 1 {
		return line[1]
	}
	return a.Points[c.Point].At.Pt()
}

// pointAt prefers the last visible marker or plate drawn, then the nearest anchor
// within a finger's reach. Layout is recomputed so input never uses stale bounds.
func (s *atlasScene) pointAt(g *Game, x, y float64) (atlas.Choice, bool) {
	s.layoutPoints(g, nil)
	for i := len(s.targets) - 1; i >= 0; i-- {
		target := s.targets[i]
		b := target.box
		if x >= b.x && x <= b.x+b.w && y >= b.y && y <= b.y+b.h {
			return s.choices[target.choice], true
		}
	}
	best, bd := -1, pointHit
	for _, p := range s.points {
		if p.choice < 0 || p.shown == 0 {
			continue
		}
		px, py := s.screen(g, p.at)
		if d := math.Hypot(px-x, py-y); d < bd {
			best, bd = p.choice, d
		}
	}
	if best < 0 && s.campaignMap(g) { // inside a region's area
		for _, p := range s.points {
			r := g.db.Regions[p.id]
			if p.choice < 0 || p.shown == 0 || r == nil || len(r.Area) < 3 {
				continue
			}
			var q geom.Poly
			for _, a := range r.Area.Poly() {
				ax, ay := s.screen(g, a)
				q = append(q, geom.Pt{X: ax, Y: ay})
			}
			if q.Inside(geom.Pt{X: x, Y: y}) {
				best = p.choice
				break
			}
		}
	}
	if best < 0 {
		return atlas.Choice{}, false
	}
	return s.choices[best], true
}

// take runs an entry of the list: a point of the map walks there, the rest as the menu.
func (s *atlasScene) take(g *Game, e world.Entry) {
	switch {
	case e.Kind == world.Menu:
		g.push(newMenu(g))
	case e.Choice != nil:
		s.goTo(g, *e.Choice)
	default:
		g.sfx("decide")
		g.events.Start(e.Commands(g.db, g.st))
		s.dirty = true
	}
}

// go_ sets off to a choice: its commands run one at a time, a road's after the token has
// walked it.
func (s *atlasScene) goTo(g *Game, c atlas.Choice) {
	if s.regionPoint(g, c.Point) {
		s.regionOrders(g, c.Point)
		return
	}
	g.sfx("decide")
	e := world.Entry{Kind: world.Exit, Choice: &c, Spot: c.Point}
	s.queue = e.Commands(g.db, g.st)
	s.dirty = true
	g.emit("travel", traceLine{"location": s.loc, "from": s.here, "to": c.Point, "roads": c.Roads})
}

// startRoad sets the token walking the road of a cross command (false when it has none).
func (s *atlasScene) startRoad(g *Game, c data.Command) bool {
	a := s.location(g).Map
	is, _, _ := strings.Cut(c.Cross, ">")
	i, err := strconv.Atoi(is)
	if err != nil || i < 0 || i >= len(a.Roads) {
		return false
	}
	s.path = roadLine(a, &a.Roads[i], atlas.Here(g.db, g.st))
	s.walked, s.cross = 0, &c
	g.sfx("step")
	return len(s.path) > 1
}

// walk moves the token along the road; at its end the road's command runs.
func (s *atlasScene) walk(g *Game) {
	img := g.pics.get(s.pic)
	rate := s.location(g).Map.TravelSpeed
	if rate == 0 {
		rate = tokenSpeed
	}
	speed := rate * tickSecond * 1000
	if img != nil {
		speed = rate * tickSecond * float64(img.Bounds().Dx())
	}
	s.walked += speed
	s.phase = math.Mod(s.phase+tickSecond*7, 2*math.Pi)
	at, dir := s.path.At(s.walked)
	s.at = at
	if dir != (geom.Pt{}) {
		s.dir = dir
	}
	if s.walked < s.path.Length() {
		return
	}
	s.path = nil
	if c := s.cross; c != nil {
		s.cross = nil
		g.events.Start([]data.Command{*c})
	}
}

// roadLine is a road's line from the point from (the via points in their order or back).
func roadLine(a *data.Atlas, r *data.Road, from string) geom.Polyline {
	if len(r.Between) != 2 || a.Points[r.Between[0]] == nil || a.Points[r.Between[1]] == nil {
		return nil
	}
	line := geom.Polyline{a.Points[r.Between[0]].At.Pt()}
	for _, v := range r.Via {
		line = append(line, v.Pt())
	}
	line = append(line, a.Points[r.Between[1]].At.Pt())
	if r.Between[1] == from {
		for i, j := 0, len(line)-1; i < j; i, j = i+1, j-1 {
			line[i], line[j] = line[j], line[i]
		}
	}
	return line
}

// shownOf is how a point shows: 0 hidden, 1 a mark only, 2 dim, 3 visited (or the token's).
// A hidden point shows as a mark once the open roads reach it (refresh).
func (s *atlasScene) shownOf(g *Game, id string, pointIndex int) int {
	mapped := g.st.Mapped[s.loc]
	if id == s.here || pointIndex >= 0 && pointIndex < len(mapped) && mapped[pointIndex] != 0 {
		return 3
	}
	switch s.location(g).Map.Unknown {
	case "hidden":
		return 0
	case "dim":
		return 2
	case "named":
		return 3
	}
	return 1
}

// labelOf is a point's name: its label, else the name of where it leads, else "".
func (s *atlasScene) labelOf(g *Game, id string) string {
	p := s.location(g).Map.Points[id]
	switch {
	case p.Label != "":
		return data.Render(p.Label, g.st.Env())
	case p.To != "" && g.db.Locations[p.To] != nil:
		return data.Render(g.db.Locations[p.To].Name, g.st.Env())
	case s.regionPoint(g, id):
		return regionLabel(g, id)
	}
	return ""
}

var (
	pinRed    = color.NRGBA{0xc4, 0x4a, 0x3a, 0xff}
	plateBack = color.NRGBA{0x1c, 0x22, 0x2a, 0xff}
	pinGray   = color.NRGBA{0x80, 0x80, 0x84, 0xff}
	plateInk  = color.NRGBA{0xf4, 0xef, 0xe2, 0xff}
)

func (s *atlasScene) draw(g *Game, dst *ebiten.Image) {
	k, ox, oy, ok := s.fit(g)
	if !ok {
		return
	}
	l := s.location(g)
	a := l.Map
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(k, k)
	op.GeoM.Translate(ox, oy)
	dst.DrawImage(g.pics.get(s.pic), op)
	u := g.ui
	toScreen := func(p geom.Pt) (float32, float32) {
		return float32((ox + p.X*k) / u), float32((oy + p.Y*k) / u)
	}
	if s.campaignMap(g) {
		sel := ""
		if !s.walking() && s.sel < len(s.choices) && !g.busy() {
			sel = s.choices[s.sel].Point
		}
		s.drawAreas(g, dst, toScreen, sel)
	}
	sel0 := ""
	if !s.walking() && s.sel < len(s.choices) && !g.busy() {
		sel0 = s.choices[s.sel].Point
	}
	if s.campaignMap(g) && a.RegionRoads != "always" {
		s.drawReach(g, dst, toScreen, sel0)
	}
	if s.campaignMap(g) && a.RegionRoads == "always" || !s.campaignMap(g) && a.DrawRoads { // only when asked: the roads are painted in the picture
		for _, r := range s.roads {
			if !r.shown {
				continue
			}
			c, w := withAlpha(plateInk, 210), float32(4)
			if !r.usable {
				c, w = withAlpha(colorShadowNRGBA, 110), 2
			}
			for j := 1; j < len(r.line); j++ {
				x0, y0 := toScreen(r.line[j-1])
				x1, y1 := toScreen(r.line[j])
				gfx.StrokeLine(dst, x0, y0, x1, y1, w, c, true)
			}
		}
	}
	s.layoutPoints(g, dst)
	sel := ""
	if !s.walking() && s.sel < len(s.choices) && !g.busy() {
		sel = s.choices[s.sel].Point
	}
	_, sh := g.screen()
	war := s.campaignMap(g)
	if !war {
		s.drawToken(g, dst, k, ox, oy)
	}
	if g.talk.active || len(g.stack) > 1 {
		return
	}
	if war {
		s.drawCampaign(g, dst, sel)
		g.text(dst, lang.T("方向：選ぶ　決定：命令　メニュー：一覧"), 36, sh-40, sizeS*0.8, withAlpha(g.th.text, 200))
		return
	}
	if s.banner > 0 {
		name := data.Render(l.Name, g.st.Env())
		al := math.Min(1, float64(s.banner)/30)
		w := math.Max(220, gfx.TextWidth(name, sizeL)+60)
		gfx.FillRoundRect(dst, 36, 28, float32(w), 60, 14, withAlpha(g.th.window, uint8(float64(g.th.window.A)*al)))
		g.text(dst, name, 64, 40, sizeL, withAlpha(g.th.text, uint8(255*al)))
	}
	g.text(dst, lang.T("方向：選ぶ　決定：進む　メニュー：一覧"), 36, sh-40, sizeS*0.8, withAlpha(g.th.text, 200))
}

// layoutPoints shares marker and plate geometry between drawing and pointer input.
// A nil destination performs layout only, including label collision avoidance.
func (s *atlasScene) layoutPoints(g *Game, dst *ebiten.Image) {
	a := s.location(g).Map
	k, ox, oy, _ := s.fit(g)
	u := g.ui
	toScreen := func(p geom.Pt) (float32, float32) { return float32((ox + p.X*k) / u), float32((oy + p.Y*k) / u) }
	sel := ""
	if !s.walking() && s.sel < len(s.choices) && !g.busy() {
		sel = s.choices[s.sel].Point
	}
	_, sh := g.screen()
	mh := markerHeight * sh
	if a.Marker != nil && a.Marker.Height > 0 {
		mh = a.Marker.Height * sh
	}
	// the markers, then the name plates laid out so they keep off each other and the markers
	s.boxes = s.boxes[:0]
	s.targets = s.targets[:0]
	war := s.campaignMap(g)
	for i := range s.points { // a region's plate follows its garrison at once
		if war && g.db.Regions[s.points[i].id] != nil && s.location(g).Map.Points[s.points[i].id].Label == "" {
			s.points[i].label = regionLabel(g, s.points[i].id)
		}
	}
	for _, p := range s.points {
		x, y := toScreen(p.at)
		if p.id == s.here && s.path == nil && !war { // the token stands on its own point: keep plates off it
			th := tokenSize(a) * sh
			if a.Token == nil || a.Token.Picture == "" && !a.Token.Doll { // the round face, its ring too
				s.boxes = append(s.boxes, plateBox{x: float64(x) - th*0.55, y: float64(y) - th*1.2, w: th * 1.1, h: th * 1.36})
			} else {
				s.boxes = append(s.boxes, plateBox{x: float64(x) - th*0.25, y: float64(y) - th*1.2, w: th * 0.5, h: th * 1.46})
			}
			continue
		}
		if p.shown == 0 {
			continue
		}
		h := mh
		if p.id == sel {
			h *= 1.3
		}
		if war && g.db.Regions[p.id] != nil && g.db.Regions[p.id].Landmark != "" {
			mw := float64(g.pics.get(s.pic).Bounds().Dx()) * k / u
			if box, ok := s.drawLandmark(g, dst, p, float64(x), float64(y), mw); ok {
				s.addPointBox(p, box)
				continue
			}
		}
		if war && g.db.Regions[p.id] != nil && len(g.db.Regions[p.id].Area) >= 3 { // the area tells the owner: only the plate
			continue
		}
		if war && g.db.Regions[p.id] != nil {
			s.drawRegionMarker(g, dst, p, float64(x), float64(y), h)
		} else {
			s.drawMarker(g, dst, p, float64(x), float64(y), h, pointAlpha(p))
		}
		if war && g.db.Regions[p.id] != nil { // a flag reaches to the right of its pole
			s.addPointBox(p, plateBox{x: float64(x) - h*0.24 - 2, y: float64(y) - h - 2, w: h*.86 + 6, h: h*1.24 + 4})
		} else {
			s.addPointBox(p, s.markerBox(g, p, float64(x), float64(y), h))
		}
	}
	if i := s.pointIndex(sel); i >= 0 && s.points[i].shown > 0 { // the chosen plate first: it keeps its place
		s.drawPlate(g, dst, s.points[i], sel, mh)
	}
	for _, p := range s.points {
		if p.shown > 0 && p.id != sel {
			s.drawPlate(g, dst, p, sel, mh)
		}
	}
}

type pointTarget struct {
	box    plateBox
	choice int
}

func (s *atlasScene) addPointBox(p mapPoint, box plateBox) {
	s.boxes = append(s.boxes, box)
	if p.shown > 0 && p.choice >= 0 {
		s.targets = append(s.targets, pointTarget{box: box, choice: p.choice})
	}
}

// markerBox uses the same aspect ratio and bottom anchor as drawMarker.
func (s *atlasScene) markerBox(g *Game, p mapPoint, x, y, h float64) plateBox {
	pic := p.marker
	if pic == "" && s.location(g).Map.Marker != nil {
		pic = s.location(g).Map.Marker.Picture
	}
	if pic != "" {
		b := g.pics.get(pic).Bounds()
		w := h * float64(b.Dx()) / float64(b.Dy())
		return plateBox{x: x - w/2, y: y - h, w: w, h: h}
	}
	r := h*.28 + 1.5
	return plateBox{x: x - r, y: y - h - 1.5, w: r * 2, h: h + 3}
}

// labelSize is the text height of the name plates in UI units: the map's label_size (a
// fraction of the screen's height), 0.03 when not given.
func (s *atlasScene) labelSize(g *Game) float64 {
	_, sh := g.screen()
	k := s.location(g).Map.LabelSize
	if k <= 0 {
		k = defaultLabel
	}
	return sh * k
}

func pointAlpha(p mapPoint) float32 {
	if p.shown == 2 {
		return 0.45
	}
	return 1
}

func (s *atlasScene) pointIndex(id string) int {
	for i, p := range s.points {
		if p.id == id && id != "" {
			return i
		}
	}
	return -1
}

// plateBox is a box taken on the screen (a plate or a marker, UI units).
type plateBox struct{ x, y, w, h float64 }

// overlap is the area two boxes share.
func (b plateBox) overlap(o plateBox) float64 {
	w := math.Min(b.x+b.w, o.x+o.w) - math.Max(b.x, o.x)
	h := math.Min(b.y+b.h, o.y+o.h) - math.Max(b.y, o.y)
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// drawPlate draws a point's name plate (the visited points and the one chosen; a point not
// visited shows "?" when the map marks them): dark, framed and lifted by a soft shadow, the
// chosen one in the window's color and larger. It goes under its point, else above its
// marker, else beside it, where it keeps off the plates and markers already placed.
func (s *atlasScene) drawPlate(g *Game, dst *ebiten.Image, p mapPoint, sel string, mh float64) {
	name := p.label
	if p.shown == 1 && p.id != sel {
		name = "?"
	}
	if name == "" || p.shown == 2 && p.id != sel {
		return
	}
	alpha := pointAlpha(p)
	size := s.labelSize(g)
	if p.id == sel {
		size *= 1.15
	}
	px, py := s.screen(g, p.at)
	w, h := gfx.TextWidth(name, size)+size*1.1, size*1.55
	under := px - w/2
	tries := []plateBox{
		{under, py + 6, w, h},                      // under the point
		{under, py - mh*1.3 - h - 6, w, h},         // over the marker
		{px + mh*0.5, py - mh*0.6 - h/2, w, h},     // to the right
		{px - mh*0.5 - w, py - mh*0.6 - h/2, w, h}, // to the left
		{under + w*0.6, py + 6, w, h},              // under, to one side or the other
		{under - w*0.6, py + 6, w, h},
		{under, py + 12 + h, w, h}, // further under
	}
	sw, sh := g.screen()
	// the first place that takes nothing already placed, else the one that takes the least
	box, least := tries[0], math.Inf(1)
	for _, t := range tries {
		t.x, t.y = math.Max(4, math.Min(sw-4-t.w, t.x)), math.Max(4, math.Min(sh-4-t.h, t.y))
		taken := 0.0
		for _, o := range s.boxes {
			taken += t.overlap(o)
		}
		if taken < least {
			box, least = t, taken
		}
		if taken == 0 {
			break
		}
	}
	s.addPointBox(p, box)
	if dst == nil {
		return
	}
	bg, edge := withAlpha(plateBack, uint8(235*alpha)), withAlpha(g.th.border, uint8(200*alpha))
	if p.id == sel {
		bg, edge = withAlpha(g.th.window, 245), withAlpha(g.th.accent, 255)
	}
	r := float32(h * 0.35)
	gfx.FillRoundRect(dst, float32(box.x+2), float32(box.y+4), float32(box.w), float32(box.h), r, withAlpha(colorShadowNRGBA, uint8(110*alpha)))
	gfx.FillRoundRect(dst, float32(box.x), float32(box.y), float32(box.w), float32(box.h), r, bg)
	gfx.StrokeRoundRect(dst, float32(box.x+1), float32(box.y+1), float32(box.w-2), float32(box.h-2), r, 2, edge)
	gfx.DrawText(dst, name, box.x+box.w/2, box.y+(box.h-size)/2-size*0.1, size, withAlpha(plateInk, uint8(255*alpha)))
}

// drawMarker draws a point's marker standing on (x, y) (UI units), h tall: its picture, the
// map's, or a small pin (gray when the point is closed).
func (s *atlasScene) drawMarker(g *Game, dst *ebiten.Image, p mapPoint, x, y, h float64, alpha float32) {
	if dst == nil {
		return
	}
	a := s.location(g).Map
	pic := p.marker
	if pic == "" && a.Marker != nil {
		pic = a.Marker.Picture
	}
	if pic != "" {
		img := g.pics.get(pic)
		b := img.Bounds()
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		sc := h / float64(b.Dy())
		op.GeoM.Scale(sc, sc)
		box := s.markerBox(g, p, x, y, h)
		op.GeoM.Translate(box.x, box.y)
		if !p.open {
			op.ColorScale.Scale(0.55, 0.55, 0.58, 1)
		}
		op.ColorScale.ScaleAlpha(alpha)
		gfx.DrawImage(dst, img, op)
		return
	}
	c := pinRed
	if !p.open {
		c = pinGray
	}
	r := float32(h * 0.28)
	hx, hy := float32(x), float32(y-h+float64(r))
	gfx.FillCircle(dst, float32(x), float32(y), r*0.45, withAlpha(colorShadowNRGBA, uint8(90*alpha)), true) // its foot
	gfx.StrokeLine(dst, float32(x), float32(y), hx, hy, 2.5, withAlpha(colorShadowNRGBA, uint8(230*alpha)), true)
	gfx.FillCircle(dst, hx, hy, r+1.5, withAlpha(colorShadowNRGBA, uint8(220*alpha)), true)
	gfx.FillCircle(dst, hx, hy, r, withAlpha(c, uint8(255*alpha)), true)
	gfx.FillCircle(dst, hx-r*0.3, hy-r*0.3, r*0.3, withAlpha(plateInk, uint8(200*alpha)), true)
}

// tokenSize is the token's height in fractions of the screen's height.
func tokenSize(a *data.Atlas) float64 {
	switch {
	case a.Token != nil && a.Token.Height > 0:
		return a.Token.Height
	case a.Token != nil && a.Token.Doll:
		return dollHeight
	}
	return tokenHeight
}

// drawToken draws the party's token on the map: the map's token picture, else the
// leader's round face sliding along the roads (bobbing a little), else with token: doll
// the leader's doll walking. A dark shadow and a small ring mark where the party is.
func (s *atlasScene) drawToken(g *Game, dst *ebiten.Image, k, ox, oy float64) {
	a := s.location(g).Map
	_, ph := g.pixels()
	h := tokenSize(a) * ph
	x, y := ox+s.at.X*k, oy+s.at.Y*k
	ux, uy, uh := float32(x/g.ui), float32(y/g.ui), float32(h/g.ui)
	face := a.Token == nil || a.Token.Picture == "" && !a.Token.Doll
	if face {
		ellipse(g, dst, ux, uy, uh*0.34, uh*0.12, 0, withAlpha(colorShadowNRGBA, 170))
		ellipse(g, dst, ux, uy, uh*0.4, uh*0.15, 2.5, withAlpha(g.th.accent, 230))
		s.drawFace(g, dst, x, y, h)
		return
	}
	defer s.drawFinder(g, dst, x/g.ui, y/g.ui, h/g.ui)
	// a dark shadow and a ring at the token's feet (where the party is), even as it walks
	gfx.FillCircle(dst, ux, uy, uh*0.2, withAlpha(colorShadowNRGBA, 170), true)
	gfx.StrokeCircle(dst, ux, uy, uh*0.24, 3, withAlpha(g.th.accent, 230), true)
	if a.Token.Picture != "" {
		img := g.pics.get(a.Token.Picture)
		b := img.Bounds()
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		sc := h / float64(b.Dy())
		op.GeoM.Scale(sc, sc)
		op.GeoM.Translate(x-float64(b.Dx())*sc/2, y-h)
		dst.DrawImage(img, op)
		return
	}
	sheet := g.dollSheet(s.leader)
	if sheet == nil {
		return
	}
	side := doll.Side(s.dir.X, s.dir.Y)
	pose := doll.Pose(s.path != nil, s.phase)
	sub, ok := sheet.SubImage(image.Rect(side*stage.SheetW, pose*stage.SheetH, (side+1)*stage.SheetW, (pose+1)*stage.SheetH)).(*ebiten.Image)
	if !ok {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	sc := h / stage.SheetH * 1.12
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(x-stage.SheetW*sc/2, y-stage.SheetH*sc)
	dst.DrawImage(sub, op)
}

// ellipse fills (width 0) or strokes a flat ellipse centered on (x, y), UI units.
func ellipse(g *Game, dst *ebiten.Image, x, y, rx, ry, width float32, c color.NRGBA) {
	u := float32(g.ui)
	var path vector.Path
	for i := range 48 {
		t := float64(i) / 48 * 2 * math.Pi
		px, py := (x+rx*float32(math.Cos(t)))*u, (y+ry*float32(math.Sin(t)))*u
		if i == 0 {
			path.MoveTo(px, py)
		} else {
			path.LineTo(px, py)
		}
	}
	path.Close()
	op := &vector.DrawPathOptions{AntiAlias: true}
	op.ColorScale.ScaleWithColor(c)
	if width > 0 {
		vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: width * u}, op)
		return
	}
	vector.FillPath(dst, &path, nil, op)
}

// drawFace draws the leader's round face (the balloons' face) h across (pixels), standing
// over (x, y): framed in the accent color, lifted by a soft shadow, bobbing a little while
// it slides. A plain round piece while the face is not ready (or there is no picture).
func (s *atlasScene) drawFace(g *Game, dst *ebiten.Image, x, y, h float64) {
	r := h / 2
	cy := y - r - h*0.12
	if s.path != nil {
		cy -= math.Abs(math.Sin(float64(s.age)/6)) * h * 0.06
	}
	u := g.ui
	ux, uy, ur := float32(x/u), float32(cy/u), float32(r/u)
	gfx.FillCircle(dst, ux+2, uy+4, ur+3, withAlpha(colorShadowNRGBA, 120), true)
	gfx.FillCircle(dst, ux, uy, ur+3, g.th.accent, true)
	img := g.dollFace(s.leader)
	if img == nil {
		gfx.FillCircle(dst, ux, uy, ur-1, g.th.window, true)
		return
	}
	b := img.Bounds()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	sc := (h - 2) / float64(b.Dx())
	op.GeoM.Scale(sc, sc)
	op.GeoM.Translate(x-float64(b.Dx())*sc/2, cy-float64(b.Dy())*sc/2)
	dst.DrawImage(img, op)
}

// drawFinder draws a small arrow over the token's head (x, y: its feet; h: its height,
// UI units), bobbing, so the party is found at a glance on a busy picture.
func (s *atlasScene) drawFinder(g *Game, dst *ebiten.Image, x, y, h float64) {
	bob := math.Sin(float64(s.age)/12) * h * 0.03
	top := math.Max(4, y-h*1.04-h*0.12+bob) // on the screen even when the head is not
	w := h * 0.09
	var path vector.Path
	u := float32(g.ui)
	path.MoveTo(float32(x-w)*u, float32(top)*u)
	path.LineTo(float32(x+w)*u, float32(top)*u)
	path.LineTo(float32(x)*u, float32(top+w*1.3)*u)
	path.Close()
	for i, c := range []color.NRGBA{withAlpha(colorShadowNRGBA, 220), g.th.accent} {
		op := &vector.DrawPathOptions{AntiAlias: true}
		if i == 0 {
			op.ColorScale.ScaleWithColor(c)
			vector.StrokePath(dst, &path, &vector.StrokeOptions{Width: 4 * u, LineJoin: vector.LineJoinRound}, op)
			continue
		}
		op.ColorScale.ScaleWithColor(c)
		vector.FillPath(dst, &path, nil, op)
	}
}

// scriptWalk is the script's walk on a map: to a point (by id) by its choice. A point
// leading somewhere ends the walk as soon as the token sets off (the road and the arrival
// go on by themselves: wait after it); a point with only do ends when the token is there.
func (s *atlasScene) scriptWalk(g *Game, arg string) (button, bool) {
	if s.walking() {
		return btnNone, false
	}
	p := s.location(g).Map.Points[arg]
	if p == nil {
		g.failure = fmt.Errorf("script: walk %s: the map %s has no such point", arg, s.loc)
		return 0, true
	}
	if s.dirty {
		s.refresh(g)
	}
	if s.here == arg && p.To == "" {
		return 0, true
	}
	for _, c := range s.choices {
		if c.Point == arg {
			s.goTo(g, c)
			if c.Locked || p.To != "" {
				return 0, true
			}
			return btnNone, false
		}
	}
	g.failure = fmt.Errorf("script: walk %s: no open road leads there from %s (or the point is hidden)", arg, s.here)
	return 0, true
}
