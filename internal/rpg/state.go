package rpg

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/safeint"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// State is a game in progress.
type State struct {
	Actors   map[string]*Actor `json:"actors"` // everyone who has joined, by id
	Party    []string          `json:"party"`
	Items    map[string]int    `json:"items"`
	Gold     int               `json:"gold"`
	Flags    map[string]bool   `json:"flags"`
	Vars     map[string]any    `json:"vars"` // numbers (float64), strings and declared booleans
	Location string            `json:"location"`
	// Spot is where the party stands at a map or a stage location: a point of the map or a
	// spot of the stage (ADR 0022). Empty is the location's start.
	Spot   string          `json:"spot,omitempty"`
	Steps  int             `json:"steps"`  // moves made
	Frames int64           `json:"frames"` // play time in ticks
	Done   map[string]bool `json:"done"`   // events with once that have run
	// Ran is how many times each event has run and the order of its last run (ran.<id>).
	Ran    map[string][2]int `json:"ran,omitempty"`
	Pos    Pos               `json:"pos"`    // where the party is in a dungeon
	Mapped map[string][]byte `json:"mapped"` // the cells of each dungeon seen (1 a cell)
	// Put holds dungeon cells changed by events (such as stairs opened into a wall with open_wall). The key is floor and coordinates; the value is the tile character.
	Put map[string]string `json:"put,omitempty"`
	// Fighters fight the battle being started instead of the party (a duel); not saved.
	Fighters []string `json:"-"`
	// Roamers are the strong foes walking the floor the party is on (a made dungeon).
	Roamers []Roamer `json:"roamers,omitempty"`

	DB   *data.DB   `json:"-"`
	Rand *rand.Rand `json:"-"`
	// Calm turns the random battles off (test play).
	Calm bool `json:"-"`
	// Moved is whether the last step forward in a dungeon moved the party (false: it
	// bumped into something). Not saved: the expression name moved.
	Moved bool `json:"-"`
	// Bumped is the last walking into a tile that set it off (internal/layer3/dungeon): the same
	// one is not set off again until the party moves. Not saved.
	Bumped string `json:"-"`
	// LoadReport lists the migrations and corrections made at load time. It is not part of the saved game state.
	LoadReport LoadReport `json:"-"`

	stateNames map[string]data.StateName
}

// Pos is a place in a dungeon's map and the way the party faces (0 north, 1 east, 2
// south, 3 west).
type Pos struct {
	In     string `json:"in"` // the dungeon (location) it is in
	X, Y   int
	Facing int `json:"facing"`
}

// New is the state a new game starts in (game.yaml start).
func New(db *data.DB, rng *rand.Rand) *State {
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) //nolint:gosec // Gameplay randomness; no security-sensitive values.
	}
	s := &State{
		Actors: map[string]*Actor{}, Items: map[string]int{}, Flags: map[string]bool{},
		Vars: map[string]any{}, Done: map[string]bool{}, Mapped: map[string][]byte{}, DB: db, Rand: rng,
	}
	s.initStateNames()
	st := db.Game.Start
	for _, id := range st.Party {
		s.Join(id)
	}
	for it, n := range st.Items {
		s.Items[it] = n
	}
	s.Gold = st.Gold
	s.Location = st.Location
	return s
}

// Join adds the actor to the party (creating them the first time), if there is room.
func (s *State) Join(id string) bool {
	if slices.Contains(s.Party, id) || len(s.Party) >= s.DB.Game.MaxParty {
		return false
	}
	if s.Actors[id] == nil {
		s.Actors[id] = NewActor(s.DB, id)
	}
	s.Party = append(s.Party, id)
	return true
}

// Leave takes the actor out of the party; they keep their progress.
func (s *State) Leave(id string) {
	s.Party = slices.DeleteFunc(s.Party, func(p string) bool { return p == id })
}

// Members are the party's actors in order.
func (s *State) Members() []*Actor {
	out := make([]*Actor, 0, len(s.Party))
	for _, id := range s.Party {
		out = append(out, s.Actors[id])
	}
	return out
}

// Wiped reports whether every member has fallen.
func (s *State) Wiped() bool {
	for _, a := range s.Members() {
		if a.Alive() {
			return false
		}
	}
	return true
}

// AddItem adds n of the item (n < 0 takes them; the count stays at 0 or more).
func (s *State) AddItem(id string, n int) {
	s.Items[id] = max(0, s.Items[id]+n)
	if s.Items[id] == 0 {
		delete(s.Items, id)
	}
}

// Buy pays for n of the item at the shop and puts them in the bag. The shop's spent
// variable (shops.<id>.spent), if it has one, counts what was paid. It buys nothing and
// says false when the money is short.
func (s *State) Buy(shop, id string, n int) bool {
	it := s.DB.Items[id]
	if it == nil || n <= 0 || it.Price < 0 || (it.Price > 0 && n > s.Gold/it.Price) {
		return false
	}
	total := it.Price * n
	if sh := s.DB.Shops[shop]; sh != nil && sh.Spent != "" {
		old, _ := expr.ToNumber(s.Vars[strings.TrimPrefix(sh.Spent, "var.")])
		if err := s.SetPath(sh.Spent, old+float64(total)); err != nil {
			return false
		}
	}
	s.Gold -= total
	s.AddItem(id, n)
	return true
}

// Sell sells n of the item from the bag at the shop for half its price. It sells
// nothing and says false when the shop does not buy, the item has no price or is a
// key, or the bag has fewer.
func (s *State) Sell(shop, id string, n int) bool {
	it, sh := s.DB.Items[id], s.DB.Shops[shop]
	if it == nil || sh == nil || sh.Sell != nil && !*sh.Sell || it.Kind == "key" || it.Price <= 0 || n <= 0 || s.Items[id] < n {
		return false
	}
	s.Gold += it.Price / 2 * n
	s.AddItem(id, -n)
	return true
}

// ItemCount is how many of the item the party has, counting what is equipped.
func (s *State) ItemCount(id string, equipped bool) int {
	n := s.Items[id]
	if equipped {
		for _, a := range s.Members() {
			for _, it := range a.Equip {
				if it == id {
					n++
				}
			}
		}
	}
	return n
}

// Bag is the items in the bag in the order they were defined.
func (s *State) Bag() []string {
	var out []string
	for _, id := range s.DB.Ordered("items") {
		if s.Items[id] > 0 {
			out = append(out, id)
		}
	}
	return out
}

// Equip puts item (from the bag, "" to empty the slot) in the actor's slot; what was there
// goes back to the bag.
func (s *State) Equip(a *Actor, slot, item string) error {
	if item != "" {
		if s.Items[item] <= 0 {
			return fmt.Errorf("no %s in the bag", item)
		}
		if !a.CanEquip(item) || !s.DB.Items[item].Fits(slot) {
			return fmt.Errorf("%s cannot equip %s in %s", a.ID, item, slot)
		}
	}
	if old := a.Equip[slot]; old != "" {
		s.AddItem(old, 1)
		delete(a.Equip, slot)
	}
	if item != "" {
		s.AddItem(item, -1)
		a.Equip[slot] = item
	}
	a.clamp()
	return nil
}

// unequip puts what a wears in slot back in the bag: Equip with an empty item, which
// cannot fail.
func (s *State) unequip(a *Actor, slot string) {
	if old := a.Equip[slot]; old != "" {
		s.AddItem(old, 1)
		delete(a.Equip, slot)
	}
	a.clamp()
}

// Flag is a flag (false when never set).
func (s *State) Flag(name string) bool { return s.Flags[name] }

// SetPath sets flag.<name> or var.<name> to v, and (schema version 2) gold, item.<id>,
// party, location and actor.<id>.<hp|mp|exp|level|sp|tactics>. The caller decides whether
// the stats (hp, mp, exp, level, sp) may be set: only in the development mode.
func (s *State) SetPath(path string, v any) error {
	if done, err := s.setOther(path, v); done {
		return err
	}
	kind, name, ok := strings.Cut(path, ".")
	if !ok || name == "" {
		return fmt.Errorf("cannot set %q", path)
	}
	if err := s.checkStateValue(path, v); err != nil {
		return err
	}
	switch kind {
	case "flag":
		s.Flags[name] = expr.Truthy(v)
	case "var":
		_, declared := s.DB.Game.State.Names[path]
		if expr.TypeOfValue(v) != expr.BoolType || (!declared && !s.DB.Game.State.Strict) {
			if f, ok := expr.ToNumber(v); ok {
				v = f
			}
		}
		s.Vars[name] = v
	default:
		return fmt.Errorf("cannot set %q (only flag.* and var.*)", path)
	}
	return nil
}

// ---- expressions

// Env is the scope of the field's expressions: flag, var, gold, party, item, actor,
// location, spot, steps, members, rand, chance, pos (x, y, facing in a dungeon), pages.
func (s *State) Env() expr.Env { return fieldEnv{s} }

type fieldEnv struct{ s *State }

func (e fieldEnv) Lookup(name string) (any, bool) {
	s := e.s
	switch name {
	case "flag":
		return flagView{s}, true
	case "var":
		return varView{s}, true
	case "gold":
		return float64(s.Gold), true
	case "party":
		return s.partyGroup(), true
	case "members":
		return float64(len(s.Party)), true
	case "item":
		return itemView{s}, true
	case "actor":
		return actorsView{s}, true
	case "location":
		return s.Location, true
	case "spot":
		return s.Spot, true
	case "steps":
		return float64(s.Steps), true
	case "pos":
		return expr.Map{"x": float64(s.Pos.X), "y": float64(s.Pos.Y), "facing": data.Facings[s.Pos.Facing&3]}, true
	case "rand":
		return randFunc(s.Rand), true
	case "chance":
		return chanceFunc(s.Rand), true
	case "pages":
		return pagesFunc(s.DB), true
	case "repairs":
		return s.repairsView(), true
	case "ran":
		return ranView{s}, true
	}
	return nil, false
}

// pagesFunc is pages(name): how many pictures name has under assets/ (data.CountPages).
func pagesFunc(db *data.DB) expr.Func {
	return func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("pages takes 1 argument")
		}
		name, ok := args[0].(string)
		if !ok {
			return nil, errors.New("pages takes a file name or pattern")
		}
		if db == nil {
			return 0.0, nil
		}
		return float64(data.CountPages(db.FS, name)), nil
	}
}

// partyGroup is the party as a group (see Group).
func (s *State) partyGroup() *Group {
	var ms []expr.Object
	for _, a := range s.Members() {
		ms = append(ms, ActorView{a})
	}
	return NewGroup(ms)
}

type flagView struct{ s *State }

func (f flagView) Field(name string) (any, bool) {
	if f.s.DB.Game.State.Strict {
		if _, ok := f.s.stateRegistry()["flag."+name]; !ok {
			return nil, false
		}
	}
	return f.s.Flags[name], true
}

type varView struct{ s *State }

func (v varView) Field(name string) (any, bool) {
	if v.s.DB.Game.State.Strict {
		if _, ok := v.s.stateRegistry()["var."+name]; !ok {
			return nil, false
		}
	}
	if x, ok := v.s.Vars[name]; ok {
		return x, true
	}
	if d, ok := v.s.DB.Game.State.Names["var."+name]; ok {
		return d.Initial, true
	}
	if v.s.DB.Game.State.Strict {
		if d, ok := v.s.stateRegistry()["var."+name]; ok && d.Type == "string" {
			return "", true
		}
	}
	return 0.0, true
}

// itemView is how many of each item the party has (equipped ones included).
type itemView struct{ s *State }

func (v itemView) Field(name string) (any, bool) {
	return float64(v.s.ItemCount(name, true)), true
}

// actorsView is every actor by id (nil for one who never joined); in_party tells whether
// they are in the party now.
type actorsView struct{ s *State }

func (v actorsView) Field(name string) (any, bool) {
	a := v.s.Actors[name]
	if a == nil {
		return nil, true
	}
	return inParty{ActorView{a}, slices.Contains(v.s.Party, name)}, true
}

type inParty struct {
	ActorView
	in bool
}

func (p inParty) Field(name string) (any, bool) {
	if name == "in_party" {
		return p.in, true
	}
	return p.ActorView.Field(name)
}

// randFunc is rand(): a number in [0, 1); rand(n): an integer in [0, n); rand(a, b): an
// integer in [a, b]. A count above 2^53 is treated as 2^53, and a NaN argument is an
// error rather than a panic.
func randFunc(r *rand.Rand) expr.Func {
	return func(args []any) (any, error) {
		nums := make([]float64, len(args))
		for i, v := range args {
			n, _ := expr.ToNumber(v)
			if math.IsNaN(n) {
				return nil, errors.New("rand: an argument is not a number")
			}
			nums[i] = n
		}
		switch len(args) {
		case 0:
			return r.Float64(), nil
		case 1:
			n := safeint.From(nums[0])
			if n < 1 {
				return 0.0, nil
			}
			return float64(r.IntN(n)), nil
		case 2:
			a, b := nums[0], nums[1]
			if b < a {
				return a, nil
			}
			return a + float64(r.IntN(safeint.From(b-a)+1)), nil
		}
		return nil, errors.New("rand takes 0 to 2 arguments")
	}
}

// chanceFunc is chance(p): true with probability p.
func chanceFunc(r *rand.Rand) expr.Func {
	return func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, errors.New("chance takes 1 argument")
		}
		p, _ := expr.ToNumber(args[0])
		return r.Float64() < p, nil
	}
}

// AllActors are everyone who has joined, in the order of the data.
func AllActors(s *State) []*Actor {
	var out []*Actor
	for _, id := range s.DB.Order["actors"] {
		if a := s.Actors[id]; a != nil {
			out = append(out, a)
		}
	}
	return out
}

// EquipBest gives the actor the best equipment in the bag for each slot (by the sum of its
// stats; a stone set the actor would complete counts too).
func (s *State) EquipBest(a *Actor) {
	score := func(id string) int {
		if id == "" {
			return 0
		}
		n := 0
		for _, v := range s.DB.Items[id].Stats {
			n += v
		}
		return n
	}
	for _, sl := range s.DB.Game.Slots {
		best := a.Equip[sl.ID]
		for _, id := range s.Bag() {
			if s.DB.Items[id].Fits(sl.ID) && a.CanEquip(id) && score(id) > score(best) {
				best = id
			}
		}
		if best != a.Equip[sl.ID] {
			if err := s.Equip(a, sl.ID, best); err != nil {
				continue
			}
		}
	}
}

// Upgrades returns, per slot, the items in the bag stronger than the current equipment (compared by total stats) (slot -> item).
// Manual slots (such as stones that work in combination) are not considered. If the bag holds only one of the same item, it goes to the earlier slot only once.
func (s *State) Upgrades(a *Actor) map[string]string {
	score := func(id string) int {
		it := s.DB.Items[id]
		if it == nil {
			return 0
		}
		n := 0
		for _, v := range it.Stats {
			n += v
		}
		return n
	}
	out := map[string]string{}
	used := map[string]int{}
	for _, sl := range s.DB.Game.Slots {
		if sl.Manual {
			continue
		}
		cur := a.Equip[sl.ID]
		best, bs := "", score(cur)
		for _, id := range s.Bag() {
			if used[id] >= s.Items[id] {
				continue
			}
			if s.DB.Items[id].Fits(sl.ID) && a.CanEquip(id) && score(id) > bs {
				best, bs = id, score(id)
			}
		}
		if best != "" {
			out[sl.ID] = best
			used[best]++
		}
	}
	return out
}

// EquipUpgrades equips the items from Upgrades (the menu's "strongest equipment"). It returns the number equipped.
func (s *State) EquipUpgrades(a *Actor) int {
	n := 0
	for _, sl := range s.DB.Game.Slots {
		if id, ok := s.Upgrades(a)[sl.ID]; ok && s.Equip(a, sl.ID, id) == nil {
			n++
		}
	}
	return n
}

// Roamer is a strong foe walking a made floor: its kind (an index of the dungeon's
// roamers), its battle and where it is.
type Roamer struct {
	Kind  int    `json:"kind"`
	Troop string `json:"troop"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	// Calm is the end of the no-pursuit step count (State.Steps). A strong enemy that was escaped from lets the player go for a while.
	Calm int `json:"calm,omitempty"`
	// Walk is the number of moves (1 per player step; counts those that move one cell per every steps).
	Walk int `json:"walk,omitempty"`
}

// Take takes n of an item: from the bag first, then off whoever wears it.
func (s *State) Take(id string, n int) {
	if b := min(n, s.Items[id]); b > 0 {
		s.AddItem(id, -b)
		n -= b
	}
	for _, aid := range s.Party {
		a := s.Actors[aid]
		for slot, it := range a.Equip {
			if n > 0 && it == id {
				delete(a.Equip, slot)
				n--
			}
		}
		a.clamp()
	}
}
