package play

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// campaignOpen opens the campaign of testdata/campaign on its map, its YAML changed by repl.
func campaignOpen(t *testing.T, repl map[string]string) (*Game, *atlasScene) {
	t.Helper()
	g := wldOpenWith(t, Options{FS: clashFiles(t, repl), Location: "war"})
	wldFrames(t, g, 10)
	as, ok := g.stack[0].(*atlasScene)
	if !ok || !as.campaignMap(g) {
		t.Fatalf("scene %T is not the campaign's map", g.stack[0])
	}
	return g, as
}

// The window of the region chosen never covers that region's point: it stays at the top
// left while the point is elsewhere, and moves away when the point lies under it.
func TestCampaignRegionWindowKeepsOffTheRegion(t *testing.T) {
	for _, tc := range []struct {
		name, at string
		left     bool
	}{
		{"elsewhere", "[320, 180]", true},
		{"under the window", "[60, 70]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, as := campaignOpen(t, map[string]string{"field: {at: [320, 180]}": "field: {at: " + tc.at + "}"})
			if got := as.choices[as.sel].Point; got != "field" {
				t.Fatalf("the map opened on %q, want field", got)
			}
			b := as.regionWindowBox(g, "field")
			px, py := as.screen(g, as.points[as.pointIndex("field")].at)
			if b.overlap(plateBox{x: px - 1, y: py - 1, w: 2, h: 2}) > 0 {
				t.Errorf("the window %+v covers the region's point (%g, %g)", b, px, py)
			}
			if left := b.x == 36 && b.y == 96; left != tc.left {
				t.Errorf("window at %+v, top left %v, want %v", b, left, tc.left)
			}
			wldFramesDrawn(t, g, 2) // the ring and the window draw
		})
	}
}

// Orders and the place's actions with a group are listed together under its heading, and
// pin_end_turn keeps the end of the turn last.
func TestCampaignListGroupsAndPinsEndTurn(t *testing.T) {
	g, _ := campaignOpen(t, map[string]string{
		"turns: 5":       "turns: 5\n  pin_end_turn: true\n  end_turn: Next month",
		"label: Attack,": "label: Attack, group: \"War {var.n}\",",
		"label: Storm,":  "label: Storm, group: Castle,",
	})
	g.db.Locations["war"].Actions = append(g.db.Locations["war"].Actions, data.Action{ID: "rest", Label: "Rest", Group: "War {var.n}"})
	wldPlay(t, g, "press menu\nwait 10\n", 100)
	l, ok := g.stack[len(g.stack)-1].(*entryList)
	if !ok {
		t.Fatalf("the menu key opened %T", g.stack[len(g.stack)-1])
	}
	var got []string
	for _, it := range l.menu.items {
		if it.heading {
			got = append(got, "#"+it.label)
		} else {
			got = append(got, it.label)
		}
	}
	want := "#War 0|Attack|Rest|#Castle|Storm|Menu|Next month"
	if s := strings.Join(got, "|"); s != want {
		t.Fatalf("rows %s, want %s", s, want)
	}
	if !l.menu.pin || l.menu.sel != 1 {
		t.Errorf("pin %v, cursor %d; want pinned, on Attack", l.menu.pin, l.menu.sel)
	}
	wldFramesDrawn(t, g, 2)
	wldPlay(t, g, "press up\nwait 4\npress ok\nwait 30\n", 200)
	if turn := data.Turn(g.db, g.st.Campaign()); turn != 2 {
		t.Errorf("choosing the pinned row left turn %d, want 2", turn)
	}
}

// campaign.heading replaces the turn's words of the band with its text, its expressions
// worked out; without it the band keeps the engine's words and the last turn.
func TestCampaignHeading(t *testing.T) {
	g, _ := campaignOpen(t, map[string]string{"turns: 5": "turns: 5\n  heading: \"Year {var.year + 1}, month {campaign.turn + 3}\""})
	if band := campaignBand(g); !strings.HasPrefix(band, "Year 1, month 4") || strings.Contains(band, "5") {
		t.Fatalf("band %q, want the heading and no last turn", band)
	}
	wldFramesDrawn(t, g, 2)
	g.db.Game.Campaign.Heading = ""
	if band := campaignBand(g); !strings.Contains(band, "5") || !strings.Contains(band, "Blue") {
		t.Fatalf("default band %q has no last turn or faction", band)
	}
}
