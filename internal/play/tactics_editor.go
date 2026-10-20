package play

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

const (
	tacticCondition = iota
	tacticPercent
	tacticTarget
	tacticAction
	tacticRowAccept
	tacticRowCancel
	tacticRowDetail
)

type tacticEditor struct {
	parent          *tacticsScreen
	actor           string
	index           int
	adding          bool
	row             data.TacticRow
	fields, options *list
	choosing        bool
}

func newTacticEditor(g *Game, parent *tacticsScreen, adding bool) *tacticEditor {
	e := &tacticEditor{parent: parent, actor: parent.actorID(), index: parent.rows.sel, adding: adding}
	if adding {
		e.row = e.defaultRow(g)
	} else {
		e.row = parent.currentRows(g)[e.index]
	}
	if e.row.Percent != nil {
		e.row.Percent = new(*e.row.Percent)
	}
	sw, _ := g.screen()
	e.fields = newList(nil, 24, 108, 390, 7)
	e.fields.size = sizeS
	e.options = newList(nil, 434, 108, sw-458, 9)
	e.options.size = sizeS
	e.refresh(g)
	return e
}

func (e *tacticEditor) overlay() bool { return false }

func catalogIDs[T any](m map[string]T) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (e *tacticEditor) defaultRow(g *Game) data.TacticRow {
	if g.db.Game.Battle.SimpleTactics {
		for _, c := range catalogIDs(g.db.TacticConditions) {
			if g.db.TacticConditions[c].Kind != "always" {
				continue
			}
			for _, a := range catalogIDs(g.db.TacticActions) {
				if g.db.TacticActions[a].Use == g.db.Game.Battle.Attack {
					return data.TacticRow{Enabled: true, Condition: c, Action: a}
				}
			}
		}
	}
	for _, c := range catalogIDs(g.db.TacticConditions) {
		if g.db.TacticConditions[c].Kind != "always" {
			continue
		}
		for _, a := range catalogIDs(g.db.TacticActions) {
			for _, target := range catalogIDs(g.db.TacticTargets) {
				row := data.TacticRow{Enabled: true, Condition: c, Target: target, Action: a}
				if _, errs := g.db.CompileTactic(row, g.st.Actors[e.actor].Skills()); len(errs) == 0 {
					return row
				}
			}
		}
	}
	return data.TacticRow{Enabled: true}
}

func tacticCost(g *Game, action string) string {
	a := g.db.TacticActions[action]
	if a != nil {
		if sk := g.db.Skills[a.Use]; sk != nil {
			return fmt.Sprintf(lang.T("必要 MP %d"), sk.MP)
		}
		if it := g.db.Items[a.Item]; it != nil {
			return fmt.Sprintf(lang.T("%s 必要 1個（所持 %d個）"), it.Name, g.st.Items[a.Item])
		}
	}
	return lang.T("行動が見つかりません")
}

func tacticThreshold(db *data.DB, id string) bool {
	c := db.TacticConditions[id]
	return c != nil && (strings.HasPrefix(c.Kind, "hp_") || strings.HasPrefix(c.Kind, "mp_"))
}

func (e *tacticEditor) errors(g *Game) []error {
	row := e.row

	row.Enabled = true
	_, errs := g.db.CompileTactic(row, g.st.Actors[e.actor].Skills())
	return errs
}

func (e *tacticEditor) description(g *Game) string {
	text := e.parent.rowText(g, e.row) + "\n" + tacticCost(g, e.row.Action)
	for _, err := range e.errors(g) {
		if g.db.Game.Battle.SimpleTactics {
			text += "\n" + lang.T("今使える行動を選んでください。")
			break
		}
		text += "\n" + err.Error()
	}
	if e.choosing && e.field(g) == tacticAction && len(e.options.items) > 0 {
		option := e.options.items[e.options.sel]
		text += "\n" + option.label + " / " + tacticCost(g, option.id)
		if option.disabled {
			text += " / " + lang.T("未習得のため選べません")
		}
	}
	return text
}

func (e *tacticEditor) refresh(g *Game) {
	p := lang.T("この条件では使いません")
	if e.row.Percent != nil {
		p = fmt.Sprintf("%d%%", *e.row.Percent)
	}
	e.fields.items = []listItem{
		{label: lang.T("条件を選ぶ")},
		{label: lang.T("HP / MP の閾値") + "  " + p, disabled: !tacticThreshold(g.db, e.row.Condition)},
		{label: lang.T("対象を選ぶ")}, {label: lang.T("行動を選ぶ")},
		{label: lang.T("この行を確定")}, {label: lang.T("この行の変更を取消")},
		{label: lang.T("説明とエラーの全文")},
	}
	if g.db.Game.Battle.SimpleTactics {
		e.fields.items = []listItem{{label: lang.T("いつ")}, {label: lang.T("何をする")}, {label: lang.T("この行を確定")}, {label: lang.T("この行の変更を取消")}, {label: lang.T("説明の全文")}}
	}
	e.options.items = nil
	selected := ""
	switch e.field(g) {
	case tacticCondition:
		selected = e.row.Condition
		for _, id := range catalogIDs(g.db.TacticConditions) {
			e.options.items = append(e.options.items, listItem{id: id, label: g.db.TacticConditions[id].Name})
		}
	case tacticPercent:
		if e.row.Percent != nil {
			selected = strconv.Itoa(*e.row.Percent)
		}
		if tacticThreshold(g.db, e.row.Condition) {
			for p := 0; p <= 100; p++ {
				e.options.items = append(e.options.items, listItem{id: strconv.Itoa(p), label: fmt.Sprintf("%d%%", p)})
			}
		}
	case tacticTarget:
		selected = e.row.Target
		for _, id := range catalogIDs(g.db.TacticTargets) {
			e.options.items = append(e.options.items, listItem{id: id, label: g.db.TacticTargets[id].Name})
		}
	case tacticAction:
		selected = e.row.Action
		for _, id := range catalogIDs(g.db.TacticActions) {
			a := g.db.TacticActions[id]
			it := listItem{id: id, label: a.Name}
			if sk := g.db.Skills[a.Use]; sk != nil {
				it.right = fmt.Sprintf("MP %d", sk.MP)
				it.disabled = a.Use != g.db.Game.Battle.Attack && a.Use != g.db.Game.Battle.Guard && !slices.Contains(g.st.Actors[e.actor].Skills(), a.Use)
			} else if a.Item != "" {
				it.right = lang.T("1個")
			}
			if g.db.Game.Battle.SimpleTactics && it.disabled {
				continue
			}
			e.options.items = append(e.options.items, it)
		}
	}
	e.options.sel, e.options.top = 0, 0
	for i, it := range e.options.items {
		if it.id == selected {
			e.options.sel = i
		}
	}
	e.options.scroll()
}

func (e *tacticEditor) apply(g *Game) {
	if len(e.errors(g)) != 0 {
		g.sfx("buzzer")
		return
	}
	draft := e.parent.drafts[e.actor]
	if draft.Mode != rpg.TacticsManual {
		draft.Mode, draft.Rows = rpg.TacticsManual, e.parent.currentRows(g)
		draft.Preset = ""
	}
	if e.adding {
		if g.db.Game.Battle.SimpleTactics && len(draft.Rows) >= data.TacticRowLimit {
			g.sfx("buzzer")
			return
		}
		i := min(e.index+1, len(draft.Rows))
		draft.Rows = slices.Insert(draft.Rows, i, e.row)
		e.parent.rows.sel = i
	} else {
		draft.Rows[e.index] = e.row
	}
	e.parent.drafts[e.actor], e.parent.dirty[e.actor] = draft, true
	e.parent.refresh(g)
	g.pop()
}

func (e *tacticEditor) update(g *Game) {
	if g.in.cancel() {
		if e.choosing {
			e.choosing = false
		} else {
			g.pop()
		}
		return
	}
	if g.in.pressed[btnLog] {
		g.push(newTacticText(g, e.description(g)))
		return
	}
	if g.in.click {
		for f, l := range []*list{e.fields, e.options} {
			for i := l.top; i < min(len(l.items), l.top+l.rows); i++ {
				if g.in.mouse.In(l.rowRect(i)) {
					e.choosing = f == 1
				}
			}
		}
	}
	if g.in.left() {
		e.choosing = false
		return
	}
	if g.in.right() && e.field(g) <= tacticAction && !e.fields.items[e.fields.sel].disabled {
		e.choosing = true
		return
	}
	if !e.choosing {
		old := e.fields.sel
		chosen, _ := e.fields.update(g)
		if e.fields.sel != old {
			e.refresh(g)
		}
		if chosen {
			switch e.field(g) {
			case tacticRowAccept:
				e.apply(g)
			case tacticRowCancel:
				g.pop()
			case tacticRowDetail:
				g.push(newTacticText(g, e.description(g)))
			default:
				e.choosing = true
			}
		}
		return
	}
	chosen, _ := e.options.update(g)
	if !chosen {
		return
	}
	id := e.options.items[e.options.sel].id
	switch e.field(g) {
	case tacticCondition:
		e.row.Condition = id
		if tacticThreshold(g.db, id) {
			if e.row.Percent == nil {
				e.row.Percent = new(50)
			}
		} else {
			e.row.Percent = nil
		}
	case tacticPercent:
		p, err := strconv.Atoi(id)
		if err != nil {
			g.failure = err
			return
		}
		e.row.Percent = new(p)
	case tacticTarget:
		e.row.Target = id
	case tacticAction:
		e.row.Action = id
	}
	e.choosing = false
	e.refresh(g)
}

func (e *tacticEditor) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, colorBlack)
	g.text(dst, g.st.Actors[e.actor].Name()+" / "+lang.T("作戦の行を編集"), 30, 25, sizeL, g.th.text)
	e.fields.draw(g, dst, !e.choosing)
	e.options.draw(g, dst, e.choosing)
	g.window(dst, 24, 552, sw-48, 100)
	g.drawLinesFit(dst, e.description(g), 44, 565, sw-88, sizeS, 3, g.th.text)
	g.drawLinesFit(dst, lang.T("← → 欄を移動　↑ ↓ 選択　決定で変更　B / L1 説明全文　取消で戻る"), 30, 674, sw-60, sizeS, 1, g.th.border)
}

type tacticText struct{ lines *list }

func newTacticText(g *Game, text string) *tacticText {
	sw, _ := g.screen()
	var items []listItem
	for _, part := range strings.Split(text, "\n") {
		for _, line := range gfx.Wrap(part, sizeS, sw-140) {
			items = append(items, listItem{label: line})
		}
	}
	l := newList(items, 24, 108, sw-48, 12)
	l.rowH, l.size = 40, sizeS
	return &tacticText{lines: l}
}
func (s *tacticText) overlay() bool { return true }
func (s *tacticText) update(g *Game) {
	chosen, cancel := s.lines.update(g)
	if chosen || cancel || g.in.pressed[btnLog] {
		g.pop()
	}
}
func (s *tacticText) draw(g *Game, dst *ebiten.Image) {
	fill(dst, colorBlack)
	g.text(dst, lang.T("説明の全文"), 30, 25, sizeL, g.th.text)
	s.lines.draw(g, dst, true)
	g.text(dst, lang.T("↑ ↓ スクロール　決定・取消で戻る"), 30, 674, sizeS, g.th.border)
}

func (e *tacticEditor) field(g *Game) int {
	if g.db.Game.Battle.SimpleTactics {
		return []int{tacticCondition, tacticAction, tacticRowAccept, tacticRowCancel, tacticRowDetail}[e.fields.sel]
	}
	return e.fields.sel
}
