package play

import (
	"image"
	"math"
	"strconv"
	"strings"
	"testing"

	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
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

func TestBacklogOpensOnOversizedLastEntry(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	g.backlog = nil
	g.remember(logLine{text: "An earlier line."})
	g.remember(logLine{text: strings.Repeat("A long remembered line.\n", 100)})
	g.in = input{pressed: [numButtons]bool{btnLog: true}}
	if !g.novelInput() || g.logView == nil {
		t.Fatal("backlog did not open")
	}
	if g.logView.top != 1 {
		t.Fatalf("backlog starts at %d of %d entries, want the last entry", g.logView.top, len(g.backlog))
	}
}

func TestNovelToolbarTraceReadiness(t *testing.T) {
	for _, tc := range []struct {
		name           string
		button         button
		choice, replay bool
	}{
		{name: "choice skip", button: btnSkip, choice: true},
		{name: "choice hide", button: btnHide, choice: true},
		{name: "empty log", button: btnLog},
		{name: "replay save", button: btnSave, replay: true},
		{name: "replay load", button: btnLoad, replay: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := wldNovel(t, "ev_auto")
			g.backlog = nil
			g.replaying = tc.replay
			if tc.choice {
				g.talk.choice = newList([]listItem{{label: "Wait"}}, 100, 100, 300, 1)
			}
			g.tr = &tracer{layout: &layoutFrame{}}
			g.talk.drawNovel(g, btlImage())
			var label string
			for _, b := range g.novelButtons() {
				if b.b == tc.button {
					label = lang.T(b.label)
				}
			}
			found := false
			for _, c := range g.tr.layout.controls {
				if c.Label != label {
					continue
				}
				found = true
				if c.Enabled || c.Reason == "" {
					t.Errorf("inactive toolbar target advertised as usable: %+v", c)
				}
			}
			if !found {
				t.Fatal("toolbar target missing")
			}
			g.in = input{}
			g.in.pressed[tc.button] = true
			if g.novelInput() {
				t.Fatal("disabled toolbar action unexpectedly acted")
			}
			g.talk.choice, g.replaying = nil, false
			g.remember(logLine{text: "Earlier dialogue"})
			g.tr.layout = &layoutFrame{}
			g.talk.drawNovel(g, btlImage())
			for _, c := range g.tr.layout.controls {
				if c.Label == label && (!c.Enabled || c.Reason != "") {
					t.Fatalf("available target remained disabled: %+v", c)
				}
			}
		})
	}
}
