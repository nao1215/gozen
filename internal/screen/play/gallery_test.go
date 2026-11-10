package play

import (
	"testing"
	"testing/fstest"
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
