package play

import (
	"strings"
	"testing"
)

const turnBoard = `planning:
  calendar:
    start: '2030-01-01'
    end: '2030-01-09'
    phases: [morning, night]
    looks:
      night: {label: Night, time: '22:00', color: '#203050', wash: 0.3, note: 'Home at {planning.date}'}
  entities: {potter: {name: Potter}}
  activities:
    glaze: {name: Bowl glazing, effort: 2}
screens:
  studio:
    title: Studio
    kind: workspace
    view: {fullscreen: true}
    entries:
    - {label: Count, style: button, bounds: [32, 300, 140, 38], do: [{set: {var.counted: 'var.counted+1'}}]}
`

// Moving into a phase with a look shows its turn card, which holds the game
// for a moment and then leaves; the first phase after a start shows none,
// and a phase without a look shows none either.
func TestPhaseTurnCard(t *testing.T) {
	fsys := wldWorldFiles(t, turnBoard, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.openScreen("studio", func() {})
	wldFrames(t, g, 4)
	if g.turn != nil {
		t.Fatal("a card showed for the first phase")
	}
	g.st.Planning.Phase = 1
	wldFrames(t, g, 2)
	if g.turn == nil || g.turn.note != "Home at 2030-01-01" || !g.busy() {
		t.Fatalf("no card for the night: %+v", g.turn)
	}
	if clock, _, lit := g.phaseClock(); clock != "22:00" || !lit {
		t.Fatal("the header has no clock", clock)
	}
	wldFrames(t, g, turnIn+turnHold+turnOut+2)
	if g.turn != nil {
		t.Fatal("the card did not leave")
	}
	g.st.Planning.Date, g.st.Planning.Phase = "2030-01-02", 0
	wldFrames(t, g, 2)
	if g.turn != nil {
		t.Fatal("a phase without a look showed a card")
	}
}

func TestPhaseTurnRetainsScriptPick(t *testing.T) {
	fsys := wldWorldFiles(t, turnBoard, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Script: "wait 1000", ShotDir: t.TempDir()})
	wldFrames(t, g, 30)
	g.openScreen("studio", func() {})
	wldFrames(t, g, 4)
	g.st.Planning.Phase = 1
	wldFrames(t, g, 2)
	if g.turn == nil {
		t.Fatal("date card did not open")
	}
	s := g.in.script
	s.cmds, s.pc, s.wait = []scriptCmd{{verb: "pick", name: "Count"}, {verb: "wait", n: 1000}}, 0, 0
	wldFrames(t, g, 1)
	if s.pc != 0 {
		t.Fatal("pick consumed while date card was showing")
	}
	wldFrames(t, g, 100)
	if g.st.Vars["counted"] != float64(1) {
		t.Fatalf("counted=%v, want one accepted pick", g.st.Vars["counted"])
	}
}

func TestPhaseTurnObservationsAfterPick(t *testing.T) {
	for _, verb := range []string{"look", "snapshot"} {
		t.Run(verb, func(t *testing.T) {
			fsys := wldWorldFiles(t, turnBoard, nil)
			fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
			g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Script: "wait 1000", ShotDir: t.TempDir()})
			wldFrames(t, g, 30)
			g.openScreen("studio", func() {})
			wldFrames(t, g, 4)
			s := g.in.script
			s.cmds, s.pc, s.wait = []scriptCmd{{verb: "pick", name: "Count"}, {verb: verb}, {verb: "wait", n: 1000}}, 0, 0
			wldFrames(t, g, 3)
			if g.st.Vars["counted"] != float64(1) {
				t.Fatal("pick was not accepted")
			}
			g.st.Planning.Phase = 1
			g.fadeT = 20
			screen, ok := g.stack[len(g.stack)-1].(*authorScreen)
			if !ok {
				t.Fatal("studio screen was not open")
			}
			screen.running = true
			observed := false
			observe := func() {
				observed = true
				kind, _ := g.scriptPickables()
				if kind != "screen:studio" || g.turn != nil || g.fadeT > 0 {
					t.Fatalf("observation during transition: %s", kind)
				}
			}
			s.look, s.snapshot = observe, observe
			wldFrames(t, g, 12)
			if observed || s.pc != 1 {
				t.Fatal("observation consumed during transition")
			}
			for range 200 {
				screen.running = true
				wldFrames(t, g, 1)
			}
			if observed {
				t.Fatal("observation consumed while screen event was running")
			}
			screen.running = false
			wldFrames(t, g, 2)
			if !observed {
				t.Fatal("ready screen was never observed")
			}
		})
	}
}
