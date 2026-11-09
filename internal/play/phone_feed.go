package play

import (
	"fmt"
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"

	"github.com/nao1215/gozen/internal/screen/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The feed of the phone (ADR 0057) is a column of posts scrolled up and down:
// with the wheel, the up and down keys (a post at a time) and a finger or the
// mouse dragged, gliding on when it lets go. With scroll: across it is a row of
// pages instead, a post each, swiped sideways as a photo app's carousel is.
const (
	phoneFeedTop   = 85.0 // under the status bar and the header
	phoneFeedGap   = 14.0 // between two posts
	phoneFeedHead  = 58.0 // the poster's icon and name over the picture
	phoneFeedWheel = 60.0 // UI units a notch of the wheel scrolls
	phoneFeedTap   = 8.0  // UI units a pointer may move and still tap
	phoneFeedLikes = 40.0 // the row of the heart and the likes under a picture
	phoneFeedDots  = 26.0 // the row of dots under the pages across
	phoneFeedFling = 3.0  // UI units a tick a swipe let go flicks to the next page
)

// feedPost is a post laid out in the feed, from the feed's top unscrolled: its
// top, its picture's top and height, its caption's lines drawn size big and its
// height. (A page across has its post on top: top is 0.)
type feedPost struct {
	top, pic, picH, h, size float64
	lines                   []string
	// text tells a post of text only: its lines fill the picture's place.
	text bool
	page int // visible page ordinal; authored indices remain unchanged
}

// feedLayout lays the posts out top to bottom on a screen w wide and view
// high under the header (once per size), each picture's box at its media's own
// shape. Across, each post is laid out from the top of its own page, its
// picture made shorter when the post would not fit the screen.
//
// Text that does not fit its lines is wrapped smaller, so its lines are kept
// with the size they were wrapped at and drawn at it: drawn bigger, they would
// run out past the right edge.
func (p *phoneView) feedLayout(g *Game, w, view float64) []feedPost {
	if p.layout != nil && p.layoutW == w && p.layoutH == view {
		return p.layout
	}
	across := p.across()
	out := make([]feedPost, len(p.cue.Posts))
	y := 0.0
	p.layoutEnd = 0
	page := 0
	for i, post := range p.cue.Posts {
		if post.Hidden {
			continue
		}
		if post.Picture == "" && post.Video == nil {
			lines, size := gfx.FitLines(post.Text, 16, w-28, 8)
			textH := float64(len(lines)) * gfx.LineHeight(size)
			h := phoneFeedHead + textH + 44
			out[i] = feedPost{top: y, pic: y + phoneFeedHead, picH: textH, h: h, size: size, lines: lines, text: true}
		} else {
			caption := post.Text
			if post.Who != "" {
				caption = post.Who + "  " + caption
			}
			lines, size := gfx.FitLines(caption, 15, w-28, 3)
			under := 46 + float64(len(lines))*gfx.LineHeight(size)
			picH := w * g.mediaAspect(post)
			if across {
				picH = math.Max(w*phoneFeedWide, math.Min(picH, view-phoneFeedDots-phoneFeedHead-under))
			}
			h := phoneFeedHead + picH + under
			out[i] = feedPost{top: y, pic: y + phoneFeedHead, picH: picH, h: h, size: size, lines: lines}
		}
		out[i].page = page
		if across {
			p.layoutEnd = float64(page) * w
		} else {
			p.layoutEnd = math.Max(0, out[i].top+out[i].h-view)
		}
		page++
		if !across {
			y += out[i].h + phoneFeedGap
		}
	}
	p.layout, p.layoutW, p.layoutH = out, w, view
	return out
}

// across tells that the feed's posts go sideways, a page each (scroll: across).
func (p *phoneView) across() bool { return p.cue.Scroll == "across" }

// Shapes of a post's box (height over width), as photo apps keep them: from
// a landscape 16:9 to a portrait 4:5.
const (
	phoneFeedWide = 9.0 / 16
	phoneFeedTall = 5.0 / 4
)

// mediaAspect is the shape of a post's box: its picture's, or its footage's
// first frame's, between 16:9 and 4:5. A clip of a picture and a puppet, and a
// picture not loaded, keep the portrait 4:5.
func (g *Game) mediaAspect(post data.PhonePost) float64 {
	path := post.Picture
	if v := post.Video; v != nil {
		if len(v.Frames) == 0 {
			return phoneFeedTall
		}
		path = v.Frames[0]
	}
	img := g.pics.get(path)
	if img == nil || img == missing || img.Bounds().Dx() == 0 {
		return phoneFeedTall
	}
	a := float64(img.Bounds().Dy()) / float64(img.Bounds().Dx())
	return math.Max(phoneFeedWide, math.Min(phoneFeedTall, a))
}

// feedView is the feed's screen: the posts laid out, the height they show in
// and the farthest they scroll (across, the last page's left edge).
func (g *Game) feedView(p *phoneView) (posts []feedPost, w, view, end float64) {
	_, _, w, h := g.phoneScreenRect(p.side)
	view = h - phoneFeedTop
	posts = p.feedLayout(g, w, view)
	end = p.layoutEnd
	return posts, w, view, end
}

// feedShown tells that post i's picture is on the screen.
func (p *phoneView) feedShown(i int, l feedPost, w, view float64) bool {
	if p.cue.Posts[i].Hidden {
		return false
	}
	if p.across() {
		return math.Abs(float64(l.page)*w-p.scroll) < w
	}
	return l.pic+l.picH > p.scroll && l.pic < p.scroll+view
}

// stepFeedClips plays, looping, the clips of the posts whose picture is on the screen.
func (g *Game) stepFeedClips(p *phoneView) {
	posts, w, view, _ := g.feedView(p)
	for i, post := range p.cue.Posts {
		if post.Video == nil || !p.feedShown(i, posts[i], w, view) {
			continue
		}
		key := fmt.Sprintf("post%d", i)
		p.clip(key, post.Video).step(g, true)
	}
}

func (g *Game) updatePhoneFeed(p *phoneView) {
	posts, w, _, end := g.feedView(p)
	like := func(at image.Point) {
		if i := g.feedPictureAt(p, posts, at); i >= 0 && !p.cue.Browse {
			p.liked[i] = !p.liked[i]
		}
	}
	stops := make([]float64, 0, len(posts))
	for _, l := range posts {
		if l.h == 0 {
			continue
		}
		stop := l.top
		if p.across() {
			stop = float64(l.page) * w
		}
		stops = append(stops, stop)
	}
	if p.across() {
		g.swipePhone(p, stops, w, end, like)
		return
	}
	g.scrollPhone(p, stops, end, like)
}

// swipePhone pages a row of the phone's pages w wide (a feed across): a drag
// sideways follows the finger and, let go, settles on the nearest page or,
// flicked, on the next one that way; left and right, up and down, a notch of
// the wheel and OK step a page, and OK on the last puts the phone down.
func (g *Game) swipePhone(p *phoneView, stops []float64, w, end float64, tap func(image.Point)) {
	in := &g.in
	was := p.grab >= 0
	g.dragPhoneAlong(p, true, tap)
	if was && p.grab < 0 && p.moved >= phoneFeedTap {
		at := p.scroll
		switch {
		case p.glide < -phoneFeedFling:
			at = math.Ceil(p.scroll/w) * w
		case p.glide > phoneFeedFling:
			at = math.Floor(p.scroll/w) * w
		}
		p.aim, p.glide = math.Round(at/w)*w, 0
	}
	switch {
	case in.pressedKey(btnRight) || in.pressedKey(btnDown) || in.wheel < 0 && p.aim < 0:
		p.aim = p.scrollNext(stops, 1)
	case in.pressedKey(btnLeft) || in.pressedKey(btnUp) || in.wheel > 0 && p.aim < 0:
		p.aim = p.scrollNext(stops, -1)
	case in.ok():
		if p.scroll >= end-1 {
			p.closing = phoneRise
			return
		}
		p.aim = p.scrollNext(stops, 1)
	}
	p.settle(end)
}

// scrollPhone scrolls a column of the phone (a feed's posts, a page's rows):
// the wheel, a drag gliding on after it, and the up and down keys stepping
// between the stops, no farther than end; OK steps down and, at the bottom,
// puts the phone down. A tap (a press that did not drag) goes to tap.
func (g *Game) scrollPhone(p *phoneView, stops []float64, end float64, tap func(image.Point)) {
	in := &g.in
	g.dragPhoneAlong(p, false, tap)
	switch {
	case in.pressedKey(btnDown):
		p.aim = p.scrollNext(stops, 1)
	case in.pressedKey(btnUp):
		p.aim = p.scrollNext(stops, -1)
	case in.ok():
		if p.scroll >= end-1 {
			p.closing = phoneRise
			return
		}
		p.aim = p.scrollNext(stops, 1)
	}
	if in.wheel != 0 {
		p.aim, p.glide = -1, 0
		p.scroll -= in.wheel * phoneFeedWheel
	}
	p.settle(end)
}

// settle glides the scroll to the aim, if any, and keeps it between 0 and end.
func (p *phoneView) settle(end float64) {
	if p.aim >= 0 {
		p.aim = math.Min(p.aim, end)
		if p.scroll += (p.aim - p.scroll) * 0.25; math.Abs(p.aim-p.scroll) < 0.5 {
			p.scroll, p.aim = p.aim, -1
		}
	}
	if to := math.Max(0, math.Min(p.scroll, end)); to != p.scroll {
		p.scroll, p.glide = to, 0
	}
}

// scrollNext is the scroll that brings the next stop (d 1) or the previous
// one (d -1) under the header.
func (p *phoneView) scrollNext(stops []float64, d int) float64 {
	at := p.scroll
	if p.aim >= 0 {
		at = p.aim
	}
	if d > 0 {
		for _, top := range stops {
			if top > at+1 {
				return top
			}
		}
		return at
	}
	to := 0.0
	for _, top := range stops {
		if top < at-1 {
			to = top
		}
	}
	return to
}

// dragPhoneAlong follows a finger or the mouse dragged up and down (or, across,
// sideways), gliding on when it lets go; a press that hardly moved is a tap.
func (g *Game) dragPhoneAlong(p *phoneView, across bool, tap func(image.Point)) {
	in := &g.in
	y := float64(in.mouse.Y)
	if across {
		y = float64(in.mouse.X)
	}
	switch {
	case in.pointerDown():
		p.grab, p.moved, p.glide, p.aim = y, 0, 0, -1
	case p.grab >= 0 && in.pointerHeld():
		d := y - p.grab
		p.grab, p.scroll, p.moved = y, p.scroll-d, p.moved+math.Abs(d)
		p.glide = 0.5*p.glide + 0.5*d
	}
	if p.grab >= 0 && in.pointerUp() {
		// The pointer may move on the tick it lets go (with no tick held between).
		d := y - p.grab
		p.scroll, p.moved = p.scroll-d, p.moved+math.Abs(d)
		p.grab = -1
		if p.moved < phoneFeedTap {
			p.glide = 0
			tap(in.mouse)
		}
		return
	}
	if p.grab < 0 && p.glide != 0 && !across {
		p.scroll -= p.glide
		if p.glide *= flingSlow; math.Abs(p.glide) < 0.2 {
			p.glide = 0
		}
	}
}

// phoneColumnAt is how far down the scrolled column a point on the game's
// screen is, and whether it is on the column at all (under the header).
func (g *Game) phoneColumnAt(p *phoneView, at image.Point) (float64, bool) {
	sx, sy, w, sh := g.phoneScreenRect(p.side)
	x, y := float64(at.X)-sx, float64(at.Y)-sy
	if x < 0 || x >= w || y < phoneFeedTop || y >= sh {
		return 0, false
	}
	return y + p.scroll - phoneFeedTop, true
}

// feedPictureAt is the post whose picture, or the row of its heart and likes
// under it, is under at (a point on the game's screen), or -1: a tap there
// likes the post, as one on the heart does in a photo app. Across, the page
// under the pointer's x is the one tapped: while two pages show in a slide,
// the nearest page is not always the one under the finger, and a like now
// changes the story.
func (g *Game) feedPictureAt(p *phoneView, posts []feedPost, at image.Point) int {
	y, ok := g.phoneColumnAt(p, at)
	if !ok {
		return -1
	}
	page := -1
	if p.across() {
		sx, _, w, _ := g.phoneScreenRect(p.side)
		y -= p.scroll
		page = int(math.Floor((p.scroll + float64(at.X) - sx) / w))
	}
	for i, l := range posts {
		if l.h == 0 || p.across() && l.page != page {
			continue
		}
		if y >= l.pic && y < l.pic+l.picH+phoneFeedLikes {
			return i
		}
	}
	return -1
}

func (g *Game) drawPhoneFeed(p *phoneView, dst *ebiten.Image, w, h float64) {
	textStart, controlStart := g.screenOffsets()
	for i, l := range p.feedLayout(g, w, h-phoneFeedTop) {
		if l.h == 0 {
			continue
		}
		x, y := 0.0, phoneFeedTop+l.top-p.scroll
		if p.across() {
			x, y = float64(l.page)*w-p.scroll, phoneFeedTop
			if x <= -w || x >= w {
				continue
			}
			// A page is the post's card down to the screen's bottom.
			gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(h-y), phoneBubble, false)
		} else if y+l.h <= phoneFeedTop || y >= h {
			continue
		}
		g.drawPhonePost(dst, p, i, l, x, y, w)
	}
	if p.across() {
		visible := 0
		for _, post := range p.cue.Posts {
			if !post.Hidden {
				visible++
			}
		}
		drawFeedDots(dst, visible, p.scroll/w, w, h-phoneFeedDots/2)
	}
	// The header goes over the posts that scroll up under it.
	g.clipScreenRecords(textStart, controlStart, phoneFeedTop, w, h)
	g.drawPhoneHeader(dst, w, p.cue.Title, 34)
}

// drawFeedDots draws a dot a page across, centered on a screen w wide at y, the
// page at (a page and a fraction while it slides) dark.
func drawFeedDots(dst *ebiten.Image, n int, at, w, y float64) {
	if n < 2 {
		return
	}
	const gap = 12.0
	x := w/2 - float64(n-1)*gap/2
	for i := range n {
		clr := phoneMuted
		if math.Round(at) == float64(i) {
			clr = phoneInk
		}
		gfx.FillCircle(dst, float32(x+float64(i)*gap), float32(y), 3.5, clr, true)
	}
}

func (g *Game) drawPhonePost(dst *ebiten.Image, p *phoneView, i int, l feedPost, x, y, w float64) {
	post := p.cue.Posts[i]
	gfx.FillRect(dst, float32(x), float32(y), float32(w), float32(l.h), phoneBubble, false)
	g.drawPhoneIcon(dst, p, post.Icon, post.Who, x+12, y+10, 38)
	g.text(dst, gfx.Ellipsis(post.Who, 16, w-80-gfx.TextWidth(post.Time, 13)), x+60, y+18, 16, phoneInk)
	g.screenTextRight(dst, post.Time, x+w-14, y+20, 13, phoneMuted)
	py := y + phoneFeedHead
	if g.controlFrame() != nil {
		reason := "transition"
		if p.cue.Browse {
			reason = "browse"
		}
		g.traceControl("", roundRect(x, py, w, l.picH+phoneFeedLikes), p.ready() && !p.cue.Browse, false, reason)
	}
	switch {
	case l.text:
		for k, line := range l.lines {
			g.text(dst, line, x+14, py+float64(k)*gfx.LineHeight(l.size), l.size, phoneInk)
		}
	case post.Video != nil:
		key := fmt.Sprintf("post%d", i)
		g.drawPhoneMedia(dst, p, key, p.clip(key, post.Video), x, py, w, l.picH, 0)
		drawPlay(dst, x+w-24, py+22, 9)
	default:
		if img := g.pics.get(post.Picture); img != nil {
			drawCoverTop(dst, img, x, py, w, l.picH)
		}
	}
	// The heart, the likes and the caption under the picture.
	ly := py + l.picH + 14
	likes := post.Likes
	if p.liked[i] {
		likes++
		drawHeart(dst, x+26, ly+11, 12, phoneHeart, true)
	} else {
		drawHeart(dst, x+26, ly+11, 12, phoneInk, false)
	}
	counts := likeCount(likes)
	if post.Replies > 0 {
		counts += "　" + fmt.Sprintf(lang.T("返信 %s 件"), strconv.Itoa(post.Replies))
	}
	if post.Reposts > 0 {
		counts += "　" + fmt.Sprintf(lang.T("リポスト %s 件"), strconv.Itoa(post.Reposts))
	}
	g.text(dst, gfx.Ellipsis(counts, 16, w-60), x+48, ly, 16, phoneInk)
	if l.text {
		return
	}
	for k, line := range l.lines {
		g.text(dst, line, x+14, ly+30+float64(k)*gfx.LineHeight(l.size), l.size, phoneInk)
	}
}
