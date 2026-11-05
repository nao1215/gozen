package play

import (
	"slices"

	lang "github.com/nao1215/gozen/internal/text"

	"github.com/hajimehoshi/ebiten/v2"
)

type sysMenu struct {
	l *list
}

var sysScreens = []string{"save", "load", "config", "title"}

// sysProfiles is the place of the character book in the menu, after loading, when
// game.yaml asks for it (ui.profiles).
const sysProfiles = 2

func newSysMenu(g *Game) *sysMenu {
	items := []listItem{{label: lang.T("記録する")}, {label: lang.T("記録を読む")}, {label: lang.T("せってい")}, {label: lang.T("タイトルへ")}, {label: lang.T("とじる")}}
	if loc := g.db.Locations[g.st.Location]; loc == nil || !loc.Safe {
		items[0].disabled = true
	}
	if !g.anySave() {
		items[1].disabled = true
	}
	if g.profilesInMenus() {
		items = slices.Insert(items, sysProfiles, listItem{label: g.profileTitle(), id: "profiles"})
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
	sel := m.l.sel
	if m.l.items[sel].id == "profiles" {
		g.pop()
		g.sfx("decide")
		g.push(newProfiles(g, nil))
		return
	}
	if g.profilesInMenus() && sel > sysProfiles {
		sel--
	}
	if sel >= len(sysScreens) {
		g.pop()
		return
	}
	g.pop()
	g.openScreen(sysScreens[sel], func() {})
}

func (m *sysMenu) draw(g *Game, dst *ebiten.Image) {
	m.l.draw(g, dst, true)
}
