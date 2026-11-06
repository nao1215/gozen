package play

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/nao1215/gozen/internal/layer5/gfx"
	"github.com/nao1215/gozen/internal/testgame"

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
func phonePress(g *Game, b button) {
	g.in = input{}
	g.in.pressed[b] = true
	g.updatePhone()
}

func phoneGame(t *testing.T, cue data.PhoneCue) *Game {
	t.Helper()
	g, _ := btlOpen(t, Options{Location: "camp"}, "ui: {message: novel}\n", "")
	g.Handle(event.PhoneEffect{Phone: cue})
	if g.phone == nil {
		t.Fatal("the phone effect did not bring the phone up")
	}
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
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phonePress(g, btnOK)
	g.drawPhone(dst)
	if p.shown != 1 {
		t.Fatalf("a press while the phone rose showed %d messages", p.shown)
	}
	phoneTicks(g, phoneRise)
	g.drawPhone(dst)
	phonePress(g, btnOK)
	g.drawPhone(dst)
	if p.shown != 2 || p.typing != 0 {
		t.Fatalf("the owner's message: shown %d typing %d", p.shown, p.typing)
	}
	phonePress(g, btnOK)
	g.drawPhone(dst)
	if p.shown != 2 || p.typing == 0 {
		t.Fatalf("another's message did not wait for the dots: shown %d typing %d", p.shown, p.typing)
	}
	phoneTicks(g, phoneTyping)
	g.drawPhone(dst)
	if p.shown != 3 {
		t.Fatalf("the dots did not end in the message: shown %d", p.shown)
	}
	phonePress(g, btnOK)
	g.drawPhone(dst)
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
	for _, tc := range []struct {
		name string
		wait int
		back button
	}{
		{"cancel", 30, btnCancel},
		{"completed", 120, btnOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := phoneGame(t, data.PhoneCue{App: "chat", Messages: []data.PhoneMessage{
				{From: "Ann", Video: &data.PhoneVideo{Picture: "x.png", Seconds: 2}}, {Mine: true, Text: "Nice"},
			}})
			p := g.phone
			dst := ebiten.NewImage(1280, 720)
			defer dst.Deallocate()
			g.drawPhone(dst)
			phoneTicks(g, phoneRise)
			g.drawPhone(dst)
			phonePress(g, btnOK)
			g.drawPhone(dst)
			if p.watch == nil || !p.side {
				t.Fatal("a press on the video did not open it on the phone's side")
			}
			phoneTicks(g, phoneTurn+tc.wait)
			g.drawPhone(dst)
			if p.watch.played == 0 {
				t.Fatal("the video did not play")
			}
			phonePress(g, tc.back)
			g.drawPhone(dst)
			if p.watch != nil || p.side || p.closing != 0 {
				t.Fatalf("finishing playback did not go back to the chat: watch %v side %v closing %d", p.watch != nil, p.side, p.closing)
			}
			phoneTicks(g, phoneTurn)
			g.drawPhone(dst)
			phonePress(g, btnOK)
			g.drawPhone(dst)
			if p.shown != 2 {
				t.Fatalf("the chat did not go on after the video: shown %d", p.shown)
			}
		})
	}
}

// TestPhoneFeed: left and right turn the posts within the feed, a click likes
// the post on the screen, and OK on the last post puts the phone down.
func TestPhoneFeed(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{
		{Picture: "x.png", Likes: 3}, {Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}},
	}})
	p := g.phone
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phoneTicks(g, phoneRise)
	g.drawPhone(dst)
	phonePress(g, btnLeft)
	g.drawPhone(dst)
	if p.post != 0 {
		t.Fatalf("left on the first post moved to %d", p.post)
	}
	phonePress(g, btnRight)
	g.drawPhone(dst)
	if p.post != 1 || p.slide == 0 {
		t.Fatalf("right did not slide to the next post: %d %v", p.post, p.slide)
	}
	phoneTicks(g, 90)
	g.drawPhone(dst)
	if c := p.clips["post1"]; c == nil || c.played == 0 || c.played > c.length() {
		t.Fatal("the post's video did not loop in its place")
	}
	phonePress(g, btnLeft)
	if p.post != 0 {
		t.Fatal("left did not return to the previous post")
	}
	phonePress(g, btnRight)
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
	g.drawPhone(dst)
	if p.closing == 0 {
		t.Fatal("OK on the last post did not put the phone down")
	}
}

// TestPhoneVideoLandscape: the video app comes up upright, turns on its side,
// plays to its end and is put down by a press there; a press before pauses it.
func TestPhoneVideoLandscape(t *testing.T) {
	g := phoneGame(t, data.PhoneCue{App: "video", Hold: "landscape", Video: &data.PhoneVideo{Picture: "x.png", Seconds: 1}})
	p := g.phone
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	g.drawPhone(dst)
	phoneTicks(g, phoneRise)
	g.drawPhone(dst)
	if !p.side || p.resting() {
		t.Fatal("the phone did not start turning on its side")
	}
	phoneTicks(g, phoneTurn)
	g.drawPhone(dst)
	phonePress(g, btnOK)
	g.drawPhone(dst)
	if !p.watch.paused {
		t.Fatal("a press did not pause the video")
	}
	phonePress(g, btnOK)
	g.drawPhone(dst)
	phoneTicks(g, 70)
	g.drawPhone(dst)
	if p.watch.played != p.watch.length() {
		t.Fatalf("the video did not stop at its end: %d of %d", p.watch.played, p.watch.length())
	}
	phonePress(g, btnOK)
	g.drawPhone(dst)
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
				if g.updatePhone() {
					return
				}
			}
			t.Fatal("presentation-only phone blocked fast-forward")
		})
	}
}

// BenchmarkPhoneDraw measures CPU-side drawing calls, not a displayed GPU frame.
// Use a fixed iteration budget because these calls queue graphics without a loop.
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
	next := sized(img, 0, -1)
	defer next.Deallocate()
	if next != img {
		t.Fatal("an unchanged minimum-size canvas was reallocated")
	}
}

func TestPhoneClipSubframeDuration(t *testing.T) {
	c := &phoneClip{v: &data.PhoneVideo{Seconds: 0.001}}
	if c.length() != 1 {
		t.Fatalf("a positive clip must play at least one tick, got %d", c.length())
	}
}

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
	wide := p.chatBubble(199, 400)
	narrow := p.chatBubble(199, 90)
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

func TestPhoneCloseReleasesOwnedReferences(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "", map[string]string{
		"assets/phone/rig.yaml": "base: base.png\nfaces: {normal: {}}\n",
		"assets/phone/base.png": string(btlPNG(32, 32)),
	})
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

func TestPhoneVideoFramesStayBounded(t *testing.T) {
	cue := data.PhoneCue{App: "feed"}
	for range 20 {
		cue.Posts = append(cue.Posts, data.PhonePost{Icon: "x.png", Video: &data.PhoneVideo{Picture: "x.png"}})
	}
	g := phoneGame(t, cue)
	dst := ebiten.NewImage(1280, 720)
	defer dst.Deallocate()
	for i := range cue.Posts {
		g.phone.post = i
		g.drawPhone(dst)
		if len(g.phone.frames) > 3 {
			t.Fatalf("visiting post %d retained %d media canvases", i, len(g.phone.frames))
		}
	}
}

func TestPhoneFeedFollowsDrag(t *testing.T) {
	for _, tc := range []struct {
		name string
		held bool
		move int
	}{
		{"continuous", true, 140},
		{"release-crosses-threshold", true, 190},
		{"release-without-held-frame", false, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := phoneGame(t, data.PhoneCue{App: "feed", Posts: []data.PhonePost{{Picture: "x.png"}, {Picture: "x.png"}}})
			phoneTicks(g, phoneRise)
			g.in = input{pdown: true}
			g.in.mouse.X = 200
			g.updatePhone()
			if tc.held {
				g.in = input{pheld: true}
				g.in.mouse.X = tc.move
				g.updatePhone()
				if got := g.phone.feedOffset(300); got != float64(tc.move-200) {
					t.Fatalf("a left drag moved the feed by %g, want %d", got, tc.move-200)
				}
			}
			g.in = input{pup: true}
			g.in.mouse.X = 140
			g.updatePhone()
			if g.phone.post != 1 || g.phone.drag != 0 {
				t.Fatal("releasing a left swipe did not advance one post")
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
		{0, "0:00"}, {1441, "0:01"}, {1e30, "12:16"}, {-1, "21:30"},
	} {
		g.st.Vars["clock_minutes"] = tc.minutes
		if got := g.phoneClock(); got != tc.want {
			t.Errorf("clock(%g) = %q, want %q", tc.minutes, got, tc.want)
		}
	}
}

func TestPhoneBounceAtStart(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "", map[string]string{
		"assets/phone/rig.yaml": "base: base.png\nsoft_sway:\n  - {ellipse: [4, 4, 20, 20], follow: body}\nfaces: {normal: {}}\n",
		"assets/phone/base.png": string(btlPNG(32, 32)),
	})
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

// FuzzPhoneInputs checks presentation state transitions without a display or assets.
func FuzzPhoneInputs(f *testing.F) {
	f.Add(uint8(0), []byte{0, 0, 0, 3})
	f.Add(uint8(1), []byte{1, 2, 0, 5, 3})
	f.Add(uint8(2), []byte{4, 0, 0, 3})
	f.Add(uint8(1), []byte{180, 96, 26, 3})
	f.Fuzz(func(t *testing.T, app uint8, actions []byte) {
		if len(actions) > 1024 {
			return
		}
		video := &data.PhoneVideo{Picture: "missing.png", Seconds: 0.05}
		cues := []data.PhoneCue{
			{App: "chat", Messages: []data.PhoneMessage{{Video: video}, {Picture: "missing.png", Mine: true}}},
			{App: "feed", Posts: []data.PhonePost{{Video: video}, {Picture: "missing.png"}}},
			{App: "video", Hold: "landscape", Video: video},
		}
		g := &Game{db: &data.DB{Game: data.Game{Screen: data.Screen{Width: 1280, Height: 720}}}}
		g.phone = newPhoneView(cues[app%3])
		defer g.closePhone()
		buttons := []button{btnOK, btnLeft, btnRight, btnCancel, btnAuto, btnSkip}
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
			g.in.mouse.X = int(action) * 5
			g.updateTap()
			for range int(action) / 9 {
				if g.phone == nil {
					break
				}
				g.in = input{}
				g.updateTap()
			}
			if p := g.phone; p != nil {
				if p.typing < 0 || p.shown < 0 || p.shown > len(p.cue.Messages) || p.post < 0 || (p.cue.App == "feed" && p.post >= len(p.cue.Posts)) {
					t.Fatalf("invalid phone position: shown=%d typing=%d post=%d", p.shown, p.typing, p.post)
				}
				for _, c := range p.clips {
					if c.played < 0 || c.played > c.length() {
						t.Fatalf("invalid clip tick %d", c.played)
					}
				}
			}
		}
		// Cancel exits the player first, then the chat, regardless of prior inputs.
		g.skipping, g.autoRead = false, false
		for range 2 {
			for range phoneRise + phoneTurn + 1 {
				if g.phone == nil {
					return
				}
				g.in = input{}
				g.updateTap()
			}
			if g.phone == nil {
				return
			}
			g.in = input{pressed: [numButtons]bool{btnCancel: true}}
			g.updateTap()
		}
		for range phoneRise {
			if g.phone == nil {
				return
			}
			g.in = input{}
			g.updateTap()
		}
		if g.phone != nil {
			t.Fatal("Cancel left the phone open")
		}
	})
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

func TestPhoneOffscreenPuppetsReleaseImagesAndKeepMotion(t *testing.T) {
	g, _ := btlOpen(t, Options{Location: "camp"}, "", "", map[string]string{
		"assets/phone/rig.yaml": "base: base.png\nfaces: {normal: {}}\n",
		"assets/phone/base.png": string(btlPNG(32, 32)),
	})
	g.Handle(event.PhoneEffect{Phone: data.PhoneCue{App: "feed", Posts: []data.PhonePost{
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
	g.phone.post = 1
	g.drawPhone(dst)
	if first.body != nil {
		t.Fatal("offscreen clip retained its puppet's drawing resources")
	}
	g.phone.post = 0
	g.drawPhone(dst)
	if first.body == nil || first.body.Motion.Pose() != pose || first.played != played {
		t.Fatal("revisiting the clip reset its pose or playback position")
	}
}
