//go:build graphics

package play

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/rules/event"
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

// hairPNG is a 200 by 600 picture opaque from opaqueTop down: a figure whose hair
// rises far above a face box at 140 to 220 when opaqueTop is small.
func hairPNG(t *testing.T, opaqueTop int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 200, 600))
	draw.Draw(img, image.Rect(40, opaqueTop, 160, 600), &image.Uniform{color.NRGBA{R: 200, G: 60, B: 90, A: 255}}, image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestGraphicsCallKeepsHairInView draws a picture self view, small and on the stage, whose
// hair rises well above its face: the hair stays inside the view (the trace's layout
// figure has its opaque top inside its clip), a picture drawn cut at its own top keeps
// its framing, and self_backdrop is drawn behind the picture.
func TestGraphicsCallKeepsHairInView(t *testing.T) {
	files := map[string]string{"assets/me.png": hairPNG(t, 8), "assets/cut.png": hairPNG(t, 0), "assets/room.png": string(btlPNG(64, 36))}
	for _, self := range []string{"me.png", "cut.png"} {
		t.Run(self, func(t *testing.T) { callHair(t, self, files) })
	}
}

func callHair(t *testing.T, self string, files map[string]string) {
	t.Helper()
	g := callGame(t, "{address: call.example.test, layout: full, captions: overlay, others: side, still: true, self: "+self+", self_stage: true, self_backdrop: room.png}", files)
	g.pics.mu.Lock()
	g.pics.found, g.pics.foundRead = map[string]data.FaceBox{self: {Box: &[4]int{60, 140, 140, 220}, Size: [2]int{200, 600}}}, true
	g.pics.mu.Unlock()
	sw, sh := g.screen()
	dst := ebiten.NewImage(int(sw), int(sh))
	// A first draw warms the pictures the call names (callPictures), so the check
	// below sees what drawCallSelf draws, not what the preloading reads.
	(&authorScreen{name: "studio"}).drawCall(g, dst)
	for _, say := range []event.Say{{Ensemble: "studio", Actor: "weaver", Name: "Weaver", Text: "The loom is strung."}, {Ensemble: "studio", Actor: "me", Name: "Me", Text: "Thank you."}} {
		g.talk.open(g, say)
		drawn := map[string]bool{}
		g.pics.seen = func(path string, _ bool) { drawn[path] = true }
		g.tr.layout = &layoutFrame{}
		(&authorScreen{name: "studio"}).drawCall(g, dst)
		figures := g.tr.layout.figures
		g.tr.layout, g.pics.seen = nil, nil
		if !drawn["room.png"] {
			t.Fatalf("%s, %s speaking: self_backdrop not drawn: %v", self, say.Actor, drawn)
		}
		found := false
		for _, f := range figures {
			if f.Picture != self {
				continue
			}
			found = true
			cut := len(f.SourceCut) > 0 && f.SourceCut[0] == "top"
			if (self == "cut.png") != cut {
				t.Fatalf("%s: source_cut %v", self, f.SourceCut)
			}
			if !cut && f.Content[1] < f.Clip[1] {
				t.Fatalf("%s, %s speaking: hair top %v above the view's top %v", self, say.Actor, f.Content[1], f.Clip[1])
			}
			if cut && f.Content[1] >= f.Clip[1] {
				t.Fatalf("%s: a picture drawn cut moved down to %v in a view at %v", self, f.Content[1], f.Clip[1])
			}
		}
		if !found {
			t.Fatalf("%s, %s speaking: no figure in %+v", self, say.Actor, figures)
		}
	}
	dst.Deallocate()
}
