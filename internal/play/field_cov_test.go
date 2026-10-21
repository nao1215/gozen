package play

import (
	"image/color"
	"testing"

	"github.com/nao1215/gozen/internal/world"
)

// The menu place (field): the place's people, actions and exits as a list; a person with a
// tall standing picture stands beside the list when talked to; a place without a menu opens
// the small system menu instead.

func wldTown(t *testing.T) (*Game, *field) {
	t.Helper()
	g := wldWorld(t, "fx_town", Options{Calm: true})
	f, ok := g.stack[0].(*field)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	wldFrames(t, g, 30)
	return g, f
}

func wldLabels(f *field) []string {
	var out []string
	for _, e := range f.entries {
		out = append(out, e.Label)
	}
	return out
}

func TestWldFieldListsPeopleActionsAndExitsInOrder(t *testing.T) {
	_, f := wldTown(t)
	got := wldLabels(f)
	want := []string{"Talk: Guard", "Talk: Dog", "Rest", "→ Camp", "→ Closed Room", "Menu"}
	if len(got) != len(want) {
		t.Fatalf("entries %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("entries %q, want %q", got, want)
		}
	}
	if f.entries[len(f.entries)-1].Kind != world.Menu {
		t.Fatal("the last entry should open the menu")
	}
}

func TestWldFieldTalkStandsTallPicturesBesideTheList(t *testing.T) {
	g, f := wldTown(t)
	gen := g.stageGen
	wldPlay(t, g, "press ok\nwait 40\n", 100)
	if !g.st.Flags["talked_guard"] {
		t.Fatal("OK on the first entry did not talk to the guard")
	}
	if g.stageGen == gen {
		t.Fatal("the guard's tall picture was not stood beside the list")
	}
	// a person with a small picture is not stood (the portrait box would show it)
	gen = g.stageGen
	for f.entries[f.menu.sel].Person == nil || f.entries[f.menu.sel].Person.Name != "Dog" {
		wldPlay(t, g, "press down\nwait 6\n", 20)
		if f.menu.sel == len(f.entries)-1 {
			t.Fatal("no Dog entry")
		}
	}
	wldPlay(t, g, "press ok\nwait 40\n", 100)
	if !g.st.Flags["talked_dog"] || g.stageGen != gen {
		t.Fatalf("dog talked %v, stage generation %d to %d", g.st.Flags["talked_dog"], gen, g.stageGen)
	}
}

func TestWldFieldPeopleAppearWhenTheirConditionHolds(t *testing.T) {
	g, f := wldTown(t)
	if n := len(f.people(g)); n != 2 {
		t.Fatalf("%d people drawn, want the guard and the dog", n)
	}
	g.st.Flags["baker_open"] = true
	f.dirty = true
	wldFrames(t, g, 3)
	if got := wldLabels(f); got[1] != "Talk: Baker" {
		t.Fatalf("entries %q: the baker should come after the guard once the shop is open", got)
	}
	placed := f.place(g, f.people(g))
	sw, _ := g.screen()
	for i, p := range placed {
		if p.x < 0 || p.x > sw {
			t.Fatalf("person %d stands off the screen at %v", i, p.x)
		}
	}
	if placed[0].head == placed[1].head {
		t.Fatalf("two people stand on one spot: %+v", placed)
	}
}

func TestWldFieldHeadingGrowsWithTheDescription(t *testing.T) {
	g, f := wldTown(t)
	if h := f.headH(g); h <= 92 {
		t.Fatalf("a described place has a heading of %v", h)
	}
	wldPlay(t, g, "press down\nwait 4\npress down\nwait 4\npress down\nwait 4\npress down\nwait 4\n", 100)
	wldPlay(t, g, "press ok\nwait 3\n", 100)
	wldFrames(t, g, 5)
	if g.st.Location != "fx_nomenu" {
		t.Fatalf("at %q", g.st.Location)
	}
	f2, ok := g.stack[0].(*field)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	if h := f2.headH(g); h != 92 {
		t.Fatalf("a place without a description has a heading of %v", h)
	}
}

func TestWldFieldPlaceWithoutMenuOpensTheSystemMenu(t *testing.T) {
	g := wldWorld(t, "fx_nomenu", Options{Calm: true})
	wldFrames(t, g, 30)
	wldPlay(t, g, "press menu\nwait 10\n", 100)
	if _, ok := g.stack[len(g.stack)-1].(*sysMenu); !ok {
		t.Fatalf("the menu key opened %T", g.stack[len(g.stack)-1])
	}
	wldPlay(t, g, "press cancel\nwait 10\n", 100)
	wldFrames(t, g, 10)
	if len(g.stack) != 1 {
		t.Fatalf("%d scenes after closing", len(g.stack))
	}
}

func TestWldFieldStateTagsGroupBadFirstAndFoldTheRest(t *testing.T) {
	g, _ := wldTown(t)
	states := map[string]int{"sleep": 2, "poison": 1}
	rows := g.stateTagRows(states, 400, 0)
	if len(rows) != 1 || len(rows[0]) != 2 {
		t.Fatalf("rows %v", rows)
	}
	// a narrow room wraps; with a limit of rows the last tags fold into "+N"
	rows = g.stateTagRows(states, 20, 1)
	if len(rows) != 1 || len(rows[0]) != 1 || rows[0][0] != "+2" {
		t.Fatalf("rows %v, want the one row folded into +2", rows)
	}
	if rows := g.stateTagRows(map[string]int{}, 400, 3); len(rows) != 0 {
		t.Fatalf("rows %v for no states", rows)
	}
	if w := g.stateTagsWidth(states); w <= 0 {
		t.Fatalf("width %v", w)
	}
	// the party's card draws them, with bubbles for the poisoned scout
	g.st.Actors["scout"].States["poison"] = 3
	wldFrames(t, g, 5)
}

func TestWldInkOnDarkAndLight(t *testing.T) {
	if inkOn(color.NRGBA{R: 10, G: 10, B: 40, A: 255}) != colorWhite || inkOn(color.NRGBA{R: 240, G: 240, B: 200, A: 255}) != colorBlack {
		t.Fatal("text should be white on a dark tag and black on a light one")
	}
}
