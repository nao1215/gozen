package play

import (
	"os"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/dungeon"
	"github.com/nao1215/gozen/internal/rpg"
)

func TestSpaceSpriteStateAndCollision(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../examples/space-town/game"), Location: "inn"})
	wldFrames(t, g, 25)
	s, ok := g.stack[0].(*dungeonScene)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	s.draw(g, wldScreen(g))
	if g.failure != nil {
		t.Fatal(g.failure)
	}
	if len(s.space.sprites) != 3 {
		t.Fatalf("sprites %d", len(s.space.sprites))
	}
	d := g.db.Locations["inn"].Dungeon
	before, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	for _, level := range []int{0, 1} {
		g.st.Pos = rpg.Pos{In: "inn", Level: level, X: 4, Y: 6, Facing: 1}
		cell := dungeon.CellAt(data.DungeonLevel(d, level), g.st, 5, 6)
		if !cell.Solid {
			t.Fatal("innkeeper lost its collider")
		}
		if s.space.pixels[((level*64+6)*64+5)*4] != 1 {
			t.Fatal("solid sprite rendered as a full wall")
		}
		if moved, _ := dungeon.Forward(g.db, g.st); moved {
			t.Fatal("walked through solid sprite")
		}
		cmds := dungeon.Look(g.db, g.st)
		if len(cmds) == 0 {
			t.Fatal("reachable NPC did not interact")
		}
		g.events.Start(cmds)
		wldPlay(t, g, "wait 10\npress z\nwait 20\n", 80)
		s.draw(g, wldScreen(g))
		if len(s.space.sprites) != 2-level {
			t.Fatalf("once removal crossed levels: %d sprites", len(s.space.sprites))
		}
	}
	if g.st.Vars["greetings"] != float64(2) {
		t.Fatalf("greetings %v", g.st.Vars["greetings"])
	}
	g.st.Pos = rpg.Pos{In: "inn", Level: 1, X: 4, Y: 3, Facing: 1}
	if moved, _ := dungeon.Forward(g.db, g.st); moved {
		t.Fatal("walked through the guest bed")
	}
	g.events.Start(dungeon.Look(g.db, g.st))
	wldPlay(t, g, "wait 10\npress z\nwait 20\n", 80)
	if !g.st.Flags["bed_seen"] {
		t.Fatal("guest bed could not be examined")
	}
	saved, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := rpg.Parse(g.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := s.space
	initial, err := rpg.Parse(g.db, before, nil)
	if err != nil {
		t.Fatal(err)
	}
	v.refreshOccupancy(initial, d)
	if len(v.sprites) != 3 {
		t.Fatal("restore did not bring back unused NPCs")
	}
	v.refreshOccupancy(restored, d)
	if len(v.sprites) != 1 || v.sprites[0].tile.Picture.File != "bed.png" {
		t.Fatal("restore changed sprite visibility")
	}
}

func TestSpaceSpriteBoundsIncludeOversizedArt(t *testing.T) {
	eye := [3]float64{0, .65, 0}
	small, _ := spriteBounds([3]float64{6, 0, -2}, eye, 0, 1, 1, 640, 360)
	large, depth := spriteBounds([3]float64{6, 0, -2}, eye, 0, 12, 3, 640, 360)
	if !small.Empty() || large.Empty() || depth != 2 {
		t.Fatalf("small=%v large=%v depth=%v", small, large, depth)
	}
	behind, _ := spriteBounds([3]float64{0, 0, 1}, eye, 0, 12, 3, 640, 360)
	if !behind.Empty() {
		t.Fatal("behind-camera billboard projected")
	}
}
