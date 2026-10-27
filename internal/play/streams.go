package play

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"io/fs"
	"log"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/chat"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	"github.com/nao1215/gozen/internal/layer5/sound"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
	"github.com/nao1215/gozen/internal/world"
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
	// speak reads the lines aloud: their recorded voices (voices: the streams have them),
	// else speech synthesis in a browser build. It starts on when there are voices.
	speak, voices bool
	avatars       map[string]*ebiten.Image
	// gear is the settings' gear in the bar.
	gear stamp
	// toast is a short message over the stream screens for toastT more ticks.
	toast  string
	toastT int
	// kept is when keepOpen last wrote the autosave.
	kept time.Time
	// left is the beat the watch view of each show was to play next when it was left: it
	// goes at its own pace, ahead of the stream's clock at times, and a show opened again
	// comes in there rather than say again what was heard.
	left map[*broadcast.Show]int
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
		// the clock keeps the offset from UTC it was read at for the run (across a change
		// of summer time too), which a replay reads back with the time
		_, offset := now.Zone()
		now = now.In(time.FixedZone("", offset))
		g.emit("now", traceLine{"unix_ms": now.UnixMilli(), "utc_offset": offset})
		g.desk = &streamDesk{base: now, tick0: g.tick, rng: rand.New(rand.NewPCG(g.opts.Seed, uint64(now.UnixMilli()))), avatars: map[string]*ebiten.Image{}, left: map[*broadcast.Show]int{}} //nolint:gosec // Who streams is not security-sensitive.
		if v := g.db.Streams.Voices; v != "" {
			st, err := fs.Stat(g.db.FS, "assets/"+v)
			g.desk.voices = err == nil && st.IsDir()
			g.desk.speak = g.desk.voices
		}
		g.desk.kept = now
		whenHidden(func() {
			if !g.updating.Load() {
				g.keepOpen()
			}
		})
	}
	now := g.streamNow()
	if g.desk.st != g.st {
		g.desk.st = g.st
		g.desk.shows = broadcast.Lineup(g.db.Streams, g.st.Streams, now, g.desk.rng)
	}
	for i, s := range g.desk.shows { // a stream over starts again after its rest (slot.rest)
		for n := broadcast.Again(g.db.Streams, g.st.Streams, s, now); n != nil; n = broadcast.Again(g.db.Streams, g.st.Streams, s, now) {
			delete(g.desk.left, s)
			s, g.desk.shows[i] = n, n
		}
	}
	if gold := broadcast.Settle(g.db.Streams, g.st.Streams, now); gold > 0 {
		g.st.Gold += gold
		g.desk.away += gold
	}
	if now.Sub(g.desk.kept) >= streamKeep {
		g.keepOpen()
	}
}

// keepOpen writes the autosave with the time the game is open now (Settle keeps it in
// the state), when the game rests where the autosave is written (rests: an idle screen,
// the watch view, the menu or the settings over it). It runs every streamKeep, when the
// game closes and when a browser hides its page, whose frames then stop.
func (g *Game) keepOpen() {
	if g.desk == nil || !g.rests() {
		return
	}
	g.desk.kept = g.streamNow()
	g.saveStreams()
}

// voiceLoad is the recorded voice of a stream line being loaded (loadVoice): done is
// closed once pcm or err is set.
type voiceLoad struct {
	file string
	done chan struct{}
	pcm  []byte
	err  error
}

// read loads the voice from the game's files and closes done.
func (v *voiceLoad) read(fsys fs.FS) {
	v.pcm, v.err = sound.VoiceClip(fsys, v.file)
	close(v.done)
}

// loaded reports whether the voice has loaded (or failed to).
func (v *voiceLoad) loaded() bool {
	select {
	case <-v.done:
		return true
	default:
		return false
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

// streamKeep is how often the autosave is written while the game stays open, for the time
// it was last open: a game closed (or its page hidden) without it would count the time it
// was open as time away.
const streamKeep = time.Minute

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
	gfx.FillRoundRect(dst, float32(x), float32(y), float32(d), float32(d), float32(d/2), streamRule)
	if img := g.avatar(c); img != nil {
		gfx.DrawImageFit(dst, img, x, y, d, d, 1)
	}
}

// drawStars draws the affection step as a dot for each step above 0, filled up to the step.
// The dots are round rectangles, whose pictures gfx keeps (a circle's path is filled again
// every frame, which costs a phone much with a dozen cards).
func (g *Game) drawStars(dst *ebiten.Image, x, y float64, step int) {
	for i := range g.db.Streams.MaxStep() {
		cx := float32(x + 6 + float64(i)*14)
		if i < step {
			gfx.FillRoundRect(dst, cx-5, float32(y+1), 10, 10, 5, streamStar)
		} else {
			gfx.StrokeRoundRect(dst, cx-4.5, float32(y+1.5), 9, 9, 4.5, 1.5, streamStar)
		}
	}
}

// stamp is a small picture of the stream screens drawn from many vector shapes, painted
// once into its own image and copied every frame while its key and the scale stay the same
// (filling the shapes again every frame costs a phone much).
type stamp struct {
	key string
	img *ebiten.Image
}

// draw copies the picture of w×h UI units that paint draws from the origin to x, y,
// painting it again when key or the scale changed.
func (s *stamp) draw(dst *ebiten.Image, key string, x, y, w, h float64, paint func(dst *ebiten.Image)) {
	k := gfx.Scale()
	key = fmt.Sprint(key, k, w, h)
	if s.img == nil || s.key != key {
		s.free()
		s.key, s.img = key, ebiten.NewImage(max(1, int(math.Ceil(w*k))), max(1, int(math.Ceil(h*k))))
		paint(s.img)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x*k, y*k)
	dst.DrawImage(s.img, op)
}

// free lets the picture go.
func (s *stamp) free() {
	if s.img != nil {
		s.img.Deallocate()
		s.img = nil
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

// streamChat is a column of chat messages: the log, each message wrapped once to the
// column's width (again when the width changes), and how far the column is scrolled back
// from the newest message.
type streamChat struct {
	log   *chat.Log
	lines map[uint64][]string
	width float64
	back  float64
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

// post adds the message, wrapped to the column; a column scrolled back keeps its place.
func (c *streamChat) post(m chat.Message) {
	m, ok := c.log.Post(m)
	if !ok {
		return
	}
	c.wrap(m)
	if c.back > 0 {
		c.back += c.height(m)
	}
	if len(c.lines) > 200 {
		keep := map[uint64][]string{}
		for _, k := range c.log.Recent(150) {
			keep[k.ID] = c.lines[k.ID]
		}
		c.lines = keep
	}
}

// wrap wraps the message to the column.
func (c *streamChat) wrap(m chat.Message) {
	if m.Values[chatTip] > 0 {
		c.lines[m.ID] = gfx.Wrap(m.Text, chatSize, c.width-16)
	} else {
		c.lines[m.ID] = gfx.Wrap(prefix(m)+m.Text, chatSize, c.width)
	}
}

// fit wraps the messages again for a column of the width.
func (c *streamChat) fit(width float64) {
	if width == c.width {
		return
	}
	c.width, c.back = width, 0
	c.rewrap()
}

// rewrap wraps the messages again: for a new width, or the streamer's mark in another
// language.
func (c *streamChat) rewrap() {
	clear(c.lines)
	for _, m := range c.log.Recent(150) {
		c.wrap(m)
	}
}

// height is how much of the column the message takes, with the space after it.
func (c *streamChat) height(m chat.Message) float64 {
	lh := gfx.LineHeight(chatSize)
	if m.Values[chatTip] > 0 {
		return float64(len(c.lines[m.ID])+1)*lh + 18
	}
	return float64(len(c.lines[m.ID]))*lh + 4
}

// scroll scrolls the column of height h back (toward older messages) by d, no farther
// than the oldest message.
func (c *streamChat) scroll(d, h float64) {
	total := 0.0
	for _, m := range c.log.Recent(150) {
		total += c.height(m)
	}
	c.back = max(0, min(c.back+d, total-h))
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

// drawChat draws the messages that fit in x, y, w, h, the newest at the bottom unless the
// column is scrolled back, the plain lines in ink.
func (g *Game) drawChat(dst *ebiten.Image, c *streamChat, x, y, w, h float64, money func(int) string, ink color.NRGBA) {
	c.fit(w)
	k := gfx.Scale()
	clip, ok := dst.SubImage(image.Rect(int(x*k), int(y*k), int((x+w)*k)+1, int((y+h)*k))).(*ebiten.Image)
	if !ok {
		return
	}
	dst = clip
	lh := gfx.LineHeight(chatSize)
	bottom := y + h + c.back
	recent := c.log.Recent(int((h+c.back)/lh) + 1) // a message takes a line or more
	for i := len(recent) - 1; i >= 0 && bottom > y; i-- {
		m := recent[i]
		lines := c.lines[m.ID]
		bottom -= c.height(m)
		if bottom > y+h {
			continue
		}
		if tip := int(m.Values[chatTip]); tip > 0 {
			bh := float64(len(lines)+1)*lh + 12
			gfx.FillRoundRect(dst, float32(x), float32(bottom), float32(c.width), float32(bh), 6, streamTips[g.tipTier(tip)])
			gfx.DrawTextLeft(dst, m.From, x+8, bottom+6, chatSize, streamInk)
			gfx.DrawTextRight(dst, money(tip), x+c.width-8, bottom+6, chatSize, streamInk)
			for j, l := range lines {
				gfx.DrawTextLeft(dst, l, x+8, bottom+6+float64(j+1)*lh, chatSize, streamInk)
			}
			continue
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

// barButton is a button of the bar over the stream screens, between the title and the
// gold: a language the screens can be shown in, or an entry of the place. x and w are where
// it is in the bar.
type barButton struct {
	label string
	lang  string // the language it shows ("" for an entry)
	entry int    // the place's entry
	gear  bool   // the entry opens the settings
	x, y  float64
	w     float64
}

// The top and the height of the bar's buttons.
const barButtonY, barButtonH = 11.0, 30.0

// barButtons are the bar's buttons, laid out leftward from the gold: the languages side by
// side when the game offers more than one, next to the gold so that switching does not move
// them (their labels do not change with the language, the entries' do), then the place's
// entries. An entry has a › after
// its label, for it goes to another screen, and the settings' a gear before it. The buttons
// that would push into the logo and the title's first letters go to a second row under the
// first, from the right edge (a narrow screen).
func (g *Game) barButtons(entries []world.Entry) []barButton {
	var bs []barButton
	for i, e := range entries {
		bs = append(bs, barButton{label: e.Label, entry: i, gear: opensConfig(e)})
	}
	if offered := g.db.Game.Offered(); len(offered) > 1 {
		for _, code := range offered {
			bs = append(bs, barButton{label: langShort(code), lang: code})
		}
	}
	sw, _ := g.screen()
	langs := 0.0 // the languages' width together: they go to the second row together
	for i := range bs {
		b := &bs[i]
		b.w = gfx.TextWidth(b.label, 14) + 20
		if b.lang == "" {
			b.w += gfx.TextWidth("›", 14) + 6
			if b.gear {
				b.w += 20
			}
		} else {
			langs += b.w + 2
		}
	}
	x, y := sw-16-gfx.TextWidth(g.money(g.st.Gold), 17)-16, barButtonY
	for i := len(bs) - 1; i >= 0; i-- {
		b := &bs[i]
		need := b.w
		if b.lang != "" && (i == len(bs)-1 || bs[i+1].lang == "") { // the last language
			need = langs - 2
		}
		if x-need < 16+30+40 && y == barButtonY {
			x, y = sw-16, streamBarH+barButtonY-6
		}
		b.x, b.y = x-b.w, y
		x = b.x - 8
		if i > 0 && b.lang != "" && bs[i-1].lang != "" {
			x = b.x - 2 // the languages side by side
		}
	}
	return bs
}

// barHeight is the height of the bar with the buttons: taller by a row when they take two.
func barHeight(bs []barButton) float64 {
	for _, b := range bs {
		if b.y > barButtonY {
			return streamBarH + barButtonH + 4
		}
	}
	return streamBarH
}

// opensConfig reports whether the entry opens the settings.
func opensConfig(e world.Entry) bool {
	return e.Action != nil && slices.ContainsFunc(e.Action.Do, func(c data.Command) bool { return c.Open == "config" })
}

// langShort is a language as the bar's button names it.
func langShort(code string) string {
	switch code {
	case lang.Japanese:
		return "日本語"
	case lang.English:
		return "EN"
	}
	return strings.ToUpper(code)
}

// barClick is the bar's button clicked now, or -1.
func (g *Game) barClick(bs []barButton) int {
	for i, b := range bs {
		if g.clickedIn(b.x, b.y, b.w, barButtonH) {
			return i
		}
	}
	return -1
}

// switchLanguage shows the stream screens in the language, as the settings' language row
// does, and keeps it in the settings; the crowds chat in it from now on, and the chats mark
// the streamers' messages in it.
func (g *Game) switchLanguage(code string) {
	g.setLanguage(code)
	if err := g.saveConfig(); err != nil {
		g.failure = err
	}
	g.sfx("decide")
	for _, s := range g.stack {
		switch s := s.(type) {
		case *streamLobby:
			if s.chat != nil {
				s.crowd = g.newCrowd()
				s.chat.rewrap()
			}
		case *streamWatch:
			s.crowd = g.newCrowd()
			s.chat.rewrap()
		}
	}
}

// streamTop draws the bar over the stream screens: the way back on the left (back, when
// not empty), the logo, the game's title, the buttons (the entry sel framed) and the gold.
func (g *Game) streamTop(dst *ebiten.Image, back string, bs []barButton, sel int) {
	sw, _ := g.screen()
	bh := barHeight(bs)
	rect(dst, 0, 0, sw, bh, streamPanel)
	rect(dst, 0, bh-1, sw, 1, streamRule)
	x := 16.0 + 30
	if back != "" {
		gfx.DrawTextLeft(dst, "‹ "+back, 16, 15, 16, streamInk2)
		x += gfx.TextWidth("‹ "+back, 16) + 24
	}
	if img := g.pics.get(g.db.Streams.Logo); img != nil {
		gfx.DrawImageFit(dst, img, x-33, 12, 28, 28, 1)
	} else {
		gfx.FillRoundRect(dst, float32(x-30), 15, 22, 22, 6, streamBrand)
	}
	end := sw - 16 - gfx.TextWidth(g.money(g.st.Gold), 17)
	for _, b := range bs {
		if b.y == barButtonY {
			end = min(end, b.x)
		}
	}
	gfx.Pop(func() {
		gfx.DrawTextLeft(dst, gfx.Ellipsis(cmp.Or(g.db.Game.Title, lang.T("配信")), 18, max(0, end-x-16)), x, 14, 18, streamInk)
	})
	shown := g.language()
	for i, b := range bs {
		x, y, w, h := float32(b.x), float32(b.y), float32(b.w), float32(barButtonH)
		if b.lang != "" {
			if b.lang == shown {
				gfx.FillRoundRect(dst, x, y, w, h, 15, streamInk)
				gfx.DrawText(dst, b.label, b.x+b.w/2, b.y+6, 14, streamPanel)
				continue
			}
			gfx.StrokeRoundRect(dst, x, y, w, h, 15, 1.5, streamRule)
			gfx.DrawText(dst, b.label, b.x+b.w/2, b.y+6, 14, streamInk2)
			continue
		}
		edge := streamRule
		if i == sel {
			edge = streamBrand
		}
		gfx.FillRoundRect(dst, x, y, w, h, 15, streamPage)
		gfx.StrokeRoundRect(dst, x, y, w, h, 15, 1.5, edge)
		lx := b.x + 10
		if b.gear {
			g.desk.gear.draw(dst, "gear", lx, b.y+7, 16, 16, func(img *ebiten.Image) { drawGear(img, 8, 8, 7, streamInk2) })
			lx += 20
		}
		gfx.DrawTextLeft(dst, b.label, lx, b.y+6, 14, streamInk)
		gfx.DrawTextRight(dst, "›", b.x+b.w-10, b.y+5, 14, streamInk2)
	}
	gfx.DrawTextRight(dst, g.money(g.st.Gold), sw-16, 15, 17, streamInk)
}

// drawGear draws a gear of radius r centred on cx, cy: eight teeth around a ring.
func drawGear(dst *ebiten.Image, cx, cy, r float32, clr color.Color) {
	for i := range 8 {
		a := float64(i) * math.Pi / 4
		c, s := float32(math.Cos(a)), float32(math.Sin(a))
		gfx.StrokeLine(dst, cx+r*0.5*c, cy+r*0.5*s, cx+r*c, cy+r*s, r*0.4, clr, true)
	}
	gfx.StrokeCircle(dst, cx, cy, r*0.55, r*0.32, clr, true)
}

// streamBarH is the height of the bar over the stream screens.
const streamBarH = 52.0

// clickedIn reports whether the pointer clicked in x, y, w, h (a finger that swiped does not
// click).
func (g *Game) clickedIn(x, y, w, h float64) bool {
	m := g.in.mouse
	return g.in.click && !g.in.swiped && float64(m.X) >= x && float64(m.X) < x+w && float64(m.Y) >= y && float64(m.Y) < y+h
}

// touchIn reports whether the touch held or just lifted began in x, y, w, h.
func (g *Game) touchIn(x, y, w, h float64) bool {
	m := g.in.touchFrom
	return (g.in.touching || g.in.click) && float64(m.X) >= x && float64(m.X) < x+w && float64(m.Y) >= y && float64(m.Y) < y+h
}

// fling is a scroll by a finger: it follows the finger as it swipes and, when the finger
// lifts, glides on at the finger's speed, slowing down by flingSlow a tick.
type fling struct{ v float64 }

const flingSlow = 0.95

// step is how far the content moves down this tick: with a finger that swipes from the
// scrolled part (from), or gliding after it lifted. A new touch stops the glide.
func (f *fling) step(in *input, from bool) float64 {
	switch {
	case in.touching && in.swiped && from:
		f.v = 0.5*f.v + 0.5*in.swipe
		return in.swipe
	case in.touching:
		f.v = 0
		return 0
	}
	if f.v *= flingSlow; math.Abs(f.v) < 0.2 {
		f.v = 0
	}
	return f.v
}

// gameView is the size the screen is drawn at when the stream screens choose it from the
// window: w×h pixels, ui pixels to a UI unit, tall for the portrait layout. The zero value
// is the game's screen.
type gameView struct {
	w, h int
	ui   float64
	tall bool
}

// The sizes the stream screens choose from (streamView).
const (
	tallMinW, tallMaxW = 360.0, 600.0 // the UI width of the portrait layout
	wideUnit           = 0.9          // the fewest window pixels to a UI unit in the landscape layout
	wideMinW, wideMinH = 760.0, 420.0 // the smallest UI size of the landscape layout
	maxPixelsPerUnit   = 3.0
)

// streamView is the size the stream screens are drawn at in a window whose inside is
// ow×oh device-independent pixels of dsf device pixels each, for a game whose screen is
// uw×uh UI units. A window taller than wide takes the portrait layout, 360 to 600 units
// wide (a phone held upright: a unit to a pixel). A wide window keeps the game's screen
// unless it would draw a unit smaller than wideUnit pixels (a phone on its side); it then
// takes a UI of the window's shape at wideUnit pixels a unit, smaller when the window is
// under wideMinW×wideMinH units. The pixels follow the device's, at most
// maxPixelsPerUnit to a unit.
func streamView(ow, oh, dsf, uw, uh float64) gameView {
	if ow <= 0 || oh <= 0 || uw <= 0 || uh <= 0 {
		return gameView{}
	}
	tall := ow < oh
	k := ow / min(max(ow, tallMinW), tallMaxW) // window pixels to a UI unit
	if !tall {
		if k = min(wideUnit, oh/wideMinH, ow/wideMinW); k <= min(ow/uw, oh/uh) {
			return gameView{}
		}
	}
	p := min(k*max(dsf, 1), max(maxPixelsPerUnit, k))
	return gameView{w: max(1, int(math.Round(ow/k*p))), h: max(1, int(math.Round(oh/k*p))), ui: p, tall: tall}
}

// streamShown reports whether a stream screen is the screen shown, with no talk over it,
// or the settings opened over one.
func (g *Game) streamShown() bool {
	if len(g.stack) == 0 {
		return false
	}
	if c, ok := g.stack[len(g.stack)-1].(*configScreen); ok {
		return c.streams
	}
	if g.talk.active || g.events != nil && g.events.Running() {
		return false
	}
	switch g.stack[len(g.stack)-1].(type) {
	case *streamLobby, *streamWatch:
		return true
	}
	return false
}

// streamBase reports whether the screen under the overlays is a stream screen.
func (g *Game) streamBase() bool {
	for _, s := range slices.Backward(g.stack) {
		if o, ok := s.(overlay); ok && o.overlay() {
			continue
		}
		switch s.(type) {
		case *streamLobby, *streamWatch:
			return true
		}
		return false
	}
	return false
}
