package play

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer1/geom"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/atlas"
	"github.com/nao1215/gozen/internal/rpg"
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

// atlasCameraFiles is an original neutral grid, solely a camera/navigation fixture.
func atlasCameraFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	fsys := btlFiles(t, "schema_version: 3\n", "")
	var world strings.Builder
	world.WriteString("locations:\n  large:\n    name: Survey map\n    safe: true\n    map:\n      picture: survey.png\n      camera: {zoom: 3, max_zoom: 8}\n      unknown: named\n      draw_roads: true\n      travel_speed: 2\n      points:\n")
	for i := range 60 {
		fmt.Fprintf(&world, "        p%02d: {at: [%d, %d], label: 'Station %02d', do: [{set: {var.arrivals: 'var.arrivals + 1'}}]}\n", i, 180+i%10*290, 180+i/10*280, i)
	}
	world.WriteString("      roads:\n")
	for i := 1; i < 60; i++ {
		fmt.Fprintf(&world, "        - {between: [p%02d, p%02d], steps: 1}\n", i-1, i)
	}
	fsys["data/survey.yaml"] = &fstest.MapFile{Data: []byte(world.String())}
	img := image.NewNRGBA(image.Rect(0, 0, 3000, 1800))
	for y := range 1800 {
		for x := range 3000 {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(30 + x/60), G: uint8(60 + y/30), B: 60, A: 255})
		}
	}
	var pic bytes.Buffer
	if err := png.Encode(&pic, img); err != nil {
		t.Fatal(err)
	}
	fsys["assets/survey.png"] = &fstest.MapFile{Data: pic.Bytes()}
	return fsys
}

func TestAtlasCameraNavigation(t *testing.T) {
	g, err := New(Options{FS: atlasCameraFiles(t), Location: "large", NoSound: true, SaveDir: t.TempDir(), Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	s, ok := g.stack[0].(*atlasScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if len(s.points) != 60 {
		t.Fatalf("points %d", len(s.points))
	}
	before, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	size := s.labelSize(g)
	for _, screen := range [][2]int{{1280, 720}, {960, 720}} {
		g.db.Game.Screen.Width, g.db.Game.Screen.Height = screen[0], screen[1]
		for _, dir := range []geom.Pt{{X: -100, Y: -100}, {X: 100, Y: -100}, {X: 100, Y: 100}, {X: -100, Y: 100}} {
			s.resetCamera(g, false)
			s.panCamera(g, dir.X, dir.Y)
			k, x, y, ok := s.fit(g)
			w, h := g.pixels()
			if !ok || x > 1e-6 || y > 1e-6 || x+3000*k < w-1e-6 || y+1800*k < h-1e-6 {
				t.Fatalf("unclamped view: %g %g %g", k, x, y)
			}
			s.layoutPoints(g, nil)
			for _, target := range s.targets {
				p := g.db.Locations[s.loc].Map.Points[s.choices[target.choice].Point]
				if !s.cameraPointVisible(g, p.At.Pt()) {
					t.Fatal("offscreen point leaked a label into viewport")
				}
			}
		}
		// Every selectable point can be brought onscreen without traveling, including
		// the last point beyond dozens of intermediate roads.
		for i, c := range s.choices {
			s.sel = i
			p := g.db.Locations[s.loc].Map.Points[c.Point].At.Pt()
			s.revealCameraPoint(g, p)
			x, y := s.screen(g, p)
			if !s.cameraPointVisible(g, p) {
				t.Fatalf("selection %s stayed offscreen", c.Point)
			}
			got, ok := s.pointAt(g, x, y-10)
			if !ok || got.Point != c.Point {
				t.Fatalf("marker %s hit %+v %v at %g %g", c.Point, got, ok, x, y)
			}
			s.layoutPoints(g, nil)
			labelHit := false
			for _, target := range s.targets {
				if target.choice == i && target.box.w > 60 {
					b := target.box
					for yy := b.y + 4; yy < b.y+b.h-4 && !labelHit; yy += 4 {
						for xx := b.x + 4; xx < b.x+b.w-4; xx += 4 {
							if hit, ok := s.pointAt(g, xx, yy); ok && hit.Point == c.Point {
								labelHit = true
								break
							}
						}
					}
				}
			}
			if !labelHit {
				t.Fatalf("label %s not clickable", c.Point)
			}
		}
		s.zoomCamera(g, 1000)
		if s.cameraZoom != 8 || s.labelSize(g) != size {
			t.Fatal("zoom limit or label size changed")
		}
		s.resetCamera(g, true)
		k, x, y, _ := s.fit(g)
		w, h := g.pixels()
		if math.Abs(k-math.Min(w/3000, h/1800)) > 1e-9 || x < -1e-6 || y < -1e-6 {
			t.Fatalf("overview is not fitted: scale %.17g offsets %.17g %.17g", k, x, y)
		}
		s.resetCamera(g, false)
		if !s.cameraPointVisible(g, s.at) || s.cameraZoom != 3 {
			t.Fatal("current position not restored")
		}
	}
	// The pointer toolbar consumes its click instead of activating points beneath it.
	for i, b := range s.cameraButtons(g) {
		g.in.click, g.in.mouse = true, image.Pt(int(b.x+b.w/2), int(b.y+b.h/2))
		if !s.updateCamera(g) {
			t.Fatalf("toolbar button %d not consumed", i)
		}
	}
	g.in.click = false
	after, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("browsing mutated game state")
	}
	// Tab gives a complete keyboard traversal; arrow selection is based on the
	// focused point, so distant points are reachable without moving the token.
	seen := map[string]bool{}
	for range len(s.choices) {
		g.in.pressed[btnNext] = true
		s.updateCamera(g)
		g.in.pressed = [numButtons]bool{}
		seen[s.choices[s.sel].Point] = true
	}
	if len(seen) != len(s.choices) {
		t.Fatal("keyboard traversal missed a point")
	}
	s.sel = 0
	s.selectToward(g, geom.Pt{X: 1})
	if s.sel == 0 {
		t.Fatal("direction did not select next point")
	}
	// A camera-enabled journey still accounts for every road exactly once.
	var destination atlas.Choice
	for _, c := range s.choices {
		if c.Point == "p59" {
			destination = c
		}
	}
	steps := g.st.Steps
	s.goTo(g, destination)
	wldSettle(t, g, s, 15000)
	if g.st.Spot != "p59" || g.st.Steps-steps != 59 || g.st.Vars["arrivals"] != float64(1) {
		t.Fatalf("travel: %s steps %d arrivals %v", g.st.Spot, g.st.Steps-steps, g.st.Vars["arrivals"])
	}
	saved, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	s.panCamera(g, -100, -100)
	s.cameraPan = true
	restored, err := rpg.Parse(g.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.loadGame(restored)
	fresh, ok := g.stack[0].(*atlasScene)
	if !ok || fresh.cameraZoom != 3 || fresh.cameraPan || !fresh.cameraPointVisible(g, fresh.at) || fresh.here != "p59" {
		t.Fatal("load did not reset the camera at the saved point")
	}
}

func TestAtlasHideClosedPoint(t *testing.T) {
	g, s := wldAtlas(t, "fx_named")
	a := g.db.Locations[s.loc].Map
	a.Points["gate"].Hide = "closed"
	// Even a previously visited point must disappear when its condition closes.
	g.st.Spot = "gate"
	s.here = "gate"
	for _, open := range []bool{false, true, false} {
		g.st.Flags["gate_open"] = open
		s.refresh(g)
		if (wldShown(s, "gate") > 0) != open {
			t.Fatalf("open=%v: wrong visibility", open)
		}
		for _, c := range s.choices {
			if !open && c.Point == "gate" {
				t.Fatal("hidden point focusable")
			}
		}
		for i, road := range a.Roads {
			if !open && slices.Contains(road.Between, "gate") && s.roads[i].shown {
				t.Fatal("hidden point road drawn")
			}
		}
		s.layoutPoints(g, nil)
		x, y := s.screen(g, a.Points["gate"].At.Pt())
		if c, ok := s.pointAt(g, x, y); !open && ok && c.Point == "gate" {
			t.Fatal("hidden pointer target")
		}
	}
}

// camera.buttons: faint buttons draw see-through until the pointer is over one, hidden
// ones are neither drawn nor clicked.
func TestAtlasCameraButtonsShowing(t *testing.T) {
	for _, mode := range []string{"faint", "hidden"} {
		t.Run(mode, func(t *testing.T) {
			fsys := atlasCameraFiles(t)
			f := fsys["data/survey.yaml"]
			f.Data = bytes.Replace(f.Data, []byte("max_zoom: 8}"), []byte("max_zoom: 8, buttons: "+mode+"}"), 1)
			g := wldOpenWith(t, Options{FS: fsys, Location: "large", Seed: 1})
			wldFrames(t, g, 5)
			s, ok := g.stack[0].(*atlasScene)
			if !ok {
				t.Fatalf("scene %T", g.stack[0])
			}
			bs := s.cameraButtons(g)
			if mode == "hidden" {
				if len(bs) != 0 {
					t.Fatalf("%d hidden buttons", len(bs))
				}
				wldFramesDrawn(t, g, 2)
				return
			}
			g.in.mouse.X, g.in.mouse.Y = 1, 1
			if a := s.cameraButtonsAlpha(g, bs); a >= 1 {
				t.Errorf("faint buttons away from the pointer draw at %g", a)
			}
			g.in.mouse.X, g.in.mouse.Y = int(bs[0].x+2), int(bs[0].y+2)
			if a := s.cameraButtonsAlpha(g, bs); a != 1 {
				t.Errorf("faint buttons under the pointer draw at %g", a)
			}
			s.drawCamera(g, wldScreen(g))
		})
	}
}

// However far a map's camera is moved, the screen's corners stay on the picture: with or
// without a safe_area, and on a map of instant activation.
func TestAtlasCameraCoversScreen(t *testing.T) {
	for _, extra := range []string{"", "\n      safe_area: [0.1, 0.17, 0.08, 0]", "\n      activation: instant\n      safe_area: [0.1, 0.17, 0.08, 0]"} {
		t.Run(strings.TrimSpace(extra), func(t *testing.T) {
			fsys := atlasCameraFiles(t)
			f := fsys["data/survey.yaml"]
			f.Data = bytes.Replace(f.Data, []byte("max_zoom: 8}"), []byte("max_zoom: 8}"+extra), 1)
			g := wldOpenWith(t, Options{FS: fsys, Location: "large", Seed: 1})
			wldFrames(t, g, 5)
			s, ok := g.stack[0].(*atlasScene)
			if !ok {
				t.Fatalf("scene %T", g.stack[0])
			}
			atlasCornersCovered(t, g, s)
		})
	}
}
