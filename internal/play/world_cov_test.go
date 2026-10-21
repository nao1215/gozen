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
func wldScreen(g *Game) *ebiten.Image { return ebiten.NewImage(g.Layout(0, 0)) }

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
	g.Draw(screen)
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

// wldPlay feeds a script to the running game and runs frames until it has been played out (at
// most max frames), so the state can be read after the keys. The script must not shoot.
func wldPlay(t *testing.T, g *Game, src string, max int) {
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
	for i := 0; (s.pc < len(s.cmds)-1 || s.wait > 0) && i < max; i++ {
		if !wldStep(t, g, screen) {
			t.Fatal("the game quit while the script was played")
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
