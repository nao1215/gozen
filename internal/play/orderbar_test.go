package play

import (
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// campaign.order_view: icons offers a region's orders as a row of icons: left and right
// choose, Cancel closes, OK gives the order; campaign.end_turn_button ends the turn with
// its key or a click.
func TestOrderBarIconsAndEndTurnButton(t *testing.T) {
	g, as := campaignOpen(t, map[string]string{
		"turns: 5":       "turns: 5\n  order_view: icons\n  end_turn_button: {icon: keep.png}\n  end_turn: Next month",
		"label: Attack,": "label: Attack, icon: keep.png,",
		"label: Storm,":  "label: Storm, group: Siege,",
		"unknown: named": "unknown: named\n      safe_area: [0.1, 0.17, 0.08, 0]\n      camera: {zoom: 1.4}",
	})
	wldPlay(t, g, "press ok\nwait 10\n", 100)
	b, ok := g.stack[len(g.stack)-1].(*orderBar)
	if !ok {
		t.Fatalf("OK on the region opened %T, not the row of icons", g.stack[len(g.stack)-1])
	}
	if len(b.items) != 2 || b.items[0].label != "Attack" || b.items[0].icon != "keep.png" || b.items[1].icon != "" {
		t.Fatalf("items %+v", b.items)
	}
	sw, sh := g.screen()
	win, buttons := b.layout(g)
	if win.x < 0 || win.x+win.w > sw || win.y+win.h > sh || buttons[1].x-(buttons[0].x+buttons[0].w) <= b.gap(0, buttons[0].w) {
		t.Errorf("row %+v, buttons %+v: off the screen, or no wider gap between groups", win, buttons)
	}
	for _, c := range as.cameraButtons(g) {
		if win.overlap(c) > 0 {
			t.Errorf("the row %+v covers the camera button %+v", win, c)
		}
	}
	wldFramesDrawn(t, g, 3) // the row over the map, its icon and its square of words
	wldPlay(t, g, "press right\nwait 4\n", 50)
	if b.sel != 1 {
		t.Fatalf("right chose %d", b.sel)
	}
	wldPlay(t, g, "press cancel\nwait 4\n", 50)
	if as.barOpen(g) || len(g.stack) != 1 {
		t.Fatal("cancel did not close the row")
	}
	wldPlay(t, g, "press ok\nwait 10\npress ok\nwait 10\n", 100)
	if as.barOpen(g) || !g.busy() {
		t.Fatal("OK on Attack did not give the order")
	}
	wldUntil(t, g, 2000, func() bool {
		wldPlay(t, g, "press ok\nwait 10\n", 100)
		return !g.busy()
	})
	if g.busy() {
		t.Fatal("the order did not play out")
	}
	if owner := data.RegionOwner(g.db, g.st.Campaign(), "field"); owner != "blue" {
		t.Fatalf("after the attack the field is held by %q", owner)
	}

	r, ok := as.endTurnRect(g)
	if !ok || r.x+r.w > sw {
		t.Fatalf("end turn button %+v (shown %v)", r, ok)
	}
	if gen, ok := as.generalsRect(g); ok && r.overlap(gen) > 0 {
		t.Errorf("the button %+v covers the generals %+v", r, gen)
	}
	turn := data.Turn(g.db, g.st.Campaign())
	wldPlay(t, g, "press end_turn_key\nwait 60\n", 200)
	if got := data.Turn(g.db, g.st.Campaign()); got != turn+1 {
		t.Errorf("the end turn key left turn %d, want %d", got, turn+1)
	}
}
