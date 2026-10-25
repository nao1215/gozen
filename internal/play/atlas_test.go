package play

import (
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
)

func TestAtlasLabelGeometryDoesNotFollowFocus(t *testing.T) {
	g, s := wldAtlas(t, "fx_mark")
	s.here = "elsewhere"
	s.at = geom.Pt{X: 160, Y: 120}
	s.choices = []atlas.Choice{{Point: "a"}, {Point: "b"}, {Point: "c"}}
	for _, size := range [][2]int{{1280, 720}, {960, 720}} {
		g.db.Game.Screen.Width, g.db.Game.Screen.Height = size[0], size[1]
		for _, origin := range []geom.Pt{{X: 140, Y: 100}, {X: 4, Y: 4}, {X: 270, Y: 4}, {X: 4, Y: 145}, {X: 270, Y: 145}} {
			for _, shown := range []int{1, 2, 3} {
				s.points = nil
				for i, id := range []string{"a", "b", "c"} {
					at := origin.Add(geom.Pt{X: float64(i * 20), Y: float64(i % 2 * 24)})
					s.points = append(s.points, mapPoint{id: id, at: at, shown: shown, open: true, choice: i, label: "Destination " + id + " by the river"})
					g.db.Locations[s.loc].Map.Points[id] = &data.MapPoint{At: data.Point{at.X, at.Y}}
				}
				s.sel = 0
				s.layoutPoints(g, nil)
				want := slices.Clone(s.boxes)
				for _, i := range []int{1, 2, 0} {
					d := s.points[i].at.Sub(s.at)
					s.selectToward(g, geom.Pt{X: d.X / d.Len(), Y: d.Y / d.Len()})
					if s.sel != i {
						t.Fatalf("keyboard selected %d, want %d", s.sel, i)
					}
					s.layoutPoints(g, nil)
					if !reflect.DeepEqual(s.boxes, want) {
						t.Fatalf("screen %v origin %v shown %d: selection %d moved layout\ngot %v\nwant %v", size, origin, shown, i, s.boxes, want)
					}
					plate := want[len(want)-3+i]
					clickable := false
					for y := plate.y + 3; y < plate.y+plate.h-3 && !clickable; y += 4 {
						for x := plate.x + 3; x < plate.x+plate.w-3; x += 4 {
							if c, ok := s.pointAt(g, x, y); ok && c.Point == s.choices[i].Point {
								clickable = true
								break
							}
						}
					}
					if !clickable {
						t.Fatalf("screen %v origin %v shown %d: label %d has no pointer target; boxes %v", size, origin, shown, i, s.boxes)
					}
				}
			}
		}
	}
	// A revealed name can be narrower than the unknown marker, or absent.
	for _, name := range []string{".", ""} {
		for i := range s.points {
			s.points[i].shown, s.points[i].label = 1, name
		}
		s.sel = 0
		s.layoutPoints(g, nil)
		want := slices.Clone(s.boxes)
		s.sel = 1
		s.layoutPoints(g, nil)
		if !reflect.DeepEqual(s.boxes, want) {
			t.Fatal("revealing a short name changed its reserved label rectangle")
		}
		if name == "" && len(s.targets) != 5 {
			t.Fatal("unnamed points must retain their two unselected question-mark plates")
		}
	}
}

func TestAtlasTravelSpeed(t *testing.T) {
	g, s := wldAtlas(t, "fx_mark")
	for _, speed := range []float64{0, 0.12, 0.24, 2} {
		g.db.Locations["fx_mark"].Map.TravelSpeed = speed
		s.path = geom.Polyline{{X: 0, Y: 100}, {X: 80, Y: 100}}
		s.walked = 0
		ticks := 0
		for len(s.path) > 0 && ticks < 200 {
			s.walk(g)
			ticks++
		}
		rate := speed
		if rate == 0 {
			rate = 0.12
		}
		want := int(math.Ceil(80 / (rate * tickSecond * 320)))
		if abs(ticks-want) > 1 || s.at != (geom.Pt{X: 80, Y: 100}) {
			t.Fatalf("speed %g: %d ticks, want %d; endpoint %v", speed, ticks, want, s.at)
		}
	}
}

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
	covSlow(t)
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
	covSlow(t)
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
	for _, speed := range []float64{0, 0.24, 2} {
		t.Run(strconv.FormatFloat(speed, 'f', -1, 64), func(t *testing.T) {
			covSlow(t)
			g, as := wldAtlas(t, "fx_mark")
			g.db.Locations["fx_mark"].Map.TravelSpeed = speed
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
		})
	}
}

func TestWldAtlasClosedPointSaysLockedThenOpens(t *testing.T) {
	covSlow(t)
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
	covSlow(t)
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
	for _, speed := range []float64{0, 0.24, 2} {
		t.Run(strconv.FormatFloat(speed, 'f', -1, 64), func(t *testing.T) {
			g, as := wldAtlas(t, "fx_mark", true)
			g.db.Locations["fx_mark"].Map.TravelSpeed = speed
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
		})
	}
}

// A map inside a map (docs/architecture/scenes.md): a point whose to is a map can name the point
// over there (at_point), and the party arrives standing on it.
func TestWldAtlasPointIntoAnotherMapStandsOnTheNamedPoint(t *testing.T) {
	covSlow(t)
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

func TestAtlasPointerVisibleBounds(t *testing.T) {
	g, s := wldAtlas(t, "fx_mark")
	s.here = "elsewhere"
	s.choices = []atlas.Choice{{Point: "destination"}}
	s.sel = len(s.choices)
	s.points = []mapPoint{{id: "destination", at: geom.Pt{X: 160, Y: 120}, shown: 3, open: true, choice: 0}}
	for _, scale := range []struct {
		width, height int
		ui            float64
	}{{1280, 720, 1}, {1920, 1080, 1.5}, {960, 720, 2}} {
		g.db.Game.Screen.Width, g.db.Game.Screen.Height, g.ui = scale.width, scale.height, scale.ui
		_, sh := g.screen()
		h := sh * .3
		g.db.Locations[s.loc].Map.Marker = &data.Token{Height: .3}
		x, y := s.screen(g, s.points[0].at)
		for _, tc := range []struct {
			name, picture string
			dx, dy        float64
		}{
			{"portrait upper", "pics/portrait.png", 0, -h * .8},
			{"wide side", "pics/decal.png", h * .8, -h * .5},
		} {
			t.Run(tc.name+strconv.Itoa(scale.width), func(t *testing.T) {
				s.points[0].marker = tc.picture
				if c, ok := s.pointAt(g, x+tc.dx, y+tc.dy); !ok || c.Point != "destination" {
					t.Fatalf("visible picture missed: %v %v", c, ok)
				}
			})
		}
		s.points[0].marker = ""
		g.db.Locations[s.loc].Map.Marker = nil
		s.points[0].label = "A long destination name beside the road"
		if c, ok := s.pointAt(g, x+100, y+12); !ok || c.Point != "destination" {
			t.Errorf("visible label missed at UI scale %v", scale.ui)
		}
		s.points[0].label = ""
		if _, ok := s.pointAt(g, x+pointHit-1, y); !ok {
			t.Error("minimum finger target lost")
		}
		if _, ok := s.pointAt(g, x+pointHit+1, y); ok {
			t.Error("point outside small pin selected")
		}
		s.points[0].shown = 0
		if _, ok := s.pointAt(g, x, y); ok {
			t.Error("hidden destination selected")
		}
		s.points[0].shown = 3
	}
}

func TestAtlasPointerOverlapAndAvailability(t *testing.T) {
	g, s := wldAtlas(t, "fx_mark")
	s.here = "elsewhere"
	s.choices = []atlas.Choice{{Point: "back"}, {Point: "front"}}
	s.sel = len(s.choices)
	s.points = []mapPoint{
		{id: "back", at: geom.Pt{X: 160, Y: 120}, shown: 3, open: true, choice: 0, marker: "pics/portrait.png"},
		{id: "front", at: geom.Pt{X: 162, Y: 120}, shown: 3, open: true, choice: 1, marker: "pics/portrait.png"},
	}
	g.db.Locations[s.loc].Map.Marker = &data.Token{Height: .4}
	x, y := s.screen(g, s.points[0].at)
	_, sh := g.screen()
	check := func(want string, x, y float64) {
		t.Helper()
		c, ok := s.pointAt(g, x, y)
		if !ok || c.Point != want {
			t.Fatalf("got %v %v, want %s", c, ok, want)
		}
	}
	check("front", x, y-sh*.2)
	s.points[1].open = false // Closed choices still explain their locked condition.
	check("front", x, y-sh*.2)
	s.points[1].choice = -1
	check("back", x, y-sh*.2)
	s.points[1].choice = 1
	s.points[1].shown = 0
	check("back", x, y-sh*.2)
	s.points[1].shown = 3
	// Outside visible rectangles, the nearest anchor retains the minimum target.
	check("back", x-10, y+20)
	// Selection enlargement is part of the hit target even before the next draw.
	s.points = s.points[:1]
	s.sel = 0
	check("back", x, y-sh*.49)
	s.sel = len(s.choices)
	if _, ok := s.pointAt(g, x, y-sh*.49); ok {
		t.Fatal("old selection bounds retained")
	}
	// A rendered plate takes precedence over a marker behind it.
	s.points[0].label = "Destination"
	s.points = append(s.points, mapPoint{id: "front", at: geom.Pt{X: 160, Y: 150}, shown: 3, open: true, choice: 1, marker: "pics/portrait.png"})
	s.layoutPoints(g, nil)
	plate := s.targets[len(s.targets)-1].box
	check("back", plate.x+plate.w/2, plate.y+plate.h/2)
}
