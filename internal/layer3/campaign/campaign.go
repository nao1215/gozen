// Package campaign is the rules of a campaign (ADR 0030): the player's orders, a march
// decided by numbers, the other factions' turns, and winning and losing. It reads and
// writes the state's variables with the generated names of internal/layer2/gamedata, and
// draws its random numbers from the state, so a seed decides a whole campaign.
package campaign

import (
	"errors"
	"fmt"
	"math"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// The defaults of the clash.
var (
	defaultPower   = data.MustExpr("a.troops * (0.8 + rand() * 0.4)")
	defaultDefense = data.MustExpr("b.troops * 1.25")
	defaultWon     = data.MustExpr("a.troops * (1 - 0.5 * defense / power)")
	defaultLost    = data.MustExpr("a.troops * 0.5")
	defaultHeld    = data.MustExpr("b.troops - power * 0.4")
)

// DoneKey marks the end of a campaign in State.Done (so it ends once).
const DoneKey = "campaign/over"

// Order is the order of the campaign with the id (nil when none).
func Order(db *data.DB, id string) *data.Order {
	if c := db.Game.Campaign; c != nil {
		return c.Orders[id]
	}
	return nil
}

// Open reports why the order cannot be given now ("" when it can): its when, or the
// actions left.
func Open(db *data.DB, st *rpg.State, o *data.Order) string {
	if data.ActionsLeft(db, st.Campaign()) < float64(o.CostOf()) {
		return lang.T("このターンの行動が残っていない")
	}
	ok, err := o.When.Bool(st.Env())
	if err != nil || !ok {
		return lang.T("今はこの命令を出せない")
	}
	return ""
}

// Generals are the generals who can lead the order now, in the order of the actors.
func Generals(db *data.DB, st *rpg.State, o *data.Order) []string {
	c := db.Game.Campaign
	var out []string
	for _, id := range db.Ordered("actors") {
		if _, ok := c.Generals[id]; !ok || st.Actors[id] == nil {
			continue
		}
		if o.General == "ready" && !data.Ready(db, st.Campaign(), id) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// Targets are the regions the order can be given, in the written order.
func Targets(db *data.DB, st *rpg.State, o *data.Order) []string {
	player := db.Game.Campaign.Player
	var out []string
	for _, id := range data.RegionIDs(db) {
		owner := data.RegionOwner(db, st.Campaign(), id)
		switch o.Target {
		case "border":
			if owner == player || !data.Borders(db, st.Campaign(), id, player) {
				continue
			}
		case "own":
			if owner != player {
				continue
			}
		}
		if o.Where.Set() {
			ok, err := o.Where.Bool(expr.Chain{expr.Map{"t": region(db, st, id)}, st.Env()})
			if err != nil || !ok {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

// GeneralLabel and TargetLabel are the options of the choices that ask for them.
func GeneralLabel(db *data.DB, st *rpg.State, id string) string {
	name := id
	if a := db.Actors[id]; a != nil && a.Name != "" {
		name = a.Name
	}
	return fmt.Sprintf(lang.T("%s（兵 %d）"), name, int(data.GeneralTroops(db, st.Campaign(), id)))
}

// TargetLabel is a region as an option: its name, its owner and its garrison.
func TargetLabel(db *data.DB, st *rpg.State, id string) string {
	return fmt.Sprintf(lang.T("%s（%s・兵 %d）"), data.RegionName(db, id), FactionName(db, data.RegionOwner(db, st.Campaign(), id)), int(data.Garrison(db, st.Campaign(), id)))
}

// FactionName is the name a faction shows (the engine's word for none when "").
func FactionName(db *data.DB, id string) string {
	f := db.Factions[id]
	switch {
	case id == "" || f == nil:
		return lang.T("空き")
	case f.Name != "":
		return f.Name
	}
	return id
}

// Give gives the order with its general and target (each "" when it needs none): it binds
// them as general and target, spends the actions, and marches; the lines tell how the
// march went. The order's do is run by the caller next.
func Give(db *data.DB, st *rpg.State, o *data.Order, general, target string) []string {
	st.Vars[data.VarGeneral], st.Vars[data.VarTarget], st.Vars[data.VarResult] = general, target, ""
	st.Vars[data.VarActions] = data.ActionsLeft(db, st.Campaign()) - float64(o.CostOf())
	if !o.March {
		return nil
	}
	c := db.Game.Campaign
	troops := data.GeneralTroops(db, st.Campaign(), general)
	a := expr.Map{"troops": troops, "might": c.Generals[general].Might, "faction": c.Player, "general": general, "from": "", "level": 0.0}
	// The general is the actor of the dungeon and the battles: the same level and stats.
	for _, n := range data.StatNames {
		a[n] = 0.0
	}
	if act := st.Actors[general]; act != nil {
		a["level"] = float64(act.Level)
		for _, n := range data.StatNames {
			a[n] = float64(act.Stat(n))
		}
	}
	won, left, held, power, defense := clash(db, st, a, target)
	st.Vars[data.GeneralVar(general, "acted")] = 1.0
	name := GeneralLabelName(db, general)
	if won {
		garrison := math.Floor(left * c.Clash.GarrisonShare())
		st.Vars[data.GeneralVar(general, "troops")] = left - garrison
		take(st, target, c.Player, garrison)
		st.Vars[data.VarResult] = "won"
		return []string{fmt.Sprintf(lang.T("%sが%sを落とした（%d 対 %d）"), name, data.RegionName(db, target), int(power), int(defense))}
	}
	st.Vars[data.GeneralVar(general, "troops")] = left
	st.Vars[data.RegionVar(target, "troops")] = held
	st.Vars[data.VarResult] = "held"
	return []string{fmt.Sprintf(lang.T("%sは持ちこたえた（%d 対 %d）"), data.RegionName(db, target), int(power), int(defense))}
}

// GeneralLabelName is the name of a general's actor.
func GeneralLabelName(db *data.DB, id string) string {
	if a := db.Actors[id]; a != nil && a.Name != "" {
		return a.Name
	}
	return id
}

func take(st *rpg.State, id, faction string, troops float64) {
	st.Vars[data.RegionVar(id, "owner")] = faction
	st.Vars[data.RegionVar(id, "troops")] = troops
}

// clash decides a march of a on the region b: whether it takes b, the troops of a left
// (won or lost), the garrison of b left when it holds, and the two strengths.
func clash(db *data.DB, st *rpg.State, a expr.Map, b string) (won bool, left, held, power, defense float64) {
	c := db.Game.Campaign.Clash
	or := func(e, def data.Expr) data.Expr {
		if e.Set() {
			return e
		}
		return def
	}
	env := expr.Map{"a": a, "b": region(db, st, b)}
	chain := expr.Chain{env, st.Env()}
	num := func(e data.Expr) float64 {
		n, err := e.Number(chain)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0
		}
		return n
	}
	power = max(0, num(or(c.Power, defaultPower)))
	defense = max(0, num(or(c.Defense, defaultDefense)))
	env["power"], env["defense"] = power, defense
	if power > defense {
		return true, troops(num(or(c.Won, defaultWon))), 0, power, defense
	}
	return false, troops(num(or(c.Lost, defaultLost))), troops(num(or(c.Held, defaultHeld))), power, defense
}

// troops is a count of troops: whole and not below 0.
func troops(n float64) float64 { return max(0, math.Floor(n)) }

// region is the region as the expressions of a march see it (t, s, b).
func region(db *data.DB, st *rpg.State, id string) any {
	v, _ := data.CampaignEnv(db, st.Campaign()).Lookup("region")
	o, ok := v.(expr.Object)
	if !ok {
		return nil
	}
	r, _ := o.Field(id)
	return r
}

// EndTurn ends the player's turn: every other faction that holds a region follows its
// rules, the next turn begins with its actions, and the generals may march again. The
// lines tell what the factions did; the commands are the on_turn event.
func EndTurn(db *data.DB, st *rpg.State) ([]string, []data.Command) {
	c := db.Game.Campaign
	var lines []string
	for _, f := range data.FactionIDs(db) {
		if f == c.Player || len(data.RegionsOf(db, st.Campaign(), f)) == 0 {
			continue
		}
		lines = append(lines, FactionTurn(db, st, f)...)
	}
	st.Vars[data.VarTurn] = float64(data.Turn(db, st.Campaign()) + 1)
	for id := range c.Generals {
		delete(st.Vars, data.GeneralVar(id, "acted"))
	}
	st.Vars[data.VarGeneral], st.Vars[data.VarTarget], st.Vars[data.VarResult] = "", "", ""
	st.Vars[data.VarActions] = data.TurnActions(db, st.Campaign())
	if ev := db.Events[c.OnTurn]; ev != nil {
		return lines, ev.Do
	}
	return lines, nil
}

// FactionTurn runs a faction's rules in their order (the other factions at the end of
// the player's turn; the player's own rules only in gozen sim campaign --as ai).
func FactionTurn(db *data.DB, st *rpg.State, f string) []string {
	var lines []string
	for i := range db.Factions[f].AI {
		r := &db.Factions[f].AI[i]
		if ok, err := r.When.Bool(st.Env()); err != nil || !ok {
			continue
		}
		if r.Draft.Set() {
			for _, id := range data.RegionsOf(db, st.Campaign(), f) {
				n, err := r.Draft.Number(expr.Chain{expr.Map{"t": region(db, st, id)}, st.Env()})
				if err == nil {
					st.Vars[data.RegionVar(id, "troops")] = troops(data.Garrison(db, st.Campaign(), id) + n)
				}
			}
			continue
		}
		if line := march(db, st, f, r); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// march is a faction's march by the rule r, if any border allows one.
func march(db *data.DB, st *rpg.State, f string, r *data.FactionRule) string {
	type pair struct {
		s, t string
		by   float64
	}
	var cands []pair
	for _, s := range data.RegionsOf(db, st.Campaign(), f) {
		if data.Garrison(db, st.Campaign(), s) < r.Keep || data.Garrison(db, st.Campaign(), s) <= 0 {
			continue
		}
		for _, t := range db.Regions[s].Next {
			if db.Regions[t] == nil || data.RegionOwner(db, st.Campaign(), t) == f {
				continue
			}
			env := expr.Chain{expr.Map{"s": region(db, st, s), "t": region(db, st, t)}, st.Env()}
			if ok, err := r.Where.Bool(env); err != nil || !ok {
				continue
			}
			by, err := r.By.Number(env)
			if err != nil {
				continue
			}
			cands = append(cands, pair{s, t, by})
		}
	}
	if len(cands) == 0 {
		return ""
	}
	best := cands[0]
	switch r.Pick {
	case "min", "max":
		for _, p := range cands[1:] {
			if r.Pick == "min" && p.by < best.by || r.Pick == "max" && p.by > best.by {
				best = p
			}
		}
	case "random":
		best = cands[st.Rand.IntN(len(cands))]
	}
	send := r.Send
	if send == 0 {
		send = 0.6
	}
	sent := math.Floor(data.Garrison(db, st.Campaign(), best.s) * send)
	st.Vars[data.RegionVar(best.s, "troops")] = data.Garrison(db, st.Campaign(), best.s) - sent
	a := expr.Map{"troops": sent, "might": 0.0, "faction": f, "general": "", "from": best.s, "level": 0.0}
	for _, n := range data.StatNames {
		a[n] = 0.0
	}
	won, left, held, _, _ := clash(db, st, a, best.t)
	if won {
		take(st, best.t, f, left)
		return fmt.Sprintf(lang.T("%sが%sを落とした"), FactionName(db, f), data.RegionName(db, best.t))
	}
	st.Vars[data.RegionVar(best.t, "troops")] = held
	return fmt.Sprintf(lang.T("%sは%sの攻めを持ちこたえた"), data.RegionName(db, best.t), FactionName(db, f))
}

// Ending is how the campaign ends now, once: the lost event and a game over when lose
// holds or the last turn has ended, else the won event and the game cleared when win
// holds, else nil.
func Ending(db *data.DB, st *rpg.State) []data.Command {
	c := db.Game.Campaign
	if c == nil || st.Done[DoneKey] {
		return nil
	}
	holds := func(e data.Expr) bool {
		if !e.Set() {
			return false
		}
		ok, err := e.Bool(st.Env())
		return err == nil && ok
	}
	end := func(event string, last data.Command) []data.Command {
		st.Done[DoneKey] = true
		var cmds []data.Command
		if db.Events[event] != nil {
			cmds = append(cmds, data.Command{Call: event})
		}
		return append(cmds, last)
	}
	switch {
	case holds(data.CampaignLose(db)) || c.Turns > 0 && data.Turn(db, st.Campaign()) > c.Turns && !holds(data.CampaignWin(db)):
		return end(c.Lost, data.Command{GameOver: true})
	case holds(data.CampaignWin(db)):
		return end(c.Won, data.Command{Clear: true})
	}
	return nil
}

// ErrNoCampaign is the error of a campaign's command in a game without one.
var ErrNoCampaign = errors.New("the game has no campaign")

// Choices lists ids as options, with a last option to cancel.
func Choices(ids []string, label func(string) string) []string {
	out := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		out = append(out, label(id))
	}
	return append(out, lang.T("やめる"))
}

// Estimate is how a general's march on a region would likely go, without drawing any
// random number: rand() is its middle (0.5) in the clash's formulas.
func Estimate(db *data.DB, st *rpg.State, general, target string) (float64, float64) {
	c := db.Game.Campaign
	a := expr.Map{"troops": data.GeneralTroops(db, st.Campaign(), general), "might": c.Generals[general].Might, "faction": c.Player, "general": general, "from": "", "level": 0.0}
	for _, n := range data.StatNames {
		a[n] = 0.0
	}
	if act := st.Actors[general]; act != nil {
		a["level"] = float64(act.Level)
		for _, n := range data.StatNames {
			a[n] = float64(act.Stat(n))
		}
	}
	middle := expr.Func(func(args []any) (any, error) {
		switch len(args) {
		case 0:
			return 0.5, nil
		case 1:
			n, _ := expr.ToNumber(args[0])
			return math.Floor(n / 2), nil
		}
		x, _ := expr.ToNumber(args[0])
		y, _ := expr.ToNumber(args[1])
		return math.Floor((x + y) / 2), nil
	})
	env := expr.Chain{expr.Map{"a": a, "b": region(db, st, target), "rand": middle}, st.Env()}
	or := func(e, def data.Expr) data.Expr {
		if e.Set() {
			return e
		}
		return def
	}
	power, perr := or(c.Clash.Power, defaultPower).Number(env)
	defense, derr := or(c.Clash.Defense, defaultDefense).Number(env)
	if perr != nil || derr != nil {
		return 0, 1 // a formula that fails: no march looks good
	}
	return power, defense
}

// Fight begins a march decided by a battle (an order with fight: battle on a region with
// defend): it binds the general and the target, spends the actions, marks the general as
// marched, and returns the battle, the general alone against the region's troop, each end
// followed by the internal clash command that writes the troops back. Nil when the target
// has no troop to fight (the march is then decided by the numbers, Give).
func Fight(db *data.DB, st *rpg.State, o *data.Order, general, target string) []data.Command {
	r := db.Regions[target]
	if o.Fight != "battle" || r == nil || r.Defend == "" || db.Troops[r.Defend] == nil {
		return nil
	}
	st.Vars[data.VarGeneral], st.Vars[data.VarTarget], st.Vars[data.VarResult] = general, target, ""
	st.Vars[data.VarActions] = data.ActionsLeft(db, st.Campaign()) - float64(o.CostOf())
	st.Vars[data.GeneralVar(general, "acted")] = 1.0
	no := false
	return []data.Command{{
		Battle: r.Defend, Fighters: []string{general}, Retry: &no,
		OnWin: []data.Command{{Clash: "won"}}, OnLose: []data.Command{{Clash: "lost"}}, OnRun: []data.Command{{Clash: "lost"}},
	}}
}

// AfterBattle writes back a march decided by a battle: won, the general keeps the share of
// the troops of the HP left (a fifth at least) and takes the region, leaving the garrison
// share there; lost (or fled), the troops left are the clash's lost and the region holds.
func AfterBattle(db *data.DB, st *rpg.State, won bool) []string {
	c := db.Game.Campaign
	cs := st.Campaign()
	general := optional.Value[string](cs.Vars[data.VarGeneral])
	target := optional.Value[string](cs.Vars[data.VarTarget])
	if c == nil || db.Regions[target] == nil {
		return nil
	}
	troops := data.GeneralTroops(db, cs, general)
	name := GeneralLabelName(db, general)
	if won {
		rate := 1.0
		if a := st.Actors[general]; a != nil && a.Stat("mhp") > 0 {
			rate = float64(a.HP) / float64(a.Stat("mhp"))
		}
		left := math.Floor(troops * max(0.2, rate))
		garrison := math.Floor(left * c.Clash.GarrisonShare())
		st.Vars[data.GeneralVar(general, "troops")] = left - garrison
		take(st, target, c.Player, garrison)
		st.Vars[data.VarResult] = "won"
		return []string{fmt.Sprintf(lang.T("%sが%sを落とした"), name, data.RegionName(db, target))}
	}
	a := expr.Map{"troops": troops, "might": c.Generals[general].Might, "faction": c.Player, "general": general, "from": "", "level": 0.0}
	for _, n := range data.StatNames {
		a[n] = 0.0
	}
	env := expr.Chain{expr.Map{"a": a, "b": region(db, st, target), "power": 0.0, "defense": 0.0}, st.Env()}
	lost := c.Clash.Lost
	if !lost.Set() {
		lost = defaultLost
	}
	n, err := lost.Number(env)
	if err != nil {
		n = troops / 2
	}
	st.Vars[data.GeneralVar(general, "troops")] = max(0, math.Floor(n))
	st.Vars[data.VarResult] = "held"
	return []string{fmt.Sprintf(lang.T("%sは%sの攻めを持ちこたえた"), data.RegionName(db, target), name)}
}
