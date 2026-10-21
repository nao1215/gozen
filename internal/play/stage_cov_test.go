package play

import (
	"strconv"
	"testing"

	"github.com/nao1215/gozen/internal/layer1/geom"
)

// Painted scenes (docs/architecture/scenes.md, ADR 0022): the leader is a small doll walking with
// the arrow keys at 1.6 m/s, kept inside the walk area and out of the blocks; OK takes what is
// within reach; walking into an exit's area leaves; a spot with do runs when stepped on.

// wldStage opens the yard from a directory, the way --dir does (the reload is one of the cases).
func wldStage(t *testing.T) (*Game, *stageScene, string) {
	t.Helper()
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "fx_yard")
	s, ok := g.stack[0].(*stageScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	wldFrames(t, g, 20)
	return g, s, dir
}

func wldStageArrivesOnStartSpotFacingIt(t *testing.T, g *Game, s *stageScene) {
	if s.player.at != (geom.Pt{X: 60, Y: 160}) {
		t.Fatalf("the leader stands at %v, want the start spot (60, 160)", s.player.at)
	}
	if s.player.facing.X <= 0 || s.player.facing.Y != 0 {
		t.Fatalf("facing %v, want east", s.player.facing)
	}
	if len(s.people) != 2 {
		t.Fatalf("%d people placed, want 2", len(s.people))
	}
	if w := g.stageWalkerOf(s, "elder"); w == nil || w.at != (geom.Pt{X: 200, Y: 150}) {
		t.Fatalf("elder at %+v", w)
	}
}

func wldStageArrowKeysWalkAndStayInsideWalkArea(t *testing.T, g *Game, s *stageScene) {
	x0 := s.player.at.X
	wldHold(t, g, 40, "right")
	if s.player.at.X <= x0 {
		t.Fatalf("pressing right did not move the leader: %v", s.player.at)
	}
	if s.player.facing.X <= 0 {
		t.Fatalf("facing %v after walking east", s.player.facing)
	}
	// up goes away from the camera; far past the back edge of the walk area the leader stops there
	wldHold(t, g, 250, "up")
	if !s.sc.Free(s.player.at) {
		t.Fatalf("the leader stands outside the walk area or in a block: %v", s.player.at)
	}
	if s.player.at.Y < 89 {
		t.Fatalf("walked out of the walk area at the back: %v", s.player.at)
	}
}

func wldStageBlocksAreSolid(t *testing.T, g *Game, s *stageScene) {
	s.player.at = geom.Pt{X: 100, Y: 130}
	wldHold(t, g, 80, "right")
	if s.player.at.X >= 140 {
		t.Fatalf("walked into the block at x 140..180: %v", s.player.at)
	}
}

func wldStageExitAreaLeaves(t *testing.T, g *Game, s *stageScene) {
	s.player.at = geom.Pt{X: 40, Y: 150}
	wldHold(t, g, 120, "down")
	wldFrames(t, g, 30)
	if g.st.Location != "camp" {
		t.Fatalf("walked into the exit's area but stayed at %q (feet %v)", g.st.Location, s.player.at)
	}
}

func wldStageOKTalksToPersonAndExaminesThing(t *testing.T, g *Game, s *stageScene) {
	s.player.at = geom.Pt{X: 185, Y: 150}
	s.player.facing = geom.Pt{X: 1}
	wldFrames(t, g, 3)
	wldPlay(t, g, "press ok\nwait 30\n", 200)
	if !g.st.Flags["met_elder"] {
		t.Fatal("OK beside Elder did not talk to them")
	}
	s.player.at = geom.Pt{X: 110, Y: 112}
	wldFrames(t, g, 3)
	wldPlay(t, g, "press ok\nwait 30\n", 200)
	if !g.st.Flags["read_sign"] {
		t.Fatal("OK beside the sign did not examine it")
	}
}

func wldStageStepTriggerThingRunsOnEntering(t *testing.T, g *Game, s *stageScene) {
	s.player.at = geom.Pt{X: 75, Y: 125}
	wldHold(t, g, 60, "up")
	wldFrames(t, g, 20)
	if !g.st.Flags["trapped"] {
		t.Fatalf("stepping into the thing's area did not run it (feet %v)", s.player.at)
	}
}

func wldStageWalkToSpotRunsItsDoOnce(t *testing.T, g *Game, s *stageScene) {
	wldPlay(t, g, "walk well\nwait 60\n", 3000)
	if g.st.Spot != "well" || !g.st.Flags["at_well"] {
		t.Fatalf("spot %q, at_well %v", g.st.Spot, g.st.Flags["at_well"])
	}
	_ = s
}

func wldStageClickWalksAndTalks(t *testing.T, g *Game, s *stageScene) {
	w := g.stageWalkerOf(s, "elder")
	pw, ph := g.pixels()
	sx, sy, _ := s.r.ScreenOf(w.at, pw, ph)
	// a click on the doll's body (above its feet) walks there and talks
	x, y := int(sx/g.ui), int(sy/g.ui)-40
	wldPlay(t, g, "click "+strconv.Itoa(x)+" "+strconv.Itoa(y)+"\nwait 2\n", 100)
	wldUntil(t, g, 400, func() bool { return g.st.Flags["met_elder"] })
	if !g.st.Flags["met_elder"] {
		t.Fatalf("clicking Elder did not talk (feet %v, goal %v)", s.player.at, s.goal)
	}
	// a click on bare floor only walks
	g.st.Flags["met_elder"] = false
	wldPlay(t, g, "click 300 600\nwait 60\n", 1000)
	if g.st.Flags["met_elder"] {
		t.Fatal("clicking the floor talked to someone")
	}
}

func wldStageListWalksToChosenEntry(t *testing.T, g *Game, s *stageScene) {
	wldPlay(t, g, "press menu\nwait 10\n", 100)
	l, ok := g.stack[len(g.stack)-1].(*entryList)
	if !ok {
		t.Fatalf("the menu key opened %T", g.stack[len(g.stack)-1])
	}
	at := -1
	for i, e := range l.entries {
		if e.Person != nil && e.Person.ID == "elder" {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no entry for Elder in the list: %+v", l.entries)
	}
	script := ""
	for range at {
		script += "press down\nwait 4\n"
	}
	script += "press ok\nwait 2\n"
	wldPlay(t, g, script, 2000)
	wldUntil(t, g, 400, func() bool { return g.st.Flags["met_elder"] })
	if !g.st.Flags["met_elder"] {
		t.Fatalf("choosing Elder in the list did not walk there and talk (feet %v)", s.player.at)
	}
	if len(g.stack) != 1 {
		t.Fatalf("%d scenes left open", len(g.stack))
	}
}

func wldStageBalloonsAppearOverNearbyPeople(t *testing.T, g *Game, s *stageScene) {
	s.player.at = geom.Pt{X: 185, Y: 150}
	wldFrames(t, g, 20)
	var elder *stagePerson
	for _, p := range s.people {
		if p.person.ID == "elder" {
			elder = p
		}
	}
	if elder.line != "Nice day." {
		t.Fatalf("Elder says %q, want the first line that holds", elder.line)
	}
	if elder.shown <= s.clock {
		t.Fatal("the balloon is not showing")
	}
	// the second line only holds once the gate is open
	g.st.Flags["gate_open"] = true
	if ls := g.lines(elder.person); len(ls) != 2 || ls[1] != "The gate is open." {
		t.Fatalf("lines %v", ls)
	}
}

func wldStageScriptWalkToMissingSpotFails(t *testing.T, g *Game, s *stageScene) {
	if _, done := s.scriptWalk(g, "nowhere", false); !done {
		t.Fatal("the walk to a missing spot should end")
	}
	if g.failure == nil {
		t.Fatal("no failure for a missing spot")
	}
	g.failure = nil
	if _, done := s.scriptWalk(g, "north", true); !done || s.player.facing.Y >= 0 {
		t.Fatalf("face north: facing %v", s.player.facing)
	}
}

func wldStageClickExaminesAThing(t *testing.T, g *Game, s *stageScene) {
	g.st.Flags["read_sign"] = false
	s.player.at = geom.Pt{X: 60, Y: 160}
	st := s.location(g).Stage
	var area geom.Pt
	for _, th := range st.Things {
		if th.ID == "sign" {
			area = th.Area.Poly().Center()
		}
	}
	pw, ph := g.pixels()
	sx, sy, _ := s.r.ScreenOf(area, pw, ph)
	wldPlay(t, g, "click "+strconv.Itoa(int(sx/g.ui))+" "+strconv.Itoa(int(sy/g.ui))+"\nwait 2\n", 100)
	wldUntil(t, g, 400, func() bool { return g.st.Flags["read_sign"] })
	if !g.st.Flags["read_sign"] {
		t.Fatalf("clicking the sign did not examine it (feet %v)", s.player.at)
	}
}

// Balloons are all one size, and only people farther than 8 m from the leader are one step smaller.
func wldStageBalloonSizeFollowsDistance(t *testing.T, _ *Game, s *stageScene) {
	for _, at := range []geom.Pt{{X: 190, Y: 150}, {X: 60, Y: 160}, {X: 20, Y: 175}, {X: 290, Y: 100}} {
		s.player.at = at
		for _, p := range s.people {
			far := s.sc.Meters(p.w.at, at) > balloonFarAt
			want := 1.0
			if far {
				want = balloonFar
			}
			if got := s.balloonScale(p); got != want {
				t.Errorf("leader at %v, %s %.1f m away: scale %v, want %v", at, p.person.ID, s.sc.Meters(p.w.at, at), got, want)
			}
		}
	}
}

// F5 makes the scene again from the new data and puts the leader back where the feet were.
func wldStageReloadKeepsTheFeet(t *testing.T, g *Game, s *stageScene, dir string) {
	s.player.at = geom.Pt{X: 120, Y: 150}
	wldEdit(t, dir, "data/world.yaml", "name: Test Yard", "name: Test Yard Two")
	wldPlay(t, g, "press reload\nwait 20\n", 100)
	s2, ok := g.stack[0].(*stageScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if g.db.Locations["fx_yard"].Name != "Test Yard Two" {
		t.Fatal("not reloaded")
	}
	if s2.player.at != (geom.Pt{X: 120, Y: 150}) {
		t.Fatalf("the leader stands at %v after the reload, want where it stood", s2.player.at)
	}
}

// The scenes bake dolls (a fraction of a second each), so one game serves the cases of a group.
func TestWldStageWalking(t *testing.T) {
	covSlow(t)
	g, _, dir := wldStage(t)
	steps := []struct {
		name string
		fn   func(*testing.T, *Game, *stageScene)
	}{
		{"arrival", wldStageArrivesOnStartSpotFacingIt},
		{"arrow keys", wldStageArrowKeysWalkAndStayInsideWalkArea},
		{"blocks", wldStageBlocksAreSolid},
		{"script", wldStageScriptWalkToMissingSpotFails},
		{"reload", func(t *testing.T, g *Game, s *stageScene) { wldStageReloadKeepsTheFeet(t, g, s, dir) }},
		{"exit", wldStageExitAreaLeaves}, // leaves the yard: last
	}
	for _, c := range steps {
		t.Run(c.name, func(t *testing.T) {
			cur, ok := g.stack[0].(*stageScene) // a reload makes the scene again
			if !ok {
				t.Fatalf("scene %T", g.stack[0])
			}
			c.fn(t, g, cur)
		})
	}
}

func TestWldStageInteractions(t *testing.T) {
	covSlow(t)
	g, s, _ := wldStage(t)
	steps := []struct {
		name string
		fn   func(*testing.T, *Game, *stageScene)
	}{
		{"balloons", wldStageBalloonsAppearOverNearbyPeople},
		{"talk and examine", wldStageOKTalksToPersonAndExaminesThing},
		{"step trigger", wldStageStepTriggerThingRunsOnEntering},
		{"spot", wldStageWalkToSpotRunsItsDoOnce},
		{"click", wldStageClickWalksAndTalks},
		{"click a thing", wldStageClickExaminesAThing},
		{"balloon size", wldStageBalloonSizeFollowsDistance},
		{"list", wldStageListWalksToChosenEntry},
	}
	for _, c := range steps {
		g.st.Flags["met_elder"] = false
		t.Run(c.name, func(t *testing.T) { c.fn(t, g, s) })
	}
}

// When the party arrives with its feet in an exit's area, it does not leave at once; it leaves when
// the feet go in again (docs/architecture/scenes.md: the exit's item is taken when the feet enter).
func TestWldStageArrivingInsideAnExitDoesNotLeaveAtOnce(t *testing.T) {
	covSlow(t)
	g := wldWorld(t, "camp", Options{Calm: true})
	wldFrames(t, g, 5)
	wldStartEvent(t, g, "ev_enter_door")
	wldFrames(t, g, 30)
	s, ok := g.stack[0].(*stageScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if g.st.Location != "fx_yard2" || g.st.Spot != "door" || s.player.at != (geom.Pt{X: 40, Y: 173}) {
		t.Fatalf("at %q spot %q feet %v", g.st.Location, g.st.Spot, s.player.at)
	}
	wldFrames(t, g, 60)
	if g.st.Location != "fx_yard2" {
		t.Fatal("the party left through the exit it arrived in")
	}
	wldHold(t, g, 30, "up")
	if s.sc.ExitAt(s.player.at) != "" {
		t.Fatalf("the feet are still in the exit's area: %v", s.player.at)
	}
	wldHold(t, g, 80, "down")
	wldFrames(t, g, 20)
	if g.st.Location != "camp" {
		t.Fatalf("walking into the exit's area again did not leave: at %q, feet %v", g.st.Location, s.player.at)
	}
}
