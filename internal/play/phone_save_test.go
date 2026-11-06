package play

import (
	"encoding/json"
	"image"
	"testing"
	"testing/fstest"

	"github.com/nao1215/gozen/internal/rpg"
)

const phoneSaveData = `events:
  bn:
    do:
      - say: "Start."
      - phone:
          app: page
          var: var.pick
          rows: [{text: Pick}, {button: Left}, {button: Right, set: {var.right: 1}}]
      - say: "Before."
      - phone: {app: banner, notices: [{app: Chat, title: Ann, text: Hi, event: reply}]}
      - say: "During."
      - say: "After."
  reply:
    do:
      - set: {var.replied: 1}
      - say: "Reply."
`

func phoneSaveGame(t *testing.T) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["data/phone.yaml"] = &fstest.MapFile{Data: []byte(phoneSaveData)}
	g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), ShotDir: t.TempDir(), Location: "camp", Event: "bn"})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestPhoneSaveReplays: a save made after a page's button and a banner's
// click loads with the same state: the button's result kept and the banner's
// event queued after the event again, as the click did.
func TestPhoneSaveReplays(t *testing.T) {
	g := phoneSaveGame(t)
	for range 60 {
		if g.phone != nil && g.phone.ready() {
			break
		}
		if g.phone == nil && g.fast == nil && at(g) == "Start." {
			btlKey(g, btnOK)
		}
		sysFrames(t, g, 1)
	}
	if g.phone == nil {
		t.Fatalf("the page did not come up: at %q fast %v req %T", at(g), g.fast != nil, g.req)
	}
	g.phone.putDown(2, -1)
	for range 60 {
		if at(g) == "Before." {
			break
		}
		sysFrames(t, g, 1)
	}
	for range 6 {
		btlKey(g, btnOK)
		sysFrames(t, g, 3)
		if at(g) == "During." {
			break
		}
	}
	if at(g) != "During." || len(g.banners) != 1 {
		t.Fatalf("at %q with %d banners", at(g), len(g.banners))
	}
	g.banners[0].age = bannerSlide + 1
	sw, _ := g.screen()
	g.in = input{click: true, mouse: image.Pt(int(sw/2), int(g.banners[0].top()+20))}
	g.updateBanners()
	if len(g.evOpened) != 1 {
		t.Fatalf("the banner's click was not kept: %v", g.evOpened)
	}
	saved, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Cursor rpg.Cursor }
	if err := json.Unmarshal(saved, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cursor.Opened) != 1 || doc.Cursor.Opened[0] != "reply" || len(doc.Cursor.Answers) != 1 || doc.Cursor.Answers[0] != 1 {
		t.Fatalf("cursor: %+v", doc.Cursor)
	}

	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	h := phoneSaveGame(t)
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	sysFrames(t, h, 2)
	st, err := rpg.Parse(h.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.loadSavedGame(st)
	if h.failure != nil {
		t.Fatal(h.failure)
	}
	if at(h) != "During." || h.st.Vars["pick"] != "Right" || h.st.Vars["right"] != float64(1) {
		t.Fatalf("loaded at %q, vars %v", at(h), h.st.Vars)
	}
	for range 10 {
		btlKey(h, btnOK)
		sysFrames(t, h, 3)
		if at(h) == "Reply." {
			break
		}
	}
	if at(h) != "Reply." || h.st.Vars["replied"] != float64(1) {
		t.Fatalf("the banner's event did not run after the loaded event: at %q, vars %v", at(h), h.st.Vars)
	}
}

const feedSaveData = `events:
  fl:
    do:
      - say: "Start."
      - phone:
          app: feed
          posts:
            - {who: ann, picture: x.png, set: {var.ann: 1}}
            - {who: bo, text: Lunch}
            - {who: cy, text: Rain, set: {var.cy: 1}}
      - say: "After."
      - say: "End."
`

// TestPhoneFeedLikesSaveReplays: the likes a feed keeps are one saved answer,
// so a save made after it and Back over it both come back with the same sets.
func TestPhoneFeedLikesSaveReplays(t *testing.T) {
	open := func() *Game {
		fsys := gameFiles(t)
		fsys["data/feed.yaml"] = &fstest.MapFile{Data: []byte(feedSaveData)}
		g, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), ShotDir: t.TempDir(), Location: "camp", Event: "fl"})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	g := open()
	for range 60 {
		if g.phone != nil && g.phone.ready() {
			break
		}
		if g.phone == nil && g.fast == nil && at(g) == "Start." {
			btlKey(g, btnOK)
		}
		sysFrames(t, g, 1)
	}
	if g.phone == nil {
		t.Fatalf("the feed did not come up: at %q", at(g))
	}
	g.phone.liked[2] = true
	g.phone.closing = phoneRise
	for range 60 {
		if at(g) == "After." {
			break
		}
		sysFrames(t, g, 1)
	}
	if at(g) != "After." || g.st.Vars["cy"] != float64(1) || g.st.Vars["ann"] != nil {
		t.Fatalf("at %q, vars %v", at(g), g.st.Vars)
	}
	for range 6 {
		btlKey(g, btnOK)
		sysFrames(t, g, 3)
		if at(g) == "End." {
			break
		}
	}
	if at(g) != "End." {
		t.Fatalf("at %q", at(g))
	}
	saved, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Cursor rpg.Cursor }
	if err := json.Unmarshal(saved, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cursor.Answers) != 1 || doc.Cursor.Answers[0] != 2 {
		t.Fatalf("cursor answers %v, want [2] (the second kept post)", doc.Cursor.Answers)
	}
	g.rollBack()
	if at(g) != "After." || g.st.Vars["cy"] != float64(1) || g.st.Vars["ann"] != nil {
		t.Fatalf("Back stopped at %q with vars %v", at(g), g.st.Vars)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	h := open()
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	sysFrames(t, h, 2)
	st, err := rpg.Parse(h.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.loadSavedGame(st)
	if h.failure != nil {
		t.Fatal(h.failure)
	}
	if at(h) != "End." || h.st.Vars["cy"] != float64(1) || h.st.Vars["ann"] != nil {
		t.Fatalf("loaded at %q, vars %v", at(h), h.st.Vars)
	}
}
