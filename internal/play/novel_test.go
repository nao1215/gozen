package play

import (
	"image"
	"math"
	"strconv"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	lang "github.com/nao1215/gozen/internal/text"
)

// ui.hide_bar_on_choice hides the buttons under the novel window while a choice is shown,
// and a click there no longer presses them; without it they stay.
func TestNovelBarHiddenOnChoice(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel, hide_bar_on_choice: true}\n", "")
	g.events.Start([]data.Command{{Prompt: "Which?", Choice: []data.ChoiceOption{{Label: "A"}, {Label: "B"}}}})
	btlUntil(t, g, "the choice", 300, func() bool { return g.talk.choice != nil })
	if g.novelBarShown() {
		t.Fatal("the bar shows under a choice")
	}
	r := g.novelButtonRects()[1] // the log
	g.in.mouse.X, g.in.mouse.Y, g.in.click = r.Min.X+2, r.Min.Y+2, true
	g.novelInput()
	if g.logView != nil || !g.in.click {
		t.Fatal("a click on the hidden log button was taken by it")
	}
	g.talk.draw(g, btlImage())
	g.db.Game.UI.HideBarOnChoice = false
	if !g.novelBarShown() {
		t.Fatal("the bar is hidden without ui.hide_bar_on_choice")
	}
}

// Every glyph stays inside the clickable band, including its padding.
func TestNovelButtonLabelEdges(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	for i, r := range g.novelButtonRects() {
		label := lang.T(g.novelButtons()[i].label)
		width := gfx.TextWidth(label, sizeS)
		left := float64(r.Min.X) + (float64(r.Dx())-width)/2
		for _, x := range []float64{left, left + width/2, math.Ceil(left+width) - 1} {
			if !image.Pt(int(x), r.Min.Y+2).In(r) {
				t.Fatalf("%s escapes %v at %v", label, r, x)
			}
		}
	}
	// A click at the rightmost glyph of Settings opens Settings, not the next line.
	r := g.novelButtonRects()[len(g.novelButtonRects())-1]
	before := wldTalkText(g)
	g.in.mouse, g.in.click = image.Pt(r.Max.X-15, r.Min.Y+2), true
	if !g.novelInput() || g.in.click {
		t.Fatal("label edge was not consumed")
	}
	if _, ok := g.stack[len(g.stack)-1].(*configScreen); !ok {
		t.Fatal("settings not opened")
	}
	if wldTalkText(g) != before {
		t.Fatal("settings advanced dialogue")
	}
}

func TestHiddenNovelPausesAndRestores(t *testing.T) {
	for _, rightClick := range []bool{false, true} {
		t.Run(strconv.FormatBool(rightClick), func(t *testing.T) {
			g := wldNovel(t, "ev_auto")
			wldFrames(t, g, 80)
			before := wldTalkText(g)
			g.autoRead = true
			g.in.rclick = rightClick
			g.in.pressed[btnHide] = !rightClick
			if !g.novelInput() || !g.talk.hidden {
				t.Fatal("not hidden")
			}
			g.in = input{}
			wldFrames(t, g, 400)
			if wldTalkText(g) != before {
				t.Fatal("hidden dialogue advanced")
			}
			g.in.click = true
			g.novelInput()
			if g.talk.hidden || wldTalkText(g) != before {
				t.Fatal("restore advanced dialogue")
			}
		})
	}
}
