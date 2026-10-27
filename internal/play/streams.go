package play

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/chat"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// The live streams (ADR 0048): the lobby (streams_lobby.go) and the watch view
// (streams_watch.go) share what is here.

// streamDesk is what the stream screens keep for the run: the clock, read once when the
// streams start and advanced by ticks, the random source of the streams, who is live, and
// the gold earned while away that the lobby has not shown yet.
type streamDesk struct {
	base  time.Time
	tick0 int
	rng   *rand.Rand
	st    *rpg.State
	shows []*broadcast.Show
	away  int
	// speak reads the lines aloud (a browser build).
	speak   bool
	avatars map[string]*ebiten.Image
	// toast is a short message over the stream screens for toastT more ticks.
	toast  string
	toastT int
}

// streamNow is the wall clock of the streams.
func (g *Game) streamNow() time.Time {
	return g.desk.base.Add(time.Duration(g.tick-g.desk.tick0) * time.Second / ebiten.DefaultTPS)
}

// stepStreams keeps the streams of a game that has them, every tick: it reads the clock
// the first time, decides who is live for the game in play, and adds the gold earned
// while the game was closed.
func (g *Game) stepStreams() {
	if g.db.Streams == nil || g.st == nil {
		return
	}
	if g.st.Streams == nil {
		g.st.Streams = &rpg.Streams{}
	}
	if g.desk == nil {
		var now time.Time
		switch {
		case g.rep != nil:
			now = g.rep.now
		case g.opts.Now != nil:
			now = g.opts.Now()
		default:
			now = time.Now()
		}
		g.emit("now", traceLine{"unix_ms": now.UnixMilli()})
		g.desk = &streamDesk{base: now, tick0: g.tick, rng: rand.New(rand.NewPCG(g.opts.Seed, uint64(now.UnixMilli()))), avatars: map[string]*ebiten.Image{}} //nolint:gosec // Who streams is not security-sensitive.
	}
	now := g.streamNow()
	if g.desk.st != g.st {
		g.desk.st = g.st
		g.desk.shows = broadcast.Lineup(g.db.Streams, g.st.Streams, now, g.desk.rng)
	}
	if gold := broadcast.Settle(g.db.Streams, g.st.Streams, now); gold > 0 {
		g.st.Gold += gold
		g.desk.away += gold
	}
}

// liveShows are the shows still on the air.
func (g *Game) liveShows() []*broadcast.Show {
	var out []*broadcast.Show
	now := g.streamNow()
	for _, s := range g.desk.shows {
		if s.Live(now) {
			out = append(out, s)
		}
	}
	return out
}

// saveStreams writes the autosave after the streams' progress changed.
func (g *Game) saveStreams() {
	g.autoKey = ""
	g.autoSave()
}

// The colors of the stream screens: a light page like a streaming site.
var (
	streamPage  = color.NRGBA{0xf4, 0xf4, 0xf6, 0xff}
	streamPanel = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	streamRule  = color.NRGBA{0xe3, 0xe3, 0xe8, 0xff}
	streamInk   = color.NRGBA{0x11, 0x11, 0x14, 0xff}
	streamInk2  = color.NRGBA{0x5b, 0x5b, 0x66, 0xff}
	streamInk3  = color.NRGBA{0x8d, 0x8d, 0x98, 0xff}
	streamBrand = color.NRGBA{0xe0, 0x45, 0x7b, 0xff}
	streamLive  = color.NRGBA{0xe9, 0x19, 0x16, 0xff}
	streamStar  = color.NRGBA{0xd9, 0x9a, 0x00, 0xff}
	streamMine  = color.NRGBA{0x0a, 0x87, 0x54, 0xff}
	streamTips  = []color.NRGBA{{0xe3, 0xf1, 0xff, 0xff}, {0xff, 0xf2, 0xc6, 0xff}, {0xff, 0xe0, 0xcf, 0xff}, {0xff, 0xd6, 0xde, 0xff}}
	streamNames = [...]color.NRGBA{
		{0xc0, 0x39, 0x2b, 0xff},
		{0x1e, 0x7d, 0x4f, 0xff},
		{0x1f, 0x5f, 0xbf, 0xff},
		{0xa0, 0x52, 0x2d, 0xff},
		{0x8e, 0x44, 0xad, 0xff},
		{0xb0, 0x47, 0x7a, 0xff},
		{0x0f, 0x7b, 0x8a, 0xff},
		{0x7a, 0x6a, 0x00, 0xff},
		{0xd2, 0x69, 0x1e, 0xff},
		{0x3f, 0x51, 0xb5, 0xff},
	}
)

// nameColor is a viewer's own color in the chat.
func nameColor(name string) color.NRGBA {
	h := fnv.New32a()
	h.Write([]byte(name))
	return streamNames[h.Sum32()%uint32(len(streamNames))]
}

// channelName is the shown name of a channel: its actor's.
func (g *Game) channelName(c *data.Channel) string {
	if a := g.db.Actors[c.Actor]; a != nil {
		return a.Name
	}
	return c.ID
}

// showName is the names of a show's hosts.
func (g *Game) showName(s *broadcast.Show) string {
	names := make([]string, len(s.Hosts))
	for i, c := range s.Hosts {
		names[i] = g.channelName(c)
	}
	return strings.Join(names, " × ")
}

// channelPuppet is a new body of the channel's puppet for one place on the screen, or nil
// when it does not load. The place owns it and closes it.
func (g *Game) channelPuppet(c *data.Channel, place string) *puppet.Puppet {
	a := g.db.Actors[c.Actor]
	if a == nil {
		return nil
	}
	rig := g.rig(a.Puppet)
	if rig == nil {
		return nil
	}
	p := seededPuppet(rig, place)
	p.Reduce = true
	return p
}

// closeBodies closes the bodies of a place.
func closeBodies(ps []*puppet.Puppet) {
	for _, p := range ps {
		if p != nil {
			p.Close()
		}
	}
}

// drawFramed draws the channel's room and its puppet's frame into the rectangle x, y, w,
// h (UI units): the frame fills the height, shifted by shift of the width.
// A duo is framed zoom times as tall, so both fit side by side.
func (g *Game) drawFramed(dst *ebiten.Image, c *data.Channel, p *puppet.Puppet, x, y, w, h, shift, zoom float64, room bool) {
	k := gfx.Scale()
	sub, ok := dst.SubImage(image.Rect(int(x*k), int(y*k), int((x+w)*k), int((y+h)*k))).(*ebiten.Image)
	if !ok {
		return
	}
	if room {
		if img := g.pics.get(c.Room); img != nil {
			gfx.DrawImageCover(sub, img, x, y, w, h, 1)
		}
	}
	if p == nil {
		return
	}
	f := c.Frame
	fh := (f.Bottom - f.Top) * zoom
	s := h / fh
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-f.CX, -(f.Top - (fh-(f.Bottom-f.Top))*0.3))
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(x+w/2+shift*w, y)
	op.GeoM.Scale(k, k)
	p.Draw(sub, op)
}

// avatar is a round picture of the channel's face, cut from its puppet's base around
// the head.
func (g *Game) avatar(c *data.Channel) *ebiten.Image {
	if img, ok := g.desk.avatars[c.ID]; ok {
		return img
	}
	var img *ebiten.Image
	if a := g.db.Actors[c.Actor]; a != nil {
		if rig := g.rig(a.Puppet); rig != nil {
			// the head of the puppet, or the top of the frame when it names none
			f := c.Frame
			d := (f.Bottom - f.Top) * 0.4
			cx, cy := f.CX, f.Top+d/2
			if h := rig.Spec.Head; h != nil && len(h.Center) == 2 && h.Radius > 0 {
				cx, cy, d = h.Center[0], h.Center[1], h.Radius*2.2
			}
			r := image.Rect(int(cx-d/2), int(cy-d/2), int(cx+d/2), int(cy+d/2)).Intersect(rig.Base.Bounds())
			if !r.Empty() {
				img = gfx.RoundedMask(ebiten.NewImageFromImage(rig.Base.SubImage(r)), float32(r.Dx())/2)
			}
		}
	}
	g.desk.avatars[c.ID] = img
	return img
}

// drawAvatar draws the channel's avatar d across at x, y.
func (g *Game) drawAvatar(dst *ebiten.Image, c *data.Channel, x, y, d float64) {
	gfx.FillCircle(dst, float32(x+d/2), float32(y+d/2), float32(d/2), streamRule, true)
	if img := g.avatar(c); img != nil {
		gfx.DrawImageFit(dst, img, x, y, d, d, 1)
	}
}

// drawStars draws the affection step as a dot for each step above 0, filled up to the step.
func (g *Game) drawStars(dst *ebiten.Image, x, y float64, step int) {
	for i := range g.db.Streams.MaxStep() {
		cx := float32(x + 6 + float64(i)*14)
		if i < step {
			gfx.FillCircle(dst, cx, float32(y+6), 5, streamStar, true)
		} else {
			gfx.StrokeCircle(dst, cx, float32(y+6), 4.5, 1.5, streamStar, true)
		}
	}
}

// step is the affection step of the channel.
func (g *Game) step(c *data.Channel) int {
	return broadcast.Step(g.db.Streams, broadcast.Progress(g.st.Streams, c.ID).Points)
}

// earn adds affection points with the channel, and says so when they raise its step.
func (g *Game) earn(c *data.Channel, points float64) {
	before := g.step(c)
	broadcast.Progress(g.st.Streams, c.ID).Points += points
	if after := g.step(c); after > before {
		g.desk.toast, g.desk.toastT = fmt.Sprintf(lang.T("%sに覚えられた（%d）"), g.channelName(c), after), 200
	}
}

// drawToast draws the stream screens' toast.
func (g *Game) drawToast(dst *ebiten.Image) {
	d := g.desk
	if d.toastT <= 0 {
		return
	}
	d.toastT--
	sw, _ := g.screen()
	w := gfx.TextWidth(d.toast, 17) + 32
	gfx.FillRoundRect(dst, float32(sw/2-w/2), streamBarH+14, float32(w), 38, 8, withAlpha(streamInk, 0xe0))
	gfx.DrawText(dst, d.toast, sw/2, streamBarH+22, 17, streamPanel)
}

// money is an amount of gold as the game writes it.
func (g *Game) money(n int) string { return fmt.Sprintf("%d %s", n, g.db.Game.Terms.Currency) }

// The crowd's moods are the hours of its lines (CrowdLines.At).
var crowdHours = map[string]int{"morning": 7, "day": 13, "evening": 20}

// crowdMood is the crowd's mood at the hour.
func crowdMood(hour int) string {
	switch {
	case hour >= 5 && hour < 11:
		return "morning"
	case hour >= 17 || hour < 5:
		return "evening"
	}
	return "day"
}

// newCrowd makes the bot viewers of the streams file, chatting in the language shown,
// in the mood of the hour now; keepCrowd changes the mood as the hours go.
func (g *Game) newCrowd() *chat.Crowd {
	cr := g.db.Streams.Crowd
	moods := map[string][]string{}
	for mood, hour := range crowdHours {
		for _, l := range cr.Lines.At(hour) {
			moods[mood] = append(moods[mood], g.db.Tr(l))
		}
	}
	names := make([]string, len(cr.Names))
	for i, n := range cr.Names {
		names[i] = g.db.Tr(n)
	}
	b, err := json.Marshal(map[string]any{"names": names, "rate": cr.Rate, "start": crowdMood(g.streamNow().Hour()), "lines": moods})
	if err == nil {
		var s *chat.Script
		if s, err = chat.Parse(b); err == nil { // JSON is YAML
			return chat.NewCrowd(s, chat.CrowdConfig{Rand: rand.New(rand.NewPCG(g.desk.rng.Uint64(), 1))}) //nolint:gosec // Bot chatter.
		}
	}
	log.Printf("streams: crowd: %v", err) // gozen check reports the crowd
	return nil
}

// keepCrowd turns the crowd to the mood of the hour.
func (g *Game) keepCrowd(c *chat.Crowd) {
	if c != nil && g.tick%600 == 0 {
		if err := c.Mood(crowdMood(g.streamNow().Hour())); err != nil {
			log.Printf("streams: crowd: %v", err) // every mood has the lines of any hour
		}
	}
}

// streamChat is a column of chat messages: the log, and each message wrapped once when
// it is posted.
type streamChat struct {
	log   *chat.Log
	lines map[uint64][]string
	width float64
}

// The values of a chat message (chat.Message.Values).
const (
	chatTip  = "tip"  // the amount of a tip
	chatHost = "host" // 1: said by the streamer
	chatMine = "mine" // 1: written by the player
)

const chatSize = 15.0

func newStreamChat(width float64) *streamChat {
	return &streamChat{log: chat.New(chat.Config{Max: 150}), lines: map[uint64][]string{}, width: width}
}

// badge is the mark before a streamer's name.
func badge() string { return lang.T("配信者") }

// prefix is the start of a message's first line: the streamer's mark and the name.
func prefix(m chat.Message) string {
	if m.Values[chatHost] > 0 {
		return badge() + "  " + m.From + " "
	}
	return m.From + " "
}

// post adds the message, wrapped to the column.
func (c *streamChat) post(m chat.Message) {
	m, ok := c.log.Post(m)
	if !ok {
		return
	}
	if m.Values[chatTip] > 0 {
		c.lines[m.ID] = gfx.Wrap(m.Text, chatSize, c.width-16)
	} else {
		c.lines[m.ID] = gfx.Wrap(prefix(m)+m.Text, chatSize, c.width)
	}
	if len(c.lines) > 200 {
		keep := map[uint64][]string{}
		for _, k := range c.log.Recent(150) {
			keep[k.ID] = c.lines[k.ID]
		}
		c.lines = keep
	}
}

// pull posts what the crowd says by now (seconds of the clock since the screen opened).
func (c *streamChat) pull(crowd *chat.Crowd, since time.Duration) {
	if crowd == nil {
		return
	}
	for _, m := range crowd.Poll(since) {
		c.post(m)
	}
}

// drawChat draws the newest messages that fit in x, y, w, h, the newest at the bottom, the
// plain lines in ink.
func (g *Game) drawChat(dst *ebiten.Image, c *streamChat, x, y, h float64, money func(int) string, ink color.NRGBA) {
	lh := gfx.LineHeight(chatSize)
	bottom := y + h
	recent := c.log.Recent(int(h/lh) + 1)
	for i := len(recent) - 1; i >= 0 && bottom > y; i-- {
		m := recent[i]
		lines := c.lines[m.ID]
		if tip := int(m.Values[chatTip]); tip > 0 {
			bh := float64(len(lines)+1)*lh + 12
			bottom -= bh + 6
			if bottom < y {
				break
			}
			gfx.FillRoundRect(dst, float32(x), float32(bottom), float32(c.width), float32(bh), 6, streamTips[g.tipTier(tip)])
			gfx.DrawTextLeft(dst, m.From, x+8, bottom+6, chatSize, streamInk)
			gfx.DrawTextRight(dst, money(tip), x+c.width-8, bottom+6, chatSize, streamInk)
			for j, l := range lines {
				gfx.DrawTextLeft(dst, l, x+8, bottom+6+float64(j+1)*lh, chatSize, streamInk)
			}
			continue
		}
		bottom -= float64(len(lines))*lh + 4
		if bottom < y {
			break
		}
		for j, l := range lines {
			ly := bottom + float64(j)*lh
			if j > 0 {
				gfx.DrawTextLeft(dst, l, x, ly, chatSize, ink)
				continue
			}
			lx := x
			rest := l
			if m.Values[chatHost] > 0 && strings.HasPrefix(rest, badge()) {
				bw := gfx.TextWidth(badge(), chatSize-3)
				gfx.FillRoundRect(dst, float32(lx), float32(ly+2), float32(bw+8), float32(chatSize+2), 3, streamBrand)
				gfx.DrawTextLeft(dst, badge(), lx+4, ly+3, chatSize-3, streamPanel)
				lx += bw + 12
				rest = strings.TrimLeft(strings.TrimPrefix(rest, badge()), " ")
			}
			if name, ok := strings.CutPrefix(rest, m.From); ok {
				nc := nameColor(m.From)
				switch {
				case m.Values[chatMine] > 0:
					nc = streamMine
				case m.Values[chatHost] > 0:
					nc = g.hostColor(m.From)
				}
				gfx.DrawTextLeft(dst, m.From, lx, ly, chatSize, nc)
				lx += gfx.TextWidth(m.From, chatSize)
				rest = name
			}
			gfx.DrawTextLeft(dst, rest, lx, ly, chatSize, ink)
		}
	}
}

// hostColor is the color of the channel whose streamer is named name.
func (g *Game) hostColor(name string) color.NRGBA {
	for _, c := range g.db.Streams.Channels {
		if g.channelName(c) == name {
			return c.Color.Or(streamBrand)
		}
	}
	return streamBrand
}

// tipTier is the place of the amount among the tips, from 0.
func (g *Game) tipTier(amount int) int {
	tier := 0
	for i, t := range g.db.Streams.Tips {
		if amount >= t {
			tier = i
		}
	}
	return min(tier, len(streamTips)-1)
}

// streamTop draws the bar over the stream screens: the game's title, the gold, and the
// way back on the left (back, when not empty).
func (g *Game) streamTop(dst *ebiten.Image, back string) {
	sw, _ := g.screen()
	rect(dst, 0, 0, sw, streamBarH, streamPanel)
	rect(dst, 0, streamBarH-1, sw, 1, streamRule)
	x := 16.0
	if back != "" {
		gfx.DrawTextLeft(dst, "‹ "+back, x, 15, 16, streamInk2)
		x += gfx.TextWidth("‹ "+back, 16) + 24
	}
	if img := g.pics.get(g.db.Streams.Logo); img != nil {
		gfx.DrawImageFit(dst, img, x-3, 12, 28, 28, 1)
	} else {
		gfx.FillRoundRect(dst, float32(x), 15, 22, 22, 6, streamBrand)
	}
	gfx.DrawTextLeft(dst, cmp.Or(g.db.Game.Title, lang.T("配信")), x+30, 14, 18, streamInk)
	gfx.DrawTextRight(dst, g.money(g.st.Gold), sw-16, 15, 17, streamInk)
}

// streamBarH is the height of the bar over the stream screens.
const streamBarH = 52.0

// clickedIn reports whether the pointer clicked in x, y, w, h.
func (g *Game) clickedIn(x, y, w, h float64) bool {
	m := g.in.mouse
	return g.in.click && float64(m.X) >= x && float64(m.X) < x+w && float64(m.Y) >= y && float64(m.Y) < y+h
}
