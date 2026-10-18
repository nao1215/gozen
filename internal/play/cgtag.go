package play

import (
	"github.com/hajimehoshi/ebiten/v2"

	lang "github.com/nao1215/gozen/text"
)

func (g *Game) updateTag() bool {
	if g.tag != nil && g.tag.box == nil {
		g.tag.toastT = max(0, g.tag.toastT-1)
		if g.tag.toastT == 0 {
			g.tag = nil
		}
	}
	if g.tag != nil && g.tag.box != nil {
		g.tag.updateBox(g)
		return g.tag != nil && g.tag.box != nil
	}
	if !g.in.pressed[btnTag] || g.rep != nil || g.auto != nil || g.db == nil || !g.cgReviewEnabled() || g.reviewOpen() != nil || g.tagKeyTaken() {
		return false
	}
	page := shownPicture(g.cg)
	if page == "" || g.db.StandIn(page) {
		g.tag = &cgReview{inPlay: true}
		g.tag.say(lang.T("一枚絵が出ていない"))
		return false
	}
	g.openTag(page)
	return true
}

func (g *Game) openTag(page string) {
	srcs := loadReviewSources(g.opts.ReviewSources)
	r := &cgReview{inPlay: true}
	r.load(g)
	r.items = []reviewItem{{page: page, slot: reviewSlot(page), src: srcs[page], group: -1}}
	r.all = r.items
	r.openBox(g)
	g.tag = r
}

func (g *Game) drawTag(screen *ebiten.Image) {
	r := g.tag
	if r == nil {
		return
	}
	if r.box != nil && len(r.items) > 0 {
		r.drawBox(g, screen, r.items[0])
	}
	r.drawToast(g, screen)
}

func (g *Game) tagKeyTaken() bool {
	if len(g.stack) == 0 {
		return false
	}
	t, ok := g.stack[len(g.stack)-1].(interface{ takesTagKey() bool })
	return ok && t.takesTagKey()
}
