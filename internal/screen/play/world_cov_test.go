package play

import (
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The helpers of the tests of the screens that show a place (dungeon, map, painted scene, menu
// place, novel window): a neutral game made in world_fixture_cov_test.go is opened in process
// and driven frame by frame with Update and Draw, the keys coming from a script.

// wldOpenWith opens a game from opts without sound, in a temporary save directory. A script is
// always present (so that the game answers to wldPlay), and shots are never taken.
func wldOpenWith(t *testing.T, opts Options) *Game {
	t.Helper()
	opts.NoSound = true
	if opts.SaveDir == "" {
		opts.SaveDir = t.TempDir()
	}
	if opts.Script == "" {
		opts.Script = "wait 1000000000\n"
	}
	if opts.ShotDir == "" {
		opts.ShotDir = t.TempDir()
	}
	if opts.Seed == 0 {
		opts.Seed = 7
	}
	g, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

// wldStartEvent runs a named event now, the way a place's arrival would (without Options.Event,
// which reads ahead to the first line on its own).
func wldStartEvent(t *testing.T, g *Game, name string) {
	t.Helper()
	ev, ok := g.db.Events[name]
	if !ok {
		t.Fatalf("no event %q", name)
	}
	g.events.Start(ev.Do)
}

// wldScreen is the image the game draws on.
func wldScreen(g *Game) *ebiten.Image {
	w, h := g.Layout(0, 0)
	if s := covScreen(); s.Bounds().Dx() == w && s.Bounds().Dy() == h {
		return s
	}
	return ebiten.NewImage(w, h)
}

// wldStep runs one frame: Update, then Draw. Screenshots are never taken (they read pixels, which
// needs a running game loop). It reports false when the script has quit.
func wldStep(t *testing.T, g *Game, screen *ebiten.Image) bool {
	t.Helper()
	err := g.Update()
	if s := g.in.script; s != nil {
		s.shot = ""
	}
	g.autoShot = "" // the screenshot of --until would read pixels
	if err != nil {
		if errors.Is(err, ebiten.Termination) || errors.Is(err, errQuit) {
			return false
		}
		t.Fatal(err)
	}
	// Only some frames are drawn (covDraws): drawing is most of what a frame costs (and several
	// times that under the race detector), and the rules do not wait for it. A test that reads
	// what a draw leaves behind runs a few frames first.
	if covDraws(g.tick) {
		g.Draw(screen)
	}
	return true
}

// wldFrames runs n frames.
func wldFrames(t *testing.T, g *Game, n int) {
	t.Helper()
	screen := wldScreen(g)
	for range n {
		if !wldStep(t, g, screen) {
			return
		}
	}
}

// wldFramesDrawn runs n frames and draws every one of them, for a test that reads what the
// drawing writes (the view and stand lines of the trace).
func wldFramesDrawn(t *testing.T, g *Game, n int) {
	t.Helper()
	screen := wldScreen(g)
	for range n {
		if !wldStep(t, g, screen) {
			return
		}
		if !covDraws(g.tick) {
			g.Draw(screen)
		}
	}
}

// wldPlay feeds a script to the running game and runs frames until it has been played out (at
// most limit frames), so the state can be read after the keys. The script must not shoot.
func wldPlay(t *testing.T, g *Game, src string, limit int) {
	t.Helper()
	sc, err := parseScript(src)
	if err != nil {
		t.Fatal(err)
	}
	s := g.in.script
	if s == nil {
		t.Fatal("the game has no script")
	}
	// a final long wait keeps the script alive (a script that ends quits the game)
	s.cmds = append(sc.cmds, scriptCmd{verb: "wait", arg: "1000000000", n: 1000000000})
	s.pc, s.wait, s.quit, s.limit = 0, 0, false, 0
	screen := wldScreen(g)
	for i := 0; (s.pc < len(s.cmds)-1 || s.wait > 0) && i < limit; i++ {
		if !wldStep(t, g, screen) {
			t.Fatal("the game quit while the script was played")
		}
	}
}

// wldUntil runs frames until cond holds (at most limit of them).
func wldUntil(t *testing.T, g *Game, limit int, cond func() bool) {
	t.Helper()
	screen := wldScreen(g)
	for range limit {
		if cond() {
			return
		}
		if !wldStep(t, g, screen) {
			t.Fatal("the game quit")
		}
	}
}

// wldHold holds the buttons (names as in a trace: "up", "left", ...) down for n frames. A script's
// press is only a first press, so keys held to walk a painted scene come from an input replay
// laid over the script for these frames.
func wldHold(t *testing.T, g *Game, n int, names ...string) {
	t.Helper()
	held := make([]any, len(names))
	for i, n := range names {
		held[i] = n
	}
	rep := &replay{inputs: map[int]traceLine{}}
	for i := 1; i <= n+1; i++ {
		rep.inputs[g.tick+i] = traceLine{"held": held}
	}
	script := g.in.script
	g.in.script, g.in.replay = nil, rep
	screen := wldScreen(g)
	for range n {
		if !wldStep(t, g, screen) {
			break
		}
	}
	g.in.script, g.in.replay = script, nil
}
