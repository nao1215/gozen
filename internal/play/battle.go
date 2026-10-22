package play

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"
	"strconv"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/sound"
)

type bphase int

type pictureFade struct {
	from  string
	ticks int
	form  bool
}

type mergeGhost struct {
	path                       string
	fromX, toX, bottom, height float64
	remaining                  int
}

const (
	bpPlay bphase = iota
	bpInput
	bpEvent
	bpResult
)

type battleScene struct {
	b     *battle.Battle
	onEnd func(res int)
	troop string
	retry []byte
	phase bphase
	bg    data.Picture

	queue []battle.Step
	stepT int

	autoAcc float64

	openTick int
	msg      []string

	members   []*battle.Unit
	mi        int
	actions   []battle.Action
	cmd       *list
	sub       *list
	subKind   string
	subIDs    []string
	target    *targeting
	auto      bool
	decisions []battle.Decision

	hp, mp map[*battle.Unit]float64

	shownHP, shownMP map[*battle.Unit]int
	shownSt          map[*battle.Unit]map[string]int
	pops             []popup
	flashU           map[*battle.Unit]int
	shakeU           map[*battle.Unit]int
	foeImages        map[*battle.Unit]string
	foeFade          map[*battle.Unit]pictureFade
	mergeGhosts      []mergeGhost
	mergeTarget      *battle.Unit
	mergeTicks       int
	react            map[*battle.Unit]reaction
	gone             map[*battle.Unit]int
	fx               []effectAnim
	hits             []pendingHit
	actor            *battle.Unit

	coverBy, coverFor *battle.Unit
	coverT            int

	cut       *cutIn
	cutDone   bool
	cutCount  int
	cutBuf    *ebiten.Image
	introT    int
	boss      string
	bossTitle string
	layer     *ebiten.Image

	lastAnim  string
	lastColor color.NRGBA
	lastFx    string
	lastPower int
	lastHarm  bool

	scr   []screenFx
	fxSeq uint32
	fxb   fxBatch

	runner  *event.Runner
	req     event.Request
	results []string
	fanfare []bool
	age     int
	endT    int

	dr drama
}

type pendingHit struct {
	t int
	f func()
}

func (s *battleScene) flushHits() {
	hs := s.hits
	s.hits = nil
	for _, h := range hs {
		h.f()
	}
}

func (s *battleScene) stepHits() {
	var due []func()
	s.hits = slices.DeleteFunc(s.hits, func(h pendingHit) bool {
		if h.t <= 1 {
			due = append(due, h.f)
			return true
		}
		return false
	})
	for i := range s.hits {
		s.hits[i].t--
	}
	for _, f := range due {
		f()
	}
}

func (s *battleScene) atHit(g *Game, kind string, dur int, f func()) {
	d := int(fxHit(kind) * float64(dur))
	if d <= 1 {
		f()
		return
	}
	s.hits = append(s.hits, pendingHit{t: d, f: f})
	s.stepT = max(s.stepT, d+g.tempo(16))
}

type popup struct {
	text  string
	x, y  float64
	t     int
	color color.NRGBA
	big   bool
}

type reaction struct {
	face string
	t    int
}

type targeting struct {
	action  battle.Action
	pool    []*battle.Unit
	sel     int
	enemies bool
}

func (g *Game) startBattle(troop string, done func(int)) *battleScene {
	g.traceBattleCapture(troop)
	b := battle.New(g.db, g.st, troop)
	g.traceBattleDecisions(b)
	g.autoKey = ""
	g.Image(data.CG{})
	s := &battleScene{
		b: b, onEnd: done,
		hp: map[*battle.Unit]float64{}, mp: map[*battle.Unit]float64{},
		flashU: map[*battle.Unit]int{}, shakeU: map[*battle.Unit]int{},
		react: map[*battle.Unit]reaction{}, gone: map[*battle.Unit]int{},
	}
	s.recordDecisions()
	s.sync()
	for _, u := range append(slices.Clone(b.Party), b.Foes...) {
		s.hp[u], s.mp[u] = float64(u.HP), float64(u.MP)
	}
	loc := g.db.Locations[g.st.Location]
	s.bg = b.Troop.Background
	if s.bg.File == "" {
		s.bg = dungeon.BattleBG(g.db, g.st)
	}
	if s.bg.File == "" {
		s.bg = loc.BattleBG
	}
	if s.bg.File == "" {
		s.bg = loc.Background
	}
	s.runner = event.New(g.db, g.st, event.HostFunc(g.Handle))
	s.runner.Extra = b.Env(nil)
	mc := dungeon.BattleMusic(g.db, g.st, b.Troop)
	if mc.Song == "" {
		mc.Song = "battle"
	}
	if _, ok := g.db.Music.Songs[mc.Song]; ok {
		mood := mc.Mood
		if mood == "" {
			mood = sound.Neutral
		}
		g.baseSong, g.baseMood = mc.Song, mood
		g.lastMood = ""
		sound.PlaySong(mc.Song, mood)
	}
	for _, u := range b.Foes {
		if u.Def.Boss {
			s.boss, s.bossTitle = u.Name, u.Def.Title
			break
		}
	}
	if s.boss != "" {
		g.sfx("boss")
		s.stepT = bossTicks - 20
	} else {
		g.sfx("encounter")
		s.stepT = introTicks
	}
	g.flash = 14
	s.queue = append(s.queue, b.Begin()...)
	var pics []string
	for _, u := range b.Foes {
		pics = append(pics, u.Def.Image.File)
		for _, f := range u.Def.Faces {
			pics = append(pics, f)
		}
	}
	g.pics.preload(pics...)
	if g.autoBattle || g.autoStartBattle() {
		s.auto = true
		for _, m := range b.Party {
			m.Auto = true
		}
	}
	s.troop = troop
	s.openTick = g.tick
	g.push(s)
	return s
}

func (s *battleScene) update(g *Game) {
	if !g.talk.active && (g.in.pressed[btnLog] || g.in.click && g.in.mouse.In(s.reasonButton(g))) {
		g.push(newTacticReasons(g, s.decisions))
		return
	}
	s.tick(g)

	if !s.auto || s.phase != bpPlay || g.in.fast || g.in.held[btnOK] {
		s.autoAcc = 0
		return
	}
	for s.autoAcc += g.autoTempo() - 1; s.autoAcc >= 1 && s.auto && s.phase == bpPlay; s.autoAcc-- {
		s.tick(g)
	}
}

func (s *battleScene) tick(g *Game) {
	if s.phase == bpPlay && s.stepDrama(g) {
		return
	}
	s.age++
	s.introT++
	if s.cut != nil {
		s.cut.t++
		if s.cut.t >= cutInTicks {
			s.cut = nil
		}
	}
	s.coverT++
	for u, n := range s.flashU {
		if n > 0 {
			s.flashU[u] = n - 1
		}
	}
	for u, n := range s.shakeU {
		if n > 0 {
			s.shakeU[u] = n - 1
		}
	}
	for u, f := range s.foeFade {
		if f.ticks <= 1 {
			delete(s.foeFade, u)
		} else {
			f.ticks--
			s.foeFade[u] = f
		}
	}
	if s.mergeTicks > 0 {
		s.mergeTicks--
	}
	for i := range s.mergeGhosts {
		s.mergeGhosts[i].remaining--
	}
	s.mergeGhosts = slices.DeleteFunc(s.mergeGhosts, func(m mergeGhost) bool { return m.remaining <= 0 })
	for u, r := range s.react {
		if r.t > 0 {
			r.t--
			s.react[u] = r
		}
	}
	for u, n := range s.gone {
		if n < 40 {
			s.gone[u] = n + 1
		}
	}
	s.pops = slices.DeleteFunc(s.pops, func(p popup) bool { return p.t > 70 })
	for i := range s.pops {
		s.pops[i].t++
	}
	s.stepFx()
	s.stepHits()

	for u := range s.hp {
		s.hp[u] += (float64(s.shownHP[u]) - s.hp[u]) * 0.15
		s.mp[u] += (float64(s.shownMP[u]) - s.mp[u]) * 0.2

		if math.Abs(float64(s.shownHP[u])-s.hp[u]) < 0.5 {
			s.hp[u] = float64(s.shownHP[u])
		}
		if math.Abs(float64(s.shownMP[u])-s.mp[u]) < 0.5 {
			s.mp[u] = float64(s.shownMP[u])
		}
	}
	if s.auto && g.in.cancel() {
		s.auto = false
		g.autoBattle = false
		for _, u := range s.b.Party {
			u.Auto = false
		}
		g.sfx("cancel")
	}
	switch s.phase {
	case bpPlay:
		s.play(g)
	case bpInput:
		s.input(g)
	case bpEvent:
		s.event(g)
	case bpResult:
		s.result(g)
	}
}

func (s *battleScene) play(g *Game) {
	if s.stepT > 0 {
		s.stepT--
		if g.in.held[btnOK] || g.in.fast {
			s.stepT -= 2
		}
		return
	}
	if len(s.queue) > 0 {
		st := s.queue[0]
		s.queue = s.queue[1:]
		s.show(g, st)
		return
	}
	s.actor = nil
	s.flushHits()
	s.sync()
	b := s.b
	if b.Result != battle.Ongoing {
		s.finish(g)
		return
	}
	if b.Phase() == "act" {
		steps, done := b.Next()
		s.queue = append(s.queue, steps...)
		if done && b.Result == battle.Ongoing {
			s.queue = append(s.queue, battle.Step{Kind: -1})
		}
		if len(s.queue) == 0 && done {
			s.beginInput(g)
		}
		return
	}
	s.beginInput(g)
}

func (g *Game) battleUnit(id string) *battle.Unit {
	for _, sc := range g.stack {
		if s, ok := sc.(*battleScene); ok {
			for _, u := range s.b.Party {
				if u.Actor != nil && u.Actor.ID == id {
					return u
				}
			}
		}
	}
	return nil
}

func (s *battleScene) sync() {
	if s.shownHP == nil {
		s.shownHP, s.shownMP, s.shownSt = map[*battle.Unit]int{}, map[*battle.Unit]int{}, map[*battle.Unit]map[string]int{}
	}
	for _, u := range append(slices.Clone(s.b.Party), s.b.Foes...) {
		s.shownHP[u], s.shownMP[u] = u.HP, u.MP
		s.shownSt[u] = maps.Clone(u.States)
	}
}

func (s *battleScene) shownStates(u *battle.Unit) map[string]int {
	if st, ok := s.shownSt[u]; ok {
		return st
	}
	return u.States
}

func (s *battleScene) catchUp(st battle.Step) {
	t := st.Target
	if t == nil {
		return
	}
	switch st.Kind {
	case battle.Message, battle.Act, battle.Miss, battle.Escape, battle.Event:
	case battle.Damage, battle.Heal, battle.Revive, battle.Summon, battle.Transform, battle.Merge:
		s.shownHP[t], s.shownMP[t] = st.HP, st.MP
		if st.Kind == battle.Summon || st.Kind == battle.Transform || st.Kind == battle.Merge {
			s.shownSt[t] = maps.Clone(t.States)
		}
	case battle.Fall:
		s.shownHP[t] = 0
	case battle.StateOn:
		m := maps.Clone(s.shownSt[t])
		if m == nil {
			m = map[string]int{}
		}
		m[st.State] = max(1, t.States[st.State])
		s.shownSt[t] = m
	case battle.StateOff:
		m := maps.Clone(s.shownSt[t])
		delete(m, st.State)
		s.shownSt[t] = m
	}
}

func (s *battleScene) show(g *Game, st battle.Step) {
	if st.Kind == -1 {
		s.beginInput(g)
		return
	}
	replay := st.Kind == battle.Act && (s.cutDone || s.dr.chargeDone)
	if !replay {
		g.traceBattleStep(st)
	}
	s.flushHits()
	if st.Kind != battle.Damage && st.Kind != battle.Heal && st.Kind != battle.Revive {
		s.catchUp(st)
	}
	if st.Text != "" && !replay {
		g.emit("battle", traceLine{"text": st.Text})
		s.msg = append(s.msg, st.Text)
		if len(s.msg) > 3 {
			s.msg = s.msg[len(s.msg)-3:]
		}
	}
	s.stepT = g.tempo(34)
	if st.Kind == battle.Heal && st.Text == "" {
		s.stepT = g.tempo(4)
	}
	if st.Kind == battle.Act {
		s.coverBy, s.coverFor = nil, nil
	}
	if st.Kind == battle.Message && st.User != nil && st.Target != nil && st.User != st.Target && !st.User.Enemy && !st.Target.Enemy {
		s.coverBy, s.coverFor, s.coverT = st.User, st.Target, 0
		g.sfx("tink")
	}
	switch st.Kind {
	case battle.Message:
	case battle.Event:
		s.runner.Extra = s.b.Env(nil)
		s.runner.Start(st.Commands)
		s.phase = bpEvent
		s.stepT = 0
	case battle.Act:
		if !s.cutDone && !s.dr.chargeDone && s.startCharge(g, st) {
			s.dr.chargeDone = true
			s.queue = append([]battle.Step{st}, s.queue...)
			return
		}
		if !s.cutDone && s.startCutIn(g, st) {
			s.cutDone = true
			s.queue = append([]battle.Step{st}, s.queue...)
			s.msg = []string{st.Text}
			s.stepT = cutInTicks - 6
			return
		}
		s.cutDone = false
		s.dr.chargeDone = false
		s.actor = st.User
		if u := st.User; u != nil {
			s.shownMP[u] = u.MP
		}
		s.msg = []string{st.Text}
		s.reactTo(g, st.User, "act", 30)
		sfx := ""
		if use := s.usage(st.Skill); use != nil {
			sfx = use.SFX
		}
		switch st.Anim {
		case "magic", "buff":
			if sfx == "" {
				sfx = "magic"
			}
		case "heal":
			if sfx == "" {
				sfx = "heal"
			}
		}
		if sfx != "" {
			g.sfx(sfx)
		}
		if st.User.Enemy {
			s.flashU[st.User] = 12
		}
		s.stepT = g.tempo(30)
		use := s.usage(st.Skill)
		s.lastAnim = st.Anim
		s.lastFx = fxKind(use)
		s.lastColor = fxColor(s.lastFx, st.Color)
		s.lastPower = 1
		s.lastHarm = false
		if use != nil {
			for _, e := range use.Effects {
				if e.Damage.Set() || e.Heal.Set() || e.Revive.Set() {
					s.lastHarm = true
				}
			}
		}
		all := use != nil && (use.Scope == "all_enemies" || use.Scope == "all_allies" || use.Scope == "everyone")
		switch {
		case use != nil && (use.Cutin || all):
			s.lastPower = 2
		case st.Skill == s.b.DB.Game.Battle.Attack:
			s.lastPower = 0
		}
		if s.lastFx != "none" && !physicalFx(s.lastFx) {

			r := s.unitRect(g, st.User)
			s.addFx(g, "cast", float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), s.lastColor, s.lastPower)
		}
		if all && s.lastFx != "none" && use.Scope != "all_allies" {

			sw, sh := g.screen()
			s.addScreen(g, "flash", 0, 0, s.lastColor, 0.28, 18)
			s.addScreen(g, "wave", sw/2, sh*0.42, s.lastColor, 0.7, 34)
			g.shake = max(g.shake, 6)
		}
	case battle.Damage:
		r := s.unitRect(g, st.Target)
		anim := s.lastAnim
		if anim == "" {
			anim = "slash"
		}
		kind, power, fc := s.lastFx, s.lastPower, s.lastColor
		if kind == "" || kind == "none" {
			kind, fc = "slash", fxColor("slash", data.Color{})
		}
		if st.User == nil || st.User != s.actor {
			kind, power, fc = "strike", 0, color.NRGBA{0xd0, 0xb0, 0xc0, 0xff}
		}
		if st.Crit || st.Weak {
			power = 2
		}
		dur := s.addFx(g, kind, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), fc, power)
		if physicalFx(kind) && slices.Contains(elemental, st.Element) {
			s.addFx(g, st.Element, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), fxColor(st.Element, data.Color{}), 0)
		}

		s.atHit(g, kind, dur, func() { s.damageHit(g, st, r, kind, anim, fc) })
	case battle.Heal, battle.Revive:
		r := s.unitRect(g, st.Target)
		cx := float64(r.Min.X+r.Max.X) / 2
		c := color.NRGBA{0x8c, 0xff, 0x9e, 0xff}
		if st.Stat == "mp" {
			c = mpColor
		}
		hc := s.lastColor
		if s.lastFx != "heal" && s.lastFx != "holy" {
			hc = fxColor("heal", data.Color{})
		}
		dur := s.addFx(g, "heal", float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), hc, min(1, s.lastPower))
		s.atHit(g, "heal", dur, func() {
			s.catchUp(st)
			s.pops = append(s.pops, popup{text: "+" + strconv.Itoa(st.Amount), x: cx, y: s.popY(cx, float64(r.Min.Y)+20), color: c})
			s.reactTo(g, st.Target, "heal", 40)
			if st.Kind == battle.Revive {
				delete(s.gone, st.Target)
			}
		})
	case battle.StateOn:
		r := s.unitRect(g, st.Target)
		kind := "buff"
		if g.db.States[st.State].Bad {
			kind = "debuff"
		}
		power, fc := 1, fxColor(kind, st.Color)
		switch {
		case s.lastFx == "buff" || s.lastFx == "debuff":
		case !s.lastHarm && s.lastFx != "none" && s.lastFx != "" && !physicalFx(s.lastFx):
			kind, power, fc = s.lastFx, s.lastPower, s.lastColor
		default:
			power = 0
		}
		s.addFx(g, kind, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), fc, power)
		if g.db.States[st.State].Bad {
			g.sfx("debuff")
		} else {
			g.sfx("buff")
		}
	case battle.Fall:
		s.gone[st.Target] = 0
		s.reactTo(g, st.Target, "fall", 9999)
		g.sfx("fall")
		s.stepT = g.tempo(40)
	case battle.Summon:
		u := st.Target
		s.hp[u], s.mp[u] = float64(u.HP), float64(u.MP)
		s.flashU[u] = 20
		g.pics.preload(u.Def.Image.File)
		r := s.unitRect(g, u)
		s.addFx(g, "cast", float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), g.th.accent, 1)
		g.sfx("magic")
	case battle.Transform, battle.Merge:
		u := st.Target
		s.hp[u], s.mp[u] = float64(u.HP), float64(u.MP)
		s.shownHP[u], s.shownMP[u] = u.HP, u.MP
		s.flashU[u] = 24
		g.pics.preload(u.Picture())
		r := s.unitRect(g, u)
		s.addFx(g, "cast", float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), g.th.accent, 2)
		s.addScreen(g, "flash", 0, 0, g.th.accent, 0.4, 12)
		if st.Kind == battle.Transform && st.Before != "" {
			if s.foeFade == nil {
				s.foeFade = map[*battle.Unit]pictureFade{}
			}
			s.foeFade[u] = pictureFade{from: st.Before, ticks: 24, form: true}
		}
		if st.Kind == battle.Merge {
			s.mergeTarget, s.mergeTicks = u, 24
			cx := float64(r.Min.X+r.Max.X) / 2
			sw, sh := g.screen()
			oldCount := len(s.b.Foes) + len(st.Members) - 1
			for _, member := range st.Members {
				x := sw/2 + (float64(member.Index)-float64(oldCount-1)/2)*min(180, (sw-foeLeft)/float64(oldCount))
				s.mergeGhosts = append(s.mergeGhosts, mergeGhost{path: member.Picture(), fromX: x, toX: cx, bottom: sh * 0.66, height: min(180, float64(r.Dy())), remaining: 18})
			}
		}
		g.sfx("magic")
	case battle.Escape:
		g.sfx("escape")
	case battle.StateOff:
		if st.User != nil && st.Target != nil && st.Target.Enemy && st.User != st.Target {
			if d := g.db.States[st.State]; d != nil && !d.Bad {
				g.sfx("break")
			}
		}
	case battle.Miss:
		g.sfx("miss")
		r := s.unitRect(g, st.Target)
		mx := float64(r.Min.X+r.Max.X) / 2
		s.pops = append(s.pops, popup{text: "MISS", x: mx, y: s.popY(mx, float64(r.Min.Y)+30), color: g.th.border})
	}
}

func (s *battleScene) damageHit(g *Game, st battle.Step, r image.Rectangle, kind, anim string, fc color.NRGBA) {
	s.catchUp(st)
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y)+float64(r.Dy())*0.35
	c := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	if !st.Target.Enemy {
		c = color.NRGBA{0xff, 0x8a, 0x8a, 0xff}
		cy = float64(r.Min.Y) - 10
	}
	if st.Stat == "mp" {
		c = mpColor
	}
	text := strconv.Itoa(st.Amount)
	s.retirePops(cx)
	cy = s.popY(cx, cy)
	if st.Crit && st.Stat != "mp" {
		c = color.NRGBA{0xff, 0xe0, 0x60, 0xff}
	}
	finishing := st.Target.Enemy && st.Stat != "mp" && st.HP <= 0 && st.Amount > 0 && s.lastFoe(st.Target)
	s.pops = append(s.pops, popup{text: text, x: cx, y: cy, color: c, big: st.Crit || st.Weak || finishing})

	var tags []popup
	if st.Crit && st.Amount > 0 {
		tags = append(tags, popup{text: lang.T("CRITICAL!"), color: colorGold})
	}
	if st.Target.Enemy && st.Element != "" && (st.Weak || st.Resisted) {
		g.st.Vars["known_"+st.Target.EID+"_"+st.Element] = 1.0
	}
	if st.Weak && st.Amount > 0 {
		tags = append(tags, popup{text: lang.T("WEAK!"), color: color.NRGBA{0xff, 0xd0, 0x50, 0xff}})
	}
	if st.Resisted {
		tags = append(tags, popup{text: lang.T("効きが悪い…"), color: color.NRGBA{0xa0, 0xb8, 0xd0, 0xff}})
	}
	for i, tg := range tags {
		tg.x, tg.y = cx, cy-popTagGap*float64(i+1)-10
		s.pops = append(s.pops, tg)
	}

	if st.Amount > 0 && st.Crit {
		s.addScreen(g, "flash", 0, 0, colorWhite, g.flashAlpha(0.3), 7)
	} else if st.Amount > 0 && st.Weak {
		s.addScreen(g, "flash", 0, 0, mixColor(fc, colorWhite, 0.5), g.flashAlpha(0.2), 7)
	} else if st.Amount > 0 && kind == "thunder" {
		s.addScreen(g, "flash", 0, 0, colorWhite, 0.22, 8)
	}
	if st.Amount > 0 {
		if st.Target.Enemy && st.Stat != "mp" {
			if p := g.puppets.live["enemy."+st.Target.EID]; p != nil {
				p.Motion.SetHealth(float64(st.HP) / float64(max(1, st.Target.MHP())))
				p.Motion.Hit()
			}
		}
		s.flashU[st.Target] = 14
		s.shakeU[st.Target] = 16
		s.reactTo(g, st.Target, "hit", 40)
		physical := anim == "slash" || anim == "hit"
		switch {
		case st.Crit:
			g.sfx("crit")
			g.shake = 14
		case physical && st.Target.Guard:
			g.sfx("tink")
		case physical && st.Weak:
			g.sfx("crit")
		case physical && !st.Target.Enemy:
			g.sfx("punch")
		case physical:
			g.sfx("hit")
		case st.Weak:
			g.sfx("boom")
		default:
			g.sfx("blast")
		}
		if physicalFx(kind) {
			g.shake = max(g.shake, 3)
		} else {
			g.shake = max(g.shake, 5)
		}
		if !st.Target.Enemy {
			g.shake = max(g.shake, 8)
		}
		if st.Weak {
			g.shake = max(g.shake, 10)
		}
		s.hitDrama(g, st)
		if st.Target.Enemy && st.Stat != "mp" && st.HP <= 0 && s.lastFoe(st.Target) {
			s.finishDrama(g, st.Target)
		}
	}
}

const popFade = 50

func (s *battleScene) retirePops(x float64) {
	for i := range s.pops {
		if p := &s.pops[i]; math.Abs(p.x-x) < 70 && p.t >= 6 {
			p.t = max(p.t, popFade)
		}
	}
}

const popTagGap = 48.0

func (s *battleScene) popY(x, y float64) float64 {
	for _, p := range s.pops {
		if math.Abs(p.x-x) < 70 && p.t < popFade && math.Abs(p.y-y) < 40 {
			y -= 46
		}
	}
	return y
}

func (s *battleScene) usage(id string) *data.Usage {
	if sk := s.b.DB.Skills[id]; sk != nil {
		return &sk.Usage
	}
	if it := s.b.DB.Items[id]; it != nil {
		return it.Use
	}
	return nil
}

func (s *battleScene) reactTo(g *Game, u *battle.Unit, kind string, t int) {
	if u == nil {
		return
	}
	if u.Def != nil {
		if p := g.puppets.live["enemy."+u.EID]; p != nil {
			if r, ok := p.Rig().Spec.ReactionOf(map[string]string{"act": "hop", "hit": "shake"}[kind]); ok {
				p.Motion.React(r)
			}
		}
	}
	var face string
	if u.Actor != nil {
		face = u.Actor.Def().Reactions[kind]
	} else if u.Def != nil {
		face = u.Def.Reactions[kind]
	}
	if face != "" {
		s.react[u] = reaction{face: face, t: t}
	}
}

func (s *battleScene) beginInput(g *Game) {
	b := s.b
	if b.Result != battle.Ongoing {
		s.finish(g)
		return
	}
	b.SetPhase("input")
	s.members = b.Commandable()
	s.actions = nil
	s.mi = 0
	s.target, s.sub = nil, nil
	if len(s.members) == 0 {
		b.StartTurn(nil)
		s.phase = bpPlay
		return
	}
	s.phase = bpInput
	s.openCommands(g)
}

func (s *battleScene) asking() {
	if s.mi < len(s.members) {
		s.msg = []string{s.members[s.mi].Name + lang.T("は どうする？")}
	}
}

func (s *battleScene) openCommands(g *Game) {
	items := []listItem{{label: lang.T("こうげき")}, {label: lang.T("スキル")}, {label: lang.T("ぼうぎょ")}, {label: lang.T("どうぐ")}, {label: lang.T("オート")}, {label: lang.T("にげる"), disabled: !s.b.Troop.Escapable()}}
	u := s.members[s.mi]
	if len(u.Actor.Skills()) == 0 {
		items[1].disabled = true
	}
	_, sh := g.screen()
	s.cmd = newList(items, 36, 0, 250, 6)
	s.cmd.rowH = 40
	s.cmd.y = sh - 170 - s.cmd.height()
	s.cmd.sel = 0
	s.asking()
}

func (s *battleScene) input(g *Game) {
	u := s.members[s.mi]
	if s.target != nil {
		s.updateTarget(g)
		return
	}
	if s.sub != nil {
		chose, cancel := s.sub.update(g)
		if cancel {
			s.sub = nil
			return
		}
		if chose {
			id := s.subIDs[s.sub.sel]
			a := battle.Action{User: u}
			if s.subKind == "skill" {
				a.Skill = id
			} else {
				a.Item = id
			}
			s.choose(g, a)
		}
		return
	}
	chose, cancel := s.cmd.update(g)
	if cancel && s.mi > 0 {
		s.mi--
		s.actions = s.actions[:len(s.actions)-1]
		s.openCommands(g)
		return
	}
	if !chose {
		return
	}
	db := s.b.DB
	switch s.cmd.sel {
	case 0:
		s.choose(g, battle.Action{User: u, Skill: db.Game.Battle.Attack})
	case 1:
		s.openSkills(g, u)
	case 2:
		s.choose(g, battle.Action{User: u, Skill: db.Game.Battle.Guard})
	case 3:
		s.openItems(g)
	case 4:
		s.auto = true
		g.autoBattle = true
		for _, m := range s.b.Party {
			m.Auto = true
		}
		s.b.StartTurn(s.actions)
		s.phase = bpPlay
	case 5:
		steps, ok := s.b.TryEscape()
		s.queue = append(s.queue, steps...)
		if !ok {
			var pass []battle.Action
			for _, m := range s.b.Party {
				pass = append(pass, battle.Action{User: m, Pass: true})
			}
			s.b.StartTurn(pass)
		}
		s.phase = bpPlay
	}
}

func (s *battleScene) openSkills(g *Game, u *battle.Unit) {
	var items []listItem
	s.subIDs = nil
	for _, id := range u.Actor.Skills() {
		sk := s.b.DB.Skills[id]
		ok, _ := s.b.CanUse(u, id)
		items = append(items, listItem{label: sk.Name, right: strconv.Itoa(sk.MP), disabled: !ok || !slices.Contains(sk.Where, "battle")})
		s.subIDs = append(s.subIDs, id)
	}
	s.openSub(g, items, "skill")
}

func (s *battleScene) openItems(g *Game) {
	var items []listItem
	s.subIDs = nil
	for _, id := range g.st.Bag() {
		it := s.b.DB.Items[id]
		if it.Use == nil {
			continue
		}
		n := g.st.Items[id]
		for _, a := range s.actions {
			if a.Item == id {
				n--
			}
		}
		items = append(items, listItem{label: it.Name, right: fmt.Sprintf("×%d", n), disabled: n <= 0 || !slices.Contains(it.Use.Where, "battle")})
		s.subIDs = append(s.subIDs, id)
	}
	if len(items) == 0 {
		g.sfx("buzzer")
		return
	}
	s.openSub(g, items, "item")
}

func (s *battleScene) openSub(g *Game, items []listItem, kind string) {
	sw, sh := g.screen()
	l := newList(items, 300, 0, math.Min(560, sw-340), 6)
	l.rowH = 40
	l.y = sh - 170 - l.height()
	s.sub, s.subKind = l, kind
}

func (s *battleScene) choose(g *Game, a battle.Action) {
	use := s.usage(a.Skill + a.Item)
	if a.Item != "" {
		use = s.b.DB.Items[a.Item].Use
	}
	scope := ""
	if use != nil {
		scope = use.Scope
	}
	switch scope {
	case "one_enemy":
		var pool []*battle.Unit
		for _, f := range s.b.Foes {
			if f.Alive() {
				pool = append(pool, f)
			}
		}
		s.target = &targeting{action: a, pool: pool, enemies: true}
		return
	case "one_ally", "dead_ally":
		var pool []*battle.Unit
		for _, m := range s.b.Party {
			if m.Alive() == (scope == "one_ally") {
				pool = append(pool, m)
			}
		}
		if len(pool) == 0 {
			g.sfx("buzzer")
			return
		}
		s.target = &targeting{action: a, pool: pool}
		if scope == "one_ally" {
			for i, m := range pool {
				if m == a.User {
					s.target.sel = i
				}
			}
		}
		return
	}
	s.commit(g, a)
}

func (s *battleScene) updateTarget(g *Game) {
	t := s.target
	n := len(t.pool)
	if g.in.cancel() {
		s.target = nil
		g.sfx("cancel")
		return
	}
	old := t.sel
	if g.in.left() || g.in.up() {
		t.sel = (t.sel - 1 + n) % n
	}
	if g.in.right() || g.in.down() {
		t.sel = (t.sel + 1) % n
	}
	hit := -1
	for i, u := range t.pool {
		if g.in.mouse.In(s.unitRect(g, u)) {
			hit = i
		}
	}
	if hit >= 0 && (g.in.moved || g.in.click) {
		t.sel = hit
	}
	if old != t.sel {
		g.sfx("cursor")
	}
	if g.in.ok() || (g.in.click && hit >= 0) {
		g.sfx("decide")
		a := t.action
		a.Target = t.pool[t.sel]
		s.target = nil
		s.commit(g, a)
	}
}

func (s *battleScene) commit(g *Game, a battle.Action) {
	s.actions = append(s.actions, a)
	s.sub = nil
	s.mi++
	if s.mi < len(s.members) {
		s.openCommands(g)
		return
	}
	s.b.StartTurn(s.actions)
	s.phase = bpPlay
	s.msg = nil
}

func (s *battleScene) event(g *Game) {
	if s.req == nil {
		s.req = s.runner.Next()
		switch r := s.req.(type) {
		case nil:
			s.phase = bpPlay
		case event.Say:
			line, ok := battleLine(s.b.Party, r)
			if !ok {
				s.req = nil
				s.event(g)
				return
			}
			s.req = line
			// Like a line outside a battle (begin), it stops skipping when not read and
			// goes to the trace, so replay and trace check see it.
			g.skipBefore(line)
			g.traceBattleRequest(line)
			g.talk.open(g, line)
		case event.Choice:
			g.skipBefore(r)
			g.traceBattleRequest(r)
			g.talk.ask(g, r)
		case event.Wait:
			s.stepT = int(r.Seconds * 60)
		case event.Tap:
			g.beginTap(r)
		case event.Notice:
			g.traceBattleRequest(r)
			g.notices.add(r.Text)
			s.req = nil
		default:
			s.req = nil
		}
		return
	}
	switch s.req.(type) {
	case event.Say, event.Choice:
		if done, ans := g.talk.update(g); done {
			if ans >= 0 {
				s.runner.Answer(ans)
			}
			s.req = nil
			s.event(g)
		}
	case event.Wait:
		s.stepT--
		if s.stepT <= 0 {
			s.req = nil
		}
	case event.Tap:
		if g.updateTap() {
			s.req = nil
		}
	}
}

func (s *battleScene) finish(g *Game) {
	if s.phase == bpResult {
		return
	}
	b := s.b
	b.SetPhase("end")
	s.phase = bpResult
	s.results = b.End()
	mood, fanfare := g.endMusic(b)
	if b.Result == battle.Won && s.boss != "" && s.bossWinMusic(g) {
		mood, fanfare = "", false
	} else if mood != "" {
		sound.JumpLast(mood)
		g.lastMood = mood
	} else if fanfare {
		sound.Stop()
	}
	g.emit("battle_music", traceLine{"mood": mood, "fanfare": fanfare})
	switch b.Result {
	case battle.Ongoing:
		return
	case battle.Won:
		if fanfare {
			g.sfx("win")
		}
		s.fanfare = make([]bool, len(s.results)+1)
		for _, i := range b.Gains.LevelUps {
			s.fanfare[i+1] = true
		}
		s.results = append([]string{lang.T("たたかいに かった！")}, s.results...)
		s.dr.bossWon = s.boss != ""
		s.dr.winT = 0
		if s.dr.bossWon {
			g.shake = max(g.shake, int(10*g.fxk(g.db.Game.UI.BattleFx.Shake)))
		}
		for _, u := range b.Party {
			if u.Alive() {
				s.reactTo(g, u, "win", 9999)
			}
		}
	case battle.Lost:
		s.results = []string{lang.T("ぜんめつ してしまった……")}
	case battle.Fled:
		s.results = nil
	}
	g.emit("battle_end", traceLine{"result": map[battle.Result]string{battle.Won: "won", battle.Lost: "lost", battle.Fled: "fled"}[b.Result], "messages": s.results})
	g.traceBattleResult(s)
	s.endT = 0
}

func (s *battleScene) result(g *Game) {
	s.endT++
	s.dr.winT++
	if s.dr.winT == bossWinStamp && s.dr.bossWon {
		g.shake = max(g.shake, int(14*g.fxk(g.db.Game.UI.BattleFx.Shake)))
	}
	if len(s.results) > 0 {
		wait := 20
		if s.dr.bossWon && s.dr.winT < bossWinHold+20 && !g.in.fast {
			wait = max(wait, s.winHold(g))
		}
		if s.endT > wait && (g.in.advance() || g.in.fast || (s.auto && float64(s.endT) > float64(g.tempo(70))/g.autoTempo()) || s.endT > g.tempo(130)) {
			s.results = s.results[1:]
			if len(s.fanfare) > 0 {
				s.fanfare = s.fanfare[1:]
			}
			s.endT = 0
			if len(s.fanfare) > 0 && s.fanfare[0] {
				g.sfx("levelup")
			}
		}
		return
	}
	if s.endT < 10 {
		return
	}

	if s.b.Result == battle.Lost && s.retry != nil {
		snap := s.retry
		s.retry = nil
		c := newConfirm(lang.T("ぜんめつ した。たたかう まえから やりなおす？（いいえ: あきらめる）"), func() {
			if !g.restoreState(snap) {
				return
			}
			g.pop()
			g.emit("request", traceLine{"what": "retry", "troop": s.troop})
			n := g.startBattle(s.troop, s.onEnd)
			n.retry = snap
		})
		g.push(c)
		return
	}
	res := map[battle.Result]int{battle.Won: event.BattleWon, battle.Lost: event.BattleLost, battle.Fled: event.BattleFled}[s.b.Result]
	if s.b.Result == battle.Lost {
		s.b.Rise()
	}
	g.pop()

	g.emit("battle_close", traceLine{"ticks": g.tick - s.openTick, "auto": s.auto})
	g.Image(data.CG{})
	if s.dr.winSong != "" && g.baseSong == s.dr.winSong {
		g.holdWinSong(s.dr.winSong)
	} else {
		g.playLocationMusic()
	}
	g.lastMood = ""
	s.onEnd(res)
	if res == event.BattleWon && !g.replaying {
		g.events.Won()
	}
}

func (g *Game) endMusic(b *battle.Battle) (mood string, fanfare bool) {
	mood = g.ruleMood("battle", b.Env(nil))
	return mood, b.Result == battle.Won && mood == ""
}

func (g *Game) ruleMood(ctx string, env expr.Env) string {
	for _, r := range g.db.Music.Rules[ctx] {
		ok, err := r.When.Bool(env)
		if err == nil && ok && g.hasMood(r.Mood) {
			return r.Mood
		}
	}
	return ""
}

func (s *battleScene) unitRect(g *Game, u *battle.Unit) image.Rectangle {
	sw, sh := g.screen()
	if !u.Enemy {
		n := len(s.b.Party)
		w := math.Min(300, (sw-72)/float64(n))
		x := sw/2 - w*float64(n)/2 + float64(u.Index)*w
		return image.Rect(int(x), int(sh-150), int(x+w-12), int(sh-14))
	}
	if r, ok := s.foeBoxes(g)[u]; ok {
		return r.rect
	}
	return image.Rectangle{}
}

type foeBox struct {
	rect      image.Rectangle
	cx        float64
	bottom, h float64
}

const (
	foeLeft   = 300.0
	foeTop    = 156.0
	foeBigMax = 400.0
)

func (s *battleScene) foeBoxes(g *Game) map[*battle.Unit]foeBox {
	sw, sh := g.screen()
	type slot struct {
		w, h, bottom float64
		person       bool
	}
	var slots []slot
	total := 0.0
	for _, f := range s.b.Foes {
		img := g.pics.get(f.Def.Image.File)
		iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
		scale := f.Def.Scale
		if scale <= 0 {
			scale = 1
		}
		var sl slot
		switch {
		case ih >= 600 && ih >= 1.6*iw:
			sl.h = sh * 1.05 * scale
			sl.w = iw * sl.h / ih * 0.6
			sl.person = true
		case ih >= 600 || f.Def.Boss:
			sl.bottom = sh - 160
			sl.h = math.Min(foeBigMax*scale, sl.bottom-foeTop)
			sl.w = iw * sl.h / ih * 0.8
		default:
			sl.bottom = sh * 0.66
			sl.h = math.Min(ih, 240) * scale
			sl.w = iw * sl.h / ih
		}
		slots = append(slots, sl)
		total += sl.w
	}
	gap := 30.0
	total += gap * float64(max(0, len(slots)-1))
	room := sw - 36 - foeLeft
	if total > room {
		k := room / total
		total = 0
		for i := range slots {
			slots[i].w *= k
			slots[i].h *= k
			total += slots[i].w
		}
		gap *= k
		total += gap * float64(max(0, len(slots)-1))
	}
	out := map[*battle.Unit]foeBox{}

	x := math.Max(foeLeft, math.Min(sw/2-total/2, sw-36-total))
	for i, f := range s.b.Foes {
		sl := slots[i]
		b := foeBox{cx: x + sl.w/2, h: sl.h, bottom: sl.bottom}
		if sl.person {
			b.bottom = personTop + sl.h
			b.rect = image.Rect(int(x), int(personTop), int(x+sl.w), int(math.Min(personTop+sl.h*0.75, sh*0.72)))
		} else {
			b.rect = image.Rect(int(x), int(sl.bottom-sl.h), int(x+sl.w), int(sl.bottom))
		}
		out[f] = b
		x += sl.w + gap
	}
	return out
}

const personTop = 150

func (s *battleScene) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	if ds, ok := g.stack[0].(*dungeonScene); ok && s.bg.File == "" && ds.view != nil {
		op := &ebiten.DrawImageOptions{}
		op.ColorScale.Scale(0.6, 0.6, 0.65, 1)
		dst.DrawImage(ds.view, op)
	} else {
		g.drawPicture(dst, s.bg, 0, 0, sw, sh, 1)
	}
	fill(dst, withAlpha(colorBlack, 0x30))
	if s.introT < introTicks {

		p := float64(s.introT) / introTicks
		off := (1 - p) * (1 - p) * -120
		tmp := s.foeLayer(g)
		tmp.Clear()
		s.drawFoes(g, tmp)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(0, off*g.ui)
		op.ColorScale.ScaleAlpha(float32(p))
		dst.DrawImage(tmp, op)
	} else {
		s.drawFoes(g, dst)
	}
	talking := g.talk.active
	if !talking {

		s.drawParty(g, dst)
		s.drawBossPlates(g, dst)
	}
	s.drawEffects(g, dst)
	s.drawCharge(g, dst)

	if len(s.msg) > 0 && !talking {
		g.window(dst, 36, 20, sw-72, 132)
		for i, m := range s.msg {
			g.text(dst, m, 66, 36+float64(i)*36, sizeM, g.th.text)
		}
	}
	for _, p := range s.pops {
		size, lift := g.popSize(p)
		rise := math.Min(1, float64(p.t)/12)
		y := p.y - 50*rise - lift
		if p.t < 12 && !p.big {
			y -= math.Sin(float64(p.t)/12*math.Pi) * 18
		}
		a := float32(1)
		if p.t > popFade {
			a = float32(1 - float64(p.t-popFade)/20)
		}
		gfx.DrawTextOutlineColor(dst, p.text, p.x, y, size, p.color, colorBlack, a)
	}
	if s.phase == bpInput {
		s.drawInput(g, dst)
		s.drawAim(g, dst)
	}
	s.drawIntro(g, dst)
	if s.cut != nil {
		s.drawCutIn(g, dst)
	}
	if s.auto {
		gfx.DrawTextOutlineColor(dst, lang.T("AUTO  (X でかいじょ)"), sw-150, 164, sizeS, g.th.accent, g.th.window, 1)
	}
	if !talking {
		r := s.reasonButton(g)
		g.window(dst, float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
		g.text(dst, lang.T("判断理由 B / L1"), float64(r.Min.X)+16, float64(r.Min.Y)+10, sizeS, g.th.text)
	}
	if s.phase == bpResult && s.dr.bossWon {
		s.drawBossWin(g, dst)
	} else if s.phase == bpResult && len(s.results) > 0 {
		g.window(dst, sw/2-380, sh*0.36, 760, 110)
		gfx.DrawText(dst, s.results[0], sw/2, sh*0.36+34, sizeL, g.th.text)
	}
}

func (s *battleScene) drawFoes(g *Game, dst *ebiten.Image) {
	boxes := s.foeBoxes(g)
	for _, u := range s.b.Foes {

		gone, isGone := s.gone[u]
		if isGone && gone >= 40 {
			continue
		}
		box := boxes[u]
		r := box.rect
		path := u.PictureAt(s.shownHP[u])
		if f := u.Face(); f != "" {
			path = u.Def.Faces[f]
		}
		if rc := s.react[u]; rc.t > 0 && u.Def.Faces[rc.face] != "" && !isGone {
			path = u.Def.Faces[rc.face]
		}
		for _, pending := range s.queue {
			if pending.Kind == battle.Transform && pending.Target == u {
				path = pending.Before
				break
			}
		}
		if s.foeImages == nil {
			s.foeImages = map[*battle.Unit]string{}
		}
		if old := s.foeImages[u]; old != "" && old != path && s.foeFade[u].ticks == 0 {
			if s.foeFade == nil {
				s.foeFade = map[*battle.Unit]pictureFade{}
			}
			s.foeFade[u] = pictureFade{from: old, ticks: 18}
		}
		s.foeImages[u] = path
		cs := &ebiten.ColorScale{}
		tint(cs, u.Def.Image)
		a := float32(1)
		for _, pending := range s.queue {
			if pending.Kind == battle.Merge && pending.Target == u {
				a = 0
				break
			}
		}
		if s.mergeTarget == u && s.mergeTicks > 12 {
			a = float32(24-s.mergeTicks) / 12
		}
		if isGone {
			a = 1 - float32(gone)/40
			if !u.Fled {
				cs.Scale(1, 0.5, 0.5, 1)
			}
		}
		x := box.cx
		if n := s.shakeU[u]; n > 0 {
			x += math.Sin(float64(n)*2.1) * float64(n) * 0.9
		}
		bob := 0.0
		phase := float64(g.tick+u.Index*19) / 25
		switch u.Def.Motion {
		case "bob":
			bob -= math.Sin(phase) * 3
		case "sway":
			x += math.Sin(phase) * 3
		}
		if s.actor == u {
			bob += 10 // a short step toward the party
		}
		fh := box.h * s.chargeScale(u)
		if u.Def.Motion == "breathe" {
			fh *= 1 + 0.012*math.Sin(phase)
		}
		fade := s.foeFade[u]
		if fade.form && fade.ticks > 16 {
			shadow := &ebiten.ColorScale{}
			shadow.Scale(0.08, 0.08, 0.12, 1)
			g.drawStanding(dst, fade.from, x, box.bottom+bob, fh, a, false, shadow)
		} else {
			newAlpha := a
			if fade.form && fade.ticks > 0 {
				newAlpha *= float32(16-fade.ticks) / 16
			}
			if p := g.enemyPuppet(u, path, s.shownHP[u]); p != nil {
				p.Motion.Speak(s.actor == u)
				g.drawPuppet(dst, p, x, box.bottom+bob, fh, newAlpha, false)
			} else {
				g.drawStanding(dst, path, x, box.bottom+bob, fh, newAlpha, false, cs)
			}
			if fade.ticks > 0 && fade.from != "" && fade.from != path {
				divisor := float32(18)
				if fade.form {
					divisor = 16
				}
				g.drawStanding(dst, fade.from, x, box.bottom+bob, fh, a*float32(fade.ticks)/divisor, false, cs)
			}
		}
		if n := s.flashU[u]; n > 0 && n%4 < 2 {
			g.drawStandingAdd(dst, path, x, box.bottom+bob, fh)
		}

		aimed := s.target != nil && s.target.enemies && s.target.pool[s.target.sel] == u
		if !u.Def.Boss && !isGone && !aimed && len(s.shownStates(u)) > 0 {
			st := s.shownStates(u)
			bw := g.stateTagsWidth(st)
			g.drawStateBadges(dst, st, box.cx-bw/2, math.Max(foeTop+4, float64(r.Min.Y)-30))
		}
	}
	for _, m := range s.mergeGhosts {
		progress := float64(18-m.remaining) / 18
		x := m.fromX + (m.toX-m.fromX)*progress
		g.drawStanding(dst, m.path, x, m.bottom, m.height*(1-0.3*progress), float32(1-progress), false, nil)
	}
	for _, pending := range s.queue {
		if pending.Kind != battle.Merge {
			continue
		}
		sw, sh := g.screen()
		oldCount := len(s.b.Foes) + len(pending.Members) - 1
		for _, member := range pending.Members {
			x := sw/2 + (float64(member.Index)-float64(oldCount-1)/2)*min(180, (sw-foeLeft)/float64(oldCount))
			g.drawStanding(dst, member.Picture(), x, sh*0.66, 160, 1, false, nil)
		}
	}
}

func (s *battleScene) drawBossPlates(g *Game, dst *ebiten.Image) {
	y := foeTop + 8
	for _, u := range s.b.Foes {
		if _, gone := s.gone[u]; !u.Def.Boss || gone || u.Fled {
			continue
		}
		const w = 250.0
		rate := s.hp[u] / float64(u.MHP())

		st := s.shownStates(u)

		room := 0
		if s.cmd != nil {
			room = max(1, int((s.cmd.y-8-(y+76))/26))
		}
		rows := g.stateTagRows(st, w-36, room)
		h := 76 + float64(len(rows))*26
		g.window(dst, 36, y, w, h)
		g.text(dst, gfx.Ellipsis(u.Name, sizeS, w-36), 54, y+8, sizeS, g.th.text)
		bar(dst, 54, y+44, w-36, 14, rate, hpColor(rate))
		g.drawTagRows(dst, rows, 54, y+68, 26)
		y += h + 8
	}
}

func (s *battleScene) drawAim(g *Game, dst *ebiten.Image) {
	t := s.target
	if t == nil || !t.enemies {
		return
	}
	u := t.pool[t.sel]
	box := s.foeBoxes(g)[u]
	r := box.rect
	line := s.known(g, u)
	if st := s.stateNames(g, u); st != "" {
		if line != "" {
			line += "　"
		}
		line += st
	}
	room := 46.0
	if line != "" {
		room = 74
	}
	my := math.Max(float64(r.Min.Y)-46, foeTop+room)
	if my > float64(r.Max.Y)-60 {
		my = float64(r.Max.Y) - 60
	}
	x := box.cx
	bobT := math.Sin(float64(g.tick)/5) * 6
	gfx.DrawTextOutlineColor(dst, "▼", x, my+bobT, sizeL, g.th.accent, colorWhite, 1)
	gfx.DrawTextOutlineColor(dst, u.Name, x, my-38, sizeM, g.th.text, g.th.window, 1)
	if line != "" {
		gfx.DrawTextOutlineColor(dst, line, x, my-66, sizeS, color.NRGBA{0xff, 0xd0, 0x50, 0xff}, g.th.window, 1)
	}
}

func (g *Game) drawStandingAdd(dst *ebiten.Image, path string, cx, bottom, h float64) {
	img := g.pics.get(path)
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	s := h / ih
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear, Blend: ebiten.BlendLighter}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx-iw*s/2, bottom-h)
	op.ColorScale.Scale(0.7, 0.7, 0.7, 0.7)
	gfx.DrawImage(dst, img, op)
}

func (s *battleScene) drawParty(g *Game, dst *ebiten.Image) {
	t := g.db.Game.Terms
	var cur *battle.Unit
	if s.phase == bpInput && s.mi < len(s.members) {
		cur = s.members[s.mi]
	}
	party := s.b.Party
	if s.coverBy != nil {
		party = append([]*battle.Unit{}, party...)
		for i, u := range party {
			if u == s.coverBy {
				party = append(append(party[:i:i], party[i+1:]...), u)
				break
			}
		}
	}
	for ui, u := range party {
		r := s.unitRect(g, u)
		x, y := float64(r.Min.X), float64(r.Min.Y)
		w, h := float64(r.Dx()), float64(r.Dy())
		if n := s.shakeU[u]; n > 0 {
			x += math.Sin(float64(n)*2.1) * float64(n) * 0.7
		}
		if u == cur || s.actor == u {
			y -= 14
		}
		if u == s.coverBy && s.coverFor != nil {
			to := s.unitRect(g, s.coverFor)
			k := math.Min(1, float64(s.coverT)/8)
			x += (float64(to.Min.X) - x) * k
			y += (float64(to.Min.Y) - 30 - y) * k
		}
		g.window(dst, x, y, w, h)
		if n := s.flashU[u]; n > 0 {
			gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 16, color.NRGBA{0xff, 0x30, 0x30, uint8(min(255, max(0, n*8)))})
		}
		if u == cur {
			gfx.StrokeRoundRect(dst, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 14, 4, g.th.accent)
		}
		if tg := s.target; tg != nil && !tg.enemies && tg.pool[tg.sel] == u {
			gfx.StrokeRoundRect(dst, float32(x+2), float32(y+2), float32(w-4), float32(h-4), 14, 5, colorWhite)
			gfx.DrawTextOutlineColor(dst, "▼", x+w/2, y-40+math.Sin(float64(g.tick)/5)*5, sizeL, g.th.accent, colorWhite, 1)
		}
		face := s.faceOf(u)
		facePath := g.faceOf(u.Actor.ID, face)
		if s.foeImages == nil {
			s.foeImages = map[*battle.Unit]string{}
		}
		if old := s.foeImages[u]; old != "" && old != facePath && s.foeFade[u].ticks == 0 {
			if s.foeFade == nil {
				s.foeFade = map[*battle.Unit]pictureFade{}
			}
			s.foeFade[u] = pictureFade{from: old, ticks: 18}
		}
		s.foeImages[u] = facePath
		g.drawFace(dst, facePath, x+12, y+12, h-24, 1, s.shownHP[u] <= 0)
		if f := s.foeFade[u]; f.ticks > 0 && f.from != facePath {
			g.drawFace(dst, f.from, x+12, y+12, h-24, float32(f.ticks)/18, s.shownHP[u] <= 0)
		}
		tx, tw := x+h, w-h-14
		g.text(dst, u.Name, tx, y+10, sizeM*0.9, g.th.text)
		hr := s.hp[u] / float64(u.MHP())
		g.text(dst, fmt.Sprintf("%s %d", t.HP, s.shownHP[u]), tx, y+44, sizeS*0.85, g.th.text)
		bar(dst, tx, y+70, tw, 9, hr, hpColor(hr))
		g.text(dst, fmt.Sprintf("%s %d", t.MP, s.shownMP[u]), tx, y+82, sizeS*0.85, g.th.text)
		bar(dst, tx, y+106, tw, 7, s.mp[u]/float64(max(1, u.MMP())), mpColor)
		if u.Guard {
			gfx.DrawTextOutlineColor(dst, "GUARD", x+w-48, y-14, sizeS*0.8, g.th.border, g.th.window, 1)
		}

		g.drawTagRows(dst, g.stateTagRows(s.shownStates(u), h-24, 3), x+12, y+h-30, -26)
		g.drawCardBubbles(dst, u.Actor.ID, s.shownStates(u), ui, x, y, w, h)
	}
}

func (s *battleScene) faceOf(u *battle.Unit) string {
	d := u.Actor.Def()

	alive := s.shownHP[u] > 0
	if r := s.react[u]; r.t > 0 && (alive || r.face == d.Reactions["fall"]) {
		return r.face
	}
	if !alive {
		if f := d.Reactions["fall"]; f != "" {
			return f
		}
	}
	env := expr.Map{"self": shownUnit{u, s}}
	for _, r := range d.FaceRule {
		if ok, err := r.When.Bool(env); err == nil && ok {
			return r.Face
		}
	}
	face := d.Face
	for _, img := range d.Images {
		if float64(s.shownHP[u])/float64(u.MHP()) < img.Below {
			face = img.Face
		}
	}
	return face
}

type shownUnit struct {
	u *battle.Unit
	s *battleScene
}

func (v shownUnit) Field(name string) (any, bool) {
	hp, ok := v.s.shownHP[v.u]
	if !ok {
		return v.u.Field(name)
	}
	mp := v.s.shownMP[v.u]
	switch name {
	case "hp":
		return float64(hp), true
	case "mp":
		return float64(mp), true
	case "hp_rate":
		return float64(hp) / float64(max(1, v.u.MHP())), true
	case "mp_rate":
		return float64(mp) / float64(max(1, v.u.MMP())), true
	case "alive":
		return hp > 0, true
	case "dead":
		return hp <= 0, true
	}
	return v.u.Field(name)
}

func (s *battleScene) drawInput(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	u := s.members[s.mi]
	s.cmd.draw(g, dst, s.sub == nil && s.target == nil)

	if s.sub == nil && s.target == nil {
		if line := s.stateTurns(g, u); line != "" {
			ls, sz := gfx.FitLines(line, sizeS*0.85, sw-s.cmd.x-s.cmd.w-60-48, 1)
			if len(ls) > 0 {
				w := gfx.TextWidth(ls[0], sz) + 44
				x, y := s.cmd.x+s.cmd.w+12, s.cmd.y+s.cmd.height()-46
				g.window(dst, x, y, w, 46)
				g.text(dst, ls[0], x+22, y+(46-sz*1.3)/2, sz, g.th.text)
			}
		}
	}
	if s.sub != nil {
		s.sub.draw(g, dst, s.target == nil)
		id := s.subIDs[s.sub.sel]
		desc := ""
		if sk := s.b.DB.Skills[id]; s.subKind == "skill" && sk != nil {
			desc = sk.Desc
		} else if it := s.b.DB.Items[id]; it != nil {
			desc = it.Desc
		}
		if desc != "" {
			x := s.sub.x
			w := math.Min(sw-x-36, 700)
			ls, sz := gfx.FitLines(desc, sizeS, w-48, 3)
			h := 32 + float64(len(ls))*gfx.LineHeight(sz)
			g.window(dst, x, s.sub.y-10-h, w, h)
			for i, ln := range ls {
				g.text(dst, ln, x+24, s.sub.y-10-h+14+float64(i)*gfx.LineHeight(sz), sz, g.th.text)
			}
		}
	}
}

func (s *battleScene) overlay() bool { return false }

func (g *Game) tempo(n int) int {
	t := g.db.Game.Battle.Tempo
	if t <= 0 {
		t = 1
	}
	return max(1, int(float64(n)/(t*g.cfg.battleScale())))
}

func (g *Game) autoTempo() float64 {
	return max(1, g.db.Game.Battle.AutoTempo)
}

func (g *Game) autoStartBattle() bool {
	if g.rep != nil && g.rep.version == 1 {
		return false
	}
	if g.cfg.BattleAuto != nil {
		return *g.cfg.BattleAuto
	}
	return g.db.Game.Battle.AutoStart
}

func (g *Game) battleAutoTalk() bool {
	b := g.currentBattle()
	return b != nil && b.auto && b.phase == bpEvent
}

func battleLine(party []*battle.Unit, r event.Say) (event.Say, bool) {
	if !downSpeaker(party, r) && (r.Actor == "" || inParty(party, r.Actor)) {
		return r, true
	}
	for _, alt := range r.Instead {
		if alt.Actor != "" && !inParty(party, alt.Actor) {
			continue
		}
		if !downSpeaker(party, alt) {
			return alt, true
		}
	}
	return event.Say{}, false
}

func downSpeaker(party []*battle.Unit, r event.Say) bool {
	for _, u := range party {
		if u.Actor == nil || u.Alive() {
			continue
		}
		if (r.Actor != "" && u.Actor.ID == r.Actor) || (r.Actor == "" && r.Name != "" && r.Name == u.Name) {
			return true
		}
	}
	return false
}

func inParty(party []*battle.Unit, id string) bool {
	for _, u := range party {
		if u.Actor != nil && u.Actor.ID == id {
			return true
		}
	}
	return false
}
