package event

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nao1215/gozen/expr"
	"github.com/nao1215/gozen/internal/layer1/optional"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/rpg"
	lang "github.com/nao1215/gozen/text"
)

func (r *Runner) stones(verb string, c *data.Command, env expr.Env) Request {
	switch verb {
	case "stone_suggest":
		r.suggest(c.StoneSuggest)
	case "stone_apply":
		best, score, now := r.st.BestStones()
		if score <= now {
			return nil
		}
		before := map[string][]string{}
		for _, a := range r.st.Members() {
			before[a.ID] = a.Skills()
		}
		r.st.ApplyStones(best)
		for _, a := range r.st.Members() {
			for _, sk := range a.Skills() {
				if !slices.Contains(before[a.ID], sk) {
					r.notes = append(r.notes, fmt.Sprintf(lang.T("%sは %sを おぼえた！"), a.Name(), r.db.Skills[sk].Name))
				}
			}
		}
	case "stone_choose":
		name := c.StoneChoose
		offers := r.st.StoneOffers()
		if len(offers) == 0 {
			return nil
		}
		labels := make([]string, len(offers))
		r.options = make([]data.ChoiceOption, len(offers))
		for i, o := range offers {
			ss := r.db.StoneSets[o.Set]
			labels[i] = fmt.Sprintf(lang.T("%s（%s）"), ss.Name, r.stoneNames(o.Set))
			r.options[i] = data.ChoiceOption{Label: labels[i]}
		}
		r.pick = func(n int) { r.fillOffer(name, offers[n].Set, r.st.Actors[offers[n].Actor]) }
		r.asked = c
		return Choice{Prompt: data.Render(c.Prompt, env), Options: labels}
	case "stone_wearer":
		name := c.StoneWearer
		set := optional.Value[string](r.st.Vars[name+"_set"])
		ws := r.st.SetWearers(set)
		if len(ws) == 0 {
			return nil
		}
		labels := make([]string, len(ws))
		r.options = make([]data.ChoiceOption, len(ws))
		for i, a := range ws {
			labels[i] = a.Name()
			r.options[i] = data.ChoiceOption{Label: labels[i]}
		}
		r.pick = func(n int) {
			if id, ok := r.st.SetFor(set, ws[n]); ok {
				r.fillOffer(name, id, ws[n])
			}
		}
		r.asked = c
		return Choice{Prompt: data.Render(c.Prompt, env), Options: labels}
	case "stone_decline":
		r.setVar(c.StoneDecline+"_no", stoneHoldingAll(r.st))
	case "equip_stones":
		who, err := c.EquipStones.Actor.Eval(env)
		r.logf(err)
		set, err := c.EquipStones.Set.Eval(env)
		r.logf(err)
		a := r.st.Actors[fmt.Sprint(who)]
		id := fmt.Sprint(set)
		if a == nil || r.db.StoneSets[id] == nil {
			r.logf(fmt.Errorf("equip_stones: no actor %v or stone set %v", who, set))
			return nil
		}
		if _, _, ok := r.st.StonePlanFor(a, id); !ok { // 書かれた並びが組めなければ、同じ名前の組める並び
			if alt, ok := r.st.SetFor(id, a); ok {
				id = alt
			}
		}
		before := a.Skills()
		r.logf(r.st.EquipSet(a, id))
		if sk := r.db.StoneSets[id].Skill; sk != "" && !slices.Contains(before, sk) && slices.Contains(a.Skills(), sk) {
			r.notes = append(r.notes, fmt.Sprintf(lang.T("%sは %sを おぼえた！"), a.Name(), r.db.Skills[sk].Name))
		}
	}
	return nil
}

// suggest writes the strongest way to wear the stones to var.<name>_* (see the top).
func (r *Runner) suggest(name string) {
	r.setVar(name+"_sets", float64(len(r.st.StoneOffers())))
	r.setVar(name, 0.0)
	r.setVar(name+"_pages", 0.0)
	if v, ok := r.st.Vars[name+"_no"].(string); ok && v == stoneHoldingAll(r.st) {
		return
	}
	best, score, now := r.st.BestStones()
	if score <= now {
		return
	}
	cur := r.st.StonesNow()
	n := 0
	for _, a := range r.st.Members() {
		if slices.Equal(cur[a.ID], best[a.ID]) {
			continue
		}
		n++
		line := fmt.Sprintf(lang.T("%s　%s → %s"), a.Name(), r.shortNames(cur[a.ID]), r.shortNames(best[a.ID]))
		var pitches []string
		for _, id := range rpg.SetsOf(r.db, a.ID, best[a.ID]) {
			if p := r.pitch(id); p != "" && !slices.Contains(pitches, p) { // 同じひとことは一度だけ
				pitches = append(pitches, p)
			}
		}
		if len(pitches) == 0 {
			pitches = append(pitches, lang.T("空いた枠には、強い石を回しておく。"))
		}
		r.setVar(fmt.Sprintf("%s_page%d", name, n), line+"\n"+strings.Join(pitches, ""))
	}
	r.setVar(name, float64(n))
	r.setVar(name+"_pages", float64(n))
}

// pitch は同じ系列のデータ順で最初にある勧めのひとことを返す。
func (r *Runner) pitch(set string) string {
	for _, id := range r.st.SetFamily(set) {
		if p := r.db.StoneSets[id].Pitch; p != "" {
			return p
		}
	}
	return ""
}

// shortNames are the short names of stones joined with ・ (なし for none).
func (r *Runner) shortNames(stones []string) string {
	if len(stones) == 0 {
		return lang.T("なし")
	}
	var names []string
	for _, st := range stones {
		names = append(names, r.db.Items[st].ShortName())
	}
	return strings.Join(names, lang.T("・"))
}

func (r *Runner) setVar(name string, v any) { r.logf(r.st.SetPath("var."+name, v)) }

// stoneNames are the short names of a set's stones: 「白と蒼」 for two, 「紅・蒼・翠」 for more.
func (r *Runner) stoneNames(set string) string {
	var names []string
	for _, st := range r.db.StoneSets[set].Stones {
		names = append(names, r.db.Items[st].ShortName())
	}
	if len(names) == 2 {
		return names[0] + lang.T("と") + names[1]
	}
	return strings.Join(names, lang.T("・"))
}

// fillOffer writes the offer of the set on a to var.<name>_*.
func (r *Runner) fillOffer(name, set string, a *rpg.Actor) {
	ss := r.db.StoneSets[set]
	skill := ""
	if sk := r.db.Skills[ss.Skill]; sk != nil {
		skill = sk.Name
	}
	var stats []string
	for _, st := range data.StatNames {
		if v := ss.Stats[st]; v != 0 {
			stats = append(stats, fmt.Sprintf("%s%+d", lang.T(data.StatLabels[st]), v))
		}
	}
	r.setVar(name+"_set", set)
	r.setVar(name+"_name", ss.Name)
	r.setVar(name+"_stones", r.stoneNames(set))
	r.setVar(name+"_skill", skill)
	r.setVar(name+"_stats", strings.Join(stats, lang.T("、")))
	r.setVar(name+"_who", a.ID)
	r.setVar(name+"_who_name", a.Name())
	r.setVar(name+"_wearers", float64(len(r.st.SetWearers(set))))
}

func stoneHoldingAll(s *rpg.State) string {
	pool := s.StonePool()
	var b strings.Builder
	for _, id := range s.DB.Ordered("items") {
		if pool[id] > 0 {
			fmt.Fprintf(&b, "%s=%d;", id, pool[id])
		}
	}
	return b.String()
}
