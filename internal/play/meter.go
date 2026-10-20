package play

import (
	"fmt"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The frame meter: the ticks and frames a second as Ebitengine counts them, and the
// longest and the average Update and Draw of the last second, in a band at the bottom
// of the screen. It is for telling whether the game keeps up, on a phone above all.
// A browser build shows it when the page is opened with ?fps (meterAsked) and puts its
// numbers on the page as window.gozenStats too (publishMeter); with --dev, F8 shows or
// hides it. Otherwise nothing of it runs.

// frameMeter is what the meter measures over a second, and the text it shows for the next.
type frameMeter struct {
	on              bool
	since           time.Time
	updMax, drawMax time.Duration
	updSum, drawSum time.Duration
	updates, draws  int
	text            string
}

// toggle shows or hides the meter; shown again, it starts a new second.
func (m *frameMeter) toggle() {
	*m = frameMeter{on: !m.on, since: time.Now()}
}

// updated counts an Update that began at start and, once a second has gone by, makes
// the meter's text from the second's numbers.
func (m *frameMeter) updated(start time.Time) {
	d := time.Since(start)
	m.updMax = max(m.updMax, d)
	m.updSum += d
	m.updates++
	if time.Since(m.since) < time.Second {
		return
	}
	s := meterStats{
		TPS: ebiten.ActualTPS(), FPS: ebiten.ActualFPS(),
		UpdateMax: ms(m.updMax), UpdateAvg: ms(m.updSum) / float64(max(1, m.updates)),
		DrawMax: ms(m.drawMax), DrawAvg: ms(m.drawSum) / float64(max(1, m.draws)),
	}
	m.text = fmt.Sprintf("TPS %.1f  FPS %.1f\n", s.TPS, s.FPS) +
		fmt.Sprintf(lang.T("Update 最大 %.1fms・平均 %.1fms   Draw 最大 %.1fms・平均 %.1fms"), s.UpdateMax, s.UpdateAvg, s.DrawMax, s.DrawAvg)
	publishMeter(s)
	*m = frameMeter{on: true, since: time.Now(), text: m.text}
}

// drawn counts a Draw that began at start.
func (m *frameMeter) drawn(start time.Time) {
	d := time.Since(start)
	m.drawMax = max(m.drawMax, d)
	m.drawSum += d
	m.draws++
}

// meterStats are the numbers of one second, as window.gozenStats holds them.
type meterStats struct {
	TPS, FPS             float64
	UpdateMax, UpdateAvg float64 // milliseconds
	DrawMax, DrawAvg     float64
}

// drawMeter draws the meter's text at the bottom of the screen, on a dark band.
func (g *Game) drawMeter(screen *ebiten.Image) {
	if !g.meter.on || g.meter.text == "" {
		return
	}
	sw, sh := g.screen()
	const size, bandH = 16.0, 52.0
	gfx.FillRect(screen, 0, float32(sh-bandH), float32(sw), bandH, color.NRGBA{0, 0, 0, 0xb0}, false)
	gfx.DrawTextLeft(screen, g.meter.text, 12, sh-bandH+4, size, color.White)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
