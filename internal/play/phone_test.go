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
