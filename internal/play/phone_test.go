package play

import (
	"image"
	"math"
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

// TestPhoneFeed: the feed scrolls down a post at a time with the down key,
// stops at its top and its bottom, follows a drag and glides on after it, a
// click on a post's picture likes it, and OK at the bottom puts the phone down.
func TestPhoneFeed(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Picture: "x.png", Likes: 3}, {Picture: "x.png"}, {Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}},
	}})
	p := g.phone
	phoneTicks(g, phoneRise)
	posts, _, _, end := g.feedView(p)
	phonePress(g, btnUp)
	phoneTicks(g, 30)
	if p.scroll != 0 {
		t.Fatalf("up at the top scrolled to %v", p.scroll)
	}
	phonePress(g, btnDown)
	phoneTicks(g, 40)
	if p.scroll != posts[1].top {
		t.Fatalf("down did not bring the second post up: %v, want %v", p.scroll, posts[1].top)
	}
	for range 4 {
		phonePress(g, btnDown)
		phoneTicks(g, 40)
	}
	if p.scroll != end {
		t.Fatalf("down past the last post scrolled to %v, want the bottom %v", p.scroll, end)
	}
	g.in = input{wheel: -5}
	g.updatePhone()
	if p.scroll != end {
		t.Fatalf("the wheel scrolled past the bottom: %v", p.scroll)
	}

	// A drag up scrolls down by as much and glides on after the finger lifts.
	g.in = input{wheel: 20}
	g.updatePhone()
	if p.scroll != 0 {
		t.Fatalf("the wheel up did not scroll to the top: %v", p.scroll)
	}
	drag := func(from, to int) {
		g.in = input{pdown: true, pheld: true}
		g.in.mouse.Y = from
		g.updatePhone()
		for y := from; y != to; y += (to - from) / 10 {
			g.in = input{pheld: true}
			g.in.mouse.Y = y + (to-from)/10
			g.updatePhone()
		}
		g.in = input{pup: true}
		g.in.mouse.Y = to
		g.updatePhone()
	}
	drag(500, 300)
	after := p.scroll
	if after < 190 || p.glide == 0 {
		t.Fatalf("a drag of 200 up scrolled %v with glide %v", after, p.glide)
	}
	phoneTicks(g, 10)
	if p.scroll <= after {
		t.Fatalf("the feed did not glide on after the drag: %v then %v", after, p.scroll)
	}
	drag(100, 700)
	phoneTicks(g, 120)
	if p.scroll != 0 || p.glide != 0 {
		t.Fatalf("a drag down past the top did not stop there: %v glide %v", p.scroll, p.glide)
	}

	// A click on the first post's picture likes it.
	sx, sy, w, _ := g.phoneScreenRect(false)
	at := image.Pt(int(sx+w/2), int(sy+phoneFeedTop+posts[0].pic+40))
	g.in = input{pdown: true, pup: true, click: true, mouse: at}
	g.updatePhone()
	if !p.liked[0] || g.feedPictureAt(p, posts, w, at) != 0 {
		t.Fatal("a click on the picture did not like the post")
	}
	phonePress(g, btnOK)
	if p.closing != 0 {
		t.Fatal("OK at the top put the phone down")
	}
	phoneTicks(g, 40)
	if p.scroll != posts[1].top {
		t.Fatalf("OK did not scroll a post down: %v", p.scroll)
	}
	g.in = input{wheel: -50}
	g.updatePhone()
	phonePress(g, btnOK)
	if p.closing == 0 {
		t.Fatal("OK at the bottom did not put the phone down")
	}
}

// TestPhoneFeedClip: a post's clip plays, looping, only while its picture is on the screen.
func TestPhoneFeedClip(t *testing.T) {
	posts := []data.PhonePost{{Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}}}
	for range 4 {
		posts = append(posts, data.PhonePost{Picture: "x.png"})
	}
	posts = append(posts, data.PhonePost{Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}})
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: posts})
	p := g.phone
	phoneTicks(g, phoneRise+90)
	first := p.clips["post0"]
	if first == nil || first.played == 0 || first.played != (phoneRise+90)%first.length() {
		t.Fatal("the first post's clip did not loop on the screen")
	}
	if p.clips["post5"] != nil {
		t.Fatal("the last post's clip played off the screen")
	}
	g.in = input{wheel: -100}
	g.updatePhone()
	held := first.played
	phoneTicks(g, 30)
	if first.played != held {
		t.Fatal("the first post's clip went on off the screen")
	}
	if c := p.clips["post5"]; c == nil || c.played == 0 {
		t.Fatal("the last post's clip did not play once on the screen")
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
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

// TestPhoneFrames: a clip's footage goes by at its rate, 12 a second unless
// told, and lasts as long as its frames.
func TestPhoneFrames(t *testing.T) {
	frames := []string{"f/1.png", "f/2.png", "f/3.png", "f/4.png"}
	c := &phoneClip{v: &data.PhoneVideo{Frames: frames, FPS: 10}}
	if c.length() != 24 {
		t.Fatalf("four frames at 10 a second last %d ticks, want 24", c.length())
	}
	if c.frame() != "f/1.png" {
		t.Fatalf("the clip starts on %q", c.frame())
	}
	c.played = 13
	if c.frame() != "f/3.png" {
		t.Fatalf("13 ticks in at 10 a second shows %q, want f/3.png", c.frame())
	}
	if (&phoneClip{v: &data.PhoneVideo{Frames: frames}}).length() != 20 {
		t.Fatal("frames without fps do not go at 12 a second")
	}

}

// TestPhoneFramesFeed: a feed's post starts its footage again at its end.
func TestPhoneFramesFeed(t *testing.T) {
	frames := []string{"f/1.png", "f/2.png", "f/3.png", "f/4.png"}
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Video: &data.PhoneVideo{Frames: frames, FPS: 10, Seconds: 1}},
	}})
	p := g.phone
	phoneTicks(g, phoneRise)
	post := p.clips["post0"]
	seen := map[string]bool{}
	for range 70 {
		phoneTicks(g, 1)
		seen[post.frame()] = true
	}
	if len(seen) != len(frames) || post.played != (phoneRise+70)%post.length() {
		t.Fatalf("the post's footage did not loop through its frames: saw %v, at %d", seen, post.played)
	}
}

// TestPhoneFramesVideo: the video app's footage advances, keeps its frame while
// paused and stays on its last frame at its end.
func TestPhoneFramesVideo(t *testing.T) {
	frames := []string{"f/1.png", "f/2.png", "f/3.png", "f/4.png"}
	g := phoneGame(t, data.PhoneCue{App: "video", Video: &data.PhoneVideo{Frames: frames, FPS: 10, Seconds: 1}})
	p := g.phone
	phoneTicks(g, phoneRise+10)
	at := p.watch.frame()
	if at == frames[0] {
		t.Fatal("the video app's footage did not advance")
	}
	phonePress(g, btnOK)
	if !p.watch.paused {
		t.Fatal("a press did not pause the video")
	}
	played, at := p.watch.played, p.watch.frame()
	phoneTicks(g, 30)
	if p.watch.played != played || p.watch.frame() != at {
		t.Fatalf("a paused clip went on: %d to %d, %q to %q", played, p.watch.played, at, p.watch.frame())
	}
	phonePress(g, btnOK)
	phoneTicks(g, 90)
	if p.watch.played != p.watch.length() || p.watch.frame() != frames[3] {
		t.Fatalf("the video did not stop on its last frame: %q at %d", p.watch.frame(), p.watch.played)
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
}

// TestPhoneFeedAspect: a post's box takes its media's shape between 16:9 and
// 4:5, a clip of a picture and a puppet keeps 4:5, and footage is drawn whole.
func TestPhoneFeedAspect(t *testing.T) {
	wide, tall := ebiten.NewImage(160, 90), ebiten.NewImage(90, 160)
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Video: &data.PhoneVideo{Frames: []string{"wide.png"}}}, {Picture: "tall.png"},
		{Video: &data.PhoneVideo{Picture: "wide.png", Puppet: "p.yaml"}},
	}})
	g.pics.imgs["wide.png"] = &picEntry{img: wide}
	g.pics.imgs["tall.png"] = &picEntry{img: tall}
	posts, w, _, _ := g.feedView(g.phone)
	for i, want := range []float64{9.0 / 16, 5.0 / 4, 5.0 / 4} {
		if got := posts[i].picH / w; math.Abs(got-want) > 1e-9 {
			t.Errorf("post %d: box %v of its width, want %v", i, got, want)
		}
	}
	if posts[1].top != posts[0].top+posts[0].h+phoneFeedGap {
		t.Error("the posts do not stack at their own heights")
	}

	// Footage in a box wider or taller than it is drawn whole and centered.
	for _, tc := range []struct {
		iw, ih        int
		w, h, x, y, s float64
	}{
		{1280, 720, 400, 100, 200 - 1280*(100.0/720)/2, 0, 100.0 / 720},
		{1280, 720, 320, 400, 0, 200 - 720*0.25/2, 0.25},
	} {
		x, y, k := containAt(tc.iw, tc.ih, 0, 0, tc.w, tc.h)
		if math.Abs(x-tc.x) > 1e-9 || math.Abs(y-tc.y) > 1e-9 || math.Abs(k-tc.s) > 1e-9 {
			t.Errorf("%dx%d in %vx%v: at %v,%v scale %v, want %v,%v scale %v", tc.iw, tc.ih, tc.w, tc.h, x, y, k, tc.x, tc.y, tc.s)
		}
	}
	dst := ebiten.NewImage(400, 300)
	defer dst.Deallocate()
	g.drawPhoneMedia(dst, g.phone, "test", &phoneClip{v: &data.PhoneVideo{Frames: []string{"wide.png"}}}, 0, 0, 400, 300, 0)
}
