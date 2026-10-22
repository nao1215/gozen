package play

import (
	"slices"
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

func TestBestiaryLineageNavigationAndFolding(t *testing.T) {
	g := ranchGame(t)
	g.opts.Development = true
	s := newBestiary(g, nil)
	s.sel = slices.Index(s.ids, "boss")
	if s.sel < 0 {
		t.Fatal("fixture has no child species")
	}
	rows := s.lineageRows(g)
	if len(rows) != 3 || rows[2].node.Ref != "seen" {
		t.Fatalf("initial tree rows = %+v", rows)
	}
	s.inTree = true
	s.folds[s.tree.Path] = false
	if rows := s.lineageRows(g); len(rows) != 1 || rows[0].expanded {
		t.Fatalf("folded rows = %+v", rows)
	}
	g.in.pressed[btnRight] = true
	s.update(g)
	g.in.pressed[btnRight] = false
	if rows := s.lineageRows(g); len(rows) != 3 {
		t.Fatalf("opened rows = %+v", rows)
	}
	s.cur = 1
	g.in.pressed[btnOK] = true
	s.update(g)
	g.in.pressed[btnOK] = false
	if s.ids[s.sel] != "mote" || s.inTree || s.tree != nil {
		t.Fatalf("choosing parent did not open page: %s", s.ids[s.sel])
	}
	g.opts.Development = false
	g.st.Vars["bestiary.boss.seen"] = true
	s.sel = slices.Index(s.ids, "boss")
	s.resetTree()
	rows = s.lineageRows(g)
	if len(rows) != 3 || !rows[1].hidden || !rows[2].hidden || s.lineageLabel(g, rows[1]) != "?" {
		t.Fatalf("unseen parent was revealed: %+v", rows)
	}
}
