package play

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/layer1/geom"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
)

// Properties of the places that hold for any way of playing them, tried with seeded random
// input: the feet stay on ground a person may stand on, the party stays on open cells, and a
// point of a map is always a point of that map.

func TestWldStageFeetStayOnWalkableGroundWhateverIsPressed(t *testing.T) {
	dir := wldDisk(t, "", nil)
	g := wldLive(t, dir, "fx_yard")
	s, ok := g.stack[0].(*stageScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // A seeded test input.
	dirs := [][]string{{"up"}, {"down"}, {"left"}, {"right"}, {"up", "left"}, {"up", "right"}, {"down", "left"}, {"down", "right"}}
	for i := range 40 {
		d := dirs[rng.IntN(len(dirs))]
		wldHold(t, g, 5+rng.IntN(30), d...)
		if !s.sc.Free(s.player.at) {
			t.Fatalf("segment %d (%v): the feet are at %v, outside the walk area or in a block", i, d, s.player.at)
		}
		if g.st.Location != "fx_yard" {
			// walked into the exit: the property holds on the other side of it, so come back
			g.Handle(event.MoveEffect{Location: "fx_yard"})
			wldFrames(t, g, 5)
			var isStage bool
			if s, isStage = g.stack[0].(*stageScene); !isStage {
				t.Fatalf("scene %T", g.stack[0])
			}
		}
	}
}

func TestWldDungeonPartyStaysOnOpenCellsWhateverIsPressed(t *testing.T) {
	g := wldMaze(t, "fx_maze", -1, 0, 0)
	d := dungeon.Of(g.db, g.st)
	w, h := d.Size()
	rng := rand.New(rand.NewPCG(5, 6)) //nolint:gosec // A seeded test input.
	keys := []string{"up", "up", "up", "left", "right", "down"}
	for chunk := range 12 {
		var b strings.Builder
		for range 12 {
			b.WriteString("press " + keys[rng.IntN(len(keys))] + "\nwait 11\n")
		}
		wldPlay(t, g, b.String(), 400)
		if g.st.Location != "fx_maze" || len(g.stack) != 1 {
			return // a tile took the party away (stairs) or a battle began: nothing more to check here
		}
		p := g.st.Pos
		if p.X < 0 || p.Y < 0 || p.X >= w || p.Y >= h {
			t.Fatalf("chunk %d: the party is at %+v, outside the %dx%d map", chunk, p, w, h)
		}
		if c := dungeon.CellAt(d, g.st, p.X, p.Y); !c.Passable() {
			t.Fatalf("chunk %d: the party stands in %q at %+v", chunk, c.Char, p)
		}
		if p.Facing < 0 || p.Facing > 3 {
			t.Fatalf("chunk %d: facing %d", chunk, p.Facing)
		}
	}
}

func TestWldMapSpotIsAlwaysAPointOfTheMap(t *testing.T) {
	g, as := wldAtlas(t, "fx_named")
	rng := rand.New(rand.NewPCG(7, 8)) //nolint:gosec // A seeded test input.
	for i := range 12 {
		x, y := rng.IntN(1280), rng.IntN(720)
		wldPlay(t, g, "click "+strconv.Itoa(x)+" "+strconv.Itoa(y)+"\nwait 2\n", 100)
		wldSettle(t, g, as, 400)
		if g.st.Location != "fx_named" {
			return // a click took the party out of the map (the inn)
		}
		if spot := g.st.Spot; spot != "" && g.db.Locations["fx_named"].Map.Points[spot] == nil {
			t.Fatalf("click %d at (%d, %d): the party stands on %q, which is no point of the map", i, x, y, spot)
		}
	}
}

// A save keeps where the party stood in a scene, and a spot that is not in the place is emptied
// when it is loaded and shown in the load report (docs/architecture/scenes.md).
func TestWldSaveKeepsTheSpotOfAScene(t *testing.T) {
	g := wldWorld(t, "fx_yard2", Options{Calm: true})
	wldFrames(t, g, 20)
	wldPlay(t, g, "walk well\nwait 5\n", 3000)
	if g.st.Spot != "well" {
		t.Fatalf("spot %q", g.st.Spot)
	}
	if err := g.save(1); err != nil {
		t.Fatal(err)
	}
	g.st.Spot = "start"
	st, err := g.load(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Spot != "well" {
		t.Fatalf("the save kept the spot %q", st.Spot)
	}
	g.loadGame(st)
	wldFrames(t, g, 10)
	s, ok := g.stack[0].(*stageScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if s.player.at != (geom.Pt{X: 250, Y: 160}) {
		t.Fatalf("the party stands at %v after loading, want the well (250, 160)", s.player.at)
	}
	// a spot the place does not have
	b, err := g.store.Read(g.slotPath(1))
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(b), `"spot": "well"`, `"spot": "nowhere"`, 1)
	if bad == string(b) {
		bad = strings.Replace(string(b), `"spot":"well"`, `"spot":"nowhere"`, 1)
	}
	if bad == string(b) {
		t.Fatal("the save has no spot to change")
	}
	if err := g.store.Write(g.slotPath(1), []byte(bad)); err != nil {
		t.Fatal(err)
	}
	st, err = g.load(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.Spot != "" {
		t.Fatalf("a spot that is not in the place was kept: %q", st.Spot)
	}
	found := false
	for _, c := range st.LoadReport.Changes {
		found = found || strings.Contains(c.Path, "spot")
	}
	if !found {
		t.Fatalf("the load report does not mention the spot: %+v", st.LoadReport.Changes)
	}
}
