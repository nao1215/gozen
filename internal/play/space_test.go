package play

import (
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/rpg"
)

func TestSpaceArrivalClearsMotion(t *testing.T) {
	s := &dungeonScene{}
	s.spaceMotion(rpg.Pos{X: 10, Y: 10, Level: 1}, 0)
	s.arrive()
	if s.space.remaining != 0 {
		t.Fatal("arrival keeps motion from the previous location")
	}
}

func TestSpacePresentation(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space/game"), Location: "courtyard"})
	g.cfg.DungeonSnap = true // Ordinary-dungeon presentation must not disable space interpolation.
	wldFrames(t, g, 25)
	s, ok := g.stack[len(g.stack)-1].(*dungeonScene)
	if !ok {
		t.Fatalf("scene = %T", g.stack[len(g.stack)-1])
	}
	g.st.Pos = rpg.Pos{In: "courtyard", X: 6, Y: 10}
	screen := wldScreen(g)
	for n := 1; n <= 4; n++ {
		s.forward(g)
		if s.space == nil || s.space.remaining != 18 {
			t.Fatal("instant dungeon transitions disabled stacked-space motion")
		}
		if g.st.Pos.Y != 10-n {
			t.Fatalf("step %d: %+v", n, g.st.Pos)
		}
		for range 18 {
			s.update(g)
			s.draw(g, screen)
		}
		if s.space.remaining != 0 {
			t.Fatal("motion did not settle")
		}
	}
	if g.st.Pos.Level != 1 {
		t.Fatal("did not reach upper level")
	}
	s.turn(g, 2, "back")
	for range 18 {
		s.update(g)
		s.draw(g, screen)
	}
	for range 4 {
		s.forward(g)
		for range 18 {
			s.update(g)
			s.draw(g, screen)
		}
	}
	if g.st.Pos.Level != 0 || g.st.Pos.Y != 10 {
		t.Fatalf("return: %+v", g.st.Pos)
	}
	// A removed floor must become open air rather than an opaque wall.
	g.st.Put = map[string]string{"courtyard/level/0@10,15": " "}
	s.draw(g, screen)
	if s.space.pixels[(15*64+10)*4] != 0 {
		t.Fatal("removed floor rendered as a wall")
	}
	// A resize retains bounded rendering, and a second draw reuses material storage.
	s.drawSpace(g, ebiten.NewImage(320, 180), g.db.Locations["courtyard"].Dungeon)
	if s.space.frame.Bounds().Dx() != 320 {
		t.Fatal("render size did not follow window")
	}
	materials := s.space.ceilingSky
	s.draw(g, screen)
	if s.space.ceilingSky != materials {
		t.Fatal("unchanged materials reallocated")
	}
}
