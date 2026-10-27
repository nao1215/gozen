package play

import (
	"cmp"
	"fmt"
	"image"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/nao1215/gozen/internal/chat"
	"github.com/nao1215/gozen/internal/layer2/puppetdef"
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
	// barH is the height of the bar over the lobby (barHeight).
	barH float64
	// scroll is how far the cards are scrolled up, and fling the finger's scrolling.
	scroll float64
	fling  fling
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
	// line is the caption: the show's line now (untranslated), and its lines as the card
	// shows them, wrapped from wrapFor (translated) to the width wrapW.
	line, wrapFor string
	wrapW         float64
	lines         []string
}

// lineAt is the caption of a show at now: the last line said or question asked, kept over
// the chat beats after it for captionLinger from its end (as the watch view's caption), and
// none after that.
func lineAt(s *broadcast.Show, now time.Time) string {
	b := s.BeatAt(now)
	for k := min(b, len(s.Beats)-1); k >= 0; k-- {
		bt := s.Beats[k]
		if bt.Line == nil && bt.Ask == nil {
			continue
		}
		if k < b && now.Sub(s.Start).Seconds() >= s.Times[k+1]+captionLinger.Seconds() {
			return ""
		}
		if bt.Line != nil {
			return bt.Line.Text
		}
		return bt.Ask.Q
	}
	return ""
}

func newStreamLobby(g *Game, done func()) *streamLobby {
	g.stepStreams()
	l := &streamLobby{done: done, open: g.streamNow(), barH: streamBarH}
	if g.st.Streams.Viewer == "" {
		l.name = &textField{}
		l.name.start(g, func() (float64, float64) {
			bx, by, _ := nameBox(g)
			return l.name.caretX(bx+40, 17), by + nameField + 11
		})
	}
	return l
}

// The box that asks the viewer's name, centred on the screen, and the box that shows the
// gold earned while away: as wide as boxW, narrower on a narrow screen.
const (
	boxW     = 520.0
	nameBoxH = 260.0
	// nameNoteSize is the letters of the notes under the name's hint, smaller when a note
	// would not fit in a line.
	nameNoteSize = 13.0
	// nameField and nameOK are how far under the box's top its field and its button are.
	nameField, nameOK = 126.0, 196.0
)

// nameNotes are the notes of the box that asks the viewer's name: the name cannot be
// changed later (nothing changes it), and the gold piles up while the game is closed when
// the streams earn any (away.per_hour).
func nameNotes(g *Game) []string {
	notes := []string{lang.T("名前は、あとから変えられません。")}
	if g.db.Streams.Away.PerHour > 0 {
		notes = append(notes, lang.T("見ていない間も、ゴールドが少しずつ貯まります。"))
	}
	return notes
}

// nameBox is where the box that asks the viewer's name is, and its width.
func nameBox(g *Game) (x, y, w float64) {
	sw, sh := g.screen()
	w = min(boxW, sw-32)
	return sw/2 - w/2, sh/2 - 120, w
}

// updateName reads the viewer's name, asked over the lobby on the first opening, and keeps
// it with the streams' progress.
func (l *streamLobby) updateName(g *Game) {
	in := &g.in
	// the keys that were text are not Enter; the button is pressed even while a phone's
	// keyboard still composes (taking the name confirms it)
	typed := in.devices() && l.name.readTyping()
	ok := !in.devices() && in.pressed[btnOK] || in.devices() && (in.pad[btnOK] || !typed && (inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter)))
	sw, _ := g.screen()
	if _, by, _ := nameBox(g); !ok && !g.clickedIn(sw/2-60, by+nameOK, 120, 40) {
		return
	}
	name := strings.TrimSpace(l.name.take())
	g.st.Streams.Viewer = cmp.Or(string([]rune(name)[:min(utf8.RuneCountInString(name), 16)]), lang.T("名無しの視聴者"))
	l.name = nil
	g.sfx("decide")
	g.saveStreams()
}

// drawName draws the box that asks the viewer's name over the lobby.
func (l *streamLobby) drawName(g *Game, dst *ebiten.Image, sw float64) {
	fill(dst, withAlpha(colorBlack, 0x80))
	bx, by, bw := nameBox(g)
	gfx.FillRoundRect(dst, float32(bx), float32(by), float32(bw), nameBoxH, 12, streamPanel)
	gfx.DrawTextLeft(dst, lang.T("あなたの名前"), bx+28, by+22, 20, streamInk)
	gfx.DrawTextLeft(dst, lang.T("配信者に読まれる名前です。"), bx+28, by+54, 15, streamInk2)
	for i, n := range nameNotes(g) { // a line each
		lines, size := gfx.FitLines(n, nameNoteSize, bw-56, 1)
		gfx.DrawTextLeft(dst, lines[0], bx+28, by+80+float64(i)*20, size, streamInk2)
	}
	gfx.StrokeRoundRect(dst, float32(bx+28), float32(by+nameField), float32(bw-56), 44, 6, 1.5, streamBrand)
	if l.name.text+l.name.comp == "" {
		gfx.DrawTextLeft(dst, lang.T("名無しの視聴者"), bx+40, by+nameField+11, 17, streamInk3)
	}
	l.name.draw(dst, bx+40, by+nameField+11, bw-80, 17, streamInk, streamBrand, true, l.age)
	gfx.FillRoundRect(dst, float32(sw/2-60), float32(by+nameOK), 120, 40, 6, streamBrand)
	gfx.DrawText(dst, lang.T("決める"), sw/2, by+nameOK+9, 16, streamPanel)
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
	// a channel still on the air keeps its card, its viewers and its bodies (a show that
	// started again goes on in it), and the card chosen stays the same channel's
	old := map[string]*lobbyCard{}
	for _, c := range l.cards {
		old[c.show.Key] = c
	}
	chosen, entry := "", -1 // the channel or the entry of the place chosen
	switch {
	case l.sel < len(l.cards):
		chosen = l.cards[l.sel].show.Key
	case l.cards != nil:
		entry = l.sel - len(l.cards)
	}
	l.cards = nil
	for _, s := range live {
		c := old[s.Key]
		switch {
		case c == nil:
			c = &lobbyCard{show: s, beat: -1, viewers: 300 + g.desk.rng.IntN(4000)}
			for i, h := range s.Hosts {
				c.bodies = append(c.bodies, g.channelPuppet(h, fmt.Sprintf("lobby:%s:%d", s.Key, i)))
			}
		case c.show != s:
			c.show, c.beat = s, -1
		}
		delete(old, s.Key)
		l.cards = append(l.cards, c)
	}
	for _, c := range old {
		closeBodies(c.bodies)
	}
	// The most watched show comes first, as on a streaming site; the order holds until
	// the live shows change, the viewers changing meanwhile.
	slices.SortStableFunc(l.cards, func(a, b *lobbyCard) int { return cmp.Compare(b.viewers, a.viewers) })
	if l.chat == nil {
		l.chat, l.crowd = newStreamChat(lobbyChatW-28), g.newCrowd()
	}
	switch i := slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Key == chosen }); {
	case entry >= 0:
		l.sel = len(l.cards) + entry
	case i >= 0:
		l.sel = i
	default:
		l.sel = min(l.sel, max(0, len(l.cards)-1))
	}
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
	// in the portrait layout: the margin left and right of the cards, and the height of a
	// card's names with the space after them.
	tallEdge  = 12.0
	tallNames = 76.0
)

// beside reports whether the i-th card has its chat beside it: the first, in the landscape
// layout.
func (g *Game) beside(i int) bool { return i == 0 && !g.view.tall }

// cardRect is where the i-th card's picture is: the first large, the others four in a row;
// in the portrait layout, one under another across the screen.
func (l *streamLobby) cardRect(g *Game, i int) (x, y, w, h float64) {
	sw, _ := g.screen()
	x0, mw := lobbyEdge, sw-2*lobbyEdge
	if g.view.tall {
		x0, mw = tallEdge, sw-2*tallEdge
		w = mw
		h = w * 9 / 16
		return x0, l.barH + 16 + float64(i)*(h+tallNames), w, h
	}
	if i == 0 {
		w = mw - lobbyChatW
		return x0, l.barH + 20, w, w * 9 / 16
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
		bottom = l.end(g)
	}
	l.scroll = max(0, min(y-top, max(l.scroll, bottom-sh)))
}

// end is where the lobby's content ends, unscrolled: after the channels offline, or the
// last card's names.
func (l *streamLobby) end(g *Game) float64 {
	if off := l.offline(g); len(off) > 0 {
		spots := l.offlineSpots(g, off)
		return l.offlineTop(g) + float64(spots[len(spots)-1].Y) + 28 + 28
	}
	if len(l.cards) == 0 {
		return 0
	}
	_, y, _, h := l.cardRect(g, len(l.cards)-1)
	return y + h + 60
}

// swipe scrolls the cards with a finger moving over them, gliding on when it lifts, no
// farther than their ends.
func (l *streamLobby) swipe(g *Game) {
	if d := l.fling.step(&g.in, true); d != 0 {
		_, sh := g.screen()
		to := max(0, min(l.scroll-d, l.end(g)-sh))
		if to != l.scroll-d {
			l.fling.v = 0
		}
		l.scroll = to
	}
}

// offlineTop is where the row of the channels offline begins, unscrolled: after the cards.
func (l *streamLobby) offlineTop(g *Game) float64 {
	if len(l.cards) == 0 {
		return l.barH + 70
	}
	_, y, _, h := l.cardRect(g, len(l.cards)-1)
	return y + h + 66
}

func (l *streamLobby) update(g *Game) {
	l.age++
	bs := g.barButtons(l.entries(g))
	l.barH = barHeight(bs)
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
				if g.db.Streams.Bounces(text) {
					for _, p := range c.bodies {
						if p != nil {
							p.Motion.React(puppetdef.Reaction{Kind: "jiggle", Amount: g.db.Streams.Bounce.Amount})
						}
					}
				}
				if i == 0 {
					l.chat.post(chat.Message{From: g.channelName(c.show.Hosts[0]), Text: g.db.Tr(text), Values: map[string]float64{chatHost: 1}})
				}
			}
		}
		c.line = lineAt(c.show, now)
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
	if l.done == nil && g.pump() { // the commands of the place's entry taken, as a field runs them
		return
	}
	if l.name != nil {
		l.updateName(g)
		return
	}
	entries := l.entries(g)
	n := len(l.cards) + len(entries)
	l.sel = min(l.sel, max(0, n-1)) // an entry chosen may be gone (its when no longer holds)
	bs = g.barButtons(entries)      // the entries as they are after the commands taken
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
	l.swipe(g)
	for i := range l.cards {
		if x, y, w, h := l.shown(g, i); y+h > l.barH && g.clickedIn(x, max(y, l.barH), w, y+h+50-max(y, l.barH)) {
			choose(i)
			return
		}
	}
	if i := g.barClick(bs); i >= 0 {
		if bs[i].lang != "" {
			g.switchLanguage(bs[i].lang)
			return
		}
		choose(len(l.cards) + bs[i].entry)
		return
	}
	switch {
	case n == 0:
	case in.left() || in.up():
		l.sel = (l.sel + n - 1) % n
		g.sfx("cursor")
		l.follow(g)
	case in.right() || in.down():
		l.sel = (l.sel + 1) % n
		g.sfx("cursor")
		l.follow(g)
	case in.ok():
		choose(l.sel)
		return
	}
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
		if c.Guest { // no channel of its own to be offline
			continue
		}
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
	bs := g.barButtons(l.entries(g))
	l.barH = barHeight(bs)
	if len(l.cards) == 0 {
		gfx.DrawTextLeft(dst, lang.T("今は誰も配信していません。"), lobbyEdge, l.barH+30, 18, streamInk2)
	}
	for i, c := range l.cards {
		x, y, w, h := l.shown(g, i)
		if y > sh {
			break
		}
		if y+h+60 < l.barH {
			continue
		}
		l.drawCard(g, dst, c, i, x, y, w, h)
	}
	l.drawOffline(g, dst, sh)
	sel := slices.IndexFunc(bs, func(b barButton) bool { return b.lang == "" && len(l.cards)+b.entry == l.sel })
	g.streamTop(dst, "", bs, sel)
	if l.name != nil {
		l.drawName(g, dst, sw)
	}
	if g.desk.away > 0 {
		fill(dst, withAlpha(colorBlack, 0x80))
		bw := min(boxW, sw-32)
		bx, by := sw/2-bw/2, sh/2-110
		gfx.FillRoundRect(dst, float32(bx), float32(by), float32(bw), 200, 12, streamPanel)
		gfx.DrawText(dst, lang.T("おかえりなさい"), sw/2, by+26, 22, streamInk)
		gfx.DrawText(dst, fmt.Sprintf(lang.T("いない間に %s たまりました。"), g.money(g.desk.away)), sw/2, by+76, 17, streamInk2)
		gfx.FillRoundRect(dst, float32(sw/2-60), float32(by+130), 120, 40, 6, streamBrand)
		gfx.DrawText(dst, lang.T("受け取る"), sw/2, by+139, 16, streamPanel)
	}
	g.drawToast(dst)
}

// drawCard draws a card: the picture with its hosts, LIVE and the viewers, the first
// card's caption (and its chat beside it), and the names under it, larger on the first card
// and in the portrait layout.
func (l *streamLobby) drawCard(g *Game, dst *ebiten.Image, c *lobbyCard, i int, x, y, w, h float64) {
	host := c.show.Hosts[0]
	if g.beside(i) {
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
	big := i == 0 || g.view.tall
	badge := 13.0 // LIVE and the viewers
	if g.view.tall {
		badge = 15
	}
	gfx.Pop(func() {
		lw := gfx.TextWidth("LIVE", badge) + 14
		gfx.FillRoundRect(dst, float32(x+8), float32(y+8), float32(lw), float32(badge+7), 3, streamLive)
		gfx.DrawText(dst, "LIVE", x+8+lw/2, y+9, badge, streamPanel)
		views := fmt.Sprintf(lang.T("%d人が視聴中"), c.viewers)
		vw := gfx.TextWidth(views, badge)
		rect(dst, x+14+lw, y+8, vw+12, badge+7, withAlpha(colorBlack, 0xa0))
		gfx.DrawTextLeft(dst, views, x+20+lw, y+9, badge, streamPanel)
		if len(c.show.Hosts) > 1 { // a duo: a mark to see it at once, unlocked or come again
			collab := lang.T("コラボ")
			bx, bw := x+20+lw+vw+6, gfx.TextWidth(collab, badge)+14
			gfx.FillRoundRect(dst, float32(bx), float32(y+8), float32(bw), float32(badge+7), 3, streamBrand)
			gfx.DrawText(dst, collab, bx+bw/2, y+9, badge, streamPanel)
		}
	})
	size, pad := 13.0, 0.0
	my := y + h + 8
	if big {
		size, pad = 15, 12
		my = y + h + 10
	}
	if g.view.tall {
		pad = 0
	}
	l.drawLine(g, dst, c, x, y, w, h)
	if g.beside(i) {
		rect(dst, x+w, y, 1, h+62, streamRule)
		g.drawChat(dst, l.chat, x+w+14, y+10, lobbyChatW-28, h+40, g.money, streamInk)
	}
	g.drawAvatar(dst, host, x+pad, my, 36)
	tx := x + 46 + pad
	nameW := w - 60
	if big { // the tag and the stars on the right of the name
		tag := g.db.Tr(host.Tag)
		tw := gfx.TextWidth(tag, 14)
		stars := float64(g.db.Streams.MaxStep()) * 14
		ax := x + w - pad - stars - tw - 26
		gfx.FillRoundRect(dst, float32(ax), float32(my+3), float32(tw+16), 23, 11, streamPage)
		gfx.DrawTextLeft(dst, tag, ax+8, my+5, 14, streamInk2)
		g.drawStars(dst, x+w-pad-stars, my+9, g.step(host))
		nameW = ax - tx - 8
	}
	gfx.Pop(func() {
		gfx.DrawTextLeft(dst, gfx.Ellipsis(g.showName(c.show), size+1, nameW), tx, my, size+1, streamInk)
	})
	// the episode's lobby lines in turn, else the channel's tagline or tag (never the title)
	sub := g.db.Tr(cmp.Or(host.Tagline, host.Tag))
	if lines := c.show.Episodes[c.show.Part(c.beat)].Lobby; len(lines) > 0 {
		sub = g.db.Tr(lines[(l.age/300)%len(lines)])
	}
	gfx.DrawTextLeft(dst, gfx.Ellipsis(sub, size-1, w-60-pad), tx, my+size+6, size-1, streamInk2)
}

// drawLine draws the card's caption at the bottom of its picture: white letters on black
// bands, as large as the card's width allows (cardCaption), in two lines at most.
func (l *streamLobby) drawLine(g *Game, dst *ebiten.Image, c *lobbyCard, x, y, w, h float64) {
	if c.line == "" {
		return
	}
	size := cardCaption(w)
	if line := g.db.Tr(c.line); c.wrapFor != line || c.wrapW != w {
		c.wrapFor, c.wrapW = line, w
		c.lines = gfx.Wrap(line, size, w*0.86)
		if len(c.lines) > 2 {
			c.lines = c.lines[:2]
			c.lines[1] = gfx.Ellipsis(c.lines[1]+"…", size, w*0.86)
		}
	}
	lh := gfx.LineHeight(size)
	top := y + h - 8 - float64(len(c.lines))*lh
	for i, s := range c.lines {
		lw := gfx.TextWidth(s, size)
		ly := top + float64(i)*lh
		rect(dst, x+w/2-lw/2-6, ly, lw+12, lh, withAlpha(colorBlack, 0xb8))
		gfx.DrawText(dst, s, x+w/2, ly+1, size, streamPanel)
	}
}

// cardCaption is the size of a card's caption for the card's width: 18 on the large card,
// 13 on a small one of the landscape layout, 16 to 17 across a phone held upright.
func cardCaption(w float64) float64 { return min(18, max(12, w/22)) }

// offlineSpots are where the avatars of the channels offline are, from the start of their
// row: one after another, onto a next row when a row is full.
func (l *streamLobby) offlineSpots(g *Game, off []int) []image.Point {
	sw, _ := g.screen()
	edge := lobbyEdge
	if g.view.tall {
		edge = tallEdge
	}
	spots := make([]image.Point, len(off))
	x, y := edge, 28.0
	for k, i := range off {
		w := 36 + gfx.TextWidth(g.channelName(g.db.Streams.Channels[i]), 14) + 24
		if x > edge && x+w-24 > sw-edge {
			x, y = edge, y+40
		}
		spots[k] = image.Pt(int(x), int(y))
		x += w
	}
	return spots
}

// drawOffline draws the channels offline after the cards.
func (l *streamLobby) drawOffline(g *Game, dst *ebiten.Image, sh float64) {
	off := l.offline(g)
	top := l.offlineTop(g) - l.scroll
	if len(off) == 0 || top > sh || top+l.end(g)-l.offlineTop(g) < l.barH {
		return
	}
	edge := lobbyEdge
	if g.view.tall {
		edge = tallEdge
	}
	gfx.DrawTextLeft(dst, lang.T("オフライン"), edge, top, 14, streamInk2)
	for k, p := range l.offlineSpots(g, off) {
		c := g.db.Streams.Channels[off[k]]
		x, y := float64(p.X), top+float64(p.Y)
		g.drawAvatar(dst, c, x, y, 28)
		gfx.DrawTextLeft(dst, g.channelName(c), x+36, y+5, 14, streamInk3)
	}
}
