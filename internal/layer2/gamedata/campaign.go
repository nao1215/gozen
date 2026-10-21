package gamedata

import (
	"fmt"
	"image"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/layer1/geom"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/diag"
	"github.com/nao1215/gozen/internal/expr"
)

// A campaign is a game of regions held by factions over turns (schema_version 5, ADR
// 0030): data/ defines the regions and the factions, game.yaml's campaign the turns, the
// orders and how a march is decided. Its state is in variables with generated names
// (campaign_*, region_<id>_*, general_<id>_*), so saves, rolling back and the panel see it.

// SchemaVersion5 adds campaigns (regions, factions and game.yaml's campaign; ADR 0030)
// and the commands order, end_turn and game_over. YAML of version 4 is read as is.
const SchemaVersion5 = 5

// Region is a region of a campaign: its neighbours, its first owner and garrison, and its
// own numbers (values: wealth and the like).
type Region struct {
	// Name is the name shown (the id when empty).
	Name string `yaml:"name"`
	// Next are the regions it borders. A border is written on both regions unless one_way.
	Next []string `yaml:"next"`
	// OneWay lets next name regions that do not name this one back.
	OneWay bool `yaml:"one_way"`
	// Owner is the faction holding it at the start ("" for a free region).
	Owner string `yaml:"owner"`
	// Troops is the garrison at the start.
	Troops int `yaml:"troops"`
	// Values are the region's own numbers at the start, read and set as region.<id>.<name>.
	Values map[string]float64 `yaml:"values"`
	// Defend is the troop a general fights on marching here with an order of fight: battle,
	// and whose members hold it, sharing its garrison, in a clash shown as units.
	Defend string `yaml:"defend"`
	// Terrain is the kind of ground (field, town, castle or any word): the background of a
	// clash shown as units (clash.backgrounds).
	Terrain string `yaml:"terrain"`
	// Area is the region's shape on the campaign's map picture (its pixels, three points
	// or more): the map fills it in the owner's colour and draws its border. Without it the
	// region shows as its point's flag.
	Area Polygon `yaml:"area"`
}

// Faction is a side of a campaign: one is the player's (campaign.player), the others move
// by their ai rules at the end of each turn.
type Faction struct {
	Name string `yaml:"name"`
	// Color is the faction's colour (the map's marks of its regions).
	Color Color `yaml:"color"`
	// Capital is a region the faction holds at the start; losing it is the player's default
	// defeat (campaign.lose).
	Capital string `yaml:"capital"`
	// AI are the rules the faction follows at the end of each turn, each in its order.
	AI []FactionRule `yaml:"ai"`
	// Generals are the enemies who lead the faction's marches: when the campaign shows
	// marches as clashes of units (an order with fight: clash_view), a march takes the
	// strongest of them (by atk), up to clash.units, and is fought as a clash.
	Generals []string `yaml:"generals"`
}

// FactionRule is one rule of a faction's turn: it drafts troops into every region it
// holds, or marches once along a border, while When holds.
type FactionRule struct {
	When Expr `yaml:"when"`
	// Draft is the troops each region of the faction gains (t is the region).
	Draft Expr `yaml:"draft"`
	// March sends Send of the garrison of a region s to a region t it borders, where Where
	// holds (both s and t are regions), picked by By (Pick: first in the written order of
	// the regions, min, max or random).
	March bool `yaml:"march"`
	// Keep is the least garrison a region must have to march from it.
	Keep float64 `yaml:"keep"`
	// Send is the share of the garrison that marches (0.6 when 0).
	Send  float64 `yaml:"send"`
	Where Expr    `yaml:"where"`
	Pick  string  `yaml:"pick"`
	By    Expr    `yaml:"by"`
}

// Campaign is game.yaml's campaign: the place of the orders, the player's faction, the
// turns, the orders, the generals and the rules of a march.
type Campaign struct {
	// Location is the place whose menu lists the orders and the end of the turn.
	Location string `yaml:"location"`
	// Player is the player's faction.
	Player string `yaml:"player"`
	// Turns is the last turn (0: no limit). Ending it without winning loses.
	Turns int `yaml:"turns"`
	// Actions is how many orders a turn has, worked out at its start (1 when empty). It
	// may read only region, faction, campaign and generals.
	Actions Expr `yaml:"actions"`
	// Win ends the campaign won (default: the player holds every region).
	Win Expr `yaml:"win"`
	// Lose ends it lost (default: the player does not hold its capital).
	Lose Expr `yaml:"lose"`
	// Won and Lost are the events run at the end before the game is cleared or over.
	Won  string `yaml:"won"`
	Lost string `yaml:"lost"`
	// OnTurn is the event run at the start of each turn after the first.
	OnTurn string `yaml:"on_turn"`
	// Calendar names the turns: "" (turn N) or week (a turn is a week of a 52-week
	// year: campaign.week and campaign.year).
	Calendar string `yaml:"calendar"`
	// Count is how a turn's orders are counted: actions (the turn's actions, each order
	// spending its cost; the default) or orders (each order may be given per_turn times).
	Count string `yaml:"count"`
	// Defeat is what becomes of a general whose troops a march brings to 0.
	Defeat Defeat `yaml:"defeat"`
	// EndTurn is the label of the entry that ends the turn (the engine's words when empty).
	EndTurn string `yaml:"end_turn"`
	// Generals are the actors who lead the player's marches, with their first troops.
	// One serves once they have joined.
	Generals map[string]General `yaml:"generals"`
	// Clash decides a march.
	Clash Clash `yaml:"clash"`
	// Orders are what the player can do in a turn, listed in the written order.
	Orders map[string]*Order `yaml:"orders"`

	orderIDs []string
}

// Defeat is what becomes of a general beaten to no troops: away for Out turns (0: stays,
// and only needs troops again). A general is never lost for good.
type Defeat struct {
	Out int `yaml:"out"`
}

// General is an actor's place in a campaign: the troops they lead at the start and their
// might (read as a.might in the clash).
type General struct {
	Troops int     `yaml:"troops"`
	Might  float64 `yaml:"might"`
	// Tactic is the tactic the sortie picks for them first (clash.tactics; the first one
	// when empty).
	Tactic string `yaml:"tactic"`
	// Skills are the skills they use in a clash shown as units when charged (clash.skills).
	Skills []string `yaml:"skills"`
}

// Clash decides a march of a (the side marching: troops, might, faction, general) on the
// region b. The march takes b when power is more than defense. Every formula has a default.
type Clash struct {
	// Power is the strength of the march (a.troops * (0.8 + rand() * 0.4)).
	Power Expr `yaml:"power"`
	// Defense is the strength of the region (b.troops * 1.25).
	Defense Expr `yaml:"defense"`
	// Won is the troops of a left after taking b (a.troops * (1 - 0.5 * defense / power)).
	Won Expr `yaml:"won"`
	// Lost is the troops of a left after failing (a.troops * 0.5).
	Lost Expr `yaml:"lost"`
	// Held is the garrison of b left after holding (b.troops - power * 0.4).
	Held Expr `yaml:"held"`
	// Garrison is the share of a general's troops left in a region taken (0.4 when
	// unset); a faction's march leaves all of them.
	Garrison *float64 `yaml:"garrison"`
	// Hit is the troops a unit's strike takes in a clash shown as units (a strikes, b is
	// struck; a.troops * 0.15 * (0.8 + rand() * 0.4)).
	Hit Expr `yaml:"hit"`
	// Rounds is the most rounds of a clash shown as units (5 when 0); when they run out,
	// the region holds.
	Rounds int `yaml:"rounds"`
	// Capture is the chance that a routed foe is caught (0.2 when unset).
	Capture *float64 `yaml:"capture"`
	// Backgrounds are the pictures behind a clash shown as units, by the region's terrain.
	Backgrounds map[string]string `yaml:"backgrounds"`
	// Soldiers are the pictures of one soldier of the player's side (player, facing right)
	// and of the foes' (foe, facing left) that a clash shown as units lines up behind each
	// leader, one for PerSoldier troops; a foe without one shows its leader's picture small.
	Soldiers map[string]string `yaml:"soldiers"`
	// PerSoldier is the troops one soldier of the lines stands for (50 when 0).
	PerSoldier int `yaml:"per_soldier"`
	// Rope draws the rope of the two sides' troops over a clash shown as units (true when
	// unset); false leaves the soldiers alone to tell them.
	Rope *bool `yaml:"rope"`
	// Units is the most units a side has in a clash shown as units, 1 to 4 (4 when 0): 1
	// makes it a duel; the first two of a side stand in front.
	Units int `yaml:"units"`
	// Tactics are the ways a unit picks the foe it strikes, chosen for each general on the
	// sortie (the engine's front, weakest, strongest, leader and guard when empty).
	Tactics map[string]*ClashTactic `yaml:"tactics"`
	// Skills are the skills a unit uses when charged (generals.<id>.skills, foes).
	Skills map[string]*ClashSkill `yaml:"skills"`
}

// ClashTactic is a way of picking the foe to strike in a clash shown as units: among the
// foes in front (every foe standing with all) for whom where holds (t the foe, a the
// unit striking), the one with the least or most by (pick; random when empty). Deal and
// take scale the troops the unit takes and loses (1 when 0).
type ClashTactic struct {
	Label string  `yaml:"label"`
	All   bool    `yaml:"all"`
	Where Expr    `yaml:"where"`
	Pick  string  `yaml:"pick"`
	By    Expr    `yaml:"by"`
	Deal  float64 `yaml:"deal"`
	Take  float64 `yaml:"take"`
}

// ClashSkill is a skill of a clash shown as units: charged after Charge rounds (2 when
// 0), it is used instead of the next strike, takes Power troops (a strikes, b is struck;
// twice the hit when empty) from the foe the tactic picks, or from every foe in front
// with all, and gives Heal troops back to the unit. Foes are the enemies who use it.
type ClashSkill struct {
	Label  string   `yaml:"label"`
	Charge int      `yaml:"charge"`
	Power  Expr     `yaml:"power"`
	All    bool     `yaml:"all"`
	Heal   Expr     `yaml:"heal"`
	Foes   []string `yaml:"foes"`
	// Cutin shows the user's picture across the screen with the skill's name before it
	// strikes (a general's cut-in picture, else the face; a foe's picture).
	Cutin bool `yaml:"cutin"`
}

// DeclaredTactics are the tactics of a clash: clash.tactics, or the engine's when empty.
func (c *Clash) DeclaredTactics() map[string]*ClashTactic {
	if len(c.Tactics) > 0 {
		return c.Tactics
	}
	return builtinTactics
}

// TacticIDs are the tactics in a fixed order: the engine's in theirs, clash.tactics by ID.
func (c *Clash) TacticIDs() []string {
	if len(c.Tactics) > 0 {
		return slices.Sorted(maps.Keys(c.Tactics))
	}
	return []string{"front", "weakest", "strongest", "leader", "guard"}
}

// builtinTactics are the tactics when a campaign declares none; their labels are words
// of the engine (lang.T in the screens).
var builtinTactics = map[string]*ClashTactic{
	"front":     {Label: "近くの敵"},
	"weakest":   {Label: "兵の少ない敵", Pick: "least", By: MustExpr("t.troops"), Deal: 0.75},
	"strongest": {Label: "兵の多い敵", Pick: "most", By: MustExpr("t.troops"), Deal: 1.2},
	"leader":    {Label: "弱った将", All: true, Pick: "least", By: MustExpr("t.hp"), Deal: 0.8},
	"guard":     {Label: "守り", Deal: 0.6, Take: 0.6},
}

// Order is one of the player's orders: it costs actions, may need a general and a target
// region, and runs its do with them bound (general and target in expressions and set).
type Order struct {
	Label string `yaml:"label"`
	// Cost is the actions it takes (1 when unset).
	Cost *int `yaml:"cost"`
	// When must hold for the order to be listed.
	When Expr `yaml:"when"`
	// General is who leads it: ready (joined, has not marched this turn, has troops), any
	// (joined), or empty for none.
	General string `yaml:"general"`
	// Target is the region it is given: border (not the player's, next to one of the
	// player's), own (the player's), any, or empty for none. Where narrows it (t is the
	// region).
	Target string `yaml:"target"`
	Where  Expr   `yaml:"where"`
	// March marches the general's troops on the target (Clash) before do.
	March bool `yaml:"march"`
	// Fight decides the march by a battle (battle): the general fights the target's defend
	// troop, the battle of the game's dungeons and fields. Empty: by the numbers of Clash.
	Fight string `yaml:"fight"`
	// PerTurn is how many times a turn the order may be given when campaign.count is
	// orders (1 when 0).
	PerTurn int `yaml:"per_turn"`
	// UntilTaken lets a march be given again and again in a turn until a region is taken
	// (each general still marches once).
	UntilTaken bool `yaml:"until_taken"`
	// Generals is how many generals may march together (1 when 0): after the first, the
	// order asks who else goes, until March is chosen or all are taken.
	Generals int `yaml:"generals"`
	// OnWin, OnLose and OnDraw run after a march by how it went (won, lost: the generals
	// were routed or held, draw: the rounds ran out), before do: a general's word on it.
	OnWin  []Command `yaml:"on_win"`
	OnLose []Command `yaml:"on_lose"`
	OnDraw []Command `yaml:"on_draw"`
	Do     []Command `yaml:"do"`
}

// CostOf is the actions the order takes.
func (o *Order) CostOf() int {
	if o.Cost == nil {
		return 1
	}
	return *o.Cost
}

// OrderIDs are the orders in the order they were written.
func (c *Campaign) OrderIDs() []string { return orderedKeys(c.Orders, c.orderIDs) }

// GarrisonShare is the share of a general's troops left in a region taken.
func (c *Clash) GarrisonShare() float64 {
	if c.Garrison == nil {
		return 0.4
	}
	return *c.Garrison
}

// The words of order.general and order.target, and of a faction rule's pick.
var (
	OrderGenerals = []string{"", "ready", "any"}
	OrderTargets  = []string{"", "border", "own", "any"}
	MarchPicks    = []string{"", "first", "min", "max", "random"}
)

// CampaignNames are the expression names of a campaign: the regions, the factions, the
// campaign's turn and actions, the generals, the order's general and target, and
// next_to(region, faction).
var CampaignNames = []string{"region", "faction", "campaign", "generals", "general", "target", "next_to", "orders"}

// The generated names of a campaign's state (var.<name>).
const (
	VarTurn    = "campaign_turn"
	VarActions = "campaign_actions"
	VarTarget  = "campaign_target"
	VarGeneral = "campaign_general"
	VarResult  = "campaign_result"
	// VarTaken is how many regions the player took this turn; VarMarching the generals of
	// the running march, joined by commas.
	VarTaken    = "campaign_taken"
	VarMarching = "campaign_marching"
	// VarClash is how the last clash shown as units ended: won, lost or draw.
	VarClash = "campaign_clash"
)

// RegionVar is the generated name of a region's field (owner, troops or a value).
func RegionVar(id, field string) string { return "region_" + id + "_" + field }

// OrderVar is the generated name of an order's field (used: times given this turn).
func OrderVar(id, field string) string { return "order_" + id + "_" + field }

// GeneralVar is the generated name of a general's field (troops, acted, out).
func GeneralVar(id, field string) string { return "general_" + id + "_" + field }

// RegionIDs are the regions in the order they were written.
func RegionIDs(db *DB) []string { return orderedKeys(db.Regions, db.Order["regions"]) }

// FactionIDs are the factions in the order they were written.
func FactionIDs(db *DB) []string { return orderedKeys(db.Factions, db.Order["factions"]) }

// RegionName is the name a region shows.
func RegionName(db *DB, id string) string {
	if r := db.Regions[id]; r != nil && r.Name != "" {
		return r.Name
	}
	return id
}

// ValueNames are the names of the regions' values, of every region.
func ValueNames(db *DB) []string {
	seen := map[string]bool{}
	for _, r := range db.Regions {
		for k := range r.Values {
			seen[k] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// campaignStateNames adds the generated names of the campaign to add.
func (db *DB) campaignStateNames(add func(name, typ string)) {
	if db.Game.Campaign == nil && len(db.Regions) == 0 {
		return
	}
	add(VarTurn, "number")
	add(VarActions, "number")
	add(VarTaken, "number")
	for _, n := range []string{VarTarget, VarGeneral, VarResult, VarMarching, VarClash} {
		add(n, "string")
	}
	values := ValueNames(db)
	for id := range db.Regions {
		add(RegionVar(id, "owner"), "string")
		add(RegionVar(id, "troops"), "number")
		for _, v := range values {
			add(RegionVar(id, v), "number")
		}
	}
	if c := db.Game.Campaign; c != nil {
		for id := range c.Generals {
			add(GeneralVar(id, "troops"), "number")
			add(GeneralVar(id, "acted"), "number")
			add(GeneralVar(id, "out"), "number")
		}
		for id := range c.Orders {
			add(OrderVar(id, "used"), "number")
		}
	}
}

// keepCampaignOrder remembers the written order of game.yaml's campaign.orders.
func (db *DB) keepCampaignOrder(raw yaml.MapSlice) {
	c := db.Game.Campaign
	if c == nil {
		return
	}
	for _, it := range sectionMap(sectionMap(raw, "campaign"), "orders") {
		c.orderIDs = append(c.orderIDs, fmt.Sprint(it.Key))
	}
}

// campaign checks the regions, the factions and game.yaml's campaign.
// regionAreas checks the regions' areas: polygons in the campaign map's picture that do
// not cross themselves nor overlap each other (a shared border is fine).
func (c *checker) regionAreas() {
	db := c.db
	var size image.Point
	known := false
	if cp := db.Game.Campaign; cp != nil {
		if l := db.Locations[cp.Location]; l != nil && l.Map != nil {
			size, known = c.pictureSize(l.Map.Picture)
		}
	}
	var with []string
	for _, id := range RegionIDs(db) {
		r := db.Regions[id]
		if len(r.Area) == 0 {
			continue
		}
		if c.polygon("regions."+id+".area", r.Area, size, known) {
			with = append(with, id)
		}
	}
	for i, a := range with {
		for _, b := range with[i+1:] {
			if areasOverlap(db.Regions[a].Area.Poly(), db.Regions[b].Area.Poly()) {
				c.errorf(diag.Shape, "regions."+b+".area", "the area overlaps %s's", a)
			}
		}
	}
}

// areasOverlap reports whether two areas share more than a border: an edge of one
// crosses an edge of the other, or a point of one lies inside the other off its border.
func areasOverlap(q, r geom.Poly) bool {
	in := func(p geom.Pt, poly geom.Poly) bool { return poly.Inside(p) && poly.Distance(p) > 0.5 }
	for _, p := range append(slices.Clone(q), q.Center()) {
		if in(p, r) {
			return true
		}
	}
	for _, p := range append(slices.Clone(r), r.Center()) {
		if in(p, q) {
			return true
		}
	}
	for i := range q {
		for j := range r {
			if geom.SegmentsCross(q[i], q[(i+1)%len(q)], r[j], r[(j+1)%len(r)]) {
				return true
			}
		}
	}
	return false
}

func (c *checker) campaign() {
	db := c.db
	if len(db.Regions) == 0 && len(db.Factions) == 0 && db.Game.Campaign == nil {
		return
	}
	if len(db.Regions) > 0 {
		c.needsVersion("regions", "regions", SchemaVersion5)
	}
	if len(db.Factions) > 0 {
		c.needsVersion("factions", "factions", SchemaVersion5)
	}
	c.regionAreas()
	names := append(slices.Clone(FieldNames), "t")
	for _, id := range RegionIDs(db) {
		r := db.Regions[id]
		w := "regions." + id
		if !declaredName.MatchString(id) {
			c.errorf(diag.NotAWord, w, "a region's id is lowercase letters, digits and _, not %q", id)
		}
		if id == "target" {
			c.errorf(diag.NotAWord, w, "target is the region of an order, not a region's id")
		}
		seen := map[string]bool{}
		for i, n := range r.Next {
			ww := fmt.Sprintf("%s.next[%d]", w, i)
			other, ok := db.Regions[n]
			switch {
			case !ok:
				c.ref(ww, "region", n, false)
			case n == id:
				c.errorf(diag.Shape, ww, "a region does not border itself")
			case seen[n]:
				c.errorf(diag.Duplicate, ww, "%s is written twice", n)
			case !r.OneWay && !other.OneWay && !slices.Contains(other.Next, id):
				c.errorf(diag.Shape, ww, "%s borders %s, but %s's next does not name %s (write it on both, or one_way: true)", id, n, n, id)
			}
			seen[n] = true
		}
		if r.Owner != "" {
			_, ok := db.Factions[r.Owner]
			c.ref(w+".owner", "faction", r.Owner, ok)
		}
		if r.Defend != "" {
			_, ok := db.Troops[r.Defend]
			c.ref(w+".defend", "troop", r.Defend, ok)
		}
		if r.Troops < 0 {
			c.errorf(diag.OutOfRange, w+".troops", "troops are 0 or more, not %d", r.Troops)
		}
		for _, k := range slices.Sorted(maps.Keys(r.Values)) {
			if !declaredName.MatchString(k) || slices.Contains(regionFields, k) {
				c.errorf(diag.NotAWord, w+".values."+k, "a value's name is lowercase letters, digits and _, and not one of %s", strings.Join(regionFields, ", "))
			}
		}
	}
	for _, id := range FactionIDs(db) {
		f := db.Factions[id]
		for i, e := range f.Generals {
			_, ok := db.Enemies[e]
			c.ref(fmt.Sprintf("factions.%s.generals[%d]", id, i), "enemy", e, ok)
		}
		w := "factions." + id
		if !declaredName.MatchString(id) {
			c.errorf(diag.NotAWord, w, "a faction's id is lowercase letters, digits and _, not %q", id)
		}
		if f.Capital != "" {
			r, ok := db.Regions[f.Capital]
			c.ref(w+".capital", "region", f.Capital, ok)
			if ok && r.Owner != id {
				c.errorf(diag.Shape, w+".capital", "%s's capital %s is held by %q at the start, not by %s", id, f.Capital, r.Owner, id)
			}
		}
		for i := range f.AI {
			c.factionRule(fmt.Sprintf("%s.ai[%d]", w, i), &f.AI[i], names)
		}
	}
	cp := db.Game.Campaign
	if cp == nil {
		if len(db.Regions) > 0 {
			c.errorf(diag.Shape, "game.yaml", "regions need game.yaml's campaign (its place, the player's faction and the orders)")
		}
		return
	}
	w := "game.yaml campaign"
	c.needsVersion(w, "campaign", SchemaVersion5)
	if len(db.Regions) == 0 {
		c.errorf(diag.Shape, w, "a campaign needs regions in data/")
	}
	_, ok := db.Locations[cp.Location]
	c.ref(w+".location", "location", cp.Location, ok)
	if cp.Location == "" {
		c.errorf(diag.Shape, w, "campaign.location is the place whose menu lists the orders")
	}
	_, ok = db.Factions[cp.Player]
	c.ref(w+".player", "faction", cp.Player, ok)
	if cp.Player == "" {
		c.errorf(diag.Shape, w, "campaign.player is the player's faction")
	}
	if cp.Calendar != "" && cp.Calendar != "week" {
		c.errorf(diag.NotAWord, w+".calendar", "calendar is week, not %q", cp.Calendar)
	}
	if cp.Count != "" && cp.Count != "actions" && cp.Count != "orders" {
		c.errorf(diag.NotAWord, w+".count", "count is actions or orders, not %q", cp.Count)
	}
	if cp.Defeat.Out < 0 {
		c.errorf(diag.OutOfRange, w+".defeat.out", "out is turns, 0 or more")
	}
	if cp.Turns < 0 {
		c.errorf(diag.OutOfRange, w+".turns", "turns are 0 (no limit) or more, not %d", cp.Turns)
	}
	c.expr(w+".actions", cp.Actions, CampaignNames)
	c.expr(w+".win", cp.Win, FieldNames)
	c.expr(w+".lose", cp.Lose, FieldNames)
	for _, ev := range []struct{ name, id string }{{"won", cp.Won}, {"lost", cp.Lost}, {"on_turn", cp.OnTurn}} {
		_, ok := db.Events[ev.id]
		c.ref(w+"."+ev.name, "event", ev.id, ok)
	}
	for _, id := range slices.Sorted(maps.Keys(cp.Generals)) {
		_, ok := db.Actors[id]
		c.ref(w+".generals."+id, "actor", id, ok)
		if cp.Generals[id].Troops < 0 {
			c.errorf(diag.OutOfRange, w+".generals."+id+".troops", "troops are 0 or more")
		}
	}
	clash := append(slices.Clone(FieldNames), "a", "b", "power", "defense")
	for _, f := range []struct {
		name string
		e    Expr
	}{{"power", cp.Clash.Power}, {"defense", cp.Clash.Defense}, {"won", cp.Clash.Won}, {"lost", cp.Clash.Lost}, {"held", cp.Clash.Held}} {
		c.exprIn(w+".clash."+f.name, f.e, clash, clashOver)
	}
	if cp.Clash.Rounds < 0 {
		c.errorf(diag.OutOfRange, w+".clash.rounds", "rounds are 0 (5) or more")
	}
	if p := cp.Clash.Capture; p != nil && (*p < 0 || *p > 1) {
		c.errorf(diag.OutOfRange, w+".clash.capture", "capture is a chance from 0 to 1, not %g", *p)
	}
	c.exprIn(w+".clash.hit", cp.Clash.Hit, append(slices.Clone(FieldNames), "a", "b", "round"), hitOver)
	for _, k := range slices.Sorted(maps.Keys(cp.Clash.Soldiers)) {
		if k != "player" && k != "foe" {
			c.errorf(diag.NotAWord, w+".clash.soldiers."+k, "soldiers are player and foe, not %q", k)
		}
		c.picture(w+".clash.soldiers."+k, Picture{File: cp.Clash.Soldiers[k]})
	}
	if cp.Clash.PerSoldier < 0 {
		c.errorf(diag.OutOfRange, w+".clash.per_soldier", "per_soldier is troops, 0 (50) or more")
	}
	if cp.Clash.Units < 0 || cp.Clash.Units > 4 {
		c.errorf(diag.OutOfRange, w+".clash.units", "units are 1 to 4 (0 for 4), not %d", cp.Clash.Units)
	}
	unit := append(slices.Clone(FieldNames), "a", "t", "round")
	for _, id := range slices.Sorted(maps.Keys(cp.Clash.Tactics)) {
		t, at := cp.Clash.Tactics[id], w+".clash.tactics."+id
		if t == nil {
			c.errorf(diag.Shape, at, "a tactic needs a label")
			continue
		}
		if t.Label == "" {
			c.errorf(diag.Shape, at+".label", "a tactic needs a label")
		}
		if t.Pick != "" && t.Pick != "least" && t.Pick != "most" && t.Pick != "random" {
			c.errorf(diag.NotAWord, at+".pick", "pick is least, most or random, not %q", t.Pick)
		}
		if (t.Pick == "least" || t.Pick == "most") && !t.By.Set() {
			c.errorf(diag.Shape, at+".by", "pick %s needs by", t.Pick)
		}
		if t.Deal < 0 || t.Take < 0 {
			c.errorf(diag.OutOfRange, at, "deal and take are 0 (1) or more")
		}
		c.exprIn(at+".where", t.Where, unit, tacticOver)
		c.exprIn(at+".by", t.By, unit, tacticOver)
	}
	for _, id := range slices.Sorted(maps.Keys(cp.Clash.Skills)) {
		s, at := cp.Clash.Skills[id], w+".clash.skills."+id
		if s == nil {
			c.errorf(diag.Shape, at, "a skill needs a label")
			continue
		}
		if s.Label == "" {
			c.errorf(diag.Shape, at+".label", "a skill needs a label")
		}
		if s.Charge < 0 {
			c.errorf(diag.OutOfRange, at+".charge", "charge is rounds, 0 (2) or more")
		}
		c.exprIn(at+".power", s.Power, append(slices.Clone(FieldNames), "a", "b", "round"), hitOver)
		c.exprIn(at+".heal", s.Heal, append(slices.Clone(FieldNames), "a", "b", "round"), hitOver)
		for _, f := range s.Foes {
			_, ok := c.db.Enemies[f]
			c.ref(at+".foes", "enemy", f, ok)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(cp.Generals)) {
		g := cp.Generals[id]
		if g.Tactic != "" {
			if _, ok := cp.Clash.DeclaredTactics()[g.Tactic]; !ok {
				c.errorf(diag.UnknownReference, w+".generals."+id+".tactic", "unknown tactic %q (clash.tactics has %s)", g.Tactic, strings.Join(cp.Clash.TacticIDs(), ", "))
			}
		}
		for _, s := range g.Skills {
			if _, ok := cp.Clash.Skills[s]; !ok {
				c.errorf(diag.UnknownReference, w+".generals."+id+".skills", "unknown skill %q of clash.skills", s)
			}
		}
	}
	for _, k := range slices.Sorted(maps.Keys(cp.Clash.Backgrounds)) {
		c.picture(w+".clash.backgrounds."+k, Picture{File: cp.Clash.Backgrounds[k]})
	}
	if g := cp.Clash.Garrison; g != nil && (*g < 0 || *g > 1) {
		c.errorf(diag.OutOfRange, w+".clash.garrison", "garrison is a share from 0 to 1, not %g", *g)
	}
	if len(cp.Orders) == 0 {
		c.errorf(diag.Shape, w+".orders", "a campaign needs orders")
	}
	for _, id := range cp.OrderIDs() {
		o := cp.Orders[id]
		ow := w + ".orders." + id
		if o == nil {
			c.errorf(diag.Shape, ow, "an order is a map")
			continue
		}
		if !declaredName.MatchString(id) {
			c.errorf(diag.NotAWord, ow, "an order's id is lowercase letters, digits and _, not %q", id)
		}
		if o.Cost != nil && *o.Cost < 0 {
			c.errorf(diag.OutOfRange, ow+".cost", "cost is 0 or more")
		}
		if !slices.Contains(OrderGenerals, o.General) {
			c.errorf(diag.NotAWord, ow+".general", "general is ready or any, not %q", o.General)
		}
		if !slices.Contains(OrderTargets, o.Target) {
			c.errorf(diag.NotAWord, ow+".target", "target is border, own or any, not %q", o.Target)
		}
		if o.Where.Set() && o.Target == "" {
			c.errorf(diag.Shape, ow+".where", "where narrows the target: write target too")
		}
		if o.PerTurn < 0 || o.Generals < 0 {
			c.errorf(diag.OutOfRange, ow, "per_turn and generals are 0 or more")
		}
		if o.PerTurn > 0 && cp.Count != "orders" {
			c.errorf(diag.Shape, ow+".per_turn", "per_turn counts the orders of a turn: write count: orders in campaign")
		}
		if (o.UntilTaken || o.Generals > 1) && !o.March {
			c.errorf(diag.Shape, ow, "until_taken and generals are for a march: write march: true")
		}
		if o.Fight != "" && o.Fight != "battle" && o.Fight != "clash_view" {
			c.errorf(diag.NotAWord, ow+".fight", "fight is battle or clash_view, not %q", o.Fight)
		}
		if (len(o.OnWin) > 0 || len(o.OnLose) > 0 || len(o.OnDraw) > 0) && !o.March {
			c.errorf(diag.Shape, ow, "on_win, on_lose and on_draw follow a march: write march: true")
		}
		c.commands(ow+".on_win", o.OnWin, FieldNames)
		c.commands(ow+".on_lose", o.OnLose, FieldNames)
		c.commands(ow+".on_draw", o.OnDraw, FieldNames)
		if o.Fight != "" && !o.March {
			c.errorf(diag.Shape, ow+".fight", "fight: battle decides a march: write march: true")
		}
		if o.March && (o.General == "" || o.Target == "") {
			c.errorf(diag.Shape, ow+".march", "a march needs a general and a target")
		}
		if o.General != "" && len(cp.Generals) == 0 {
			c.errorf(diag.Shape, ow+".general", "the order asks for a general, but campaign.generals is empty")
		}
		c.text(ow+".label", o.Label, FieldNames)
		c.expr(ow+".when", o.When, FieldNames)
		c.exprIn(ow+".where", o.Where, names, regionOver)
		c.commands(ow+".do", o.Do, FieldNames)
	}
}

// regionFields are the fields of region.<id> that are not values.
var regionFields = []string{"id", "name", "owner", "troops", "next", "capital", "border"}

func (c *checker) factionRule(w string, r *FactionRule, names []string) {
	c.expr(w+".when", r.When, FieldNames)
	if r.Draft.Set() == r.March {
		c.errorf(diag.Shape, w, "a rule drafts (draft) or marches (march: true), one of them")
	}
	c.exprIn(w+".draft", r.Draft, names, regionOver)
	march := append(slices.Clone(names), "s")
	c.exprIn(w+".where", r.Where, march, regionOver)
	c.exprIn(w+".by", r.By, march, regionOver)
	if !slices.Contains(MarchPicks, r.Pick) {
		c.errorf(diag.NotAWord, w+".pick", "pick is first, min, max or random, not %q", r.Pick)
	}
	if (r.Pick == "min" || r.Pick == "max") && !r.By.Set() {
		c.errorf(diag.Shape, w+".by", "pick: %s needs by", r.Pick)
	}
	if r.Send < 0 || r.Send > 1 {
		c.errorf(diag.OutOfRange, w+".send", "send is a share from 0 to 1, not %g", r.Send)
	}
	if !r.March && (r.Where.Set() || r.By.Set() || r.Keep != 0 || r.Send != 0 || r.Pick != "") {
		c.errorf(diag.Shape, w, "where, by, pick, keep and send are for march: true")
	}
}

// campaignWarnings are the warnings of a campaign: regions split into parts no march
// crosses, a faction no other borders, and a win or a defeat that holds at the start.
func (c *checker) campaignWarnings() {
	db := c.db
	cp := db.Game.Campaign
	if cp == nil || len(db.Regions) == 0 {
		return
	}
	ids := RegionIDs(db)
	reached := map[string]bool{ids[0]: true}
	queue := []string{ids[0]}
	for len(queue) > 0 {
		r := db.Regions[queue[0]]
		queue = queue[1:]
		for _, n := range r.Next {
			if _, ok := db.Regions[n]; ok && !reached[n] {
				reached[n] = true
				queue = append(queue, n)
			}
		}
	}
	if len(reached) < len(ids) {
		var apart []string
		for _, id := range ids {
			if !reached[id] {
				apart = append(apart, id)
			}
		}
		c.warnf(diag.Shape, "regions", "the regions are in more than one part: %s cannot be reached from %s by next", strings.Join(apart, ", "), ids[0])
	}
	for _, f := range FactionIDs(db) {
		borders := false
		for _, id := range ids {
			r := db.Regions[id]
			if r.Owner != f {
				continue
			}
			for _, n := range r.Next {
				if o := db.Regions[n]; o != nil && o.Owner != f {
					borders = true
				}
			}
		}
		if !borders && slices.ContainsFunc(ids, func(id string) bool { return db.Regions[id].Owner == f }) {
			c.warnf(diag.Shape, "factions."+f, "%s borders no region of another side: it can neither march nor be marched on", f)
		}
	}
	if l := db.Locations[cp.Location]; l != nil && l.Map != nil {
		c.campaignMapWarnings(l.Map)
	}
	env := CampaignEnv(db, CampaignState{JoinedIDs: db.Game.Start.Party})
	for _, end := range []struct {
		name string
		e    Expr
	}{{"win", CampaignWin(db)}, {"lose", CampaignLose(db)}} {
		if !end.e.Set() {
			continue
		}
		if ok, err := end.e.Bool(env); err == nil && ok {
			c.warnf(diag.Shape, "game.yaml campaign."+end.name, "campaign.%s holds at the start (%s)", end.name, end.e.String())
		}
	}
	if cp.Actions.Set() {
		if n, err := cp.Actions.Number(env); err == nil && n < 1 {
			c.warnf(diag.Shape, "game.yaml campaign.actions", "the first turn has %g actions: no order can be given", n)
		}
	}
}

// CampaignWin is the condition that wins the campaign (its default when unset).
func CampaignWin(db *DB) Expr {
	cp := db.Game.Campaign
	if cp != nil && cp.Win.Set() {
		return cp.Win
	}
	if cp == nil {
		return Expr{}
	}
	return MustExpr(fmt.Sprintf("faction.%s.regions == %d", cp.Player, len(db.Regions)))
}

// CampaignLose is the condition that loses the campaign (its default when unset; none
// when the player's faction has no capital).
func CampaignLose(db *DB) Expr {
	cp := db.Game.Campaign
	if cp != nil && cp.Lose.Set() {
		return cp.Lose
	}
	if cp == nil {
		return Expr{}
	}
	f := db.Factions[cp.Player]
	if f == nil || f.Capital == "" {
		return Expr{}
	}
	return MustExpr(fmt.Sprintf("region.%s.owner != '%s'", f.Capital, cp.Player))
}

// campaignSetKey checks a left-hand side of set that names the campaign's state:
// region.<id>.<field>, target.<field>, generals.<id>.troops or general.troops. ok is false
// when the key is not one.
func (c *checker) campaignSetKey(w, k string, e Expr) bool {
	kind, rest, _ := strings.Cut(k, ".")
	var field string
	switch kind {
	case "region":
		var id string
		id, field, _ = strings.Cut(rest, ".")
		_, ok := c.db.Regions[id]
		c.ref(w+"."+k, "region", id, ok)
	case "target":
		field = rest
	case "generals":
		id, f, _ := strings.Cut(rest, ".")
		if cp := c.db.Game.Campaign; cp == nil || !hasGeneral(cp, id) {
			c.errorf(diag.UnknownReference, w+"."+k, "%q is not a general of campaign.generals", id)
		}
		if f != "troops" {
			c.errorf(diag.Shape, w+"."+k, "set can change generals.<id>.troops only (not %q)", f)
		}
		c.needsVersion(w+"."+k, "set "+kind, SchemaVersion5)
		return true
	case "general":
		if rest != "troops" {
			c.errorf(diag.Shape, w+"."+k, "set can change general.troops only (not %q)", rest)
		}
		c.needsVersion(w+"."+k, "set "+kind, SchemaVersion5)
		return true
	default:
		return false
	}
	if field == "owner" && e.Set() {
		if v, err := e.Eval(nil); err == nil {
			if f, ok := v.(string); ok && f != "" && c.db.Factions[f] == nil {
				c.errorf(diag.UnknownReference, w+"."+k, "unknown faction %q", f)
			}
		}
	}
	if field != "owner" && field != "troops" && !slices.Contains(ValueNames(c.db), field) {
		c.errorf(diag.Shape, w+"."+k, "set can change a region's owner, troops and values (%s), not %q", strings.Join(ValueNames(c.db), ", "), field)
	}
	c.needsVersion(w+"."+k, "set "+kind, SchemaVersion5)
	return true
}

func hasGeneral(cp *Campaign, id string) bool { _, ok := cp.Generals[id]; return ok }

// MustExpr is the expression of src, which must parse (the engine's own expressions).
func MustExpr(src string) Expr {
	p, err := expr.Parse(src)
	if err != nil {
		panic(fmt.Sprintf("gamedata: %q: %v", src, err))
	}
	return Expr{p}
}

// campaignMapWarnings warns when the campaign's map and the regions disagree: a region
// without a point named like it (the screen cannot show it), or a road between two regions
// that do not border each other, or two neighbours without a road.
func (c *checker) campaignMapWarnings(a *Atlas) {
	db := c.db
	w := "locations." + db.Game.Campaign.Location + ".map"
	for _, id := range RegionIDs(db) {
		if a.Points[id] == nil {
			c.warnf(diag.Shape, w+".points", "region %s has no point on the campaign's map (name a point %s)", id, id)
		}
	}
	road := map[[2]string]bool{}
	for i, r := range a.Roads {
		if len(r.Between) != 2 || db.Regions[r.Between[0]] == nil || db.Regions[r.Between[1]] == nil {
			continue
		}
		x, y := r.Between[0], r.Between[1]
		road[[2]string{x, y}], road[[2]string{y, x}] = true, true
		if !slices.Contains(db.Regions[x].Next, y) && !slices.Contains(db.Regions[y].Next, x) {
			c.warnf(diag.Shape, fmt.Sprintf("%s.roads[%d]", w, i), "a road joins %s and %s, which do not border each other (next)", x, y)
		}
	}
	for _, id := range RegionIDs(db) {
		for _, n := range db.Regions[id].Next {
			if id < n && a.Points[id] != nil && a.Points[n] != nil && !road[[2]string{id, n}] {
				c.warnf(diag.Shape, "regions."+id+".next", "%s borders %s, but the campaign's map has no road between them", id, n)
			}
		}
	}
}

// factionWords reports a faction written in an expression that is none: compared with an
// owner (region.<id>.owner, target.owner, s, t or b's owner, a.faction, campaign.player) or
// given to next_to. Only a game with factions is checked.
func (c *checker) factionWords(where string, e Expr) {
	if !e.Set() || len(c.db.Factions) == 0 {
		return
	}
	known := func(f string) bool { return f == "" || c.db.Factions[f] != nil }
	for _, cmp := range e.Comparisons() {
		r := cmp.Ref
		last := ""
		if n := len(r.Path); n > 0 {
			last = r.Path[n-1].Name
		}
		owner := last == "owner" && slices.Contains([]string{"region", "target", "s", "t", "b"}, r.Name) ||
			r.Name == "a" && last == "faction" || r.Name == "campaign" && last == "player"
		if owner && !known(cmp.Text) {
			c.errorf(diag.UnknownReference, where, "%q: unknown faction %q (factions: %s)", e.String(), cmp.Text, strings.Join(FactionIDs(c.db), ", "))
		}
	}
	for _, args := range e.StringArgs("next_to") {
		if len(args) == 2 {
			if args[0] != "" && c.db.Regions[args[0]] == nil {
				c.errorf(diag.UnknownReference, where, "%q: unknown region %q", e.String(), args[0])
			}
			if args[1] != "" && !known(args[1]) {
				c.errorf(diag.UnknownReference, where, "%q: unknown faction %q (factions: %s)", e.String(), args[1], strings.Join(FactionIDs(c.db), ", "))
			}
		}
	}
}

// callLoops reports the events that call themselves on every run: a cycle of calls each
// written at the top of an event's do (not inside if, a choice or a battle's ends). Such
// an event never ends; the runner stops it at its step limit. A call in a branch may end
// and is not reported.
func (c *checker) callLoops() {
	db := c.db
	calls := map[string][]string{}
	for id, ev := range db.Events {
		for _, cmd := range ev.Do {
			if cmd.Call != "" && db.Events[cmd.Call] != nil {
				calls[id] = append(calls[id], cmd.Call)
			}
		}
	}
	reported := map[string]bool{}
	for _, start := range slices.Sorted(maps.Keys(calls)) {
		// a walk from start along the calls that comes back to start
		seen := map[string]bool{}
		var path []string
		var visit func(id string) bool
		visit = func(id string) bool {
			for _, n := range calls[id] {
				if n == start {
					path = append(path, n)
					return true
				}
				if seen[n] {
					continue
				}
				seen[n] = true
				path = append(path, n)
				if visit(n) {
					return true
				}
				path = path[:len(path)-1]
			}
			return false
		}
		if !visit(start) || reported[start] {
			continue
		}
		for _, id := range path {
			reported[id] = true
		}
		c.errorf(diag.Shape, "events."+start+".do", "event %s calls itself on every run (%s -> %s): it never ends; put the call under an if that stops it", start, start, strings.Join(path, " -> "))
	}
}
