package event

import (
	"fmt"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/campaign"
	lang "github.com/nao1215/gozen/internal/text"
)

// order gives an order of the campaign (ADR 0030): it asks for the general and then the
// target when the command does not name them (each choice ends with Cancel), gives it,
// shows how a march went, and runs the order's do with them bound.
func (r *Runner) order(c *data.Command) Request {
	oc := c.Order
	o := campaign.Order(r.db, oc.ID)
	if o == nil {
		r.logf(fmt.Errorf("order %s: %w or no such order", oc.ID, campaign.ErrNoCampaign))
		return nil
	}
	if why := campaign.Open(r.db, r.st, o); why != "" {
		r.notes = append(r.notes, why)
		return nil
	}
	again := func(general, target string) {
		next := *oc
		next.General, next.Target = general, target
		r.push([]data.Command{{Order: &next}})
	}
	if o.General != "" && oc.General == "" {
		ids := campaign.Generals(r.db, r.st, o)
		if len(ids) == 0 {
			r.notes = append(r.notes, lang.T("率いる将がいない"))
			return nil
		}
		return r.ask(c, lang.T("誰が率いる？"), campaign.Choices(ids, func(id string) string { return campaign.GeneralLabel(r.db, r.st, id) }), func(n int) {
			if n < len(ids) {
				again(ids[n], oc.Target)
			}
		})
	}
	if o.Target != "" && oc.Target == "" {
		ids := campaign.Targets(r.db, r.st, o)
		if len(ids) == 0 {
			r.notes = append(r.notes, lang.T("向かえる地域がない"))
			return nil
		}
		return r.ask(c, lang.T("どこへ？"), campaign.Choices(ids, func(id string) string { return campaign.TargetLabel(r.db, r.st, id) }), func(n int) {
			if n < len(ids) {
				again(oc.General, ids[n])
			}
		})
	}
	r.notes = append(r.notes, campaign.Give(r.db, r.st, o, oc.General, oc.Target)...)
	r.push(o.Do)
	return nil
}

// ask is a choice the engine makes: labels, and pick to run with the option taken.
func (r *Runner) ask(c *data.Command, prompt string, labels []string, pick func(int)) Request {
	r.options = make([]data.ChoiceOption, len(labels))
	for i, l := range labels {
		r.options[i] = data.ChoiceOption{Label: l}
	}
	r.pick, r.asked = pick, c
	return Choice{Prompt: prompt, Options: labels}
}

// endTurn ends the campaign's turn: what the other factions did, then the on_turn event.
func (r *Runner) endTurn() {
	if r.db.Game.Campaign == nil {
		r.logf(fmt.Errorf("end_turn: %w", campaign.ErrNoCampaign))
		return
	}
	lines, cmds := campaign.EndTurn(r.db, r.st)
	r.notes = append(r.notes, lines...)
	r.push(cmds)
}
