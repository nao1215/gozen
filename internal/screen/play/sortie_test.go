package play

import (
	"fmt"
	"image"
	"slices"
	"testing"

	"github.com/nao1215/gozen/internal/rules/event"
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

// TestSortieListScrollsByTouch: with more generals than rows, a player with a touch screen
// only reaches the last general: by a tap on the arrow under the rows, or with a finger
// dragged up the list (whose lift puts no general in a slot).
func TestSortieListScrollsByTouch(t *testing.T) {
	open := func(t *testing.T) (*Game, *sortieScene) {
		g, _ := campaignOpen(t, nil)
		s := &sortieScene{r: event.SortieScene{Cap: 3}, tactics: map[string]string{}, swap: -1}
		for i := range 12 {
			s.all = append(s.all, fmt.Sprintf("g%02d", i))
		}
		s.r.Ready = s.all
		return g, s
	}
	t.Run("arrow", func(t *testing.T) {
		g, s := open(t)
		cx, up, down := s.markPlaces(g)
		g.in = input{click: true, pdown: true, pup: true, moved: true, mouse: image.Pt(int(cx), int(down+sizeS/2))}
		s.update(g)
		if first, last := s.shownRows(g); first != s.rows(g)-1 || len(s.picked) != 0 {
			t.Fatalf("a tap on ▼ shows rows %d to %d with %d picked, want a page on and nobody picked", first, last, len(s.picked))
		}
		g.in.mouse = image.Pt(int(cx), int(up+sizeS/2))
		s.update(g)
		g.in = input{}
		if first, _ := s.shownRows(g); first != 0 {
			t.Fatalf("a tap on ▲ left the list at row %d", first)
		}
	})
	t.Run("drag", func(t *testing.T) {
		g, s := open(t)
		_, last := s.shownRows(g)
		x, y, w, h := s.rowBox(g, last-1)
		at := image.Pt(int(x+w/2), int(y+h/2))
		g.in = input{mouse: at, moved: true, pdown: true, pheld: true}
		s.update(g)
		for range 3 {
			at.Y -= int(h+8) + 1
			g.in = input{mouse: at, moved: true, pheld: true}
			s.update(g)
		}
		g.in = input{mouse: at, moved: true, pup: true, click: true}
		s.update(g)
		g.in = input{}
		if first, _ := s.shownRows(g); first != 3 || len(s.picked) != 0 {
			t.Fatalf("a finger dragged up three rows: the list is at row %d with %d picked, want 3 and nobody", first, len(s.picked))
		}
	})
}

// TestSortieSlotsStayClearOfTheButtons: a march takes as many generals as its order says
// (generals has no upper limit outside clash_view), so every slot must stay on the screen
// and off the buttons, which a click must still reach.
func TestSortieSlotsStayClearOfTheButtons(t *testing.T) {
	g, _ := campaignOpen(t, nil)
	_, sh := g.screen()
	ids := []string{"g0", "g1", "g2", "g3", "g4", "g5"}
	done := 0
	s := &sortieScene{r: event.SortieScene{Cap: 6, Ready: ids, March: func([]string, map[string]string) {}}, all: ids, picked: slices.Clone(ids), tactics: map[string]string{}, swap: -1, done: func() { done++ }}
	g.push(s)
	for k := range s.r.Cap {
		if _, y, _, h := s.slotBox(g, k); y+h > sh {
			t.Errorf("slot %d runs to %g, below the screen's %g", k, y+h, sh)
		}
	}
	x, y, w, h := s.buttonBox(g, 1) // Cancel
	g.in = input{click: true, moved: true, mouse: image.Pt(int(x+w/2), int(y+h/2))}
	s.update(g)
	g.in = input{}
	if !s.closed || done != 1 {
		t.Fatalf("a click on Cancel did not close the screen (swap %d): the sixth slot lies over the button and took it", s.swap)
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
