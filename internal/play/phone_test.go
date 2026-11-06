package play

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/testgame"
	"rsc.io/qr"
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
	t.Cleanup(g.closePhone)
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
	if !p.liked[0] || g.feedPictureAt(p, posts, at) != 0 {
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

// TestPhonePage: a page scrolls with the down key and the wheel no farther
// than its bottom, and a tap on its button puts the phone down.
func TestPhonePage(t *testing.T) {
	rows := []data.PhoneRow{{Text: "Transfer", Size: "large", Bold: true}}
	for range 8 {
		rows = append(rows, data.PhoneRow{Item: &data.PhoneItem{Title: "Rent", Text: "Nov 1", Value: "-1,000"}})
	}
	rows = append(rows, data.PhoneRow{Pictures: []string{"x.png", "x.png", "x.png", "x.png"}}, data.PhoneRow{Button: "Confirm"})
	g := phoneGame(t, data.PhoneCue{App: "page", Title: "Bank", Color: "#1a73e8", Rows: rows})
	p := g.phone
	phoneTicks(g, phoneRise)
	laid, _, end := g.pageView(p)
	if end <= 0 {
		t.Fatal("the long page does not scroll")
	}
	phonePress(g, btnDown)
	phoneTicks(g, 40)
	if p.scroll != laid[1].top-phonePageGap {
		t.Fatalf("down scrolled to %v, want the second row at %v", p.scroll, laid[1].top-phonePageGap)
	}
	g.in = input{wheel: -50}
	g.updatePhone()
	if p.scroll != end {
		t.Fatalf("the wheel scrolled to %v, want the bottom %v", p.scroll, end)
	}
	// A tap on a row that is no button keeps the phone up; one on the button puts it down.
	sx, sy, w, _ := g.phoneScreenRect(false)
	tap := func(row int) {
		y := sy + phoneFeedTop + laid[row].top - p.scroll + laid[row].h/2
		g.in = input{pdown: true, pup: true, click: true, mouse: image.Pt(int(sx+w/2), int(y))}
		g.updatePhone()
	}
	tap(len(rows) - 2)
	if p.closing != 0 {
		t.Fatal("a tap on the grid put the phone down")
	}
	tap(len(rows) - 1)
	if p.closing == 0 {
		t.Fatal("a tap on the button did not put the phone down")
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
}

// TestPhoneLock: the notices come one a press (or by themselves with auto),
// declining a call brings the next one, and answering puts the phone down.
func TestPhoneLock(t *testing.T) {
	alert := data.PhoneNotice{App: "On-call", Title: "API error rate", Call: true}
	g := phoneGame(t, data.PhoneCue{App: "lock", Date: "Tuesday", Notices: []data.PhoneNotice{
		{App: "Monitor", Title: "Error rate 4%"}, alert, alert,
	}})
	p := g.phone
	phoneTicks(g, phoneRise)
	if p.shown != 1 || p.calling() {
		t.Fatalf("the lock screen came up with %d notices", p.shown)
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phonePress(g, btnOK)
	if p.shown != 2 || !p.calling() {
		t.Fatal("a press did not bring the call")
	}
	g.drawPhone(dst)
	phonePress(g, btnCancel)
	if p.shown != 3 || !p.calling() || p.closing != 0 {
		t.Fatalf("declining did not ring again: shown %d closing %d", p.shown, p.closing)
	}
	answer, _ := g.callButtons(p)
	g.in = input{click: true, mouse: image.Pt(int(answer.x), int(answer.y))}
	g.updatePhone()
	if p.closing == 0 {
		t.Fatal("answering did not put the phone down")
	}
}

// TestPhoneLockAuto: with auto the notices arrive by themselves, and the last
// call declined puts the phone down.
func TestPhoneLockAuto(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "lock", Auto: true, Notices: []data.PhoneNotice{
		{Title: "One"}, {Title: "Two"}, {Title: "Call", Call: true},
	}})
	p := g.phone
	phoneTicks(g, phoneRise+phoneLockAuto*2+2)
	if p.shown != 3 {
		t.Fatalf("auto brought %d notices, want 3", p.shown)
	}
	phonePress(g, btnCancel)
	if p.closing == 0 {
		t.Fatal("declining the last call did not put the phone down")
	}
}

// TestPhoneResultsPressed: the phone is put down with the result pressed: a
// page's button, a call answered, or the last call declined.
func TestPhoneResultsPressed(t *testing.T) {
	cue := data.PhoneCue{App: "page", Var: "var.mfa", Rows: []data.PhoneRow{
		{Text: "Approve?"}, {Button: "Approve"}, {Button: "Deny"},
		{Pictures: []string{"x.png", "x.png"}, Shut: true, Label: "Supporters only"},
	}}
	g := phoneGame(t, cue)
	p := g.phone
	phoneTicks(g, phoneRise)
	rows, _, _ := g.pageView(p)
	sx, sy, w, _ := g.phoneScreenRect(false)
	y := sy + phoneFeedTop + rows[2].top - p.scroll + rows[2].h/2
	g.in = input{pdown: true, pup: true, click: true, mouse: image.Pt(int(sx+w/2), int(y))}
	g.updatePhone()
	if p.closing == 0 || p.result != 1 {
		t.Fatalf("Deny put the phone down with result %d (closing %d), want 1", p.result, p.closing)
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)

	q := newPhoneView(data.PhoneCue{App: "lock", Var: "var.x", Notices: []data.PhoneNotice{{Title: "A", Call: true}, {Title: "B"}, {Title: "A", Call: true}}})
	q.putDown(-1, 2)
	if q.result != 1 {
		t.Fatalf("the second call answered is result %d, want 1", q.result)
	}
	q.putDown(-1, -1)
	if q.result != 2 {
		t.Fatalf("the last call declined is result %d, want 2", q.result)
	}
}

// TestPhoneTextPost: a post of text only is laid out as a card of its lines.
func TestPhoneTextPost(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Who: "ann", Text: "The booking app is down again. Third time this week.", Likes: 12, Replies: 3, Reposts: 4},
		{Picture: "x.png"},
	}})
	posts, w, _, _ := g.feedView(g.phone)
	if !posts[0].text || len(posts[0].lines) == 0 || posts[0].picH >= w*0.5 || posts[1].text {
		t.Fatalf("the text post: %+v", posts[0])
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	phoneTicks(g, phoneRise)
	g.drawPhone(dst)
}

// TestPhoneBanner: a banner slides down without stopping anything, a click on
// it opens its event, and it goes away by itself.
func TestPhoneBanner(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "")
	g.Handle(event.PhoneEffect{Phone: data.PhoneCue{App: "banner", Notices: []data.PhoneNotice{
		{App: "Chat", Title: "Ann", Text: "Lunch?", Event: "nope"}, {App: "Mail", Title: "Bob"},
	}}})
	if g.phone != nil || len(g.banners) != 2 {
		t.Fatalf("the banner brought a phone up or was not queued: %d", len(g.banners))
	}
	for range bannerSlide + 5 {
		g.in = input{}
		g.updateBanners()
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawBanners(dst)
	sw, _ := g.screen()
	g.in = input{click: true, mouse: image.Pt(int(sw/2), int(g.banners[0].top()+20))}
	g.updateBanners()
	if g.in.click || g.banners[0].n.Event != "" {
		t.Fatal("a click on the banner was not taken to open its event")
	}
	for range bannerSlide * 2 {
		g.in = input{}
		g.updateBanners()
	}
	if len(g.banners) != 1 || g.banners[0].n.Title != "Bob" {
		t.Fatalf("the clicked banner did not go up for the next: %d left", len(g.banners))
	}
	for range bannerSlide*2 + bannerStay {
		g.updateBanners()
	}
	if len(g.banners) != 0 {
		t.Fatal("the last banner did not go away by itself")
	}
}

// TestPhonePageKeys: the keys reach the page's buttons: the first OK rings the
// button in view, right and Tab move the ring, left wraps, and OK presses the
// ringed button with its result.
func TestPhonePageKeys(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "page", Var: "var.sso", Rows: []data.PhoneRow{
		{Text: "Approve sign-in?"}, {Button: "Approve"}, {Button: "Deny"},
	}})
	p := g.phone
	phoneTicks(g, phoneRise)
	phonePress(g, btnOK)
	if p.closing != 0 || p.focus != 1 || !p.keyed {
		t.Fatalf("the first OK: focus %d keyed %v closing %d", p.focus, p.keyed, p.closing)
	}
	phonePress(g, btnNext)
	if p.focus != 2 {
		t.Fatalf("Tab moved the ring to %d, want 2", p.focus)
	}
	phonePress(g, btnRight)
	if p.focus != 1 {
		t.Fatalf("right from the last button went to %d, want 1", p.focus)
	}
	phonePress(g, btnLeft)
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phonePress(g, btnOK)
	if p.closing == 0 || p.result != 1 {
		t.Fatalf("OK on Deny put the phone down with %d (closing %d)", p.result, p.closing)
	}
}

// TestPhonePageKeysScroll: a button off the screen is scrolled to when the
// keys move the ring onto it.
func TestPhonePageKeysScroll(t *testing.T) {
	rows := []data.PhoneRow{{Button: "Top"}}
	for range 12 {
		rows = append(rows, data.PhoneRow{Item: &data.PhoneItem{Title: "Row"}})
	}
	rows = append(rows, data.PhoneRow{Button: "Bottom"})
	g := phoneGame(t, data.PhoneCue{App: "page", Rows: rows})
	p := g.phone
	phoneTicks(g, phoneRise)
	phonePress(g, btnLeft)
	phoneTicks(g, 40)
	laid, _, _ := g.pageView(p)
	if p.focus != len(rows)-1 || !g.pageInView(p, laid[p.focus]) {
		t.Fatalf("left did not ring and show the last button: focus %d scroll %v", p.focus, p.scroll)
	}
	phonePress(g, btnUp)
	phoneTicks(g, 40)
	if p.focus >= 0 && !g.pageInView(p, laid[p.focus]) {
		t.Fatal("scrolling up left the ring on a button off the screen")
	}
}

// TestPhoneHidesHUD: the place's header, gold and party hide while the phone
// is up and come back when it is put down.
func TestPhoneHidesHUD(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hi"}}})
	if g.placeHUDShown() {
		t.Fatal("the place's HUD shows behind the phone")
	}
	g.phone.close()
	g.phone = nil
	if !g.placeHUDShown() {
		t.Fatal("the place's HUD did not come back")
	}
}

// TestPhoneSizeOnItsSide: on its side the phone is nearly as wide as the
// screen and its screen is 16:9, on a 1280x720 and a 1920x1080 layout alike.
func TestPhoneSizeOnItsSide(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "video", Video: &data.PhoneVideo{Picture: "x.png"}})
	view, ui := g.view, g.ui
	defer func() { g.view, g.ui = view, ui }()
	for _, v := range []gameView{{w: 1280, h: 720, ui: 1}, {w: 1920, h: 1080, ui: 1.5}, {w: 1920, h: 1080, ui: 1}} {
		g.view, g.ui = v, v.ui
		sw, sh := g.screen()
		w, h := g.phoneSize(true)
		_, _, scw, sch := g.phoneScreenRect(true)
		if w/sw < 0.88 || w > sw*0.94+0.01 || h > sh*0.93+0.01 {
			t.Errorf("%dx%d: the phone on its side is %.0fx%.0f of %.0fx%.0f", v.w, v.h, w, h, sw, sh)
		}
		if a := scw / sch; math.Abs(a-16.0/9) > 0.01 {
			t.Errorf("%dx%d: its screen is %.3f:1, want 16:9", v.w, v.h, a)
		}
		uw, uh := g.phoneSize(false)
		if math.Abs(uh/sh-0.94) > 1e-9 || math.Abs(uw/uh-0.49) > 1e-9 {
			t.Errorf("%dx%d: upright %.0fx%.0f", v.w, v.h, uw, uh)
		}
	}
}

// TestPhoneFeedLikeScripted: a capture script's click on a post's picture, a
// click on its heart and a short drag on it each like or unlike the post.
func TestPhoneFeedLikeScripted(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{{Picture: "x.png", Likes: 3}, {Picture: "x.png"}}})
	p := g.phone
	phoneTicks(g, phoneRise)
	posts, _, _, _ := g.feedView(p)
	sx, sy, w, _ := g.phoneScreenRect(false)
	x := int(sx + w/2)
	onPicture := int(sy + phoneFeedTop + posts[0].pic + 60)
	onHeart := int(sy + phoneFeedTop + posts[0].pic + posts[0].picH + 20)
	run := func(src string) {
		t.Helper()
		sc, err := parseScript(src + "\nwait 20\nquit\n")
		if err != nil {
			t.Fatal(err)
		}
		g.in.script = sc
		for range 12 {
			g.in.update()
			g.updatePhone()
		}
		g.in.script = nil
	}
	run(fmt.Sprintf("click %d %d", x, onPicture))
	if !p.liked[0] {
		t.Fatal("a scripted click on the picture did not like the post")
	}
	run(fmt.Sprintf("click %d %d", x, onHeart))
	if p.liked[0] {
		t.Fatal("a scripted click on the heart did not take the like back")
	}
	run(fmt.Sprintf("drag %d %d %d %d", x, onPicture, x, onPicture-3))
	if !p.liked[0] || p.scroll > 4 {
		t.Fatalf("a short scripted drag did not like the post (scroll %v)", p.scroll)
	}
}

// TestPhoneChatPicture: a picture's bubble takes the picture's shape, a press
// on the newest picture opens it over the screen, Cancel goes back, and a
// click on a picture opens it again; a day of its own comes without dots.
func TestPhoneChatPicture(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{
		{From: "Ann", Picture: "wide.png", Time: "9:41"}, {Day: "Today"}, {Mine: true, Text: "Nice", Read: true, Time: "9:42"},
	}})
	g.pics.imgs["wide.png"] = &picEntry{img: ebiten.NewImage(160, 90)}
	p := g.phone
	b := g.chatBubble(p, 0, 200)
	if math.Abs(b.bh/b.bw-9.0/16) > 1e-9 {
		t.Fatalf("the bubble is %vx%v, want the picture's 16:9", b.bw, b.bh)
	}
	phoneTicks(g, phoneRise)
	phonePress(g, btnOK)
	if p.viewPic != "wide.png" {
		t.Fatal("a press on the newest picture did not open it")
	}
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phonePress(g, btnCancel)
	if p.viewPic != "" || p.closing != 0 {
		t.Fatal("Cancel did not go back to the chat")
	}
	g.drawPhone(dst)
	r, ok := p.pics[0]
	if !ok {
		t.Fatal("the picture's place was not kept")
	}
	sx, sy, _, _ := g.phoneScreenRect(false)
	g.in = input{click: true, mouse: image.Pt(int(sx+r.x+r.w/2), int(sy+r.y+r.h/2))}
	g.updatePhone()
	if p.viewPic != "wide.png" {
		t.Fatal("a click on the picture did not open it")
	}
	phonePress(g, btnOK)
	phonePress(g, btnOK)
	if p.shown != 2 || p.typing != 0 {
		t.Fatalf("the day did not come at once: shown %d typing %d", p.shown, p.typing)
	}
	phonePress(g, btnOK)
	if p.shown != 3 {
		t.Fatalf("the owner's message did not come: %d", p.shown)
	}
	g.drawPhone(dst)
}

// TestPhoneInitial: an icon without a picture shows the name's first letter.
func TestPhoneInitial(t *testing.T) {
	for in, want := range map[string]string{"sam.travels": "S", "@lena_k": "L", "": "", "3am": "3"} {
		if got := nameInitial(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if nameColor("sam") != nameColor("sam") {
		t.Error("a name's color is not kept")
	}
}

// TestPhoneFramesFade: with fade the next frame fades in over the last fade
// seconds of each frame, and a clip stopping on its last frame fades no more.
func TestPhoneFramesFade(t *testing.T) {
	c := &phoneClip{v: &data.PhoneVideo{Frames: []string{"a", "b", "c"}, FPS: 1, Fade: 0.5, Pan: true}}
	for _, tc := range []struct {
		played    int
		cur, next string
		mix, into float64
	}{
		{0, "a", "b", 0, 0},
		{30, "a", "b", 0, 0.5},
		{45, "a", "b", 0.5, 0.75},
		{60, "b", "c", 0, 0},
		{105, "b", "c", 0.5, 0.75},
		{150, "c", "", 0, 0.5},
	} {
		c.played = tc.played
		cur, next, mix, into := c.frames()
		if cur != tc.cur || next != tc.next || math.Abs(mix-tc.mix) > 1e-9 || math.Abs(into-tc.into) > 1e-9 {
			t.Errorf("at %d: %q %q mix %v into %v, want %q %q %v %v", tc.played, cur, next, mix, into, tc.cur, tc.next, tc.mix, tc.into)
		}
	}
	c.loop, c.played = true, 165
	if cur, next, mix, _ := c.frames(); cur != "c" || next != "a" || math.Abs(mix-0.5) > 1e-9 {
		t.Errorf("a looping clip's last frame fades to %q from %q by %v", next, cur, mix)
	}
	g := phoneGame(t, data.PhoneCue{App: "video", Video: &data.PhoneVideo{Frames: []string{"x.png", "x.png"}, FPS: 2, Fade: 0.25, Pan: true}})
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	phoneTicks(g, phoneRise+20)
	g.drawPhone(dst)
}

// TestPhoneQR: a QR row's pixels are rsc.io/qr's modules, black on white with
// a quiet zone of four, and each module takes a whole number of pixels.
func TestPhoneQR(t *testing.T) {
	const url = "https://github.com/nao1215/gozen"
	pix, n, err := qrPixels(url)
	if err != nil {
		t.Fatal(err)
	}
	code, err := qr.Encode(url, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	if n != code.Size+2*qrQuiet {
		t.Fatalf("%d pixels a side for %d modules", n, code.Size)
	}
	for y := range n {
		for x := range n {
			black := pix[(y*n+x)*4] == 0
			want := code.Black(x-qrQuiet, y-qrQuiet)
			if black != want {
				t.Fatalf("module %d,%d: black %v, want %v", x, y, black, want)
			}
		}
	}
	if m := qrModule(330, n); m*n > 330 || (m+1)*n <= 330 {
		t.Fatalf("a module of %d pixels for %d modules in 330", m, n)
	}
	if _, _, err := qrPixels(strings.Repeat("x", 4000)); err == nil {
		t.Fatal("a text too long made a QR code")
	}
	g := phoneGame(t, data.PhoneCue{App: "page", Rows: []data.PhoneRow{{QR: url, Caption: "Scan to add Mia"}, {Button: "Add"}}})
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	phoneTicks(g, phoneRise)
	g.drawPhone(dst)
	if g.phone.qrs[url] == nil {
		t.Fatal("the QR code was not drawn")
	}
}

func TestPhoneFastForward(t *testing.T) {
	for _, mode := range []string{"cursor", "held skip"} {
		t.Run(mode, func(t *testing.T) {
			g := phoneGame(t, data.PhoneCue{App: "video", Video: &data.PhoneVideo{Picture: "missing.png"}})
			if mode == "cursor" {
				g.fast = &fastTo{lines: 2}
			} else {
				g.in.fast = true
			}
			for range 3 {
				fast := g.in.fast
				if g.updatePhone() {
					return
				}
				g.in.fast = fast
			}
			t.Fatal("presentation-only phone blocked fast-forward")
		})
	}
}

// BenchmarkPhoneDraw measures CPU-side drawing calls, not a displayed GPU frame.
func BenchmarkPhoneDraw(b *testing.B) {
	for _, tc := range []struct {
		name  string
		count int
		text  string
	}{
		{"chat-1", 1, "The blue tiles are ready for the courtyard."},
		{"chat-20", 20, "The blue tiles are ready for the courtyard."},
		{"chat-200", 200, "The blue tiles are ready for the courtyard."},
		{"long-message", 1, strings.Repeat("The blue tiles are ready.\n", 10000)},
	} {
		b.Run(tc.name, func(b *testing.B) {
			if err := gfx.LoadFonts(goregular.TTF, nil); err != nil {
				b.Fatal(err)
			}
			defer gfx.Reset()
			gfx.SetScale(1)
			g := &Game{db: testgame.Load(b, nil)}
			cue := data.PhoneCue{App: "chat", Title: "Workshop"}
			for i := range tc.count {
				cue.Messages = append(cue.Messages, data.PhoneMessage{From: "Maker", Text: fmt.Sprintf("Message %d: %s", i, tc.text)})
			}
			g.phone = newPhoneView(cue)
			defer g.closePhone()
			g.phone.shown, g.phone.age = tc.count, phoneRise
			dst := ebiten.NewImage(1280, 720)
			defer dst.Deallocate()
			g.drawPhone(dst)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				g.drawPhone(dst)
			}
		})
	}
}

func TestPhoneCanvasMinimumSizeIsReused(t *testing.T) {
	img := sized(nil, 0, -1)
	defer img.Deallocate()
	if next := sized(img, 0, -1); next != img {
		t.Fatal("an unchanged minimum-size canvas was reallocated")
	}
}

func TestPhoneClipSubframeDuration(t *testing.T) {
	c := &phoneClip{v: &data.PhoneVideo{Seconds: 0.001}}
	if c.length() != 1 {
		t.Fatalf("a positive clip must play at least one tick, got %d", c.length())
	}
}

// TestPhoneLongChatMustBeReadBeforeAdvancing: a message taller than the
// screen shows from its top, and presses read it down before the chat goes on.
func TestPhoneLongChatMustBeReadBeforeAdvancing(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{
		{Text: strings.Repeat("A tile for the courtyard. ", 100)}, {Mine: true, Text: "Next"},
	}})
	phoneTicks(g, phoneRise)
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phonePress(g, btnOK)
	if g.phone.shown != 1 || g.phone.closing != 0 {
		t.Fatal("the first press discarded the unread part of a long message")
	}
	for range 100 {
		phonePress(g, btnOK)
		g.drawPhone(dst)
		if g.phone.shown == 2 {
			return
		}
	}
	t.Fatal("the long message never reached its end")
}

func TestPhoneDrawReusesCanvasesAndBoundsLayouts(t *testing.T) {
	cue := data.PhoneCue{App: "chat", Title: "Workshop"}
	for range 200 {
		cue.Messages = append(cue.Messages, data.PhoneMessage{Text: "One tile", Mine: true})
	}
	cue.Messages[198] = data.PhoneMessage{Picture: "x.png"}
	g := phoneGame(t, cue)
	p := g.phone
	p.shown = len(cue.Messages)
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	canvas, screen := p.canvas, p.screen
	if len(p.bubbles) == 0 || len(p.bubbles) >= len(cue.Messages) {
		t.Fatalf("layout retained %d of %d messages", len(p.bubbles), len(cue.Messages))
	}
	g.drawPhone(dst)
	if p.canvas != canvas || p.screen != screen {
		t.Fatal("an unchanged draw replaced its canvases")
	}
	cue.Messages[199].Text = strings.Repeat("One tile ", 8)
	p.cue = cue
	clear(p.bubbles)
	wide := g.chatBubble(p, 199, 400)
	narrow := g.chatBubble(p, 199, 90)
	if narrow.bh <= wide.bh {
		t.Fatal("resize retained the old line wrapping")
	}
	p.turn(true)
	g.drawPhone(dst)
	if p.canvas == canvas || p.screen == screen {
		t.Fatal("turning the phone retained incorrectly sized canvases")
	}
}

func TestPhoneAutoChat(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{
		{Video: &data.PhoneVideo{Picture: "missing.png"}}, {Text: "Next"},
	}})
	g.autoRead = true
	if !phoneTicks(g, 600) {
		t.Fatal("auto reading did not finish the chat")
	}
	if g.phone.shown != 2 || g.phone.watch != nil {
		t.Fatal("auto reading played a video or lost a message")
	}
}

// phonePuppetGame is a game with a puppet for the phone's clips.
func phonePuppetGame(t *testing.T, rig string) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "", map[string]string{
		"assets/phone/rig.yaml": rig,
		"assets/phone/base.png": string(btlPNG(32, 32)),
	})
	t.Cleanup(g.closePhone)
	return g
}

func TestPhoneCloseReleasesOwnedReferences(t *testing.T) {
	g := phonePuppetGame(t, "base: base.png\nfaces: {normal: {}}\n")
	g.Handle(event.PhoneEffect{Phone: data.PhoneCue{App: "feed", Posts: []data.PhonePost{{Icon: "x.png", Video: &data.PhoneVideo{Puppet: "phone/rig.yaml", Seconds: 2, Bounce: []float64{0, 1}}}}}})
	p := g.phone
	phoneTicks(g, 70)
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	c := p.clips["post0"]
	if c == nil || c.body == nil || p.canvas == nil || p.screen == nil || len(p.icons) == 0 || len(p.frames) == 0 {
		t.Fatal("fixture did not allocate the phone's images and puppet")
	}
	g.closePhone()
	if g.phone != nil || p.canvas != nil || p.screen != nil || len(p.icons) != 0 || len(p.frames) != 0 || c.body != nil {
		t.Fatal("closed phone retained owned drawing resources")
	}
	p.close() // repeated cleanup is harmless
}

// TestPhoneVideoFramesStayBounded: scrolling down a long feed keeps only the
// canvases of the posts on the screen.
func TestPhoneVideoFramesStayBounded(t *testing.T) {
	cue := data.PhoneCue{App: "feed"}
	for range 20 {
		cue.Posts = append(cue.Posts, data.PhonePost{Icon: "x.png", Video: &data.PhoneVideo{Picture: "x.png"}})
	}
	g := phoneGame(t, cue)
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	posts, _, _, _ := g.feedView(g.phone)
	for i, l := range posts {
		g.phone.scroll = l.top
		g.drawPhone(dst)
		if len(g.phone.frames) > 3 {
			t.Fatalf("visiting post %d retained %d media canvases", i, len(g.phone.frames))
		}
	}
}

// TestPhoneFeedFollowsDrag: the feed follows the pointer, also on the tick it
// lets go with no tick held between, and such a drag is not a tap.
func TestPhoneFeedFollowsDrag(t *testing.T) {
	for _, tc := range []struct {
		name string
		held bool
	}{
		{"continuous", true},
		{"release-without-held-frame", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{{Picture: "x.png"}, {Picture: "x.png"}}})
			phoneTicks(g, phoneRise)
			_, sy, _, _ := g.phoneScreenRect(false)
			down := int(sy + 400)
			g.in = input{pdown: true}
			g.in.mouse = image.Pt(640, down)
			g.updatePhone()
			if tc.held {
				g.in = input{pheld: true}
				g.in.mouse = image.Pt(640, down-60)
				g.updatePhone()
				if g.phone.scroll != 60 {
					t.Fatalf("an upward drag scrolled the feed by %g, want 60", g.phone.scroll)
				}
			}
			g.in = input{pup: true}
			g.in.mouse = image.Pt(640, down-100)
			g.updatePhone()
			if g.phone.scroll < 100 || g.phone.liked[0] || g.phone.liked[1] {
				t.Fatalf("releasing a drag: scroll %g liked %v", g.phone.scroll, g.phone.liked)
			}
		})
	}
}

func TestPhoneClockLargeMinutes(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}}})
	for _, tc := range []struct {
		minutes float64
		want    string
	}{
		{0, "0:00"}, {1441, "0:01"}, {1e30, "12:16"}, {-1, "21:30"}, {math.Inf(1), "21:30"},
	} {
		g.st.Vars["clock_minutes"] = tc.minutes
		if got := g.phoneClock(); got != tc.want {
			t.Errorf("clock(%g) = %q, want %q", tc.minutes, got, tc.want)
		}
	}
}

func TestPhoneBounceAtStart(t *testing.T) {
	g := phonePuppetGame(t, "base: base.png\nsoft_sway:\n  - {ellipse: [4, 4, 20, 20], follow: body}\nfaces: {normal: {}}\n")
	jump := &phoneClip{v: &data.PhoneVideo{Puppet: "phone/rig.yaml", Bounce: []float64{0}}}
	still := &phoneClip{v: &data.PhoneVideo{Puppet: "phone/rig.yaml"}}
	for range 5 {
		jump.step(g, false)
		still.step(g, false)
	}
	if jump.body == nil || still.body == nil {
		t.Fatal("fixture did not load puppets")
	}
	defer jump.body.Close()
	defer still.body.Close()
	if jump.body.Motion.Pose() == still.body.Motion.Pose() {
		t.Fatal("a bounce at second zero never started")
	}
}

func TestPhoneModeShortcuts(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{{Text: "Hello"}}})
	phoneTicks(g, phoneRise)
	phonePress(g, btnAuto)
	if !g.autoRead {
		t.Fatal("Auto did not enable automatic reading on the phone")
	}
	phonePress(g, btnAuto)
	if g.autoRead {
		t.Fatal("Auto did not stop automatic reading on the phone")
	}
	phonePress(g, btnSkip)
	if !g.skipping || !phoneTicks(g, 3) {
		t.Fatal("Skip did not put the phone down")
	}
}

func TestPhoneFitsNarrowScreen(t *testing.T) {
	g := &Game{db: &data.DB{Game: data.Game{Screen: data.Screen{Width: 160, Height: 720}}}}
	for _, side := range []bool{false, true} {
		w, h := g.phoneSize(side)
		if w <= 0 || h <= 0 || w > 160*.94+1e-9 || h > 720*.94+1e-9 {
			t.Errorf("side=%v: phone %gx%g does not fit the screen", side, w, h)
		}
	}
}

// TestPhoneOffscreenPuppetsReleaseImagesAndKeepMotion: a post scrolled off
// the screen frees its puppet's images, and comes back in the same pose.
func TestPhoneOffscreenPuppetsReleaseImagesAndKeepMotion(t *testing.T) {
	g := phonePuppetGame(t, "base: base.png\nfaces: {normal: {}}\n")
	g.Handle(event.PhoneEffect{Phone: data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Video: &data.PhoneVideo{Puppet: "phone/rig.yaml"}},
		{Video: &data.PhoneVideo{Puppet: "phone/rig.yaml"}},
		{Video: &data.PhoneVideo{Puppet: "phone/rig.yaml"}},
		{Video: &data.PhoneVideo{Puppet: "phone/rig.yaml"}},
	}}})
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	phoneTicks(g, phoneRise+10)
	g.drawPhone(dst)
	first := g.phone.clips["post0"]
	if first == nil || first.body == nil {
		t.Fatal("first puppet was not drawn")
	}
	pose, played := first.body.Motion.Pose(), first.played
	posts, _, _, _ := g.feedView(g.phone)
	g.phone.scroll = posts[2].top
	g.drawPhone(dst)
	if first.body != nil {
		t.Fatal("offscreen clip retained its puppet's drawing resources")
	}
	g.phone.scroll = 0
	g.drawPhone(dst)
	if first.body == nil || first.body.Motion.Pose() != pose || first.played != played {
		t.Fatal("revisiting the clip reset its pose or playback position")
	}
}

// FuzzPhoneInputs checks the phone's state over any input, and that Cancel
// still puts it down.
func FuzzPhoneInputs(f *testing.F) {
	f.Add(uint8(0), []byte{0, 0, 0, 3})
	f.Add(uint8(1), []byte{1, 2, 0, 5, 3})
	f.Add(uint8(2), []byte{4, 0, 0, 3})
	f.Add(uint8(1), []byte{180, 96, 26, 3})
	f.Add(uint8(3), []byte{0, 1, 2, 0, 3})
	f.Fuzz(func(t *testing.T, app uint8, actions []byte) {
		if len(actions) > 1024 {
			return
		}
		video := &data.PhoneVideo{Picture: "missing.png", Seconds: 0.05}
		cues := []data.PhoneCue{
			{App: "chat", Messages: []data.PhoneMessage{{Video: video}, {Picture: "missing.png", Mine: true}}},
			{App: "feed", Posts: []data.PhonePost{{Video: video}, {Picture: "missing.png"}}},
			{App: "video", Hold: "landscape", Video: video},
			{App: "lock", Notices: []data.PhoneNotice{{Text: "One"}, {Text: "Two"}}},
		}
		g := &Game{db: &data.DB{Game: data.Game{Screen: data.Screen{Width: 1280, Height: 720}}}, pics: newPictures(fstest.MapFS{})}
		g.phone = newPhoneView(cues[int(app)%len(cues)])
		defer g.closePhone()
		buttons := []button{btnOK, btnUp, btnDown, btnCancel, btnAuto, btnSkip}
		tick := func() { g.updateTapOrPhone(event.Tap{}, func(int) {}) }
		for _, action := range actions {
			if g.phone == nil {
				break
			}
			g.in = input{}
			kind := int(action) % 9
			switch {
			case kind < len(buttons):
				g.in.pressed[buttons[kind]] = true
			case kind == 6:
				g.in.pdown = true
			case kind == 7:
				g.in.pheld = true
			case kind == 8:
				g.in.pup, g.in.click = true, true
			}
			g.in.mouse = image.Pt(int(action)*5, int(action)*3)
			tick()
			for range int(action) / 9 {
				if g.phone == nil {
					break
				}
				g.in = input{}
				tick()
			}
			if p := g.phone; p != nil {
				if p.typing < 0 || p.shown < 0 || p.shown > max(len(p.cue.Messages), len(p.cue.Notices)) || p.scroll < 0 || math.IsNaN(p.scroll) {
					t.Fatalf("invalid phone position: shown=%d typing=%d scroll=%g", p.shown, p.typing, p.scroll)
				}
				for _, c := range p.clips {
					if c.played < 0 || c.played > c.length() {
						t.Fatalf("invalid clip tick %d", c.played)
					}
				}
			}
		}
		// Cancel exits the player first, then the phone, whatever came before.
		g.skipping, g.autoRead = false, false
		for range 3 {
			for range phoneRise + phoneTurn + 1 {
				if g.phone == nil {
					return
				}
				g.in = input{}
				tick()
			}
			if g.phone == nil {
				return
			}
			g.in = input{pressed: [numButtons]bool{btnCancel: true}}
			tick()
		}
		for range phoneRise {
			if g.phone == nil {
				return
			}
			g.in = input{}
			tick()
		}
		if g.phone != nil {
			t.Fatal("Cancel left the phone open")
		}
	})
}
