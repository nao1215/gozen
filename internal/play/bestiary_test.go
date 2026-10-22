package play

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
)

func TestBestiaryStagesAndDevelopmentKey(t *testing.T) {
	g := bookGame(t, Options{Location: "camp"}, "ui: {bestiary: true}\n")
	if !g.bestiaryInMenus() {
		t.Fatal("the menu does not offer the enabled bestiary")
	}
	id := g.db.Order["enemies"][0]
	s := newBestiary(g, nil)
	if g.bestiarySeen(id) {
		t.Fatal("an unseen enemy is unlocked")
	}
	g.st.Vars["bestiary."+id+".seen"] = true
	if !g.bestiarySeen(id) || g.bestiaryDefeated(id) != 0 {
		t.Fatal("the seen stage is not available")
	}
	g.st.Vars["bestiary."+id+".defeated"] = float64(3)
	if got := g.bestiaryDefeated(id); got != 3 {
		t.Fatalf("defeat count = %v", got)
	}
	s.draw(g, ebiten.NewImage(1280, 720))

	g.opts.Development = true
	g.in.pressed[btnBestiary] = true
	g.bestiaryKey()
	if _, ok := g.stack[len(g.stack)-1].(*bestiaryScreen); !ok {
		t.Fatal("F4 did not open the bestiary in development play")
	}
	for _, enemy := range g.db.Order["enemies"] {
		if !g.bestiarySeen(enemy) {
			t.Errorf("development page %s is locked", enemy)
		}
	}
}

func pressBestiary(g *Game, s *bestiaryScreen, key button) {
	g.in.pressed[key] = true
	s.update(g)
	g.in.pressed[key] = false
}

func TestBestiaryPictureLineageNavigationAndHistory(t *testing.T) {
	g := ranchGame(t)
	g.opts.Development = true
	s := newBestiary(g, nil)
	s.sel = slices.Index(s.ids, "boss")
	pressBestiary(g, s, btnOK)
	if !s.inTree || s.root != "boss" {
		t.Fatalf("tree not opened: %+v", s)
	}
	tiles := s.lineageTiles(322, 104, 930, 535)
	if len(tiles) != 3 || tiles[1].node.Species != "mote" || tiles[2].node.Species != "mote" {
		t.Fatalf("picture nodes = %+v", tiles)
	}
	pressBestiary(g, s, btnDown)
	if s.cur != 1 {
		t.Fatalf("down cursor = %d", s.cur)
	}
	pressBestiary(g, s, btnRight)
	if s.cur != 2 {
		t.Fatalf("right cursor = %d", s.cur)
	}
	pressBestiary(g, s, btnLeft)
	if s.cur != 1 {
		t.Fatalf("left cursor = %d", s.cur)
	}
	pressBestiary(g, s, btnUp)
	if s.cur != 0 {
		t.Fatalf("up cursor = %d", s.cur)
	}
	pressBestiary(g, s, btnDown)
	pressBestiary(g, s, btnRight)
	pressBestiary(g, s, btnOK)
	if s.root != "mote" || s.cur != 0 || len(s.history) != 1 || len(s.lineageTiles(322, 104, 930, 535)) != 1 {
		t.Fatalf("capture leaf navigation = %+v", s)
	}
	pressBestiary(g, s, btnOK)
	if len(s.history) != 2 {
		t.Fatalf("history depth = %d", len(s.history))
	}
	pressBestiary(g, s, btnCancel)
	if s.root != "mote" || len(s.history) != 1 {
		t.Fatalf("first back = %+v", s)
	}
	pressBestiary(g, s, btnCancel)
	if s.root != "boss" || s.cur != 2 || len(s.history) != 0 {
		t.Fatalf("back history = %+v", s)
	}
	pressBestiary(g, s, btnBack)
	if s.inTree || s.ids[s.sel] != "mote" {
		t.Fatalf("page key = %+v", s)
	}
}

func TestBestiaryPictureLineageRecipeAndUnseen(t *testing.T) {
	g := ranchGame(t)
	g.db.Enemies["boss"].Recruit = g.db.Enemies["mote"].Recruit
	s := newBestiary(g, nil)
	s.root, s.inTree = "boss", true
	g.st.Vars["bestiary.boss.seen"] = true
	if tiles := s.lineageTiles(322, 104, 930, 535); len(tiles) != 1 || tiles[0].node.Recipe.Kind != "capture" {
		t.Fatalf("default capture = %+v", tiles)
	}
	pressBestiary(g, s, btnNext)
	tiles := s.lineageTiles(322, 104, 930, 535)
	if len(tiles) != 3 || tiles[0].node.Choice != 1 || s.cur != 0 {
		t.Fatalf("other recipe = %+v", tiles)
	}
	if s.speciesName(g, "mote", !g.bestiarySeen("mote")) != "?" {
		t.Fatal("unseen name revealed")
	}
	pressBestiary(g, s, btnDown)
	pressBestiary(g, s, btnBack)
	if !s.inTree {
		t.Fatal("unseen page opened")
	}
	g.opts.Development = true
	if s.speciesName(g, "mote", !g.bestiarySeen("mote")) == "?" {
		t.Fatal("development node hidden")
	}
}

func TestBestiaryFamilyRuleHasExampleParents(t *testing.T) {
	g := ranchGame(t)
	g.db.Breeding.Pairs = nil
	g.db.Breeding.Families[0].Children = []string{"boss"}
	g.db.Enemies["boss"].Rank = 1
	s := newBestiary(g, nil)
	s.root = "boss"
	tiles := s.lineageTiles(322, 104, 930, 535)
	if len(tiles) != 3 || tiles[0].node.Recipe.Kind != data.RecipeFamily || tiles[0].node.Recipe.Families != [2]string{"slime", "slime"} || tiles[1].node.Species != "mote" {
		t.Fatalf("family example = %+v", tiles)
	}
}

// The book uses the same 1280x720 layout units at both native and scaled Full HD.
// Check the measured labels, framed columns, and tree rows against those units.
func TestBestiaryFitsBothScreenSizes(t *testing.T) {
	g := ranchGame(t)
	elder := *g.db.Enemies["boss"]
	elder.Name = "Elder Sentinel"
	g.db.Enemies["elder"] = &elder
	g.db.Order["enemies"] = append(g.db.Order["enemies"], "elder")
	g.db.Breeding.Pairs = append(g.db.Breeding.Pairs, data.BreedPair{Parents: []string{"boss", "boss"}, Child: "elder"})
	g.opts.Development = true
	s := newBestiary(g, nil)
	originalScreen := g.db.Game.Screen
	defer func() {
		g.db.Game.Screen = originalScreen
		g.ui = 1
		gfx.SetScale(1)
	}()
	for _, tc := range []struct {
		w, h  int
		scale float64
	}{{1280, 720, 1}, {1920, 1080, 1.5}} {
		w, h, scale := tc.w, tc.h, tc.scale
		g.db.Game.Screen.Width, g.db.Game.Screen.Height = w, h
		g.ui = scale
		gfx.SetScale(scale)
		sw, sh := g.screen()
		if sw != 1280 || sh != 720 {
			t.Fatalf("logical screen %v x %v", sw, sh)
		}
		if 42+252 > 322 || 322+sw-350 > sw-20 || 100+sh-175 > sh-20 {
			t.Fatal("book columns escape or overlap the frame")
		}
		for _, id := range s.ids {
			e := g.db.Enemies[id]
			if gfx.TextWidth(gfx.Ellipsis(e.Name, sizeS*0.9, 184), sizeS*0.9) > 184 {
				t.Fatalf("%s list label does not fit", id)
			}
			for _, line := range gfx.Wrap(e.Lore, sizeS*0.85, 500) {
				if gfx.TextWidth(line, sizeS*0.85) > 500.1 {
					t.Fatalf("%s lore does not fit", id)
				}
			}
		}
		s.draw(g, ebiten.NewImage(w, h))
		s.inTree = true
		s.draw(g, ebiten.NewImage(w, h))
		s.inTree = false
		s.root = "elder"
		tiles := s.lineageTiles(322, 104, sw-350, sh-185)
		if len(tiles) != 7 {
			t.Fatalf("want seven pictures, got %d", len(tiles))
		}
		for _, tile := range tiles {
			if tile.x < 322+20 || tile.y < 104+50 || tile.x+tile.w > sw-28 || tile.y+tile.h > sh-90 {
				t.Fatalf("picture escapes panel at %dx%d: %+v", w, h, tile)
			}
		}
	}
}
