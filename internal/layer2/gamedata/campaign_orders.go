package gamedata

import (
	"fmt"
	"strings"

	"github.com/nao1215/gozen/internal/diag"
)

// OrderCmd is the order command: the id of an order, and the general and the target region
// when they are not asked for. Written as the id alone or as a map.
type OrderCmd struct {
	ID      string `yaml:"id"`
	General string `yaml:"general"`
	Target  string `yaml:"target"`
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
		when := fmt.Sprintf("campaign.actions_left >= %d", o.CostOf())
		if o.When.Set() {
			when += " && (" + o.When.String() + ")"
		}
		acts = append(acts, Action{ID: "order_" + id, Label: label, When: MustExpr(when), Do: []Command{{Order: &OrderCmd{ID: id}}}})
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
	if o.Target != "" {
		_, ok := c.db.Regions[o.Target]
		c.ref(w+".order.target", "region", o.Target, ok)
	}
}
