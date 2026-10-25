package play

import (
	"image/color"

	"github.com/nao1215/gozen/internal/event"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// authorScreen presents an authored list without requiring a custom Go screen.
type authorScreen struct {
	name    string
	title   string
	indices []int
	running bool
	parent  *event.Runner
	request event.Request
	origin  string
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
		for i, entry := range def.Entries {
			visible, err := entry.When.Bool(g.st.Env())
			if err != nil || !visible {
				continue
			}
			entry.Label = data.Render(g.db.Tr(entry.Label), g.st.Env())
			entry.Text = data.Render(g.db.Tr(entry.Text), g.st.Env())
			s.entries = append(s.entries, entry)
			s.indices = append(s.indices, i)
			items = append(items, listItem{label: entry.Label})
		}
	}
	sw, sh := g.screen()
	s.list = newList(items, 24, 88, sw*0.32, max(1, int((sh-140)/44)))
	return s
}

func (s *authorScreen) modal() bool   { return !s.running }
func (s *authorScreen) overlay() bool { return s.running }

func (s *authorScreen) liveKeep(g *Game) scene { return s.refreshed(g) }

func (s *authorScreen) refreshed(g *Game) *authorScreen {
	n := newAuthorScreen(g, s.name, s.done)
	n.list.sel = min(s.list.sel, max(0, len(n.entries)-1))
	if s.list.sel < len(s.indices) {
		for i, index := range n.indices {
			if index == s.indices[s.list.sel] {
				n.list.sel = i
				break
			}
		}
	}
	n.running, n.origin = s.running, s.origin
	n.list.scroll()
	return n
}

func (s *authorScreen) update(g *Game) {
	if s.running {
		if g.pump() {
			return
		}
		if s.parent != nil {
			g.events, g.req = s.parent, s.request
		}
		s.parent, s.request, s.running = nil, nil, false
		g.boundary()
		// The resumed opener already crossed an open-screen boundary. Its
		// runner must not inherit the completed child's replay snapshot: F5
		// would otherwise restore state from before the entry action.
		g.evCmds, g.evSnap, g.evRand = nil, nil, nil
		g.evName, g.evLines, g.evArrival = "", 0, false
		g.evAnswers, g.evHist = nil, nil
		if g.st.Location != s.origin {
			g.pop()
			if s.done != nil {
				s.done()
			}
			return
		}
		s.refresh(g)
		return
	}
	chose, cancel := s.list.update(g)
	if cancel {
		g.pop()
		if s.done != nil {
			s.done()
		}
		return
	}
	if chose {
		s.activate(g)
	}
}

func (s *authorScreen) refresh(g *Game) {
	n := s.refreshed(g)
	*s = *n
}

func (s *authorScreen) activate(g *Game) {
	def := g.db.Game.Screens[s.name]
	if def == nil || s.list.sel >= len(s.indices) {
		s.refresh(g)
		return
	}
	index := s.indices[s.list.sel]
	if index >= len(def.Entries) {
		s.refresh(g)
		return
	}
	entry := def.Entries[index]
	visible, err := entry.When.Bool(g.st.Env())
	if err != nil || !visible {
		s.refresh(g)
		return
	}
	if len(entry.Do) == 0 {
		return
	}
	// Suspend the opener so entry commands run immediately and have their own
	// named save cursor. Closing the screen resumes the opener after its open.
	s.parent, s.request, s.origin, s.running = g.events, g.req, g.st.Location, true
	g.events, g.req = g.newEventRunner(), nil
	g.events.Start(entry.Do)
	g.emit("screen_action", traceLine{"screen": s.name, "entry": index})
}

func (s *authorScreen) draw(g *Game, dst *ebiten.Image) {
	if s.running {
		return
	}
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
