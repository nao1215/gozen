package play

import (
	"os"
	"path/filepath"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// mnuFinishReplay pushes the recollection and plays it to its end.
func mnuFinishReplay(t *testing.T, g *Game, r *sceneReplay) {
	t.Helper()
	g.push(r)
	for i := 0; i < 600 && mnuTop(g) == scene(r); i++ {
		if i%3 == 0 {
			mnuPress(g, btnOK)
		} else {
			mnuPress(g)
		}
	}
	if mnuTop(g) == scene(r) {
		t.Fatal("the recollection never ended")
	}
}

// A recollection starts in the place its event happens (or the scene's own), with the
// scene's flag and set on a fresh state, and plays its music.
func TestGalleryRecollectionPlaceAndState(t *testing.T) {
	g := mnuGalleryGame(t, Options{Seed: 1})
	saved := g.st
	saved.Gold = 12
	scenes := g.db.Game.Gallery.Scenes

	r := newSceneReplay(g, scenes[3]) // location, set and music of its own
	g.push(r)
	mnuPress(g, btnOK) // the set runs as the first command of the event
	g.stack = g.stack[:1]
	if g.st.Location != "field" || g.st.Gold != 777 || !g.st.Flags["extra"] {
		t.Fatalf("location %q gold %d flags %v", g.st.Location, g.st.Gold, g.st.Flags)
	}
	if g.baseSong != "plain" {
		t.Fatalf("scene music %q", g.baseSong)
	}
	if saved.Gold != 12 {
		t.Fatal("the saved state changed")
	}
	mnuFinishReplay(t, g, r)
	if g.st != saved || saved.Gold != 12 || g.replaying {
		t.Fatalf("after: replaying %v gold %d", g.replaying, saved.Gold)
	}

	r = newSceneReplay(g, scenes[4]) // the place of its event, the gallery's music
	if g.st.Location != "field" || g.baseSong != "theme" {
		t.Fatalf("location %q song %q", g.st.Location, g.baseSong)
	}
	mnuFinishReplay(t, g, r)

	// an event in a dungeon does not move the state; it shows the place's picture instead
	g.db.Locations["room"].Background = data.Picture{File: "cg/a.png"}
	r = newSceneReplay(g, scenes[5])
	if g.st.Location == "room" || r.bg.File != "cg/a.png" {
		t.Fatalf("location %q bg %q", g.st.Location, r.bg.File)
	}
	mnuDrawAll(g)
	g.db.Locations["room"].Background = data.Picture{}
	g.db.Locations["room"].BattleBG = data.Picture{File: "cg/b.001.png"}
	mnuFinishReplay(t, g, r)
	r = newSceneReplay(g, scenes[5])
	if r.bg.File != "cg/b.001.png" {
		t.Fatalf("battle background %q", r.bg.File)
	}
	g.push(r)
	mnuDrawAll(g)
	g.stack = g.stack[:1]
	mnuFinishRestore(g, r)
}

func mnuFinishRestore(g *Game, r *sceneReplay) {
	g.st, g.events, g.req, g.replaying = r.saved, r.events, nil, false
}

// Starting from a page reads on to that line without the player pressing anything.
func TestGalleryRecollectionFromPageReadsOn(t *testing.T) {
	g, _ := mnuOpenGallery(t, Options{Seed: 1})
	mnuPress(g, btnRight)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	rep, ok := mnuTop(g).(*sceneReplay)
	if !ok || g.fast == nil {
		t.Fatalf("scene %T fast %v", mnuTop(g), g.fast)
	}
	for i := 0; i < 600 && g.fast != nil; i++ {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if g.fast != nil {
		t.Fatal("never arrived at the page")
	}
	if g.evLines != 2 || !g.talk.active || g.talk.say.Text != "Two." {
		t.Fatalf("lines read %d, talk %+v", g.evLines, g.talk.say)
	}
	if mnuTop(g) != scene(rep) {
		t.Fatal("the recollection ended while seeking")
	}
	// the rest plays out normally
	mnuFinishReplay(t, g, rep)
}

func TestGalleryUnreadableRecordIsKept(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "gallery.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	g := mnuGalleryGame(t, Options{SaveDir: dir, Seed: 1})
	if g.unlockReadErr == nil || len(g.unlocked) != 0 {
		t.Fatalf("err %v unlocks %v", g.unlockReadErr, g.unlocked)
	}
	g.unlocked = nil
	g.unlock("cg:cg/a.png") // a nil map is allowed; nothing is written over the unreadable record
	if !g.unlocked["cg:cg/a.png"] {
		t.Fatal("the unlock was not remembered in this run")
	}
	if st, err := os.Stat(filepath.Join(dir, "gallery.json")); err != nil || !st.IsDir() {
		t.Fatalf("the unreadable record was replaced: %v", err)
	}
}

func TestGalleryRefreshKeepsMusicSongAndDropsGoneTab(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	mnuPress(g, btnLeft)
	s.music.play(g, 1)
	if s.music.playing != 1 {
		t.Fatalf("playing %d", s.music.playing)
	}
	s.refresh(g)
	if s.music.playing != 1 || s.tab != galleryMusicTab {
		t.Fatalf("after a reload: playing %d tab %d", s.music.playing, s.tab)
	}
	// the songs went away on a reload: the music tab is gone and the screen returns to the first
	g.db.Music.Songs = nil
	g.db.Order["songs"] = nil
	s.refresh(g)
	if s.tab != 0 || len(s.tabIDs(g)) != 2 {
		t.Fatalf("tab %d ids %v", s.tab, s.tabIDs(g))
	}
	if got := s.tabLabels(g); len(got) != 2 {
		t.Fatalf("labels %v", got)
	}
}

func TestGalleryEmptyMenuAndHint(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	g.db.Game.Gallery.Scenes = nil
	g.db.Game.Gallery.CGs = nil
	g.db.Music.Songs = nil
	s.refresh(g)
	mnuDrawAll(g)
	mnuPress(g, btnOK) // nothing to open
	mnuPress(g, btnRight)
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if len(g.stack) != 2 || g.hasGallery() {
		t.Fatalf("stack %d hasGallery %v", len(g.stack), g.hasGallery())
	}
}
