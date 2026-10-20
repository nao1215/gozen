package play

import (
	"math"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/layer5/gfx"
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
}

type noticeBoard struct{ list []notice }

func noticeLen(text string) int {
	return max(noticeTicks, 60+utf8.RuneCountInString(text)*6)
}

func (b *noticeBoard) add(text string) {
	n := noticeLen(text)
	for i := range b.list {
		if b.list[i].text == text {
			b.list[i].t = max(b.list[i].t, n-12)
			return
		}
	}
	b.list = append(b.list, notice{text: text, t: n, total: n})
	if len(b.list) > noticeMax+noticeQueue {
		b.list = b.list[len(b.list)-noticeMax-noticeQueue:]
	}
}

func (b *noticeBoard) update() {
	out := b.list[:0]
	for i, n := range b.list {
		if i >= noticeMax {
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

func (b *noticeBoard) draw(g *Game, dst *ebiten.Image) {
	if len(b.list) == 0 {
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

		for k, l := range lines {
			gfx.DrawTextOutlineColor(dst, l, cx, y-size*0.55+lh*float64(k), size, g.th.text, g.th.window, float32(a))
		}
		y += h + 6
	}
}
