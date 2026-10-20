package event

import (
	"fmt"
	"slices"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// Advice on how to spend leftover SP (sp_suggest, sp_apply, sp_decline). Same shape as the stone shop advice (stones.go);
// plans are written to var.<name>_*, and the dialogue and choices are written in the game's YAML.
//
// sp_suggest (per member, the auto plan = the same allocation as rpg's SpendSP; does not spend):
//
//	var.<name>        number of members to allocate for (0 means no advice: no SP, SP unchanged since declining, or set to allocate manually)
//	var.<name>_pages  number of text pages per member; var.<name>_page1... hold those pages
//	                  (a line like "Scout　Guard Lv2→3、Focus Lv1 を覚える" plus the job's sp_pitch)
//	var.<name>_no     the member's SP when declined (written by sp_decline; clearing it makes the next sp_suggest advise again)

// spHost is a host that knows whether the player spends the skill points by hand: when
// ManualSP says so, sp_suggest suggests nothing.
type spHost interface {
	ManualSP() bool
}

func (r *Runner) sp(verb string, c *data.Command) {
	switch verb {
	case "sp_suggest":
		r.suggestSP(c.SPSuggest)
	case "sp_apply":
		for _, a := range r.st.Members() {
			before := a.Skills()
			a.ApplySP(a.PlanSP())
			var learnt []string // one line per member (many after a respec)
			for _, sk := range a.Skills() {
				if !slices.Contains(before, sk) {
					learnt = append(learnt, r.db.Skills[sk].Name)
				}
			}
			if len(learnt) > 0 {
				r.notes = append(r.notes, fmt.Sprintf(lang.T("%sは %sを おぼえた！"), a.Name(), strings.Join(learnt, lang.T("・"))))
			}
		}
	case "sp_decline":
		r.setVar(c.SPDecline+"_no", spKey(r.st.Members()))
	}
}

// suggestSP writes the plan of every member with points left to var.<name>_* (see the top).
func (r *Runner) suggestSP(name string) {
	r.setVar(name, 0.0)
	r.setVar(name+"_pages", 0.0)
	if h, ok := r.host.(spHost); ok && h.ManualSP() {
		return
	}
	if v, ok := r.st.Vars[name+"_no"].(string); ok && v == spKey(r.st.Members()) {
		return
	}
	n := 0
	for _, a := range r.st.Members() {
		plan := a.PlanSP()
		if len(plan) == 0 {
			continue
		}
		n++
		r.setVar(fmt.Sprintf("%s_page%d", name, n), SPPage(r.db, a, plan))
	}
	r.setVar(name, float64(n))
	r.setVar(name+"_pages", float64(n))
}

// SPPage is the page of a member's plan, e.g. 「名前　ノード Lv1→2、ノード Lv2、技 Lv1・技 Lv2 を覚える」.
// (the nodes raised, then those new with a skill) and the pitch of their class on the next line.
func SPPage(_ *data.DB, a *rpg.Actor, plan []string) string {
	var steps, learn []string
	for _, s := range a.PlanSteps(plan) {
		switch n := a.Node(s.Node); {
		case s.From == 0 && n != nil && n.Skill != "":
			learn = append(learn, fmt.Sprintf(lang.T("%s Lv%d"), a.NodeName(s.Node), s.To))
		case s.From == 0:
			steps = append(steps, fmt.Sprintf(lang.T("%s Lv%d"), a.NodeName(s.Node), s.To))
		default:
			steps = append(steps, fmt.Sprintf(lang.T("%s Lv%d→%d"), a.NodeName(s.Node), s.From, s.To))
		}
	}
	if len(learn) > 0 {
		steps = append(steps, fmt.Sprintf(lang.T("%s を覚える"), strings.Join(learn, lang.T("・"))))
	}
	page := fmt.Sprintf(lang.T("%s　%s"), a.Name(), strings.Join(steps, lang.T("、")))
	if c := a.Class(); c != nil && c.SPPitch != "" {
		page += "\n" + c.SPPitch
	}
	return page
}

func spKey(actors []*rpg.Actor) string {
	var b strings.Builder
	for _, a := range actors {
		fmt.Fprintf(&b, "%s=%d;", a.ID, a.SP)
	}
	return b.String()
}
