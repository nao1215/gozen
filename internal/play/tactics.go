package play

import (
	"fmt"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

type tacticsScreen struct {
	ids                    []string
	drafts                 map[string]rpg.ActorTactics
	dirty                  map[string]bool
	members, rows, actions *list
	presets                *list
	editing                bool
	focus                  int
	done                   func()
}

const (
	tacticMoveUp = iota
	tacticMoveDown
	tacticToggle
	tacticAuto
	tacticAutoAll
	tacticAccept
	tacticCancel
	tacticEdit
	tacticAdd
	tacticDelete
)

func newTacticsScreen(g *Game, done func()) *tacticsScreen {
	s := &tacticsScreen{drafts: map[string]rpg.ActorTactics{}, dirty: map[string]bool{}, done: done}
	var members []listItem
	for _, id := range g.db.Order["actors"] {
		if a := g.st.Actors[id]; a != nil {
			s.ids = append(s.ids, id)
			s.drafts[id] = a.TacticsDraft()
			members = append(members, listItem{label: a.Name()})
		}
	}
	sw, _ := g.screen()
	s.members = newList(members, 24, 108, 250, 5)
	s.members.size = sizeS
	s.rows = newList(nil, 290, 108, sw-598, 9)
	s.rows.size = sizeS
	s.actions = newList(nil, sw-292, 108, 268, 10)
	s.actions.rowH = 38
	s.actions.size = sizeS
	if g.db.Game.Battle.SimpleTactics {
		s.members.w = 240
		s.rows.x, s.rows.w, s.rows.rows = 284, sw-588, 5
		s.presets = newList(nil, 284, 108, sw-308, 7)
		s.presets.size = sizeS
		s.presets.rowH = 52
	}
	s.refresh(g)
	return s
}

func (s *tacticsScreen) overlay() bool { return true }

func (s *tacticsScreen) actorID() string {
	if len(s.ids) == 0 {
		return ""
	}
	return s.ids[s.members.sel]
}

func (s *tacticsScreen) currentRows(g *Game) []data.TacticRow {
	id := s.actorID()
	if id == "" {
		return nil
	}
	draft := s.drafts[id]
	if g.db.Game.Battle.SimpleTactics {
		return g.st.Actors[id].TacticRows(draft)
	}
	if draft.Mode != rpg.TacticsManual {
		return g.st.Actors[id].AutoTactics()
	}
	return draft.Rows
}

func (s *tacticsScreen) rowText(g *Game, row data.TacticRow) string {
	c, t, a := row.Condition, row.Target, row.Action
	if def := g.db.TacticConditions[c]; def != nil {
		c = def.Name
	}
	if def := g.db.TacticTargets[t]; def != nil {
		t = def.Name
	}
	if def := g.db.TacticActions[a]; def != nil {
		a = def.Name
	}
	if g.db.Game.Battle.SimpleTactics {
		return fmt.Sprintf(lang.T("%s → %s"), c, a)
	}
	if row.Percent != nil {
		c += fmt.Sprintf(lang.T("（%d%%）"), *row.Percent)
	}
	return fmt.Sprintf(lang.T("%s → %s → %s"), c, t, a)
}

func (s *tacticsScreen) refresh(g *Game) {
	for i, id := range s.ids {
		mode := lang.T("おまかせ")
		if s.drafts[id].Mode == rpg.TacticsManual {
			mode = lang.T("手動")
			if s.presets != nil {
				mode = lang.T("自作")
			}
		}
		if p, ok := g.db.Game.Battle.TacticPresets[s.drafts[id].Preset]; ok {
			mode = p.Name
		}
		if s.dirty[id] {
			mode += " *"
		}
		s.members.items[i].right = mode
	}
	s.rows.items = nil
	for i, row := range s.currentRows(g) {
		state := "ON"
		if !row.Enabled {
			state = "OFF"
		}
		label := fmt.Sprintf("%d  %s", i+1, s.rowText(g, row))
		if g.db.Game.Battle.SimpleTactics {
			mark := "▶ "
			if !row.Enabled {
				mark = "○ "
			}
			label, state = mark+s.rowText(g, row), ""
		}
		s.rows.items = append(s.rows.items, listItem{label: label, right: state})
	}
	s.rows.sel = min(s.rows.sel, max(0, len(s.rows.items)-1))
	s.rows.scroll()
	s.actions.items = []listItem{
		{label: lang.T("一つ上へ"), disabled: len(s.rows.items) == 0 || s.rows.sel == 0},
		{label: lang.T("一つ下へ"), disabled: len(s.rows.items) == 0 || s.rows.sel == len(s.rows.items)-1},
		{label: lang.T("ON / OFF"), disabled: len(s.rows.items) == 0},
		{label: lang.T("この仲間をおまかせに"), disabled: len(s.ids) == 0},
		{label: lang.T("全員おまかせに"), disabled: len(s.ids) == 0},
		{label: lang.T("確定して閉じる")},
		{label: lang.T("取り消して閉じる")},
		{label: lang.T("条件・対象・行動を編集"), disabled: len(s.rows.items) == 0},
		{label: lang.T("行を追加"), disabled: len(s.ids) == 0 || g.db.Game.Battle.SimpleTactics && len(s.rows.items) >= data.TacticRowLimit},
		{label: lang.T("行を削除"), disabled: len(s.rows.items) == 0},
	}
	if s.presets != nil {
		s.actions.items[tacticEdit].label = lang.T("いつ・行動を編集")
		s.actions.items[tacticAutoAll].label = lang.T("型を選ぶ画面へ")
		s.presets.items = []listItem{{id: "", label: lang.T("おまかせ")}}
		for _, id := range catalogIDs(g.db.Game.Battle.TacticPresets) {
			s.presets.items = append(s.presets.items, listItem{id: id, label: g.db.Game.Battle.TacticPresets[id].Name})
		}
		s.presets.items = append(s.presets.items, listItem{id: "manual", label: lang.T("自分で組む")}, listItem{id: "accept", label: lang.T("確定して閉じる")}, listItem{id: "cancel", label: lang.T("取り消して閉じる")})
	}
}

func (s *tacticsScreen) close(g *Game) {
	g.pop()
	if s.done != nil {
		s.done()
	}
}

func (s *tacticsScreen) act(g *Game) {
	id := s.actorID()
	switch s.actions.sel {
	case tacticAccept:
		for _, id := range s.ids {
			if s.dirty[id] {
				if err := g.st.Actors[id].SetTactics(s.drafts[id]); err != nil {
					return
				}
			}
		}
		s.close(g)
		return
	case tacticCancel:
		s.close(g)
		return
	case tacticEdit, tacticAdd:
		if id == "" || s.actions.sel == tacticEdit && len(s.rows.items) == 0 || s.actions.sel == tacticAdd && g.db.Game.Battle.SimpleTactics && len(s.rows.items) >= data.TacticRowLimit {
			return
		}
		g.push(newTacticEditor(g, s, s.actions.sel == tacticAdd))
		return
	case tacticAuto, tacticAutoAll:
		if s.presets != nil && s.actions.sel == tacticAutoAll {
			s.editing, s.focus = false, 1
			return
		}
		ids := s.ids
		if s.actions.sel == tacticAuto {
			ids = []string{id}
		}
		for _, id := range ids {
			s.drafts[id] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto}
			s.dirty[id] = true
		}
	default:
		if id == "" || len(s.rows.items) == 0 {
			return
		}
		draft := s.drafts[id]
		if draft.Mode != rpg.TacticsManual {
			draft.Mode, draft.Rows = rpg.TacticsManual, s.currentRows(g)
			draft.Preset = ""
		}
		i := s.rows.sel
		switch s.actions.sel {
		case tacticMoveUp, tacticMoveDown:
			j := i - 1
			if s.actions.sel == tacticMoveDown {
				j = i + 1
			}
			if j < 0 || j >= len(draft.Rows) {
				return
			}
			draft.Rows[i], draft.Rows[j] = draft.Rows[j], draft.Rows[i]
			s.rows.sel = j
		case tacticToggle:
			draft.Rows[i].Enabled = !draft.Rows[i].Enabled
		case tacticDelete:
			draft.Rows = append(draft.Rows[:i], draft.Rows[i+1:]...)
		}
		s.drafts[id], s.dirty[id] = draft, true
	}
	s.refresh(g)
}

func (s *tacticsScreen) update(g *Game) {
	if s.presets != nil && !s.editing {
		s.updatePresets(g)
		return
	}
	if g.in.pressed[btnLog] && len(s.currentRows(g)) > 0 {
		g.push(newTacticText(g, s.description(g)))
		return
	}
	if g.in.cancel() {
		g.sfx("cancel")
		s.close(g)
		return
	}

	lists := []*list{s.members, s.rows, s.actions}
	if g.in.click {
		for f, l := range lists {
			for i := l.top; i < min(len(l.items), l.top+l.rows); i++ {
				if g.in.mouse.In(l.rowRect(i)) {
					s.focus = f
				}
			}
		}
	}
	if d := g.in.tab(); d != 0 && len(s.ids) > 0 {
		s.members.sel = (s.members.sel + d + len(s.ids)) % len(s.ids)
		s.members.scroll()
		s.rows.sel, s.rows.top = 0, 0
		s.refresh(g)
		return
	}
	if g.in.left() || g.in.right() {
		d := 1
		if g.in.left() {
			d = -1
		}
		s.focus = (s.focus + d + len(lists)) % len(lists)
		g.sfx("cursor")
		return
	}
	oldMember := s.members.sel
	chosen, _ := lists[s.focus].update(g)
	if s.members.sel != oldMember {
		s.rows.sel, s.rows.top = 0, 0
	}
	s.refresh(g)
	if chosen {
		switch s.focus {
		case 0:
			s.focus = 1
		case 1:
			s.focus = 2
		case 2:
			s.act(g)
		}
	}
}

func (s *tacticsScreen) draw(g *Game, dst *ebiten.Image) {
	if s.presets != nil && !s.editing {
		s.drawPresets(g, dst)
		return
	}
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0xe8))
	g.text(dst, lang.T("作戦"), 30, 22, sizeL, g.th.text)
	g.text(dst, lang.T("上から成立する行を使います。使えない行は次へ進みます。"), 160, 29, sizeS, g.th.text)
	rowTitle := lang.T("条件 → 対象 → 行動")
	if s.presets != nil {
		rowTitle = lang.T("いつ → 何をする（５行まで）")
	}
	for i, title := range []string{lang.T("仲間"), rowTitle, lang.T("操作")} {
		l := []*list{s.members, s.rows, s.actions}[i]
		col := g.th.border
		if s.focus == i {
			col = g.th.cursor
		}
		g.text(dst, title, l.x+12, 77, sizeS, col)
		if i == 1 && len(l.items) == 0 {
			g.window(dst, l.x, l.y, l.w, float64(l.rows)*l.rowH+24)
		} else {
			l.draw(g, dst, s.focus == i || i < 2)
		}
	}
	if len(s.rows.items) == 0 {
		text := lang.T("作戦の行がありません。通常攻撃を使います。おまかせ未定義の場合は従来の AI を使います。")
		if s.presets != nil {
			text = lang.T("作戦の行がありません。通常攻撃を使います。")
		}
		g.drawLinesFit(dst, text, s.rows.x+24, 125, s.rows.w-48, sizeS, 3, g.th.dim)
	}
	g.window(dst, 24, 552, sw-48, 100)
	if rows := s.currentRows(g); len(rows) > 0 {
		g.drawLinesFit(dst, s.description(g), 44, 565, sw-88, sizeS, 3, g.th.text)
	} else {
		g.text(dst, lang.T("仲間を選び、行を選んでから右の操作を選びます。"), 44, 572, sizeS, g.th.text)
	}
	help := []string{lang.T("← → 列を移動"), lang.T("↑ ↓ 行を選択"), lang.T("決定で操作"), lang.T("B / L1 説明全文"), lang.T("取消で全変更を破棄"), lang.T("* は未確定")}
	g.drawLinesFit(dst, strings.Join(help, "   "), 30, 674, sw-60, sizeS*0.85, 1, g.th.border)
}

func (s *tacticsScreen) description(g *Game) string {
	row := s.currentRows(g)[s.rows.sel]
	text := s.rowText(g, row) + " / " + tacticCost(g, row.Action)

	_, errs := g.db.CompileTactic(row, g.st.Actors[s.actorID()].Skills())
	for _, err := range errs {
		if s.presets != nil {
			text += "\n" + lang.T("まだ使えない行は飛ばします。技を覚えると使えます。")
			break
		}
		text += "\n" + err.Error()
	}
	return text
}

func (s *tacticsScreen) updatePresets(g *Game) {
	if g.in.cancel() {
		s.close(g)
		return
	}
	if d := g.in.tab(); d != 0 && len(s.ids) > 0 {
		s.members.sel = (s.members.sel + d + len(s.ids)) % len(s.ids)
		s.refresh(g)
		return
	}
	if g.in.left() || g.in.right() {
		s.focus = 1 - s.focus
		return
	}
	lists := []*list{s.members, s.presets}
	if g.in.click {
		for f, l := range lists {
			for i := l.top; i < min(len(l.items), l.top+l.rows); i++ {
				if g.in.mouse.In(l.rowRect(i)) {
					s.focus = f
				}
			}
		}
	}
	chosen, _ := lists[s.focus].update(g)
	if chosen {
		if s.focus == 0 {
			s.focus = 1
			return
		}
		id := s.actorID()
		choice := s.presets.items[s.presets.sel].id
		switch choice {
		case "accept":
			s.actions.sel = tacticAccept
			s.act(g)
			return
		case "cancel":
			s.close(g)
			return
		case "manual":
			if id == "" {
				return
			}
			s.drafts[id] = rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: s.currentRows(g)}
			s.dirty[id], s.editing, s.focus = true, true, 1
		default:
			if id == "" {
				return
			}
			draft := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsAuto}
			if choice != "" {
				draft.Mode, draft.Preset = rpg.TacticsPreset, choice
			}
			s.drafts[id], s.dirty[id] = draft, true
		}
	}
	s.refresh(g)
}

func (s *tacticsScreen) drawPresets(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0xe8))
	g.text(dst, lang.T("作戦の型を選ぶ"), 30, 22, sizeL, g.th.text)
	g.text(dst, lang.T("仲間"), 36, 77, sizeS, g.th.text)
	g.text(dst, lang.T("型を一つ選び、確定するだけ"), 296, 77, sizeS, g.th.text)
	s.members.draw(g, dst, s.focus == 0)
	s.presets.draw(g, dst, s.focus == 1)
	desc := lang.T("回復を優先し、使える技で戦います。")
	if len(s.presets.items) > 0 {
		id := s.presets.items[s.presets.sel].id
		if p, ok := g.db.Game.Battle.TacticPresets[id]; ok {
			desc = p.Description
		}
		if id == "manual" {
			desc = lang.T("５行まで編集できます。上から順に試します。")
		}
	}
	g.window(dst, 24, 552, sw-48, 100)
	g.drawLinesFit(dst, desc+"\n"+lang.T("薬・MPが足りない行は飛ばします。誰に使うかは自動です。"), 44, 565, sw-88, sizeS, 3, g.th.text)
	g.drawLinesFit(dst, lang.T("← → 欄を移動　↑ ↓ 選択　決定で型を選ぶ　取消で全変更を破棄　* は未確定"), 30, 674, sw-60, sizeS*0.85, 1, g.th.border)
}
