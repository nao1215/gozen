package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// The chat of the phone (ADR 0057): bubbles arriving one a press, a day's
// separator, the time and the read mark beside a bubble, and pictures and
// clips that open over the whole screen.

// chatBubble is a message laid out in the chat: its lines (a text's), its
// bubble's size and its height with the sender's name and the day over it.
type chatBubble struct {
	i      int
	lines  []string
	bw, bh float64
	h      float64
}

const phoneChatDay = 34.0 // the height of a day's separator

// chatPictureAspect is a picture's shape in a bubble, as the feed keeps it.
func (g *Game) chatPictureAspect(path string) float64 {
	return math.Max(phoneFeedWide, math.Min(phoneFeedTall, g.pictureAspect(path)))
}

// chatBubble lays a message out once per width: the chat keeps the visible
// ones, so a long chat is not laid out again every frame.
func (g *Game) chatBubble(p *phoneView, i int, maxW float64) chatBubble {
	if p.bubblesW != maxW {
		clear(p.bubbles)
		p.bubblesW = maxW
	}
	if b, ok := p.bubbles[i]; ok {
		return b
	}
	b := g.layChatBubble(p, i, maxW)
	p.bubbles[i] = b
	return b
}

func (g *Game) layChatBubble(p *phoneView, i int, maxW float64) chatBubble {
	m := p.cue.Messages[i]
	b := chatBubble{i: i}
	switch {
	case m.Video != nil:
		b.bw, b.bh = maxW, maxW*0.5625
	case m.Picture != "":
		b.bw = maxW * 0.85
		b.bh = b.bw * g.chatPictureAspect(m.Picture)
	case m.Text != "":
		b.lines = gfx.Wrap(m.Text, phoneChatSize, maxW-phoneChatPad*2)
		for _, l := range b.lines {
			b.bw = math.Max(b.bw, gfx.TextWidth(l, phoneChatSize))
		}
		b.bw += phoneChatPad * 2
		b.bh = float64(len(b.lines))*gfx.LineHeight(phoneChatSize) + phoneChatPad*1.4
	}
	b.h = b.bh
	if !m.Mine && m.From != "" && b.bh > 0 {
		b.h += 20
	}
	if m.Day != "" {
		b.h += phoneChatDay
	}
	return b
}

func (g *Game) updatePhoneChat(p *phoneView) {
	if p.cue.History {
		g.updatePhoneHistory(p)
		return
	}
	if p.typing > 0 {
		p.typing--
		if p.typing == 0 {
			p.shown = p.nextChat(p.shown) + 1
			p.chatRead = 0
		}
		return
	}
	// A click on a picture in the chat opens it, one on a clip plays it.
	if in := &g.in; in.inspect(in.click) {
		for i, r := range p.pics {
			if r.has(g, p, in.mouse.X, in.mouse.Y) {
				in.query(true)
				g.openChatMedia(p, i)
				return
			}
		}
	}
	if !g.in.advance() && !g.phoneAuto(p) {
		return
	}
	// A message taller than the screen is read from its top, a screen at a
	// time (each overlapping the last), before the chat goes on.
	_, _, w, h := g.phoneScreenRect(p.side)
	if room := chatRoom(h); p.chatOverflow(g, w, room) > 0 {
		p.chatRead += room * 0.8
		return
	}
	// The newest message is a picture or a video not yet seen: the press opens it.
	last := p.shown - 1
	if last < 0 {
		p.closing = phoneRise
		return
	}
	if m := p.cue.Messages[last]; !g.autoRead && g.openNewest(p, last, m) {
		return
	}
	nextIndex := p.nextChat(p.shown)
	if nextIndex >= len(p.cue.Messages) {
		p.closing = phoneRise
		return
	}
	if next := p.cue.Messages[nextIndex]; next.Mine || next.Text == "" && next.Picture == "" && next.Video == nil {
		p.shown = nextIndex + 1
		p.chatRead = 0
		return
	}
	p.typing = phoneTyping
}

// chatRoom is the height the chat's messages show in on a screen h tall.
func chatRoom(h float64) float64 { return math.Max(1, h-28-phoneFeedTop) }

// chatOverflow is how much of the newest message, a text taller than room,
// is still under the bottom of the chat: it shows from its top until read.
func (p *phoneView) chatOverflow(g *Game, w, room float64) float64 {
	if p.shown < 1 || p.shown > len(p.cue.Messages) || p.cue.Messages[p.shown-1].Text == "" {
		return 0
	}
	return math.Max(0, g.chatBubble(p, p.shown-1, w*0.68).h-room-p.chatRead)
}

// openNewest opens the newest message when it is a picture or a clip not seen
// yet, and tells whether it did.
func (g *Game) openNewest(p *phoneView, last int, m data.PhoneMessage) bool {
	switch {
	case m.Video != nil:
		key := fmt.Sprintf("msg%d", last)
		if c := p.clip(key, m.Video); c.played == 0 {
			p.watch, p.watchMsg = c, last
			p.turn(true)
			return true
		}
	case m.Picture != "" && !p.seenPic[last]:
		p.seenPic[last] = true
		p.viewPic = m.Picture
		return true
	}
	return false
}

// openChatMedia opens a chat's picture over the whole screen, or plays its
// clip (again from its start once it has been watched).
func (g *Game) openChatMedia(p *phoneView, i int) {
	m := p.cue.Messages[i]
	if m.Video == nil {
		p.seenPic[i] = true
		p.viewPic = m.Picture
		return
	}
	p.watch, p.watchMsg = p.clip(fmt.Sprintf("msg%d", i), m.Video), i
	if p.watch.played >= p.watch.length() {
		p.watch.played = 0
	}
	p.turn(true)
}

// picRect is where a chat's picture or clip was drawn, on the phone's screen.
type picRect struct{ x, y, w, h float64 }

// has tells that a point on the game's screen is in the part of the rectangle
// under the chat's header.
func (r picRect) has(g *Game, p *phoneView, mx, my int) bool {
	sx, sy, _, _ := g.phoneScreenRect(p.side)
	x, y := float64(mx)-sx, float64(my)-sy
	return x >= r.x && x < r.x+r.w && y >= math.Max(r.y, phoneFeedTop) && y < r.y+r.h
}

// The back chevron of a chat's header: the width and the top of where a press
// on it goes back.
const (
	phoneChevron    = 56.0
	phoneChevronTop = 34.0
)

// chatChevron tells that the chat's header with its back chevron is on the screen.
func (p *phoneView) chatChevron() bool {
	return p.cue.App == "chat" && !p.cue.History && p.viewPic == "" && p.watch == nil
}

func (g *Game) drawPhoneChat(p *phoneView, dst *ebiten.Image, w, h float64) {
	if p.cue.History {
		g.drawPhoneHistory(p, dst, w, h)
		return
	}
	// The header goes over the messages that scroll up under it.
	textStart, controlStart := g.screenOffsets()
	defer func() {
		g.clipScreenRecords(textStart, controlStart, phoneFeedTop, w, h)
		g.drawPhoneHeader(dst, w, p.cue.Title, phoneChevronTop)
		g.text(dst, "<", 18, 44, 22, g.phoneAccent())
		g.traceControl("<", roundRect(0, phoneChevronTop, phoneChevron, phoneFeedTop-phoneChevronTop), p.ready(), false, "transition")
	}()
	clear(p.pics)
	top, maxW := phoneFeedTop, w*0.68
	// The newest message sits at the bottom; the older ones scroll up. A tall
	// newest message shows from its top until it has been read down.
	y := h - 28 + p.chatOverflow(g, w, chatRoom(h))
	if p.typing > 0 {
		y -= 48
		g.drawTypingDots(dst, p, 64, y+8)
	}
	first := min(p.shown, len(p.cue.Messages))
	for i := first - 1; i >= 0 && y > top-200; i-- {
		if p.cue.Messages[i].Hidden {
			continue
		}
		b := g.chatBubble(p, i, maxW)
		first = i
		y -= b.h
		g.drawChatBubble(dst, p, b, w, y)
		y -= 12
	}
	// Only the layouts on the screen are kept.
	for i := range p.bubbles {
		if i < first || i >= p.shown {
			delete(p.bubbles, i)
		}
	}
}

func (g *Game) drawChatBubble(dst *ebiten.Image, p *phoneView, b chatBubble, w, y float64) {
	m := p.cue.Messages[b.i]
	if m.Day != "" {
		dw := gfx.TextWidth(m.Day, 12) + 24
		gfx.FillRoundRect(dst, float32(w/2-dw/2), float32(y+6), float32(dw), 22, 11, phoneLine)
		g.screenText(dst, m.Day, w/2, y+9, 12, phoneMuted)
		y += phoneChatDay
	}
	if b.bh == 0 {
		return
	}
	x := 64.0
	if m.Mine {
		x = w - b.bw - 16
	} else {
		g.drawPhoneIcon(dst, p, m.Icon, m.From, 14, y, 40)
		if m.From != "" {
			g.text(dst, gfx.Ellipsis(m.From, 13, w-x-16), x, y, 13, phoneMuted)
			y += 20
		}
	}
	g.drawChatContent(dst, p, b, x, y)
	// The time beside the bubble's foot, with the read mark over it for the owner's.
	foot := y + b.bh - 14
	if m.Mine {
		g.screenTextRight(dst, m.Time, x-6, foot, 11, phoneMuted)
		if m.Read {
			g.screenTextRight(dst, lang.T("既読"), x-6, foot-14, 11, phoneMuted)
		}
		return
	}
	g.text(dst, m.Time, x+b.bw+6, foot, 11, phoneMuted)
}

func (g *Game) drawChatContent(dst *ebiten.Image, p *phoneView, b chatBubble, x, y float64) {
	m := p.cue.Messages[b.i]
	switch {
	case m.Video != nil:
		key := fmt.Sprintf("msg%d", b.i)
		c := p.clip(key, m.Video)
		g.drawPhoneMedia(dst, p, key+".thumb", c, x, y, b.bw, b.bh, 14)
		gfx.FillCircle(dst, float32(x+b.bw/2), float32(y+b.bh/2), 26, color.NRGBA{0, 0, 0, 0x88}, true)
		drawPlay(dst, x+b.bw/2+4, y+b.bh/2, 14)
		g.text(dst, clipTime(c.length()), x+10, y+b.bh-24, 13, phoneOnVideo)
		g.chatMediaAt(p, b, x, y)
	case m.Picture != "":
		g.chatMediaAt(p, b, x, y)
		if img := g.pics.get(m.Picture); img != nil {
			frame := p.frame(fmt.Sprintf("pic%d", b.i), b.bw, b.bh)
			drawCoverTop(frame, img, 0, 0, b.bw, b.bh)
			gfx.RoundedMask(frame, float32(14*gfx.Scale()))
			drawFrame(dst, frame, x, y)
		}
	default:
		fill, ink := phoneBubble, phoneInk
		if m.Mine {
			fill, ink = g.phoneAccent(), phoneOnVideo
		}
		// A message many screens tall draws only its part on the screen.
		screenH := float64(dst.Bounds().Dy()) / gfx.Scale()
		if top, bottom := math.Max(y, -18), math.Min(y+b.bh, screenH+18); bottom > top {
			gfx.FillRoundRect(dst, float32(x), float32(top), float32(b.bw), float32(bottom-top), 18, fill)
		}
		lineH := gfx.LineHeight(phoneChatSize)
		for k := max(0, int(math.Floor((phoneFeedTop-y-phoneChatPad*0.6)/lineH))-1); k < len(b.lines); k++ {
			ly := y + phoneChatPad*0.6 + float64(k)*lineH
			if ly > screenH {
				break
			}
			g.text(dst, b.lines[k], x+phoneChatPad, ly, phoneChatSize, ink)
		}
	}
}

// chatMediaAt keeps where a picture or a clip was drawn, for a press on it.
func (g *Game) chatMediaAt(p *phoneView, b chatBubble, x, y float64) {
	p.pics[b.i] = picRect{x, y, b.bw, b.bh}
	if g.controlFrame() != nil {
		reason := "transition"
		if p.typing > 0 {
			reason = "typing"
		}
		g.traceControl("", roundRect(x, y, b.bw, b.bh), p.ready() && p.typing == 0, false, reason)
	}
}

// drawPhonePicture draws the picture opened over the phone's whole screen,
// whole and centered on black.
func (g *Game) drawPhonePicture(p *phoneView, dst *ebiten.Image, w, h float64) {
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), color.Black, false)
	if img := g.pics.get(p.viewPic); img != nil {
		drawContain(dst, img, 0, 30, w, h-60)
	}
}

// nextChat returns an authored index, preserving stable identity while skipping
// messages whose condition was false when the phone opened.
func (p *phoneView) nextChat(from int) int {
	for i := from; i < len(p.cue.Messages); i++ {
		if !p.cue.Messages[i].Hidden {
			return i
		}
	}
	return len(p.cue.Messages)
}

// historyLayout computes positions only when an opted-in history opens or changes
// width. Bubble text layouts are still retained only for messages on screen.
func (g *Game) historyLayout(p *phoneView, w, h float64) float64 {
	if p.historyRows == nil || p.historyW != w {
		first := p.historyRows == nil
		p.historyRows = make([]pageRow, len(p.cue.Messages))
		p.replyRows = make([]pageRow, len(p.cue.Replies))
		p.historyW = w
		p.historyVisible = p.historyVisible[:0]
		p.historyStops = p.historyStops[:0]
		y := phonePageGap
		for i, m := range p.cue.Messages {
			if m.Hidden {
				continue
			}
			b := g.layChatBubble(p, i, w*0.68)
			p.historyRows[i] = pageRow{top: y, h: b.h}
			p.historyVisible = append(p.historyVisible, i)
			p.historyStops = append(p.historyStops, y-phonePageGap)
			y += b.h + 12
		}
		for i, r := range p.cue.Replies {
			if r.Hidden {
				continue
			}
			lines := gfx.Wrap(r.Button, 16, w-phonePagePad*2-24)
			height := float64(len(lines))*gfx.LineHeight(16) + 42
			p.replyRows[i] = pageRow{top: y, h: height, lines: lines}
			p.historyStops = append(p.historyStops, y-phonePageGap)
			y += height + phonePageGap
		}
		p.historyHeight = y
		if first {
			p.scroll = math.Max(0, y-(h-phoneFeedTop))
		}
	}
	return math.Max(0, p.historyHeight-(h-phoneFeedTop))
}

func (g *Game) updatePhoneHistory(p *phoneView) {
	_, _, w, h := g.phoneScreenRect(p.side)
	end := g.historyLayout(p, w, h)
	if g.historyReplyKeys(p, end, h) {
		return
	}

	g.scrollPhone(p, p.historyStops, end, func(at image.Point) {
		y, on := g.phoneColumnAt(p, at)
		if !on {
			return
		}
		sx, _, _, _ := g.phoneScreenRect(p.side)
		x := float64(at.X) - sx
		for i, r := range p.replyRows {
			if r.h > 0 && x >= phonePagePad && x < w-phonePagePad && y >= r.top && y < r.top+r.h {
				p.putDown(-(i + 10), -1, false)
				return
			}
		}
		for i, r := range p.pics {
			if r.has(g, p, at.X, at.Y) {
				g.openChatMedia(p, i)
				return
			}
		}
	})
}

// Reply focus is explicit: scrolling or advancing never sends an authored draft.
func (g *Game) historyReplyKeys(p *phoneView, end, h float64) bool {
	buttons := pageButtons(p.cue.Replies)
	if len(buttons) == 0 {
		return false
	}
	in := &g.in
	var d int
	switch {
	case in.pressedKey(btnRight) || in.pressedKey(btnNext):
		d = 1
	case in.pressedKey(btnLeft):
		d = -1
	case in.inspect(in.pressed[btnOK]):
		if p.focus >= 0 && p.keyed {
			p.putDown(-(p.focus + 10), -1, false)
			return in.query(true)
		}
		d = 1
	default:
		return false
	}
	at := slices.Index(buttons, p.focus)
	if at < 0 {
		if d > 0 {
			at = 0
		} else {
			at = len(buttons) - 1
		}
	} else {
		at = (at + d + len(buttons)) % len(buttons)
	}
	p.focus, p.keyed = buttons[at], true
	r := p.replyRows[p.focus]
	p.aim = math.Max(0, math.Min(end, r.top+r.h+phonePageGap-(h-phoneFeedTop)))
	if r.top < p.scroll {
		p.aim = math.Max(0, r.top-phonePageGap)
	}
	return true
}

func (g *Game) drawPhoneHistory(p *phoneView, dst *ebiten.Image, w, h float64) {
	textStart, controlStart := g.screenOffsets()
	end := g.historyLayout(p, w, h)
	p.scroll = math.Max(0, math.Min(p.scroll, end))
	clear(p.pics)
	for i := range p.bubbles {
		r := p.historyRows[i]
		if r.top+r.h <= p.scroll || r.top >= p.scroll+h-phoneFeedTop {
			delete(p.bubbles, i)
		}
	}
	first := sort.Search(len(p.historyVisible), func(k int) bool { r := p.historyRows[p.historyVisible[k]]; return r.top+r.h > p.scroll })
	for _, i := range p.historyVisible[first:] {
		r := p.historyRows[i]
		y := phoneFeedTop + r.top - p.scroll
		if y >= h {
			break
		}
		b := g.chatBubble(p, i, w*0.68)
		g.drawChatBubble(dst, p, b, w, y)
	}
	for i, r := range p.replyRows {
		y := phoneFeedTop + r.top - p.scroll
		if r.h == 0 || y+r.h <= phoneFeedTop || y >= h {
			continue
		}
		x, iw := phonePagePad, w-phonePagePad*2
		if p.keyed && p.focus == i {
			gfx.StrokeRoundRect(dst, float32(x-3), float32(y-3), float32(iw+6), float32(r.h+6), 12, 2, g.pageInk(p))
		}
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(iw), float32(r.h), 10, g.phoneAccent())
		for k, line := range r.lines {
			g.text(dst, line, x+12, y+10+float64(k)*gfx.LineHeight(16), 16, phoneOnVideo)
		}
		g.screenTextRight(dst, lang.T("送信"), x+iw-12, y+r.h-24, 13, phoneOnVideo)
		g.traceControl(p.cue.Replies[i].Button, roundRect(x, y, iw, r.h), p.ready(), p.keyed && p.focus == i, "transition")
	}
	g.clipScreenRecords(textStart, controlStart, phoneFeedTop, w, h)
	g.drawPhoneHeader(dst, w, p.cue.Title, 34)
}
