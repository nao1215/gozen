package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	lang "github.com/nao1215/gozen/internal/text"
)

// mnuTop is the scene on top of the stack.
func mnuTop(g *Game) scene { return g.stack[len(g.stack)-1] }

// mnuPress feeds one tick of input (the given buttons pressed) to the top scene.
func mnuPress(g *Game, btns ...button) {
	g.in.pressed = [numButtons]bool{}
	for _, b := range btns {
		g.in.pressed[b] = true
	}
	mnuTop(g).update(g)
	g.in.pressed = [numButtons]bool{}
}

// mnuShiftTab is a Shift+Tab tick.
func mnuShiftTab(g *Game) {
	g.in.shift = true
	mnuPress(g, btnNext)
	g.in.shift = false
}

// mnuDrawAll draws the whole game once (only queues GPU commands, no pixel reads).
func mnuDrawAll(g *Game) { g.Draw(ebiten.NewImage(1280, 720)) }

// mnuMenu opens the field menu of a new camp game with the guide in the party.
func mnuMenu(t *testing.T) (*Game, *menu) {
	t.Helper()
	g := openGame(t, Options{Location: "camp", Seed: 3})
	g.st.Join("guide")
	g.push(newMenu(g))
	return g, mnuTop(g).(*menu)
}

// mnuTab picks the tab with the label and presses OK.
func mnuTab(t *testing.T, g *Game, label string) {
	t.Helper()
	m := mnuTop(g).(*menu)
	for i, it := range m.tabs.items {
		if it.label == label {
			m.tabs.sel = i
			mnuPress(g, btnOK)
			return
		}
	}
	t.Fatalf("no tab %q in %v", label, labels(m.tabs.items))
}

func TestMenuTabsAndSaveRules(t *testing.T) {
	g, m := mnuMenu(t)
	// camp is not marked safe in the test game: saving is disabled (shown with a cross).
	var save, load *listItem
	for i := range m.tabs.items {
		switch m.tabs.items[i].label {
		case lang.T("セーブ"):
			save = &m.tabs.items[i]
		case lang.T("ロード"):
			load = &m.tabs.items[i]
		}
	}
	if save == nil || load == nil {
		t.Fatalf("tabs %v", labels(m.tabs.items))
	}
	if !save.disabled || save.right != "×" {
		t.Fatalf("save on an unsafe place is allowed: %+v", save)
	}
	if !load.disabled {
		t.Fatal("load offered with no save")
	}
	mnuDrawAll(g)
	mnuPress(g, btnCancel)
	if len(g.stack) != 1 {
		t.Fatalf("cancel left the menu open: %d scenes", len(g.stack))
	}
}
