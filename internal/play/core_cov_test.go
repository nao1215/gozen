package play

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestUpdateReturnsAFailedScreenshot: a screenshot that could not be written stops the game
// at the next update with the error.
func TestUpdateReturnsAFailedScreenshot(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	g.shots.done(errors.New("disk full"))
	if err := g.Update(); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
	// the failure is kept: the game does not run on.
	if err := g.Update(); err == nil {
		t.Fatal("the game went on after a failure")
	}
	g.failure = nil
}

// TestUntilStopsAfterTheShot: --until takes one picture at that tick and then ends the run.
func TestUntilStopsAfterTheShot(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Until: 5}, nil)
	var quit bool
	for i := range 20 {
		err := g.Update()
		if errors.Is(err, ebiten.Termination) {
			quit = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if g.autoShot != "" {
			if g.autoShot != "until_000005" {
				t.Fatalf("shot name %q", g.autoShot)
			}
			g.autoShot = "" // what Draw does after taking it
		}
		if g.tick > 5 {
			t.Fatalf("ran past the until tick: %d at iteration %d", g.tick, i)
		}
	}
	if !quit {
		t.Fatal("never ended")
	}
}

// TestScriptShotHoldsTheGameUntilDrawn: while a script's shot waits to be drawn, updates change
// nothing.
func TestScriptShotHoldsTheGameUntilDrawn(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp", Script: "wait 1\nshot hold\nwait 100\nquit"}, nil)
	for g.in.script.shot == "" {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		g.rendering = true
	}
	tick := g.tick
	for range 5 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if g.tick != tick {
		t.Fatalf("the game ran %d ticks while a shot was waiting", g.tick-tick)
	}
}

// TestFieldNoticesCanBeDropped: with ui.field_notices off, notices raised on the field are
// dropped; with it on they stay.
func TestFieldNoticesCanBeDropped(t *testing.T) {
	g := sysOpen4(t, Options{Location: "camp"}, map[string]string{"game.yaml+": "ui: {field_notices: false}\n"})
	g.notices.add("Found a coin")
	sysFrames(t, g, 3)
	if len(g.notices.list) != 0 {
		t.Fatalf("notices %v on a field that drops them", g.notices.list)
	}
}

// TestFieldNoticesStayByDefault.
func TestFieldNoticesStayByDefault(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	g.notices.add("Found a coin")
	sysFrames(t, g, 3)
	if len(g.notices.list) != 1 {
		t.Fatalf("notices %v", g.notices.list)
	}
	g.notices.add("Found a coin")
	if len(g.notices.list) != 1 {
		t.Fatal("the same notice twice")
	}
	for i := range 30 {
		g.notices.add(strings.Repeat("n", i+1))
	}
	if len(g.notices.list) > noticeMax+noticeQueue {
		t.Fatalf("%d notices queued", len(g.notices.list))
	}
}

// TestToggleFullscreenKeepsTheChoice: the full screen key flips the setting and writes it.
func TestToggleFullscreenKeepsTheChoice(t *testing.T) {
	dir := t.TempDir()
	g := sysOpen(t, Options{SaveDir: dir, Location: "camp"}, nil)
	before := g.cfg.Fullscreen
	g.toggleFullscreen()
	if g.cfg.Fullscreen == before {
		t.Fatal("not toggled")
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil || !strings.Contains(string(b), `"fullscreen": true`) {
		t.Fatalf("config %s %v", b, err)
	}
	g.toggleFullscreen()
	if g.cfg.Fullscreen != before {
		t.Fatal("not toggled back")
	}
}

// TestConfigAppliesOnStart: a config.json with volumes and a window value is applied to the
// theme when the game opens; other fields keep their defaults.
func TestConfigAppliesOnStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"text_speed": 4, "window": 5, "bgm": 2, "voice": 3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	g := sysOpen(t, Options{SaveDir: dir}, nil)
	if g.cfg.TextSpeed != 4 || g.cfg.Window != 5 || g.cfg.AutoWait != 2 {
		t.Fatalf("config %+v", g.cfg)
	}
	if want := uint8(float64(g.windowA) * 5 / 10); g.th.window.A != want {
		t.Fatalf("window alpha %d, want %d", g.th.window.A, want)
	}
	if g.cfg.textStep() != 9999 {
		t.Fatalf("text step %d", g.cfg.textStep())
	}
}

// TestScreenAndLayout: the logical screen is the game's size over the UI scale.
func TestScreenAndLayout(t *testing.T) {
	g := sysOpen(t, Options{}, nil)
	w, h := g.Layout(1, 1)
	if w != g.db.Game.Screen.Width || h != g.db.Game.Screen.Height {
		t.Fatalf("layout %dx%d", w, h)
	}
	g.ui = 0
	sw, sh := g.screen()
	if sw != float64(w) || sh != float64(h) {
		t.Fatalf("screen %vx%v with no scale", sw, sh)
	}
	g.ui = 2
	sw, sh = g.screen()
	if sw != float64(w)/2 || sh != float64(h)/2 {
		t.Fatalf("screen %vx%v at scale 2", sw, sh)
	}
}

// TestPushPopReplaceKeepTheBottomScene: pop never empties the stack; replace clears the talk
// and the picture.
func TestPushPopReplaceKeepTheBottomScene(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	bottom := g.stack[0]
	g.pop()
	if len(g.stack) != 1 || g.stack[0] != bottom {
		t.Fatal("pop emptied the stack")
	}
	g.push(newConfirm("?", func() {}))
	if len(g.stack) != 2 || !g.busy() {
		t.Fatal("an open screen should make the game busy")
	}
	g.cg.File = "x.png"
	g.replace(newTitle(g))
	if len(g.stack) != 1 || g.cg.File != "" {
		t.Fatalf("replace: stack %d cg %q", len(g.stack), g.cg.File)
	}
}

// TestImageCommandsDrawEveryLook: CGs shown with the cover fit, an overlay picture, a dream
// look and a crossfade all draw on a frame, and the pictures are loaded.
func TestImageCommandsDrawEveryLook(t *testing.T) {
	events := `events:
  looks:
    do:
      - image: {file: x.png}
      - say: "plain"
      - image: {file: x.png, fit: cover, brightness: 0.5, over: {file: x.png}, over_size: 0.3}
      - say: "cover"
      - image: {file: x.png, look: dream}
      - say: "dream"
      - image: {files: [x.png, x.png], fade: 0.1}
      - say: "pages"
`
	g := sysOpen4(t, Options{Location: "camp", Event: "looks", Line: 1, Script: "read 3\nread 3\nread 3\nread 3\nread 3\nquit"},
		map[string]string{"data/looks.yaml": events})
	got := sysPlayLines(t, g, 3000)
	if got["plain"].cg != "x.png" || got["cover"].cg != "x.png" || got["dream"].cg != "x.png" {
		t.Fatalf("pictures: %+v", got)
	}
	if g.cg.Look != "" && g.cg.Look != "dream" {
		t.Fatalf("look %q", g.cg.Look)
	}
}
