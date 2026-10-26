package play

import (
	"testing"
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
