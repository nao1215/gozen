package play

import (
	"fmt"
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// The feed of the phone (ADR 0057) is a column of posts scrolled up and down:
// with the wheel, the up and down keys (a post at a time) and a finger or the
// mouse dragged, gliding on when it lets go.
const (
	phoneFeedTop   = 85.0 // under the status bar and the header
	phoneFeedGap   = 14.0 // between two posts
	phoneFeedHead  = 58.0 // the poster's icon and name over the picture
	phoneFeedWheel = 60.0 // UI units a notch of the wheel scrolls
	phoneFeedTap   = 8.0  // UI units a pointer may move and still tap
)

// feedPost is a post laid out in the feed, from the feed's top unscrolled: its
// top, its picture's top and height, its caption's lines and its height.
type feedPost struct {
	top, pic, picH, h float64
	lines             []string
	// text tells a post of text only: its lines fill the picture's place.
	text bool
}

// feedLayout lays the posts out top to bottom on a screen w wide (once per
// width), each picture's box at its media's own shape.
func (p *phoneView) feedLayout(g *Game, w float64) []feedPost {
	if p.layout != nil && p.layoutW == w {
		return p.layout
	}
	out := make([]feedPost, len(p.cue.Posts))
	y := 0.0
	for i, post := range p.cue.Posts {
		if post.Picture == "" && post.Video == nil {
			lines, _ := gfx.FitLines(post.Text, 16, w-28, 8)
			textH := float64(len(lines)) * gfx.LineHeight(16)
			h := phoneFeedHead + textH + 44
			out[i] = feedPost{top: y, pic: y + phoneFeedHead, picH: textH, h: h, lines: lines, text: true}
			y += h + phoneFeedGap
			continue
		}
		caption := post.Text
		if post.Who != "" {
			caption = post.Who + "  " + caption
		}
		lines, _ := gfx.FitLines(caption, 15, w-28, 3)
		picH := w * g.mediaAspect(post)
		h := phoneFeedHead + picH + 46 + float64(len(lines))*gfx.LineHeight(15)
		out[i] = feedPost{top: y, pic: y + phoneFeedHead, picH: picH, h: h, lines: lines}
		y += h + phoneFeedGap
	}
	p.layout, p.layoutW = out, w
	return out
}

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
// and the farthest they scroll.
func (g *Game) feedView(p *phoneView) (posts []feedPost, w, view, end float64) {
	_, _, w, h := g.phoneScreenRect(p.side)
	posts = p.feedLayout(g, w)
	view = h - phoneFeedTop
	if n := len(posts); n > 0 {
		end = math.Max(0, posts[n-1].top+posts[n-1].h-view)
	}
	return posts, w, view, end
}

// stepFeedClips plays, looping, the clips of the posts whose picture is on the screen.
func (g *Game) stepFeedClips(p *phoneView) {
	posts, _, view, _ := g.feedView(p)
	for i, post := range p.cue.Posts {
		l := posts[i]
		if post.Video == nil || l.pic+l.picH <= p.scroll || l.pic >= p.scroll+view {
			continue
		}
		key := fmt.Sprintf("post%d", i)
		p.clip(key, post.Video).step(g, key, true)
	}
}

func (g *Game) updatePhoneFeed(p *phoneView) {
	posts, _, _, end := g.feedView(p)
	stops := make([]float64, len(posts))
	for i, l := range posts {
		stops[i] = l.top
	}
	g.scrollPhone(p, stops, end, func(at image.Point) {
		if i := g.feedPictureAt(p, posts, at); i >= 0 {
			p.liked[i] = !p.liked[i]
		}
	})
}

// scrollPhone scrolls a column of the phone (a feed's posts, a page's rows):
// the wheel, a drag gliding on after it, and the up and down keys stepping
// between the stops, no farther than end; OK steps down and, at the bottom,
// puts the phone down. A tap (a press that did not drag) goes to tap.
func (g *Game) scrollPhone(p *phoneView, stops []float64, end float64, tap func(image.Point)) {
	in := &g.in
	g.dragPhone(p, tap)
	switch {
	case in.pressed[btnDown]:
		p.aim = p.scrollNext(stops, 1)
	case in.pressed[btnUp]:
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

// dragPhone follows a finger or the mouse dragged up and down, gliding on
// when it lets go; a press that hardly moved is a tap.
func (g *Game) dragPhone(p *phoneView, tap func(image.Point)) {
	in := &g.in
	y := float64(in.mouse.Y)
	switch {
	case in.pdown:
		p.grab, p.moved, p.glide, p.aim = y, 0, 0, -1
	case in.pheld && p.grab >= 0:
		d := y - p.grab
		p.grab, p.scroll, p.moved = y, p.scroll-d, p.moved+math.Abs(d)
		p.glide = 0.5*p.glide + 0.5*d
	}
	if in.pup && p.grab >= 0 {
		p.grab = -1
		if p.moved < phoneFeedTap {
			p.glide = 0
			tap(in.mouse)
		}
		return
	}
	if p.grab < 0 && p.glide != 0 {
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

// feedPictureAt is the post whose picture is under at (a point on the game's
// screen), or -1.
func (g *Game) feedPictureAt(p *phoneView, posts []feedPost, at image.Point) int {
	y, ok := g.phoneColumnAt(p, at)
	if !ok {
		return -1
	}
	for i, l := range posts {
		if y >= l.pic && y < l.pic+l.picH {
			return i
		}
	}
	return -1
}

func (g *Game) drawPhoneFeed(p *phoneView, dst *ebiten.Image, w, h float64) {
	for i, l := range p.feedLayout(g, w) {
		y := phoneFeedTop + l.top - p.scroll
		if y+l.h <= phoneFeedTop || y >= h {
			continue
		}
		g.drawPhonePost(dst, p, i, l, y, w)
	}
	// The header goes over the posts that scroll up under it.
	g.drawPhoneHeader(dst, w, p.cue.Title, 34)
}

func (g *Game) drawPhonePost(dst *ebiten.Image, p *phoneView, i int, l feedPost, y, w float64) {
	post := p.cue.Posts[i]
	gfx.FillRect(dst, 0, float32(y), float32(w), float32(l.h), phoneBubble, false)
	g.drawPhoneIcon(dst, p, post.Icon, 12, y+10, 38)
	gfx.DrawTextLeft(dst, gfx.Ellipsis(post.Who, 16, w-80), 60, y+18, 16, phoneInk)
	py := y + phoneFeedHead
	switch {
	case l.text:
		for k, line := range l.lines {
			gfx.DrawTextLeft(dst, line, 14, py+float64(k)*gfx.LineHeight(16), 16, phoneInk)
		}
	case post.Video != nil:
		key := fmt.Sprintf("post%d", i)
		g.drawPhoneMedia(dst, p, key, p.clip(key, post.Video), 0, py, w, l.picH, 0)
		drawPlay(dst, w-24, py+22, 9)
	default:
		if img := g.pics.get(post.Picture); img != nil {
			drawCoverTop(dst, img, 0, py, w, l.picH)
		}
	}
	// The heart, the likes and the caption under the picture.
	ly := py + l.picH + 14
	likes := post.Likes
	if p.liked[i] {
		likes++
		drawHeart(dst, 26, ly+11, 12, phoneHeart, true)
	} else {
		drawHeart(dst, 26, ly+11, 12, phoneInk, false)
	}
	counts := likeCount(likes)
	if post.Replies > 0 {
		counts += "　" + fmt.Sprintf(lang.T("返信 %s 件"), strconv.Itoa(post.Replies))
	}
	if post.Reposts > 0 {
		counts += "　" + fmt.Sprintf(lang.T("リポスト %s 件"), strconv.Itoa(post.Reposts))
	}
	gfx.DrawTextLeft(dst, gfx.Ellipsis(counts, 16, w-60), 48, ly, 16, phoneInk)
	if l.text {
		return
	}
	for k, line := range l.lines {
		gfx.DrawTextLeft(dst, line, 14, ly+30+float64(k)*gfx.LineHeight(15), 15, phoneInk)
	}
}
