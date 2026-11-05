//go:build graphics

package play

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
)

// TestCallDrawsAtSeveralSizes draws the call, with a self view and named
// controls, at several logical sizes without touching the simulation.
func TestCallDrawsAtSeveralSizes(t *testing.T) {
	g := callGame(t, `{address: call.example.test, self: potter, self_label: You,
  controls: [{label: Mic, icon: mic}, {label: Hand, icon: hand}, {label: Leave, icon: leave}],
  colors: {speaking: '#ffcc00', tile: '#202020'}}`)
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "The loom is strung, and the warp is even across the whole width."})
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{1280, 720}, {1280, 960}, {1920, 1080}, {1280, 600}} {
		g.db.Game.Screen.Width, g.db.Game.Screen.Height = size[0], size[1]
		g.talk.layout(g)
		for _, page := range g.talk.pages {
			if len(page) == 0 {
				t.Fatalf("%v caption page has %d lines", size, len(page))
			}
		}
		dst := ebiten.NewImage(size[0], size[1])
		(&authorScreen{name: "studio"}).drawCall(g, dst)
		g.talk.drawNovel(g, dst)
		dst.Deallocate()
	}
	after, err := json.Marshal(g.st.Planning)
	if err != nil || string(before) != string(after) {
		t.Fatalf("drawing changed state: %v", err)
	}
}

// TestCallOverlayWindow lays the message window over a full call: it keeps the top and
// the width of the plain window (the lines wrap and page as everywhere else), reaches
// the screen's lower edge, and the call and its side list draw without changing state.
func TestCallOverlayWindow(t *testing.T) {
	g := callGame(t, "{address: call.example.test, layout: full, captions: overlay, others: side, others_title: People, window_opacity: 0.85, self: me.png, self_stage: true}")
	g.db.Game.UI.QuickMenu = "inside"
	sw, sh := g.screen()
	plain := g.novelFrame()
	text := "The loom is strung, and the warp is even across the whole width of the frame."
	g.talk.open(g, event.Say{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: text})
	f := g.novelFrame()
	if f.win.y != plain.win.y || f.win.x != plain.win.x || f.win.w != plain.win.w || f.win.y+f.win.h != sh {
		t.Fatalf("overlay window %+v, plain %+v, screen height %v", f.win, plain.win, sh)
	}
	if v := g.db.Game.Screens["studio"].View; captionsOnStage(v) || !callOverlay(v) {
		t.Fatal("an overlay call shows its lines as captions")
	}
	if pages, _ := MessagePages(&g.db.Game, text); len(pages) != len(g.talk.pages) {
		t.Fatalf("overlay paged the line as %d, the window as %d", len(g.talk.pages), len(pages))
	}
	l := g.callLayout(sw, sh, g.db.Game.Screens["studio"].View.Call)
	if !l.full || l.main.y+l.seen > f.win.y || l.chat.y+l.chat.h > f.win.y {
		t.Fatalf("full layout under the window %+v: %+v", f.win, l)
	}
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	dst := ebiten.NewImage(int(sw), int(sh))
	defer dst.Deallocate()
	for _, say := range []event.Say{{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: text}, {Ensemble: "studio", Actor: "me", Name: "Me", Text: "Thank you."}} {
		g.talk.open(g, say)
		(&authorScreen{name: "studio"}).drawCall(g, dst)
		g.talk.drawNovel(g, dst)
	}
	if after, err := json.Marshal(g.st.Planning); err != nil || string(before) != string(after) {
		t.Fatalf("drawing changed state: %v", err)
	}
}
