package battle

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
)

// Result is how a battle ended.
type Result int

// The results.
const (
	Ongoing Result = iota
	Won
	Lost
	Fled
)

// String returns the outcome token used in reports, or "unknown" for an invalid value.
func (r Result) String() string {
	if r < Ongoing || r > Fled {
		return "unknown"
	}
	return [...]string{"", "win", "lose", "escape"}[r]
}

// StepKind is the kind of a Step.
type StepKind int

// The kinds of steps.
const (
	Message   StepKind = iota // Text
	Act                       // User starts an action (Skill); Text is its message
	Damage                    // Target loses Amount of Stat
	Heal                      // Target gains Amount of Stat
	Miss                      // the action missed Target
	StateOn                   // State was put on Target
	StateOff                  // State left Target (User: the one whose action removed it; nil when it wore off)
	Fall                      // Target fell
	Revive                    // Target got up
	Escape                    // the party ran
	Event                     // a troop event: run Commands, then go on
	Summon                    // Target was called into the battle (a new foe)
	Transform                 // Target changed enemy form in its place
	Merge                     // Members combined into Target
)

// Step is one thing that happened, for the screen to show.
type Step struct {
	Kind     StepKind
	Text     string
	User     *Unit
	Target   *Unit
	Amount   int
	Stat     string // hp or mp
	Crit     bool
	Weak     bool   // the element hit hard
	Resisted bool   // the element hardly hurt
	Element  string // the element of the damage ("" none)
	State    string
	Skill    string
	Anim     string
	Color    data.Color
	HP, MP   int // the target's after the step
	Commands []data.Command
	Before   string  // previous picture for a form transition
	Members  []*Unit // consumed units in a merge
}

// Action is what a battler does on its turn.
type Action struct {
	User   *Unit
	Skill  string // a skill, or
	Item   string // an item from the bag
	Target *Unit  // the chosen target of a one_* scope (nil: chosen at random)
	Say    string // a line said first
	Pass   bool   // does nothing (the party after a failed escape)
	tactic *actionTactic
	speed  float64
}

// Battle is a battle in progress.
type Battle struct {
	DB      *data.DB
	St      *rpg.State
	TroopID string
	common  bool // a troop met at random (battle.foe_scale)
	Troop   *data.Troop
	Party   []*Unit
	Foes    []*Unit
	Turn    int
	Result  Result
	Rand    *rand.Rand

	// OnDecision receives the record of an actual evaluation. If nil, nothing is notified
	// externally.
	OnDecision func(Decision)

	advice battleAdvice

	queue      []Action
	using      string // the skill being used (for lv in its formulas)
	countering bool   // a counter is being played (it is not countered back)
	fired      map[int]bool
	// scale is the troop's level_scale for this party (1: none). Set at the start, kept for the summons.
	scale float64
	phase string
	// gone are the foes whose places a summon took: still counted for the kills and the rewards.
	gone []*Unit
	// Gains are the rewards of a won battle (set when the battle is won).
	Gains Gains
}

// commonTroop reports whether the troop is met at random (in some location's encounters):
// the battles battle.foe_scale toughens, not the arena's nor the strong foes walking a floor.
func commonTroop(db *data.DB, troop string) bool {
	for _, l := range db.Locations {
		for _, p := range l.Encounters.Troops {
			if p.Troop == troop {
				return true
			}
		}
	}
	return false
}

// New starts a battle of the party in st against the troop.
func New(db *data.DB, st *rpg.State, troop string) *Battle {
	b := &Battle{DB: db, St: st, TroopID: troop, Troop: db.Troops[troop], Rand: st.Rand, fired: map[int]bool{}, Turn: 1, phase: "start"}
	b.common = commonTroop(db, troop)
	if w := db.Game.Battle.FoeScaleWhen; b.common && w.Set() {
		ok, err := w.Bool(st.Env())
		b.common = err == nil && ok
	}
	members := st.BattleMembers()
	for i, a := range members {
		b.Party = append(b.Party, newActorUnit(b, a, i))
	}
	for _, p := range db.Game.Battle.Passives { // hidden abilities and the like (only for the battle; made into states that do not remain after End)
		if db.States[p.State] == nil {
			continue
		}
		if p.When.Set() {
			if ok, err := p.When.Bool(st.Env()); err != nil || !ok {
				continue
			}
		}
		for _, u := range b.Party {
			if u.Actor.ID == p.Actor {
				u.States[p.State] = stateForever
			}
		}
	}
	b.scale = b.Troop.LevelScale.Factor(avgLevel(b.Party))
	names := map[string]int{}
	for _, m := range b.Troop.Members {
		names[m]++
	}
	seen := map[string]int{}
	for i, m := range b.Troop.Members {
		u := newEnemyUnit(b, m, i)
		if names[m] > 1 {
			letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
			if n := seen[m]; n >= 0 && n < len(letters) {
				u.Name += letters[n : n+1]
			} else {
				u.Name += fmt.Sprintf("(%d)", n+1)
			}
			seen[m]++
		}
		b.Foes = append(b.Foes, u)
	}
	return b
}

// avgLevel is the average level of the units, rounded down (0 for none).
func avgLevel(us []*Unit) int {
	sum := 0
	for _, u := range us {
		sum += u.level()
	}
	return sum / max(1, len(us))
}

// Env is the scope of battle expressions for unit u (nil for the troop's events): self,
// allies, foes, party, enemies, turn, result, phase, boss, and the field's names.
func (b *Battle) Env(u *Unit) expr.Env {
	m := expr.Map{
		"turn":    float64(b.Turn),
		"party":   group(b.Party),
		"enemies": group(b.Foes),
		"result":  b.Result.String(),
		"phase":   b.phase,
		"boss":    b.boss(),
	}
	if u != nil {
		m["self"] = u
		m["allies"] = group(u.allies())
		m["foes"] = group(u.foes())
	} else {
		m["allies"] = group(b.Foes)
		m["foes"] = group(b.Party)
	}
	return expr.Chain{m, b.St.Env()}
}

// boss is the troop's boss (the first enemy marked boss, else the first enemy).
func (b *Battle) boss() any {
	for _, u := range b.Foes {
		if u.Def.Boss {
			return u
		}
	}
	if len(b.Foes) > 0 {
		return b.Foes[0]
	}
	return nil
}

// Phase is "input" while the party chooses, "act" while actions are played, "start" and
// "end" around them (the battle music rules see it).
func (b *Battle) Phase() string { return b.phase }

// SetPhase sets the phase the screen is in.
func (b *Battle) SetPhase(p string) { b.phase = p }

// Begin is the opening of the battle: the troop's intro and the events of turn 1.
func (b *Battle) Begin() []Step {
	var steps []Step
	intro := b.Troop.Intro
	env := b.Env(nil)
	if intro == "" {
		// the kinds of foes (their names without the letters A, B...): "X が", "X たちが",
		// "X と Y が", or a mixed troop of more kinds
		var kinds []string
		for _, f := range b.Foes {
			if !slices.Contains(kinds, f.Def.Name) {
				kinds = append(kinds, f.Def.Name)
			}
		}
		switch {
		case len(b.Foes) == 1:
			intro = lang.T("{first}が あらわれた！")
		case len(kinds) == 1:
			intro = lang.T("{first}たちが あらわれた！")
		case len(kinds) == 2:
			intro = lang.T("{first}と {second}が あらわれた！")
		default:
			intro = lang.T("{first}や {second}たちが あらわれた！")
		}
		names := expr.Map{"first": kinds[0], "second": ""}
		if len(kinds) > 1 {
			names["second"] = kinds[1]
		}
		env = expr.Chain{names, env}
	}
	steps = append(steps, Step{Kind: Message, Text: data.Render(intro, env)})
	return append(steps, b.events()...)
}

// Commandable are the party members who choose an action this turn: those standing and
// free to act and not on AUTO.
func (b *Battle) Commandable() []*Unit {
	var out []*Unit
	for _, u := range b.Party {
		if r, _ := u.restrict(); u.Alive() && r == "" && !u.Auto {
			out = append(out, u)
		}
	}
	return out
}

// CanUse reports whether the unit can use the skill now (enough MP), and why not.
func (b *Battle) CanUse(u *Unit, skill string) (bool, string) {
	s := b.DB.Skills[skill]
	if s == nil {
		return false, "unknown skill"
	}
	if u.MP < s.MP {
		return false, lang.T("MPが足りない")
	}
	return true, ""
}

// StartTurn queues the turn's actions: the party's (given) and every other's (from
// their rules), fastest first.
func (b *Battle) StartTurn(party []Action) {
	b.queue = b.queue[:0]
	b.phase = "act"
	chosen := map[*Unit]bool{}
	for _, a := range party {
		if a.User != nil && a.User.Alive() {
			b.queue = append(b.queue, a)
			chosen[a.User] = true
		}
	}
	for _, u := range b.Party {
		if u.Alive() && !chosen[u] {
			if !u.restricted() {
				// Defense and speed-specified skills are fixed at the start of the turn. Others
				// are chosen again at action time.
				if d := b.decide(u, DecisionTurnStart); b.earlyAction(d) {
					b.queue = append(b.queue, d)
					continue
				}
			}
			b.queue = append(b.queue, Action{User: u}) // decided on its turn (AUTO, or restricted)
		}
	}
	for _, u := range b.Foes {
		if u.Alive() {
			for range u.Def.Actions {
				b.queue = append(b.queue, Action{User: u})
			}
		}
	}
	for i := range b.queue {
		a := &b.queue[i]
		a.speed = b.speed(a)
		if a.Skill == b.DB.Game.Battle.Guard {
			a.User.Guard = true
			a.speed += 1e6 // guarding takes effect before anyone acts
		}
	}
	slices.SortStableFunc(b.queue, func(x, y Action) int { return cmp.Compare(y.speed, x.speed) })
}

func (b *Battle) speed(a *Action) float64 {
	u := a.User
	env := expr.Chain{expr.Map{"a": u, "b": u}, b.Env(u)}
	v := float64(u.Stat("agi")) * (0.8 + 0.4*b.Rand.Float64())
	if e := b.DB.Game.Battle.Speed; e.Set() {
		if x, err := e.Number(env); err == nil {
			v = x
		}
	}
	if s := b.usage(a); s != nil && s.Speed.Set() {
		if x, err := s.Speed.Number(env); err == nil {
			v += x
		}
	}
	return v
}

func (b *Battle) usage(a *Action) *data.Usage {
	if a.Item != "" {
		if it := b.DB.Items[a.Item]; it != nil {
			return it.Use
		}
		return nil
	}
	if s := b.DB.Skills[a.Skill]; s != nil {
		return &s.Usage
	}
	return nil
}

// Next plays the next action of the turn. When the queue is empty it ends the turn (the
// states' turn-end effects) and reports done. Every call may also report troop events.
func (b *Battle) Next() (steps []Step, done bool) {
	for len(b.queue) > 0 && b.Result == Ongoing {
		a := b.queue[0]
		b.queue = b.queue[1:]
		if !a.User.Alive() {
			continue
		}
		steps = b.act(a)
		b.observeFalls(steps)
		b.judge()
		steps = append(steps, b.events()...)
		if len(steps) > 0 {
			return steps, false
		}
	}
	if b.Result != Ongoing {
		return b.finish(steps), true
	}
	steps = b.endTurn()
	b.observeFalls(steps)
	b.judge()
	if b.Result != Ongoing {
		return b.finish(steps), true
	}
	b.Turn++
	b.phase = "input"
	steps = append(steps, b.events()...)
	return steps, true
}

// judge sets the result when a side has no one standing.
func (b *Battle) judge() {
	if b.Result != Ongoing {
		return
	}
	switch {
	case len(living(b.Party)) == 0:
		b.Result = Lost
	case len(living(b.Foes)) == 0:
		b.Result = Won
	}
}

func (b *Battle) finish(steps []Step) []Step {
	b.phase = "end"
	b.queue = nil
	return steps
}

// TryEscape tries to run away (the ESCAPE command). It reports the steps and whether the
// party got away; when not, the turn goes on without the party acting.
func (b *Battle) TryEscape() ([]Step, bool) {
	if !b.Troop.Escapable() {
		return []Step{{Kind: Message, Text: lang.T("にげられない！")}}, false
	}
	p := 0.5
	if e := b.DB.Game.Battle.Escape; e.Set() {
		if v, err := e.Number(b.Env(b.Party[0])); err == nil {
			p = v
		}
	}
	if b.Rand.Float64() < p {
		b.Result = Fled
		b.phase = "end"
		return []Step{{Kind: Escape, Text: lang.T("うまく にげきれた！")}}, true
	}
	return []Step{{Kind: Message, Text: lang.T("しかし まわりこまれてしまった！")}}, false
}

// events are the troop events whose condition holds now.
func (b *Battle) events() []Step {
	var out []Step
	env := b.Env(nil)
	for i := range b.Troop.Events {
		ev := &b.Troop.Events[i]
		if ev.RunsOnce() && b.fired[i] {
			continue
		}
		ok, err := ev.When.Bool(env)
		if err != nil || !ok {
			continue
		}
		b.fired[i] = true
		out = append(out, Step{Kind: Event, Commands: ev.Do})
	}
	return out
}

// endTurn plays the states' turn-end effects and counts their turns down.
func (b *Battle) endTurn() []Step {
	var steps []Step
	for _, u := range append(slices.Clone(b.Party), b.Foes...) {
		u.Guard = false
		if !u.Alive() {
			continue
		}
		for _, s := range u.sortedStates() {
			st := b.DB.States[s]
			for i := range st.Tick {
				steps = append(steps, b.effect(u, u, &st.Tick[i], nil, "")...)
			}
			for i := range st.TickAllies {
				for _, t := range living(u.allies()) {
					steps = append(steps, b.effect(u, t, &st.TickAllies[i], nil, "")...)
				}
			}
			if !u.Alive() {
				break
			}
			if u.States[s] < stateForever {
				u.States[s]--
				if u.States[s] <= 0 {
					delete(u.States, s)
					steps = append(steps, b.stateOff(u, s))
				}
			}
		}
	}
	return steps
}

func (b *Battle) stateOff(u *Unit, s string) Step {
	st := b.DB.States[s]
	msg := st.Messages.Remove
	if msg == "" {
		msg = lang.T("{target}の") + st.Name + lang.T("が なおった！")
	}
	return Step{Kind: StateOff, Target: u, State: s, Text: b.text(msg, nil, u, "", 0), HP: u.HP, MP: u.MP}
}

// text renders a battle message: {user}, {target}, {skill}, {amount}, {state} and any
// expression of the battle's scope.
func (b *Battle) text(msg string, user, target *Unit, skill string, amount int) string {
	m := expr.Map{"skill": skill, "amount": float64(amount)}
	if user != nil {
		m["user"] = user.Name
	}
	if target != nil {
		m["target"] = target.Name
	}
	return data.Render(msg, expr.Chain{m, b.Env(user)})
}

// VarLastKill is the prefix of the variables holding the number of enemies defeated in the
// previous battle
// (var.last_kill_<enemy ID>; an enemy not defeated is absent, meaning 0).
const VarLastKill = "last_kill_"

// End writes the battle back to the party: HP, MP, the states that persist, and, for a
// win, the exp, the money and the items found. It returns the messages to show.
func (b *Battle) End() []string {
	b.saveAdvice()
	for _, u := range b.Party {
		a := u.Actor
		a.HP, a.MP = max(0, u.HP), max(0, u.MP)
		clear(a.States)
		for s, n := range u.States {
			if st := b.DB.States[s]; st != nil && st.Persist && u.HP > 0 {
				a.States[s] = n
			}
		}
	}
	participants := make([]*rpg.Actor, 0, len(b.Party))
	for _, u := range b.Party {
		participants = append(participants, u.Actor)
	}
	b.St.RecordBattle(b.Result.String(), participants)
	// The number defeated in the previous battle (var.last_kill_<enemy>) is rewritten every
	// battle, whether won, fled or lost
	// (so the previous battle's count does not linger and mix into the banter after the next
	// victory).
	for k := range b.St.Vars {
		if strings.HasPrefix(k, VarLastKill) {
			delete(b.St.Vars, k)
		}
	}
	for _, f := range b.beaten() { // what was beaten, counted in var.kill_<enemy> (for requests)
		if !f.Alive() && !f.Fled && f.EID != "" {
			k := "kill_" + f.EID
			n := optional.Value[float64](b.St.Vars[k])
			b.St.Vars[k] = n + 1
			last := optional.Value[float64](b.St.Vars[VarLastKill+f.EID])
			b.St.Vars[VarLastKill+f.EID] = last + 1
		}
	}
	if b.Result != Won {
		return nil
	}
	g := b.rewards()
	b.Gains = g
	t := b.DB.Game.Terms
	var msgs []string
	if g.Exp > 0 {
		msgs = append(msgs, fmt.Sprintf(lang.T("%dポイントの%sを かくとく！"), g.Exp, t.Exp))
	}
	if g.Gold > 0 {
		msgs = append(msgs, fmt.Sprintf(lang.T("%d%sを てにいれた！"), g.Gold, t.Currency))
	}
	b.St.Gold += g.Gold
	for _, it := range g.Items {
		b.St.AddItem(it, 1)
		msgs = append(msgs, b.DB.Items[it].Name+lang.T("を てにいれた！"))
	}
	// Members who reached the same level are combined into one line (when 5 members level up
	// together, it used to send 5 times). Learned skills are per member.
	var levels []int
	names := map[int][]string{}
	var learnt []string
	for _, u := range b.Party {
		if !u.Alive() {
			continue
		}
		for _, up := range u.Actor.GainExp(g.Exp) {
			if names[up.Level] == nil {
				levels = append(levels, up.Level)
			}
			names[up.Level] = append(names[up.Level], u.Name)
			for _, s := range up.Skills {
				learnt = append(learnt, fmt.Sprintf(lang.T("%sは %sを おぼえた！"), u.Name, b.DB.Skills[s].Name))
			}
		}
	}
	alive := 0
	for _, u := range b.Party {
		if u.Alive() {
			alive++
		}
	}
	for _, lv := range levels {
		msgs = append(msgs, rpg.LevelUpLine(names[lv], alive, t.Level, lv))
		b.Gains.LevelUps = append(b.Gains.LevelUps, len(msgs)-1)
	}
	for _, m := range learnt {
		msgs = append(msgs, m)
		b.Gains.LevelUps = append(b.Gains.LevelUps, len(msgs)-1)
	}
	return msgs
}

// Rise revives the members who took part in this battle with 1 HP when the story continues even
// after a loss (the event's on_lose).
// Members who did not fight (were outside the duel) are not revived. Call it
// after End.
func (b *Battle) Rise() {
	for _, u := range b.Party {
		u.Actor.HP = max(1, u.Actor.HP)
	}
}

// Gains are the rewards of a battle.
type Gains struct {
	// LevelUps are the indexes of End's messages that tell of a level gained or a skill learnt.
	LevelUps  []int
	Exp, Gold int
	Items     []string
}

// beaten are all the foes met in the battle: the ones standing in the ranks and the ones
// whose places a summon took.
func (b *Battle) beaten() []*Unit {
	return append(slices.Clone(b.gone), b.Foes...)
}

func (b *Battle) rewards() Gains {
	var g Gains
	for _, u := range b.beaten() {
		if u.Fled {
			continue
		}
		g.Exp += u.Def.Exp
		g.Gold += u.Def.Gold
		for _, d := range u.Def.Drops {
			if d.When.Set() {
				if ok, err := d.When.Bool(b.Env(u)); err != nil || !ok {
					continue
				}
			}
			if b.Rand.Float64() < d.Chance {
				g.Items = append(g.Items, d.Item)
			}
		}
	}
	return g
}
