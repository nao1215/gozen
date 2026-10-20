// Package balance measures win rates and the difficulty curve of a game by
// running its battles without a screen, for gozen sim and gozen curve.
package balance

import (
	"errors"
	"math/rand/v2"
	"sort"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/layer4/itemuse"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/world"
	data "github.com/nao1215/gozen/schema"
)

type Config struct {
	Location string
	Floors   int
	Battles  int
	Bosses   map[int]string

	Guards map[int]string

	Retreat bool

	GuardsOnly bool

	NoTown   bool
	Shops    []string
	Tries    int
	Grind    float64
	Seed     uint64
	MaxTurns int

	RestEvery int

	Supplies map[string]int

	WornHP, WornMP float64

	Events bool

	RestCost string

	RestEvent string

	BackItem string

	SceneEvent string
	SceneWhen  string

	FloorEvent string

	Arrive func(floor int, st *rpg.State)
}

type Floor struct {
	Floor        int      `json:"floor"`
	Level        int      `json:"level"`
	Battles      int      `json:"battles"`
	Lost         int      `json:"lost"`
	Turns        float64  `json:"turns"`
	HPLeft       float64  `json:"hp_left"`
	Gold         int      `json:"gold"`
	MPLeft       float64  `json:"mp_left"`
	Wiped        int      `json:"wiped"`
	Rested       bool     `json:"rested"`
	WentBack     bool     `json:"went_back"`
	Backs        int      `json:"backs"`
	Boss         string   `json:"boss"`
	BossRate     float64  `json:"boss_rate"`
	BossRateEnd  float64  `json:"boss_rate_end"`
	Grinding     int      `json:"grinding"`
	Troops       []string `json:"troops"`
	Guard        string   `json:"guard"`
	GuardRate    float64  `json:"guard_rate"`
	GuardRateEnd float64  `json:"guard_rate_end"`
	GuardLost    int      `json:"guard_lost"`
	GuardBack    bool     `json:"guard_back"`
	Spent        int      `json:"spent"`
	Unpaid       int      `json:"unpaid"`
	Stranded     int      `json:"stranded"`
	GoldLost     int      `json:"gold_lost"`
	Scenes       int      `json:"scenes"`
	Fallen       int      `json:"fallen"`
}

type Result struct {
	Floors []Floor `json:"floors"`

	Used map[string]map[string]int `json:"used"`

	Known map[string][]string `json:"known"`
	Names map[string]string   `json:"names"`
	Level map[string]int      `json:"level"`
	Order []string            `json:"order"`
}

type runner struct {
	measuring bool // measuring: measuring a single win rate does not consume exploration resources.
	err       error
	db        *data.DB
	o         Config
	used      map[string]map[string]int
	cost      *expr.Expr
	when      *expr.Expr
	fl        *Floor
}

func Run(db *data.DB, o Config) (Result, error) {
	var err error
	if o.Floors == 0 {
		o.Floors = 20
	}
	if o.Tries == 0 {
		o.Tries = 30
	}
	if o.MaxTurns == 0 {
		o.MaxTurns = 60
	}
	r := &runner{db: db, o: o, used: map[string]map[string]int{}}
	if o.RestCost != "" {
		r.cost, err = expr.Parse(o.RestCost)
		if err != nil {
			return Result{}, err
		}
	}
	if o.SceneEvent != "" && o.SceneWhen != "" {
		r.when, err = expr.Parse(o.SceneWhen)
		if err != nil {
			return Result{}, err
		}
	}
	st := rpg.New(db, rand.New(rand.NewPCG(o.Seed, 7))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
	st.Location = o.Location
	var res Result
	for f := 1; f <= o.Floors; f++ {
		st.Vars[dungeon.VarFloor] = float64(f)
		fl := Floor{Floor: f}
		r.fl = &fl
		if ev := db.Events[o.FloorEvent]; ev != nil {
			r.run(st, ev.Do)
		}
		seen := map[string]bool{}
		var turns int
		hp := 0.0
		for range o.Battles {
			troop := world.PickTroop(db, st)
			if troop == "" {
				continue
			}
			seen[troop] = true
			b := r.fight(st, troop)
			turns += b.Turn
			fl.Battles++
			if b.Result == battle.Lost {
				fl.Lost++
			}
			for _, u := range b.Party {
				if u.HP <= 0 {
					fl.Fallen++
					break
				}
			}
			hp += r.end(st, b)
			if b.Result == battle.Lost {
				r.lost(st)
			} else if o.RestEvery > 0 && (partyHP(st) < o.WornHP || partyMP(st) < o.WornMP) {

				fl.WentBack = true
				fl.Backs++
				r.goBack(st)
				r.rest(st)
				r.supply(st)
			}
		}
		if t := o.Guards[f]; t != "" && o.Bosses[f] == "" {
			r.guard(st, &fl, t, f)
		}
		fl.MPLeft = partyMP(st)
		fl.Wiped = fl.Lost
		worn := o.RestEvery > 0 && (partyHP(st) < o.WornHP || fl.MPLeft < o.WornMP)
		if o.RestEvery <= 0 || f%o.RestEvery == 0 || o.Bosses[f] != "" || worn {
			fl.Rested = true
			if worn && f%o.RestEvery != 0 && o.Bosses[f] == "" {
				fl.WentBack = true
				fl.Backs++
			}
			if o.Bosses[f] == "" {
				r.goBack(st)
			}
			r.rest(st)
			r.supply(st)
		} else {
			standUp(st)
			for _, a := range st.Members() {
				if !o.NoTown {
					a.SpendSP()
				}
			}
		}
		r.shop(st)
		if t := o.Bosses[f]; t != "" {
			fl.Boss = t
			if o.Arrive != nil {
				o.Arrive(f, st)
			}
			rate := r.rate(st, t, f)
			fl.BossRate = rate
			for o.Grind > 0 && rate < o.Grind && fl.Grinding < 200 {
				for range 10 {
					if tr := world.PickTroop(db, st); tr != "" {
						b := r.fight(st, tr)
						r.end(st, b)
						if b.Result == battle.Lost {
							r.lost(st)
						}
						fl.Grinding++
					}
					r.rest(st)
				}
				r.shop(st)
				rate = r.rate(st, t, f)
			}
			fl.BossRateEnd = rate
		}
		fl.Level = avgLevel(st)
		if t := o.Bosses[f]; t != "" {
			b := r.fight(st, t)
			for i := 0; b.Result != battle.Won && i < 5; i++ {
				r.end(st, b)
				r.lost(st)
				b = r.fight(st, t)
			}
			r.end(st, b)
			r.rest(st)
		}
		fl.Turns = float64(turns) / float64(max(1, fl.Battles))
		fl.HPLeft = hp / float64(max(1, fl.Battles))
		fl.Gold = st.Gold
		for t := range seen {
			fl.Troops = append(fl.Troops, t)
		}
		sort.Strings(fl.Troops)
		res.Floors = append(res.Floors, fl)
	}
	res.Used = r.used
	res.Known, res.Names, res.Level = map[string][]string{}, map[string]string{}, map[string]int{}
	for _, a := range st.Members() {
		res.Order = append(res.Order, a.ID)
		res.Known[a.ID] = a.Skills()
		res.Names[a.ID] = a.Name()
		res.Level[a.ID] = a.Level
	}
	return res, r.err
}

func (r *runner) guard(st *rpg.State, fl *Floor, troop string, floor int) {
	fl.Guard = troop
	if r.o.NoTown {
		recoverAll(st)
	}
	fl.GuardRate = r.rate(st, troop, floor)
	fl.GuardRateEnd = fl.GuardRate
	if r.o.Retreat && r.o.Grind > 0 && fl.GuardRate < r.o.Grind {
		fl.GuardBack = true
		r.goBack(st)
		r.rest(st)
		r.supply(st)
		r.shop(st)
		rate := r.rate(st, troop, floor)
		for n := 0; rate < r.o.Grind && n < 20; n++ {
			for range 10 {
				if tr := world.PickTroop(r.db, st); tr != "" {
					b := r.fight(st, tr)
					r.end(st, b)
					if b.Result == battle.Lost {
						r.lost(st)
					}
					fl.Grinding++
				}
				r.rest(st)
			}
			r.shop(st)
			rate = r.rate(st, troop, floor)
		}
		fl.GuardRateEnd = rate
	}
	if r.o.GuardsOnly {
		return
	}
	b := r.fight(st, troop)
	for i := 0; b.Result != battle.Won && i < 5; i++ {
		fl.GuardLost++
		r.end(st, b)
		r.lost(st)
		b = r.fight(st, troop)
	}
	r.end(st, b)
}

func (r Result) Unused() map[string][]string {
	out := map[string][]string{}
	for id, known := range r.Known {
		for _, s := range known {
			if r.Used[id][s] == 0 {
				out[id] = append(out[id], s)
			}
		}
	}
	return out
}

func (r *runner) rate(st *rpg.State, troop string, floor int) float64 {
	r.measuring = true
	defer func() { r.measuring = false }()
	wins := 0
	for i := range r.o.Tries {
		c, err := clone(r.db, st, uint64(i))
		if err != nil {
			r.err = errors.Join(r.err, err)
			return 0
		}
		c.Vars[dungeon.VarFloor] = float64(floor)
		if r.fight(c, troop).Result == battle.Won {
			wins++
		}
	}
	return float64(wins) / float64(r.o.Tries)
}

func (r *runner) fight(st *rpg.State, troop string) *battle.Battle {
	if r.o.Events && !r.measuring {
		if ev := r.db.Events[r.db.Game.Battle.Before]; ev != nil {
			r.run(st, ev.Do)
			if st.Location != r.o.Location {
				r.returnAndResume(st)
				r.run(st, ev.Do)
			}
		}
	}
	b := battle.New(r.db, st, troop)
	headless.AutoBattle(r.db, st, b, nil, r.o.MaxTurns, func(steps []battle.Step) {
		for _, s := range steps {
			if s.Kind == battle.Act && s.User != nil && !s.User.Enemy && s.User.Actor != nil {
				m := r.used[s.User.Actor.ID]
				if m == nil {
					m = map[string]int{}
					r.used[s.User.Actor.ID] = m
				}
				m[s.Skill]++
			}
		}
	})
	return b
}

func rest(st *rpg.State, noTown bool) {
	for _, a := range st.Members() {
		a.Recover()
		if !noTown {
			a.SpendSP()
		}
	}
}

func (r *runner) rest(st *rpg.State) {
	if r.cost != nil {
		if v, err := r.cost.Eval(st.Env()); err == nil {
			if n, ok := v.(float64); ok && n > 0 {
				if st.Gold >= int(n) {
					st.Gold -= int(n)
					r.fl.Spent += int(n)
				} else {
					r.fl.Unpaid++
				}
			}
		}
	}
	rest(st, r.o.NoTown)
	if ev := r.db.Events[r.o.RestEvent]; ev != nil {
		r.run(st, ev.Do)
	}
	st.Location = r.o.Location
}

func (r *runner) goBack(st *rpg.State) {
	if r.o.BackItem == "" {
		return
	}
	if st.Items[r.o.BackItem] > 0 {
		st.AddItem(r.o.BackItem, -1)
	} else {
		r.fl.Stranded++
	}
}

func (r *runner) end(st *rpg.State, b *battle.Battle) float64 {
	b.End()
	if r.o.Events && b.Result == battle.Won {
		if ev := r.db.Events[r.db.Game.Battle.AfterWin]; ev != nil {
			r.run(st, ev.Do)
		}
	}
	if b.Result == battle.Won && r.when != nil {
		if v, err := r.when.Eval(st.Env()); err == nil && v == true {
			if ev := r.db.Events[r.o.SceneEvent]; ev != nil {
				r.run(st, ev.Do)
				if r.fl != nil {
					r.fl.Scenes++
				}
			}
		}
	}
	// do not overwrite the post-battle attrition rate with the recovery after returning.
	hp := partyHP(st)
	if r.o.Events && b.Result != battle.Lost {
		if ev := r.db.Events[r.db.Game.Battle.After]; ev != nil {
			r.run(st, ev.Do)
			if st.Location != r.o.Location {
				r.returnAndResume(st)
			}
		}
	}
	return hp
}

func (r *runner) returnAndResume(st *rpg.State) {
	if r.fl != nil {
		r.fl.Backs++
		r.fl.WentBack = true
	}
	r.rest(st)
	r.supply(st)
}

func (r *runner) lost(st *rpg.State) {
	recoverAll(st)
	if !r.o.Events {
		return
	}
	if ev := r.db.Events[r.db.Game.GameOver.Event]; ev != nil {
		g := st.Gold
		r.run(st, ev.Do)
		r.fl.GoldLost += max(0, g-st.Gold)
	}
	r.rest(st)
}

func (r *runner) run(st *rpg.State, cmds []data.Command) {
	ev := event.New(r.db, st, event.HostFunc(func(effect event.Effect) {
		if move, ok := effect.(event.MoveEffect); ok {
			st.Location = move.Location
		}
	}))
	ev.Log = func(err error) { r.err = errors.Join(r.err, err) }
	ev.UseItem = itemuse.For(r.db, st)
	ev.Start(cmds)
	for range 10000 {
		if !ev.Running() {
			return
		}
		switch ev.Next().(type) {
		case event.Choice:
			ev.Answer(0)
		case event.Battle:
			ev.Answer(event.BattleWon)
		}
	}
	r.err = errors.Join(r.err, errors.New("balance: event step limit exceeded"))
}

func recoverAll(st *rpg.State) {
	for _, a := range st.Members() {
		a.Recover()
	}
}

func clone(db *data.DB, st *rpg.State, seed uint64) (*rpg.State, error) {
	b, err := st.Save()
	if err != nil {
		return nil, err
	}
	c, err := rpg.Parse(db, b, rand.New(rand.NewPCG(seed, 99))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
	if err != nil {
		return nil, err
	}
	c.Location = st.Location
	return c, nil
}

func partyHP(st *rpg.State) float64 {
	hp, mhp := 0, 0
	for _, a := range st.Members() {
		hp += max(0, a.HP)
		mhp += a.Stat("mhp")
	}
	return float64(hp) / float64(max(1, mhp))
}

func avgLevel(st *rpg.State) int {
	sum, n := 0, 0
	for _, a := range st.Members() {
		sum += a.Level
		n++
	}
	return sum / max(1, n)
}

func (r *runner) shop(st *rpg.State) {
	if r.o.NoTown {
		return
	}
	var goods []string
	for _, id := range r.o.Shops {
		if s := r.db.Shops[id]; s != nil {
			goods = append(goods, s.Goods...)
		}
	}
	sort.SliceStable(goods, func(i, j int) bool { return r.db.Items[goods[i]].Price > r.db.Items[goods[j]].Price })
	sum := func(id string) int {
		n := 0
		if it := r.db.Items[id]; it != nil {
			for _, v := range it.Stats {
				n += v
			}
		}
		return n
	}
	for _, a := range st.Members() {
		for _, id := range goods {
			it := r.db.Items[id]
			if it.Slot == "" || it.Kind == "stone" || !a.CanEquip(id) || it.Price > st.Gold {
				continue
			}
			if sum(id) <= sum(a.Equip[it.Slot]) {
				continue
			}
			st.Gold -= it.Price
			st.AddItem(id, 1)
			st.EquipBest(a)
		}
	}
}

func partyMP(st *rpg.State) float64 {
	mp, mmp := 0, 0
	for _, a := range st.Members() {
		mp += max(0, a.MP)
		mmp += a.Stat("mmp")
	}
	return float64(mp) / float64(max(1, mmp))
}

func standUp(st *rpg.State) {
	for _, a := range st.Members() {
		if a.HP <= 0 {
			a.HP = 1
		}
	}
}

func (r *runner) supply(st *rpg.State) {
	ids := make([]string, 0, len(r.o.Supplies))
	for id := range r.o.Supplies {
		if r.db.Items[id] != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for bought := true; bought; {
		bought = false
		for _, id := range ids {
			it := r.db.Items[id]
			if st.ItemCount(id, false) >= r.o.Supplies[id] || st.Gold < it.Price {
				continue
			}
			st.Gold -= it.Price
			if r.fl != nil {
				r.fl.Spent += it.Price
			}
			st.AddItem(id, 1)
			bought = true
		}
	}
}

func WinRateNow(db *data.DB, st *rpg.State, troop string, tries, maxTurns int) (float64, error) {
	if maxTurns <= 0 {
		maxTurns = 60
	}
	r := &runner{db: db, o: Config{MaxTurns: maxTurns}, used: map[string]map[string]int{}}
	wins := 0
	for i := range tries {
		c, err := clone(db, st, uint64(i))
		if err != nil {
			return 0, err
		}
		recoverAll(c)
		if r.fight(c, troop).Result == battle.Won {
			wins++
		}
	}
	return float64(wins) / float64(max(1, tries)), nil
}

func DuelRateNow(db *data.DB, st *rpg.State, troop string, fighters []string, tries, maxTurns int) (float64, error) {
	if maxTurns <= 0 {
		maxTurns = 60
	}
	r := &runner{db: db, o: Config{MaxTurns: maxTurns}, used: map[string]map[string]int{}}
	wins := 0
	for i := range tries {
		c, err := clone(db, st, uint64(i))
		if err != nil {
			return 0, err
		}
		c.Fighters = fighters
		for _, id := range fighters {
			if a := c.Actors[id]; a != nil {
				a.Recover()
			}
		}
		if r.fight(c, troop).Result == battle.Won {
			wins++
		}
	}
	return float64(wins) / float64(max(1, tries)), nil
}
