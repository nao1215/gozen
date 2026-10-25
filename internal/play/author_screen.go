package play

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// authorScreen presents an authored list without requiring a custom Go screen.
type authorScreen struct {
	name    string
	title   string
	entries []data.ScreenEntry
	list    *list
	done    func()
}

func newAuthorScreen(g *Game, name string, done func()) *authorScreen {
	s := &authorScreen{name: name, done: done}
	def := g.db.Game.Screens[name]
	var items []listItem
	if def != nil {
		s.title = data.Render(g.db.Tr(def.Title), g.st.Env())
		for _, entry := range def.Entries {
			visible, err := entry.When.Bool(g.st.Env())
			if err != nil || !visible {
				continue
			}
			entry.Label = data.Render(g.db.Tr(entry.Label), g.st.Env())
			entry.Text = data.Render(g.db.Tr(entry.Text), g.st.Env())
			s.entries = append(s.entries, entry)
			items = append(items, listItem{label: entry.Label})
		}
	}
	sw, sh := g.screen()
	s.list = newList(items, 24, 88, sw*0.32, max(1, int((sh-140)/44)))
	return s
}

func (s *authorScreen) modal() bool { return true }

func (s *authorScreen) liveKeep(g *Game) scene {
	n := newAuthorScreen(g, s.name, s.done)
	n.list.sel = min(s.list.sel, max(0, len(n.entries)-1))
	n.list.scroll()
	return n
}

func (s *authorScreen) update(g *Game) {
	_, cancel := s.list.update(g)
	if cancel {
		g.pop()
		if s.done != nil {
			s.done()
		}
	}
}

func (s *authorScreen) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	fill(dst, color.NRGBA{0, 0, 0, 220})
	g.drawLinesFit(dst, s.title, 36, 28, sw-72, sizeL, 1, g.th.accent)
	s.list.draw(g, dst, true)
	x, w := sw*0.36, sw*0.64-36
	g.window(dst, x, 88, w, sh-120)
	if len(s.entries) == 0 {
		g.text(dst, "No entries yet", x+20, 108, sizeM, g.th.dim)
		return
	}
	e := s.entries[s.list.sel]
	y := float64(108)
	if e.Picture != "" {
		g.drawPicture(dst, data.Picture{File: e.Picture}, x+20, y, w-40, (sh-160)*0.55, 1)
		y += (sh-160)*0.55 + 16
	}
	g.drawLinesFit(dst, e.Text, x+20, y, w-40, sizeM, max(1, int((sh-y-52)/32)), g.th.text)
}
