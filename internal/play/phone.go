package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode"

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
	// since counts the ticks since the newest notice of a lock screen came.
	since int
	// result is the index in the cue's Results the phone was put down with
	// (a page's button, a call answered or declined), -1 for none.
	result int
	// focus is the page's button the keys are on (-1 for none), and keyed
	// tells that the keys moved it there, so its ring shows.
	focus int
	keyed bool
	// viewPic is the chat's picture opened over the whole screen ("" for
	// none), seenPic the pictures opened already, and pics where the chat's
	// pictures were drawn, for a click on them.
	viewPic string
	// qrs are the page's QR codes, by their text (nil for one that cannot be made).
	qrs     map[string]*ebiten.Image
	seenPic map[int]bool
	pics    map[int]picRect
	// watch is the clip playing on the phone turned on its side: the video app's
	// own, or a chat message's opened by the player (watchMsg is its index).
	watch    *phoneClip
	watchMsg int
	// The feed: scroll is how far its posts are scrolled up in UI units, aim
	// the scroll a key glides to (-1 for none), glide the speed it goes on at
	// after a drag, grab the pointer's last height while it drags (-1 for
	// none) and moved how far it moved since it went down; liked are the posts
	// the player liked, and layout the posts laid out for a screen layoutW wide.
	scroll, aim, glide, grab, moved float64
	liked                           map[int]bool
	layout                          []feedPost
	layoutW, layoutH                float64
	// rows are a page's rows laid out for a screen rowsW wide.
	rows  []pageRow
	rowsW float64
	// chatRead is how far the newest message, when it is taller than the
	// screen, has been read down from its top.
	chatRead float64
	// bubbles are the chat's messages on the screen laid out for a screen
	// bubblesW wide (only the visible ones are kept).
	bubbles  map[int]chatBubble
	bubblesW float64
	// clips are the clips the phone has played, by where they are (so a feed's
	// post keeps its place when it comes back), each with its own puppet motion.
	clips map[string]*phoneClip

	canvas, screen *ebiten.Image
	icons          map[string]*ebiten.Image
	frames         map[string]*ebiten.Image
	// imageUse are the icons and frames drawn this frame, and bodies the
	// clips whose puppet was drawn (true this frame): the rest are released.
	imageUse map[*ebiten.Image]bool
	bodies   map[*phoneClip]bool
}

// phoneClip is a video playing: where it is in ticks, whether it is paused,
// whether it starts again at its end (a feed's post), and the body of its puppet.
type phoneClip struct {
	v      *data.PhoneVideo
	key    string
	played int
	paused bool
	loop   bool
	body   *puppet.Puppet
	motion *puppet.Motion // kept while an offscreen body has released its images
}

const (
	phoneRise   = 18 // ticks to lift the phone up or put it down
	phoneTurn   = 27 // ticks to turn it a quarter
	phoneTyping = 36 // ticks the typing dots show
)

func newPhoneView(cue data.PhoneCue) *phoneView {
	p := &phoneView{
		cue: cue, liked: map[int]bool{}, clips: map[string]*phoneClip{},
		icons: map[string]*ebiten.Image{}, frames: map[string]*ebiten.Image{}, watchMsg: -1, turned: phoneTurn, aim: -1, grab: -1, result: -1, focus: -1,
		seenPic: map[int]bool{}, pics: map[int]picRect{}, qrs: map[string]*ebiten.Image{},
		bubbles: map[int]chatBubble{}, imageUse: map[*ebiten.Image]bool{}, bodies: map[*phoneClip]bool{},
	}
	switch cue.App {
	case "chat", "lock":
		p.shown = 1
	case "video":
		p.watch = p.clip("video", cue.Video)
	}
	return p
}

// putDown puts the phone down with the result of a page's row or a notice
// (-1 for neither: the last call declined), when the story keeps it.
func (p *phoneView) putDown(row, notice int) {
	p.closing = phoneRise
	for i, r := range p.cue.Results() {
		if r.Row == row && r.Notice == notice {
			p.result = i
		}
	}
}

// answer is what the phone answers its event with: the result it was put
// down with or, for a feed that keeps likes, bit k for the k-th post of
// data.PhoneCue.Liking liked as the phone goes down (a like taken back by a
// second tap is not kept). Skip, which puts the phone down at once, keeps the
// likes given before it.
func (p *phoneView) answer() int {
	likes := p.cue.Liking()
	if len(likes) == 0 {
		return p.result
	}
	n := 0
	for k, i := range likes {
		if p.liked[i] {
			n |= 1 << k
		}
	}
	return n
}

func (p *phoneView) clip(key string, v *data.PhoneVideo) *phoneClip {
	if v == nil {
		return nil
	}
	c := p.clips[key]
	if c == nil {
		c = &phoneClip{v: v, key: key}
		p.clips[key] = c
	}
	return c
}

// length is the clip's length in ticks: at least one, so a clip shorter than
// a tick still plays.
func (c *phoneClip) length() int {
	return max(1, int(c.v.Length(len(c.v.Frames))*60))
}

// frames are the clip's footage now: the frame on the screen, the next one
// fading in over it by mix (with fade, over the frame's last fade seconds),
// and how far into the frame it is (0 to 1, for the push-in of pan).
func (c *phoneClip) frames() (cur, next string, mix, into float64) {
	n := len(c.v.Frames)
	if n == 0 {
		return "", "", 0, 0
	}
	at := float64(c.played) * c.v.FrameRate() / 60
	i := int(at)
	into = at - float64(i)
	if c.loop {
		i %= n
	}
	if i >= n-1 && !c.loop {
		return c.v.Frames[n-1], "", 0, math.Min(1, at-float64(n-1))
	}
	cur = c.v.Frames[i]
	if fade := c.v.Fade * c.v.FrameRate(); fade > 0 {
		next = c.v.Frames[(i+1)%n]
		mix = math.Max(0, math.Min(1, (into-(1-fade))/fade))
	}
	return cur, next, mix, into
}

// phonePictures are the pictures the phone shows: icons, pictures, posters and
// every frame of its clips, to decode before it comes up.
func phonePictures(cue data.PhoneCue) []string {
	var out []string
	video := func(v *data.PhoneVideo) {
		if v != nil {
			out = append(append(out, v.Picture), v.Frames...)
		}
	}
	for _, m := range cue.Messages {
		out = append(out, m.Icon, m.Picture)
		video(m.Video)
	}
	for _, post := range cue.Posts {
		out = append(out, post.Icon, post.Picture)
		video(post.Video)
	}
	video(cue.Video)
	for _, r := range cue.Rows {
		out = append(append(out, r.Picture), r.Pictures...)
		if r.Item != nil {
			out = append(out, r.Item.Icon)
		}
	}
	for _, n := range cue.Notices {
		out = append(out, n.Icon)
	}
	return out
}

// prepare makes the clip's puppet, the same for its thumbnail and its
// playback, taking back the motion it had when it was released offscreen.
func (c *phoneClip) prepare(g *Game) {
	if c.v.Puppet == "" || c.body != nil {
		return
	}
	if rig := g.rig(c.v.Puppet); rig != nil {
		c.body = seededPuppet(rig, "phone."+c.key)
		if c.motion != nil {
			c.body.Motion, c.motion = c.motion, nil
		}
	}
}

// step plays the clip one tick: the puppet moves and jiggles at its seconds
// (one at 0 on the first tick). loop starts it again at its end (a feed's
// post), else it stops there.
func (c *phoneClip) step(g *Game, loop bool) {
	c.loop = loop
	c.prepare(g)
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
			if max(1, int(at*60)) == c.played {
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
	p.since++
	if p.age == phoneRise && p.cue.Hold == "landscape" {
		p.turn(true)
	}
	if p.closing > 0 {
		p.closing--
		return p.closing == 0
	}
	if g.skipping || g.replaying || g.fast != nil || g.in.fast {
		p.closing = 1
		return false
	}
	if p.watch != nil && p.resting() && p.age >= phoneRise {
		p.watch.step(g, false)
	}
	if p.cue.App == "feed" && p.watch == nil {
		g.stepFeedClips(p)
	}
	if !p.ready() {
		return false
	}
	in := &g.in
	// The auto and skip keys work over the phone as over the lines.
	if in.pressed[btnAuto] {
		g.autoRead = !g.autoRead
		return false
	}
	if in.pressed[btnSkip] {
		g.toggleSkip()
		return false
	}
	if in.cancel() || in.esc {
		if p.watchMsg >= 0 {
			g.stopWatching(p)
			return false
		}
		if p.viewPic != "" {
			p.viewPic = ""
			return false
		}
		if p.declineCall() {
			return false
		}
		if p.calling() {
			p.putDown(-1, -1)
			return false
		}
		p.closing = phoneRise
		return false
	}
	switch {
	case p.viewPic != "":
		if in.advance() {
			p.viewPic = ""
		}
	case p.watch != nil:
		g.updatePhoneWatch(p)
	case p.cue.App == "chat":
		g.updatePhoneChat(p)
	case p.cue.App == "feed":
		g.updatePhoneFeed(p)
	case p.cue.App == "page":
		g.updatePhonePage(p)
	case p.cue.App == "lock":
		g.updatePhoneLock(p)
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

// phoneAuto lets the auto mode read a chat as it reads lines.
func (g *Game) phoneAuto(p *phoneView) bool {
	return g.autoRead && p.age%int(120*g.cfg.autoScale()+1) == 0
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
// of the screen's height; on its side, as wide as fits (94% of the screen's
// width at most, 93% of its height) with a 16:9 screen inside the bezel, so a
// video fills it.
func (g *Game) phoneSize(side bool) (w, h float64) {
	sw, sh := g.screen()
	if !side {
		// A narrow screen keeps the whole phone on it.
		h = math.Min(sh*0.94, sw*0.94/0.49)
		return h * 0.49, h
	}
	// The bezel is 3.5% of the height on each side: h - 0.07h = (w - 0.07h) * 9/16.
	const k = 0.5625 / (0.93 + 0.07*0.5625)
	w = sw * 0.94
	h = w * k
	if h > sh*0.93 {
		h = sh * 0.93
		w = h / k
	}
	return w, h
}

func phoneBezel(w, h float64) float64 { return math.Min(w, h) * 0.035 }

// phoneScreenRect is the phone's screen as it rests, in UI units of the game's
// screen: its top left and its size.
func (g *Game) phoneScreenRect(side bool) (x, y, w, h float64) {
	sw, sh := g.screen()
	pw, ph := g.phoneSize(side)
	b := phoneBezel(pw, ph)
	return (sw-pw)/2 + b, (sh-ph)/2 + b, pw - b*2, ph - b*2
}

func (g *Game) drawPhone(dst *ebiten.Image) {
	p := g.phone
	if p == nil {
		return
	}
	clear(p.imageUse)
	defer p.trimImages()
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
	w, h = max(1, w), max(1, h)
	if img != nil && img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	if img != nil {
		img.Deallocate()
	}
	return ebiten.NewImage(w, h)
}

func (g *Game) phoneHint(p *phoneView) string {
	switch {
	case p.viewPic != "":
		return lang.T("クリック・Esc: チャットに戻る")
	case p.watchMsg >= 0:
		return lang.T("クリック: 一時停止　Esc: チャットに戻る")
	case p.watch != nil:
		return lang.T("クリック: 一時停止　Esc: 閉じる")
	case p.cue.App == "feed" && p.across() && p.cue.Browse:
		return lang.T("左右・ホイール・横にドラッグ: 送る　Esc: 閉じる")
	case p.cue.App == "feed" && p.across():
		return lang.T("左右・ホイール・横にドラッグ: 送る　写真をクリック: いいね　Esc: 閉じる")
	case p.cue.App == "feed" && p.cue.Browse:
		return lang.T("上下・ホイール・ドラッグ: スクロール　Esc: 閉じる")
	case p.cue.App == "feed":
		return lang.T("上下・ホイール・ドラッグ: スクロール　写真をクリック: いいね　Esc: 閉じる")
	case p.calling():
		return lang.T("Enter: 応答　Esc: 拒否")
	case p.cue.App == "lock":
		return lang.T("クリック: 次の通知　Esc: 閉じる")
	case p.cue.App == "page" && len(pageButtons(p.cue.Rows)) > 0:
		return lang.T("上下・ドラッグ: スクロール　左右・Tab: ボタン　Enter: 押す　Esc: 閉じる")
	case p.cue.App == "page":
		return lang.T("上下・ホイール・ドラッグ: スクロール　Esc: 閉じる")
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
	bezel := phoneBezel(w, h)
	scw, sch := w-bezel*2, h-bezel*2
	k := gfx.Scale()
	p.screen = sized(p.screen, int(math.Ceil(scw*k)), int(math.Ceil(sch*k)))
	screen := p.screen
	screen.Clear()
	gfx.FillRect(screen, 0, 0, float32(scw), float32(sch), phoneScreen, false)
	switch {
	case p.viewPic != "":
		g.drawPhonePicture(p, screen, scw, sch)
	case p.watch != nil && p.side:
		g.drawPhoneWatch(p, screen, scw, sch)
	case p.cue.App == "chat":
		g.drawPhoneChat(p, screen, scw, sch)
	case p.cue.App == "feed":
		g.drawPhoneFeed(p, screen, scw, sch)
	case p.cue.App == "page":
		g.drawPhonePage(p, screen, scw, sch)
	case p.cue.App == "lock":
		g.drawPhoneLock(p, screen, scw, sch)
	case p.watch != nil:
		g.drawPhoneMedia(screen, p, "rising", p.watch, 0, 0, scw, sch, 0)
	}
	if !p.side {
		_, bar := p.pageAccent()
		g.drawPhoneStatus(screen, scw, p.clock(g), p.viewPic != "" || p.cue.App == "video" || p.cue.App == "lock" || p.cue.App == "page" && bar && p.cue.URL == "")
	}
	gfx.RoundedMask(screen, float32((r-bezel)*k))
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(bezel*k, bezel*k)
	dst.DrawImage(screen, op)
}

// clock is the time on the phone: a lock screen's own time, else the phone's clock.
func (p *phoneView) clock(g *Game) string {
	if p.cue.App == "lock" && p.cue.Time != "" {
		return p.cue.Time
	}
	return g.phoneClock()
}

func (g *Game) drawPhoneStatus(dst *ebiten.Image, w float64, clock string, dark bool) {
	ink := phoneInk
	if dark {
		ink = phoneOnVideo
	}
	gfx.DrawTextLeft(dst, clock, 22, 9, 15, ink)
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
		if v, ok := g.st.Vars["clock_minutes"].(float64); ok && v >= 0 && !math.IsInf(v, 0) {
			// Wrap to a day before the conversion: a huge value overflows an int.
			m := int(math.Mod(v, 24*60))
			return fmt.Sprintf("%d:%02d", m/60, m%60)
		}
	}
	return "21:30"
}

func (g *Game) drawPhoneHeader(dst *ebiten.Image, w float64, title string, top float64) {
	gfx.FillRect(dst, 0, 0, float32(w), float32(top+50), phoneBubble, false)
	gfx.DrawText(dst, gfx.Ellipsis(title, 19, w-90), w/2, top+12, 19, phoneInk)
	gfx.FillRect(dst, 0, float32(top+50), float32(w), 1, phoneLine, false)
}

const (
	phoneChatSize = 17.0
	phoneChatPad  = 12.0
)

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

// drawPhoneMedia draws a clip in x, y, w, h: its footage's frame over its
// picture (or the picture alone, pushed in slowly) and its puppet in front, cut
// to the box (with corners of radius r).
func (g *Game) drawPhoneMedia(dst *ebiten.Image, p *phoneView, key string, c *phoneClip, x, y, w, h, r float64) {
	frame := p.frame(key, w, h)
	frame.Fill(color.Black)
	// Footage is drawn whole, letterboxed as a player does; a still fills the
	// box and is pushed in.
	if img := g.pics.get(c.v.Picture); img != nil && len(c.v.Frames) > 0 {
		drawContain(frame, img, 0, 0, w, h)
	} else if img != nil {
		zoom := 1 + 0.06*float64(c.played)/float64(max(1, c.length()))
		zw, zh := w*zoom, h*zoom
		drawCoverTop(frame, img, (w-zw)/2, (h-zh)/2, zw, zh)
	}
	cur, next, mix, into := c.frames()
	zoom := 1.0
	if c.v.Pan {
		zoom += 0.06 * into
	}
	if img := g.pics.get(cur); img != nil {
		drawContainZoom(frame, img, w, h, zoom, 1)
	}
	if img := g.pics.get(next); img != nil && mix > 0 {
		drawContainZoom(frame, img, w, h, 1, float32(mix))
	}
	c.prepare(g)
	if c.body != nil {
		p.bodies[c] = true
		zoom := c.v.Zoom
		if zoom == 0 {
			zoom = 1.5
		}
		// The head sits a little under the frame's top, clear of the title.
		ph := h * zoom
		g.drawPuppet(frame, c.body, w/2, ph+h*0.1, ph, 1, false, nil)
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
	p.imageUse[img] = true
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
//
// Without a picture it is a circle of a color kept for the name, with the
// name's first letter, as a messaging app draws one.
func (g *Game) drawPhoneIcon(dst *ebiten.Image, p *phoneView, path, name string, x, y, size float64) {
	icon := p.icon(g, path, size)
	if icon != nil {
		drawFrame(dst, icon, x, y)
		return
	}
	letter := nameInitial(name)
	if letter == "" {
		gfx.FillCircle(dst, float32(x+size/2), float32(y+size/2), float32(size/2), phoneLine, true)
		return
	}
	c := nameColor(name)
	c.A = 0xff
	gfx.FillCircle(dst, float32(x+size/2), float32(y+size/2), float32(size/2), c, true)
	ts := size * 0.48
	gfx.DrawText(dst, letter, x+size/2, y+size/2-gfx.LineHeight(ts)/2, ts, phoneOnVideo)
}

// nameInitial is the first letter or digit of a name, upper-cased ("" for none).
func nameInitial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return ""
}

// icon is a picture cut to a circle at size UI units, made once per phone.
func (p *phoneView) icon(g *Game, path string, size float64) *ebiten.Image {
	if path == "" {
		return nil
	}
	px := max(1, int(math.Ceil(size*gfx.Scale())))
	key := path + "@" + strconv.Itoa(px)
	if img, ok := p.icons[key]; ok {
		p.imageUse[img] = true
		return img
	}
	src := g.pics.get(path)
	if src == nil {
		return nil
	}
	img := ebiten.NewImage(px, px)
	drawCoverTop(img, src, 0, 0, size, size)
	gfx.RoundedMask(img, float32(px)/2)
	p.icons[key] = img
	p.imageUse[img] = true
	return img
}

// trimImages releases the icons and frames not drawn this frame, and the
// images of the puppets not drawn (keeping their motion for when they come
// back). Where a clip is stays in clips.
func (p *phoneView) trimImages() {
	for _, images := range []map[string]*ebiten.Image{p.icons, p.frames} {
		for key, img := range images {
			if !p.imageUse[img] {
				img.Deallocate()
				delete(images, key)
			}
		}
	}
	clear(p.imageUse)
	for c, drawn := range p.bodies {
		if drawn {
			p.bodies[c] = false
			continue
		}
		c.motion = c.body.Motion
		c.body.Close()
		c.body = nil
		delete(p.bodies, c)
	}
}

// closePhone puts the phone away when its wait ends or the game resets (a
// load, a new game, closing the game), freeing what it holds.
func (g *Game) closePhone() {
	if g.phone != nil {
		g.phone.close()
		g.phone = nil
	}
}

// close frees the phone's own images and puppets.
func (p *phoneView) close() {
	for _, img := range []*ebiten.Image{p.canvas, p.screen} {
		if img != nil {
			img.Deallocate()
		}
	}
	p.canvas, p.screen = nil, nil
	for _, m := range []map[string]*ebiten.Image{p.icons, p.frames, p.qrs} {
		for _, img := range m {
			if img != nil {
				img.Deallocate()
			}
		}
		clear(m)
	}
	clear(p.bubbles)
	clear(p.imageUse)
	clear(p.bodies)
	for _, c := range p.clips {
		c.motion = nil
		if c.body != nil {
			c.body.Close()
			c.body = nil
		}
	}
}

// drawContain draws the whole picture in x, y, w, h, centered at its own
// shape (the rest of the box is left as it is).
func drawContain(dst, img *ebiten.Image, x, y, w, h float64) {
	cx, cy, s := containAt(img.Bounds().Dx(), img.Bounds().Dy(), x, y, w, h)
	if s <= 0 {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s*gfx.Scale(), s*gfx.Scale())
	op.GeoM.Translate(cx*gfx.Scale(), cy*gfx.Scale())
	dst.DrawImage(img, op)
}

// drawContainZoom draws a frame of footage whole in a box w x h, pushed in
// zoom times about its middle, at alpha.
func drawContainZoom(dst, img *ebiten.Image, w, h, zoom float64, alpha float32) {
	cx, cy, s := containAt(img.Bounds().Dx(), img.Bounds().Dy(), 0, 0, w, h)
	if s <= 0 {
		return
	}
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(cx-w/2, cy-h/2)
	op.GeoM.Scale(zoom*gfx.Scale(), zoom*gfx.Scale())
	op.GeoM.Translate(w/2*gfx.Scale(), h/2*gfx.Scale())
	op.ColorScale.ScaleAlpha(alpha)
	dst.DrawImage(img, op)
}

// containAt is where a picture iw x ih drawn whole in x, y, w, h goes: its top
// left and its scale (0 when there is nothing to draw).
func containAt(iw, ih int, x, y, w, h float64) (cx, cy, s float64) {
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return 0, 0, 0
	}
	s = math.Min(w/float64(iw), h/float64(ih))
	return x + (w-float64(iw)*s)/2, y + (h-float64(ih)*s)/2, s
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
