package play

import (
	"fmt"
	"image"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/battle"
	lang "github.com/nao1215/gozen/text"
)

const tacticHistoryLimit = 128

func cloneTacticDecision(d battle.Decision) battle.Decision {
	d.Rules = slices.Clone(d.Rules)
	if d.Target != nil {
		d.Target = new(*d.Target)
	}
	for i := range d.Rules {
		if d.Rules[i].Target != nil {
			d.Rules[i].Target = new(*d.Rules[i].Target)
		}
	}
	return d
}

func (s *battleScene) recordDecisions() {
	trace := s.b.OnDecision
	s.b.OnDecision = func(d battle.Decision) {
		if len(s.decisions) == tacticHistoryLimit {
			s.decisions = slices.Delete(s.decisions, 0, 1)
		}
		s.decisions = append(s.decisions, cloneTacticDecision(d))
		if trace != nil {
			trace(d)
		}
	}
}

func (s *battleScene) reasonButton(g *Game) image.Rectangle {

	sw, _ := g.screen()
	x := int(sw/2) - 115
	return image.Rect(x, 164, x+230, 210)
}

type tacticReasons struct {
	decisions      []battle.Decision
	history, rules *list
	focus          int
}

func tacticUnitName(g *Game, u battle.DecisionUnit) string {
	if u.Enemy {
		if d := g.db.Enemies[u.ID]; d != nil {
			return fmt.Sprintf("%s %d", d.Name, u.Index+1)
		}
	} else if a := g.st.Actors[u.ID]; a != nil {
		return a.Name()
	}
	return u.ID
}

func newTacticReasons(g *Game, decisions []battle.Decision) *tacticReasons {
	s := &tacticReasons{}
	sw, _ := g.screen()
	s.history = newList(nil, 24, 108, 390, 9)
	s.rules = newList(nil, 434, 108, sw-458, 9)
	s.history.size, s.rules.size = sizeS, sizeS
	for i := len(decisions) - 1; i >= 0; i-- {
		d := cloneTacticDecision(decisions[i])
		s.decisions = append(s.decisions, d)
		timing := lang.T("行動時")
		if d.Timing == battle.DecisionTurnStart {
			timing = lang.T("ターン開始時")
		}
		if d.Timing == battle.DecisionDirect {
			timing = lang.T("直接判断")
		}
		s.history.items = append(s.history.items, listItem{label: fmt.Sprintf(lang.T("%dターン %s / %s"), d.Turn, tacticUnitName(g, d.User), timing)})
	}
	s.refresh(g)
	return s
}

func (s *tacticReasons) overlay() bool { return true }

func tacticReason(r battle.RuleEvaluation) string {
	var text string
	switch r.Reason {
	case battle.RuleApplicable:
		text = lang.T("条件成立")
	case battle.RuleOnceUsed:
		text = lang.T("一度だけの行を使用済み")
	case battle.RuleConditionFalse:
		text = lang.T("条件不成立")
	case battle.RuleConditionError:
		text = lang.T("条件式のエラー")
	case battle.RuleChanceFailed:
		text = lang.T("抽選不成立")
	case battle.RuleChanceError:
		text = lang.T("抽選式のエラー")
	case battle.RuleMPShortage:
		text = lang.T("MP 不足")
	case battle.RuleSkillUnlearned:
		text = lang.T("未習得の技")
	case battle.RuleItemShortage:
		text = lang.T("品の所持数不足")
	case battle.RuleEnemyItem:
		text = lang.T("敵は品を使えません")
	case battle.RuleInvalidAction:
		text = lang.T("行動が見つかりません")
	case battle.RuleNoTarget:
		text = lang.T("対象がいません")
	case battle.RuleDisabled:
		text = lang.T("OFF の行")
	case battle.RuleInvalidTactic:
		text = lang.T("無効な作戦の行")
	default:
		text = string(r.Reason)
	}
	switch r.Selection {
	case "selected":
		text += " / " + lang.T("この行を選択")
	case "later_rule":
		text += " / " + lang.T("先に成立した行を優先")
	case "lower_priority":
		text += " / " + lang.T("ほかの行の優先度が高い")
	case "weight_not_selected":
		text += " / " + lang.T("重みの抽選で別の行を選択")
	}
	return text
}

func tacticDecisionAction(g *Game, skill, item string) string {
	if sk := g.db.Skills[skill]; sk != nil {
		return fmt.Sprintf(lang.T("%s（必要 MP %d）"), sk.Name, sk.MP)
	}
	if it := g.db.Items[item]; it != nil {
		return fmt.Sprintf(lang.T("%s（必要 1個）"), it.Name)
	}
	return skill + item
}

func (s *tacticReasons) refresh(g *Game) {
	s.rules.items = nil
	if len(s.decisions) == 0 {
		return
	}
	for _, r := range s.decisions[s.history.sel].Rules {
		s.rules.items = append(s.rules.items, listItem{label: fmt.Sprintf("%d  %s / %s", r.Index+1, tacticDecisionAction(g, r.Skill, r.Item), tacticReason(r))})
	}
	s.rules.sel, s.rules.top = 0, 0
}

func (s *tacticReasons) description(g *Game) string {
	if len(s.decisions) == 0 {
		return lang.T("まだ自動判断の記録がありません。戦闘を進めると実際の判断を表示します。")
	}
	d := s.decisions[s.history.sel]
	text := s.history.items[s.history.sel].label + "\n" + tacticDecisionAction(g, d.Skill, d.Item)
	if d.Committed {
		text += " / " + lang.T("採用した判断")
	} else {
		text += " / " + lang.T("仮の判断。行動時に選び直します")
	}
	if d.Selected < 0 {
		text += " / " + lang.T("成立行なし。通常攻撃")
	}
	if d.Target != nil {
		text += " / " + tacticUnitName(g, *d.Target)
	}
	if len(d.Rules) > 0 {
		r := d.Rules[s.rules.sel]
		text += fmt.Sprintf(lang.T("\n%d行目 %s / %s"), r.Index+1, tacticDecisionAction(g, r.Skill, r.Item), tacticReason(r))
		if r.Target != nil {
			text += " / " + tacticUnitName(g, *r.Target)
		}
		if r.Detail != "" {
			text += "\n" + r.Detail
		}
	}
	return text
}

func (s *tacticReasons) update(g *Game) {
	if g.in.cancel() || g.in.pressed[btnLog] {
		g.pop()
		return
	}
	lists := []*list{s.history, s.rules}
	if g.in.click {
		for f, l := range lists {
			for i := l.top; i < min(len(l.items), l.top+l.rows); i++ {
				if g.in.mouse.In(l.rowRect(i)) {
					s.focus = f
				}
			}
		}
	}
	if g.in.left() || g.in.right() {
		s.focus = 1 - s.focus
		return
	}
	old := s.history.sel
	chosen, _ := lists[s.focus].update(g)
	if old != s.history.sel {
		s.refresh(g)
	}
	if chosen {
		if s.focus == 0 {
			s.focus = 1
		} else {
			g.push(newTacticText(g, s.description(g)))
		}
	}
}

func (s *tacticReasons) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0xf8))
	g.text(dst, lang.T("実際の判断理由（閲覧中は戦闘を停止）"), 30, 25, sizeL, g.th.text)
	g.text(dst, lang.T("最新128件・新しい順"), 30, 77, sizeS, g.th.border)
	g.text(dst, lang.T("評価した行・必要量・結果"), 446, 77, sizeS, g.th.border)
	s.history.draw(g, dst, s.focus == 0)
	s.rules.draw(g, dst, s.focus == 1)
	g.window(dst, 24, 552, sw-48, 100)
	g.drawLinesFit(dst, s.description(g), 44, 565, sw-88, sizeS, 3, g.th.text)
	g.drawLinesFit(dst, lang.T("← → 欄を移動　↑ ↓ 選択　行を決定で全文　B / L1・取消で戦闘へ戻る"), 30, 674, sw-60, sizeS, 1, g.th.border)
}
