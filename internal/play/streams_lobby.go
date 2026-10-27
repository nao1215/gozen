package play

import (
	"cmp"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/internal/chat"
	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
)

// streamLobby lists the live channels: a large first card with its chat beside it,
// smaller cards for the others and a row of the channels offline after them, and, when the
// lobby is a place, the place's entries in the bar.
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
	// name is the field over the lobby that asks the viewer's name on the first opening
	// (nil once it is given).
	name *textField
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
	l := &streamLobby{done: done, open: g.streamNow()}
	if g.st.Streams.Viewer == "" {
		l.name = &textField{}
		l.name.start(g, func() (float64, float64) {
			sw, sh := g.screen()
			return l.name.caretX(sw/2-220, 17), sh/2 - 19
		})
	}
	return l
}

// The box that asks the viewer's name, centred on the screen, and its button.
const (
	nameBoxW = 520.0
	nameBoxH = 220.0
)

// updateName reads the viewer's name, asked over the lobby on the first opening, and keeps
// it with the streams' progress.
func (l *streamLobby) updateName(g *Game) {
	in := &g.in
	if in.devices() && l.name.readTyping() {
		return
	}
	ok := !in.devices() && in.pressed[btnOK] || in.devices() && (inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter))
	sw, sh := g.screen()
	if !ok && !g.clickedIn(sw/2-60, sh/2+40, 120, 40) {
		return
	}
	name := strings.TrimSpace(l.name.take())
	g.st.Streams.Viewer = cmp.Or(string([]rune(name)[:min(utf8.RuneCountInString(name), 16)]), lang.T("名無しの視聴者"))
	l.name = nil
	g.sfx("decide")
	g.saveStreams()
}

// drawName draws the box that asks the viewer's name over the lobby.
func (l *streamLobby) drawName(dst *ebiten.Image, sw, sh float64) {
	fill(dst, withAlpha(colorBlack, 0x80))
	bx, by := sw/2-nameBoxW/2, sh/2-120
	gfx.FillRoundRect(dst, float32(bx), float32(by), nameBoxW, nameBoxH, 12, streamPanel)
	gfx.DrawTextLeft(dst, lang.T("あなたの名前"), bx+28, by+22, 20, streamInk)
	gfx.DrawTextLeft(dst, lang.T("配信者に読まれる名前です。"), bx+28, by+56, 15, streamInk2)
	gfx.StrokeRoundRect(dst, float32(bx+28), float32(by+90), nameBoxW-56, 44, 6, 1.5, streamBrand)
	if l.name.text+l.name.comp == "" {
		gfx.DrawTextLeft(dst, lang.T("名無しの視聴者"), bx+40, by+101, 17, streamInk3)
	}
	l.name.draw(dst, bx+40, by+101, nameBoxW-80, 17, streamInk, streamBrand, true, l.age)
	gfx.FillRoundRect(dst, float32(sw/2-60), float32(sh/2+40), 120, 40, 6, streamBrand)
	gfx.DrawText(dst, lang.T("決める"), sw/2, sh/2+49, 16, streamPanel)
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

// entries are the place's entries, listed in the bar when the lobby is a place.
func (l *streamLobby) entries(g *Game) []world.Entry {
	if l.done != nil {
		return nil
	}
	return world.Entries(g.db, g.st)
}

// The layout of the lobby.
const (
	lobbyChatW = 300.0
	lobbyGap   = 16.0
	lobbyEdge  = 24.0 // the margin left and right of the cards
)

// cardRect is where the i-th card's picture is: the first large, the others four in a row.
func (l *streamLobby) cardRect(g *Game, i int) (x, y, w, h float64) {
	sw, _ := g.screen()
	x0, mw := lobbyEdge, sw-2*lobbyEdge
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

// follow scrolls so the chosen card, with the names under it, is in view, and the row of
// the channels offline with the last card.
func (l *streamLobby) follow(g *Game) {
	if l.sel >= len(l.cards) {
		return
	}
	_, sh := g.screen()
	_, y, _, h := l.cardRect(g, l.sel)
	_, top, _, _ := l.cardRect(g, 0)
	bottom := y + h + 60
	if l.sel == len(l.cards)-1 && len(l.offline(g)) > 0 {
		bottom = l.offlineTop(g) + offlineH
	}
	l.scroll = max(0, min(y-top, max(l.scroll, bottom-sh)))
}

// offlineH is the height of the row of the channels offline, with its heading.
const offlineH = 84.0

// offlineTop is where the row of the channels offline begins, unscrolled: after the cards.
func (l *streamLobby) offlineTop(g *Game) float64 {
	if len(l.cards) == 0 {
		return streamBarH + 70
	}
	_, y, _, h := l.cardRect(g, len(l.cards)-1)
	return y + h + 66
}

// entryRect is where the i-th entry of the place is in the bar: after the game's title.
func (l *streamLobby) entryRect(g *Game, i int) (x, y, w, h float64) {
	x = 16 + 30 + gfx.TextWidth(cmp.Or(g.db.Game.Title, lang.T("配信")), 18) + 32
	for k, e := range l.entries(g) {
		w = gfx.TextWidth(e.Label, 14) + 24
		if k == i {
			break
		}
		x += w + 8
	}
	return x, 11, w, 30
}

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
	if l.name != nil {
		l.updateName(g)
		return
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
		if x, y, w, h := l.shown(g, i); y+h > streamBarH && g.clickedIn(x, max(y, streamBarH), w, y+h+50-max(y, streamBarH)) {
			choose(i)
			return
		}
	}
	for i := range entries {
		if g.clickedIn(l.entryRect(g, i)) {
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
	case l.done == nil && !g.db.Game.NoMenus() && in.openList():
		g.sfx("decide")
		g.push(newMenu(g))
	}
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
		gfx.DrawTextLeft(dst, lang.T("今は誰も配信していません。"), lobbyEdge, streamBarH+30, 18, streamInk2)
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
	l.drawOffline(g, dst, sh)
	g.streamTop(dst, "")
	l.drawEntries(g, dst)
	if l.name != nil {
		l.drawName(dst, sw, sh)
	}
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
		g.drawChat(dst, l.chat, x+w+14, y+10, h+40, g.money, streamInk)
	}
	g.drawAvatar(dst, host, x+pad, my, 36)
	tx := x + 46 + pad
	gfx.DrawTextLeft(dst, gfx.Ellipsis(g.showName(c.show), size+1, w-60), tx, my, size+1, streamInk)
	// the episode's lobby lines in turn, else the channel's tagline or tag (never the title)
	sub := g.db.Tr(cmp.Or(host.Tagline, host.Tag))
	if lines := c.show.Episode.Lobby; len(lines) > 0 {
		sub = g.db.Tr(lines[(l.age/300)%len(lines)])
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

// drawOffline draws the channels offline in a row after the cards.
func (l *streamLobby) drawOffline(g *Game, dst *ebiten.Image, sh float64) {
	off := l.offline(g)
	y := l.offlineTop(g) - l.scroll
	if len(off) == 0 || y > sh || y+offlineH < streamBarH {
		return
	}
	gfx.DrawTextLeft(dst, lang.T("オフライン"), lobbyEdge, y, 14, streamInk2)
	x := lobbyEdge
	for _, i := range off {
		c := g.db.Streams.Channels[i]
		name := g.channelName(c)
		g.drawAvatar(dst, c, x, y+28, 28)
		gfx.DrawTextLeft(dst, name, x+36, y+33, 14, streamInk3)
		x += 36 + gfx.TextWidth(name, 14) + 24
		if sw, _ := g.screen(); x > sw-lobbyEdge {
			break
		}
	}
}

// drawEntries draws the place's entries as small buttons in the bar.
func (l *streamLobby) drawEntries(g *Game, dst *ebiten.Image) {
	for i, e := range l.entries(g) {
		x, y, w, h := l.entryRect(g, i)
		edge := streamRule
		if len(l.cards)+i == l.sel {
			edge = streamBrand
		}
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 15, streamPage)
		gfx.StrokeRoundRect(dst, float32(x), float32(y), float32(w), float32(h), 15, 1.5, edge)
		gfx.DrawText(dst, e.Label, x+w/2, y+6, 14, streamInk)
	}
}
