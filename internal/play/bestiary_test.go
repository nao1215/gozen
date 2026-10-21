package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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
