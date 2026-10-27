package play

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	world "github.com/nao1215/gozen/internal/layer3/field"
)

// The title's menu keeps clear of the key hint under it however many entries it has, and
// the hint stays inside the screen.
func TestTitleMenuKeepsClearOfTheHint(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "-"}, "schema_version: 4\nui: {profiles: true, bestiary: true}\n", "")
	sw, sh := g.screen()
	m := g.titleMenu()
	if len(m.items) < 6 {
		t.Fatalf("%d entries, want the long menu", len(m.items))
	}
	if bottom := m.y + m.height(); bottom > sh-titleHintRoom {
		t.Errorf("the menu reaches %g, into the hint's room under %g", bottom, sh-titleHintRoom)
	}
	for _, tw := range []float64{100, 700, sw - 40} {
		if x := titleHintX(g, tw); x-tw/2 < 16 || x+tw/2 > sw-16 {
			t.Errorf("a hint %g wide centred at %g leaves the screen", tw, x)
		}
	}
	newTitle(g).draw(g, btlImage())
}

// A game with menus: none has no title and no menu: it opens on a new game at start
// without a save, then on the save written last (the autosave too), and the menu key, the
// right click and the place's entries open no menu.
func TestMenusNoneOpensOnTheNewestSave(t *testing.T) {
	const none = "schema_version: 9\nmenus: none\n"
	dir := t.TempDir()
	g, _ := btlOpen(t, Options{Location: "-", SaveDir: dir}, none, "")
	if _, title := g.stack[0].(*title); title || g.st == nil || g.st.Location != "camp" {
		t.Fatalf("without a save the game opens on %T at %v, want a new game at camp", g.stack[0], g.st)
	}
	for _, e := range world.Entries(g.db, g.st) {
		if e.Kind == world.Menu {
			t.Fatal("the place lists the menu")
		}
	}
	g.in.pressed[btnMenu] = true
	if g.menuKey() {
		t.Fatal("the menu key opens the menu")
	}

	at := func(slot int, loc string, when time.Time) {
		t.Helper()
		g.st.Location = loc
		if err := g.writeSave(g.slotPath(slot)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(dir, g.slotPath(slot)), when, when); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	at(1, "field", now.Add(-time.Hour))
	at(autoSlot, "camp", now)
	btlClose(t, g)
	for _, tc := range []struct{ newest, want string }{{"auto", "camp"}, {"save2", "field"}} {
		if tc.newest == "save2" {
			at(1, "field", now.Add(time.Hour))
		}
		g, _ := btlOpen(t, Options{Location: "-", SaveDir: dir}, none, "")
		if _, title := g.stack[0].(*title); title || g.st == nil || g.st.Location != tc.want {
			t.Errorf("with %s newest the game opens on %T at %v, want %s", tc.newest, g.stack[0], g.st, tc.want)
		}
		if i, _ := g.newestSave(); g.slotPath(i) != tc.newest+".json" {
			t.Errorf("the load list offers %s first, want %s", g.slotPath(i), tc.newest)
		}
		g.toTitle()
		if _, title := g.stack[0].(*title); title || g.st == nil || g.st.Location != tc.want {
			t.Errorf("going back to the title opens %T at %v, want the newest save again", g.stack[0], g.st)
		}
		btlClose(t, g)
	}

	g, _ = btlOpen(t, Options{Location: "-", SaveDir: dir}, "", "")
	if _, title := g.stack[0].(*title); !title {
		t.Errorf("a game with its menus opens on %T, want the title", g.stack[0])
	}
}
