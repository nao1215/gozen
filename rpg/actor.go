// Package rpg is the state of a game in progress: the actors and their growth and
// equipment, the party, the bag, the money, the flags and variables the events set, and
// where the party is. It knows the rules that hold outside battle (levels, equipment,
// using an item from the menu) and is saved as JSON.
package rpg

import (
	"maps"
	"math"
	"slices"

	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/layer1/safeint"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// StateForever は期限なし（治すまで続く）の状態の残りターン。戦闘中もイベントの affect もこの値で持つ。
const StateForever = 999

// Actor is a character who has joined (or may join) the party, with their progress.
type Actor struct {
	ID     string            `json:"id"`
	Level  int               `json:"level"`
	Exp    int               `json:"exp"`
	HP     int               `json:"hp"`
	MP     int               `json:"mp"`
	Equip  map[string]string `json:"equip"`
	Learnt []string          `json:"learnt"` // skills taught by events
	States map[string]int    `json:"states"` // states kept outside battle (turns left)
	SP     int               `json:"sp"`     // skill points not spent
	Tree   map[string]int    `json:"tree"`   // the level of each node of the skill tree
	Back   bool              `json:"back"`   // stands in the back row
	// Tactics は確定済みの作戦だけ。編集画面は TacticsDraft の控えを使う。
	Tactics *ActorTactics `json:"tactics,omitempty"`

	db *data.DB
}

// NewActor is the actor id at the level the data gives, fully healed, with their
// starting equipment.
func NewActor(db *data.DB, id string) *Actor {
	d := db.Actors[id]
	a := &Actor{ID: id, Level: d.Level, Back: d.Back, Equip: map[string]string{}, States: map[string]int{}, Tactics: &ActorTactics{Version: TacticsVersion, Mode: TacticsAuto}, db: db}
	for s, it := range d.Equip {
		a.Equip[s] = it
	}
	a.Exp = a.ExpFor(a.Level)
	a.Tree = map[string]int{}
	if c := a.Class(); c != nil {
		a.SP = c.StartSP + c.SP*(a.Level-1)
		for _, n := range c.Tree { // the nodes raised from the start, in the tree's order
			for range d.Tree[n.ID] {
				a.Raise(n.ID)
			}
		}
	}
	a.HP, a.MP = a.Stat("mhp"), a.Stat("mmp")
	return a
}

func (a *Actor) bind(db *data.DB) {
	a.db = db
	if a.Equip == nil {
		a.Equip = map[string]string{}
	}
	// データから消えた装備・合わない枠の装備は外す（EquipBest などが nil を引かないように）。
	for slot, it := range a.Equip {
		if d := db.Items[it]; it == "" || d == nil || !slices.ContainsFunc(db.Game.Slots, func(s data.Slot) bool { return s.ID == slot }) || !d.Fits(slot) {
			delete(a.Equip, slot)
		}
	}
	// データから消えた・名前の変わったスキルと状態も捨てる（技能の画面が nil を引かないように）。
	a.Learnt = slices.DeleteFunc(a.Learnt, func(s string) bool { return db.Skills[s] == nil })
	if a.States == nil {
		a.States = map[string]int{}
	}
	for s := range a.States {
		if db.States[s] == nil {
			delete(a.States, s)
		}
	}
	if a.Tree == nil {
		a.Tree = map[string]int{}
	}
	a.fitTree()
}

// fitTree は古いセーブのスキルの木を今のデータに合わせる。消えたノードの Lv と、
// 上限を下げたノードの上限を超えた分は SP に戻す（説明と実際の強さが食い違わないように）。
func (a *Actor) fitTree() {
	if a.Def() == nil || a.Class() == nil {
		return
	}
	for id, lv := range a.Tree {
		n := a.Node(id)
		switch {
		case lv <= 0:
			delete(a.Tree, id)
		case n == nil:
			a.SP += lv
			delete(a.Tree, id)
		case lv > n.Max:
			a.SP += lv - n.Max
			a.Tree[id] = n.Max
		}
	}
}

// Def is the actor's data.
func (a *Actor) Def() *data.Actor { return a.db.Actors[a.ID] }

// Class is the actor's class.
func (a *Actor) Class() *data.Class { return a.db.Classes[a.Def().Class] }

// Name is the actor's name.
func (a *Actor) Name() string { return a.Def().Name }

// levelEnv is the scope of a class's formulas.
func levelEnv(level int) expr.Env { return expr.Map{"level": float64(level)} }

// Base is the stat from the actor's class at their level, without equipment.
func (a *Actor) Base(stat string) int {
	return BaseAt(a.Class(), stat, a.Level)
}

// BaseAt is a class's stat at a level.
func BaseAt(c *data.Class, stat string, level int) int {
	v, err := c.Stats[stat].Number(levelEnv(level))
	if err != nil {
		return 0
	}
	return max(0, int(math.Floor(v)))
}

// Stat is the stat with the equipment's bonuses.
func (a *Actor) Stat(stat string) int {
	v := a.Base(stat)
	for _, it := range a.Equip {
		if item := a.db.Items[it]; item != nil {
			v += item.Stats[stat]
		}
	}
	for _, n := range a.Class().Tree { // the passive nodes, a level at a time
		v += n.Stats[stat] * a.Tree[n.ID]
	}
	for _, set := range a.Sets() {
		v += a.db.StoneSets[set].Stats[stat]
	}
	if stat == "mhp" {
		return max(1, v)
	}
	return max(0, v)
}

// StatWith is the stat if item were put in its slot (for the equip menu's comparison).
func (a *Actor) StatWith(stat, slot, item string) int {
	old := a.Equip[slot]
	if item == "" {
		delete(a.Equip, slot)
	} else {
		a.Equip[slot] = item
	}
	v := a.Stat(stat)
	if old == "" {
		delete(a.Equip, slot)
	} else {
		a.Equip[slot] = old
	}
	return v
}

// ExpFor is the total exp needed to reach a level.
func (a *Actor) ExpFor(level int) int {
	if level <= 1 {
		return 0
	}
	v, err := a.Class().ExpFor.Number(levelEnv(level))
	if err != nil {
		return math.MaxInt32
	}
	return safeint.From(v)
}

// ExpNext is the exp still needed for the next level (0 at the top).
func (a *Actor) ExpNext() int {
	if a.Level >= a.Class().Max {
		return 0
	}
	return max(0, a.ExpFor(a.Level+1)-a.Exp)
}

// Alive reports whether the actor can fight.
func (a *Actor) Alive() bool { return a.HP > 0 }

// StateResist is how much more (or less) likely state s is to take the actor outside
// battle: the state_resist of the gear they have on and of the states they are in,
// multiplied (0: never). The same as in battle, without a foe's own resistance. The
// factors are multiplied in slot order and then state ID order, so the result is the
// same on every run.
func (a *Actor) StateResist(s string) float64 {
	r := 1.0
	for _, slot := range a.db.Game.Slots {
		if d := a.db.Items[a.Equip[slot.ID]]; d != nil {
			if v, ok := d.StateResist[s]; ok {
				r *= v
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(a.States)) {
		if d, n := a.db.States[id], a.States[id]; d != nil && n > 0 {
			if v, ok := d.StateResist[s]; ok {
				r *= v
			}
		}
	}
	return r
}

// Skills are the skills the actor knows: the class's up to their level, then those taught.
func (a *Actor) Skills() []string {
	out := a.OwnSkills()
	for _, set := range a.Sets() {
		if sk := a.db.StoneSets[set].Skill; sk != "" && !slices.Contains(out, sk) {
			out = append(out, sk)
		}
	}
	return out
}

// OwnSkills are the skills the actor knows without the stone sets.
func (a *Actor) OwnSkills() []string {
	var out []string
	for _, l := range a.Class().Skills {
		if l.Level <= a.Level && !slices.Contains(out, l.Skill) {
			out = append(out, l.Skill)
		}
	}
	for _, s := range a.Learnt {
		if a.db.Skills[s] != nil && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	for _, n := range a.Class().Tree {
		if n.Skill != "" && a.Tree[n.ID] > 0 && !slices.Contains(out, n.Skill) {
			out = append(out, n.Skill)
		}
	}
	return out
}

// SkillLevel is the level of a skill: its node's in the skill tree, 1 for a skill known
// otherwise (0 for one not known).
func (a *Actor) SkillLevel(skill string) int {
	for _, n := range a.Class().Tree {
		if n.Skill == skill && a.Tree[n.ID] > 0 {
			return a.Tree[n.ID]
		}
	}
	if slices.Contains(a.Skills(), skill) {
		return 1
	}
	return 0
}

// Sets はすべての石を着けたセットをデータの順で返す。同じ系列は最初に成立した一つだけ。
// 専用のセットは対象の仲間だけに成り立つ。
func (a *Actor) Sets() []string {
	worn := make([]string, 0, len(a.Equip))
	for _, it := range a.Equip {
		worn = append(worn, it)
	}
	return SetsOf(a.db, a.ID, worn)
}

// SetsOf are the stone sets that the stones make on the actor (see Actor.Sets).
func SetsOf(db *data.DB, actor string, stones []string) []string {
	var out []string
	families := map[string]bool{}
	for _, id := range db.Order["stone_sets"] {
		set := db.StoneSets[id]
		key := setFamilyKey(set)
		if families[key] || len(set.Stones) == 0 || (set.Only && !slices.Contains(set.For, actor)) {
			continue
		}
		worn := map[string]int{}
		for _, it := range stones {
			worn[it]++
		}
		all := true
		for _, st := range set.Stones {
			if worn[st] == 0 {
				all = false
				break
			}
			worn[st]--
		}
		if all {
			out = append(out, id)
			families[key] = true
		}
	}
	return out
}

// NodeOpen reports whether a node of the skill tree can be raised: its requirements met,
// below its top, a point to spend.
func (a *Actor) NodeOpen(id string) bool {
	n := a.Node(id)
	if n == nil || a.Tree[id] >= n.Max || a.SP <= 0 {
		return false
	}
	return a.NodeReached(id)
}

// NodeReached reports whether a node's requirements are met.
func (a *Actor) NodeReached(id string) bool {
	n := a.Node(id)
	if n == nil {
		return false
	}
	for _, r := range n.Requires {
		if a.Tree[r.Node] < r.Level {
			return false
		}
	}
	return true
}

// Node is a node of the actor's skill tree (nil when none).
func (a *Actor) Node(id string) *data.TreeNode {
	for i, n := range a.Class().Tree {
		if n.ID == id {
			return &a.Class().Tree[i]
		}
	}
	return nil
}

// Raise spends a point on a node; it reports whether it could.
func (a *Actor) Raise(id string) bool {
	if !a.NodeOpen(id) {
		return false
	}
	before := a.Stat("mhp")
	a.Tree[id]++
	a.SP--
	if a.HP > 0 {
		a.HP += a.Stat("mhp") - before
	}
	return true
}

// LevelUp is a level gained: the new level, the stats before and after and the skills learned.
type LevelUp struct {
	Level  int
	Before map[string]int
	After  map[string]int
	Skills []string
}

// ResetTree gives back every skill point put into the skill tree (the HP above the new
// maximum is cut).
func (a *Actor) ResetTree() {
	for id, n := range a.Tree {
		a.SP += n
		delete(a.Tree, id)
	}
	a.Clamp()
}

// SpendSP spends the skill points as a plain player would: each point on the open node of
// the lowest level (so the tree grows evenly and the deeper nodes open). The nodes raised,
// in order. PlanSP is the same without spending.
func (a *Actor) SpendSP() []string { return a.ApplySP(a.PlanSP()) }

// spend spends the points as SpendSP says (PlanSP runs it on a copy).
func (a *Actor) spend() []string {
	var raised []string
	for a.SP > 0 {
		best := ""
		for _, n := range a.Class().Tree {
			if a.NodeOpen(n.ID) && (best == "" || a.Tree[n.ID] < a.Tree[best]) {
				best = n.ID
			}
		}
		if best == "" || !a.Raise(best) {
			break
		}
		raised = append(raised, best)
	}
	return raised
}

// GainExp adds exp and raises the level as far as it reaches. HP and MP rise by as much as
// their maximum.
func (a *Actor) GainExp(n int) []LevelUp {
	a.Exp += n
	var ups []LevelUp
	for a.Level < a.Class().Max && a.Exp >= a.ExpFor(a.Level+1) {
		before := map[string]int{}
		for _, s := range data.StatNames {
			before[s] = a.Stat(s)
		}
		known := a.Skills()
		a.Level++
		a.SP += a.Class().SP
		up := LevelUp{Level: a.Level, Before: before, After: map[string]int{}}
		for _, s := range data.StatNames {
			up.After[s] = a.Stat(s)
		}
		for _, s := range a.Skills() {
			if !slices.Contains(known, s) {
				up.Skills = append(up.Skills, s)
			}
		}
		if a.HP > 0 {
			a.HP += up.After["mhp"] - before["mhp"]
		}
		a.MP += up.After["mmp"] - before["mmp"]
		ups = append(ups, up)
	}
	return ups
}

// Clamp keeps HP and MP within their maximum (after a change of equipment).
func (a *Actor) Clamp() {
	a.HP = min(a.HP, a.Stat("mhp"))
	a.MP = min(a.MP, a.Stat("mmp"))
}

// Recover fully heals the actor and cures every state.
func (a *Actor) Recover() {
	a.HP, a.MP = a.Stat("mhp"), a.Stat("mmp")
	clear(a.States)
}

// CanEquip reports whether the actor may equip the item (its kind is in the class's list).
func (a *Actor) CanEquip(item string) bool {
	it := a.db.Items[item]
	if it == nil || !it.Equipment() {
		return false
	}
	eq := a.Class().Equip
	return len(eq) == 0 || slices.Contains(eq, it.Kind)
}

// Face is the face the actor shows now: the first of their face rules that holds.
func (a *Actor) Face() string {
	d := a.Def()
	env := expr.Map{"self": ActorView{a}}
	for _, r := range d.FaceRule {
		if ok, err := r.When.Bool(env); err == nil && ok {
			return r.Face
		}
	}
	return d.Face
}

// ActorView is an actor as expressions see them.
type ActorView struct{ A *Actor }

// Field implements expr.Object.
func (v ActorView) Field(name string) (any, bool) {
	a := v.A
	switch name {
	case "id":
		return a.ID, true
	case "name":
		return a.Name(), true
	case "level":
		return float64(a.Level), true
	case "exp":
		return float64(a.Exp), true
	case "hp":
		return float64(a.HP), true
	case "mp":
		return float64(a.MP), true
	case "hp_rate":
		return float64(a.HP) / float64(max(1, a.Stat("mhp"))), true
	case "mp_rate":
		return float64(a.MP) / float64(max(1, a.Stat("mmp"))), true
	case "alive":
		return a.Alive(), true
	case "dead":
		return !a.Alive(), true
	case "class":
		return a.Def().Class, true
	case "equip":
		m := expr.Map{}
		for s, it := range a.Equip {
			m[s] = it
		}
		return equipView(m), true
	case "states":
		return statesView(a.States), true
	case "back":
		return a.Back, true
	case "sp":
		return float64(a.SP), true
	case "wears":
		return WearsView{a}, true
	}
	if slices.Contains(data.StatNames, name) {
		return float64(a.Stat(name)), true
	}
	return nil, false
}

// WearsView tells whether an actor has an item on (wears.<item>: true or false).
type WearsView struct{ A *Actor }

// Field implements expr.Object.
func (w WearsView) Field(name string) (any, bool) {
	if w.A == nil {
		return false, true
	}
	for _, it := range w.A.Equip {
		if it == name {
			return true, true
		}
	}
	return false, true
}

// equipView is the items in an actor's slots ("" for an empty slot).
type equipView expr.Map

func (e equipView) Field(name string) (any, bool) {
	if v, ok := e[name]; ok {
		return v, true
	}
	return "", true
}

// statesView is the turns left of each state (0, which is false, when the state is not on).
type statesView map[string]int

func (s statesView) Field(name string) (any, bool) { return float64(s[name]), true }
