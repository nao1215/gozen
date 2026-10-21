package play

import "testing"

// TestDevPanelHoldsTheGame: while the panel's body is shown the game's screens do not update but the
// tick goes on; on the Frames tab the game goes on; the panel always opens on the State tab; and
// it is only there under --dev.
func TestDevPanelHoldsTheGame(t *testing.T) {
	g, _ := btlOpen(t, Options{Battle: "one", Development: true, ShotDir: t.TempDir(), Script: "wait 5\npress panel\nwait 100\npress panel\nwait 100\nquit\n"}, "", "")
	s := btlBattle(t, g)
	var opened, held, ticks0 int
	for range 400 {
		btlFrame(t, g)
		switch {
		case g.panel != nil && opened == 0:
			opened, held, ticks0 = 1, s.age, g.tick
		case g.panel != nil && g.tick-ticks0 == 40:
			if s.age != held {
				t.Fatalf("the battle moved while the panel was open: age %d -> %d", held, s.age)
			}
			opened = 2
		}
		if opened == 2 && g.panel == nil {
			break
		}
	}
	if opened != 2 {
		t.Fatalf("the panel did not stay open for 40 ticks (state %d)", opened)
	}
	if g.panel != nil {
		t.Fatal("the second press did not close the panel")
	}
	for range 10 {
		btlFrame(t, g)
	}
	if s.age == held {
		t.Fatal("the battle did not go on after the panel closed")
	}
}

// TestDevPanelOpensOnTheFirstTab: every time it opens, the panel is on the State tab, and it is not
// opened at all without --dev.
func TestDevPanelOpensOnTheFirstTab(t *testing.T) {
	g := btlPanel(t)
	g.togglePanel()
	g.panelTab(true)
	g.panelTab(true)
	if g.panel.tab != tabScenes {
		t.Fatalf("tab %d", g.panel.tab)
	}
	g.closePanel()
	g.togglePanel()
	if g.panel.tab != tabState || g.panel.sel != 0 {
		t.Fatalf("reopened on tab %d row %d", g.panel.tab, g.panel.sel)
	}
}

// TestDevPanelNeedsDevelopmentMode: the panel key does nothing in a game started without --dev.
func TestDevPanelNeedsDevelopmentMode(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp", ShotDir: t.TempDir(), Script: "wait 3\npress panel\nwait 10\nquit\n"}, "", "")
	btlRunToEnd(t, g, 100)
	if g.panel != nil {
		t.Fatal("the panel opened without --dev")
	}
}
