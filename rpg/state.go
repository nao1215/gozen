package rpg

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/nao1215/gozen/expr"
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
	Vars     map[string]any    `json:"vars"` // 数値（float64）・文字列・宣言した真偽値
	Location string            `json:"location"`
	Steps    int               `json:"steps"`  // moves made
	Frames   int64             `json:"frames"` // play time in ticks
	Done     map[string]bool   `json:"done"`   // events with once that have run
	Pos      Pos               `json:"pos"`    // where the party is in a dungeon
	Mapped   map[string][]byte `json:"mapped"` // the cells of each dungeon seen (1 a cell)
	// Put は イベントで 変えた 迷宮の マス（open_wall で 壁に 開けた 階段など）。キーは 階と 座標、値は タイルの 文字。
	Put map[string]string `json:"put,omitempty"`
	// Fighters fight the battle being started instead of the party (a duel); not saved.
	Fighters []string `json:"-"`
	// Roamers are the strong foes walking the floor the party is on (a made dungeon).
	Roamers []Roamer `json:"roamers,omitempty"`

	DB   *data.DB   `json:"-"`
	Rand *rand.Rand `json:"-"`
	// Calm turns the random battles off (test play).
	Calm bool `json:"-"`
	// Bumped is the last walking into a tile that set it off (internal/layer3/dungeon): the same
	// one is not set off again until the party moves. Not saved.
	Bumped string `json:"-"`
	// LoadReport は読み込み時の移行・補正の一覧。保存するゲーム状態には含めない。
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
	a.Clamp()
	return nil
}

// Flag is a flag (false when never set).
func (s *State) Flag(name string) bool { return s.Flags[name] }

// SetPath sets flag.<name> or var.<name> to v.
func (s *State) SetPath(path string, v any) error {
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
// location, steps, members, rand, chance, pos (x, y, facing in a dungeon), pages.
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
		return s.PartyGroup(), true
	case "members":
		return float64(len(s.Party)), true
	case "item":
		return itemView{s}, true
	case "actor":
		return actorsView{s}, true
	case "location":
		return s.Location, true
	case "steps":
		return float64(s.Steps), true
	case "pos":
		return expr.Map{"x": float64(s.Pos.X), "y": float64(s.Pos.Y), "facing": data.Facings[s.Pos.Facing&3]}, true
	case "rand":
		return RandFunc(s.Rand), true
	case "chance":
		return ChanceFunc(s.Rand), true
	case "pages":
		return PagesFunc(s.DB), true
	}
	return nil, false
}

// PagesFunc is pages(name): how many pictures name has under assets/ (data.CountPages).
func PagesFunc(db *data.DB) expr.Func {
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

// PartyGroup is the party as a group (see Group).
func (s *State) PartyGroup() *Group {
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

// RandFunc is rand(): a number in [0, 1); rand(n): an integer in [0, n); rand(a, b): an
// integer in [a, b]. A count above 2^53 is treated as 2^53, and a NaN argument is an
// error rather than a panic.
func RandFunc(r *rand.Rand) expr.Func {
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

// ChanceFunc is chance(p): true with probability p.
func ChanceFunc(r *rand.Rand) expr.Func {
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

// Upgrades は袋の中で、今の装備より強い物（能力の合計で比べる）を枠ごとに返す（枠 → 品）。
// manual の枠（組み合わせで効く石など）は見ない。同じ品が袋に 1 つしかなければ、先の枠に 1 回だけ。
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

// EquipUpgrades は Upgrades の品を着ける（メニューの「さいきょうそうび」）。着けた数を返す。
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
	// Calm は追わない歩数の終わり（State.Steps）。逃げられた強敵は、しばらく追わずに見送る。
	Calm int `json:"calm,omitempty"`
	// Walk は 動いた 回数（遊び手の 一歩ごとに 1。every の 歩数ごとに 1 マス 動く 者の 数え）。
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
		a.Clamp()
	}
}
