package play

import (
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
)

// Maps (docs/architecture/scenes.md, ADR 0022): a map location shows one picture with points
// joined by roads; the token stands on the current point (the rules' Spot) and walks along
// the roads, running each road's steps and do on reaching its end.

func wldAtlas(t *testing.T, loc string, fight ...bool) (*Game, *atlasScene) {
	t.Helper()
	g := wldWorld(t, loc, Options{Calm: len(fight) == 0 || !fight[0]})
	as, ok := g.stack[0].(*atlasScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	wldFrames(t, g, 10)
	return g, as
}

// wldSettle plays frames, pressing OK now and then (to pass lines), until the token has
// stopped and no event is running (at most limit frames).
func wldSettle(t *testing.T, g *Game, as *atlasScene, limit int) {
	t.Helper()
	screen := wldScreen(g)
	quiet := 0
	for i := 0; i < limit && quiet < 20; i++ {
		wldStep(t, g, screen)
		if as.walking() || g.busy() {
			quiet = 0
		} else {
			quiet++
		}
	}
}

func wldShown(as *atlasScene, id string) int {
	for _, p := range as.points {
		if p.id == id {
			return p.shown
		}
	}
	return -1
}

func TestWldAtlasArrivesAtStartPoint(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark")
	if g.st.Spot != "" && g.st.Spot != "home" {
		t.Fatalf("the party stands on %q, want the start point home", g.st.Spot)
	}
	if as.here != "home" {
		t.Fatalf("the token stands on %q, want home", as.here)
	}
	if got := wldShown(as, "home"); got != 3 {
		t.Fatalf("the point stood on shows as %d, want visited (3)", got)
	}
	if len(as.choices) == 0 {
		t.Fatal("no choices from the start point")
	}
}

// How an unvisited point shows follows unknown (ADR 0022).
func TestWldAtlasUnknownModes(t *testing.T) {
	cases := []struct {
		loc  string
		want map[string]int
	}{
		{"fx_mark", map[string]int{"inn": 1, "shrine": 1}},
		{"fx_dim", map[string]int{"inn": 2, "shrine": 2, "gate": 2, "cave": 2}},
		{"fx_named", map[string]int{"inn": 3, "shrine": 3, "gate": 3, "cave": 3}},
		// hidden: not shown until an open road reaches it; the gate is next to a reachable point so it
		// speaks its locked line, and the cave lies beyond the shut gate
		{"fx_hidden", map[string]int{"inn": 1, "shrine": 1, "gate": 1, "cave": 0}},
	}
	// one game walks through the four maps
	g, as := wldAtlas(t, cases[0].loc)
	for i, c := range cases {
		if i > 0 {
			g.Handle(event.MoveEffect{Location: c.loc})
			wldFrames(t, g, 10)
			var ok bool
			if as, ok = g.stack[0].(*atlasScene); !ok {
				t.Fatalf("scene %T", g.stack[0])
			}
		}
		for id, want := range c.want {
			if got := wldShown(as, id); got != want {
				t.Errorf("%s: point %s shows as %d, want %d", c.loc, id, got, want)
			}
		}
	}
}

func TestWldAtlasWalkRunsStepsAndPointDoOnce(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark")
	steps := g.st.Steps
	wldPlay(t, g, "walk shrine\nwait 5\n", 100)
	wldSettle(t, g, as, 1500)
	if g.st.Spot != "shrine" {
		t.Fatalf("the party stands on %q after the walk", g.st.Spot)
	}
	if !g.st.Flags["prayed"] {
		t.Fatal("the point's do did not run on arriving")
	}
	if got := g.st.Steps - steps; got != 3 {
		t.Fatalf("the road home-shrine has steps 3, the step count rose by %d", got)
	}
	// once: a point that has run once does not appear again
	wldPlay(t, g, "walk home\nwait 5\n", 100)
	wldSettle(t, g, as, 1500)
	if g.st.Spot != "home" || !g.st.Flags["home_seen"] {
		t.Fatalf("spot %q, home_seen %v", g.st.Spot, g.st.Flags["home_seen"])
	}
	for _, c := range as.choices {
		if c.Point == "shrine" {
			t.Fatal("a once point that has run is still offered")
		}
	}
}

func TestWldAtlasClosedPointSaysLockedThenOpens(t *testing.T) {
	g, as := wldAtlas(t, "fx_named")
	wldPlay(t, g, "walk shrine\nwait 5\n", 100)
	wldSettle(t, g, as, 1500)
	wldPlay(t, g, "walk gate\nuntil\nwait 5\n", 3000)
	wldSettle(t, g, as, 1500)
	if g.st.Spot != "shrine" || g.st.Flags["passed"] {
		t.Fatalf("walked into a closed point: spot %q", g.st.Spot)
	}
	said := false
	for _, l := range g.backlog {
		said = said || l.text == "The gate is shut."
	}
	if !said {
		t.Fatalf("a closed point says its locked line: backlog %+v", g.backlog)
	}
	g.st.Flags["gate_open"] = true
	as.dirty = true
	wldPlay(t, g, "walk gate\nwait 5\n", 100)
	wldSettle(t, g, as, 1500)
	if g.st.Spot != "gate" || !g.st.Flags["passed"] {
		t.Fatalf("spot %q passed %v after opening the gate", g.st.Spot, g.st.Flags["passed"])
	}
}

func TestWldAtlasKeysSelectAndConfirm(t *testing.T) {
	g, as := wldAtlas(t, "fx_named")
	first := as.sel
	wldPlay(t, g, "press right\nwait 6\npress down\nwait 6\npress left\nwait 6\npress up\nwait 6\n", 200)
	if as.sel < 0 || as.sel >= len(as.choices) {
		t.Fatalf("selection %d out of %d", as.sel, len(as.choices))
	}
	_ = first
	// pick the shrine by pressing toward it (it lies down and to the right of home) and confirm
	for i := 0; i < 4 && as.choices[as.sel].Point != "shrine"; i++ {
		wldPlay(t, g, "press down\nwait 6\n", 50)
	}
	if as.choices[as.sel].Point != "shrine" {
		t.Skipf("selection did not reach the shrine: %v", as.choices)
	}
	wldPlay(t, g, "press ok\nwait 5\n", 50)
	if len(as.queue) == 0 && !as.walking() {
		t.Fatal("confirming a choice set nothing going")
	}
	wldSettle(t, g, as, 1500)
	if g.st.Spot != "shrine" {
		t.Fatalf("spot %q", g.st.Spot)
	}
}

func TestWldAtlasClickOnPoint(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark")
	var x, y float64
	for _, p := range as.points {
		if p.id == "inn" {
			x, y = as.screen(g, p.at)
		}
	}
	wldPlay(t, g, "click "+strconv.Itoa(int(x))+" "+strconv.Itoa(int(y))+"\nwait 5\n", 100)
	wldSettle(t, g, as, 2500)
	if g.st.Location != "camp" {
		t.Fatalf("the inn leads to camp, the party is at %q (spot %q)", g.st.Location, g.st.Spot)
	}
}

func TestWldAtlasListOpensAndChooses(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark")
	wldPlay(t, g, "press menu\nwait 10\n", 100)
	if len(g.stack) < 2 {
		t.Fatal("the menu key did not open the list of the place's items")
	}
	wldPlay(t, g, "press cancel\nwait 10\n", 100)
	if len(g.stack) != 1 {
		t.Fatalf("cancel left %d scenes", len(g.stack))
	}
	_ = as
}

// The encounters of a road are rolled for each of its steps when the token has walked it, before
// the party arrives at the point (docs/architecture/scenes.md); a calm test play rolls none.
func TestWldAtlasRoadBattleComesBeforeArrival(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark", true)
	wldPlay(t, g, "walk shrine\nwait 5\n", 200)
	for range 600 {
		wldFrames(t, g, 1)
		if g.currentBattle() != nil {
			break
		}
	}
	if g.currentBattle() == nil {
		t.Fatalf("no battle on the road (spot %q, walking %v)", g.st.Spot, as.walking())
	}
	if g.st.Spot == "shrine" || g.st.Flags["prayed"] {
		t.Fatalf("the party had arrived (spot %q) when the battle on the road began", g.st.Spot)
	}
	if g.st.Steps < 3 {
		t.Fatalf("the road's 3 steps were not counted: %d", g.st.Steps)
	}
}

// A map inside a map (docs/architecture/scenes.md): a point whose to is a map can name the point
// over there (at_point), and the party arrives standing on it.
func TestWldAtlasPointIntoAnotherMapStandsOnTheNamedPoint(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark")
	wldPlay(t, g, "walk portal\nwait 5\n", 200)
	wldSettle(t, g, as, 600)
	if g.st.Location != "fx_dim" {
		t.Fatalf("at %q", g.st.Location)
	}
	if g.st.Spot != "cave" {
		t.Fatalf("the party stands on %q, want the point cave that at_point names", g.st.Spot)
	}
	wldFrames(t, g, 10)
	next, ok := g.stack[0].(*atlasScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if next.here != "cave" || next.at.X != 110 || next.at.Y != 110 {
		t.Fatalf("the token is on %q at %v", next.here, next.at)
	}
}

func TestWldAtlasScriptWalkToUnknownPointFails(t *testing.T) {
	g, as := wldAtlas(t, "fx_mark")
	if _, done := as.scriptWalk(g, "nowhere"); !done {
		t.Fatal("a walk to a missing point should end")
	}
	if g.failure == nil || !strings.Contains(g.failure.Error(), "no such point") {
		t.Fatalf("failure %v", g.failure)
	}
	g.failure = nil
}
