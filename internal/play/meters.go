package play

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

const meterH = 44.0

func (g *Game) meterValue(m *gamedata.Meter) (v, mx float64) {
	mx = m.Max
	if mx == 0 {
		mx = 100
	}
	if m.MaxValue.Set() {
		value, err := m.MaxValue.Number(g.st.Env())
		if err == nil {
			mx = max(1, value)
		}
	}
	var err error
	v, err = m.Value.Number(g.st.Env())
	if err != nil {
		return 0, mx
	}
	return max(0, min(mx, v)), mx
}

func (g *Game) shownMeters(actor string, page bool) []*gamedata.Meter {
	var out []*gamedata.Meter
	for i := range g.db.Game.Meters {
		m := &g.db.Game.Meters[i]
		if page && m.Actor != "" && m.Actor != actor {
			continue
		}
		if m.When.Set() {
			if ok, err := m.When.Bool(g.st.Env()); err != nil || !ok {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

func (g *Game) metersHeight() float64 {
	n := len(g.shownMeters("", false))
	if n == 0 {
		return 0
	}
	return float64(n)*(meterH+8) - 8
}

func (g *Game) drawMeters(dst *ebiten.Image, ms []*gamedata.Meter, x, y, w float64) {
	for _, m := range ms {
		g.window(dst, x, y, w, meterH)
		g.drawMeterRow(dst, m, x+14, y+10, w-28)
		y += meterH + 8
	}
}

func (g *Game) drawMeterRow(dst *ebiten.Image, m *gamedata.Meter, x, y, w float64) {
	size := sizeS * 0.8
	v, mx := g.meterValue(m)
	g.text(dst, m.Label, x, y, size, g.th.dim)
	bx := x + gfx.TextWidth(m.Label, size) + 12
	right := fmt.Sprintf("%d", int(v))
	if m.MaxValue.Set() {
		right = fmt.Sprintf("%d/%d", int(v), int(mx))
	}
	full := v >= mx
	if full && m.Full != "" {
		right = m.Full
	}
	rc := g.th.text
	c := m.Color.Or(color.NRGBA{0xff, 0x9c, 0xc8, 0xff})
	if full {
		c = m.FullColor.Or(c)
		rc = c
	}
	rw := gfx.TextWidth(right, size)
	bw := max(20, w-(bx-x)-rw-12)
	bar(dst, bx, y+8, bw, 10, v/mx, c)
	for _, k := range m.Marks {
		if k > 0 && k < mx {
			mxp := float32(bx + bw*k/mx)
			gfx.FillRoundRect(dst, mxp-1, float32(y+4), 2, 18, 1, color.NRGBA{0xff, 0xff, 0xff, 0xc0})
		}
	}
	gfx.DrawTextRight(dst, right, x+w, y, size, rc)
}
