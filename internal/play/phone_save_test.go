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
