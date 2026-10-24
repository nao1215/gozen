package balance

import (
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/headless"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// Odds is the tally of repeated fights against one enemy group from the same start. Both gozen sim battle and the dev
// balance display use this type with Try and Add so they print the same values.
type Odds struct {
	Troop    string
	Battles  int
	Won      int
	Lost     int
	Fled     int
	Draw     int
	Turns    int // total turns over all battles
	Standing int // total party members left standing in won battles
	// HP is the remaining HP fraction (0..1) of the party row in each won battle, in the order fought.
	HP []float64
}

// WinRate is the fraction of battles won (0..1).
func (o Odds) WinRate() float64 { return ratio(o.Won, o.Battles) }

// AvgTurns is the average number of turns per battle.
func (o Odds) AvgTurns() float64 {
	if o.Battles == 0 {
		return 0
	}
	return float64(o.Turns) / float64(o.Battles)
}

// AvgStanding is the average number of members left standing in won battles.
func (o Odds) AvgStanding() float64 {
	return float64(o.Standing) / float64(max(1, o.Won))
}

// HPBand is the 10%, 50% and 90% points of remaining HP over won battles. ok is false if there are no wins.
func (o Odds) HPBand() (low, mid, high float64, ok bool) {
	if len(o.HP) == 0 {
		return 0, 0, 0, false
	}
	s := append([]float64(nil), o.HP...)
	sort.Float64s(s)
	at := func(p float64) float64 { return s[int(p*float64(len(s)-1)+0.5)] }
	return at(0.1), at(0.5), at(0.9), true
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// Try fights troop once with AUTO battle using st's party row (one battle of gozen sim battle). st is changed.
// If seen is non-nil, it is passed each Step in order. Exceeding maxTurns is a draw.
func Try(db *data.DB, st *rpg.State, troop string, maxTurns int, seen func(*battle.Battle, []battle.Step)) *battle.Battle {
	b := battle.New(db, st, troop)
	var f func([]battle.Step)
	if seen != nil {
		f = func(steps []battle.Step) { seen(b, steps) }
	}
	headless.AutoBattle(db, st, b, nil, maxTurns, f)
	return b
}

// Add adds a finished battle b to the tally.
func (o *Odds) Add(b *battle.Battle) {
	o.Battles++
	o.Turns += b.Turn
	switch b.Result {
	case battle.Won:
		o.Won++
		hp, mhp := 0, 0
		for _, u := range b.Party {
			if u.Alive() {
				o.Standing++
			}
			hp += max(0, u.HP)
			mhp += u.Stat("mhp")
		}
		o.HP = append(o.HP, ratio(hp, mhp))
	case battle.Lost:
		o.Lost++
	case battle.Fled:
		o.Fled++
	case battle.Ongoing:
		o.Draw++
	}
}

// Start builds the starting state for battle i (from 0). It returns a fresh state each time.
type Start func(i int) (*rpg.State, error)

// Prep adjusts the starting state (gozen sim battle's --level, --no-tree, --gear, --as-is). The zero value does nothing.
type Prep struct {
	AsIs  bool     // with SavedStart, start with the saved HP, MP and status instead of fully healing (--level's level-up still heals)
	Level int      // if above 0, set each member's experience to this level and fully heal
	Tree  bool     // spend skill points
	Gear  []string // put items into the bag for the number of row members and equip them where they fit best
}

func (p Prep) apply(db *data.DB, st *rpg.State) error {
	if p.Level > 0 {
		for _, a := range st.Members() {
			a.GainExp(a.ExpFor(p.Level) - a.Exp)
			a.Recover()
		}
	}
	if p.Tree {
		for _, a := range st.Members() {
			a.SpendSP()
		}
	}
	for _, id := range p.Gear {
		if db.Items[id] == nil {
			return fmt.Errorf("unknown item %q", id)
		}
		st.AddItem(id, len(st.Members()))
	}
	if len(p.Gear) > 0 {
		for _, a := range st.Members() {
			st.EquipBest(a)
			a.Recover()
		}
	}
	return nil
}

// FreshStart is the party row of a newly started game (the starting row if party is empty). Battle i's randomness is PCG(seed, i).
func FreshStart(db *data.DB, seed uint64, party []string, p Prep) Start {
	return func(i int) (*rpg.State, error) {
		st := rpg.New(db, rand.New(rand.NewPCG(seed, uint64(i)))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		if len(party) > 0 {
			st.Party = nil
			for _, id := range party {
				if db.Actors[id] == nil {
					return nil, fmt.Errorf("unknown actor %q", id)
				}
				st.Join(id)
			}
		}
		if err := p.apply(db, st); err != nil {
			return nil, err
		}
		return st, nil
	}
}

// SavedStart is the party row of the save data save (the shape of rpg.State.Save, the .save of an F9 memo), fully healed
// (or with the saved HP, MP and status if p.AsIs). Battle i's randomness is PCG(seed, i). The adjustment p is applied after healing.
func SavedStart(db *data.DB, save []byte, seed uint64, p Prep) Start {
	return func(i int) (*rpg.State, error) {
		st, err := rpg.Parse(db, save, rand.New(rand.NewPCG(seed, uint64(i)))) //nolint:gosec // Reproducible gameplay simulation, not security randomness.
		if err != nil {
			return nil, err
		}
		if !p.AsIs {
			recoverAll(st)
		}
		if err := p.apply(db, st); err != nil {
			return nil, err
		}
		return st, nil
	}
}

// StreakOne is the number of consecutive wins (up to limit) of st's party row fighting troop without healing
// until it loses, draws or flees. Between battles, battle.End restores HP, MP, status and experience to the row. st is changed.
func StreakOne(db *data.DB, st *rpg.State, troop string, maxTurns, limit int) int {
	wins := 0
	for wins < limit {
		b := Try(db, st, troop, maxTurns, nil)
		b.End()
		if b.Result != battle.Won {
			break
		}
		wins++
	}
	return wins
}

// Streak is StreakOne for each i-th start of start (gozen sim battle --streak).
func Streak(db *data.DB, troop string, n, maxTurns, limit int, start Start) ([]int, error) {
	out := make([]int, 0, n)
	for i := range n {
		st, err := start(i)
		if err != nil {
			return out, err
		}
		out = append(out, StreakOne(db, st, troop, maxTurns, limit))
	}
	return out, nil
}

// Percentile is the p point (0..1, nearest rank) of the sorted values. 0 if empty.
func Percentile(v []int, p float64) int {
	if len(v) == 0 {
		return 0
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[int(p*float64(len(s)-1)+0.5)]
}

// Sim is the tally of n fights against troop from start's starting states (gozen sim battle). seen receives the battle number and Step.
func Sim(db *data.DB, troop string, n, maxTurns int, start Start, seen func(i int, b *battle.Battle, steps []battle.Step)) (Odds, error) {
	o := Odds{Troop: troop}
	for i := range n {
		st, err := start(i)
		if err != nil {
			return o, err
		}
		var f func(*battle.Battle, []battle.Step)
		if seen != nil {
			f = func(b *battle.Battle, steps []battle.Step) { seen(i, b, steps) }
		}
		o.Add(Try(db, st, troop, maxTurns, f))
	}
	return o, nil
}

// Here is the enemy group met at the current place: place encounters (those whose when holds, with weight shares),
// then the place's action battles, then extra (groups mid-battle, etc.), in that order with overlaps removed.
func Here(db *data.DB, st *rpg.State, extra ...string) []Encounter {
	var out []Encounter
	seen := map[string]bool{}
	add := func(e Encounter) {
		if e.Troop == "" || seen[e.Troop] || db.Troops[e.Troop] == nil {
			return
		}
		seen[e.Troop] = true
		out = append(out, e)
	}
	loc := db.Locations[st.Location]
	if loc != nil {
		env := st.Env()
		total := 0.0
		var picks []Encounter
		for _, t := range loc.Encounters.Troops {
			if ok, err := t.When.Bool(env); err != nil || !ok {
				continue
			}
			w := t.Weight
			if w <= 0 {
				w = 1
			}
			total += w
			picks = append(picks, Encounter{Troop: t.Troop, Share: w})
		}
		for _, p := range picks {
			p.Share /= total
			add(p)
		}
		for _, a := range loc.Actions {
			if ok, err := a.When.Bool(env); err == nil && ok {
				for _, id := range battlesIn(a.Do) {
					add(Encounter{Troop: id})
				}
			}
		}
	}
	for _, id := range extra {
		add(Encounter{Troop: id})
	}
	return out
}

// Encounter is a group met at a place. Share is the encounter weight fraction (0 for an action battle).
type Encounter struct {
	Troop string
	Share float64
}

func battlesIn(cmds []data.Command) []string {
	var out []string
	for _, c := range cmds {
		if c.Battle != "" {
			out = append(out, c.Battle)
		}
	}
	return out
}
