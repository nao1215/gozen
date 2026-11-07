package play

import (
	"os"
	"path/filepath"
	"slices"
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

// ui.title.hide_disabled leaves out the entries that cannot be chosen (Continue with no
// save, Gallery with nothing to show), and each entry still opens its own screen: choosing
// goes by the entry, not by its row. Omitted, they stay in the menu, grayed.
func TestTitleHideDisabledLeavesOutWhatCannotBeChosen(t *testing.T) {
	labels := func(m *list) (out []string) {
		for _, it := range m.items {
			out = append(out, it.id)
		}
		return out
	}
	t.Run("omitted", func(t *testing.T) {
		g, _ := btlOpen(t, Options{Location: "-"}, "schema_version: 10\nui: {}\n", "")
		if got := labels(g.titleMenu()); !slices.Contains(got, "continue") || !slices.Contains(got, "gallery") {
			t.Fatalf("without hide_disabled the menu is %v, want continue and gallery grayed in it", got)
		}
	})
	g, _ := btlOpen(t, Options{Location: "-"}, "schema_version: 10\nui: {title: {hide_disabled: true}}\n", "")
	m := g.titleMenu()
	if got := labels(m); slices.Contains(got, "continue") || slices.Contains(got, "gallery") || got[0] != "new" || got[1] != "config" {
		t.Fatalf("with hide_disabled and no save the menu is %v", got)
	}
	for _, it := range m.items {
		if it.disabled {
			t.Errorf("%s is shown disabled", it.id)
		}
	}
	tt := newTitle(g)
	g.stack = []scene{tt}
	tt.menu.sel = 1
	before := len(g.stack)
	g.in.pressed[btnOK] = true
	tt.update(g)
	g.in.pressed[btnOK] = false
	if len(g.stack) != before+1 {
		t.Fatalf("choosing the second row opened %d screens", len(g.stack)-before)
	}
	if _, ok := g.stack[len(g.stack)-1].(*configScreen); !ok {
		t.Errorf("the second row opened %T, want the settings", g.stack[len(g.stack)-1])
	}
	newTitle(g).draw(g, btlImage())
}

// Script input stays pending throughout the end screen's input lockout.
func TestEndScreenScriptWaitsForInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		screen  scene
		lockout int
	}{
		{"ending", &ending{}, 150},
		{"game over", &gameOver{}, 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := btlOpen(t, Options{Location: "-", SaveDir: t.TempDir()}, "", "")
			g.stack = []scene{tc.screen}
			g.fadeT = 0
			g.turnSeen = g.turnKey()
			s := &script{cmds: []scriptCmd{{verb: "press", arg: "ok"}, {verb: "wait", n: 1000}}, ready: g.scriptReady}
			for tick := 0; tick <= tc.lockout; tick++ {
				g.in = input{}
				s.step(&g.in)
				if s.pc != 0 || g.in.pressed[btnOK] {
					t.Fatalf("input consumed during lockout at tick %d: pc=%d", tick, s.pc)
				}
				tc.screen.update(g)
				if g.stack[len(g.stack)-1] != tc.screen {
					t.Fatal("screen closed during lockout")
				}
			}
			s.step(&g.in)
			if s.pc != 1 || !g.in.pressed[btnOK] {
				t.Fatalf("ready screen did not receive OK: pc=%d", s.pc)
			}
			tc.screen.update(g)
			if _, ok := g.stack[len(g.stack)-1].(*title); !ok {
				t.Fatalf("OK opened %T, want title", g.stack[len(g.stack)-1])
			}
		})
	}
}
