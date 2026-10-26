package gamedata

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
)

// OrderCmd is the order command: the id of an order, and the general and the target region
// when they are not asked for. Written as the id alone or as a map.
type OrderCmd struct {
	ID      string `yaml:"id"`
	General string `yaml:"general"`
	// With are the generals who march with General (an order with generals above 1).
	With   []string `yaml:"with"`
	Target string   `yaml:"target"`
	// Tactics are the tactics of the generals marching, by general (clash.tactics; each
	// general's own when missing).
	Tactics map[string]string `yaml:"tactics"`
	// asked: the order has asked who else marches and the answer was to march now.
	asked bool
}

// Asked reports whether the order asked who else marches and was told to march now.
func (o *OrderCmd) Asked() bool { return o.asked }

// MarchNow is the order with who else marches settled.
func (o OrderCmd) MarchNow() *OrderCmd {
	o.asked = true
	o.With = append([]string(nil), o.With...)
	o.Tactics = maps.Clone(o.Tactics)
	return &o
}

// UnmarshalYAML reads the id alone, or the map.
func (o *OrderCmd) UnmarshalYAML(unmarshal func(any) error) error {
	var id string
	if err := unmarshal(&id); err == nil {
		o.ID = id
		return nil
	}
	type plain OrderCmd
	return unmarshal((*plain)(o))
}

// campaignActions puts the orders and the end of the turn in the menu of the campaign's
// place: an action per order (listed while it holds and the turn has the actions it
// costs), before the place's own actions, and the end of the turn after them.
func campaignActions(db *DB) {
	c := db.Game.Campaign
	if c == nil {
		return
	}
	l := db.Locations[c.Location]
	if l == nil {
		return
	}
	var acts []Action
	for _, id := range c.OrderIDs() {
		o := c.Orders[id]
		if o == nil {
			continue
		}
		label := o.Label
		if label == "" {
			label = id
		}
		when := fmt.Sprintf("orders.%s.open", id)
		if o.When.Set() {
			when += " && (" + o.When.String() + ")"
		}
		acts = append(acts, Action{ID: "order_" + id, Label: label, Group: o.Group, When: MustExpr(when), Do: []Command{{Order: &OrderCmd{ID: id}}}})
	}
	end := c.EndTurn
	if end == "" {
		end = db.Game.word("ターンを終える")
	}
	l.Actions = append(append(acts, l.Actions...), Action{ID: "end_turn", Label: end, Do: []Command{{EndTurn: true}}})
}

// campaignCommand checks the commands of a campaign: order, end_turn and game_over.
func (c *checker) campaignCommand(w, verb string, cmd *Command) {
	if !c.needsVersion(w+"."+verb, verb, SchemaVersion5) {
		return
	}
	cp := c.db.Game.Campaign
	if verb == "game_over" {
		return
	}
	if cp == nil {
		c.errorf(diag.Shape, w+"."+verb, "%s needs game.yaml's campaign", verb)
		return
	}
	if verb != "order" {
		return
	}
	o := cmd.Order
	if _, ok := cp.Orders[o.ID]; !ok {
		c.errorf(diag.UnknownReference, w+".order", "unknown order %q (campaign.orders has %s)", o.ID, strings.Join(cp.OrderIDs(), ", "))
	}
	if o.General != "" {
		if _, ok := cp.Generals[o.General]; !ok {
			c.errorf(diag.UnknownReference, w+".order.general", "%q is not a general of campaign.generals", o.General)
		}
	}
	for _, g := range o.With {
		if _, ok := cp.Generals[g]; !ok {
			c.errorf(diag.UnknownReference, w+".order.with", "%q is not a general of campaign.generals", g)
		}
	}
	for _, g := range slices.Sorted(maps.Keys(o.Tactics)) {
		if g != o.General && !slices.Contains(o.With, g) {
			c.errorf(diag.UnknownReference, w+".order.tactics."+g, "%q is not marching (general or with)", g)
		}
		if _, ok := cp.Clash.DeclaredTactics()[o.Tactics[g]]; !ok {
			c.errorf(diag.UnknownReference, w+".order.tactics."+g, "unknown tactic %q (clash.tactics has %s)", o.Tactics[g], strings.Join(cp.Clash.TacticIDs(), ", "))
		}
	}
	if o.Target != "" {
		_, ok := c.db.Regions[o.Target]
		c.ref(w+".order.target", "region", o.Target, ok)
	}
}
