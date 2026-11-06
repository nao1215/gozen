package play

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// phoneTicks runs n ticks of the phone with nothing pressed and tells whether it
// was put down.
func phoneTicks(g *Game, n int) bool {
	for range n {
		g.in = input{}
		if g.updatePhone() {
			return true
		}
	}
	return false
}

// phonePress runs one tick with b pressed.
func phonePress(g *Game, b button) bool {
	g.in = input{}
	g.in.pressed[b] = true
	return g.updatePhone()
}

func phoneGame(t *testing.T, cue data.PhoneCue) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "")
	g.Handle(event.PhoneEffect{Phone: cue})
	if g.phone == nil {
		t.Fatal("the phone effect did not bring the phone up")
	}
	t.Cleanup(func() {
		if g.phone != nil {
			g.phone.close()
		}
	})
	return g
}

// TestPhoneChat: the chat shows its first message, a press brings the owner's
// next one at once and another's after the typing dots, and a press on the last
// one puts the phone down. Input before the phone has come up is ignored.
func TestPhoneChat(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{
		{From: "Ann", Text: "Hi"}, {Mine: true, Text: "Hello"}, {From: "Ann", Text: "Lunch?"},
	}})
	p := g.phone
	phonePress(g, btnOK)
	if p.shown != 1 {
		t.Fatalf("a press while the phone rose showed %d messages", p.shown)
	}
	phoneTicks(g, phoneRise)
	phonePress(g, btnOK)
	if p.shown != 2 || p.typing != 0 {
		t.Fatalf("the owner's message: shown %d typing %d", p.shown, p.typing)
	}
	phonePress(g, btnOK)
	if p.shown != 2 || p.typing == 0 {
		t.Fatalf("another's message did not wait for the dots: shown %d typing %d", p.shown, p.typing)
	}
	phoneTicks(g, phoneTyping)
	if p.shown != 3 {
		t.Fatalf("the dots did not end in the message: shown %d", p.shown)
	}
	phonePress(g, btnOK)
	if p.closing == 0 {
		t.Fatal("a press on the last message did not put the phone down")
	}
	if !phoneTicks(g, phoneRise) {
		t.Fatal("the phone did not go down")
	}
}

// TestPhoneChatVideo: a press on a chat's newest video turns the phone on its
// side and plays it; Cancel turns it back to the chat, and the next press goes
// on to the next message.
func TestPhoneChatVideo(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{
		{From: "Ann", Video: &data.PhoneVideo{Picture: "x.png", Seconds: 2}}, {Mine: true, Text: "Nice"},
	}})
	p := g.phone
	phoneTicks(g, phoneRise)
	phonePress(g, btnOK)
	if p.watch == nil || !p.side {
		t.Fatal("a press on the video did not open it on the phone's side")
	}
	phoneTicks(g, phoneTurn+30)
	if p.watch.played == 0 {
		t.Fatal("the video did not play")
	}
	phonePress(g, btnCancel)
	if p.watch != nil || p.side || p.closing != 0 {
		t.Fatalf("Cancel did not go back to the chat: watch %v side %v closing %d", p.watch != nil, p.side, p.closing)
	}
	phoneTicks(g, phoneTurn)
	phonePress(g, btnOK)
	if p.shown != 2 {
		t.Fatalf("the chat did not go on after the video: shown %d", p.shown)
	}
}

// TestPhoneFeed: left and right turn the posts within the feed, a click likes
// the post on the screen, and OK on the last post puts the phone down.
func TestPhoneFeed(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Picture: "x.png", Likes: 3}, {Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}},
	}})
	p := g.phone
	phoneTicks(g, phoneRise)
	phonePress(g, btnLeft)
	if p.post != 0 {
		t.Fatalf("left on the first post moved to %d", p.post)
	}
	phonePress(g, btnRight)
	if p.post != 1 || p.slide == 0 {
		t.Fatalf("right did not slide to the next post: %d %v", p.post, p.slide)
	}
	phoneTicks(g, 90)
	if c := p.clips["post1"]; c == nil || c.played == 0 || c.played > c.length() {
		t.Fatal("the post's video did not loop in its place")
	}
	g.in = input{pdown: true}
	g.in.mouse.X = 200
	g.updatePhone()
	g.in = input{pup: true, click: true}
	g.in.mouse.X = 200
	g.updatePhone()
	if !p.liked[1] {
		t.Fatal("a click did not like the post")
	}
	phonePress(g, btnOK)
	if p.closing == 0 {
		t.Fatal("OK on the last post did not put the phone down")
	}
}

// TestPhoneVideoLandscape: the video app comes up upright, turns on its side,
// plays to its end and is put down by a press there; a press before pauses it.
func TestPhoneVideoLandscape(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "video", Hold: "landscape", Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}})
	p := g.phone
	phoneTicks(g, phoneRise)
	if !p.side || p.resting() {
		t.Fatal("the phone did not start turning on its side")
	}
	phoneTicks(g, phoneTurn)
	phonePress(g, btnOK)
	if !p.watch.paused {
		t.Fatal("a press did not pause the video")
	}
	phonePress(g, btnOK)
	phoneTicks(g, 70)
	if p.watch.played != p.watch.length() {
		t.Fatalf("the video did not stop at its end: %d of %d", p.watch.played, p.watch.length())
	}
	phonePress(g, btnOK)
	if p.closing == 0 {
		t.Fatal("a press at the end did not put the phone down")
	}
}

// TestPhoneSkip: skipping puts the phone down at once, and it draws without a
// picture to show.
func TestPhoneSkip(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "feed", Title: "Photos", Posts: []data.PhonePost{{Who: "ann", Picture: "missing.png"}}})
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	for range phoneRise + 2 {
		g.drawPhone(dst)
		phoneTicks(g, 1)
	}
	g.skipping = true
	if !phoneTicks(g, 3) {
		t.Fatal("skipping did not put the phone down")
	}
}
