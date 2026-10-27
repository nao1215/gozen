package play

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The radio of the watch view (ADR 0052): the show is heard rather than watched. The lines
// are always read aloud (their recorded voices, else speech synthesis) and each waits for
// its reading to end; the chat messages the show picks up (its chat beats, the answer to a
// question, the comment the streamer answers) are read in the viewers' voice; at the end of
// a show it tunes to another show on the air. The screen is the streamer's face, the name,
// the line in large letters and the buttons to go on to the next show and to stop the radio.
// The choice is a setting (stream_radio), kept for every save.

// radioTune is how long the radio stays on a show that ended before it tunes to another.
const radioTune = 3 * time.Second

// The viewers' voice: a pitch apart from the streamer's and a little faster.
const (
	viewerPitchLow, viewerPitchHigh = 0.7, 1.4
	viewerRate                      = 1.15
)

// toggleRadio turns the radio on or off, kept in the settings.
func (w *streamWatch) toggleRadio(g *Game) {
	g.cfg.StreamRadio = !g.cfg.StreamRadio
	g.sfx("decide")
	if err := g.saveConfig(); err != nil {
		g.failure = err
	}
	if !g.cfg.StreamRadio {
		w.hush()
		keepAwake(false)
	}
}

// keepRadio keeps the screen awake while the radio plays (a browser, every few seconds,
// as the page may have lost the lock when it was hidden), and tunes to another show a
// while after the show ended.
func (w *streamWatch) keepRadio(g *Game) {
	if w.age%300 == 1 {
		keepAwake(true)
	}
	if w.ended && !g.streamNow().Before(w.endAt) {
		w.tune(g)
	}
}

// tune goes on to the next show on the air after this one, in the order of the lineup,
// and reports whether there is one.
func (w *streamWatch) tune(g *Game) bool {
	next := w.nextShow(g)
	if next == nil {
		return false
	}
	w.close(g)
	g.push(newStreamWatch(g, next, w.viewers))
	return true
}

// nextShow is the show on the air after this one in the order of the lineup, or nil.
func (w *streamWatch) nextShow(g *Game) *broadcast.Show {
	shows, now := g.desk.shows, g.streamNow()
	at := 0
	for i, s := range shows {
		if s == w.show {
			at = i
		}
	}
	for k := 1; k <= len(shows); k++ {
		if s := shows[(at+k)%len(shows)]; s != w.show && s.Live(now) {
			return s
		}
	}
	return nil
}

// hear reads a viewer's message in the viewers' voice (speech synthesis; captioned only
// outside a browser), the next step waiting for it.
func (w *streamWatch) hear(g *Game, from, text string) {
	w.heard = from + "  " + text
	w.busy = g.streamNow().Add(seconds(g.db.Streams.Timing.Chat))
	pitch := viewerPitchHigh
	if v := w.show.Hosts[0].Voice; v.Pitch > 1 {
		pitch = viewerPitchLow
	}
	w.speakAloud(g, text, pitch, viewerRate)
}

// hearFirst has the radio read a viewer's message before the lines queued (the comment or
// the answer they reply to).
func (w *streamWatch) hearFirst(g *Game, from, text string) {
	if g.cfg.StreamRadio && text != "" {
		w.queue = append([]streamSay{{from: from, text: text}}, w.queue...)
	}
}

// radioButton is where the radio's button i is: 0 the next show, 1 stop.
func (g *Game) radioButton(i int) (x, y, bw, bh float64) {
	sw, sh := g.screen()
	bw, bh = 150, 48
	return sw/2 - bw - 10 + float64(i)*(bw+20), sh - bh - 40, bw, bh
}

// radioInput reads the radio's buttons and keys: the next show (right, OK), stop (cancel,
// the radio's key).
func (w *streamWatch) radioInput(g *Game) {
	in := &g.in
	x, y, bw, bh := g.radioButton(0)
	sx, sy, _, _ := g.radioButton(1)
	switch {
	case g.clickedIn(x, y, bw, bh) || in.right() || in.ok():
		if w.tune(g) {
			g.sfx("decide")
		} else {
			g.sfx("buzzer")
		}
	case g.clickedIn(sx, sy, bw, bh) || in.cancel() || in.pressed[btnAuto]:
		w.toggleRadio(g)
	}
}

// drawRadio draws the radio: the face of the streamer speaking, the name and the title,
// the line in large letters, the viewer's message being read, and the buttons.
func (w *streamWatch) drawRadio(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	dst.Fill(streamInk)
	c := w.show.Hosts[w.captionWho]
	d := min(sw, sh) * 0.22
	top := sh * 0.08
	g.drawAvatar(dst, c, sw/2-d/2, top, d)
	gfx.DrawText(dst, g.channelName(c), sw/2, top+d+12, 22, c.Color.Or(streamBrand))
	gfx.DrawText(dst, gfx.Ellipsis(g.db.Tr(w.show.Episode.Title), 15, sw-32), sw/2, top+d+44, 15, streamInk3)
	size := 30.0
	if g.view.tall {
		size = 24
	}
	text := w.caption
	if w.ended {
		text = lang.T("配信は終了しました")
	}
	y := top + d + 88
	for _, l := range gfx.Wrap(text, size, sw*0.86) {
		gfx.DrawText(dst, l, sw/2, y, size, streamPanel)
		y += gfx.LineHeight(size)
	}
	if w.heard != "" {
		y += 12
		for _, l := range gfx.Wrap(w.heard, 18, sw*0.8) {
			gfx.DrawText(dst, l, sw/2, y, 18, streamRule)
			y += gfx.LineHeight(18)
		}
	}
	for i, label := range []string{lang.T("次へ"), lang.T("止める")} {
		x, by, bw, bh := g.radioButton(i)
		if i == 0 {
			gfx.FillRoundRect(dst, float32(x), float32(by), float32(bw), float32(bh), 24, streamBrand)
		} else {
			gfx.StrokeRoundRect(dst, float32(x), float32(by), float32(bw), float32(bh), 24, 1.5, streamInk3)
		}
		gfx.DrawText(dst, label, x+bw/2, by+14, 16, streamPanel)
	}
	g.drawToast(dst)
}
