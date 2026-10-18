package designeval

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/cmd/rpgtool/internal/balance"
	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/rpg"
	"github.com/nao1215/gozen/world"
)

type Event struct {
	T      float64 `json:"t"`
	Day    int     `json:"day"`
	Place  string  `json:"place"`
	Kind   string  `json:"kind"`
	Reward bool    `json:"reward,omitempty"`
	Expect bool    `json:"expect,omitempty"`
	Label  string  `json:"label,omitempty"`
	Who    string  `json:"who,omitempty"`
	Flow   string  `json:"flow,omitempty"`
	Troop  string  `json:"troop,omitempty"`
}

type Segment struct {
	From, To float64
	Place    string
	Dungeon  bool
	Floor    int
}

type Boss struct {
	Floor    int
	Troop    string
	Name     string
	Rate     float64
	Level    int
	RateEnd  float64
	Grind    int
	Tries    int
	Wins     int
	Measured bool
	At       *Snap `json:"-"`
}

type DuelRec struct {
	Troop      string  `json:"troop"`
	Name       string  `json:"name"`
	First      float64 `json:"first"`
	FirstLevel int     `json:"first_level"`
	FirstDay   int     `json:"first_day"`
	Opened     bool    `json:"opened"`
	Again      float64 `json:"again"`
	Later      float64 `json:"later"`
	LaterLevel int     `json:"later_level"`
	LaterDay   int     `json:"later_day"`
	Grown      bool    `json:"grown"`
	Tries      int     `json:"tries"`
	Wins       int     `json:"wins"`
	At         *Snap   `json:"-"`
	LaterAt    *Snap   `json:"-"`
}

type Snap struct {
	T        float64
	Day      int
	Location string
	Place    string
	Floor    int
	Pos      []int
	Facing   int
	Vars     map[string]any
	Save     []byte
	Rand     []byte
	Note     string
}

const SnapEvery = 120

type Run struct {
	Persona    Persona
	Events     []Event
	Segments   []Segment
	Bosses     []*Boss
	Duels      []*DuelRec
	Cleared    bool
	ClearT     float64
	End        float64
	Days       int
	Battles    int
	Lost       int
	Retries    int
	Wipes      int
	Stranded   int
	ReleaseSec float64
	Deepest    int
	Level      int
	Problems   []string
	Snaps      []Snap
	Stuck      []Snap
	Seed       uint64
}

const marker = "\x00event:"

func (b *bot) logf(format string, a ...any) {
	if b.c.Debug != nil {
		b.c.Debug("%s d%d t%.0f %s: "+format, append([]any{b.p.ID, b.day, b.t / 60, b.where()}, a...)...)
	}
}

type bot struct {
	db              *data.DB
	c               *Config
	p               Persona
	d               *headless.Driver
	st              *rpg.State
	run             *Run
	t               float64
	day             int
	ctx             []string
	rot             []string
	accept          string
	used            map[string]int
	seen            map[string]bool
	eventConditions []map[*Rule]bool
	prev            map[*Rule]float64
	know            map[string]map[string]bool
	errs            map[string]bool

	asked        map[string]int
	battleQ      []float64
	worn         bool
	policyBusy   bool
	policyFailed map[string]bool
	noRetry      map[string]bool
	bosses       map[int]*Boss
	gateDone     map[string]bool
	doneToday    map[string]bool
	rewardCounts map[string]int
	releaseCount int
	pcg          *rand.PCG
}

func (b *bot) snap(note string) Snap {
	s := Snap{T: b.t, Day: b.day, Location: b.st.Location, Place: b.where(), Floor: b.floor(), Note: note, Vars: map[string]any{}}
	if b.inDungeon() {
		s.Pos, s.Facing = []int{b.st.Pos.X, b.st.Pos.Y}, b.st.Pos.Facing
	}
	for _, v := range b.db.Game.Memo.Vars {
		if x, err := v.Value.Eval(b.st.Env()); err == nil {
			s.Vars[v.Name] = x
		}
	}
	var err error
	s.Save, err = b.st.Save()
	if err != nil {
		b.note(err)
	}
	s.Rand, err = b.pcg.MarshalBinary()
	if err != nil {
		b.note(err)
	}
	return s
}

func (b *bot) stuck(note string) {
	b.run.Stuck = append(b.run.Stuck, b.snap(note))
}

func Play(db *data.DB, c *Config, p Persona) *Run {
	c.defaults()
	original := map[string][]data.Command{}
	defer func() {
		for id, cmds := range original {
			db.Events[id].Do = cmds
		}
	}()
	for id, ev := range db.Events {
		original[id] = ev.Do
		ev.Do = append([]data.Command{{Image: &data.CG{File: marker + id}}}, ev.Do...)
	}
	seed := p.Seed
	if seed == 0 {
		seed = 1
	}
	pcg := rand.NewPCG(seed, 1)
	st := rpg.New(db, rand.New(pcg)) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
	b := &bot{db: db, c: c, p: p, st: st, run: &Run{Persona: p, Seed: seed}, pcg: pcg,
		rewardCounts: map[string]int{}, used: map[string]int{}, seen: map[string]bool{}, prev: map[*Rule]float64{}, know: map[string]map[string]bool{},
		errs: map[string]bool{}, asked: map[string]int{}, policyFailed: map[string]bool{}, noRetry: noRetryTroops(db),
		bosses: map[int]*Boss{}, gateDone: map[string]bool{}}
	d := headless.New(db, st)
	d.Record = true
	d.OnStep = func(s headless.Step) {
		if s.Kind != "image" || !strings.HasPrefix(s.File, marker) {
			return
		}
		eligible := map[*Rule]bool{}
		for _, rs := range [][]Rule{c.Rewards, c.Expects} {
			for i := range rs {
				r := &rs[i]
				if r.Events != "" && r.re.MatchString(strings.TrimPrefix(s.File, marker)) {
					eligible[r] = !r.When.Set() || b.holds(r.When)
				}
			}
		}
		b.eventConditions = append(b.eventConditions, eligible)
	}
	d.MaxTurns = c.Game.MaxTurns
	d.Choose = b.choose
	d.Result = b.fight
	b.d = d
	for _, du := range c.Duels.List {
		r := &DuelRec{Troop: du.Troop, Name: du.Troop}
		if tr := db.Troops[du.Troop]; tr != nil && tr.Name != "" {
			r.Name = tr.Name
		}
		b.run.Duels = append(b.run.Duels, r)
	}
	b.prime()
	b.run.Snaps = append(b.run.Snaps, b.snap(""))
	b.act(func() error { return d.Start() })
	days := p.Days
	if days == 0 {
		days = c.Game.Days
	}
	for b.day = 1; b.day <= days && !b.run.Cleared; b.day++ {
		b.doneToday = map[string]bool{}
		b.measureDuels()
		b.policies("town")
		b.town()
		if b.run.Cleared {
			break
		}
		b.prepare()
		b.dive()
		if b.run.Cleared {
			break
		}
		for _, l := range c.Game.Evening {
			b.visit(l)
		}
		if !b.policies("night") {
			b.night()
		}
	}
	b.run.End = b.t
	b.run.Days = min(b.day, days)
	b.run.Deepest = int(b.num(c.Game.Deepest))
	b.run.Level = avgLevel(st)
	floors := make([]int, 0, len(b.bosses))
	for f := range b.bosses {
		floors = append(floors, f)
	}
	sort.Ints(floors)
	for _, f := range floors {
		b.run.Bosses = append(b.run.Bosses, b.bosses[f])
	}
	return b.run
}

func noRetryTroops(db *data.DB) map[string]bool {
	out := map[string]bool{}
	var walk func([]data.Command)
	walk = func(cmds []data.Command) {
		for _, c := range cmds {
			if c.Battle != "" && c.Retry != nil && !*c.Retry {
				out[c.Battle] = true
			}
			walk(c.Then)
			walk(c.Else)
			walk(c.OnWin)
			walk(c.OnLose)
			walk(c.OnRun)
			for _, o := range c.Choice {
				walk(o.Do)
			}
		}
	}
	for _, ev := range db.Events {
		walk(ev.Do)
	}
	return out
}

func (b *bot) num(e data.Expr) float64 {
	if !e.Set() {
		return 0
	}
	v, err := e.Number(b.st.Env())
	if err != nil {
		return 0
	}
	return v
}

func (b *bot) holds(e data.Expr) bool {
	if !e.Set() {
		return false
	}
	ok, err := e.Bool(b.st.Env())
	return err == nil && ok
}

func (b *bot) inDungeon() bool {
	l := b.db.Locations[b.st.Location]
	return l != nil && l.Dungeon != nil
}

func (b *bot) where() string {
	l := b.db.Locations[b.st.Location]
	if l == nil {
		return b.st.Location
	}
	if l.Dungeon != nil && b.c.Game.Place != "" {
		return data.Render(b.c.Game.Place, b.st.Env())
	}
	return data.Render(l.Name, b.st.Env())
}

func (b *bot) adv(dt float64, place string) {
	if dt <= 0 {
		return
	}
	segs := &b.run.Segments
	if n := len(*segs); n > 0 && (*segs)[n-1].Place == place && (*segs)[n-1].To == b.t {
		(*segs)[n-1].To += dt
	} else {
		*segs = append(*segs, Segment{From: b.t, To: b.t + dt, Place: place, Dungeon: b.inDungeon(), Floor: int(b.num(b.c.Game.Floor))})
	}
	before := int(b.t / SnapEvery)
	b.t += dt
	if int(b.t/SnapEvery) > before {
		b.run.Snaps = append(b.run.Snaps, b.snap(""))
	}
}

func (b *bot) emit(e Event) {
	e.T = b.t
	e.Day = b.day
	if e.Place == "" {
		e.Place = b.where()
	}
	if e.Reward {
		b.rewardCounts[e.Kind]++
	}
	if e.Reward && e.Kind == b.c.Release {
		b.releaseCount++
	}
	b.run.Events = append(b.run.Events, e)
}

func (b *bot) prime() {
	for _, rs := range [][]Rule{b.c.Rewards, b.c.Expects} {
		for i := range rs {
			if rs[i].Value.Set() {
				b.prev[&rs[i]] = b.num(rs[i].Value)
			}
		}
	}
	for _, a := range b.st.Members() {
		b.know[a.ID] = map[string]bool{}
		for _, s := range a.Skills() {
			b.know[a.ID][s] = true
		}
	}
}

func (b *bot) act(f func() error) {
	b.asked = map[string]int{}
	before := b.where()
	b.note(f())
	b.absorb(before)
	if b.d.Over {
		b.d.Over = false
		b.run.Wipes++
		b.emit(Event{Kind: "wipe"})
		b.stuck("wipe")
		for _, a := range b.st.Members() {
			a.Recover()
		}
		b.d.Restore(b.st)
		if ev := b.db.Events[b.db.Game.GameOver.Event]; ev != nil {
			at := b.where()
			b.note(b.d.Run(context.Background(), ev.Do))
			b.absorb(at)
		}
	}
	if (b.d.Cleared || b.holds(b.c.Game.Cleared)) && !b.run.Cleared {
		b.run.Cleared, b.run.ClearT = true, b.t
		b.emit(Event{Kind: "clear"})
	}
}

func (b *bot) note(err error) {
	if err == nil {
		return
	}
	for _, e := range strings.Split(err.Error(), "\n") {
		if e == "" || b.errs[e] {
			continue
		}
		b.errs[e] = true
		if strings.Contains(e, "no way") || strings.Contains(e, "could not reach") {
			continue
		}
		b.run.Problems = append(b.run.Problems, fmt.Sprintf("%d: %s", b.day, e))
	}
}

type hit struct {
	r     *Rule
	who   string
	t     float64
	place string
}

func (b *bot) absorb(place string) {
	tm := b.c.Time
	t0, h0 := b.t, b.releaseCount
	defer func() {
		if b.releaseCount > h0 {
			b.run.ReleaseSec += b.t - t0
		}
	}()
	var hits []hit
	once := map[string]bool{}
	match := func(r *Rule, s string) {
		m := r.re.FindStringSubmatch(s)
		if m == nil {
			return
		}
		who := r.Who
		if len(m) > 1 && m[1] != "" {
			who = m[1]
			if n, ok := r.Names[m[1]]; ok {
				who = n
			}
		}
		if r.Images != "" {
			if r.First && b.seen[s] {
				return
			}
			key := fmt.Sprintf("%p|%s", r, who)
			if once[key] {
				return
			}
			once[key] = true
		}
		hits = append(hits, hit{r: r, who: who, t: b.t, place: place})
	}
	rules := func(f func(r *Rule)) {
		for _, rs := range [][]Rule{b.c.Rewards, b.c.Expects} {
			for i := range rs {
				f(&rs[i])
			}
		}
	}
	for _, s := range b.d.Steps {
		switch s.Kind {
		case "say":
			b.adv(tm.Line+float64(utf8.RuneCountInString(s.Text))/tm.CharsPerSec, place)
		case "notice":
			b.adv(tm.Notice, place)
		case "choice":
			b.adv(tm.Choice, place)
		case "move":
			b.adv(tm.Move, place)
			if l := b.db.Locations[s.Loc]; l != nil {
				if l.Dungeon != nil && b.c.Game.Place != "" {
					place = data.Render(b.c.Game.Place, b.st.Env())
				} else {
					place = data.Render(l.Name, b.st.Env())
				}
			}
		case "battle":
			if len(b.battleQ) > 0 {
				b.adv(b.battleQ[0], place)
				b.battleQ = b.battleQ[1:]
			}
		case "image":
			if id, ok := strings.CutPrefix(s.File, marker); ok {
				eligible := b.eventConditions[0]
				b.eventConditions = b.eventConditions[1:]
				rules(func(r *Rule) {
					if r.Events != "" && eligible[r] {
						match(r, id)
					}
				})
				continue
			}
			if s.File == "" {
				continue
			}
			for _, it := range tm.Images {
				if it.re.MatchString(s.File) {
					b.adv(it.Sec, place)
					break
				}
			}
			rules(func(r *Rule) {
				if r.Images != "" {
					match(r, s.File)
				}
			})
			b.seen[s.File] = true
		}
	}
	b.d.Steps = b.d.Steps[:0]
	for len(b.battleQ) > 0 {
		b.adv(b.battleQ[0], place)
		b.battleQ = b.battleQ[1:]
	}
	kinds := map[string]bool{}
	for _, h := range hits {
		kinds[h.r.Kind] = true
	}
	for _, h := range hits {
		if h.r.Unless != "" && kinds[h.r.Unless] {
			continue
		}
		b.hitEvent(h)
	}

	now := b.where()
	rules(func(r *Rule) {
		if !r.Value.Set() {
			return
		}
		v := b.num(r.Value)
		if v > b.prev[r] {
			b.hitEvent(hit{r: r, who: r.Who, t: b.t, place: now})
		}
		b.prev[r] = v
	})
	for _, a := range b.st.Members() {
		k := b.know[a.ID]
		if k == nil {
			k = map[string]bool{}
			b.know[a.ID] = k
		}
		for _, s := range a.Skills() {
			if k[s] {
				continue
			}
			k[s] = true
			rules(func(r *Rule) {
				if r.Skills {
					b.hitEvent(hit{r: r, who: a.Name(), t: b.t, place: now})
				}
			})
		}
	}
}

func (b *bot) isExpect(r *Rule) bool {
	for i := range b.c.Expects {
		if &b.c.Expects[i] == r {
			return true
		}
	}
	return false
}

func (b *bot) hitEvent(h hit) {
	e := Event{Kind: h.r.Kind, Label: h.r.Label, Flow: h.r.Flow, Who: h.who, Place: h.place}
	if b.isExpect(h.r) {
		e.Expect = true
	} else {
		e.Reward = true
	}
	t := b.t
	b.t = h.t
	b.emit(e)
	b.t = t
}

func hasAny(s string, words []string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func (b *bot) avoid(words []string) []string {
	out := slices.Clone(b.p.Avoid)
	for _, w := range append(slices.Clone(b.c.Game.Never), b.c.Game.OptIn...) {
		if !slices.Contains(words, w) {
			out = append(out, w)
		}
	}
	return out
}

func (b *bot) choose(_ *rpg.State, q event.Choice) int {
	if len(q.Options) == 0 {
		return 0
	}
	key := q.Prompt + "\x00" + strings.Join(q.Options, "\x00")
	b.asked[key]++
	if b.asked[key] > 2 {
		return len(q.Options) - 1
	}
	n := b.pickIDs(q.Options, q.IDs, b.ctx, b.rot)
	b.logf("choice %q %q -> %d", q.Prompt, q.Options, n)
	return n
}

func (b *bot) pickIDs(opts, ids, words, rot []string) int {
	match := func(i int, w string) bool {
		if id, ok := strings.CutPrefix(w, "@"); ok {
			return i < len(ids) && ids[i] == id
		}
		return w != "" && strings.Contains(opts[i], w)
	}
	avoid := b.avoid(words)
	var cands []int
	for i := range opts {
		blocked := false
		for _, w := range avoid {
			blocked = blocked || match(i, w)
		}
		if !blocked {
			cands = append(cands, i)
		}
	}
	if len(cands) == 0 {
		return len(opts) - 1
	}
	if mark := b.accept; mark != "" {
		var ok []int
		for _, i := range cands {
			if strings.Contains(opts[i], mark) {
				ok = append(ok, i)
			}
		}
		if len(ok) > 0 {
			cands = ok
		}
	}
	if len(rot) > 0 {

	narrow:
		for _, w := range words {
			var in []int
			for _, i := range cands {
				if match(i, w) {
					in = append(in, i)
				}
			}
			if len(in) > 0 {
				cands = in
				break narrow
			}
		}
		best, bestN, bw := -1, 1<<30, ""
		for _, i := range cands {
			for _, w := range rot {
				if match(i, w) && b.used[w] < bestN {
					best, bestN, bw = i, b.used[w], w
				}
			}
		}
		if best >= 0 {
			b.used[bw]++
			return best
		}
	}
	all := append(append(slices.Clone(words), b.p.Prefer...), b.c.Game.Prefer...)
	for _, w := range all {
		for _, i := range cands {
			if match(i, w) {
				return i
			}
		}
	}
	return cands[0]
}

func (b *bot) with(words, rot []string, mark string, f func()) {
	pw, pr, ph := b.ctx, b.rot, b.accept
	b.ctx, b.rot, b.accept = words, rot, mark
	f()
	b.ctx, b.rot, b.accept = pw, pr, ph
}

func (b *bot) fight(st *rpg.State, troop string) (int, bool) {
	tm := b.c.Time
	tries := 1
	if b.db.Game.Battle.Retry && !b.noRetry[troop] {
		tries += b.c.Game.Retry
	}
	var snap []byte
	if tries > 1 {
		var err error
		snap, err = st.Save()
		if err != nil {
			b.note(err)
			return event.BattleLost, true
		}
	}
	boss := b.bossOf(troop)
	duel := b.duelOf(troop)
	sec := 0.0
	defer func() { b.battleQ = append(b.battleQ, sec) }()
	for i := 0; ; i++ {
		bt := battle.New(b.db, st, troop)
		headless.AutoBattle(b.db, st, bt, nil, b.c.Game.MaxTurns, nil)
		if bt.Result == battle.Ongoing {
			bt.Result = battle.Lost
		}
		bt.End()
		b.run.Battles++
		sec += tm.Battle + float64(bt.Turn)*tm.BattleTurn
		if boss != nil {
			boss.Tries++
		}
		if duel != nil {
			duel.Tries++
			if bt.Result == battle.Won {
				duel.Wins++
			}
		}
		switch bt.Result {
		case battle.Won:
			if boss != nil {
				boss.Wins++
				b.emit(Event{Kind: "boss", Troop: troop, Label: boss.Name})
			}
			return event.BattleWon, true
		case battle.Fled:
			return event.BattleFled, true
		case battle.Ongoing, battle.Lost:
		}
		b.run.Lost++
		b.emit(Event{Kind: "lost", Troop: troop})
		if i+1 < tries && snap != nil {
			n, err := rpg.Parse(b.db, snap, st.Rand)
			if err == nil {
				fighters, calm := st.Fighters, st.Calm
				*st = *n
				st.Fighters, st.Calm = fighters, calm
				b.run.Retries++
				sec += tm.Retry
				continue
			}
		}
		bt.Rise()
		return event.BattleLost, true
	}
}

func (b *bot) duelOf(troop string) *DuelRec {
	for i, d := range b.c.Duels.List {
		if d.Troop == troop || d.Rematch == troop {
			return b.run.Duels[i]
		}
	}
	return nil
}

func (b *bot) measureDuels() {
	ds := b.c.Duels
	lv := func() int {
		s, n := 0, 0
		for _, id := range ds.Fighters {
			if a := b.st.Actors[id]; a != nil {
				s += a.Level
				n++
			}
		}
		return s / max(1, n)
	}

	var geared *rpg.State
	st := func() *rpg.State {
		if geared != nil {
			return geared
		}
		save, err := b.st.Save()
		if err != nil {
			geared = b.st
			return geared
		}
		c, err := rpg.Parse(b.db, save, rand.New(rand.NewPCG(uint64(b.day), 5))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		if err != nil {
			geared = b.st
			return geared
		}
		c.Location = b.st.Location
		d := headless.New(b.db, c)
		for _, s := range b.c.Game.Shops {
			d.Shop(s, nil)
		}
		d.EquipBest()
		if b.c.Game.Stones {
			if best, score, now := c.BestStones(); score > now {
				c.ApplyStones(best)
			}
		}
		for _, a := range c.Members() {
			a.SpendSP()
		}
		geared = c
		return geared
	}
	for i, d := range ds.List {
		r := b.run.Duels[i]
		t := d.Rematch
		if t == "" {
			t = d.Troop
		}
		if !r.Opened && b.holds(d.Open) {
			r.Opened, r.FirstDay, r.FirstLevel = true, b.day, lv()
			sn := b.snap("")
			r.At = &sn
			r.First = b.duelRateNow(b.db, st(), d.Troop, ds.Fighters, ds.Tries, b.c.Game.MaxTurns)
			r.Again = b.duelRateNow(b.db, st(), t, ds.Fighters, ds.Tries, b.c.Game.MaxTurns)
		}
		if r.Opened && !r.Grown && b.holds(d.Later) {
			r.Grown, r.LaterDay, r.LaterLevel = true, b.day, lv()
			sn := b.snap("")
			r.LaterAt = &sn
			r.Later = b.duelRateNow(b.db, st(), t, ds.Fighters, ds.Tries, b.c.Game.MaxTurns)
		}
	}
}

func (b *bot) bossOf(troop string) *Boss {
	for f, t := range b.c.Game.Bosses {
		if t == troop {
			return b.boss(f)
		}
	}
	return nil
}

func (b *bot) boss(f int) *Boss {
	if r := b.bosses[f]; r != nil {
		return r
	}
	t := b.c.Game.Bosses[f]
	r := &Boss{Floor: f, Troop: t, Name: t}
	if tr := b.db.Troops[t]; tr != nil && tr.Name != "" {
		r.Name = tr.Name
	}
	b.bosses[f] = r
	return r
}

func partyHP(st *rpg.State) float64 {
	hp, m := 0, 0
	for _, a := range st.Members() {
		hp += max(0, a.HP)
		m += a.Stat("mhp")
	}
	return float64(hp) / float64(max(1, m))
}

func partyMP(st *rpg.State) float64 {
	mp, m := 0, 0
	for _, a := range st.Members() {
		mp += max(0, a.MP)
		m += a.Stat("mmp")
	}
	return float64(mp) / float64(max(1, m))
}

func avgLevel(st *rpg.State) int {
	s, n := 0, 0
	for _, a := range st.Members() {
		s += a.Level
		n++
	}
	return s / max(1, n)
}

func (b *bot) entriesAt(loc string) []world.Entry {
	if b.db.Locations[loc] == nil {
		return nil
	}
	here := b.st.Location
	b.st.Location = loc
	defer func() { b.st.Location = here }()
	return world.Entries(b.db, b.st)
}

func (b *bot) goTo(loc string) bool {
	if b.st.Location == loc {
		return true
	}
	b.act(func() error { return b.d.GoTo(loc) })
	return b.st.Location == loc
}

func (b *bot) townLocations() []string {
	if len(b.c.Game.Town) > 0 {
		return b.c.Game.Town
	}
	var out []string
	for _, id := range b.db.Ordered("locations") {
		l := b.db.Locations[id]
		if l == nil || l.Dungeon != nil || id == b.c.Game.Inn || slices.Contains(b.c.Game.Evening, id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func (b *bot) town() {
	for _, l := range b.townLocations() {
		if b.run.Cleared {
			return
		}
		b.visit(l)
	}
}

func (b *bot) visit(loc string) {
	for range 10 {
		if b.run.Cleared {
			return
		}
		label, key, words, rot, ok := b.nextEntry(loc)
		if !ok || !b.goTo(loc) {
			return
		}
		b.doneToday[key] = true
		var e *world.Entry
		for _, x := range world.Entries(b.db, b.st) {
			if entryMatches(x, label) {
				e = &x
				break
			}
		}
		if e == nil {
			continue
		}
		b.adv(b.c.Time.Menu, b.where())
		b.with(words, rot, "", func() {
			b.act(func() error { return b.d.Run(context.Background(), e.Commands(b.db, b.st)) })
		})
		if b.st.Location != loc {
			return
		}
	}
}

func (b *bot) nextEntry(loc string) (label, key string, words, rot []string, ok bool) {
	es := b.entriesAt(loc)
	avoid := append(slices.Clone(b.p.Avoid), b.c.Game.NotByDay...)
	for _, e := range es {
		if e.Kind != world.Talk && e.Kind != world.Do || !e.New {
			continue
		}
		k := loc + "|" + entrySelector(e)
		if b.doneToday[k] || hasAny(e.Label, avoid) {
			continue
		}
		return entrySelector(e), k, nil, nil, true
	}
	for i, v := range b.p.Town {
		k := fmt.Sprintf("%s|visit%d", loc, i)
		if v.At != loc || b.doneToday[k] || (v.When.Set() && !b.holds(v.When)) {
			continue
		}
		for _, e := range es {
			if (e.Kind == world.Talk || e.Kind == world.Do) && entryMatches(e, v.Do) {
				return entrySelector(e), k, v.Words, v.Rotate, true
			}
		}
	}
	return "", "", nil, nil, false
}

func (b *bot) prepare() {
	g := b.c.Game
	gold := b.st.Gold
	changed := false
	for _, s := range g.Shops {
		b.d.Shop(s, nil)
	}
	if g.SupplyShop != "" {
		ids := make([]string, 0, len(g.Supplies))
		for id := range g.Supplies {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			b.d.Shop(g.SupplyShop, map[string]int{id: g.Supplies[id]})
		}
	}
	b.d.EquipBest()
	if g.Stones {
		if best, score, now := b.st.BestStones(); score > now {
			b.st.ApplyStones(best)
			changed = true
		}
	}
	for _, a := range b.st.Members() {
		if len(a.SpendSP()) > 0 {
			changed = true
		}
	}
	if changed || gold != b.st.Gold {
		b.adv(b.c.Time.Shop, b.where())
	}
	b.absorb(b.where())
}

func (b *bot) night() {
	g := b.c.Game
	if g.Inn == "" || !b.goTo(g.Inn) {
		return
	}
	talked := false
	if len(b.p.Night) > 0 && g.Talk != "" {
		for _, e := range world.Entries(b.db, b.st) {
			if entryMatches(e, g.Talk) && (e.New || b.p.NightAlways) && !hasAny(e.Label, b.p.Avoid) {
				b.adv(b.c.Time.Menu, b.where())
				b.with(b.p.Night, b.p.Rotate, "", func() {
					b.act(func() error { return b.d.Run(context.Background(), e.Commands(b.db, b.st)) })
				})
				talked = true
				break
			}
		}
	}

	if talked && g.Talk == g.Sleep {
		b.worn = false
		return
	}
	if b.st.Location != g.Inn && !b.goTo(g.Inn) {
		return
	}
	if g.Sleep != "" && b.d.Has(g.Sleep) {
		b.adv(b.c.Time.Menu, b.where())
		b.with(g.SleepWords, nil, "", func() {
			b.act(func() error { return b.d.Do(g.Sleep) })
		})
	}
	b.worn = false
}

func (b *bot) floor() int { return int(b.num(b.c.Game.Floor)) }

func (b *bot) dive() {
	g := b.c.Game
	deepest := int(b.num(g.Deepest))
	goal := deepest + b.p.Floors
	if b.p.Floors <= 0 {
		goal = 1 << 20
	}
	if g.Dungeon == "" {
		return
	}
	if !b.goTo(g.Gate) {
		return
	}
	b.with(g.DiveWords, nil, "", func() {
		for _, w := range g.Dive {
			if b.d.Has(w) {
				b.adv(b.c.Time.Menu, b.where())
				b.act(func() error { return b.d.Do(w) })
				return
			}
		}
	})
	b.worn = false
	b.policyFailed = map[string]bool{}
	b.logf("dive (deepest %d, goal %d, gold %d, shards %d, Lv%d)", deepest, goal, b.st.Gold, b.st.ItemCount(g.BackItem, false), avgLevel(b.st))
	for range 100 {
		if !b.inDungeon() || b.run.Cleared {
			return
		}
		f := b.floor()
		if f > deepest {
			b.explore()
		}
		if !b.inDungeon() || b.run.Cleared {
			return
		}
		if b.worn {
			break
		}
		if t := g.Bosses[f]; t != "" && !b.holds(g.BossDone) {
			if !b.bossFloor(f) {
				break
			}
		} else if f >= goal {
			break
		}
		if g.GuardTile != "" && b.tileOnFloor(g.GuardTile) {

			for range 4 {
				if !b.inDungeon() || b.floor() != f || b.run.Cleared || !b.tileOnFloor(g.GuardTile) {
					break
				}
				if !b.walkChar(g.GuardTile, g.StairsWords) {
					b.walkThrough(g.GuardTile, g.StairsWords)
				}
			}
			if !b.inDungeon() || b.floor() != f || b.run.Cleared {
				continue
			}
		}
		stairs := b.stairs()
		if stairs == "" {

			b.with(g.StairsWords, b.rot, b.accept, b.walkWhole)
			if !b.inDungeon() || b.floor() != f || b.run.Cleared {
				continue
			}
			stairs = b.stairs()
			if stairs == "" && b.inDungeon() && b.floor() == f && !b.worn {

				b.with(g.StairsWords, b.rot, b.accept, b.stepOff)
				if !b.inDungeon() || b.floor() != f || b.run.Cleared {
					continue
				}
				stairs = b.stairs()
			}
			if stairs == "" && b.worn {
				break
			}
		}
		if stairs != "" && !b.walkChar(stairs, g.StairsWords) && b.inDungeon() && b.floor() == f {

			b.walkThrough(stairs, g.StairsWords)
		}
		if stairs != "" && b.inDungeon() && b.floor() == f && !b.run.Cleared {
			b.stepOff()
			b.walkChar(stairs, g.StairsWords)
		}
		if b.inDungeon() && b.floor() == f && !b.run.Cleared {
			b.logf("could not go down (pos %v)\n%s", b.st.Pos, b.mapText())
			b.stuck(fmt.Sprintf("could not go down from floor %d", f))
			b.note(fmt.Errorf("could not go down from floor %d", f))
			break
		}
	}
	b.goHome()
}

func (b *bot) bossFloor(f int) bool {
	g := b.c.Game
	rec := b.boss(f)
	key := fmt.Sprintf("%d|%d", b.day, f)
	if g.GateTile != "" && !b.gateDone[key] {
		b.gateDone[key] = true
		if !b.walkChar(g.GateTile, g.GateWords) {
			b.walkThrough(g.GateTile, g.GateWords)
		}
		if !b.inDungeon() {
			return false
		}
	}
	rate := b.winRateNow(b.db, b.st, rec.Troop, g.Tries, g.MaxTurns)
	if !rec.Measured {
		rec.Measured, rec.Rate, rec.Level = true, rate, avgLevel(b.st)
		sn := b.snap("")
		rec.At = &sn
	}
	for rate < b.p.Grind && rec.Grind < 80 {
		if b.worn {
			return false
		}
		b.act(func() error { return b.d.Fights(5) })
		rec.Grind += 5
		b.checkWorn()
		b.policies("step")
		if !b.inDungeon() {
			return false
		}
		rate = b.winRateNow(b.db, b.st, rec.Troop, g.Tries, g.MaxTurns)
	}
	rec.RateEnd = rate

	if partyHP(b.st) < 0.7 || partyMP(b.st) < 0.4 {
		return false
	}
	return true
}

func (b *bot) checkWorn() {
	if !b.inDungeon() {
		return
	}
	if partyHP(b.st) < b.c.Game.WornHP || partyMP(b.st) < b.c.Game.WornMP {
		b.worn = true
	}
}

func (b *bot) goHome() {
	g := b.c.Game
	if !b.inDungeon() {
		return
	}
	if g.GateTile != "" && b.tileOnFloor(g.GateTile) {
		b.walkChar(g.GateTile, g.HomeWords)
		if !b.inDungeon() {
			return
		}
	}
	if g.BackEvent == "" {
		return
	}
	if g.BackItem != "" && b.st.ItemCount(g.BackItem, false) > 0 {
		b.st.AddItem(g.BackItem, -1)
	} else {
		b.run.Stranded++
		b.adv(b.c.Time.Stranded, b.where())
	}
	b.adv(b.c.Time.Menu, b.where())
	b.act(func() error { return b.d.Run(context.Background(), b.db.Events[g.BackEvent].Do) })
}

func (b *bot) tileOnFloor(chars string) bool {
	dg := dungeon.Of(b.db, b.st)
	if dg == nil {
		return false
	}
	for y := range dg.Grid {
		for x := range dg.Grid[y] {
			if strings.ContainsRune(chars, dungeon.CellAt(dg, b.st, x, y).Char) {
				return true
			}
		}
	}
	return false
}

func (b *bot) explore() {
	switch b.p.Explore {
	case "events":
		b.walkEvents(false)
	case "all":
		b.walkEvents(false)
		b.walkUnseen()
	}
}

func (b *bot) afterStep() {
	b.checkWorn()
	b.policies("step")
}

func (b *bot) stairs() string {
	dg := dungeon.Of(b.db, b.st)
	if dg == nil {
		return ""
	}
	w, h := dg.Size()
	for _, r := range b.c.Game.Stairs {
		for y := range h {
			for x := range w {
				if dungeon.CellAt(dg, b.st, x, y).Char == r {
					return string(r)
				}
			}
		}
	}
	return ""
}

func (b *bot) walkChar(chars string, words []string) bool {
	if chars == "" {
		return false
	}
	goal := []rune(chars)[0]
	loc, f := b.st.Location, b.floor()
	for range 3000 {
		if b.run.Cleared || b.st.Location != loc || b.floor() != f {
			return true
		}
		move, ok := dungeon.Next(b.db, b.st, goal)
		if !ok {
			b.logf("no way to %q from %v", goal, b.st.Pos)
			return false
		}
		switch move {
		case dungeon.MoveHere:
			return true
		case dungeon.MoveLeft:
			dungeon.Turn(b.st, -1)
			b.adv(b.c.Time.Turn, b.where())
		case dungeon.MoveRight:
			dungeon.Turn(b.st, 1)
			b.adv(b.c.Time.Turn, b.where())
		case dungeon.MoveLook:
			b.adv(b.c.Time.Menu, b.where())
			b.with(words, b.rot, b.accept, func() {
				b.act(func() error { return b.d.Run(context.Background(), dungeon.Look(b.db, b.st)) })
			})
			b.afterStep()
			return true
		case dungeon.MoveForward:
			there := dungeon.Front(b.db, b.st).Char == goal
			b.forward(words)
			if there {
				return true
			}
		}
	}
	return false
}

func (b *bot) forward(words []string) {
	moved, cmds := dungeon.Forward(b.db, b.st)
	if moved {
		b.adv(b.c.Time.Step, b.where())
	}
	b.with(words, b.rot, b.accept, func() {
		b.act(func() error { return b.d.Run(context.Background(), cmds) })
	})
	b.afterStep()
}

func (b *bot) walkEvents(force bool) {
	g := b.c.Game
	loc, f := b.st.Location, b.floor()
	skip := g.SkipTiles + g.GateTile + g.GuardTile + g.Stairs
	done := map[rune]int{}
	failed := map[rune]bool{}
	progressed := false
	for range 60 {
		if b.run.Cleared || (b.worn && !force) || b.st.Location != loc || b.floor() != f {
			return
		}
		dg := dungeon.Of(b.db, b.st)
		if dg == nil {
			return
		}
		count := map[rune]int{}
		for y := range dg.Grid {
			for x := range dg.Grid[y] {
				c := dungeon.CellAt(dg, b.st, x, y)
				if c.Tile == nil || len(c.Tile.Do) == 0 || strings.ContainsRune(skip, c.Char) {
					continue
				}
				if c.Tile.Once && dungeon.Spent(b.st, loc, x, y) {
					continue
				}
				count[c.Char]++
			}
		}
		keys := make([]rune, 0, len(count))
		for ch := range count {
			keys = append(keys, ch)
		}
		slices.Sort(keys)
		next := rune(0)
		for _, ch := range keys {
			limit := 1
			if strings.ContainsRune(g.RepeatTiles, ch) {
				limit = 6
			}
			if failed[ch] || done[ch] >= limit {
				continue
			}
			next = ch
			break
		}
		if next == 0 {
			if len(failed) == 0 || !progressed {
				return
			}
			failed, progressed = map[rune]bool{}, false
			continue
		}
		if !b.walkChar(string(next), nil) {
			failed[next] = true
			continue
		}
		done[next]++
		progressed = true
	}
}

func (b *bot) walkThrough(chars string, words []string) {
	goal := []rune(chars)[0]
	loc, f := b.st.Location, b.floor()
	for range 2000 {
		if b.run.Cleared || b.st.Location != loc || b.floor() != f {
			return
		}
		dg := dungeon.Of(b.db, b.st)
		if dg == nil || dungeon.CellAt(dg, b.st, b.st.Pos.X, b.st.Pos.Y).Char == goal {
			return
		}
		dir, ok := b.wayDir(func(c dungeon.Cell) bool { return c.Char == goal }, true)
		if !ok {
			b.logf("no way through to %q from %v\n%s", goal, b.st.Pos, b.mapText())
			return
		}
		b.face(dir)
		b.forward(words)
	}
}

func (b *bot) stepOff() {
	dg := dungeon.Of(b.db, b.st)
	if dg == nil {
		return
	}
	for f := range 4 {
		x, y := b.st.Pos.X+dxs[f], b.st.Pos.Y+dys[f]
		w, h := dg.Size()
		if x < 0 || y < 0 || x >= w || y >= h {
			continue
		}
		c := dungeon.CellAt(dg, b.st, x, y)
		if c.Passable() && (c.Tile == nil || len(c.Tile.Do) == 0) {
			b.face(f)
			b.forward(nil)
			return
		}
	}
}

func (b *bot) face(dir int) {
	switch (dir - b.st.Pos.Facing + 4) & 3 {
	case 1:
		dungeon.Turn(b.st, 1)
		b.adv(b.c.Time.Turn, b.where())
	case 3:
		dungeon.Turn(b.st, -1)
		b.adv(b.c.Time.Turn, b.where())
	case 2:
		dungeon.Turn(b.st, 2)
		b.adv(2*b.c.Time.Turn, b.where())
	}
}

var dxs, dys = [4]int{0, 1, 0, -1}, [4]int{-1, 0, 1, 0}

func (b *bot) walkUnseen() { b.walkUnseenThrough(false) }

func (b *bot) walkWhole() {
	b.walkUnseenThrough(false)
	b.walkUnseenThrough(true)
}

func (b *bot) walkUnseenThrough(through bool) {
	loc, f := b.st.Location, b.floor()
	for range 4000 {
		if b.run.Cleared || b.worn || b.st.Location != loc || b.floor() != f {
			return
		}
		dg := dungeon.Of(b.db, b.st)
		if dg == nil {
			return
		}
		dir, ok := b.wayDir(func(c dungeon.Cell) bool {
			r := dg.Grid[c.Y][c.X]
			return r != data.CellWall && r != data.CellNone && !dungeon.Seen(b.db, b.st, loc, c.X, c.Y)
		}, through)
		if !ok {
			return
		}
		b.face(dir)
		b.forward(nil)
	}
}

func (b *bot) wayDir(goal func(c dungeon.Cell) bool, through bool) (int, bool) {
	dg := dungeon.Of(b.db, b.st)
	if dg == nil {
		return 0, false
	}
	loc := b.st.Location
	w, h := dg.Size()
	p := b.st.Pos
	const heavy = 40
	dist := make([]int, w*h)
	for i := range dist {
		dist[i] = 1 << 30
	}
	type node struct{ x, y, first, d int }
	dist[p.Y*w+p.X] = 0
	queue := []node{{p.X, p.Y, -1, 0}}
	for len(queue) > 0 {
		bi := 0
		for i := range queue {
			if queue[i].d < queue[bi].d {
				bi = i
			}
		}
		n := queue[bi]
		queue = append(queue[:bi], queue[bi+1:]...)
		if n.d > dist[n.y*w+n.x] {
			continue
		}
		if n.first >= 0 && goal(dungeon.CellAt(dg, b.st, n.x, n.y)) {
			return n.first, true
		}
		for f := range 4 {
			x, y := n.x+dxs[f], n.y+dys[f]
			if x < 0 || y < 0 || x >= w || y >= h {
				continue
			}
			c := dungeon.CellAt(dg, b.st, x, y)
			if !c.Passable() {
				if c.Solid && goal(c) {
					first := n.first
					if first < 0 {
						first = f
					}
					if d := n.d + 1; d < dist[y*w+x] {
						dist[y*w+x] = d
						queue = append(queue, node{x, y, first, d})
					}
				}
				continue
			}
			cost := 1
			if c.Tile != nil && len(c.Tile.Do) > 0 && dungeon.Trigger(c.Tile, c.Solid) == "step" && (!c.Tile.Once || !dungeon.Spent(b.st, loc, x, y)) && !goal(c) {
				if !through {
					continue
				}
				cost = heavy
			}
			first := n.first
			if first < 0 {
				first = f
			}
			if d := n.d + cost; d < dist[y*w+x] {
				dist[y*w+x] = d
				queue = append(queue, node{x, y, first, d})
			}
		}
	}
	return 0, false
}

func (b *bot) mapText() string {
	dg := dungeon.Of(b.db, b.st)
	if dg == nil {
		return ""
	}
	var w strings.Builder
	for y := range dg.Grid {
		for x := range dg.Grid[y] {
			c := dungeon.CellAt(dg, b.st, x, y).Char
			if x == b.st.Pos.X && y == b.st.Pos.Y {
				c = '*'
			}
			w.WriteRune(c) //nolint:errcheck // In-memory formatting cannot fail.

		}
		w.WriteByte('\n') //nolint:errcheck // In-memory formatting cannot fail.

	}
	return w.String()
}

func (b *bot) duelRateNow(db *data.DB, st *rpg.State, troop string, fighters []string, tries, turns int) float64 {
	rate, err := balance.DuelRateNow(db, st, troop, fighters, tries, turns)
	b.note(err)
	return rate
}
func (b *bot) winRateNow(db *data.DB, st *rpg.State, troop string, tries, turns int) float64 {
	rate, err := balance.WinRateNow(db, st, troop, tries, turns)
	b.note(err)
	return rate
}
