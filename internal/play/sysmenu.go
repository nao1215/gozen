package play

import (
	lang "github.com/nao1215/gozen/text"

	"github.com/hajimehoshi/ebiten/v2"
)

type sysMenu struct {
	l *list
}

var sysScreens = []string{"save", "load", "config", "title"}

func newSysMenu(g *Game) *sysMenu {
	items := []listItem{{label: lang.T("記録する")}, {label: lang.T("記録を読む")}, {label: lang.T("せってい")}, {label: lang.T("タイトルへ")}, {label: lang.T("とじる")}}
	if loc := g.db.Locations[g.st.Location]; loc == nil || !loc.Safe {
		items[0].disabled = true
	}
	if !g.anySave() {
		items[1].disabled = true
	}
	sw, sh := g.screen()
	const w = 300.0
	l := newList(items, (sw-w)/2, sh/2-130, w, len(items))
	return &sysMenu{l: l}
}

func (m *sysMenu) overlay() bool { return true }

func (m *sysMenu) update(g *Game) {
	chose, cancel := m.l.update(g)
	if cancel {
		g.pop()
		return
	}
	if !chose {
		return
	}
	if m.l.sel >= len(sysScreens) {
		g.pop()
		return
	}
	g.pop()
	g.openScreen(sysScreens[m.l.sel], func() {})
}

func (m *sysMenu) draw(g *Game, dst *ebiten.Image) {
	m.l.draw(g, dst, true)
}
