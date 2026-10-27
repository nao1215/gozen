package play

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/chat"
	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
)

// streamLobby lists the live channels: a large first card with its chat beside it,
// smaller cards for the others, and on the left the channels live and offline (and, when
// the lobby is a place, the place's entries).
type streamLobby struct {
	done  func() // nil: the lobby is the place's scene
	key   string // the live shows the cards were made for
	cards []*lobbyCard
	chat  *streamChat // the first card's chat
	crowd *chat.Crowd
	open  time.Time
	sel   int // the card (then the entry) chosen with the keys
	age   int
	// scroll is how far the cards are scrolled up.
	scroll float64
}

// lobbyCard is one live show in the lobby.
type lobbyCard struct {
	show    *broadcast.Show
	bodies  []*puppet.Puppet
	beat    int
	talk    time.Time
	viewers int
}

func newStreamLobby(g *Game, done func()) *streamLobby {
	g.stepStreams()
	return &streamLobby{done: done, open: g.streamNow()}
}

// rebuild makes the cards again when the live shows changed.
func (l *streamLobby) rebuild(g *Game) {
	live := g.liveShows()
	keys := make([]string, len(live))
	for i, s := range live {
		keys[i] = s.Key + s.Start.String()
	}
	key := strings.Join(keys, "|")
	if key == l.key && l.cards != nil {
		return
	}
	l.key = key
	l.close()
	l.cards = nil
	for _, s := range live {
		c := &lobbyCard{show: s, beat: -1, viewers: 300 + g.desk.rng.IntN(4000)}
		for i, h := range s.Hosts {
			c.bodies = append(c.bodies, g.channelPuppet(h, fmt.Sprintf("lobby:%s:%d", s.Key, i)))
		}
		l.cards = append(l.cards, c)
	}
	l.chat, l.crowd = newStreamChat(lobbyChatW-28), g.newCrowd()
	l.sel = min(l.sel, max(0, len(l.cards)-1))
}

func (l *streamLobby) close() {
	for _, c := range l.cards {
		closeBodies(c.bodies)
	}
}

// entries are the place's entries, listed under the channels when the lobby is a place.
func (l *streamLobby) entries(g *Game) []world.Entry {
	if l.done != nil {
		return nil
	}
	return world.Entries(g.db, g.st)
}

// The layout of the lobby.
const (
	lobbySideW = 260.0
	lobbyChatW = 300.0
	lobbyGap   = 16.0
)

// cardRect is where the i-th card's picture is: the first large, the others four in a row.
func (l *streamLobby) cardRect(g *Game, i int) (x, y, w, h float64) {
	sw, _ := g.screen()
	x0, mw := lobbySideW+24, sw-lobbySideW-48
	if i == 0 {
		w = mw - lobbyChatW
		return x0, streamBarH + 20, w, w * 9 / 16
	}
	_, fy, _, fh := l.cardRect(g, 0)
	w = (mw - 3*lobbyGap) / 4
	h = w * 9 / 16
	row, col := (i-1)/4, (i-1)%4
	return x0 + float64(col)*(w+lobbyGap), fy + fh + 66 + float64(row)*(h+56), w, h
}

// shown is where the i-th card is on the screen, scrolled.
func (l *streamLobby) shown(g *Game, i int) (x, y, w, h float64) {
	x, y, w, h = l.cardRect(g, i)
	return x, y - l.scroll, w, h
}

// follow scrolls so the chosen card, with the names under it, is in view.
func (l *streamLobby) follow(g *Game) {
	if l.sel >= len(l.cards) {
		return
	}
	_, sh := g.screen()
	_, y, _, h := l.cardRect(g, l.sel)
	_, top, _, _ := l.cardRect(g, 0)
	l.scroll = max(0, min(y-top, max(l.scroll, y+h+60-sh)))
}

// sideRow is where the i-th row of the left list is.
func sideRow(i int) (y float64) { return streamBarH + 44 + float64(i)*48 }

func (l *streamLobby) update(g *Game) {
	l.age++
	l.rebuild(g)
	now := g.streamNow()
	in := &g.in
	if g.desk.away > 0 {
		if in.ok() || in.cancel() || in.click {
			g.desk.away = 0
			g.sfx("decide")
		}
		return
	}
	for i, c := range l.cards {
		b := c.show.BeatAt(now)
		if b != c.beat && b < len(c.show.Beats) {
			c.beat = b
			beat := c.show.Beats[b]
			text := ""
			switch {
			case beat.Line != nil:
				text = beat.Line.Text
			case beat.Ask != nil:
				text = beat.Ask.Q
			case i == 0 && beat.Chat != nil:
				l.chat.post(chat.Message{From: g.db.Tr(beat.Chat[0]), Text: g.db.Tr(beat.Chat[1])})
			}
			if text != "" {
				c.talk = now.Add(seconds(min(3.5, 0.6+0.07*float64(len([]rune(text))))))
				if i == 0 {
					l.chat.post(chat.Message{From: g.channelName(c.show.Hosts[0]), Text: g.db.Tr(text), Values: map[string]float64{chatHost: 1}})
				}
			}
		}
		for _, p := range c.bodies {
			if p != nil {
				p.Motion.Speak(now.Before(c.talk))
				p.Motion.Step()
			}
		}
		if l.age%180 == 0 {
			c.viewers = max(20, c.viewers+g.desk.rng.IntN(25)-12)
		}
	}
	if l.chat != nil {
		g.keepCrowd(l.crowd)
		l.chat.pull(l.crowd, now.Sub(l.open))
	}
	entries := l.entries(g)
	n := len(l.cards) + len(entries)
	choose := func(i int) {
		if i < len(l.cards) {
			g.sfx("decide")
			g.push(newStreamWatch(g, l.cards[i].show, l.cards[i].viewers))
			return
		}
		e := entries[i-len(l.cards)]
		if e.Kind == world.Menu {
			g.sfx("decide")
			g.push(newMenu(g))
			return
		}
		g.sfx("decide")
		g.events.Start(e.Commands(g.db, g.st))
	}
	for i := range l.cards {
		if x, y, w, h := l.shown(g, i); y+h > streamBarH && g.clickedIn(x, y, w, h+50) || g.clickedIn(0, sideRow(i), lobbySideW, 48) {
			choose(i)
			return
		}
	}
	for i := range entries {
		if g.clickedIn(0, l.entryRow(g, i), lobbySideW, 40) {
			choose(len(l.cards) + i)
			return
		}
	}
	switch {
	case n == 0:
	case in.left() || in.up():
		l.sel = (l.sel + n - 1) % n
		g.sfx("cursor")
	case in.right() || in.down():
		l.sel = (l.sel + 1) % n
		g.sfx("cursor")
	case in.ok():
		choose(l.sel)
		return
	}
	l.follow(g)
	switch {
	case l.done != nil && in.cancel():
		g.sfx("cancel")
		l.close()
		g.pop()
		l.done()
	case l.done == nil && in.openList():
		g.sfx("decide")
		g.push(newMenu(g))
	}
}

// entryRow is where the i-th entry of the place is in the left list.
func (l *streamLobby) entryRow(g *Game, i int) float64 {
	return sideRow(len(l.cards)+len(l.offline(g))) + 30 + float64(i)*40
}

// offline are the channels not live.
func (l *streamLobby) offline(g *Game) []int {
	var out []int
	for i, c := range g.db.Streams.Channels {
		on := false
		for _, card := range l.cards {
			for _, h := range card.show.Hosts {
				on = on || h == c
			}
		}
		if !on {
			out = append(out, i)
		}
	}
	return out
}

func (l *streamLobby) draw(g *Game, dst *ebiten.Image) {
	sw, sh := g.screen()
	dst.Fill(streamPage)
	if len(l.cards) == 0 {
		gfx.DrawTextLeft(dst, lang.T("今は誰も配信していません。"), lobbySideW+24, streamBarH+30, 18, streamInk2)
	}
	for i, c := range l.cards {
		x, y, w, h := l.shown(g, i)
		if y > sh {
			break
		}
		if y+h+60 < streamBarH {
			continue
		}
		l.drawCard(g, dst, c, i, x, y, w, h)
	}
	l.drawSide(g, dst, sh)
	g.streamTop(dst, "")
	if g.desk.away > 0 {
		fill(dst, withAlpha(colorBlack, 0x80))
		bx, by := sw/2-260, sh/2-110
		gfx.FillRoundRect(dst, float32(bx), float32(by), 520, 200, 12, streamPanel)
		gfx.DrawText(dst, lang.T("おかえりなさい"), sw/2, by+26, 22, streamInk)
		gfx.DrawText(dst, fmt.Sprintf(lang.T("いない間に %s たまりました。"), g.money(g.desk.away)), sw/2, by+76, 17, streamInk2)
		gfx.FillRoundRect(dst, float32(sw/2-60), float32(by+130), 120, 40, 6, streamBrand)
		gfx.DrawText(dst, lang.T("受け取る"), sw/2, by+139, 16, streamPanel)
	}
	g.drawToast(dst)
}

// drawCard draws a card: the picture with its hosts, LIVE and the viewers, the first
// card's caption and chat, and the names under it.
func (l *streamLobby) drawCard(g *Game, dst *ebiten.Image, c *lobbyCard, i int, x, y, w, h float64) {
	host := c.show.Hosts[0]
	if i == 0 {
		gfx.FillRoundRect(dst, float32(x-1), float32(y-1), float32(w+lobbyChatW+2), float32(h+64), 8, streamRule)
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w+lobbyChatW), float32(h+62), 8, streamPanel)
	}
	n := len(c.show.Hosts)
	for k, hc := range c.show.Hosts {
		shift, zoom := 0.0, 1.0
		if n > 1 {
			shift, zoom = 0.22-0.44*float64(k), 1.25
		}
		g.drawFramed(dst, hc, c.bodies[k], x, y, w, h, shift, zoom, k == 0)
	}
	if i == l.sel {
		gfx.StrokeRoundRect(dst, float32(x+1), float32(y+1), float32(w-2), float32(h-2), 4, 3, host.Color.Or(streamBrand))
	}
	gfx.FillRoundRect(dst, float32(x+8), float32(y+8), 42, 20, 3, streamLive)
	gfx.DrawText(dst, "LIVE", x+29, y+9, 13, streamPanel)
	views := fmt.Sprintf(lang.T("%d人が視聴中"), c.viewers)
	vw := gfx.TextWidth(views, 13)
	rect(dst, x+56, y+8, vw+12, 20, withAlpha(colorBlack, 0xa0))
	gfx.DrawTextLeft(dst, views, x+62, y+9, 13, streamPanel)
	size, pad := 13.0, 0.0
	my := y + h + 8
	if i == 0 {
		size, pad = 15, 12
		my = y + h + 10
		if b := c.beat; b >= 0 && b < len(c.show.Beats) {
			text := ""
			if bt := c.show.Beats[b]; bt.Line != nil {
				text = bt.Line.Text
			} else if bt.Ask != nil {
				text = bt.Ask.Q
			}
			if text != "" {
				line := gfx.Ellipsis(g.db.Tr(text), 18, w*0.8)
				lw := gfx.TextWidth(line, 18)
				rect(dst, x+w/2-lw/2-6, y+h-36, lw+12, 26, withAlpha(colorBlack, 0xb8))
				gfx.DrawText(dst, line, x+w/2, y+h-33, 18, streamPanel)
			}
		}
		rect(dst, x+w, y, 1, h+62, streamRule)
		g.drawChat(dst, l.chat, x+w+14, y+10, h+40, g.money)
	}
	g.drawAvatar(dst, host, x+pad, my, 36)
	tx := x + 46 + pad
	gfx.DrawTextLeft(dst, gfx.Ellipsis(g.showName(c.show), size+1, w-60), tx, my, size+1, streamInk)
	sub := g.db.Tr(c.show.Episode.Title)
	if i == 0 && len(c.show.Episode.Lobby) > 0 {
		sub = g.db.Tr(c.show.Episode.Lobby[(l.age/300)%len(c.show.Episode.Lobby)])
	}
	gfx.DrawTextLeft(dst, gfx.Ellipsis(sub, size-1, w-60), tx, my+size+6, size-1, streamInk2)
	if i == 0 {
		tag := g.db.Tr(host.Tag)
		tw := gfx.TextWidth(tag, 13)
		ax := x + w - tw - 120
		gfx.FillRoundRect(dst, float32(ax), float32(my+4), float32(tw+16), 22, 11, streamPage)
		gfx.DrawTextLeft(dst, tag, ax+8, my+6, 13, streamInk2)
		g.drawStars(dst, x+w-96, my+9, g.step(host))
	}
}

// drawSide draws the left list: the live channels, those offline and the place's entries.
func (l *streamLobby) drawSide(g *Game, dst *ebiten.Image, sh float64) {
	rect(dst, 0, streamBarH, lobbySideW, sh-streamBarH, streamPanel)
	rect(dst, lobbySideW-1, streamBarH, 1, sh-streamBarH, streamRule)
	gfx.DrawTextLeft(dst, lang.T("配信中のチャンネル"), 16, streamBarH+14, 14, streamInk2)
	row := 0
	for _, c := range l.cards {
		y := sideRow(row)
		h := c.show.Hosts[0]
		g.drawAvatar(dst, h, 14, y+6, 32)
		gfx.DrawTextLeft(dst, gfx.Ellipsis(g.showName(c.show), 15, 140), 56, y+4, 15, streamInk)
		gfx.DrawTextLeft(dst, gfx.Ellipsis(g.db.Tr(h.Tag), 13, 140), 56, y+24, 13, streamInk3)
		gfx.FillCircle(dst, float32(lobbySideW-62), float32(y+14), 4, streamLive, true)
		gfx.DrawTextLeft(dst, strconv.Itoa(c.viewers), lobbySideW-54, y+6, 13, streamInk2)
		row++
	}
	for _, i := range l.offline(g) {
		y := sideRow(row)
		c := g.db.Streams.Channels[i]
		g.drawAvatar(dst, c, 14, y+6, 32)
		gfx.DrawTextLeft(dst, g.channelName(c), 56, y+4, 15, streamInk3)
		gfx.DrawTextLeft(dst, lang.T("オフライン"), 56, y+24, 13, streamInk3)
		row++
	}
	entries := l.entries(g)
	for i, e := range entries {
		y := l.entryRow(g, i)
		if len(l.cards)+i == l.sel {
			rect(dst, 0, y, lobbySideW-1, 40, streamPage)
		}
		gfx.DrawTextLeft(dst, gfx.Ellipsis(e.Label, 15, lobbySideW-32), 16, y+10, 15, streamInk)
	}
}
