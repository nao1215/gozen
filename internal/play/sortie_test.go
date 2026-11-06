package play

import (
	"fmt"
	"image"
	"slices"
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

func TestSortieRosterChangeClearsPendingSwap(t *testing.T) {
	g, _ := campaignOpen(t, nil)
	s := &sortieScene{r: event.SortieScene{Cap: 3, Ready: []string{"hero", "wing", "rear"}}, all: []string{"hero", "wing", "rear"}, picked: []string{"hero", "wing", "rear"}, tactics: map[string]string{}, swap: -1}
	click := func(x, y, w, h float64) {
		g.in = input{click: true, mouse: image.Pt(int(x+w/2), int(y+h/3))}
		s.update(g)
	}
	click(s.slotBox(g, 0))
	if s.swap != 0 {
		t.Fatal("first slot did not arm a swap")
	}
	click(s.rowBox(g, 0))
	if !slices.Equal(s.picked, []string{"wing", "rear"}) {
		t.Fatalf("roster removal: %v", s.picked)
	}
	click(s.slotBox(g, 1))
	if !slices.Equal(s.picked, []string{"wing", "rear"}) || s.swap != 1 {
		t.Fatalf("roster change kept a stale swap: picked=%v swap=%d", s.picked, s.swap)
	}
}

func TestSortieSelectionAndConfirmation(t *testing.T) {
	for _, march := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "march"}[march], func(t *testing.T) {
			g, _ := campaignOpen(t, nil)
			var sent []string
			var sentTactics map[string]string
			done := 0
			s := newSortie(g, event.SortieScene{Cap: 2, Ready: []string{"hero", "wing"}, March: func(ids []string, tactics map[string]string) { sent, sentTactics = ids, tactics }}, func() { done++ })
			s.all = []string{"hero", "wing", "away"}
			s.tactics["wing"] = s.tactics["hero"]
			g.push(s)
			depth := len(g.stack)
			s.toggle(g, "away")
			if !slices.Equal(s.picked, []string{"hero"}) {
				t.Fatal("unready general was selected")
			}
			s.swap = 0
			s.toggle(g, "wing")
			if !slices.Equal(s.picked, []string{"hero", "wing"}) || s.swap != -1 {
				t.Fatal("adding a general retained a stale swap")
			}
			s.r.Ready = append(s.r.Ready, "away")
			s.toggle(g, "away")
			if len(s.picked) != 2 {
				t.Fatal("selection exceeded capacity")
			}
			// A pair of slot clicks swaps the leader, without altering membership.
			for _, slot := range []int{0, 1} {
				x, y, w, h := s.slotBox(g, slot)
				g.in = input{click: true, mouse: image.Pt(int(x+w/2), int(y+h/3))}
				s.update(g)
			}
			if !slices.Equal(s.picked, []string{"wing", "hero"}) || s.swap != -1 {
				t.Fatalf("swap: %v", s.picked)
			}
			// Both pointer arrows and the keyboard cycle tactics reversibly.
			before := s.tactics["wing"]
			x, y, w, h := s.slotBox(g, 0)
			g.in = input{click: true, mouse: image.Pt(int(x+w*.9), int(y+h-20))}
			s.update(g)
			s.cursor = 1
			g.in = input{pressed: [numButtons]bool{btnLeft: true}}
			s.update(g)
			if s.tactics["wing"] != before {
				t.Fatal("opposite tactic steps did not restore the selection")
			}
			button := 1
			if march {
				button = 0
			}
			x, y, w, h = s.buttonBox(g, button)
			g.in = input{click: true, mouse: image.Pt(int(x+w/2), int(y+h/2))}
			s.update(g)
			s.close(g, march)
			if done != 1 || len(g.stack) != depth-1 {
				t.Fatal("confirmation did not close exactly once")
			}
			if !march {
				if sent != nil {
					t.Fatal("cancel dispatched a march")
				}
				return
			}
			if !slices.Equal(sent, []string{"wing", "hero"}) || len(sentTactics) != 2 {
				t.Fatalf("march lost formation: %v %v", sent, sentTactics)
			}
			sent[0] = "changed"
			sentTactics["hero"] = "changed"
			if s.picked[0] != "wing" || s.tactics["hero"] == "changed" || g.sortieTactics["hero"] == "changed" {
				t.Fatal("march callback aliases selection or remembered tactics")
			}
		})
	}
}

func TestSortieEmptyFormationCannotMarch(t *testing.T) {
	g, _ := campaignOpen(t, nil)
	done := 0
	s := newSortie(g, event.SortieScene{Cap: 2}, func() { done++ })
	g.push(s)
	depth := len(g.stack)
	s.cursor = len(s.all)
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	s.update(g)
	x, y, w, h := s.buttonBox(g, 0)
	g.in = input{click: true, mouse: image.Pt(int(x+w/2), int(y+h/2))}
	s.update(g)
	if s.closed || done != 0 || len(g.stack) != depth {
		t.Fatal("empty formation marched")
	}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	s.update(g)
	if !s.closed || done != 1 {
		t.Fatal("empty formation could not cancel")
	}
}
