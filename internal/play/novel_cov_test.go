package play

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// The novel window (ui.message: novel) and its stage (ADR 0028, ADR 0026): portraits stand
// left and right by themselves from the speakers of say, the band has buttons for the log, auto,
// skip and hiding the window, a long line is paged, and a choice takes the keys.

const wldNovelData = `events:
  ev_auto:
    do:
      - say: {who: Courier, face: pics/tall1.png, text: "One."}
      - say: {who: Guard, face: pics/tall2.png, text: "Two."}
      - say: {who: Courier, face: pics/tall1.png, text: "Three."}
      - say: {who: Stranger, face: pics/tall3.png, text: "Four."}
      - say: "Five."
  ev_notice:
    do:
      - notice: "Found a coin."
      - notice: "The lamp flickers."
      - say: "After the notices."
  ev_dream:
    do:
      - image: {file: pics/bg.png, look: dream, haze: "#aabbcc"}
      - say: "A dream."
  ev_box:
    do:
      - say: {who: Courier, face: pics/tall1.png, side: left, text: "Left one."}
      - say: {who: Guard, face: pics/tall2.png, side: right, text: "Right one."}
      - say: {who: Courier, face: pics/tall1.png, side: left, text: "Left again."}
      - say: {who: Short, face: x.png, text: "Small portrait."}
      - say: {who: scout, text: "Party default."}
      - say: {text: "No name."}
  ev_long:
    do:
      - say: "Aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz. Aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz. Aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz. Aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz. Aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp qqqq rrrr ssss tttt uuuu vvvv wwww xxxx yyyy zzzz."
      - say: "After."
  ev_choice:
    do:
      - say: "Before the choice."
      - choice:
          - {label: Left, do: [{set: {flag.went_left: true}}]}
          - {label: Right, do: [{set: {flag.went_right: true}}]}
        prompt: "Which way?"
        id: way
      - say: "After the choice."
  ev_bare:
    do:
      - choice:
          - {label: Yes, do: [{set: {flag.yes: true}}]}
          - {label: No, do: []}
  ev_cast:
    do:
      - stand: {who: Courier, face: pics/tall1.png, at: left, enter: slide}
      - say: {who: Courier, face: pics/tall1.png, text: "Hi."}
      - stand: {who: Guard, face: pics/tall2.png, at: 0.7}
      - stand: {who: Courier, at: center}
      - say: {who: Guard, face: pics/tall2.png, text: "Move over."}
      - effect: flash
      - effect: {name: shake, seconds: 0.3}
      - stand: {who: Guard, leave: slide}
      - stand: {who: Courier, leave: fade}
      - say: "Alone."
      - stand: none
      - say: "Nobody."
`

// wldNovel opens the neutral game with the novel window at camp, starting the event.
func wldNovel(t *testing.T, event string) *Game {
	t.Helper()
	opts := Options{Location: "camp"}
	opts.FS = wldWorldFiles(t, "ui: {message: novel}\n", map[string]string{"data/novel_cov.yaml": wldNovelData})
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	wldStartEvent(t, g, event)
	wldFrames(t, g, 30)
	return g
}

func wldTalkText(g *Game) string {
	if !g.talk.active || g.talk.page >= len(g.talk.pages) {
		return ""
	}
	return strings.Join(g.talk.pages[g.talk.page], "")
}

// wldAdvance presses OK once the line is fully shown and returns the next line's text.
func wldAdvance(t *testing.T, g *Game) string {
	t.Helper()
	wldFrames(t, g, 80)
	wldPlay(t, g, "press ok\nwait 40\n", 200)
	return wldTalkText(g)
}

func TestWldNovelSpeakersTakeOppositeSides(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	if got := wldTalkText(g); got != "One." {
		t.Fatalf("first line %q", got)
	}
	if got := wldAdvance(t, g); got != "Two." {
		t.Fatalf("second line %q", got)
	}
	a, aok := g.sides["name:Courier"]
	b, bok := g.sides["name:Guard"]
	if !aok || !bok || (a < 0.5) == (b < 0.5) {
		t.Fatalf("the two speakers share a side: Courier %v, Guard %v", g.sides["name:Courier"], g.sides["name:Guard"])
	}
	// the first speaker speaks again: they keep their side
	wldAdvance(t, g)
	if g.sides["name:Courier"] != a {
		t.Fatalf("Courier moved from %v to %v", a, g.sides["name:Courier"])
	}
	// a third speaker takes the place of the one who spoke longest ago
	if got := wldAdvance(t, g); got != "Four." {
		t.Fatalf("fourth line %q", got)
	}
	if g.stage[1].id != "name:Stranger" {
		t.Fatalf("the stage's newest speaker is %q", g.stage[1].id)
	}
	if got := wldAdvance(t, g); got != "Five." {
		t.Fatalf("last line %q", got)
	}
}

func TestWldNovelBandButtons(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	wldAdvance(t, g)
	wldFrames(t, g, 80)
	// log: opens the backlog of the lines said so far; cancel closes it
	wldPlay(t, g, "press log\nwait 6\n", 50)
	if g.logView == nil {
		t.Fatal("the log key did not open the backlog")
	}
	if len(g.backlog) != 2 {
		t.Fatalf("backlog has %d lines, want the 2 said", len(g.backlog))
	}
	wldPlay(t, g, "press cancel\nwait 6\n", 50)
	if g.logView != nil {
		t.Fatal("cancel did not close the backlog")
	}
	// hide: the window goes away; any key brings it back
	wldPlay(t, g, "press hide\nwait 6\n", 50)
	if !g.talk.hidden {
		t.Fatal("hide did not hide the window")
	}
	wldPlay(t, g, "press ok\nwait 6\n", 50)
	if g.talk.hidden {
		t.Fatal("OK did not bring the window back")
	}
	// auto: toggles, and a line read is passed by itself after a while
	wldPlay(t, g, "press auto\nwait 6\n", 50)
	if !g.autoRead {
		t.Fatal("the auto key did not turn auto on")
	}
	before := wldTalkText(g)
	for i := 0; i < 900 && wldTalkText(g) == before; i++ {
		wldFrames(t, g, 1)
	}
	if got := wldTalkText(g); got == before {
		t.Fatalf("auto never passed %q", before)
	}
	wldPlay(t, g, "press auto\nwait 6\n", 50)
	if g.autoRead {
		t.Fatal("the auto key did not turn auto off")
	}
	// the config screen opens from the band
	wldPlay(t, g, "press config\nwait 6\n", 50)
	if _, ok := g.stack[len(g.stack)-1].(*configScreen); !ok {
		t.Fatalf("config key opened %T", g.stack[len(g.stack)-1])
	}
}

func TestWldNovelClickOnBandButtons(t *testing.T) {
	g := wldNovel(t, "ev_auto")
	wldFrames(t, g, 80)
	rects := g.novelButtonRects()
	buttons := g.novelButtons()
	for i, b := range buttons {
		if b.b != btnAuto {
			continue
		}
		r := rects[i]
		x, y := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
		wldPlay(t, g, "click "+strconv.Itoa(x)+" "+strconv.Itoa(y)+"\nwait 6\n", 50)
		if !g.autoRead {
			t.Fatal("clicking the auto button did not turn auto on")
		}
		return
	}
	t.Fatal("the band has no auto button")
}

func TestWldNovelLongLineIsPaged(t *testing.T) {
	g := wldNovel(t, "ev_long")
	if len(g.talk.pages) < 2 {
		t.Fatalf("a very long line fits one page: %d", len(g.talk.pages))
	}
	first := wldTalkText(g)
	wldFrames(t, g, 200)
	wldPlay(t, g, "press ok\nwait 20\n", 100)
	if g.talk.page != 1 || wldTalkText(g) == first {
		t.Fatalf("OK did not turn to the second page (page %d)", g.talk.page)
	}
	for i := 0; i < len(g.talk.pages)+2 && wldTalkText(g) != "After."; i++ {
		wldFrames(t, g, 200)
		wldPlay(t, g, "press ok\nwait 20\n", 100)
	}
	if wldTalkText(g) != "After." {
		t.Fatalf("the pages did not lead to the next line: %q", wldTalkText(g))
	}
}

func TestWldNovelChoiceTakesBranchAndMarksKey(t *testing.T) {
	g := wldNovel(t, "ev_choice")
	for i := 0; i < 5 && g.talk.choice == nil; i++ {
		wldAdvance(t, g)
	}
	if g.talk.choice == nil {
		t.Fatalf("no choice reached (text %q)", wldTalkText(g))
	}
	if n := len(g.talk.choice.items); n != 2 {
		t.Fatalf("%d options", n)
	}
	wldPlay(t, g, "press down\nwait 8\npress ok\nwait 40\n", 200)
	if !g.st.Flags["went_right"] || g.st.Flags["went_left"] {
		t.Fatalf("flags left %v right %v: the second option was chosen", g.st.Flags["went_left"], g.st.Flags["went_right"])
	}
	if g.st.Chose["way"] != "1" {
		t.Fatalf("chose.way is %q, want the number of the option (1, from 0)", g.st.Chose["way"])
	}
	if got := wldTalkText(g); got != "After the choice." {
		t.Fatalf("line after the choice: %q", got)
	}
}

func TestWldNovelBareChoice(t *testing.T) {
	g := wldNovel(t, "ev_bare")
	if g.talk.choice == nil || !g.talk.bare {
		t.Fatal("a choice without a prompt or a line before it should be a bare choice")
	}
	wldPlay(t, g, "press ok\nwait 30\n", 100)
	if !g.st.Flags["yes"] {
		t.Fatal("the first option was not taken")
	}
}

func TestWldNovelCastStaging(t *testing.T) {
	g := wldNovel(t, "ev_cast")
	if !g.castOn {
		t.Fatal("stand did not switch the event to placing portraits as written")
	}
	wldFrames(t, g, 40)
	sw, _ := g.screen()
	figs := g.castFigures("name:Courier")
	if len(figs) != 1 || figs[0].x != sw*0.25 {
		t.Fatalf("figures %+v, want Courier at the left (a quarter of the width)", figs)
	}
	wldAdvance(t, g) // Guard stands at 0.7, Courier moves to the center
	figs = g.castFigures("name:Guard")
	if len(figs) != 2 {
		t.Fatalf("figures %+v", figs)
	}
	var courier, guard stageFigure
	for _, f := range figs {
		if f.id == "name:Courier" {
			courier = f
		} else {
			guard = f
		}
	}
	if courier.x != sw*0.5 || guard.x != sw*0.7 || guard.dim != 0 || courier.dim == 0 {
		t.Fatalf("courier %+v guard %+v: the speaker should be lit and the other dimmed", courier, guard)
	}
	// flash and a timed shake of the whole screen run, then both end
	wldPlay(t, g, "press ok\nwait 5\n", 50)
	wldFrames(t, g, 4)
	if g.flash == 0 && g.shakeFor == 0 && g.shake == 0 {
		t.Fatalf("no flash or shake started: flash %d shakeFor %d shake %d", g.flash, g.shakeFor, g.shake)
	}
	wldFrames(t, g, 200)
	if g.flash != 0 || g.shakeFor != 0 {
		t.Fatalf("flash %d and shake %d never ended", g.flash, g.shakeFor)
	}
	// both leave; stand: none clears the rest
	for wldTalkText(g) != "Alone." {
		wldAdvance(t, g)
		if g.st == nil || !g.talk.active {
			break
		}
	}
	wldFrames(t, g, 40)
	if figs := g.castFigures(""); len(figs) != 0 {
		t.Fatalf("figures after both left: %+v", figs)
	}
}

func TestWldNovelVisualEffectsAreEvents(t *testing.T) {
	// the rules see the staging only as effects (ADR 0028)
	g := wldNovel(t, "ev_cast")
	g.visual(event.VisualEffect{Name: "flash", Cue: data.EffectCue{Name: "flash"}})
	if g.flash != 20 {
		t.Fatalf("flash lasts %d ticks, want the default 20", g.flash)
	}
}
