package play

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

// callSharer is the participant sharing a screen in this call (call_view.share), or
// -1: a share by someone not in the call is not drawn on it.
func (g *Game) callSharer(all []ensembleMember) int {
	if g.callShare == nil {
		return -1
	}
	for i, m := range all {
		if m.id == g.callShare.Who || g.memberActor(m) == g.callShare.Who {
			return i
		}
	}
	return -1
}

// drawCallShare keeps the shared picture in UI coordinates above the caption.
func (g *Game) drawCallShare(dst *ebiten.Image, v data.PlanningView, all []ensembleMember, l callLayout) bool {
	i := g.callSharer(all)
	if i < 0 {
		return false
	}
	r := l.main
	if r.w <= 0 || r.h <= 0 {
		return false
	}
	rect(dst, r.x, r.y, r.w, r.h, color.NRGBA{14, 16, 20, 255})
	seen := seenOr(l.seen, r.h)
	const pad = 8.0
	img := g.pics.get(g.callShare.Picture)
	if img != nil {
		if bw, bh := r.w-2*pad, seen-2*pad; bw > 0 && bh > 0 {
			gfx.DrawImageFit(dst, img, r.x+pad, r.y+pad, bw, bh, 1)
		}
	}
	_, _, ring, _, _ := callColors(v.Call)
	name := all[i].name
	gfx.FillRoundRect(dst, float32(r.x+pad), float32(r.y+5), float32(min(r.w-2*pad, 60+float64(len([]rune(name)))*14)), 22, 6, color.NRGBA{18, 24, 31, 200})
	gfx.FillCircle(dst, float32(r.x+pad+14), float32(r.y+16), 9, ring, true)
	drawCallControl(dst, "share", r.x+pad+14, r.y+16, .5)
	g.drawPlanningText(dst, name, r.x+pad+30, r.y+8, r.w-2*pad-34, 13, 1, color.White)
	return true
}
