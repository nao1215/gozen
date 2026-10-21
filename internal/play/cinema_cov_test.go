package play

import (
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func sysQuit(err error) bool { return errors.Is(err, ebiten.Termination) }

const sysCinema = `events:
  cinema:
    do:
      - image: {file: x.png, fade: 0.2}
      - say: "A"
      - effect: shake
      - image: {file: x.png, fit: cover}
      - say: "B"
      - effect: flash
      - mood: tense
      - sfx: door
      - notice: "Careful."
      - image: ""
      - say: "C"
      - effect: fade
      - wait: {seconds: 0.2}
      - move: room
      - say: "D"
`

// sysSample is the screen as seen when a line opened.
type sysSample struct {
	cg, old, under string
	oldT           int
	shake, flash   int
	fade           int
	mood           string
	loc            string
	top            string
}

// sysPlayLines reads every line of a script-driven game, sampling the screen at each line.
func sysPlayLines(t *testing.T, g *Game, max int) map[string]sysSample {
	t.Helper()
	out := map[string]sysSample{}
	for range max {
		err := g.Update()
		if err != nil {
			if !sysQuit(err) {
				t.Fatal(err)
			}
			return out
		}
		sysSafeDraw(g)
		if what, age := g.waiting(); what == "text" && age == 0 && g.talk.active {
			s := sysSample{cg: g.cg.File, old: g.cgOld.File, under: g.cgUnder.File, oldT: g.cgOldT,
				shake: g.shake, flash: g.flash, fade: g.fadeT, mood: g.moodOverride, loc: g.st.Location}
			out[g.talk.say.Text] = s
		}
	}
	t.Fatal("script did not end")
	return nil
}

// TestCinemaEventDrivesTheScreen: an event's image, effect, mood and move commands reach the
// screen state, and the lines come after them.
func TestCinemaEventDrivesTheScreen(t *testing.T) {
	g := sysOpen4(t, Options{Seed: 1, Location: "camp", Event: "cinema", Line: 1,
		Script: "read 3\nread 3\nread 3\nread 3\nread 3\nquit"}, map[string]string{"data/cinema.yaml": sysCinema})
	got := sysPlayLines(t, g, 2000)
	if got["A"].cg != "x.png" {
		t.Errorf("line A: picture %q", got["A"].cg)
	}
	if b := got["B"]; b.cg != "x.png" || b.old != "x.png" {
		t.Errorf("line B should show the cover picture over the old one: %+v", b)
	}
	if c := got["C"]; c.cg != "" {
		t.Errorf("line C: the picture should be taken away, still %q", c.cg)
	}
	if got["C"].mood != "tense" {
		t.Errorf("mood override %q after the mood command", got["C"].mood)
	}
	if d, ok := got["D"]; !ok || d.loc != "room" {
		t.Errorf("line D at %+v", d)
	}
	if _, ok := g.stack[0].(*dungeonScene); !ok {
		t.Errorf("after the move the scene is %T", g.stack[0])
	}
	if g.st.Steps == 0 {
		t.Error("moving did not count a step")
	}
	if g.pics.imgs["x.png"] == nil {
		t.Error("the shown picture was not loaded")
	}
}
