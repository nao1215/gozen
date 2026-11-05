//go:build graphics

package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
)

func TestEntryPartsDraw(t *testing.T) {
	g, s := openPartsBoard(t)
	if s.entries[3].Ticket.After[0] != "Bowl glazing" || len(s.entries[3].Ticket.After) != 1 {
		t.Fatal("ticket prerequisites were not interpolated", s.entries[3].Ticket.After)
	}
	if s.entries[4].Cost != "1 slot" {
		t.Fatal("cost was not rendered", s.entries[4].Cost)
	}
	for range 3 {
		g.Draw(covScreen())
		wldPlay(t, g, "press right\nwait 2\n", 20)
	}
	// The plate is not a focus stop and a click on it does nothing.
	wldPlay(t, g, "click 100 200\nwait 8\n", 40)
	if len(g.st.Vars) != 0 {
		t.Fatal("a plate acted", g.st.Vars)
	}
}

// Drawing uses UI coordinates at all supported capture scales. Pixel evidence
// is produced by the workboard capture recipe inside the Ebitengine loop.
func TestEntryPartsAtCaptureScales(t *testing.T) {
	g, _ := openPartsBoard(t)
	defer gfx.SetScale(1)
	for _, scale := range []float64{1, 1.5, 3} {
		gfx.SetScale(scale)
		dst := ebiten.NewImage(int(1280*scale), int(720*scale))
		g.Draw(dst)
		dst.Deallocate()
	}
}

func TestGuideVeilOwnership(t *testing.T) {
	g := openFlowHoldGame(t)
	g.guideVeil = ebiten.NewImage(1920, 1080)
	g.reloadAssets()
	if g.guideVeil != nil {
		t.Fatal("reload retained veil")
	}
	g.guideVeil = ebiten.NewImage(3840, 2160)
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if g.guideVeil != nil {
		t.Fatal("Close retained veil")
	}
}
