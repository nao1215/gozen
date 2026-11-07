package play

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
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
	if p.typing > 0 {
		p.typing--
		if p.typing == 0 {
			p.shown++
			p.chatRead = 0
		}
		return
	}
	// A click on a picture in the chat opens it.
	if in := &g.in; in.clicked() {
		for i, r := range p.pics {
			if r.has(g, p, in.mouse.X, in.mouse.Y) {
				p.viewPic = p.cue.Messages[i].Picture
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
	if m := p.cue.Messages[last]; !g.autoRead && g.openNewest(p, last, m) {
		return
	}
	if p.shown >= len(p.cue.Messages) {
		p.closing = phoneRise
		return
	}
	if next := p.cue.Messages[p.shown]; next.Mine || next.Text == "" && next.Picture == "" && next.Video == nil {
		p.shown++
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

// picRect is where a chat's picture was drawn, on the phone's screen.
type picRect struct{ x, y, w, h float64 }

// has tells that a point on the game's screen is in the rectangle.
func (r picRect) has(g *Game, p *phoneView, mx, my int) bool {
	sx, sy, _, _ := g.phoneScreenRect(p.side)
	x, y := float64(mx)-sx, float64(my)-sy
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

func (g *Game) drawPhoneChat(p *phoneView, dst *ebiten.Image, w, h float64) {
	// The header goes over the messages that scroll up under it.
	defer func() {
		g.drawPhoneHeader(dst, w, p.cue.Title, 34)
		g.text(dst, "<", 18, 44, 22, g.phoneAccent())
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
	case m.Picture != "":
		p.pics[b.i] = picRect{x, y, b.bw, b.bh}
		if g.tr != nil {
			g.traceControl("", roundRect(x, y, b.bw, b.bh), p.ready(), false, "transition")
		}
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

// drawPhonePicture draws the picture opened over the phone's whole screen,
// whole and centered on black.
func (g *Game) drawPhonePicture(p *phoneView, dst *ebiten.Image, w, h float64) {
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), color.Black, false)
	if img := g.pics.get(p.viewPic); img != nil {
		drawContain(dst, img, 0, 30, w, h-60)
	}
}
