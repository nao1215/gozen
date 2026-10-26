package play

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"slices"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/expr"
	"github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/rpg"
)

const meterH = 44.0

func (g *Game) meterValue(m *gamedata.Meter) (v, mx float64) {
	mx = m.Max
	if mx == 0 {
		mx = 100
	}
	if m.MaxValue.Set() {
		value, err := m.MaxValue.Number(g.viewEnv())
		if err == nil {
			mx = max(1, value)
		}
	}
	var err error
	v, err = m.Value.Number(g.viewEnv())
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
			if ok, err := m.When.Bool(g.viewEnv()); err != nil || !ok {
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

// meterNumber is what a number meter shows: its value, whole, not capped by max (0 when
// the value cannot be worked out).
func (g *Game) meterNumber(m *gamedata.Meter) string {
	v, err := m.Value.Number(g.viewEnv())
	if err != nil {
		v = 0
	}
	return strconv.Itoa(int(v))
}

func (g *Game) drawMeterRow(dst *ebiten.Image, m *gamedata.Meter, x, y, w float64) {
	size := sizeS * 0.8
	if m.Style == "number" { // the label and the value, uncapped: no bar
		g.text(dst, m.Label, x, y, size, g.th.dim)
		gfx.DrawTextRight(dst, g.meterNumber(m), x+w, y, size, m.Color.Or(g.th.text))
		return
	}
	v, mx := g.meterValue(m)
	g.text(dst, m.Label, x, y, size, g.th.dim)
	bx := x + gfx.TextWidth(m.Label, size) + 12
	right := strconv.Itoa(int(v))
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
	for i, k := range m.Marks {
		if k > 0 && k < mx {
			mxp := float32(bx + bw*k/mx)
			gfx.FillRoundRect(dst, mxp-1, float32(y+4), 2, 18, 1, color.NRGBA{0xff, 0xff, 0xff, 0xc0})
			if i < len(m.MarkLabels) && m.MarkLabels[i] != "" {
				gfx.DrawText(dst, m.MarkLabels[i], float64(mxp), y+21, size*0.6, g.th.dim)
			}
		}
	}
	gfx.DrawTextRight(dst, right, x+w, y, size, rc)
}

// goldShown reports whether the windows of the money show where the party is: not at a kind
// of place listed in ui.hide_gold.
func (g *Game) goldShown() bool {
	hide := g.db.Game.UI.HideGold
	if len(hide) == 0 {
		return true
	}
	kind := "field"
	if l := g.db.Locations[g.st.Location]; l != nil {
		switch {
		case l.Dungeon != nil:
			kind = "dungeon"
		case l.Stage != nil:
			kind = "stage"
		case l.Map != nil:
			kind = "map"
		}
	}
	return !slices.Contains(hide, kind)
}

// viewState is the state as the screen shows it: the play's, but rand() and chance() draw
// from a copy of the generator, so drawing (as often as the frames come) or previewing
// never moves the play's random numbers.
func (g *Game) viewState() *rpg.State {
	state := *g.st
	source := rand.PCG{}
	if g.pcg != nil {
		source = *g.pcg
	}
	state.Rand = rand.New(&source) //nolint:gosec // Display only, not security.
	return &state
}

// viewEnv is the names of viewState.
func (g *Game) viewEnv() expr.Env { return g.viewState().Env() }
