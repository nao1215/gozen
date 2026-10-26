package play

import (
	"fmt"
	"testing"

	"github.com/nao1215/gozen/internal/event"
)

// The sortie's list of generals scrolls: however many have joined, the rows drawn keep
// above the bottom line and the buttons, and the row the keys or the wheel move to is one
// of them.
func TestSortieListScrolls(t *testing.T) {
	g, _ := campaignOpen(t, nil)
	_, sh := g.screen()
	for _, n := range []int{7, 12, 20} {
		s := &sortieScene{r: event.SortieScene{Cap: 3}, tactics: map[string]string{}, swap: -1}
		for i := range n {
			s.all = append(s.all, fmt.Sprintf("g%02d", i))
		}
		for step := range n + 3 {
			first, last := s.shownRows(g)
			if s.cursor < len(s.all) && (s.cursor < first || s.cursor >= last) {
				t.Fatalf("%d generals, step %d: the cursor %d is off the rows %d to %d", n, step, s.cursor, first, last)
			}
			for i := first; i < last; i++ {
				if _, y, _, h := s.rowBox(g, i); y+h > sh*0.86 {
					t.Fatalf("%d generals: row %d reaches %g, into the bottom line at %g", n, i, y+h, sh*0.86)
				}
			}
			g.in.pressed[btnDown] = true
			s.update(g)
			g.in.pressed = [numButtons]bool{}
		}
		// the wheel moves the cursor (up and down), and the list follows it
		s.cursor, s.top = 0, 0
		g.in.wheel = 1
		s.update(g)
		g.in.wheel = 0
		if s.cursor != n+1 {
			t.Errorf("%d generals: the wheel took the cursor to %d", n, s.cursor)
		}
		g.in.wheel = -1
		s.update(g)
		g.in.wheel = 0
		if first, _ := s.shownRows(g); s.cursor != 0 || first != 0 {
			t.Errorf("%d generals: back on the first general, the list starts at %d", n, first)
		}
		s.draw(g, wldScreen(g))
	}
}
