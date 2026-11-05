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
