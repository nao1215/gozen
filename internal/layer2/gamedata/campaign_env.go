package gamedata

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
)

// CampaignState is what a campaign's names read of a play: its variables (by name,
// without var.) and the actors who have joined. The zero value is the start of a game.
type CampaignState struct {
	Vars      map[string]any
	JoinedIDs []string
}

// Var is a variable of the play.
func (s CampaignState) Var(name string) (any, bool) {
	v, ok := s.Vars[name]
	return v, ok
}

// Joined reports whether the actor has joined.
func (s CampaignState) Joined(id string) bool { return slices.Contains(s.JoinedIDs, id) }

// CampaignEnv is the campaign's names on a play: region, faction, campaign, generals,
// general, target and next_to (ADR 0030).
func CampaignEnv(db *DB, st CampaignState) expr.Env { return campaignEnv{db, st} }

type campaignEnv struct {
	db *DB
	st CampaignState
}

func (e campaignEnv) Lookup(name string) (any, bool) {
	switch name {
	case "region":
		return regionsView(e), true
	case "faction":
		return factionsView(e), true
	case "campaign":
		return e.campaign(), true
	case "generals":
		return generalsView(e), true
	case "general":
		if id := e.str(VarGeneral); id != "" {
			return e.general(id), true
		}
		return nil, true
	case "target":
		if id := e.str(VarTarget); id != "" && e.db.Regions[id] != nil {
			return e.region(id), true
		}
		return nil, true
	case "next_to":
		return expr.Func(e.nextTo), true
	}
	return nil, false
}

func (e campaignEnv) str(name string) string {
	v, _ := e.st.Var(name)
	return optional.Value[string](v)
}

// Owner is the faction holding the region now ("" for none).
func RegionOwner(db *DB, st CampaignState, id string) string {
	if v, ok := st.Var(RegionVar(id, "owner")); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	if r := db.Regions[id]; r != nil {
		return r.Owner
	}
	return ""
}

// Troops is the region's garrison now.
func Garrison(db *DB, st CampaignState, id string) float64 {
	return RegionValue(db, st, id, "troops")
}

// Value is the region's troops or one of its values now.
func RegionValue(db *DB, st CampaignState, id, name string) float64 {
	if v, ok := st.Var(RegionVar(id, name)); ok {
		if f, ok := expr.ToNumber(v); ok {
			return f
		}
	}
	r := db.Regions[id]
	if r == nil {
		return 0
	}
	if name == "troops" {
		return float64(r.Troops)
	}
	return r.Values[name]
}

// GeneralTroops is the troops a general leads now.
func GeneralTroops(db *DB, st CampaignState, id string) float64 {
	if v, ok := st.Var(GeneralVar(id, "troops")); ok {
		if f, ok := expr.ToNumber(v); ok {
			return f
		}
	}
	if c := db.Game.Campaign; c != nil {
		return float64(c.Generals[id].Troops)
	}
	return 0
}

// Acted reports whether the general has marched this turn.
func Acted(_ *DB, st CampaignState, id string) bool {
	v, _ := st.Var(GeneralVar(id, "acted"))
	return expr.Truthy(v)
}

// Ready reports whether the general can march: joined, not marched this turn, with troops.
func Ready(db *DB, st CampaignState, id string) bool {
	return st.Joined(id) && !Acted(db, st, id) && GeneralTroops(db, st, id) > 0
}

// Turn is the campaign's turn (1 at the start).
func Turn(_ *DB, st CampaignState) int {
	if v, ok := st.Var(VarTurn); ok {
		if f, ok := expr.ToNumber(v); ok {
			return int(f)
		}
	}
	return 1
}

// TurnActions is how many actions a turn has now (campaign.actions; 1 when unset).
func TurnActions(db *DB, st CampaignState) float64 {
	c := db.Game.Campaign
	if c == nil || !c.Actions.Set() {
		return 1
	}
	n, err := c.Actions.Number(CampaignEnv(db, st))
	if err != nil {
		return 1
	}
	return max(0, float64(int(n)))
}

// ActionsLeft is the actions left in the turn.
func ActionsLeft(db *DB, st CampaignState) float64 {
	if v, ok := st.Var(VarActions); ok {
		if f, ok := expr.ToNumber(v); ok {
			return f
		}
	}
	return TurnActions(db, st)
}

// RegionsOf are the regions a faction holds now, in the written order.
func RegionsOf(db *DB, st CampaignState, faction string) []string {
	var out []string
	for _, id := range RegionIDs(db) {
		if RegionOwner(db, st, id) == faction {
			out = append(out, id)
		}
	}
	return out
}

// Borders reports whether region id borders a region the faction holds.
func Borders(db *DB, st CampaignState, id, faction string) bool {
	r := db.Regions[id]
	if r == nil {
		return false
	}
	for _, n := range r.Next {
		if db.Regions[n] != nil && RegionOwner(db, st, n) == faction {
			return true
		}
	}
	return false
}

func (e campaignEnv) region(id string) expr.Map {
	db, st := e.db, e.st
	r := db.Regions[id]
	next := make([]any, len(r.Next))
	for i, n := range r.Next {
		next[i] = n
	}
	owner := RegionOwner(db, st, id)
	capital := false
	if f := db.Factions[owner]; f != nil {
		capital = f.Capital == id
	}
	player := ""
	if c := db.Game.Campaign; c != nil {
		player = c.Player
	}
	m := expr.Map{
		"id": id, "name": RegionName(db, id), "owner": owner, "troops": Garrison(db, st, id),
		"next": next, "capital": capital, "border": owner != player && Borders(db, st, id, player),
	}
	for _, v := range ValueNames(db) {
		m[v] = RegionValue(db, st, id, v)
	}
	return m
}

// regionsView is region.<id>.
type regionsView campaignEnv

func (v regionsView) Field(id string) (any, bool) {
	if v.db.Regions[id] == nil {
		return nil, true
	}
	return campaignEnv(v).region(id), true
}

func (v regionsView) Keys() []string { return RegionIDs(v.db) }

// factionsView is faction.<id>.
type factionsView campaignEnv

func (v factionsView) Field(id string) (any, bool) {
	if v.db.Factions[id] == nil {
		return nil, true
	}
	return factionView{campaignEnv(v), id}, true
}

func (v factionsView) Keys() []string { return FactionIDs(v.db) }

// factionView is one faction: name, color, capital, regions (how many it holds), troops
// (its garrisons), alive (holds a region), player, and sum_<value> of its regions.
type factionView struct {
	e  campaignEnv
	id string
}

func (v factionView) Field(name string) (any, bool) {
	db, st := v.e.db, v.e.st
	f := db.Factions[v.id]
	held := RegionsOf(db, st, v.id)
	sum := func(field string) float64 {
		t := 0.0
		for _, r := range held {
			t += RegionValue(db, st, r, field)
		}
		return t
	}
	switch name {
	case "id":
		return v.id, true
	case "name":
		if f.Name != "" {
			return f.Name, true
		}
		return v.id, true
	case "color":
		if f.Color.Valid {
			c := f.Color.NRGBA
			return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B), true
		}
		return "", true
	case "capital":
		return f.Capital, true
	case "regions":
		return float64(len(held)), true
	case "troops":
		return sum("troops"), true
	case "alive":
		return len(held) > 0, true
	case "player":
		c := db.Game.Campaign
		return c != nil && c.Player == v.id, true
	}
	if value, ok := strings.CutPrefix(name, "sum_"); ok && slices.Contains(ValueNames(db), value) {
		return sum(value), true
	}
	return nil, false
}

func (v factionView) Keys() []string {
	keys := []string{"id", "name", "color", "capital", "regions", "troops", "alive", "player"}
	for _, n := range ValueNames(v.e.db) {
		keys = append(keys, "sum_"+n)
	}
	return keys
}

func (e campaignEnv) campaign() expr.Map {
	c := e.db.Game.Campaign
	m := expr.Map{
		"turn": float64(Turn(e.db, e.st)), "actions": TurnActions(e.db, e.st), "actions_left": ActionsLeft(e.db, e.st),
		"target": e.str(VarTarget), "general": e.str(VarGeneral), "result": e.str(VarResult), "turns": 0.0, "player": "",
	}
	if c != nil {
		m["turns"], m["player"] = float64(c.Turns), c.Player
	}
	return m
}

// generalsView is generals.<actor id>.
type generalsView campaignEnv

func (v generalsView) Field(id string) (any, bool) {
	c := v.db.Game.Campaign
	if c == nil {
		return nil, true
	}
	if _, ok := c.Generals[id]; !ok {
		return nil, true
	}
	return campaignEnv(v).general(id), true
}

func (v generalsView) Keys() []string {
	if c := v.db.Game.Campaign; c != nil {
		return slices.Sorted(maps.Keys(c.Generals))
	}
	return nil
}

func (e campaignEnv) general(id string) expr.Map {
	db, st := e.db, e.st
	name := id
	if a := db.Actors[id]; a != nil && a.Name != "" {
		name = a.Name
	}
	might := 0.0
	if c := db.Game.Campaign; c != nil {
		might = c.Generals[id].Might
	}
	return expr.Map{
		"id": id, "name": name, "troops": GeneralTroops(db, st, id), "might": might,
		"acted": Acted(db, st, id), "joined": st.Joined(id), "ready": Ready(db, st, id),
	}
}

// nextTo is next_to(region, faction): the region borders a region the faction holds.
func (e campaignEnv) nextTo(args []any) (any, error) {
	if len(args) != 2 {
		return nil, errors.New("next_to takes a region and a faction")
	}
	id, ok1 := args[0].(string)
	f, ok2 := args[1].(string)
	if !ok1 || !ok2 {
		return nil, errors.New("next_to takes the ids of a region and a faction")
	}
	return Borders(e.db, e.st, id, f), nil
}

// The types of the campaign's names for the checks.
var (
	tRegion = objType("a region", map[string]*vtype{
		"id": tStr, "name": tStr, "owner": tStr, "troops": tNum, "capital": tBool, "border": tBool,
		"next": {kind: vList, desc: "next", elem: tStr},
	}, func(c *checker, name string) (*vtype, string) {
		if slices.Contains(ValueNames(c.db), name) {
			return tNum, ""
		}
		return nil, ""
	})
	tFaction = objType("a faction", map[string]*vtype{
		"id": tStr, "name": tStr, "color": tStr, "capital": tStr, "regions": tNum, "troops": tNum, "alive": tBool, "player": tBool,
	}, func(c *checker, name string) (*vtype, string) {
		if v, ok := strings.CutPrefix(name, "sum_"); ok {
			if !slices.Contains(ValueNames(c.db), v) {
				return nil, fmt.Sprintf("no region has the value %q", v)
			}
			return tNum, ""
		}
		return nil, ""
	})
	tGeneral = objType("a general", map[string]*vtype{
		"id": tStr, "name": tStr, "troops": tNum, "might": tNum, "acted": tBool, "joined": tBool, "ready": tBool,
	}, nil)
	tCampaign = objType("campaign", map[string]*vtype{
		"turn": tNum, "turns": tNum, "actions": tNum, "actions_left": tNum, "player": tStr, "target": tStr, "general": tStr, "result": tStr,
	}, nil)
)

// regionOver and clashOver are the types of s, t, a, b, power and defense where they are
// names of a campaign (a faction's rule, an order's where, the clash).
var (
	regionOver = map[string]*vtype{"s": tRegion, "t": tRegion}
	clashOver  = map[string]*vtype{
		"a": objType("the side marching", map[string]*vtype{"troops": tNum, "might": tNum, "faction": tStr, "general": tStr, "from": tStr}, nil),
		"b": tRegion, "power": tNum, "defense": tNum,
	}
)
