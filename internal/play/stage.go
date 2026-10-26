package play

import (
	"fmt"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/stage"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
)

// A painted scene (ADR 0022): the location's background from a fixed camera, walked by the
// party's leader as a small doll. The menu's entries are what the scene offers: walking to
// an exit's area takes it, OK by someone or something takes theirs, and the menu key shows
// them all as the usual list (for the keyboard, the autopilot and anyone who prefers it).

const (
	walkSpeed  = 1.6 // meters a second
	talkReach  = 2.0 // meters from someone to talk to them
	spotReach  = 0.6 // meters from a spot to step on it
	tickSecond = 1.0 / 60
)

// walker is someone moving in the scene (the screen only: never saved).
type walker struct {
	at, facing geom.Pt
	moving     bool
	phase      float64
}

func (w *walker) step(to geom.Pt, sc *stage.Scene) {
	d := to.Sub(w.at)
	if d.Len() < 1e-6 {
		w.moving = false
		return
	}
	w.facing = d.Scale(1 / d.Len())
	w.moving = true
	w.phase = math.Mod(w.phase+sc.Meters(w.at, to)/1.3*2*math.Pi, 2*math.Pi)
	w.at = to
}

type stageScene struct {
	loc    string
	sc     *stage.Scene
	r      *stage.Renderer
	player walker
	spot   string // the spot the screen last put the party on
	age    int
	busy   bool
	// armed is false while the feet stand in an exit's area they arrived in.
	armed bool
	// goal is where the leader walks by itself (a click, a script, an entry of the list);
	// then is the entry taken on arriving there.
	goal     *geom.Pt
	way      []geom.Pt // the points to walk to, the goal last
	goalSpot string
	then     *world.Entry
	stuck    int
	best     float64 // the nearest the walk to the goal has come (meters)
	banner   int

	people  []*stagePerson
	talking string  // the person talked to (stands still and faces the leader)
	clock   float64 // seconds in the scene (the balloons' timing)
	aside   float64 // when a far one says a line next
	// onSpot and inThing are the spot and the stepped-on thing the feet are on (their do
	// runs again only after the feet leave).
	onSpot, inThing string

	// What the screen keeps from frame to frame instead of making it again: the leader's
	// doll, the entry within reach (found once an update), the figures to draw.
	leader   dollSpec
	leaderID string
	reach    world.Entry
	reachOK  bool
	figs     []stage.Figure
}

// ready makes the scene's textures when they are missing (the first showing, or after
// a reload of the pictures dropped them); false when they cannot be made.
func (s *stageScene) ready(g *Game) bool {
	if s.r == nil && g.failure == nil {
		s.r = g.stageRenderer(s.loc, s.location(g), s.sc)
	}
	return s.r != nil
}

// leaderSpec is the leader's doll, made again when another actor leads.
func (s *stageScene) leaderSpec(g *Game) dollSpec {
	if len(g.st.Party) > 0 && g.st.Party[0] != s.leaderID || s.leader.k == "" {
		s.leader = g.leaderDoll()
		if len(g.st.Party) > 0 {
			s.leaderID = g.st.Party[0]
		}
	}
	return s.leader
}

// liveFeet keeps where the leader stood across a reload (F5).
type liveFeet struct {
	loc        string
	at, facing geom.Pt
}

func newStageScene(g *Game) *stageScene {
	s := &stageScene{}
	s.arrive(g)
	return s
}

func (s *stageScene) location(g *Game) *data.Location { return g.db.Locations[s.loc] }

// arrive sets the scene up for the party's location, standing on its spot.
func (s *stageScene) arrive(g *Game) {
	s.loc = g.st.Location
	l := s.location(g)
	s.sc = stage.New(l.Stage)
	s.r = g.stageRenderer(s.loc, l, s.sc)
	s.age, s.goal, s.goalSpot, s.then, s.banner = 0, nil, "", nil, 240
	s.clock, s.aside, s.talking = 0, 2, ""
	s.placeOnSpot(g)
	s.people = g.newStagePeople(s)
	if k := g.keptFeet; k != nil && k.loc == s.loc {
		if s.sc.Free(k.at) {
			s.player.at, s.player.facing = k.at, k.facing
		}
		g.keptFeet = nil
	}
	s.armed = s.sc.ExitAt(s.player.at) == ""
	s.onSpot, s.inThing = s.spotUnder(g), s.thingUnder(g)
	s.leader, s.leaderID = dollSpec{}, ""
	g.dollSheet(s.leaderSpec(g))
}

// placeOnSpot stands the leader on the party's spot.
func (s *stageScene) placeOnSpot(g *Game) {
	l := s.location(g)
	id := atlas.Here(g.db, g.st)
	s.spot = g.st.Spot
	sp := l.Stage.Spots[id]
	if sp == nil {
		return
	}
	s.player = walker{at: sp.At.Pt(), facing: stage.Facing(sp.Facing)}
	g.emit("spot", traceLine{"location": s.loc, "spot": id})
}

// liveKeep remembers where the leader stands, for the scene made again after a reload.
func (s *stageScene) liveKeep(g *Game) scene {
	g.keptFeet = &liveFeet{loc: s.loc, at: s.player.at, facing: s.player.facing}
	return s
}

func (s *stageScene) update(g *Game) {
	if !s.ready(g) {
		return
	}
	s.age++
	s.clock += tickSecond
	s.banner = max(0, s.banner-1)
	s.advancePeople(g)
	if g.pump() {
		s.busy = true
		return
	}
	if s.busy {
		s.busy = false
		s.talking = ""
	}
	if len(g.stack) == 1 {
		s.chatter(g)
	}
	if g.enterAt != "" {
		loc := g.enterAt
		g.enterAt = ""
		if g.st.Location == loc {
			cmds := g.startArrival(true)
			if len(cmds) > 0 {
				return
			}
		}
	}
	if cmds := world.Auto(g.db, g.st); cmds != nil {
		g.events.Start(cmds)
		return
	}
	if g.st.Spot != s.spot { // an event put the party on another spot
		s.placeOnSpot(g)
		s.armed = s.sc.ExitAt(s.player.at) == ""
	}
	in := &g.in
	if in.openList() {
		g.sfx("decide")
		g.push(newStageList(g, s))
		return
	}
	if in.click {
		pw, ph := g.pixels()
		p := s.r.PointOf(float64(in.mouse.X)*g.ui, float64(in.mouse.Y)*g.ui, pw, ph)
		if e, ok := s.entryAt(g, p); ok { // someone or something: walk there and talk or look
			s.takeListed(g, e)
		} else {
			s.walkTo(p, "", nil)
		}
	}
	var dx, dy float64
	if in.held[btnLeft] {
		dx--
	}
	if in.held[btnRight] {
		dx++
	}
	if in.held[btnUp] {
		dy++
	}
	if in.held[btnDown] {
		dy--
	}
	speed := s.location(g).Stage.WalkSpeed
	if speed == 0 {
		speed = walkSpeed
	}
	// Keep each collision/trigger check within the original per-tick distance.
	// Faster movement consumes several bounded steps, stopping at the first event.
	steps := max(1, int(math.Ceil(speed/walkSpeed)))
	distance := speed * tickSecond / float64(steps)
	for range steps {
		switch {
		case dx != 0 || dy != 0:
			s.goal, s.then = nil, nil
			l := math.Hypot(dx, dy)
			to := s.sc.Move(s.player.at, dx/l*distance, dy/l*distance)
			s.player.facing = geom.Pt{X: dx / l, Y: -dy / l}
			if to != s.player.at && !s.crowded(to) {
				s.player.step(to, s.sc)
				s.player.facing = geom.Pt{X: dx / l, Y: -dy / l}
			} else {
				s.player.moving = false
			}
		case s.goal != nil:
			s.walkOn(g, distance)
		default:
			s.player.moving = false
		}
		if s.leave(g) || s.stepOn(g) {
			s.reachOK = false
			return
		}
		if g.busy() {
			return
		}
	}
	s.reach, s.reachOK = s.near(g)
	if in.ok() && s.goal == nil {
		s.interact(g)
	}
}

// spotUnder is the spot with do the feet are on ("" when none).
func (s *stageScene) spotUnder(g *Game) string {
	st := s.location(g).Stage
	for _, id := range st.SpotIDs() {
		if sp := st.Spots[id]; len(sp.Do) > 0 && s.sc.Meters(s.player.at, sp.At.Pt()) <= spotReach {
			return id
		}
	}
	return ""
}

// thingUnder is the thing with trigger step the feet are in ("" when none).
func (s *stageScene) thingUnder(g *Game) string {
	for _, t := range s.location(g).Stage.Things {
		if t.Trigger == "step" && t.Area.Poly().Inside(s.player.at) {
			return t.ID
		}
	}
	return ""
}

// stepOn takes the entry of a spot with do or a thing with trigger step the feet come onto.
func (s *stageScene) stepOn(g *Game) bool {
	spot, thing := s.spotUnder(g), s.thingUnder(g)
	enter := func(prev *string, now string, match func(world.Entry) bool) bool {
		if now == *prev {
			return false
		}
		*prev = now
		if now == "" {
			return false
		}
		for _, e := range world.Entries(g.db, g.st) {
			if match(e) {
				s.goal, s.then = nil, nil
				s.player.moving = false
				s.take(g, e)
				return true
			}
		}
		return false
	}
	return enter(&s.onSpot, spot, func(e world.Entry) bool { return e.SpotDo == spot }) ||
		enter(&s.inThing, thing, func(e world.Entry) bool { return e.Thing != nil && e.Thing.ID == thing })
}

// walkTo sends the leader walking to p by the way around what blocks (A*), taking the
// entry then on arriving. A place where feet may not stand ends at the nearest they may
// (a spot, always free by the rules, is walked to as it is).
func (s *stageScene) walkTo(p geom.Pt, spot string, then *world.Entry) {
	s.way = s.sc.Path(s.player.at, p)
	if len(s.way) == 0 || spot != "" && s.way[len(s.way)-1] != p {
		s.way = append(s.way, p)
	}
	end := s.way[len(s.way)-1]
	s.goal, s.goalSpot, s.then, s.stuck, s.best = &end, spot, then, 0, math.Inf(1)
}

// walkOn takes a step along the way toward the goal; on arriving (or when the way is
// blocked for a while) it stops and takes the waiting entry.
func (s *stageScene) walkOn(g *Game, distance float64) {
	next := *s.goal
	for len(s.way) > 1 {
		if _, _, d := s.sc.Toward(s.player.at, s.way[0], 1); d > 0.05 {
			break
		}
		s.way = s.way[1:]
		s.best = math.Inf(1)
	}
	if len(s.way) > 0 {
		next = s.way[0]
	}
	dx, dy, _ := s.sc.Toward(s.player.at, next, distance)
	_, _, left := s.sc.Toward(s.player.at, *s.goal, 1)
	if len(s.way) > 1 {
		_, _, left = s.sc.Toward(s.player.at, next, 1)
		left++ // not there yet: the way goes on
	}
	to := s.sc.Move(s.player.at, dx, dy)
	crowded := s.crowded(to)
	// stuck: no nearer for a while (a block slid along, someone in the way)
	if crowded || left >= s.best-1e-3 {
		s.stuck++
	} else {
		s.stuck, s.best = 0, left
	}
	if left > 0.05 && s.stuck < 45 {
		if !crowded {
			s.player.step(to, s.sc)
		} else {
			s.player.moving = false
		}
		return
	}
	if left > 0.05 && s.goalSpot != "" { // blocked: step there (a spot is always reachable by the rules)
		s.player.at = *s.goal
	}
	s.player.moving = false
	s.goal = nil
	if e := s.then; e != nil {
		s.then = nil
		s.take(g, *e)
	}
}

// leave takes the exit whose area the feet are in.
func (s *stageScene) leave(g *Game) bool {
	to := s.sc.ExitAt(s.player.at)
	if to == "" {
		s.armed = true
		return false
	}
	if !s.armed {
		return false
	}
	for _, e := range world.Entries(g.db, g.st) {
		if e.Kind == world.Exit && e.Exit != nil && e.Exit.To == to {
			s.armed = false
			s.take(g, e)
			return true
		}
	}
	return false
}

// interact takes what is near the leader: someone to talk to, a thing, a spot with do.
func (s *stageScene) interact(g *Game) {
	if s.reachOK {
		s.reachOK = false
		s.take(g, s.reach)
	}
}

// near is the entry of what is nearest the leader within reach.
func (s *stageScene) near(g *Game) (world.Entry, bool) {
	var best world.Entry
	bd := math.Inf(1)
	for _, e := range world.Entries(g.db, g.st) {
		p, reach, ok := s.placeOf(g, e)
		if !ok {
			continue
		}
		d := s.sc.Meters(s.player.at, p)
		if d > reach {
			continue
		}
		// what the leader faces comes first (half as far straight ahead, twice behind)
		if dir := p.Sub(s.player.at); dir.Len() > 0 {
			d *= 1.25 - 0.75*(dir.X*s.player.facing.X+dir.Y*s.player.facing.Y)/dir.Len()
		}
		if d < bd {
			best, bd = e, d
		}
	}
	return best, bd < math.Inf(1)
}

// entryAt is the entry of the person (their doll) or the thing (its area) under a point of
// the background, the nearest when several.
func (s *stageScene) entryAt(g *Game, p geom.Pt) (world.Entry, bool) {
	var best world.Entry
	bd := math.Inf(1)
	for _, e := range world.Entries(g.db, g.st) {
		at, _, ok := s.placeOf(g, e)
		if !ok || e.SpotDo != "" {
			continue
		}
		var d float64
		switch {
		case e.Person != nil: // the doll stands over its feet: a box as wide as half its height
			h := s.sc.Height(at.Y)
			if p.Y > at.Y+h*0.15 || p.Y < at.Y-h*1.1 || math.Abs(p.X-at.X) > h*0.35 {
				continue
			}
			d = math.Abs(p.X - at.X)
		case e.Thing != nil:
			if d = e.Thing.Area.Poly().Distance(p); d > 6 {
				continue
			}
		case e.Action != nil:
			t := s.location(g).Stage.ThingOf(e.Action.ID)
			if d = t.Area.Poly().Distance(p); d > 6 {
				continue
			}
		default:
			continue
		}
		if d < bd {
			best, bd = e, d
		}
	}
	return best, bd < math.Inf(1)
}

// placeOf is where an entry is taken in the scene and how near one must be.
func (s *stageScene) placeOf(g *Game, e world.Entry) (geom.Pt, float64, bool) {
	st := s.location(g).Stage
	switch {
	case e.Person != nil:
		if w := g.stageWalkerOf(s, e.Person.ID); w != nil {
			return w.at, talkReach, true
		}
	case e.Thing != nil:
		return e.Thing.Area.Poly().Center(), talkReach, true
	case e.SpotDo != "":
		return st.Spots[e.SpotDo].At.Pt(), spotReach, true
	case e.Action != nil:
		if t := st.ThingOf(e.Action.ID); t != nil {
			return t.Area.Poly().Center(), talkReach, true
		}
	}
	return geom.Pt{}, 0, false
}

// take runs an entry as the field's menu does.
func (s *stageScene) take(g *Game, e world.Entry) {
	if e.Kind == world.Menu {
		g.push(newMenu(g))
		return
	}
	switch {
	case e.Kind == world.Exit && e.Exit != nil && e.Exit.To != "":
		g.sfx("step")
	default:
		g.sfx("decide")
	}
	if e.Kind == world.Talk && e.Person != nil {
		s.standTalker(g, e.Person)
		if w := g.stageWalkerOf(s, e.Person.ID); w != nil { // face each other
			w.facing = unit(s.player.at.Sub(w.at))
			s.player.facing = unit(w.at.Sub(s.player.at))
			w.moving = false
		}
		s.talking = e.Person.ID
		for _, p := range s.people { // the balloons give way to the talk
			p.shown = 0
		}
	}
	g.events.Start(e.Commands(g.db, g.st))
	if g.st.Spot != s.spot {
		s.spot = g.st.Spot
		g.emit("spot", traceLine{"location": s.loc, "spot": s.spot})
	}
}

func unit(p geom.Pt) geom.Pt {
	if l := p.Len(); l > 0 {
		return p.Scale(1 / l)
	}
	return geom.Pt{Y: 1}
}

// standTalker shows the person's standing picture in the talk, on the side away from them.
func (s *stageScene) standTalker(g *Game, p *data.Person) {
	if p.Image == "" || g.pics.get(p.Image).Bounds().Dy() < 600 {
		return
	}
	x := 0.7
	if w := g.stageWalkerOf(s, p.ID); w != nil {
		pw, ph := g.pixels()
		if sx, _, _ := s.r.ScreenOf(w.at, pw, ph); sx > pw*0.55 {
			x = 0.3
		}
	}
	id := "face:" + p.Image
	g.stage = [2]stageActor{{}, {id: id, face: p.Image, entered: g.tick - 10}}
	g.stageOut = stageActor{}
	g.sides = map[string]float64{id: x}
	g.stageGen++
}

// scriptWalk is the script's walk in a scene: to a spot (by id), or face a way.
func (s *stageScene) scriptWalk(g *Game, arg string, face bool) (button, bool) {
	if face {
		s.player.facing = stage.Facing(arg)
		return 0, true
	}
	if s.goal == nil {
		sp := s.location(g).Stage.Spots[arg]
		if sp == nil {
			g.failure = fmt.Errorf("script: walk %s: %s has no such spot (it has %s)", arg, s.loc, strings.Join(s.location(g).Stage.SpotIDs(), ", "))
			return 0, true
		}
		if s.sc.Meters(s.player.at, sp.At.Pt()) < 0.05 {
			if sp.Facing != "" {
				s.player.facing = stage.Facing(sp.Facing)
			}
			if s.spot != arg { // standing on it (its do ran on stepping onto it)
				g.st.Spot, s.spot = arg, arg
				g.emit("spot", traceLine{"location": s.loc, "spot": arg})
			}
			return 0, true
		}
		s.walkTo(sp.At.Pt(), arg, nil)
	}
	return btnNone, false
}

func (s *stageScene) draw(g *Game, dst *ebiten.Image) {
	if !s.ready(g) {
		return
	}
	s.figs = append(s.figs[:0], stage.Figure{At: s.player.at, Facing: s.player.facing, Moving: s.player.moving, Phase: s.player.phase, Sheet: g.dollSheet(s.leaderSpec(g))})
	s.figs = g.stagePeopleFigures(s, s.figs)
	s.r.Draw(dst, s.figs)
	if g.talk.active || g.cg.File != "" || len(g.stack) > 1 {
		return
	}
	g.drawStageBalloons(dst, s)
	g.drawReach(dst, s)
	if s.banner > 0 { // the place's name, a while after coming in
		l := s.location(g)
		a := math.Min(1, float64(s.banner)/30)
		name := data.Render(l.Name, g.st.Env())
		w := math.Max(220, gfx.TextWidth(name, sizeL)+60)
		gfx.FillRoundRect(dst, 36, 28, float32(w), 60, 14, withAlpha(g.th.window, uint8(float64(g.th.window.A)*a)))
		g.text(dst, name, 64, 40, sizeL, withAlpha(g.th.text, uint8(255*a)))
	}
	_, sh := g.screen()
	g.text(dst, lang.T("決定：話す・調べる　メニュー：一覧"), 36, sh-40, sizeS*0.8, withAlpha(g.th.text, 200))
}

// entryList is the list of the place's entries over a map or a scene (the menu key); the
// entry chosen goes to take.
type entryList struct {
	take    func(*Game, world.Entry)
	entries []world.Entry
	rows    []int // the entry of each row of menu, -1 for a group's heading
	menu    *list
}

func newStageList(g *Game, s *stageScene) *entryList { return newEntryList(g, s.takeListed) }

func newEntryList(g *Game, take func(*Game, world.Entry)) *entryList {
	return newEntryListOf(g, take, nil)
}

// entryGroup is the heading an entry is listed under (its action's group), "" for none.
func entryGroup(e world.Entry) string {
	if e.Action != nil {
		return e.Action.Group
	}
	return ""
}

// groupEntries lists the entries of one group together, the groups in the order each is
// first met, a heading row (-1) before each group that has a name. The entries keep their
// order when none has a group. pinned is an entry kept apart at the end (-1 for none).
func groupEntries(entries []world.Entry, pinned int) (rows []int, heads []string) {
	var order []string
	byGroup := map[string][]int{}
	for i, e := range entries {
		if i == pinned {
			continue
		}
		k := entryGroup(e)
		if _, ok := byGroup[k]; !ok {
			order = append(order, k)
		}
		byGroup[k] = append(byGroup[k], i)
	}
	for _, k := range order {
		if k != "" {
			rows = append(rows, -1)
			heads = append(heads, k)
		}
		rows = append(rows, byGroup[k]...)
	}
	if pinned >= 0 {
		rows = append(rows, pinned)
	}
	return rows, heads
}

// newEntryListOf is the list of the entries keep lets in (all of them when nil).
func newEntryListOf(g *Game, take func(*Game, world.Entry), keep func(world.Entry) bool) *entryList {
	l := &entryList{take: take}
	for _, e := range world.Entries(g.db, g.st) {
		if keep == nil || keep(e) {
			l.entries = append(l.entries, e)
		}
	}
	pinned := -1
	if c := g.db.Game.Campaign; c != nil && c.PinEndTurn && c.Location == g.st.Location {
		for i, e := range l.entries {
			if e.Action != nil && e.Action.ID == "end_turn" {
				pinned = i
			}
		}
	}
	rows, heads := groupEntries(l.entries, pinned)
	l.rows = rows
	var items []listItem
	for _, r := range rows {
		if r < 0 {
			items = append(items, listItem{label: data.Render(heads[0], g.st.Env()), heading: true})
			heads = heads[1:]
			continue
		}
		e := l.entries[r]
		it := listItem{label: e.Label}
		switch e.Kind {
		case world.Talk:
			it.color = g.th.cursor
		case world.Exit:
			it.color = g.th.border
		case world.Menu:
			it.right = "X"
		case world.Do, world.Search:
		}
		items = append(items, it)
	}
	_, sh := g.screen()
	l.menu = newList(items, 36, 110, 470, int((sh-260)/44))
	l.menu.pin = pinned >= 0
	return l
}

func (l *entryList) overlay() bool { return true }

func (l *entryList) update(g *Game) {
	chose, cancel := l.menu.update(g)
	switch {
	case cancel:
		g.pop()
	case chose:
		e := l.entries[l.rows[l.menu.sel]]
		g.pop()
		l.take(g, e)
	}
}

// takeListed takes an entry chosen from the list: the leader walks to its place first.
func (s *stageScene) takeListed(g *Game, e world.Entry) {
	if e.Spot != "" && e.Choice == nil {
		if sp := s.location(g).Stage.Spots[e.Spot]; sp != nil && e.Kind != world.Exit {
			target, _, ok := s.placeOf(g, e)
			if !ok || e.SpotDo != "" {
				target = sp.At.Pt()
			} else {
				target = s.approach(target)
			}
			s.walkTo(target, e.Spot, &e)
			return
		}
	}
	s.take(g, e)
}

// approach is a free point beside p, on the leader's side (to stand by someone or something).
func (s *stageScene) approach(p geom.Pt) geom.Pt {
	ppm := s.sc.PPM(p.Y)
	best, bd := p, math.Inf(1)
	for i := range 16 {
		a := float64(i) * math.Pi / 8
		q := geom.Pt{X: p.X + math.Cos(a)*ppm*0.9, Y: p.Y + math.Sin(a)*ppm*0.5}
		if d := geom.Dist(q, s.player.at); s.sc.Free(q) && d < bd {
			best, bd = q, d
		}
	}
	return best
}

func (l *entryList) draw(g *Game, dst *ebiten.Image) {
	_, sh := g.screen()
	g.window(dst, 24, 96, 494, sh-230)
	l.menu.draw(g, dst, true)
}

// dollSpec is a doll to bake: the picture its colors come from, its look and the camera.
type dollSpec struct {
	picture string
	doll    *data.Doll
	camera  float64
	k       string // the key of the cache (newDollSpec)
}

// leaderDoll is the doll of the party's leader in the scene.
func (g *Game) leaderDoll() dollSpec {
	cam := 30.0
	if l := g.db.Locations[g.st.Location]; l != nil && l.Stage != nil && l.Stage.Camera > 0 {
		cam = l.Stage.Camera
	}
	if len(g.st.Party) == 0 || g.db.Actors[g.st.Party[0]] == nil {
		return newDollSpec("", nil, cam)
	}
	a := g.db.Actors[g.st.Party[0]]
	return newDollSpec(g.db.FacePicture(a, a.Face, ""), a.Doll, cam)
}
