package play

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"

	"github.com/nao1215/gozen/internal/layer5/gfx"
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
	in := &g.in
	posts, w, _, end := g.feedView(p)
	g.dragPhoneFeed(p, posts, w)
	switch {
	case in.pressed[btnDown]:
		p.aim = p.feedNext(posts, 1)
	case in.pressed[btnUp]:
		p.aim = p.feedNext(posts, -1)
	case in.ok():
		if p.scroll >= end-1 {
			p.closing = phoneRise
			return
		}
		p.aim = p.feedNext(posts, 1)
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

// feedNext is the scroll that brings the next post's top (d 1) or the
// previous one's (d -1) under the header.
func (p *phoneView) feedNext(posts []feedPost, d int) float64 {
	at := p.scroll
	if p.aim >= 0 {
		at = p.aim
	}
	if d > 0 {
		for _, l := range posts {
			if l.top > at+1 {
				return l.top
			}
		}
		return at
	}
	to := 0.0
	for _, l := range posts {
		if l.top < at-1 {
			to = l.top
		}
	}
	return to
}

// dragPhoneFeed follows a finger or the mouse dragged up and down, gliding on
// when it lets go; a tap on a post's picture likes it (or takes the like back).
func (g *Game) dragPhoneFeed(p *phoneView, posts []feedPost, w float64) {
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
			if i := g.feedPictureAt(p, posts, w, in.mouse); i >= 0 {
				p.liked[i] = !p.liked[i]
			}
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

// feedPictureAt is the post whose picture is under at (a point on the game's
// screen), or -1.
func (g *Game) feedPictureAt(p *phoneView, posts []feedPost, w float64, at image.Point) int {
	sx, sy, _, sh := g.phoneScreenRect(p.side)
	x, y := float64(at.X)-sx, float64(at.Y)-sy
	if x < 0 || x >= w || y < phoneFeedTop || y >= sh {
		return -1
	}
	y += p.scroll - phoneFeedTop
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
	if post.Video != nil {
		key := fmt.Sprintf("post%d", i)
		g.drawPhoneMedia(dst, p, key, p.clip(key, post.Video), 0, py, w, l.picH, 0)
		drawPlay(dst, w-24, py+22, 9)
	} else if img := g.pics.get(post.Picture); img != nil {
		drawCoverTop(dst, img, 0, py, w, l.picH)
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
	gfx.DrawTextLeft(dst, likeCount(likes), 48, ly, 16, phoneInk)
	for k, line := range l.lines {
		gfx.DrawTextLeft(dst, line, 14, ly+30+float64(k)*gfx.LineHeight(15), 15, phoneInk)
	}
}
