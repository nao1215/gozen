package play

import (
	"math"
	"slices"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/screen/gfx"
)

const (
	noticeTicks = 170
	noticeMax   = 3
	noticeQueue = 8
)

type notice struct {
	text  string
	t     int
	total int
	// act is what a tap on the notice does (nil for a notice that only tells),
	// and at where it was last drawn (x, y, w, h; no width while off the screen).
	act func()
	at  [4]float64
}

type noticeBoard struct{ list []notice }

func noticeLen(text string) int {
	return max(noticeTicks, 60+utf8.RuneCountInString(text)*6)
}

func (b *noticeBoard) add(text string) { b.offer(text, nil) }

// offer adds a notice that does act when the player taps it: an offer that
// asks nothing and goes away by itself, where a question would stop the play.
func (b *noticeBoard) offer(text string, act func()) {
	n := noticeLen(text)
	for i := range b.list {
		if b.list[i].text == text {
			b.list[i].t = max(b.list[i].t, n-12)
			b.list[i].act = act
			return
		}
	}
	b.list = append(b.list, notice{text: text, t: n, total: n, act: act})
	if len(b.list) > noticeMax+noticeQueue {
		b.list = b.list[len(b.list)-noticeMax-noticeQueue:]
	}
}

// tapNotice runs the offer under a tap and tells that it took the tap. Only
// the notices over a scene with nothing on top of it take one.
func (g *Game) tapNotice() bool {
	in := &g.in
	if !in.inspect(in.click) || len(g.stack) != 1 || g.phone != nil || !g.noticesShown() {
		return false
	}
	x, y := float64(in.mouse.X), float64(in.mouse.Y)
	for i, n := range g.notices.list {
		if n.act == nil || n.at[2] == 0 || x < n.at[0] || x >= n.at[0]+n.at[2] || y < n.at[1] || y >= n.at[1]+n.at[3] {
			continue
		}
		in.query(true)
		g.notices.list = slices.Delete(g.notices.list, i, i+1)
		n.act()
		return true
	}
	return false
}

func (b *noticeBoard) update(visible int) {
	out := b.list[:0]
	for i, n := range b.list {
		if i >= visible {
			out = append(out, n)
			continue
		}
		n.t--
		if n.t > 0 {
			out = append(out, n)
		}
	}
	b.list = out
}

// A fullscreen authored surface owns its footer, including short results.
// Other screens keep the ordinary floating notice stack.
func (g *Game) noticeSurface() *authorScreen {
	if len(g.stack) == 0 {
		return nil
	}
	s, ok := g.stack[len(g.stack)-1].(*authorScreen)
	if !ok {
		return nil
	}
	def := g.db.Game.Screens[s.name]
	if def == nil || !def.View.Fullscreen {
		return nil
	}
	return s
}

func (g *Game) noticeVisible() int {
	if g.noticeSurface() != nil {
		return 1
	}
	return noticeMax
}

func (b *noticeBoard) draw(g *Game, dst *ebiten.Image) {
	if len(b.list) == 0 || g.noticeSurface() != nil {
		return
	}
	sw, _ := g.screen()
	cx, y, most, wrap := sw/2, 168.0, sw*0.9, false
	if len(g.stack) == 1 {
		if _, ok := g.stack[0].(*field); ok {
			left := 36 + fieldHeadW + 12
			top := goldBoxY + goldBoxH + 8 + 60 + 24
			cx, y, most, wrap = (left+sw-36)/2, top+30, sw-36-left, true
		}
	}
	for i := range b.list {
		b.list[i].at = [4]float64{}
	}
	for i, n := range b.list {
		if i >= noticeMax {
			break
		}
		a := math.Min(1, float64(n.total-n.t)/12)
		if n.t < 30 {
			a = float64(n.t) / 30
		}
		size := sizeS
		lines := []string{n.text}
		w := gfx.TextWidth(n.text, size) + 48
		if w > most {
			if wrap {
				lines = gfx.Wrap(n.text, size, most-48)
				w = most
			} else {
				size *= most / w
				w = most
			}
		}
		lh := gfx.LineHeight(size)
		h := size + 18 + lh*float64(len(lines)-1)
		top := y - (size+18)/2
		gfx.FillRect(dst, float32(cx-w/2), float32(top), float32(w), float32(h), withAlpha(g.th.window, uint8(200*a)), false)
		if n.act != nil {
			// An offer shows it can be pressed: a frame in the accent color.
			b.list[i].at = [4]float64{cx - w/2, top, w, h}
			gfx.StrokeRoundRect(dst, float32(cx-w/2), float32(top), float32(w), float32(h), 3, 2, withAlpha(g.th.accent, uint8(255*a)))
			g.traceControl(n.text, roundRect(cx-w/2, top, w, h), len(g.stack) == 1, false, "covered")
		}

		for k, l := range lines {
			ly := y - size*0.55 + lh*float64(k)
			gfx.DrawTextOutlineColor(dst, l, cx, ly, size, g.th.text, g.th.window, float32(a))
			if g.tr != nil && a >= 1 {
				// The outlined text is centered on cx; ly is its top.
				lw := gfx.TextWidth(l, size)
				g.traceText(l, cx-lw/2, ly, lw, lh, size, g.th.text)
			}
		}
		y += h + 6
	}
}
