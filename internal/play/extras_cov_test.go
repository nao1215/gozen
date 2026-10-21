package play

import (
	"image"
	"image/color"
	"io/fs"
	"os"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

// ---- notices (the notice command: a short message that does not stop play)

func TestWldNoticeBoardQueuesAndDeduplicates(t *testing.T) {
	var b noticeBoard
	b.add("Found a coin")
	b.add("Found a coin") // the same text refreshes, it is not shown twice
	if len(b.list) != 1 {
		t.Fatalf("%d notices for one text", len(b.list))
	}
	for i := range 20 {
		b.add(strings.Repeat("x", i+1))
	}
	if len(b.list) != noticeMax+noticeQueue {
		t.Fatalf("%d notices kept, want at most %d", len(b.list), noticeMax+noticeQueue)
	}
	if last := b.list[len(b.list)-1].text; last != strings.Repeat("x", 20) {
		t.Fatalf("the newest was dropped: %q", last)
	}
}

func TestWldNoticeLengthGrowsWithTheText(t *testing.T) {
	if noticeLen("hi") != noticeTicks {
		t.Fatalf("a short notice lasts %d", noticeLen("hi"))
	}
	long := strings.Repeat("あ", 60)
	if noticeLen(long) <= noticeTicks || noticeLen(long) != 60+60*6 {
		t.Fatalf("a long notice lasts %d", noticeLen(long))
	}
}

func TestWldNoticeOnlyTheFirstFewCountDown(t *testing.T) {
	var b noticeBoard
	for _, s := range []string{"a", "b", "c", "d"} {
		b.add(s)
	}
	for range noticeTicks - 1 {
		b.update()
	}
	if len(b.list) != 4 || b.list[3].t != noticeTicks {
		t.Fatalf("a queued notice aged before it showed: %+v", b.list)
	}
	b.update()
	if len(b.list) != 1 || b.list[0].text != "d" {
		t.Fatalf("after the first three ended: %+v", b.list)
	}
}

func TestWldNoticeCommandDoesNotStopTheEvent(t *testing.T) {
	g := wldBoxWith(t, "ev_notice")
	wldFrames(t, g, 3)
	if len(g.notices.list) != 2 {
		t.Fatalf("notices %+v", g.notices.list)
	}
	if !g.talk.active || g.talk.say.Text != "After the notices." {
		t.Fatalf("the event stopped at the notice: talk %+v", g.talk.say)
	}
	// a long one is drawn too (wrapped on the field's side column)
	g.notices.add(strings.Repeat("A very long notice that does not fit. ", 6))
	wldFrames(t, g, 3)
}

// wldBoxWith opens the message-box fixture with the extra events and starts one.
func wldBoxWith(t *testing.T, event string) *Game {
	t.Helper()
	opts := Options{Location: "camp", FS: wldWorldFiles(t, wldMeters, map[string]string{"data/novel_cov.yaml": wldNovelData})}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	wldStartEvent(t, g, event)
	return g
}

// ---- dream pictures (image look: dream)

func TestWldDreamPictureZoomsSlowlyAndKeepsItsHaze(t *testing.T) {
	g := wldBoxWith(t, "ev_dream")
	wldFrames(t, g, 3)
	if g.cg.File != "pics/bg.png" || g.cg.Look != "dream" {
		t.Fatalf("cg %+v", g.cg)
	}
	k0 := g.dreamScale("pics/bg.png")
	if k0 < 1 || k0 > 1.001 {
		t.Fatalf("the dream starts at scale %v", k0)
	}
	wldFrames(t, g, dreamTicks/2)
	mid := g.dreamScale("pics/bg.png")
	wldFrames(t, g, dreamTicks)
	end := g.dreamScale("pics/bg.png")
	if !(k0 < mid && mid < end) || end > 1+dreamZoom+1e-9 || end < 1+dreamZoom-0.001 {
		t.Fatalf("zoom %v -> %v -> %v: it should grow slowly to 1+%v and stay", k0, mid, end, dreamZoom)
	}
	if g.dreamScale("pics/other.png") != 1 {
		t.Fatal("a picture that is not a dream is scaled")
	}
	if len(g.dream.haze) != 1 || g.dream.mask == nil || g.dream.buf == nil {
		t.Fatalf("haze %d mask %v buf %v", len(g.dream.haze), g.dream.mask != nil, g.dream.buf != nil)
	}
	if !g.AsideBlocked() {
		t.Fatal("asides should not break into a picture that hides the portraits")
	}
}

func TestWldDreamBookkeepingStaysSmall(t *testing.T) {
	g := wldBoxWith(t, "ev_dream")
	wldFrames(t, g, 2)
	for i := range 70 {
		g.tick++
		g.dreamShown(data.CG{File: "p" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".png", Look: "dream"})
	}
	if len(g.dream.at) > 65 {
		t.Fatalf("%d pictures remembered", len(g.dream.at))
	}
	g.dreamShown(data.CG{File: "", Look: "dream"})
	if _, ok := g.dream.at[""]; ok {
		t.Fatal("an empty picture was remembered")
	}
}

// ---- moving portraits in the talk (the actor's puppet)

func wldPuppetFiles(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{"data/puppet_cov.yaml": `actors:
  streamer: {name: Streamer, class: ranger, faces: streamer, face: smile, puppet: streamer/puppet.yaml}
  broken: {name: Broken, class: ranger, faces: streamer, face: default, puppet: bad/puppet.yaml}
events:
  ev_puppet:
    do:
      - say: {who: streamer, face: smile, text: "Hello there, a line that takes a while to type out."}
      - say: {who: streamer, face: surprised, text: "Oh."}
      - effect: {name: nod, who: streamer}
      - effect: {name: nod, who: scout}
      - say: {who: broken, face: default, text: "A broken puppet."}
`}
}

func wldPuppetGame(t *testing.T) *Game {
	t.Helper()
	extra := wldPuppetFiles(t)
	src := os.DirFS("../../examples/puppet/assets/streamer")
	for _, n := range []string{"puppet.yaml", "streamer.png", "surprised.png", "eyes_closed.png", "mouth_closed.png", "mouth_half.png"} {
		b, err := fs.ReadFile(src, n)
		if err != nil {
			t.Fatal(err)
		}
		extra["assets/streamer/"+n] = string(b)
	}
	extra["assets/streamer/smile.png"] = extra["assets/streamer/streamer.png"]
	// a puppet whose picture cannot be read loads as YAML (so the data passes) and fails when made
	extra["assets/streamer/default.png"] = extra["assets/streamer/streamer.png"]
	extra["assets/bad/puppet.yaml"] = "base: base.png\n"
	extra["assets/bad/base.png"] = "not a picture"
	opts := Options{Location: "camp", FS: wldWorldFiles(t, "", extra)}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	wldStartEvent(t, g, "ev_puppet")
	wldFrames(t, g, 5)
	return g
}

func TestWldPuppetStartsOnFirstTalkAndFallsBack(t *testing.T) {
	covSlow(t)
	g := wldPuppetGame(t)
	p := g.puppets.live["streamer"]
	if p == nil {
		t.Fatal("the first line of an actor with a puppet did not make it")
	}
	if g.talk.say.Text != "Hello there, a line that takes a while to type out." {
		t.Fatalf("line %q", g.talk.say.Text)
	}
	if got, i := g.talkPuppet(g.talk.say); got != p || i != 0 {
		t.Fatalf("talkPuppet gave %p face %d", got, i)
	}
	wldFrames(t, g, 120)
	wldPlay(t, g, "press ok\nwait 60\n", 100)
	if g.talk.say.Text != "Oh." {
		t.Fatalf("line %q", g.talk.say.Text)
	}
	if _, i := g.talkPuppet(g.talk.say); i != 1 {
		t.Fatalf("surprised is the second face of the puppet, got %d", i)
	}
	// a reaction on the puppet moves it; on someone without one it does nothing
	before := p.Motion.Pose()
	g.react("streamer", "nod")
	for range 10 {
		p.Motion.Step()
	}
	if p.Motion.Pose() == before {
		t.Fatal("the nod did not move the portrait")
	}
	g.react("scout", "nod")
	g.react("streamer", "no such reaction")
	// a face the puppet does not have shows the picture as before
	lacking := g.talk.say
	lacking.Face = "streamer/angry.png"
	if got, _ := g.talkPuppet(lacking); got != nil {
		t.Fatal("a face the puppet lacks was drawn by the puppet")
	}
	lacking.Actor = ""
	if got, _ := g.talkPuppet(lacking); got != nil {
		t.Fatal("a speaker who is no actor got a puppet")
	}
	// a puppet that cannot be loaded is remembered as failed and the picture is shown
	wldPlay(t, g, "press ok\nwait 60\npress ok\nwait 60\n", 300)
	if g.talk.say.Text != "A broken puppet." || !g.puppets.failed["broken"] || g.puppets.live["broken"] != nil {
		t.Fatalf("line %q failed %v", g.talk.say.Text, g.puppets.failed)
	}
	g.closePuppets()
	if len(g.puppets.live) != 0 || len(g.puppets.failed) != 0 {
		t.Fatalf("puppets after closing: %+v", g.puppets)
	}
}

// ---- the dolls of a scene bake in the background; closing the game must not wait for them or
// race with them, and the next game starts clean

func TestWldClosingWhileDollsBakeIsSafe(t *testing.T) {
	covSlow(t)
	opts := Options{FS: wldWorldFiles(t, "", nil), Location: "fx_yard", NoSound: true, SaveDir: t.TempDir(), Calm: true}
	g, err := New(opts) // no script: the dolls bake while the frames go on
	if err != nil {
		t.Fatal(err)
	}
	screen := wldScreen(g)
	for range 3 {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
		g.Draw(screen)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	// the cache was dropped with the game; another game opens at once and shows its scene
	h := wldWorld(t, "fx_yard2", Options{Calm: true})
	wldFrames(t, h, 5)
	if _, ok := h.stack[0].(*stageScene); !ok {
		t.Fatalf("scene %T", h.stack[0])
	}
}

// ---- face icons shaded (actor face_shade, face_shade_top)

func TestWldShadeFaceDarkensFromTopToBottom(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 5))
	for y := range 5 {
		for x := range 4 {
			src.SetNRGBA(x, y, color.NRGBA{R: 200, G: 100, B: 50, A: 180})
		}
	}
	out := shadeFace(src, image.Rect(1, 0, 3, 5), 0.2, 0.8)
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 5 {
		t.Fatalf("bounds %v", out.Bounds())
	}
	top, bottom := out.NRGBAAt(0, 0), out.NRGBAAt(0, 4)
	if top.R != 160 || bottom.R != 40 || top.A != 180 || bottom.A != 180 {
		t.Fatalf("top %v bottom %v: 0.2 darker at the top, 0.8 at the bottom, alpha kept", top, bottom)
	}
	mid := out.NRGBAAt(1, 2)
	if mid.R != 100 || mid.G != 50 || mid.B != 25 {
		t.Fatalf("middle %v: half dark", mid)
	}
	// more than black is clamped
	if p := shadeFace(src, image.Rect(0, 0, 4, 5), 1.5, 2); p.NRGBAAt(0, 0).R != 0 {
		t.Fatalf("over-dark gave %v", p.NRGBAAt(0, 0))
	}
	// one row takes the top darkness
	if o := shadeFace(src, image.Rect(0, 0, 4, 1), 0.5, 1); o.NRGBAAt(0, 0).R != 100 {
		t.Fatalf("one row gave %v", o.NRGBAAt(0, 0))
	}
}

func TestWldShadedFaceIsMadeOnceAndOnlyForShadedActors(t *testing.T) {
	extra := map[string]string{
		"data/shade_cov.yaml":          "actors:\n  dim: {name: Dim, class: ranger, faces: pics/scout, face_shade: 0.5, face_shade_top: 0.2}\n",
		"assets/pics/scout/broken.png": "not a picture",
	}
	g := wldOpenWith(t, Options{Location: "camp", FS: wldWorldFiles(t, "", extra)})
	r := image.Rect(2, 2, 12, 12)
	a := g.pics.shadedFace("pics/scout/normal.png", r)
	if a == nil || a.Bounds().Dx() != 10 || a.Bounds().Dy() != 10 {
		t.Fatalf("shaded face %v", a)
	}
	if b := g.pics.shadedFace("pics/scout/normal.png", r); b != a {
		t.Fatal("the shaded face was made again")
	}
	if g.pics.shadedFace("pics/scout/normal.png", image.Rectangle{}) != nil {
		t.Fatal("an empty box was shaded")
	}
	if g.pics.shadedFace("normal.png", r) != nil {
		t.Fatal("a picture outside a faces folder was shaded")
	}
	if g.pics.shadedFace("pics/other/normal.png", r) != nil {
		t.Fatal("a folder without face_shade was shaded")
	}
	if g.pics.shadedFace("pics/scout/missing.png", r) != nil {
		t.Fatal("a missing picture gave a face")
	}
	if g.pics.shadedFace("pics/scout/broken.png", r) != nil {
		t.Fatal("a picture that cannot be read gave a face")
	}
}
