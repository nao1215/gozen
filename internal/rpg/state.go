package rpg

import (
	"errors"
	"fmt"
	"maps"
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
	// Chose is the option last taken at each choice with an id (chose.<id>; ADR 0025).
	Chose map[string]string `json:"chose,omitempty"`
	// RecruitSeed and RecruitDraws keep capture and birth draws separate from
	// battle, encounter and story randomness. Both are saved for replay.
	RecruitSeed  uint64 `json:"recruit_seed,omitempty"`
	RecruitDraws uint64 `json:"recruit_draws,omitempty"`

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
	// Resume is the cursor of a save made in the middle of an event (nil: none). Set by Parse.
	Resume *Resume `json:"-"`
	// SavedRand is the random generator's bytes of a save made between events (nil: none
	// were saved); a loader goes on with them, so a restored game draws the same numbers.
	SavedRand []byte `json:"-"`

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
		RecruitSeed: collectionSeed(db.Game.ID),
	}
	s.initStateNames()
	st := db.Game.Start
	for _, id := range st.Party {
		s.Join(id)
	}
	maps.Copy(s.Items, st.Items)
	s.Gold = st.Gold
	s.Location = st.Location
	return s
}

func collectionSeed(id string) uint64 {
	var hash uint64 = 14695981039346656037
	for i := range len(id) {
		hash = (hash ^ uint64(id[i])) * 1099511628211
	}
	return hash&((1<<53)-1) | 1
}

// SetRecruitSeed chooses a collection stream without advancing gameplay RNG.
// It is used at session creation, before any capture or birth draw.
func (s *State) SetRecruitSeed(seed uint64) {
	s.RecruitSeed = seed&((1<<53)-1) | 1
	s.RecruitDraws = 0
}

// RecruitFloat64 draws from the saved monster collection stream. It never
// advances the stream used by encounters, battle or authored expressions.
func (s *State) RecruitFloat64() float64 {
	// Older saves have no collection seed. A stable game-specific default keeps
	// their unrelated random stream unchanged when the first offer appears.
	if s.RecruitSeed == 0 {
		s.RecruitSeed = collectionSeed(s.DB.Game.ID)
	}
	rng := rand.New(rand.NewPCG(s.RecruitSeed, s.RecruitDraws)) //nolint:gosec // Deterministic gameplay randomness.
	s.RecruitDraws++
	return rng.Float64()
}

// Join adds an actor to the party, moving an active monster to the reserve if full.
func (s *State) Join(id string) bool {
	if slices.Contains(s.Party, id) {
		return false
	}
	if len(s.Party) >= s.DB.Game.MaxParty {
		at := -1
		for i, member := range s.Party {
			if a := s.Actors[member]; a != nil && a.Monster != "" {
				at = i
			}
		}
		if at < 0 {
			return false
		}
		s.Party = slices.Delete(s.Party, at, at+1)
	}
	if s.Actors[id] == nil {
		s.Actors[id] = NewActor(s.DB, id)
	}
	s.Party = append(s.Party, id)
	return true
}

// JoinStory ensures an authored event can add its required member. If a full
// party has no active monster, the last active actor moves to the reserve.
func (s *State) JoinStory(id string) bool {
	if s.Join(id) {
		return true
	}
	if slices.Contains(s.Party, id) || len(s.Party) < s.DB.Game.MaxParty || len(s.Party) == 0 {
		return false
	}
	s.Party = s.Party[:len(s.Party)-1]
	return s.Join(id)
}

// Recruit creates a distinct monster actor and places it in the active party if there
// is room. Otherwise it remains in the reserve. The returned ID is save-stable.
func (s *State) Recruit(species string, level, plus int, skills []string) (string, bool) {
	return s.RecruitWithSex(species, level, plus, skills, "")
}

// RecruitWithSex creates a monster with the sex the event runner drew for its
// visible offer. An empty sex draws one here, as Recruit does.
func (s *State) RecruitWithSex(species string, level, plus int, skills []string, sex string) (string, bool) {
	e := s.DB.Enemies[species]
	if e == nil || s.DB.Classes[s.DB.MonsterClass(species)] == nil || sex != "" && !data.AllowsMonsterSex(s.DB, species, sex) {
		return "", false
	}
	seq, ok := s.Vars["monster.next"].(float64)
	if !ok {
		seq = 0
	}
	id := fmt.Sprintf("monster_%d", int(seq)+1)
	for s.Actors[id] != nil || s.DB.Actors[id] != nil {
		seq++
		id = fmt.Sprintf("monster_%d", int(seq)+1)
	}
	s.Vars["monster.next"] = float64(int(seq) + 1)
	s.DB.Actors[id] = &data.Actor{Name: e.Name, Class: s.DB.MonsterClass(species), Level: max(1, level)}
	a := NewActor(s.DB, id)
	a.Monster, a.Plus = species, plus
	if sex == "" {
		sex = data.MonsterSex(s.DB, species, s.RecruitFloat64())
	}
	a.Sex = sex
	a.HP, a.MP = a.Stat("mhp"), a.Stat("mmp")
	for _, skill := range skills {
		if s.DB.Skills[skill] != nil && !slices.Contains(a.Learnt, skill) {
			a.Learnt = append(a.Learnt, skill)
		}
	}
	s.Actors[id] = a
	if len(s.Party) < s.DB.Game.MaxParty {
		s.Party = append(s.Party, id)
	}
	key := "bestiary." + species + ".caught"
	n, _ := expr.ToNumber(s.Vars[key])
	s.Vars[key] = n + 1
	s.Vars["bestiary."+species+".seen"] = true
	return id, true
}

// ReserveIDs lists all joined actors currently outside the active party.
func (s *State) ReserveIDs() []string {
	var ids []string
	for id := range s.Actors {
		if !slices.Contains(s.Party, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// SwapMonster places a reserve actor in an active slot, preserving the party limit.
func (s *State) SwapMonster(in, out string) bool {
	a := s.Actors[in]
	if a == nil || slices.Contains(s.Party, in) {
		return false
	}
	for i, id := range s.Party {
		if id == out {
			s.Party[i] = in
			return true
		}
	}
	return false
}

// ReleaseMonster permanently removes a monster instance from the party or reserve.
func (s *State) ReleaseMonster(id string) bool {
	a := s.Actors[id]
	if a == nil || a.Monster == "" || len(s.Party) == 1 && s.Party[0] == id {
		return false
	}
	s.Party = slices.DeleteFunc(s.Party, func(p string) bool { return p == id })
	delete(s.Actors, id)
	delete(s.DB.Actors, id)
	return true
}

// Breed consumes two owned monsters and creates their child. It checks the full
// recipe and skill selection before changing state.
func (s *State) Breed(first, second string, skills []string) (string, error) {
	a, b := s.Actors[first], s.Actors[second]
	if first == second || a == nil || b == nil || a.Monster == "" || b.Monster == "" {
		return "", errors.New("breed needs two distinct owned monsters")
	}
	if s.DB.Breeding == nil {
		return "", errors.New("breeding is not defined")
	}
	if a.Sex == b.Sex || a.Sex != "male" && a.Sex != "female" || b.Sex != "male" && b.Sex != "female" {
		return "", errors.New("breed needs one male and one female parent")
	}
	child := s.DB.BreedResult(a.Monster, b.Monster, a.Level, b.Level, a.Plus, b.Plus)
	if child == "" {
		return "", fmt.Errorf("no eligible breeding result for %s and %s", a.Monster, b.Monster)
	}
	if len(skills) > s.DB.Breeding.SkillSlots {
		return "", fmt.Errorf("breed allows at most %d inherited skills", s.DB.Breeding.SkillSlots)
	}
	known := append(a.Skills(), b.Skills()...)
	selected := map[string]bool{}
	for _, skill := range skills {
		if !slices.Contains(known, skill) || selected[skill] {
			return "", fmt.Errorf("skill %q is not a distinct parental skill", skill)
		}
		selected[skill] = true
	}
	plus := a.Plus + b.Plus + max(1, (a.Level+b.Level)/20)
	firstSlot := slices.Index(s.Party, first)
	secondSlot := slices.Index(s.Party, second)
	preferredSlot := firstSlot
	if preferredSlot < 0 || secondSlot >= 0 && secondSlot < preferredSlot {
		preferredSlot = secondSlot
	}
	id, ok := s.Recruit(child, 1, plus, skills)
	if !ok {
		return "", fmt.Errorf("child %q has no monster class", child)
	}
	for _, parent := range []string{first, second} {
		s.Party = slices.DeleteFunc(s.Party, func(p string) bool { return p == parent })
		delete(s.Actors, parent)
		delete(s.DB.Actors, parent)
	}
	if preferredSlot >= 0 {
		s.Party = slices.DeleteFunc(s.Party, func(p string) bool { return p == id })
		preferredSlot = min(preferredSlot, len(s.Party))
		s.Party = slices.Insert(s.Party, preferredSlot, id)
	}
	return id, nil
}

// Leave takes the actor out of the party; they keep their progress. The last member never
// leaves (a party is never empty: such a state could not be saved and loaded again), so it
// reports false then. Leaving when the actor is not in the party changes nothing and says true.
func (s *State) Leave(id string) bool {
	if len(s.Party) == 1 && s.Party[0] == id {
		return false
	}
	s.Party = slices.DeleteFunc(s.Party, func(p string) bool { return p == id })
	return true
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
	if done, err := s.setCampaign(path, v); done {
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
		// A variable keeps a number, text, true/false or nothing: a function (set: {var.x: rand}) or an object
		// would make the state impossible to save.
		if expr.TypeOfValue(v) == expr.UnknownType {
			return fmt.Errorf("set %s: a value of type %T cannot be kept in a variable (want a number, text or true/false)", path, v)
		}
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
	if campaignName(name) {
		return data.CampaignEnv(s.DB, s.Campaign()).Lookup(name)
	}
	switch name {
	case "flag":
		return flagView{s}, true
	case "var":
		return varView{s}, true
	case "gold":
		return float64(s.Gold), true
	case "party":
		return s.partyGroup(), true
	case "reserve":
		var ms []expr.Object
		for _, id := range s.ReserveIDs() {
			ms = append(ms, ActorView{s.Actors[id]})
		}
		return NewGroup(ms), true
	case "monster":
		return monsterView{s}, true
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
	case "bestiary":
		return bestiaryView{s}, true
	case "chose":
		return choseView{s}, true
	}
	return nil, false
}

type bestiaryView struct{ s *State }

type monsterView struct{ s *State }

func (v monsterView) Field(id string) (any, bool) {
	if a := v.s.Actors[id]; a != nil && a.Monster != "" {
		return ActorView{a}, true
	}
	if a := v.s.Actors["monster_"+id]; a != nil && a.Monster != "" {
		return ActorView{a}, true
	}
	return nil, false
}

func (v bestiaryView) Field(id string) (any, bool) {
	if v.s.DB.Enemies[id] == nil {
		return nil, false
	}
	return bestiaryEnemyView{s: v.s, id: id}, true
}

type bestiaryEnemyView struct {
	s  *State
	id string
}

func (v bestiaryEnemyView) Field(name string) (any, bool) {
	switch name {
	case "seen":
		return expr.Truthy(v.s.Vars["bestiary."+v.id+".seen"]), true
	case "defeated":
		n, _ := expr.ToNumber(v.s.Vars["bestiary."+v.id+".defeated"])
		return n, true
	case "caught":
		n, _ := expr.ToNumber(v.s.Vars["bestiary."+v.id+".caught"])
		return n, true
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
