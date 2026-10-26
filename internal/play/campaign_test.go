package play

import (
	"math"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/geom"

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

// A map's safe_area fits the picture to the screen less its edges, and the camera brings
// the picture's corner to the edge of that part, not under the windows; the generals'
// window keeps right of the camera's buttons however many generals there are.
func TestCampaignSafeAreaAndGenerals(t *testing.T) {
	area := map[string]string{"unknown: named": "unknown: named\n      safe_area: [0.1, 0.2, 0.25, 0.05]"}
	g, as := campaignOpen(t, area)
	k, ox, oy, _ := as.fit(g)
	pw, ph := g.pixels()
	l, top, w, h := pw*0.05, ph*0.1, pw*0.75, ph*0.65
	if ox < l-1e-9 || oy < top-1e-9 || ox+640*k > l+w+1e-9 || oy+360*k > top+h+1e-9 {
		t.Errorf("the picture lies at (%g, %g) x%g, outside the safe part (%g, %g, %g, %g)", ox, oy, k, l, top, w, h)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	area["unknown: named"] += "\n      camera: {zoom: 4}"
	g, as = campaignOpen(t, area)
	as.cameraCenter = geom.Pt{}
	k, ox, oy, _ = as.fit(g)
	if math.Abs(ox-l) > 1e-6 || math.Abs(oy-top) > 1e-6 || k <= 0 {
		t.Errorf("the camera brings the corner to (%g, %g), want the safe part's corner (%g, %g)", ox, oy, l, top)
	}
	sw, sh := g.screen()
	bs := as.cameraButtons(g)
	for _, n := range []int{1, 5, 9} {
		x, y, bw, cell := as.generalsBox(g, n, sw, sh)
		b := bs[len(bs)-1]
		if x < b.x+b.w && y+cell+30 > b.y-40 || x < 0 || x+bw > sw || cell > 132 {
			t.Errorf("%d generals: window at %g wide %g (cell %g) meets the buttons or the edge", n, x, bw, cell)
		}
	}
	wldFramesDrawn(t, g, 2)
}

// map.talk darkens the map under a conversation, but not under the map's own question of
// a region's orders.
func TestCampaignMapUnderTalk(t *testing.T) {
	for _, view := range []string{"dim", "hide"} {
		t.Run(view, func(t *testing.T) {
			g, as := campaignOpen(t, map[string]string{"unknown: named": "unknown: named\n      talk: " + view})
			g.events.Start([]data.Command{{Say: &data.Say{Text: "News from the border."}}})
			wldFramesDrawn(t, g, 5)
			if !g.talk.active || as.talkView(g) != view {
				t.Fatalf("talk %v, view %q, want %q", g.talk.active, as.talkView(g), view)
			}
			wldUntil(t, g, 300, func() bool {
				wldPlay(t, g, "press ok\nwait 10\n", 100)
				return !g.talk.active
			})
			if g.talk.active {
				t.Fatal("the line did not close")
			}
			wldPlay(t, g, "press ok\nwait 20\n", 100) // the region's orders
			wldFramesDrawn(t, g, 2)
			if !g.talk.active || as.talkView(g) != "" {
				t.Fatalf("under the region's orders: talk %v, view %q, want none", g.talk.active, as.talkView(g))
			}
		})
	}
}
