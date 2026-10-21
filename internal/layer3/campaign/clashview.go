package campaign

import (
	"maps"
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
// strikes the unit of the other side its tactic picks (clash.tactics; one in front by
// default), or uses its skill when charged (clash.skills); a unit with no troops left is routed, and the
// next behind steps up. The march takes the region when the other side is routed; when the
// rounds run out, the region holds (a draw). The numbers are the rules': the screen only
// plays them back.

// MaxUnits is the most units a side has (clash.units at most); the first Front of them
// stand in front.
const (
	MaxUnits = 4
	Front    = 2
)

// Units is the most units a side has in the campaign's clashes (clash.units, 4 when 0).
func Units(db *data.DB) int {
	if n := db.Game.Campaign.Clash.Units; n > 0 {
		return min(n, MaxUnits)
	}
	return MaxUnits
}

// Fronts is how many units of a side stand in front (1 in a duel, else 2).
func Fronts(db *data.DB) int {
	return min(Front, Units(db))
}

// Cap is the most generals an order marches with (the order's generals, clash.units).
func Cap(db *data.DB, o *data.Order) int {
	n := max(1, o.Generals)
	if o.Fight == "clash_view" {
		n = min(n, Units(db))
	}
	return n
}

// TacticOf is the tactic a general marches with: the one given, the general's own, or
// the first of the campaign's.
func TacticOf(db *data.DB, general string, given map[string]string) string {
	c := db.Game.Campaign
	if t := given[general]; t != "" {
		return t
	}
	if t := c.Generals[general].Tactic; t != "" {
		return t
	}
	return c.Clash.TacticIDs()[0]
}

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
	// Army is the kind of army the unit is shown as (a key of clash.armies; "" when the
	// leader has none).
	Army    string
	Troops  float64
	Max     float64
	HP, MHP int
	might   float64
	stats   map[string]float64
	Routed  bool
	Caught  bool // routed and taken (a foe)
	// Tactic is how it picks the foe it strikes (clash.tactics); Skills are its skills.
	Tactic string
	Skills []string
	charge int
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
	// Skill is the skill used ("" for a strike); Heal the troops it gave back.
	Skill string
	Heal  int
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
	// Front is how many units of a side stand in front.
	Front int
}

// FightView begins a march decided by a clash shown as units (an order with fight:
// clash_view): it spends the order, plays the clash by the rules, writes the troops back
// (Give's way: the region taken with the garrison share, the troops left shared among the
// generals, a general with none left away), and returns the clash for the screen. Nil
// when the order is not one.
func FightView(db *data.DB, st *rpg.State, id, general string, with []string, target string, tactics map[string]string) *Clash {
	c := db.Game.Campaign
	o := c.Orders[id]
	r := db.Regions[target]
	if o == nil || o.Fight != "clash_view" || r == nil {
		return nil
	}
	Spend(db, st, id, general, with, target)
	cl := &Clash{Region: target, Owner: data.RegionOwner(db, st.Campaign(), target), MaxRounds: defaultRounds, Front: Fronts(db), Background: c.Clash.Backgrounds[r.Terrain]}
	if c.Clash.Rounds > 0 {
		cl.MaxRounds = c.Clash.Rounds
	}
	generals := append([]string{general}, with...)
	for _, g := range generals {
		if len(cl.Sides[0]) == Cap(db, o) {
			break
		}
		u := generalUnit(db, st, g)
		u.Tactic = TacticOf(db, g, tactics)
		cl.Sides[0] = append(cl.Sides[0], u)
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
		fought(st, target)
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
	u := Unit{Name: GeneralLabelName(db, id), Actor: id, might: c.Generals[id].Might, stats: map[string]float64{}, Skills: c.Generals[id].Skills}
	u.Army = c.Generals[id].Army
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
	if len(ids) > Units(db) {
		ids = ids[:Units(db)]
	}
	if len(ids) == 0 {
		return []Unit{{Name: data.RegionName(db, target), Troops: garrison, Max: garrison, stats: map[string]float64{}, Tactic: db.Game.Campaign.Clash.TacticIDs()[0]}}
	}
	var out []Unit
	given := 0.0
	for i, id := range ids {
		n := math.Floor(garrison / float64(len(ids)))
		if i == len(ids)-1 {
			n = garrison - given
		}
		given += n
		out = append(out, enemyUnit(db, id, n))
	}
	return out
}

// enemyUnit is a unit led by an enemy with troops.
func enemyUnit(db *data.DB, id string, n float64) Unit {
	u := Unit{Name: id, Enemy: id, Troops: n, Max: n, stats: map[string]float64{}, Tactic: db.Game.Campaign.Clash.TacticIDs()[0], Skills: foeSkills(db, id)}
	if e := db.Enemies[id]; e != nil {
		u.Name, u.Picture, u.Army = e.Name, e.Image.File, e.Army
		u.MHP = e.Stats["mhp"]
		u.HP = u.MHP
		for _, s := range data.StatNames {
			u.stats[s] = float64(e.Stats[s])
		}
		u.stats["level"] = float64(e.Level)
	}
	return u
}

// UsesClashes reports whether the campaign shows marches as clashes of units (an order
// with fight: clash_view).
func UsesClashes(db *data.DB) bool {
	for _, o := range db.Game.Campaign.Orders {
		if o != nil && o.Fight == "clash_view" {
			return true
		}
	}
	return false
}

// FactionGenerals are the generals a faction marches with: its strongest (by atk, in
// the written order on a tie), up to clash.units.
func FactionGenerals(db *data.DB, f string) []string {
	fa := db.Factions[f]
	if fa == nil {
		return nil
	}
	ids := slices.Clone(fa.Generals)
	atk := func(id string) int {
		if e := db.Enemies[id]; e != nil {
			return e.Stats["atk"]
		}
		return 0
	}
	slices.SortStableFunc(ids, func(a, b string) int { return atk(b) - atk(a) })
	return ids[:min(len(ids), Units(db))]
}

// factionClash fights a faction's march as a clash of units: its generals sharing the
// troops sent against the target's defenders. It returns whether the march took the
// region, the troops left of the march and of the region.
func factionClash(db *data.DB, st *rpg.State, f string, sent float64, target string) (bool, float64, float64) {
	gs := FactionGenerals(db, f)
	cl := &Clash{Region: target, MaxRounds: defaultRounds, Front: Fronts(db)}
	if db.Game.Campaign.Clash.Rounds > 0 {
		cl.MaxRounds = db.Game.Campaign.Clash.Rounds
	}
	given := 0.0
	for i, g := range gs {
		n := math.Floor(sent / float64(len(gs)))
		if i == len(gs)-1 {
			n = sent - given
		}
		given += n
		cl.Sides[0] = append(cl.Sides[0], enemyUnit(db, g, n))
	}
	cl.Sides[1] = foeUnits(db, st, target)
	play(db, st, cl)
	left, held := 0.0, 0.0
	for _, u := range cl.Sides[0] {
		left += u.Troops
	}
	for _, u := range cl.Sides[1] {
		held += u.Troops
	}
	return cl.Result == "won", math.Floor(left), math.Floor(held)
}

// play fights the rounds: each round every unit standing gains a charge; a unit whose
// skill is charged uses it, else strikes a foe its tactic picks.
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
	front := Fronts(db)
	alive := func(side int) []int {
		var out []int
		for i, u := range cl.Sides[side] {
			if !u.Routed {
				out = append(out, i)
			}
		}
		return out
	}
	heal := 0.0 // the troops a skill gave back, told with its first strike
	strike := func(round int, side, at, to int, n float64, skill string) {
		u, v := &cl.Sides[side][at], &cl.Sides[1-side][to]
		if math.IsNaN(n) || n < 0 {
			n = 0
		}
		n *= scale(tactic(db, u.Tactic).Deal) * scale(tactic(db, v.Tactic).Take)
		loss := math.Min(v.Troops, math.Max(1, math.Floor(n)))
		v.Troops -= loss
		h := Hit{Round: round, Side: side, At: at, Of: 1 - side, To: to, Attackers: int(math.Ceil(u.Troops * 0.3)), Loss: int(loss), Skill: skill}
		if skill != "" && heal > 0 {
			h.Heal, heal = int(heal), 0
		}
		if v.Troops <= 0 {
			v.Troops, v.Routed, h.Routed = 0, true, true
			if v.Enemy != "" && st.Rand.Float64() < capture {
				v.Caught, h.Caught = true, true
			}
		}
		cl.Hits = append(cl.Hits, h)
	}
	for round := 1; round <= cl.MaxRounds; round++ {
		cl.Rounds = round
		type turn struct{ side, at int }
		var order []turn
		for side := range 2 {
			for _, i := range alive(side) {
				cl.Sides[side][i].charge++
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
			inFront := foes[:min(front, len(foes))]
			env := func(v *Unit) expr.Chain {
				return expr.Chain{expr.Map{"a": unitMap(u, t.side), "b": unitMap(v, 1-t.side), "round": float64(round)}, st.Env()}
			}
			if sk, id := chargedSkill(db, u); sk != nil {
				u.charge = 0
				power := sk.Power
				targets := []int{pickFoe(db, st, u, t.side, cl, inFront, foes, round)}
				if sk.All {
					targets = slices.Clone(inFront)
				}
				if sk.Heal.Set() {
					if n, err := sk.Heal.Number(env(u)); err == nil && n > 0 {
						heal = math.Min(u.Max-u.Troops, math.Floor(n))
						u.Troops += heal
					}
				}
				for _, to := range targets {
					v := &cl.Sides[1-t.side][to]
					var n float64
					var err error
					if power.Set() {
						n, err = power.Number(env(v))
					} else {
						n, err = hit.Number(env(v))
						n *= 2
					}
					if err != nil {
						n = 0
					}
					strike(round, t.side, t.at, to, n, id)
				}
				continue
			}
			to := pickFoe(db, st, u, t.side, cl, inFront, foes, round)
			n, err := hit.Number(env(&cl.Sides[1-t.side][to]))
			if err != nil {
				n = 0
			}
			strike(round, t.side, t.at, to, n, "")
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

// tactic is a unit's tactic (the engine's front when it has none).
func tactic(db *data.DB, id string) *data.ClashTactic {
	if t := db.Game.Campaign.Clash.DeclaredTactics()[id]; t != nil {
		return t
	}
	return &data.ClashTactic{}
}

func scale(f float64) float64 {
	if f <= 0 {
		return 1
	}
	return f
}

// pickFoe is the foe a unit strikes: among those its tactic looks at (in front, or all)
// and allows (where), the least or most by by, or one at random; one in front at random
// when none is left.
func pickFoe(db *data.DB, st *rpg.State, u *Unit, side int, cl *Clash, inFront, all []int, round int) int {
	t := tactic(db, u.Tactic)
	pool := inFront
	if t.All {
		pool = all
	}
	env := func(v *Unit) expr.Env {
		return expr.Chain{expr.Map{"a": unitMap(u, side), "t": unitMap(v, 1-side), "round": float64(round)}, st.Env()}
	}
	var ok []int
	for _, i := range pool {
		if !t.Where.Set() {
			ok = append(ok, i)
			continue
		}
		if yes, err := t.Where.Bool(env(&cl.Sides[1-side][i])); err == nil && yes {
			ok = append(ok, i)
		}
	}
	if len(ok) == 0 {
		ok = inFront
	}
	if (t.Pick == "least" || t.Pick == "most") && t.By.Set() {
		best, bestN := ok[0], math.NaN()
		for _, i := range ok {
			n, err := t.By.Number(env(&cl.Sides[1-side][i]))
			if err != nil || math.IsNaN(n) {
				continue
			}
			if math.IsNaN(bestN) || (t.Pick == "least" && n < bestN) || (t.Pick == "most" && n > bestN) {
				best, bestN = i, n
			}
		}
		return best
	}
	return ok[st.Rand.IntN(len(ok))]
}

// chargedSkill is the unit's first skill charged (its rounds since the last one reached
// the skill's charge), with its id.
func chargedSkill(db *data.DB, u *Unit) (*data.ClashSkill, string) {
	for _, id := range u.Skills {
		sk := db.Game.Campaign.Clash.Skills[id]
		if sk == nil {
			continue
		}
		need := sk.Charge
		if need <= 0 {
			need = 2
		}
		if u.charge >= need {
			return sk, id
		}
	}
	return nil, ""
}

// foeSkills are the skills an enemy uses (clash.skills.<id>.foes).
func foeSkills(db *data.DB, enemy string) []string {
	var out []string
	sk := db.Game.Campaign.Clash.Skills
	for _, id := range slices.Sorted(maps.Keys(sk)) {
		if slices.Contains(sk[id].Foes, enemy) {
			out = append(out, id)
		}
	}
	return out
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
