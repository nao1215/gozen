// Package battle runs a battle: the turns, the actions of the party and the foes, the
// effects of skills and items, and the rules (AI) that choose the foes' actions. It draws
// nothing: each action comes out as a list of Steps (a message, a hit, a state put on)
// which the screen plays one after the other, and the same battle runs headless in the
// tests and in `gozen sim`.
package battle

import (
	"math"
	"slices"
	"strings"

	"github.com/nao1215/gozen/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/rpg"
)

// Unit is a battler: a party member or a foe.
type Unit struct {
	Name  string
	Enemy bool
	Index int // its place on its side
	Actor *rpg.Actor
	EID   string // the enemy id
	Def   *data.Enemy

	HP, MP int
	base   map[string]int
	States map[string]int // turns left (stateForever when it lasts until cured)
	Guard  bool
	Fled   bool
	Auto   bool // a party member acting on their own rules

	fellWith map[string]int // the states it had when it fell (see Drop.When)

	fired   map[int]bool // the rules with once that have been used
	b       *Battle
	tactics *tacticRules // 戦闘開始時の作戦と習得技。nil は従来の AI。

	endured map[string]int // 状態の endure で 耐えた 回数（状態ごと。この 戦いの 間）
	again   int            // 状態の again で 続けて 攻撃して いる 回数
}

const stateForever = rpg.StateForever

func newActorUnit(b *Battle, a *rpg.Actor, i int) *Unit {
	u := &Unit{Name: a.Name(), Index: i, Actor: a, HP: a.HP, MP: a.MP, base: map[string]int{}, States: map[string]int{}, fired: map[int]bool{}, b: b}
	for _, s := range data.StatNames {
		u.base[s] = a.Stat(s)
	}
	for s, n := range a.States {
		if n <= 0 { // 古いセーブの 0 は期限なしとして読む
			n = stateForever
		}
		u.States[s] = n
	}
	u.tactics = snapshotTactics(a)
	return u
}

func newEnemyUnit(b *Battle, id string, i int) *Unit {
	d := b.DB.Enemies[id]
	u := &Unit{Name: d.Name, Enemy: true, Index: i, EID: id, Def: d, base: map[string]int{}, States: map[string]int{}, fired: map[int]bool{}, b: b}
	for _, s := range data.StatNames {
		u.base[s] = d.Stats[s]
		if r, ok := b.DB.Game.Battle.FoeScale[s]; ok && !d.Boss && b.common { // the common foes' tension
			u.base[s] = int(math.Round(float64(d.Stats[s]) * r))
		}
		if b.scale > 1 && b.Troop.LevelScale.Scales(s) { // 想定より強いパーティへの上限つきの補正（level_scale）
			u.base[s] = int(math.Round(float64(u.base[s]) * b.scale))
		}
	}
	u.HP, u.MP = u.base["mhp"], u.base["mmp"]
	return u
}

// Stat is the stat with the states' multipliers.
func (u *Unit) Stat(stat string) int {
	v := float64(u.base[stat])
	for s, n := range u.States {
		if n <= 0 { // 一覧に出ない状態は能力にも効かせない
			continue
		}
		if st := u.b.DB.States[s]; st != nil {
			if r, ok := st.Stats[stat]; ok {
				v *= r
			}
			if e, ok := st.StatsBy[stat]; ok && e.Set() {
				if r, err := e.Number(expr.Chain{expr.Map{"a": u}, u.b.St.Env()}); err == nil {
					v *= r
				}
			}
		}
	}
	return max(0, int(math.Round(v)))
}

// MHP is the unit's max HP.
func (u *Unit) MHP() int { return max(1, u.Stat("mhp")) }

// MMP is the unit's max MP.
func (u *Unit) MMP() int { return u.Stat("mmp") }

// Alive reports whether the unit stands (and has not run away).
func (u *Unit) Alive() bool { return u.HP > 0 && !u.Fled }

// Level is the actor's level, or the enemy's.
func (u *Unit) Level() int {
	if u.Actor != nil {
		return u.Actor.Level
	}
	return u.Def.Level
}

// ID is the actor or the enemy id.
func (u *Unit) ID() string {
	if u.Actor != nil {
		return u.Actor.ID
	}
	return u.EID
}

// Restrict is the strongest restriction of the unit's states: "", skip or confuse.
func (u *Unit) Restrict() (string, string) {
	r, id := "", ""
	for _, s := range u.sortedStates() {
		st := u.b.DB.States[s]
		if st.RestrictChance > 0 { // 時々 だけ 止める 状態は 行動の とき（act）に 引く
			continue
		}
		switch st.Restrict {
		case "skip":
			return "skip", s
		case "confuse":
			r, id = "confuse", s
		}
	}
	return r, id
}

func (u *Unit) sortedStates() []string {
	var out []string
	for _, s := range u.b.DB.Ordered("states") {
		if u.States[s] > 0 {
			out = append(out, s)
		}
	}
	return out
}

// StateList is the unit's states in the order the data defines them.
func (u *Unit) StateList() []string { return u.sortedStates() }

// Face is the picture an enemy drawn as a person shows now (its face rules), or "".
func (u *Unit) Face() string {
	if u.Def == nil || len(u.Def.FaceRule) == 0 {
		return ""
	}
	env := expr.Chain{expr.Map{"self": u}, u.b.Env(u)}
	for _, r := range u.Def.FaceRule {
		if ok, err := r.When.Bool(env); err == nil && ok {
			return r.Face
		}
	}
	return ""
}

// allies are the unit's side; foes the other.
func (u *Unit) allies() []*Unit {
	if u.Enemy {
		return u.b.Foes
	}
	return u.b.Party
}

func (u *Unit) foes() []*Unit {
	if u.Enemy {
		return u.b.Party
	}
	return u.b.Foes
}

// Field implements expr.Object: the unit as expressions see it.
func (u *Unit) Field(name string) (any, bool) {
	switch name {
	case "name":
		return u.Name, true
	case "id":
		return u.ID(), true
	case "hp":
		return float64(u.HP), true
	case "mp":
		return float64(u.MP), true
	case "hp_rate":
		return float64(u.HP) / float64(u.MHP()), true
	case "mp_rate":
		return float64(u.MP) / float64(max(1, u.MMP())), true
	case "alive":
		return u.Alive(), true
	case "dead":
		return !u.Alive(), true
	case "level":
		return float64(u.Level()), true
	case "is_enemy":
		return u.Enemy, true
	case "guarding":
		return u.Guard, true
	case "index":
		return float64(u.Index), true
	case "boss":
		return u.Def != nil && u.Def.Boss, true
	case "states":
		return stateView(u.States), true
	case "fell_with": // the states it had when it fell
		return stateView(u.fellWith), true
	case "state_count":
		return float64(len(u.sortedStates())), true
	case "element":
		return u.weaponElement(), true
	case "class": // a party member's class ("" for a foe)
		if u.Actor != nil {
			return u.Actor.Def().Class, true
		}
		return "", true
	case "back": // stands in the back row (a party member placed there)
		return u.Back(), true
	case "wears": // the items a party member has on (a foe wears nothing)
		return rpg.WearsView{A: u.Actor}, true
	case "lv": // the level of the skill being used (its node in the skill tree; 1 otherwise)
		if u.Actor != nil && u.b.using != "" {
			return float64(max(1, u.Actor.SkillLevel(u.b.using))), true
		}
		return 1.0, true
	}
	if slices.Contains(data.StatNames, name) {
		return float64(u.Stat(name)), true
	}
	if el, ok := strings.CutPrefix(name, "resist_"); ok { // how hard an element hits it (1: as usual)
		if u.Def != nil {
			if r, ok := u.Def.Resist[el]; ok {
				return r, true
			}
		}
		return 1.0, true
	}
	return nil, false
}

// stateResist is how much more (or less) likely state s is to take the unit: its own
// state_resist (a foe), the gear it has on and the states it is in, multiplied.
func (u *Unit) stateResist(s string) float64 {
	r := 1.0
	if u.Def != nil {
		if v, ok := u.Def.SResist[s]; ok {
			r *= v
		}
	}
	if u.Actor != nil {
		for _, it := range u.Actor.Equip {
			if d := u.b.DB.Items[it]; d != nil {
				if v, ok := d.StateResist[s]; ok {
					r *= v
				}
			}
		}
	}
	for id, n := range u.States {
		if n <= 0 {
			continue
		}
		if d := u.b.DB.States[id]; d != nil {
			if v, ok := d.StateResist[s]; ok {
				r *= v
			}
		}
	}
	return r
}

type stateView map[string]int

func (s stateView) Field(name string) (any, bool) { return float64(s[name]), true }

// weaponElement is the element of a party member's weapon ("" for none).
func (u *Unit) weaponElement() string {
	if u.Actor == nil {
		return ""
	}
	for _, it := range u.Actor.Equip {
		if d := u.b.DB.Items[it]; d != nil && d.Element != "" {
			return d.Element
		}
	}
	return ""
}

// weaponStates are the states a party member's weapon may inflict.
func (u *Unit) weaponStates() []data.Grant {
	if u.Actor == nil {
		return nil
	}
	var out []data.Grant
	for _, s := range u.b.DB.Game.Slots {
		if d := u.b.DB.Items[u.Actor.Equip[s.ID]]; d != nil {
			out = append(out, d.States...)
		}
	}
	return out
}

func group(us []*Unit) *rpg.Group {
	ms := make([]expr.Object, len(us))
	for i, u := range us {
		ms[i] = u
	}
	return rpg.NewGroup(ms)
}

func living(us []*Unit) []*Unit {
	var out []*Unit
	for _, u := range us {
		if u.Alive() {
			out = append(out, u)
		}
	}
	return out
}

func fallen(us []*Unit) []*Unit {
	var out []*Unit
	for _, u := range us {
		if !u.Alive() && !u.Fled {
			out = append(out, u)
		}
	}
	return out
}

// Back reports whether the unit stands in the back row.
func (u *Unit) Back() bool { return u.Actor != nil && u.Actor.Back }

// frontFirst weighs a pool of targets towards the front row: by the weights of game.yaml
// battle (front_weight, back_weight; by default a foe aims at someone in the back row a
// quarter as often).
func (b *Battle) frontFirst(pool []*Unit) *Unit {
	weight := func(c *Unit) float64 {
		p := b.DB.Game.Battle.FrontWeight
		if c.Back() {
			p = b.DB.Game.Battle.BackWeight
		}
		if p == nil {
			if c.Back() {
				return 1
			}
			return 4
		}
		return max(0, *p) // 負の重みは 0 とみなす
	}
	w := 0.0
	for _, c := range pool {
		w += weight(c)
	}
	if w <= 0 {
		return pool[b.Rand.IntN(len(pool))]
	}
	r := b.Rand.Float64() * w
	for _, c := range pool {
		r -= weight(c)
		if r < 0 {
			return c
		}
	}
	return pool[len(pool)-1]
}
