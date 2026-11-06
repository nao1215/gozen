package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/puppetdef"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/layer5/puppet"
	lang "github.com/nao1215/gozen/internal/text"
)

// phoneView is a phone shown over the scene by the phone command (ADR 0057):
// held upright or on its side, with a chat, a photo feed or a video on it. It
// changes no state; the event waits on a Tap until the player puts it down.
type phoneView struct {
	cue data.PhoneCue
	// age counts ticks since the phone came up; closing counts down the ticks
	// left of putting it down (0 while it is up).
	age, closing int
	// side tells the phone is held on its side; turned counts the ticks since
	// it last turned (phoneTurn and over: it rests).
	side   bool
	turned int
	// shown is the number of chat messages on the screen; typing counts the
	// ticks the dots show before the next message from someone else.
	shown, typing int
	// watch is the clip playing on the phone turned on its side: the video app's
	// own, or a chat message's opened by the player (watchMsg is its index).
	watch    *phoneClip
	watchMsg int
	// post is the feed's post on the screen, slide the eased offset of the
	// change to it (-1..1 of the screen's width), drag the finger's pull in UI
	// units, and liked the posts the player liked.
	post   int
	slide  float64
	drag   float64
	dragAt float64
	liked  map[int]bool
	// clips are the clips the phone has played, by where they are (so a feed's
	// post keeps its place when it comes back), each with its own puppet.
	clips map[string]*phoneClip

	canvas, screen *ebiten.Image
	icons          map[string]*ebiten.Image
	frames         map[string]*ebiten.Image
}

// phoneClip is a video playing: where it is in ticks, whether it is paused, and
// the body of its puppet.
type phoneClip struct {
	v      *data.PhoneVideo
	played int
	paused bool
	body   *puppet.Puppet
}

const (
	phoneRise   = 18 // ticks to lift the phone up or put it down
	phoneTurn   = 27 // ticks to turn it a quarter
	phoneTyping = 36 // ticks the typing dots show
)

func newPhoneView(cue data.PhoneCue) *phoneView {
	p := &phoneView{cue: cue, liked: map[int]bool{}, clips: map[string]*phoneClip{},
		icons: map[string]*ebiten.Image{}, frames: map[string]*ebiten.Image{}, watchMsg: -1, turned: phoneTurn}
	switch cue.App {
	case "chat":
		p.shown = 1
	case "video":
		p.watch = p.clip("video", cue.Video)
	}
	return p
}

func (p *phoneView) clip(key string, v *data.PhoneVideo) *phoneClip {
	if v == nil {
		return nil
	}
	c := p.clips[key]
	if c == nil {
		c = &phoneClip{v: v}
		p.clips[key] = c
	}
	return c
}

func (c *phoneClip) length() int {
	s := 30.0
	if c.v.Seconds > 0 {
		s = c.v.Seconds
	}
	return int(s * 60)
}

// step plays the clip one tick: the puppet moves and jiggles at its seconds.
// loop starts it again at its end (a feed's post), else it stops there.
func (c *phoneClip) step(g *Game, key string, loop bool) {
	if c.v.Puppet != "" && c.body == nil {
		if rig := g.rig(c.v.Puppet); rig != nil {
			c.body = seededPuppet(rig, "phone."+key)
		}
	}
	if c.paused {
		return
	}
	if c.played >= c.length() {
		if !loop {
			return
		}
		c.played = 0
	}
	c.played++
	if c.body != nil {
		for _, at := range c.v.Bounce {
			if int(at*60) == c.played {
				c.body.Motion.React(puppetdef.Reaction{Kind: "jiggle", Amount: 1})
			}
		}
		c.body.Motion.Step()
	}
}

func (p *phoneView) resting() bool { return p.turned >= phoneTurn }

// ready tells that the phone has come up and turned, so it takes input.
func (p *phoneView) ready() bool {
	return p.age >= phoneRise && p.resting() && p.closing == 0
}

// turn holds the phone on its side or upright.
func (p *phoneView) turn(side bool) {
	if p.side != side {
		p.side, p.turned = side, 0
	}
}

// updatePhone runs one tick and tells when the phone has been put down.
func (g *Game) updatePhone() bool {
	p := g.phone
	p.age++
	p.turned++
	if p.age == phoneRise && p.cue.Hold == "landscape" {
		p.turn(true)
	}
	if p.closing > 0 {
		p.closing--
		return p.closing == 0
	}
	if g.skipping || g.replaying {
		p.closing = 1
		return false
	}
	p.slide *= 0.78
	if math.Abs(p.slide) < 0.002 {
		p.slide = 0
	}
	if p.watch != nil && p.resting() && p.age >= phoneRise {
		p.watch.step(g, p.watchKey(), false)
	}
	if p.cue.App == "feed" && p.watch == nil {
		if post := p.cue.Posts[p.post]; post.Video != nil {
			key := fmt.Sprintf("post%d", p.post)
			p.clip(key, post.Video).step(g, key, true)
		}
	}
	if !p.ready() {
		return false
	}
	in := &g.in
	if in.cancel() || in.esc {
		if p.watchMsg >= 0 {
			g.stopWatching(p)
			return false
		}
		p.closing = phoneRise
		return false
	}
	switch {
	case p.watch != nil:
		g.updatePhoneWatch(p)
	case p.cue.App == "chat":
		g.updatePhoneChat(p)
	case p.cue.App == "feed":
		g.updatePhoneFeed(p)
	}
	return false
}

func (p *phoneView) watchKey() string {
	if p.watchMsg >= 0 {
		return fmt.Sprintf("msg%d", p.watchMsg)
	}
	return "video"
}

// updatePhoneWatch plays the clip on the phone on its side: a press pauses it
// or, at its end, ends it (the video app's phone is put down; a chat's video
// turns the phone back upright to the chat).
func (g *Game) updatePhoneWatch(p *phoneView) {
	if !g.in.advance() {
		return
	}
	if p.watch.played < p.watch.length() {
		p.watch.paused = !p.watch.paused
		return
	}
	if p.watchMsg >= 0 {
		g.stopWatching(p)
		return
	}
	p.closing = phoneRise
}

func (g *Game) stopWatching(p *phoneView) {
	p.watch, p.watchMsg = nil, -1
	p.turn(false)
}

func (g *Game) updatePhoneChat(p *phoneView) {
	if p.typing > 0 {
		p.typing--
		if p.typing == 0 {
			p.shown++
		}
		return
	}
	if !g.in.advance() && !g.phoneAuto(p) {
		return
	}
	// The newest message is a video not yet seen: the press opens it.
	last := p.shown - 1
	if m := p.cue.Messages[last]; m.Video != nil && !g.autoRead {
		key := fmt.Sprintf("msg%d", last)
		if c := p.clip(key, m.Video); c.played == 0 {
			p.watch, p.watchMsg = c, last
			p.turn(true)
			return
		}
	}
	if p.shown >= len(p.cue.Messages) {
		p.closing = phoneRise
		return
	}
	if p.cue.Messages[p.shown].Mine {
		p.shown++
		return
	}
	p.typing = phoneTyping
}

// phoneAuto lets the auto mode read a chat as it reads lines.
func (g *Game) phoneAuto(p *phoneView) bool {
	return g.autoRead && p.age%int(120*g.cfg.autoScale()+1) == 0
}

func (g *Game) updatePhoneFeed(p *phoneView) {
	in := &g.in
	n := len(p.cue.Posts)
	turn := func(d int) {
		next := p.post + d
		if next < 0 || next >= n {
			return
		}
		p.post = next
		p.slide = float64(d)
	}
	x := float64(in.mouse.X)
	switch {
	case in.pdown:
		p.dragAt, p.drag = x, 0
	case in.pheld && p.dragAt != 0:
		p.drag = x - p.dragAt
	case in.pup && p.dragAt != 0:
		d := p.drag
		p.dragAt, p.drag = 0, 0
		switch {
		case d < -40:
			turn(1)
		case d > 40:
			turn(-1)
		case in.click && math.Abs(d) < 8:
			p.liked[p.post] = !p.liked[p.post]
		}
		return
	}
	if in.left() {
		turn(-1)
	}
	if in.right() {
		turn(1)
	}
	if in.ok() {
		if p.post == n-1 {
			p.closing = phoneRise
		} else {
			turn(1)
		}
	}
}

// Phone colors: a light app on a dark body. They are fixed rather than taken
// from the game's theme so that the phone reads as a phone in every game.
var (
	phoneBody    = color.NRGBA{0x16, 0x17, 0x1b, 0xff}
	phoneEdge    = color.NRGBA{0x4a, 0x4d, 0x55, 0xff}
	phoneScreen  = color.NRGBA{0xf4, 0xf5, 0xf8, 0xff}
	phoneInk     = color.NRGBA{0x1d, 0x1f, 0x24, 0xff}
	phoneMuted   = color.NRGBA{0x80, 0x86, 0x90, 0xff}
	phoneBubble  = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	phoneMine    = color.NRGBA{0x3d, 0x8b, 0xf2, 0xff}
	phoneHeart   = color.NRGBA{0xee, 0x3d, 0x5c, 0xff}
	phoneLine    = color.NRGBA{0xdd, 0xe0, 0xe6, 0xff}
	phoneVeil    = color.NRGBA{0, 0, 0, 0x99}
	phoneOnVideo = color.NRGBA{0xff, 0xff, 0xff, 0xff}
)

// phoneSize is the phone's outside in UI units as it rests: upright, it is 94%
// of the screen's height; on its side, a quarter larger than that phone turned,
// or less where the screen is narrow.
func (g *Game) phoneSize(side bool) (w, h float64) {
	sw, sh := g.screen()
	h = sh * 0.94
	w = h * 0.49
	if side {
		w, h = h*1.25, w*1.25
		if w > sw*0.94 {
			k := sw * 0.94 / w
			w, h = w*k, h*k
		}
	}
	return w, h
}

func (g *Game) drawPhone(dst *ebiten.Image) {
	p := g.phone
	if p == nil {
		return
	}
	sw, sh := g.screen()
	rise := easeOut(math.Min(1, float64(p.age)/phoneRise))
	if p.closing > 0 {
		rise = easeOut(float64(p.closing) / phoneRise)
	}
	gfx.FillRect(dst, 0, 0, float32(sw), float32(sh), alphaColor(phoneVeil, rise), false)

	w, h := g.phoneSize(p.side)
	k := gfx.Scale()
	pw, ph := int(math.Ceil(w*k)), int(math.Ceil(h*k))
	p.canvas = sized(p.canvas, pw, ph)
	p.canvas.Clear()
	g.drawPhoneBody(p, p.canvas, w, h)

	// A turn starts from the other way up, a quarter back, at the size the
	// phone had that way.
	angle, scale := 0.0, 1.0
	if t := easeInOut(math.Min(1, float64(p.turned)/phoneTurn)); t < 1 {
		ow, oh := g.phoneSize(!p.side)
		from := math.Min(ow/h, oh/w)
		quarter := math.Pi / 2
		if p.side {
			quarter = -quarter
		}
		angle = quarter * (1 - t)
		scale = from + (1-from)*t
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Translate(-float64(pw)/2, -float64(ph)/2)
	op.GeoM.Scale(scale, scale)
	op.GeoM.Rotate(angle)
	op.GeoM.Translate(sw/2*k, (sh/2+(1-rise)*sh*0.9)*k)
	op.ColorScale.ScaleAlpha(float32(math.Min(1, rise*1.5)))
	dst.DrawImage(p.canvas, op)

	if p.ready() {
		gfx.DrawText(dst, g.phoneHint(p), sw/2, sh-24, 14, alphaColor(phoneOnVideo, 0.8))
	}
}

func sized(img *ebiten.Image, w, h int) *ebiten.Image {
	if img != nil && img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	if img != nil {
		img.Deallocate()
	}
	return ebiten.NewImage(max(1, w), max(1, h))
}

func (g *Game) phoneHint(p *phoneView) string {
	switch {
	case p.watchMsg >= 0:
		return lang.T("クリック: 一時停止　Esc: チャットに戻る")
	case p.watch != nil:
		return lang.T("クリック: 一時停止　Esc: 閉じる")
	case p.cue.App == "feed":
		return lang.T("左右・スワイプ: 投稿を送る　クリック: いいね　Esc: 閉じる")
	}
	return lang.T("クリック: 次のメッセージ　Esc: 閉じる")
}

// drawPhoneBody draws the phone (its body, its screen and the app) into the
// canvas, in UI units from its top left. The app is drawn on a canvas of its
// own, cut to the screen's rounded corners.
func (g *Game) drawPhoneBody(p *phoneView, dst *ebiten.Image, w, h float64) {
	r := math.Min(w, h) * 0.13
	gfx.FillRoundRect(dst, 0, 0, float32(w), float32(h), float32(r), phoneBody)
	gfx.StrokeRoundRect(dst, 1, 1, float32(w-2), float32(h-2), float32(r), 2, phoneEdge)
	bezel := math.Min(w, h) * 0.035
	scw, sch := w-bezel*2, h-bezel*2
	k := gfx.Scale()
	p.screen = sized(p.screen, int(math.Ceil(scw*k)), int(math.Ceil(sch*k)))
	screen := p.screen
	screen.Clear()
	gfx.FillRect(screen, 0, 0, float32(scw), float32(sch), phoneScreen, false)
	switch {
	case p.watch != nil && p.side:
		g.drawPhoneWatch(p, screen, scw, sch)
	case p.cue.App == "chat":
		g.drawPhoneChat(p, screen, scw, sch)
	case p.cue.App == "feed":
		g.drawPhoneFeed(p, screen, scw, sch)
	case p.watch != nil:
		g.drawPhoneMedia(screen, p, "rising", p.watch, 0, 0, scw, sch, 0)
	}
	if !p.side {
		g.drawPhoneStatus(screen, scw, p.cue.App == "video")
	}
	gfx.RoundedMask(screen, float32((r-bezel)*k))
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(bezel*k, bezel*k)
	dst.DrawImage(screen, op)
}

func (g *Game) drawPhoneStatus(dst *ebiten.Image, w float64, dark bool) {
	ink := phoneInk
	if dark {
		ink = phoneOnVideo
	}
	gfx.DrawTextLeft(dst, g.phoneClock(), 22, 9, 15, ink)
	x := w - 24
	gfx.StrokeRoundRect(dst, float32(x-22), 12, 24, 12, 3, 1.3, ink)
	gfx.FillRect(dst, float32(x-20), 14, 17, 8, ink, false)
	for i := range 4 {
		bh := float32(3 + i*2)
		gfx.FillRect(dst, float32(x-56+float64(i)*5), 24-bh, 3, bh, ink, false)
	}
}

// phoneClock is the time on the phone: var.clock_minutes when the game keeps
// it, else a fixed evening hour.
func (g *Game) phoneClock() string {
	if g.st != nil {
		if v, ok := g.st.Vars["clock_minutes"].(float64); ok && v >= 0 {
			m := int(v) % (24 * 60)
			return fmt.Sprintf("%d:%02d", m/60, m%60)
		}
	}
	return "21:30"
}

func (g *Game) drawPhoneHeader(dst *ebiten.Image, w float64, title string, top float64) float64 {
	gfx.FillRect(dst, 0, 0, float32(w), float32(top+50), phoneBubble, false)
	gfx.DrawText(dst, gfx.Ellipsis(title, 19, w-90), w/2, top+12, 19, phoneInk)
	gfx.FillRect(dst, 0, float32(top+50), float32(w), 1, phoneLine, false)
	return top + 51
}

const (
	phoneChatSize = 17.0
	phoneChatPad  = 12.0
)

func (g *Game) drawPhoneChat(p *phoneView, dst *ebiten.Image, w, h float64) {
	// The header goes over the messages that scroll up under it.
	defer func() {
		g.drawPhoneHeader(dst, w, p.cue.Title, 34)
		gfx.DrawTextLeft(dst, "<", 18, 44, 22, phoneMine)
	}()
	top := 85.0
	maxW := w * 0.68
	type bubble struct {
		i      int
		lines  []string
		bw, bh float64
	}
	var bubbles []bubble
	for i, m := range p.cue.Messages[:min(p.shown, len(p.cue.Messages))] {
		b := bubble{i: i}
		switch {
		case m.Video != nil:
			b.bw, b.bh = maxW, maxW*0.5625
		case m.Picture != "":
			b.bw, b.bh = maxW*0.85, maxW*0.85
		default:
			b.lines = gfx.Wrap(m.Text, phoneChatSize, maxW-phoneChatPad*2)
			for _, l := range b.lines {
				b.bw = math.Max(b.bw, gfx.TextWidth(l, phoneChatSize))
			}
			b.bw += phoneChatPad * 2
			b.bh = float64(len(b.lines))*gfx.LineHeight(phoneChatSize) + phoneChatPad*1.4
		}
		if !m.Mine && m.From != "" {
			b.bh += 20
		}
		bubbles = append(bubbles, b)
	}
	// The newest message sits at the bottom; the older ones scroll up.
	y := h - 28
	if p.typing > 0 {
		y -= 48
		g.drawTypingDots(dst, p, 64, y+8)
	}
	for i := len(bubbles) - 1; i >= 0 && y > top-200; i-- {
		b := bubbles[i]
		y -= b.bh
		g.drawChatBubble(dst, p, b.i, b.lines, w, y, b.bw, b.bh)
		y -= 12
	}
}

func (g *Game) drawChatBubble(dst *ebiten.Image, p *phoneView, i int, lines []string, w, y, bw, bh float64) {
	m := p.cue.Messages[i]
	x := 64.0
	fill, ink := phoneBubble, phoneInk
	if m.Mine {
		x, fill, ink = w-bw-16, phoneMine, phoneOnVideo
	} else {
		g.drawPhoneIcon(dst, p, m.Icon, 14, y, 40)
		if m.From != "" {
			gfx.DrawTextLeft(dst, m.From, x, y, 13, phoneMuted)
			y += 20
			bh -= 20
		}
	}
	switch {
	case m.Video != nil:
		key := fmt.Sprintf("msg%d", i)
		c := p.clip(key, m.Video)
		g.drawPhoneMedia(dst, p, key+".thumb", c, x, y, bw, bh, 14)
		gfx.FillCircle(dst, float32(x+bw/2), float32(y+bh/2), 26, color.NRGBA{0, 0, 0, 0x88}, true)
		drawPlay(dst, x+bw/2+4, y+bh/2, 14)
		gfx.DrawTextLeft(dst, clipTime(c.length()), x+10, y+bh-24, 13, phoneOnVideo)
	case m.Picture != "":
		if img := g.pics.get(m.Picture); img != nil {
			frame := p.frame(fmt.Sprintf("pic%d", i), bw, bh)
			drawCoverTop(frame, img, 0, 0, bw, bh)
			gfx.RoundedMask(frame, float32(14*gfx.Scale()))
			drawFrame(dst, frame, x, y)
		}
	default:
		gfx.FillRoundRect(dst, float32(x), float32(y), float32(bw), float32(bh), 18, fill)
		for k, l := range lines {
			gfx.DrawTextLeft(dst, l, x+phoneChatPad, y+phoneChatPad*0.6+float64(k)*gfx.LineHeight(phoneChatSize), phoneChatSize, ink)
		}
	}
}

func clipTime(ticks int) string {
	s := ticks / 60
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (g *Game) drawTypingDots(dst *ebiten.Image, p *phoneView, x, y float64) {
	gfx.FillRoundRect(dst, float32(x), float32(y), 70, 36, 18, phoneBubble)
	for i := range 3 {
		up := math.Max(0, math.Sin(float64(p.age)/6-float64(i)*0.9)) * 4
		gfx.FillCircle(dst, float32(x+18+float64(i)*17), float32(y+18-up), 4.5, phoneMuted, true)
	}
}

func (g *Game) drawPhoneFeed(p *phoneView, dst *ebiten.Image, w, h float64) {
	top := g.drawPhoneHeader(dst, w, p.cue.Title, 34)
	offset := p.slide*w - p.drag
	for i := p.post - 1; i <= p.post+1; i++ {
		if i < 0 || i >= len(p.cue.Posts) {
			continue
		}
		x := float64(i-p.post)*w + offset
		if x <= -w || x >= w {
			continue
		}
		g.drawPhonePost(dst, p, i, x, top, w, h-top)
	}
	// The dots of the posts, the one on the screen filled.
	if n := len(p.cue.Posts); n > 1 {
		x0 := w/2 - float64(n-1)*8
		for i := range n {
			clr := phoneLine
			if i == p.post {
				clr = phoneMine
			}
			gfx.FillCircle(dst, float32(x0+float64(i)*16), float32(h-18), 4.5, clr, true)
		}
	}
}

func (g *Game) drawPhonePost(dst *ebiten.Image, p *phoneView, i int, x, y, w, h float64) {
	post := p.cue.Posts[i]
	gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(h), phoneBubble, false)
	g.drawPhoneIcon(dst, p, post.Icon, x+12, y+10, 38)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(post.Who, 16, w-80), x+60, y+18, 16, phoneInk)
	py := y + 58
	ph := math.Min(w*1.25, h-170)
	if post.Video != nil {
		key := fmt.Sprintf("post%d", i)
		g.drawPhoneMedia(dst, p, key, p.clip(key, post.Video), x, py, w, ph, 0)
		drawPlay(dst, x+w-24, py+22, 9)
	} else if img := g.pics.get(post.Picture); img != nil {
		drawCoverTop(dst, img, x, py, w, ph)
	}
	// The heart, the likes and the caption under the picture.
	ly := py + ph + 14
	likes := post.Likes
	if p.liked[i] {
		likes++
		drawHeart(dst, x+26, ly+11, 12, phoneHeart, true)
	} else {
		drawHeart(dst, x+26, ly+11, 12, phoneInk, false)
	}
	gfx.DrawTextLeft(dst, likeCount(likes), x+48, ly, 16, phoneInk)
	caption := post.Text
	if post.Who != "" {
		caption = post.Who + "  " + caption
	}
	lines, _ := gfx.FitLines(caption, 15, w-28, 3)
	for k, l := range lines {
		gfx.DrawTextLeft(dst, l, x+14, ly+30+float64(k)*gfx.LineHeight(15), 15, phoneInk)
	}
}

func likeCount(n int) string {
	return fmt.Sprintf(lang.T("いいね！ %s 件"), strconv.Itoa(n))
}

// drawPhoneWatch draws the clip playing on the phone on its side: the picture,
// the puppet, the title (it fades after three seconds and comes back while
// paused) and the seek bar.
func (g *Game) drawPhoneWatch(p *phoneView, dst *ebiten.Image, w, h float64) {
	c := p.watch
	gfx.FillRect(dst, 0, 0, float32(w), float32(h), color.Black, false)
	g.drawPhoneMedia(dst, p, p.watchKey()+".full", c, 0, 0, w, h, 0)
	length := c.length()
	alpha := 1 - math.Max(0, math.Min(1, float64(c.played-180)/40))
	if c.paused || c.played >= length {
		alpha = 1
	}
	if alpha > 0 {
		gfx.FillRect(dst, 0, 0, float32(w), 60, alphaColor(color.NRGBA{0, 0, 0, 0xaa}, alpha), false)
		gfx.DrawTextLeft(dst, gfx.Ellipsis(c.v.Text, 19, w-48), 26, 18, 19, alphaColor(phoneOnVideo, alpha))
	}
	if c.paused {
		gfx.FillCircle(dst, float32(w/2), float32(h/2), 38, color.NRGBA{0, 0, 0, 0x88}, true)
		drawPlay(dst, w/2+5, h/2, 20)
	}
	bar := w - 170
	done := float64(c.played) / float64(max(1, length))
	gfx.FillRect(dst, 130, float32(h-24), float32(bar), 5, color.NRGBA{0xff, 0xff, 0xff, 0x55}, false)
	gfx.FillRect(dst, 130, float32(h-24), float32(bar*done), 5, phoneHeart, false)
	gfx.FillCircle(dst, float32(130+bar*done), float32(h-21.5), 8, phoneHeart, true)
	gfx.DrawTextLeft(dst, clipTime(c.played)+" / "+clipTime(length), 18, h-34, 14, phoneOnVideo)
}

// drawPhoneMedia draws a clip in x, y, w, h: its picture pushed in slowly and
// its puppet in front, cut to the box (with corners of radius r).
func (g *Game) drawPhoneMedia(dst *ebiten.Image, p *phoneView, key string, c *phoneClip, x, y, w, h, r float64) {
	frame := p.frame(key, w, h)
	frame.Fill(color.Black)
	if img := g.pics.get(c.v.Picture); img != nil {
		zoom := 1 + 0.06*float64(c.played)/float64(max(1, c.length()))
		zw, zh := w*zoom, h*zoom
		drawCoverTop(frame, img, (w-zw)/2, (h-zh)/2, zw, zh)
	}
	if c.v.Puppet != "" && c.body == nil {
		if rig := g.rig(c.v.Puppet); rig != nil {
			c.body = seededPuppet(rig, "phone."+key)
		}
	}
	if c.body != nil {
		zoom := c.v.Zoom
		if zoom == 0 {
			zoom = 1.5
		}
		ph := h * zoom
		g.drawPuppet(frame, c.body, w/2, ph*0.96, ph, 1, false, nil)
	}
	if r > 0 {
		gfx.RoundedMask(frame, float32(r*gfx.Scale()))
	}
	drawFrame(dst, frame, x, y)
}

// frame is the phone's own canvas named key at w x h UI units, kept between ticks.
func (p *phoneView) frame(key string, w, h float64) *ebiten.Image {
	k := gfx.Scale()
	img := sized(p.frames[key], int(math.Ceil(w*k)), int(math.Ceil(h*k)))
	p.frames[key] = img
	img.Clear()
	return img
}

func drawFrame(dst, frame *ebiten.Image, x, y float64) {
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Translate(x*gfx.Scale(), y*gfx.Scale())
	dst.DrawImage(frame, op)
}

func drawPlay(dst *ebiten.Image, cx, cy, r float64) {
	for i := 0.0; i < r*1.6; i++ {
		x := cx - r*0.8 + i
		half := r * (1 - i/(r*1.6))
		gfx.FillRect(dst, float32(x), float32(cy-half), 1, float32(half*2), phoneOnVideo, false)
	}
}

func drawHeart(dst *ebiten.Image, cx, cy, r float64, clr color.Color, filled bool) {
	if !filled {
		gfx.StrokeCircle(dst, float32(cx-r*0.45), float32(cy-r*0.2), float32(r*0.5), 1.7, clr, true)
		gfx.StrokeCircle(dst, float32(cx+r*0.45), float32(cy-r*0.2), float32(r*0.5), 1.7, clr, true)
		gfx.StrokeLine(dst, float32(cx-r*0.93), float32(cy), float32(cx), float32(cy+r*0.85), 1.7, clr, true)
		gfx.StrokeLine(dst, float32(cx+r*0.93), float32(cy), float32(cx), float32(cy+r*0.85), 1.7, clr, true)
		return
	}
	gfx.FillCircle(dst, float32(cx-r*0.45), float32(cy-r*0.2), float32(r*0.52), clr, true)
	gfx.FillCircle(dst, float32(cx+r*0.45), float32(cy-r*0.2), float32(r*0.52), clr, true)
	for i := 0.0; i <= r*0.85; i++ {
		half := r * 0.97 * (1 - i/(r*0.85))
		gfx.FillRect(dst, float32(cx-half), float32(cy+i-1), float32(half*2), 1.5, clr, false)
	}
}

// drawPhoneIcon draws a round picture (a sender's or a poster's icon), or a
// plain circle when there is none.
func (g *Game) drawPhoneIcon(dst *ebiten.Image, p *phoneView, path string, x, y, size float64) {
	icon := p.icon(g, path, size)
	if icon == nil {
		gfx.FillCircle(dst, float32(x+size/2), float32(y+size/2), float32(size/2), phoneLine, true)
		return
	}
	drawFrame(dst, icon, x, y)
}

// icon is a picture cut to a circle at size UI units, made once per phone.
func (p *phoneView) icon(g *Game, path string, size float64) *ebiten.Image {
	if path == "" {
		return nil
	}
	key := fmt.Sprintf("%s@%.0f", path, size)
	if img, ok := p.icons[key]; ok {
		return img
	}
	src := g.pics.get(path)
	if src == nil {
		return nil
	}
	px := int(math.Ceil(size * gfx.Scale()))
	img := ebiten.NewImage(px, px)
	drawCoverTop(img, src, 0, 0, size, size)
	gfx.RoundedMask(img, float32(px)/2)
	p.icons[key] = img
	return img
}

// close frees the phone's own images and puppets.
func (p *phoneView) close() {
	for _, img := range []*ebiten.Image{p.canvas, p.screen} {
		if img != nil {
			img.Deallocate()
		}
	}
	for _, m := range []map[string]*ebiten.Image{p.icons, p.frames} {
		for _, img := range m {
			img.Deallocate()
		}
	}
	for _, c := range p.clips {
		if c.body != nil {
			c.body.Close()
		}
	}
}

// drawCoverTop fills x, y, w, h with the picture as a cover crop, keeping the
// top of a tall picture (a standing picture's face) instead of its middle.
func drawCoverTop(dst, img *ebiten.Image, x, y, w, h float64) {
	iw, ih := float64(img.Bounds().Dx()), float64(img.Bounds().Dy())
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return
	}
	s := math.Max(w/iw, h/ih)
	cw, ch := w/s, h/s
	cx := (iw - cw) / 2
	cy := (ih - ch) * 0.12
	crop := image.Rect(int(cx), int(cy), int(cx+cw), int(cy+ch)).Add(img.Bounds().Min)
	sub, ok := img.SubImage(crop).(*ebiten.Image)
	if !ok {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s*gfx.Scale(), s*gfx.Scale())
	op.GeoM.Translate(x*gfx.Scale(), y*gfx.Scale())
	dst.DrawImage(sub, op)
}

func easeInOut(t float64) float64 { return t * t * (3 - 2*t) }

func alphaColor(c color.NRGBA, a float64) color.NRGBA {
	c.A = uint8(float64(c.A) * math.Max(0, math.Min(1, a)))
	return c
}
