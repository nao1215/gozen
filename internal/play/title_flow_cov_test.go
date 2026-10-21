package play

import (
	"testing"
)

const sysEndings = `events:
  finish:
    do:
      - say: "Fin."
      - clear: true
  recover:
    do:
      - say: "You wake at camp."
  gallery_look:
    do:
      - image: {file: x.png}
      - say: "A picture."
`

func sysUntilStack(t *testing.T, g *Game, limit int, ok func() bool) {
	t.Helper()
	for range limit {
		if ok() {
			return
		}
		sysFrames(t, g, 1)
	}
	t.Fatal("never reached the expected screen")
}

// TestClearEventLeadsToTheEndingThenTheTitle: the clear command ends the playthrough with the
// ending screen; after it has been shown a while any confirmation goes back to a fresh title.
func TestClearEventLeadsToTheEndingThenTheTitle(t *testing.T) {
	g := sysOpen(t, Options{Seed: 1, Location: "camp", Event: "finish", Line: 1}, map[string]string{"data/end.yaml": sysEndings})
	// read on: the line then the clear
	sysUntilStack(t, g, 400, func() bool { return g.talk.active })
	for range 400 {
		if _, ok := g.stack[0].(*ending); ok {
			break
		}
		sysFramesWithKey(t, g, btnOK)
	}
	e := sysAs[*ending](t, g.stack[0])
	// pressing early does nothing.
	sysKey(g, btnOK)
	e.update(g)
	if _, ok := g.stack[0].(*ending); !ok {
		t.Fatal("the ending closed before it was shown")
	}
	e.age = 200
	sysKey(g, btnOK)
	e.update(g)
	if _, ok := g.stack[0].(*title); !ok || g.st != nil {
		t.Fatalf("after the ending: %T, state %v", g.stack[0], g.st)
	}
	sysFrames(t, g, 3)
}

// sysFramesWithKey runs one frame whose input is the given button; it works by replacing the
// input of the frame with a one-command script.
func sysFramesWithKey(t *testing.T, g *Game, b button) {
	t.Helper()
	prev := g.in.script
	name := "ok"
	for n, bb := range scriptButtons {
		if bb == b && n != "ok" && b != btnOK {
			name = n
		}
	}
	sc, err := parseScript("press " + name + "\nquit")
	if err != nil {
		t.Fatal(err)
	}
	g.in.script = sc
	sysFrames(t, g, 1)
	g.in.script = prev
}

// TestGameOverReturnsToTheTitle: without a game_over event the title comes back with no state.
func TestGameOverReturnsToTheTitle(t *testing.T) {
	g := sysOpen(t, Options{Seed: 1, Location: "camp"}, nil)
	g.replace(newGameOver(g))
	s := sysAs[*gameOver](t, g.stack[0])
	sysFrames(t, g, 3)
	sysKey(g, btnOK)
	s.update(g)
	if _, ok := g.stack[0].(*gameOver); !ok {
		t.Fatal("left the game over screen before it had been shown")
	}
	s.age = 100
	sysKey(g, btnOK)
	s.update(g)
	if _, ok := g.stack[0].(*title); !ok || g.st != nil {
		t.Fatalf("after the game over: %T state %v", g.stack[0], g.st)
	}
	sysFrames(t, g, 3)
}

// TestGameOverEventHealsAndRuns: with game_over.event the party is healed and the event runs.
func TestGameOverEventHealsAndRuns(t *testing.T) {
	g2 := sysOpen4(t, Options{Seed: 1, Location: "camp"}, map[string]string{
		"data/end.yaml": sysEndings,
		"game.yaml+":    "game_over: {event: recover, text: Down}\n",
	})
	a := g2.st.Members()[0]
	a.HP = 1
	g2.replace(newGameOver(g2))
	s2 := sysAs[*gameOver](t, g2.stack[0])
	sysFrames(t, g2, 3)
	s2.age = 100
	sysKey(g2, btnCancel)
	s2.update(g2)
	if a.HP != a.Stat("mhp") {
		t.Fatalf("the fallen were not healed: %d/%d", a.HP, a.Stat("mhp"))
	}
	if !g2.events.Running() {
		t.Fatal("the game over event did not start")
	}
	if _, ok := g2.stack[0].(*field); !ok {
		t.Fatalf("scene %T, want the place the party fell in", g2.stack[0])
	}
}
