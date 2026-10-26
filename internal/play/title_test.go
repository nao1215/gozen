package play

import "testing"

// The title's menu keeps clear of the key hint under it however many entries it has, and
// the hint stays inside the screen.
func TestTitleMenuKeepsClearOfTheHint(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "-"}, "schema_version: 4\nui: {profiles: true, bestiary: true}\n", "")
	sw, sh := g.screen()
	m := g.titleMenu()
	if len(m.items) < 6 {
		t.Fatalf("%d entries, want the long menu", len(m.items))
	}
	if bottom := m.y + m.height(); bottom > sh-titleHintRoom {
		t.Errorf("the menu reaches %g, into the hint's room under %g", bottom, sh-titleHintRoom)
	}
	for _, tw := range []float64{100, 700, sw - 40} {
		if x := titleHintX(g, tw); x-tw/2 < 16 || x+tw/2 > sw-16 {
			t.Errorf("a hint %g wide centred at %g leaves the screen", tw, x)
		}
	}
	newTitle(g).draw(g, btlImage())
}
