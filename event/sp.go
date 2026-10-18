package event

import (
	"fmt"
	"slices"
	"strings"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/rpg"
	lang "github.com/nao1215/gozen/text"
)

// 余った SP の振り方の勧め（sp_suggest・sp_apply・sp_decline）。石屋の勧め（stones.go）と同じ形で、
// 計画は var.<名前>_* に書き、台詞と選択肢はゲームの YAML が書く。
//
// sp_suggest（仲間ごとに、おまかせ＝rpg の SpendSP と同じ振り方の計画。振らない）:
//
//	var.<名前>        振る人数（0 なら勧めない。SP が無い・断った後で SP が変わっていない・自分で振る設定）
//	var.<名前>_pages  人ごとの文の数、var.<名前>_page1〜 はその文
//	                  （「Scout　Guard Lv2→3、Focus Lv1 を覚える」の行と、職の sp_pitch）
//	var.<名前>_no     断ったときの仲間の SP（sp_decline が書く。消すと次の sp_suggest はまた勧める）

// SPHost is a host that knows whether the player spends the skill points by hand: when
// ManualSP says so, sp_suggest suggests nothing.
type SPHost interface {
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
			var learnt []string // 一人 1 行に まとめる（振り直しの 後は 多い）
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
	if h, ok := r.host.(SPHost); ok && h.ManualSP() {
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

// SPPage is the page of a member's plan: 「名前　ノード Lv1→2、ノード Lv2、技 Lv1・技 Lv2 を覚える」
// (the nodes raised, then those new with a skill) and the pitch of their class on the next line.
func SPPage(db *data.DB, a *rpg.Actor, plan []string) string {
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
		fmt.Fprintf(&b, "%s=%d;", a.ID, a.SP) //nolint:errcheck // strings.Builder writes always succeed.
	}
	return b.String()
}
