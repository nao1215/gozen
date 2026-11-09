package play

import (
	"fmt"
	"image"
	"slices"
	"testing"

	lang "github.com/nao1215/gozen/internal/base/text"
	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/screen/gfx"
)

// quickLabelWidths are the widths of the novel bar's labels: the eight buttons, and
// with the scene skip and the character book the most the bar ever shows.
func quickLabelWidths(all bool) []float64 {
	buttons := novelButtons
	if all {
		buttons = append([]novelButton{sceneSkipButton}, buttons...)
		buttons = append(buttons[:len(buttons)-1:len(buttons)-1], bookButton, buttons[len(buttons)-1])
	}
	out := make([]float64, len(buttons))
	for i, b := range buttons {
		out[i] = gfx.TextWidth(lang.T(b.label), sizeS)
	}
	return out
}

func rectIn(r, outer rectF) bool {
	const e = 1e-6
	return r.x >= outer.x-e && r.y >= outer.y-e && r.x+r.w <= outer.x+outer.w+e && r.y+r.h <= outer.y+outer.h+e
}

func rectsOverlap(a, b rectF) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

// checkQuickFrame holds for every layout: the buttons stay on the screen and in their
// band, each wide enough for its label and apart from the others, and the window stays
// on the screen; below and inside give the band the window's edges.
func checkQuickFrame(t *testing.T, name string, f novelFrame, sw, sh float64, widths []float64) {
	t.Helper()
	screen := rectF{0, 0, sw, sh}
	if !rectIn(f.win, screen) || !rectIn(f.bar, screen) {
		t.Errorf("%s: window %+v or band %+v leaves the screen %vx%v", name, f.win, f.bar, sw, sh)
	}
	for i, b := range f.buttons {
		if !rectIn(b, f.bar) {
			t.Errorf("%s: button %d %+v leaves its band %+v", name, i, b, f.bar)
		}
		if b.w < widths[i] {
			t.Errorf("%s: button %d is %v wide for a label of %v", name, i, b.w, widths[i])
		}
		if i > 0 && rectsOverlap(f.buttons[i-1], b) {
			t.Errorf("%s: buttons %d and %d overlap", name, i-1, i)
		}
	}
	switch f.place {
	case "below":
		if rectsOverlap(f.win, f.bar) {
			t.Errorf("%s: window %+v overlaps the band %+v", name, f.win, f.bar)
		}
	case "inside":
		if !rectIn(f.bar, f.win) || f.bar.y+f.bar.h != f.win.y+f.win.h {
			t.Errorf("%s: band %+v is not in the lower edge of the window %+v", name, f.bar, f.win)
		}
	default:
		if f.strip {
			for i, b := range f.buttons {
				if !rectIn(b, f.win) {
					t.Errorf("%s: button %d %+v is off the strip %+v", name, i, b, f.win)
				}
			}
		} else if rectsOverlap(f.win, f.bar) {
			t.Errorf("%s: window %+v runs under the band %+v", name, f.win, f.bar)
		}
	}
	if f.place != "" {
		if f.bar.x != f.win.x || f.bar.w != f.win.w {
			t.Errorf("%s: band %+v does not take the window's edges %+v", name, f.bar, f.win)
		}
	}
	if f.textL < f.win.x || f.textR > f.win.x+f.win.w || f.textR-f.textL < 200 {
		t.Errorf("%s: lines %v..%v leave the window %+v", name, f.textL, f.textR, f.win)
	}
}

// TestQuickMenuLayout lays the novel window and its buttons out on several screens,
// with and without ui.material, for the three places of ui.quick_menu and windows
// narrowed and moved by ui.novel_window: they never overlap, and below and inside
// give the band the window's left and right edges.
func TestQuickMenuLayout(t *testing.T) {
	wldNovel(t, "ev_auto") // loads the fonts the labels are measured with
	sizes := [][2]float64{{1280, 720}, {1706, 720}, {1280, 800}, {1600, 900}, {1920, 1080}, {2560, 1080}, {960, 720}}
	windows := []*data.NovelWindow{nil, {Width: 0.8}, {Width: 0.6, X: 0.05}, {Width: 0.5, Bottom: 0.05}, {Width: 0.3, X: 0.7}, {Bottom: 0.1}}
	for _, size := range sizes {
		sw, sh := size[0], size[1]
		for _, material := range []bool{false, true} {
			for _, nw := range windows {
				for _, place := range []string{"", "below", "inside"} {
					for _, all := range []bool{false, true} {
						if place == "" && sw < 1280 {
							continue // the row of v0.15 is as wide as its labels, whatever the screen
						}
						ui := &data.UIStyle{QuickMenu: place, NovelWindow: nw}
						widths := quickLabelWidths(all)
						base, strip := novelBase(ui, material, sw, sh)
						f := quickMenu(place, base, strip, sw, sh, widths)
						name := fmt.Sprintf("%vx%v material=%v window=%+v place=%q all=%v", sw, sh, material, nw, place, all)
						checkQuickFrame(t, name, f, sw, sh, widths)
						if place != "" && f.bar.y+f.bar.h != base.y+base.h {
							t.Errorf("%s: band bottom %v, want the window's bottom %v", name, f.bar.y+f.bar.h, base.y+base.h)
						}
						if place == "inside" && f.win.h != base.h+f.bar.h {
							t.Errorf("%s: window %v tall, want %v with the band", name, f.win.h, base.h+f.bar.h)
						}
						if nw != nil && nw.Width > 0 && place != "" && f.win.w != sw*nw.Width {
							t.Errorf("%s: window %v wide, want %v", name, f.win.w, sw*nw.Width)
						}
					}
				}
			}
		}
	}
}

// TestQuickMenuKeepsTheStrip: without ui.quick_menu, ui.material and ui.novel_window,
// the v0.15 strip and its row of buttons stay where they were.
func TestQuickMenuKeepsTheStrip(t *testing.T) {
	wldNovel(t, "ev_auto") // loads the fonts the labels are measured with
	widths := quickLabelWidths(false)
	for _, size := range [][2]float64{{1280, 720}, {1920, 1080}} {
		sw, sh := size[0], size[1]
		base, strip := novelBase(&data.UIStyle{}, false, sw, sh)
		f := quickMenu("", base, strip, sw, sh, widths)
		if !strip || f.win != (rectF{0, sh - novelH, sw, novelH}) || f.textL != 130 || f.textR != sw-130 {
			t.Fatalf("%vx%v the strip moved: %+v", sw, sh, f)
		}
		x := sw - 40
		for i, lw := range slices.Backward(widths) {
			w := lw + 28
			x -= w
			if want := image.Rect(int(x), int(sh-40), int(x+w), int(sh-10)); f.buttons[i].buttonRect() != want {
				t.Errorf("%vx%v button %d at %v, was %v", sw, sh, i, f.buttons[i].buttonRect(), want)
			}
			x -= 10
		}
	}
}

// TestQuickMenuOverCaptions: the captions of a call take the band too, on screens of
// several sizes: they stay on the stage with the self view above them, apart from it.
func TestQuickMenuOverCaptions(t *testing.T) {
	wldNovel(t, "ev_auto") // loads the fonts the labels are measured with
	widths := quickLabelWidths(true)
	for _, size := range [][2]float64{{1280, 720}, {1706, 720}, {1280, 800}, {1920, 1080}, {2560, 1080}} {
		sw, sh := size[0], size[1]
		for _, place := range []string{"", "below", "inside"} {
			g := &Game{db: &data.DB{}}
			g.db.Game.UI.QuickMenu = place
			l := g.callLayout(sw, sh, nil)
			raw := callGeometry(sw, sh)
			f := quickMenu(place, raw.caption, false, sw, sh, widths)
			name := fmt.Sprintf("call %vx%v place=%q", sw, sh, place)
			if f.win != l.caption {
				t.Errorf("%s: captions drawn at %+v, laid out at %+v", name, f.win, l.caption)
			}
			if place == "" {
				if l != raw {
					t.Errorf("%s: the call moved without quick_menu", name)
				}
				continue
			}
			checkQuickFrame(t, name, f, sw, sh, widths)
			for part, r := range map[string]rectF{"captions": l.caption, "band": f.bar, "self": l.self} {
				if !rectIn(r, l.main) {
					t.Errorf("%s: %s %+v leaves the stage %+v", name, part, r, l.main)
				}
			}
			if rectsOverlap(l.self, l.caption) || rectsOverlap(l.self, f.bar) {
				t.Errorf("%s: self view %+v covers the captions %+v or the band %+v", name, l.self, l.caption, f.bar)
			}
		}
		chat := chatCaption(false, sw, sh)
		for _, place := range []string{"below", "inside"} {
			f := quickMenu(place, chat, false, sw, sh, widths)
			checkQuickFrame(t, fmt.Sprintf("chat %vx%v place=%q", sw, sh, place), f, sw, sh, widths)
		}
	}
}

// TestQuickMenuInPlay: in a game, the bar's clicks follow ui.quick_menu: the buttons
// share the window's edges, and the lines wrap within the narrowed window.
func TestQuickMenuInPlay(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	g.db.Game.UI.QuickMenu = "inside"
	g.db.Game.UI.NovelWindow = &data.NovelWindow{Width: 0.8}
	f := g.novelFrame()
	rs := g.novelButtonRects()
	if len(rs) == 0 || rs[0].Min.X < int(f.win.x) || rs[len(rs)-1].Max.X > int(f.win.x+f.win.w)+1 {
		t.Fatalf("buttons %v outside the window %+v", rs, f.win)
	}
	sw, _ := g.screen()
	if w := novelTextWidth(&g.db.Game, sw); w != sw*0.8-52-40 {
		t.Fatalf("lines wrap at %v in a window %v wide", w, sw*0.8)
	}
	g.talk.draw(g, btlImage())
}

// TestCallAboveTheWindow: with call.captions window, the call fits above the novel
// window and its buttons on screens of several sizes, for every place of the buttons:
// its parts stay in the browser frame and apart, the controls under the messages.
func TestCallAboveTheWindow(t *testing.T) {
	wldNovel(t, "ev_auto") // loads the fonts the labels are measured with
	call := &data.EnsembleCall{Captions: "window"}
	for _, size := range [][2]float64{{1280, 720}, {1706, 720}, {1280, 800}, {1600, 900}, {1920, 1080}, {2560, 1080}} {
		sw, sh := size[0], size[1]
		for _, material := range []bool{false, true} {
			for _, place := range []string{"", "below", "inside"} {
				g := &Game{db: &data.DB{}}
				g.db.Game.UI.QuickMenu = place
				if material {
					g.db.Game.UI.Material = &data.MaterialStyle{Kind: "glass"}
				}
				name := fmt.Sprintf("%vx%v material=%v place=%q", sw, sh, material, place)
				l := g.callLayout(sw, sh, call)
				base, strip := novelBase(&g.db.Game.UI, material, sw, sh)
				f := quickMenu(place, base, strip, sw, sh, quickLabelWidths(false))
				if l.frameBottom >= f.win.y || l.frameBottom >= f.bar.y {
					t.Errorf("%s: the call reaches %v, over the window %+v or the band %+v", name, l.frameBottom, f.win, f.bar)
				}
				frame := rectF{18, 16, sw - 36, l.frameBottom - 16}
				parts := map[string]rectF{"main": l.main, "strip": l.strip, "chat": l.chat, "self": l.self}
				controls := rectF{l.controlsX - l.controlR, l.controlsY - l.controlR, 5*l.controlDX + 2*l.controlR, 2 * l.controlR}
				parts["controls"] = controls
				for part, r := range parts {
					if r.w <= 0 || r.h <= 0 || !rectIn(r, frame) {
						t.Errorf("%s: %s %+v leaves the frame %+v", name, part, r, frame)
					}
				}
				for _, pair := range [][2]string{{"main", "strip"}, {"main", "chat"}, {"strip", "chat"}, {"controls", "chat"}, {"controls", "strip"}, {"controls", "main"}} {
					if rectsOverlap(parts[pair[0]], parts[pair[1]]) {
						t.Errorf("%s: %s overlaps %s", name, pair[0], pair[1])
					}
				}
				if !rectIn(l.self, l.main) || l.main.h < 200 {
					t.Errorf("%s: the speaker view %+v is too small or loses the self view %+v", name, l.main, l.self)
				}
			}
		}
	}
}

// TestChoiceInTheCenter: ui.choice_position center puts the novel window's choices in
// the middle of the screen, above the window and its buttons.
func TestChoiceInTheCenter(t *testing.T) {
	for _, place := range []string{"", "inside"} {
		t.Run(place, func(t *testing.T) { choiceInTheCenter(t, place) })
	}
}

func choiceInTheCenter(t *testing.T, place string) {
	t.Helper()
	{
		g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "")
		g.db.Game.UI.ChoicePosition, g.db.Game.UI.QuickMenu = "center", place
		g.events.Start([]data.Command{{Prompt: "Which?", Choice: []data.ChoiceOption{{Label: "A"}, {Label: "B"}}}})
		btlUntil(t, g, "the choice", 300, func() bool { return g.talk.choice != nil })
		l, f := g.talk.choice, g.novelFrame()
		sw, _ := g.screen()
		if l.x+l.w/2 != sw/2 || l.y+l.height() > f.win.y || l.y+l.height() > f.bar.y {
			t.Errorf("place %q: choices at %v,%v (%vx%v), window %+v band %+v", place, l.x, l.y, l.w, l.height(), f.win, f.bar)
		}
		g.talk.draw(g, btlImage())
	}
}
