package play

import (
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/testgame"
)

const galleryEndingYAML = `
gallery:
  scenes:
    - {id: fin, title: Finale, event: finale, when: "true"}
    - {id: sv, title: Save point, event: savepoint, when: "true"}
`

const galleryEndingData = `
events:
  finale:
    do:
      - say: "The end."
      - clear: true
  savepoint:
    do:
      - say: "Rest here."
      - open: save
      - say: "Onward."
`

func galleryEndingGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append([]byte("schema_version: 4\n"), append(fsys["game.yaml"].Data, []byte(galleryEndingYAML)...)...)
	fsys["data/finale.yaml"] = &fstest.MapFile{Data: []byte(galleryEndingData)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Location: "camp", Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

// A recollection whose event ends the game (clear) ends as any other does: back in the
// gallery with the play it was opened from, not on the ending with the recollection mode
// left on (the play after it would win its battles unfought and record no unlock).
func TestGalleryRecollectionOfAnEndingLeavesReplayMode(t *testing.T) {
	g := galleryEndingGame(t)
	st := g.st
	g.push(newGallery(g))
	r := newSceneReplay(g, g.db.Game.Gallery.Scenes[0])
	g.push(r)
	for i := 0; i < 2000 && mnuTop(g) == scene(r); i++ {
		if i%3 == 0 {
			mnuPress(g, btnOK)
		} else {
			mnuPress(g)
		}
	}
	if _, ok := mnuTop(g).(*galleryScreen); !ok {
		t.Fatalf("the recollection of the ending did not come back to the gallery (top %T)", mnuTop(g))
	}
	if g.replaying || g.st != st {
		t.Fatalf("after the recollection the game still replays (%v) or lost its play (%v)", g.replaying, g.st != st)
	}
}

// A recollection does not open the save screen its event opens (as it skips save:): the
// state replayed is not the player's.
func TestGalleryRecollectionSkipsOpenSave(t *testing.T) {
	g := galleryEndingGame(t)
	g.push(newGallery(g))
	r := newSceneReplay(g, g.db.Game.Gallery.Scenes[1])
	g.push(r)
	for i := 0; i < 600 && mnuTop(g) == scene(r); i++ {
		if i%3 == 0 {
			mnuPress(g, btnOK)
		} else {
			mnuPress(g)
		}
	}
	if _, ok := mnuTop(g).(*saveMenu); ok && g.replaying {
		t.Fatal("a recollection opened the save screen over its made-up state")
	}
}

// TestGalleryReviewButton: the picture review opens by its button, as by Tab.
func TestGalleryReviewButton(t *testing.T) {
	g, _ := mnuReviewGame(t, true)
	s := newGallery(g)
	g.push(s)
	mnuTap(g, s.reviewButton(g))
	if _, ok := mnuTop(g).(*cgReview); !ok {
		t.Fatalf("a tap on the review button left %T on top", mnuTop(g))
	}
}

// TestGalleryEndings: an ending reached on the last line before the title is recorded,
// stays reached in a new play, and the gallery's Endings tab lists it by its title while
// the one not reached shows ??? with its hint.
func TestGalleryEndings(t *testing.T) {
	fsys := gameFiles(t)
	fsys["game.yaml"].Data = append([]byte("schema_version: 4\n"), append(fsys["game.yaml"].Data, []byte(`
gallery:
  endings:
    - {id: warm, title: Warm Night, flag: warm, picture: end.png}
    - {id: cold, title: Cold Road, when: "flag.cold", hint: Leave before the toast.}
`)...)...)
	fsys["assets/end.png"] = &fstest.MapFile{Data: testgame.Picture(32, 18)}
	dir := t.TempDir()
	open := func() *Game {
		g, err := New(Options{FS: fsys, NoSound: true, SaveDir: dir, Location: "camp", Seed: 1})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := g.Close(); err != nil { // a second Close returns nil
				t.Error(err)
			}
		})
		return g
	}
	g := open()
	if !g.hasGallery() {
		t.Fatal("a gallery of endings alone is no gallery")
	}
	g.st.Flags["warm"] = true
	g.toTitle()
	g.saveUnlocks()
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	g = open()
	if !g.unlocked["ending:warm"] || g.unlocked["ending:cold"] {
		t.Fatalf("unlocks after a new play %v", g.unlocked)
	}
	s := newGallery(g)
	at := -1
	for i, id := range s.tabIDs(g) {
		if id == galleryEndingTab {
			at = i
		}
	}
	if at != 2 || s.tabLabels(g)[at] != "Endings" {
		t.Fatalf("tabs %v %v: want Endings after Scenes", s.tabIDs(g), s.tabLabels(g))
	}
	s.tab = galleryEndingTab
	s.build(g)
	if it := s.menu.items; len(it) != 2 || it[0].label != "Warm Night" || it[0].disabled || it[1].label != "???" || !it[1].disabled {
		t.Fatalf("ending rows %+v", it)
	}
	s.menu.sel = 1
	s.draw(g, btlImage()) // the hint of the ending not reached
	g.in = input{}
	g.in.pressed[btnOK] = true
	s.menu.sel = 0
	s.update(g)
	if s.view != "" || len(g.stack) == 0 {
		t.Fatalf("choosing a reached ending opened %q", s.view)
	}
}
