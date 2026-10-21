package play

import (
	"image"
	"testing"

	lang "github.com/nao1215/gozen/internal/text"
)

// mnuOpenGallery opens the gallery with the first two pictures seen.
func mnuOpenGallery(t *testing.T, opts Options) (*Game, *galleryScreen) {
	t.Helper()
	g := mnuGalleryGame(t, opts)
	if !opts.GalleryAll {
		g.seenPicture("cg/a.png")
		g.seenPicture("cg/b.001.png")
	}
	s := newGallery(g)
	g.push(s)
	return g, s
}

func TestGalleryCGTab(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	if got := labels(s.menu.items); got[0] != "Alpha" || got[1] != "Beta" || got[2] != lang.T("？？？") {
		t.Fatalf("items %v", got)
	}
	if !s.menu.items[2].disabled || s.menu.items[0].disabled {
		t.Fatalf("locked flags %+v", s.menu.items)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if s.view != "cg/a.png" || len(s.pages) != 1 {
		t.Fatalf("view %q pages %v", s.view, s.pages)
	}
	mnuDrawAll(g)
	mnuPress(g, btnLeft) // nothing on a single page
	mnuPress(g, btnRight)
	if s.view != "cg/a.png" {
		t.Fatalf("view %q", s.view)
	}
	mnuPress(g, btnOK)
	if s.view != "" {
		t.Fatal("OK did not close the picture")
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	if s.view != "cg/b.001.png" || len(s.pages) != 2 {
		t.Fatalf("view %q pages %v", s.view, s.pages)
	}
	mnuDrawAll(g)
	mnuPress(g, btnRight)
	if s.view != "cg/b.002.png" || s.page != 1 {
		t.Fatalf("view %q", s.view)
	}
	mnuPress(g, btnLeft)
	if s.page != 0 {
		t.Fatalf("page %d", s.page)
	}
	// a click on the right half of the page label goes on, on the left half goes back
	r := s.pageLabelRect(g)
	g.in.click, g.in.mouse = true, image.Pt(r.Max.X-3, (r.Min.Y+r.Max.Y)/2)
	mnuTop(g).update(g)
	g.in.click = false
	if s.page != 1 {
		t.Fatalf("click forward: page %d", s.page)
	}
	g.in.click, g.in.mouse = true, image.Pt(r.Min.X+3, (r.Min.Y+r.Max.Y)/2)
	mnuTop(g).update(g)
	g.in.click = false
	if s.page != 0 {
		t.Fatalf("click back: page %d", s.page)
	}
	mnuPress(g, btnLeft) // first page, going back: stays
	mnuPress(g, btnOK)
	mnuPress(g, btnOK) // on the last page OK closes
	if s.view != "" || s.page != 0 {
		t.Fatalf("view %q page %d", s.view, s.page)
	}
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // the locked one
	if s.view != "" {
		t.Fatal("a locked picture opened")
	}
	// a flag opens a picture while the gallery is open and liveKeep keeps the place
	g.st.Flags["gamma_seen"] = true
	g.checkUnlocks()
	n := s.liveKeep(g).(*galleryScreen)
	if n != s || s.menu.items[2].label != "Gamma" || s.menu.sel != 2 {
		t.Fatalf("after the unlock: %v sel %d", labels(s.menu.items), s.menu.sel)
	}
	mnuPress(g, btnCancel)
	if len(g.stack) != 1 {
		t.Fatal("cancel did not leave the gallery")
	}
}

func TestGalleryRefreshWhileViewing(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	mnuPress(g, btnRight)
	if s.page != 1 {
		t.Fatalf("page %d", s.page)
	}
	s.refresh(g) // a hot reload while a picture is open stays on its page
	if s.view != "cg/b.002.png" || s.page != 1 {
		t.Fatalf("view %q page %d", s.view, s.page)
	}
	// the picture vanished from the game: the view closes
	g.db.Game.Gallery.CGs[1].Files = []string{"cg/none.*.png"}
	s.refresh(g)
	if s.view != "" || s.pages != nil {
		t.Fatalf("view %q pages %v", s.view, s.pages)
	}
}

func TestGalleryAllShowsEverything(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1, GalleryAll: true})
	for _, it := range s.menu.items {
		if it.disabled {
			t.Fatalf("GalleryAll left %q locked", it.label)
		}
	}
	mnuPress(g, btnRight)
	for _, it := range s.menu.items {
		if it.disabled {
			t.Fatalf("GalleryAll left the recollection %q locked", it.label)
		}
	}
	if len(g.musicBackgrounds()) != 4 {
		t.Fatalf("backgrounds %v", g.musicBackgrounds())
	}
	if !g.pictureOpen("cg/c.png") {
		t.Fatal("picture closed under GalleryAll")
	}
}

func TestGalleryRecollection(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	mnuPress(g, btnRight)
	if s.tab != 1 {
		t.Fatalf("tab %d", s.tab)
	}
	if got := labels(s.menu.items); got[0] != lang.T("？？？") || got[1] != "Second" || got[2] != lang.T("？？？") {
		t.Fatalf("items %v", got)
	}
	mnuDrawAll(g)
	g.st.Gold = 12
	mnuPress(g, btnDown)
	mnuPress(g, btnOK) // Second has pages: choose where to start
	if s.scenePages == nil || len(s.scenePages.items) != 2 {
		t.Fatalf("scenePages %+v", s.scenePages)
	}
	mnuDrawAll(g)
	mnuPress(g, btnCancel)
	if s.scenePages != nil {
		t.Fatal("cancel kept the page list")
	}
	mnuPress(g, btnOK)
	mnuPress(g, btnOK) // from the start
	rep, ok := mnuTop(g).(*sceneReplay)
	if !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	if !g.replaying || g.st == rep.saved || g.st.Gold == 12 {
		t.Fatal("the recollection does not run on a fresh state")
	}
	mnuDrawAll(g)
	for i := 0; i < 400 && len(g.stack) > 2; i++ {
		if i%3 == 0 {
			mnuPress(g, btnOK)
		} else {
			mnuPress(g)
		}
	}
	if len(g.stack) != 2 || g.replaying || g.st != rep.saved || g.st.Gold != 12 {
		t.Fatalf("after the recollection: stack %d replaying %v gold %d", len(g.stack), g.replaying, g.st.Gold)
	}
	// nothing the recollection did lasted
	if g.st.Flags["met_one"] {
		t.Fatal("a recollection changed the real flags")
	}
}

func TestGalleryRecollectionFromAPage(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	mnuPress(g, btnRight)
	mnuPress(g, btnDown)
	mnuPress(g, btnOK)
	mnuPress(g, btnDown) // "Middle"
	mnuPress(g, btnOK)
	if _, ok := mnuTop(g).(*sceneReplay); !ok {
		t.Fatalf("scene %T", mnuTop(g))
	}
	if g.fast == nil || g.fast.lines != 2 || !g.fast.page {
		t.Fatalf("fast %+v", g.fast)
	}
	_ = s
}

func TestGalleryTabsWrapAndMouse(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	if got := s.tabIDs(g); len(got) != 3 || got[2] != galleryMusicTab {
		t.Fatalf("tabs %v", got)
	}
	if got := s.tabLabels(g); len(got) != 3 || got[2] != lang.T("音楽") {
		t.Fatalf("labels %v", got)
	}
	mnuPress(g, btnLeft)
	if s.tab != galleryMusicTab {
		t.Fatalf("left from the first tab: %d", s.tab)
	}
	mnuPress(g, btnRight)
	if s.tab != 0 {
		t.Fatalf("right from the last tab: %d", s.tab)
	}
	g.in.click, g.in.mouse = true, image.Pt(tabRect(1).Min.X+5, tabRect(1).Min.Y+5)
	mnuTop(g).update(g)
	g.in.click = false
	if s.tab != 1 {
		t.Fatalf("click on the second tab: %d", s.tab)
	}
	mnuDrawAll(g)
}

func TestGalleryMusicRoom(t *testing.T) {
	g, s := mnuOpenGallery(t, Options{Seed: 1})
	mnuPress(g, btnLeft) // wraps to the music tab
	r := s.music
	if r == nil {
		t.Fatal("no music room")
	}
	want := []string{"Theme", "Theme" + lang.T("・") + "Fight", "plain"}
	if got := labels(s.menu.items); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("entries %v, want %v", got, want)
	}
	if !r.entries[1].oneShot || r.entries[0].oneShot || r.entries[2].oneShot {
		t.Fatalf("entries %+v", r.entries)
	}
	mnuDrawAll(g)
	mnuPress(g, btnOK)
	if r.playing != 0 || g.baseSong != "theme" || s.menu.items[0].right != "♪" {
		t.Fatalf("playing %d song %q right %q", r.playing, g.baseSong, s.menu.items[0].right)
	}
	if r.bg == "" {
		t.Fatal("no picture behind the song")
	}
	mnuDrawAll(g)
	mnuPress(g, btnNext) // the next song
	if r.playing != 1 || r.shotT == 0 || r.shotT != r.moduleTicks(g, "theme") {
		t.Fatalf("playing %d shotT %d", r.playing, r.shotT)
	}
	if want := 2 * 4 * 60 * 60 / 120; r.shotT != want {
		t.Fatalf("a module of 2 bars at 120 BPM lasts %d ticks, got %d", want, r.shotT)
	}
	// when the one-shot is over the base track shows as the one playing
	for range r.shotT {
		r.tick()
	}
	if r.playing != 0 {
		t.Fatalf("after the one-shot: playing %d", r.playing)
	}
	mnuShiftTab(g)
	if r.playing != 2 {
		t.Fatalf("backwards from the first: %d", r.playing)
	}
	// OK on the playing song stops it
	s.menu.sel = 2
	mnuPress(g, btnOK)
	if r.playing != -1 || g.baseSong != "" {
		t.Fatalf("playing %d song %q", r.playing, g.baseSong)
	}
	// the buttons: next, play, previous, stop
	sw, sh := g.screen()
	btns := musicButtons(sw, sh)
	click := func(i int) {
		c := btns[i]
		g.in.click, g.in.mouse = true, image.Pt((c.Min.X+c.Max.X)/2, (c.Min.Y+c.Max.Y)/2)
		mnuTop(g).update(g)
		g.in.click = false
	}
	s.menu.sel = 0
	click(1)
	if r.playing != 0 {
		t.Fatalf("play button: %d", r.playing)
	}
	click(2)
	if r.playing != 1 {
		t.Fatalf("next button: %d", r.playing)
	}
	click(0)
	if r.playing != 0 {
		t.Fatalf("previous button: %d", r.playing)
	}
	click(1)
	if r.playing != -1 {
		t.Fatalf("stop button: %d", r.playing)
	}
	// V hides the screen so only the picture shows; any press brings it back
	mnuPress(g, btnHide)
	if !r.hidden {
		t.Fatal("not hidden")
	}
	mnuDrawAll(g)
	mnuPress(g, btnRight) // a hidden screen does not switch tabs
	if s.tab != galleryMusicTab {
		t.Fatalf("tab %d", s.tab)
	}
	mnuPress(g, btnOK)
	if r.hidden {
		t.Fatal("still hidden")
	}
	// leaving after a song was played brings back the title music
	r.play(g, 0)
	mnuPress(g, btnCancel)
	if r.played || len(g.stack) != 1 {
		t.Fatalf("played %v stack %d", r.played, len(g.stack))
	}
}

func TestMusicBackgroundSequence(t *testing.T) {
	g := mnuGalleryGame(t, Options{Seed: 1, GalleryAll: true})
	r := newMusicRoom(g)
	if got := g.songPictures()["theme"]; len(got) != 2 {
		t.Fatalf("song pictures %v", got)
	}
	r.play(g, 0)
	// the song's own pictures come first, in order
	if r.bg != "cg/a.png" && r.bg != "cg/b.001.png" {
		t.Fatalf("bg %q", r.bg)
	}
	first := r.bg
	for range musicBGHold {
		r.tick()
	}
	if r.bg == first || r.bg == "" {
		t.Fatalf("the picture did not change after %d ticks: %q", musicBGHold, r.bg)
	}
	if r.prev != first {
		t.Fatalf("prev %q", r.prev)
	}
	// with one picture only there is nothing to change to
	r.pool = []string{"cg/a.png"}
	r.queue = nil
	r.bg = "cg/a.png"
	r.bgT = musicBGHold
	r.nextBackground(musicBGFade)
	if r.bg != "cg/a.png" || r.bgT != r.fade {
		t.Fatalf("bg %q bgT %d", r.bg, r.bgT)
	}
	if pickBackground(nil, "", r.rnd) != "" || pickBackground([]string{"x"}, "x", r.rnd) != "x" {
		t.Fatal("pickBackground edge cases")
	}
	for range 20 {
		if p := pickBackground([]string{"x", "y"}, "x", r.rnd); p != "y" {
			t.Fatalf("picked the current one: %q", p)
		}
	}
	// not-yet-open pictures are not offered
	g.opts.GalleryAll = false
	if got := g.songPictures()["theme"]; len(got) != 0 {
		t.Fatalf("closed pictures offered: %v", got)
	}
	if g.pictureOpen("cg/a.png") || !g.pictureOpen("elsewhere.png") {
		t.Fatal("pictureOpen rules")
	}
}
