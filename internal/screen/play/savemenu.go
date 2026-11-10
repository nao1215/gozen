package play

import (
	"fmt"
	"math"

	lang "github.com/nao1215/gozen/internal/base/text"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/screen/gfx"
)

type saveMenu struct {
	saving bool
	inPlay bool
	slots  *list
	done   func()
	msg    string
	msgT   int
}

func newSaveMenu(g *Game, saving bool, done func()) *saveMenu {
	s := &saveMenu{saving: saving, done: done}
	s.refresh(g)
	return s
}

func (s *saveMenu) overlay() bool { return true }

func (s *saveMenu) refresh(g *Game) {
	var items []listItem
	n := saveSlots
	if !s.saving {
		n++
	}
	for i := range n {
		label, ok := g.slotLabel(i)
		items = append(items, listItem{label: label, disabled: !s.saving && !ok})
	}
	sw, sh := g.screen()
	sel := 0
	if s.slots != nil {
		sel = s.slots.sel
	} else if !s.saving {
		sel, _ = g.newestSave()
	}
	width := math.Min(800, sw-32)
	_, _, titleGap := s.heading(width)
	y := math.Max(sh/2-110, 16+titleGap)
	// Reserve the status window even before a write, so saving does not move
	// the selected slot and short landscape views retain every row.
	rowHeight := math.Min(60, (sh-y-24-20-64-16)/float64(n))
	s.slots = newList(items, (sw-width)/2, y, width, n)
	s.slots.rowH = math.Max(24, rowHeight)
	s.slots.size = math.Min(sizeM, s.slots.rowH/1.5)
	s.slots.sel = sel
}

func (s *saveMenu) update(g *Game) {
	if s.msgT > 0 {
		s.msgT--
		if s.msgT == 0 || g.in.advance() {
			s.msgT = 0
			g.leave(s.done)
		}
		return
	}
	chose, cancel := s.slots.update(g)
	if cancel {
		g.leave(s.done)
		return
	}
	if !chose {
		return
	}
	i := s.slots.sel
	if s.saving {
		if g.storeHas(g.slotPath(i)) {
			g.push(newConfirm(fmt.Sprintf(lang.T("スロット%dに 上書きしますか？"), i+1), func() { s.write(g, i) }))
			return
		}
		s.write(g, i)
		return
	}
	st, err := g.load(i)
	if err != nil {
		s.msg = lang.T("ロードできなかった: ") + err.Error()
		s.msgT = 90
		return
	}
	if s.inPlay {
		g.push(newConfirm(fmt.Sprintf(lang.T("スロット%dを ロードしますか？（セーブしていない 進み具合は 消えます）"), i+1), func() { g.loadSavedGame(st) }))
		return
	}
	g.loadSavedGame(st)
}

func (s *saveMenu) write(g *Game, i int) {
	if err := g.save(i); err != nil {
		s.msg = lang.T("セーブできなかった: ") + err.Error()
	} else {
		s.msg = fmt.Sprintf(lang.T("スロット%dに セーブしました。"), i+1)
		g.sfx("item")
	}
	s.refresh(g)
	s.msgT = 80
}

// heading fits the prompt above the slots without changing ordinary layouts.
func (s *saveMenu) heading(width float64) ([]string, float64, float64) {
	title := lang.T("どこに セーブしますか？")
	if !s.saving {
		title = lang.T("どの きろくから はじめますか？")
	}
	lines, size := gfx.FitLines(title, sizeL, width, 2)
	return lines, size, math.Max(70, float64(len(lines))*gfx.LineHeight(size)+16)
}

func (s *saveMenu) draw(g *Game, dst *ebiten.Image) {
	sw, _ := g.screen()
	fill(dst, withAlpha(colorBlack, 0x90))
	lines, size, gap := s.heading(s.slots.w)
	for i, line := range lines {
		gfx.DrawTextOutlineColor(dst, line, sw/2, s.slots.y-gap+float64(i)*gfx.LineHeight(size), size, g.th.text, g.th.window, 1)
	}
	s.slots.draw(g, dst, s.msgT == 0)
	if s.msgT > 0 {
		width := math.Min(600, s.slots.w)
		y := s.slots.y + s.slots.height() + 20
		g.window(dst, (sw-width)/2, y, width, 64)
		lines, size := gfx.FitLines(s.msg, sizeM, width-32, 2)
		top := y + (64-float64(len(lines))*gfx.LineHeight(size))/2
		// Preserve the established single-line baseline on ordinary screens.
		if len(lines) == 1 && size == sizeM {
			top = y + 16
		}
		for i, line := range lines {
			gfx.DrawText(dst, line, sw/2, top+float64(i)*gfx.LineHeight(size), size, g.th.text)
		}
	}
}
