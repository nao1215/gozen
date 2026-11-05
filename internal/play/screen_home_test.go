package play

import (
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/savefile"
	"github.com/nao1215/gozen/internal/testgame"
)

func homeGame(t *testing.T) *Game {
	t.Helper()
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{}
	db.Game.Screens = map[string]*data.AuthorScreen{
		"desk":   {Title: "desk", Kind: "entities", Home: true, View: data.PlanningView{Fullscreen: true}},
		"ledger": {Title: "ledger", Kind: "entities", Back: "desk", View: data.PlanningView{Fullscreen: true}},
		"notes":  {Title: "notes", Entries: []data.ScreenEntry{{Label: "Read"}, {Label: "Write"}}},
	}
	return &Game{db: db, st: testgame.State(db, 1), store: savefile.DirStore(t.TempDir())}
}

// TestHomeScreenCancelOpensSystemMenu: cancel on a home screen never closes it
// (so the player never falls to the place under it); it opens the system menu.
func TestHomeScreenCancelOpensSystemMenu(t *testing.T) {
	g := homeGame(t)
	home := newAuthorScreen(g, "desk", nil, nil)
	g.stack = []scene{&field{}, home}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	home.updateSurface(g)
	if len(g.stack) != 3 || g.stack[1] != home {
		t.Fatalf("home closed or was replaced: %d scenes", len(g.stack))
	}
	if _, ok := g.stack[2].(*sysMenu); !ok {
		t.Fatalf("cancel opened %T, want the system menu", g.stack[2])
	}
}

// TestBackOpensScreenOnlyWithoutOpener: back replaces the screen only when no
// authored screen lies under it; over another screen cancel returns to it.
func TestBackOpensScreenOnlyWithoutOpener(t *testing.T) {
	g := homeGame(t)
	s := newAuthorScreen(g, "ledger", nil, nil)
	g.stack = []scene{&field{}, s}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	s.updateSurface(g)
	if len(g.stack) != 2 || s.name != "desk" {
		t.Fatalf("back did not open desk in place: %d scenes, %s", len(g.stack), s.name)
	}
	under := newAuthorScreen(g, "desk", nil, nil)
	over := newAuthorScreen(g, "ledger", nil, nil)
	g.stack = []scene{&field{}, under, over}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	over.updateSurface(g)
	if len(g.stack) != 2 || g.stack[1] != under {
		t.Fatal("cancel over an opener did not return to it")
	}
}

// TestOpeningHomeFromActionGoesHome: an action that opens the home screen does
// not stack a copy; the screen on top goes home once the action ends.
func TestOpeningHomeFromActionGoesHome(t *testing.T) {
	g := homeGame(t)
	home := newAuthorScreen(g, "desk", nil, nil)
	top := newAuthorScreen(g, "ledger", nil, nil)
	top.running = true
	g.stack = []scene{&field{}, home, top}
	if !g.goingHome(event.Open{Screen: "desk"}) || top.destination != "desk" || len(g.stack) != 3 {
		t.Fatalf("opening home stacked a screen: %d scenes, destination %q", len(g.stack), top.destination)
	}
	if g.goingHome(event.Open{Screen: "ledger"}) {
		t.Fatal("a screen that is not a home must open normally")
	}
}

// TestScriptPickSelectsVisibleControl: pick finds a control by its label and
// confirms it with OK; an unknown label is an error listing what is there.
func TestScriptPickSelectsVisibleControl(t *testing.T) {
	g := homeGame(t)
	s := newAuthorScreen(g, "notes", nil, nil)
	g.stack = []scene{&field{}, s}
	var in input
	if err := g.scriptPick(&in, "Write"); err != nil || s.list.sel != 1 || !in.pressed[btnOK] {
		t.Fatalf("pick did not select Write: %v sel=%d", err, s.list.sel)
	}
	if err := g.scriptPick(&in, "Missing"); err == nil {
		t.Fatal("pick of an absent label must fail")
	}
}
