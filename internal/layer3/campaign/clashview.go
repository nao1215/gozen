package campaign

import (
	"math"
	"slices"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// A march decided by a clash shown as units (fight: clash_view, ADR 0030): the generals
// marching on the left, the region's defend troop on the right (its garrison shared among
// its members), at most four units a side, the first two of each in front. For a few
// rounds every unit still standing acts, the one with the fewest troops first, and
// strikes a unit in the other side's front; a unit with no troops left is routed, and the
// next behind steps up. The march takes the region when the other side is routed; when the
// rounds run out, the region holds (a draw). The numbers are the rules': the screen only
// plays them back.

// MaxUnits is the most units a side has; the first Front of them stand in front.
const (
	MaxUnits = 4
	Front    = 2
)

// The defaults of a clash shown as units.
var (
	defaultHit    = data.MustExpr("a.troops * 0.15 * (0.8 + rand() * 0.4)")
	defaultRounds = 5
)

// Unit is one unit of a clash: who leads it, its picture, its troops at the start and now,
// and its leader's HP.
type Unit struct {
	Name    string
	Actor   string // the general's actor ("" for a foe)
	Enemy   string // the foe's enemy id ("" for a general)
	Picture string
	Troops  float64
	Max     float64
	HP, MHP int
	might   float64
	stats   map[string]float64
	Routed  bool
	Caught  bool // routed and taken (a foe)
}

// Hit is one strike of a clash: the side and unit striking (0 the player's), the side and
// unit struck, how many soldiers went, the troops lost, and whether it routed the unit.
type Hit struct {
	Round     int
	Side, Of  int
	At, To    int
	Attackers int
	Loss      int
	Routed    bool
	Caught    bool
}

// Clash is a clash shown as units: the units of both sides at the start, the strikes in
// order, the rounds fought of the most, the terrain's background, and how it ended (won,
// lost or draw) with the region.
type Clash struct {
	Region string
	// Owner is the faction that held the region when the clash began ("" for none).
	Owner      string
	Background string
	Sides      [2][]Unit
	Hits       []Hit
	Rounds     int
	MaxRounds  int
	Result     string
}

// FightView begins a march decided by a clash shown as units (an order with fight:
// clash_view): it spends the order, plays the clash by the rules, writes the troops back
// (Give's way: the region taken with the garrison share, the troops left shared among the
// generals, a general with none left away), and returns the clash for the screen. Nil
// when the order is not one.
func FightView(db *data.DB, st *rpg.State, id, general string, with []string, target string) *Clash {
	c := db.Game.Campaign
	o := c.Orders[id]
	r := db.Regions[target]
	if o == nil || o.Fight != "clash_view" || r == nil {
		return nil
	}
	Spend(db, st, id, general, with, target)
	cl := &Clash{Region: target, Owner: data.RegionOwner(db, st.Campaign(), target), MaxRounds: defaultRounds, Background: c.Clash.Backgrounds[r.Terrain]}
	if c.Clash.Rounds > 0 {
		cl.MaxRounds = c.Clash.Rounds
	}
	generals := append([]string{general}, with...)
	for _, g := range generals {
		if len(cl.Sides[0]) == MaxUnits {
			break
		}
		cl.Sides[0] = append(cl.Sides[0], generalUnit(db, st, g))
	}
	cl.Sides[1] = foeUnits(db, st, target)
	play(db, st, cl)
	// write back
	left := 0.0
	for _, u := range cl.Sides[0] {
		left += u.Troops
	}
	held := 0.0
	for _, u := range cl.Sides[1] {
		held += u.Troops
	}
	switch cl.Result {
	case "won":
		garrison := math.Floor(left * c.Clash.GarrisonShare())
		shareUnits(db, st, cl.Sides[0], garrison)
		take(st, target, c.Player, garrison)
		st.Vars[data.VarTaken] = float64(data.Taken(st.Campaign()) + 1)
	default:
		shareUnits(db, st, cl.Sides[0], 0)
		st.Vars[data.RegionVar(target, "troops")] = math.Floor(held)
	}
	st.Vars[data.VarResult] = map[string]string{"won": "won", "lost": "held", "draw": "held"}[cl.Result]
	st.Vars[data.VarClash] = cl.Result
	return cl
}

// shareUnits writes each general's troops left (less its share of the garrison left in a
// region taken), marks it as marched, and sends one with none away.
func shareUnits(db *data.DB, st *rpg.State, units []Unit, garrison float64) {
	total := 0.0
	for _, u := range units {
		total += u.Troops
	}
	for _, u := range units {
		n := u.Troops
		if total > 0 {
			n -= math.Floor(garrison * u.Troops / total)
		}
		n = max(0, math.Floor(n))
		st.Vars[data.GeneralVar(u.Actor, "troops")] = n
		st.Vars[data.GeneralVar(u.Actor, "acted")] = 1.0
		if n <= 0 && db.Game.Campaign.Defeat.Out > 0 {
			st.Vars[data.GeneralVar(u.Actor, "out")] = float64(db.Game.Campaign.Defeat.Out)
		}
	}
}

func generalUnit(db *data.DB, st *rpg.State, id string) Unit {
	c := db.Game.Campaign
	u := Unit{Name: GeneralLabelName(db, id), Actor: id, might: c.Generals[id].Might, stats: map[string]float64{}}
	u.Troops = data.GeneralTroops(db, st.Campaign(), id)
	u.Max = u.Troops
	if a := db.Actors[id]; a != nil {
		u.Picture = db.FacePicture(a, a.Face, "")
	}
	if act := st.Actors[id]; act != nil {
		u.HP, u.MHP = act.HP, act.Stat("mhp")
		u.stats["level"] = float64(act.Level)
		for _, n := range data.StatNames {
			u.stats[n] = float64(act.Stat(n))
		}
	}
	return u
}

// foeUnits are the region's defenders: the members of its defend troop (at most four),
// sharing the garrison; one unit named for the region when it has no troop.
func foeUnits(db *data.DB, st *rpg.State, target string) []Unit {
	r := db.Regions[target]
	garrison := data.Garrison(db, st.Campaign(), target)
	var ids []string
	if t := db.Troops[r.Defend]; t != nil {
		ids = t.Members
	}
	if len(ids) > MaxUnits {
		ids = ids[:MaxUnits]
	}
	if len(ids) == 0 {
		return []Unit{{Name: data.RegionName(db, target), Troops: garrison, Max: garrison, stats: map[string]float64{}}}
	}
	var out []Unit
	given := 0.0
	for i, id := range ids {
		e := db.Enemies[id]
		n := math.Floor(garrison / float64(len(ids)))
		if i == len(ids)-1 {
			n = garrison - given
		}
		given += n
		u := Unit{Name: id, Enemy: id, Troops: n, Max: n, stats: map[string]float64{}}
		if e != nil {
			u.Name, u.Picture = e.Name, e.Image.File
			u.MHP = e.Stats["mhp"]
			u.HP = u.MHP
			for _, s := range data.StatNames {
				u.stats[s] = float64(e.Stats[s])
			}
			u.stats["level"] = float64(e.Level)
		}
		out = append(out, u)
	}
	return out
}

// play fights the rounds.
func play(db *data.DB, st *rpg.State, cl *Clash) {
	c := db.Game.Campaign
	hit := c.Clash.Hit
	if !hit.Set() {
		hit = defaultHit
	}
	capture := 0.2
	if c.Clash.Capture != nil {
		capture = *c.Clash.Capture
	}
	alive := func(side int) []int {
		var out []int
		for i, u := range cl.Sides[side] {
			if !u.Routed {
				out = append(out, i)
			}
		}
		return out
	}
	for round := 1; round <= cl.MaxRounds; round++ {
		cl.Rounds = round
		// who acts: every unit standing, the fewest troops first (the player's first on a tie)
		type turn struct{ side, at int }
		var order []turn
		for side := range 2 {
			for _, i := range alive(side) {
				order = append(order, turn{side, i})
			}
		}
		slices.SortStableFunc(order, func(x, y turn) int {
			a, b := cl.Sides[x.side][x.at].Troops, cl.Sides[y.side][y.at].Troops
			switch {
			case a < b:
				return -1
			case a > b:
				return 1
			}
			return x.side - y.side
		})
		for _, t := range order {
			u := &cl.Sides[t.side][t.at]
			if u.Routed {
				continue
			}
			foes := alive(1 - t.side)
			if len(foes) == 0 {
				break
			}
			front := foes[:min(Front, len(foes))]
			to := front[st.Rand.IntN(len(front))]
			v := &cl.Sides[1-t.side][to]
			env := expr.Chain{expr.Map{"a": unitMap(u, t.side), "b": unitMap(v, 1-t.side), "round": float64(round)}, st.Env()}
			n, err := hit.Number(env)
			if err != nil || math.IsNaN(n) || n < 0 {
				n = 0
			}
			loss := math.Min(v.Troops, math.Max(1, math.Floor(n)))
			v.Troops -= loss
			h := Hit{Round: round, Side: t.side, At: t.at, Of: 1 - t.side, To: to, Attackers: int(math.Ceil(u.Troops * 0.3)), Loss: int(loss)}
			if v.Troops <= 0 {
				v.Troops, v.Routed, h.Routed = 0, true, true
				if v.Enemy != "" && st.Rand.Float64() < capture {
					v.Caught, h.Caught = true, true
				}
			}
			cl.Hits = append(cl.Hits, h)
		}
		switch {
		case len(alive(1)) == 0:
			cl.Result = "won"
			return
		case len(alive(0)) == 0:
			cl.Result = "lost"
			return
		}
	}
	cl.Result = "draw"
}

func unitMap(u *Unit, side int) expr.Map {
	m := expr.Map{"troops": u.Troops, "max": u.Max, "might": u.might, "hp": float64(u.HP), "mhp": float64(u.MHP), "player": side == 0}
	for k, v := range u.stats {
		m[k] = v
	}
	for _, n := range data.StatNames {
		if _, ok := m[n]; !ok {
			m[n] = 0.0
		}
	}
	if _, ok := m["level"]; !ok {
		m["level"] = 0.0
	}
	return m
}
