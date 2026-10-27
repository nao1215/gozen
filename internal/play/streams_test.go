package play

import (
	"cmp"
	"encoding/json"
	"fmt"
	"image"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nao1215/gozen/internal/chat"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/layer5/sound"
	"github.com/nao1215/gozen/internal/rpg"
	lang "github.com/nao1215/gozen/internal/text"
)

// streamsData is a neutral stream game: two channels of the example puppet, one short
// episode each and a duo, and a reaction book that answers comments and tips.
var streamsData = map[string]string{
	"data/streams_cast.yaml": `actors:
  ann: {name: Ann, face: smile, puppet: streamer/puppet.yaml}
  bo: {name: Bo, face: smile, puppet: streamer/puppet.yaml}
locations:
  booth: {name: Booth, stream: lobby, exits: [{to: camp, label: Camp}], actions: [{label: Settings, do: [{open: config}]}]}
`,
	"data/streams.yaml": `logo: x.png
timing: {say: 2, per_char: 0.01, chat: 0.3, ask: 1}
slot: {minutes: 0.01}   # one episode a stream
live: {share: 1, min_share: 1, duo_chance: DUO}
away: {per_hour: 100, cap_hours: 24, star_bonus: 0.5}
tips: [10, 50]
crowd: {names: [sprout, leaf], lines: [Hi!], tip_lines: [For seeds], rate: 2}
bounce: {on: "!$", amount: 1}
channels:
  - {id: ann, actor: ann, tag: Gardening, tagline: Seeds every week, color: "#2a8c4a", room: x.png, frame: {cx: 373, top: 40, bottom: 640}, episodes: [streams/one.yaml], first: ["Welcome, {from}!"], stayed: ["Thanks, {from}."]}
  - {id: bo, actor: bo, tag: Cooking, room: x.png, frame: {cx: 373, top: 40, bottom: 640}, episodes: [streams/one.yaml]}
duos:
  - {host: ann, with: bo, episodes: [streams/duo.yaml]}
`,
	"streams/one.yaml": `title: Planting day
lobby: [Beans today]
parts:
  - - say: Hello, everyone.
    - chat: [sprout, Beans!]
    - ask: {q: Which seeds?, options: [{c: Beans, r: ["{from} picked beans."]}, {c: Peas, r: [Peas it is.]}], npc: [[leaf, 1]]}
    - say: {face: surprised, text: That is all for today!}
`,
	"streams/duo.yaml": `title: Two of us
parts:
  - - say: We are two.
    - bo: {face: surprised, text: Hello from Bo!}
`,
	"assets/talk.yaml": `lexicon:
  plant: [beans, seeds]
people:
  - id: ann
    rules:
      - id: tip
        kind: tip
        say: ["Thank you, {from}!"]
      - id: plant
        when: tag.plant > 0
        say: [[Plants!, "You said {text}."]]
    dodge:
      unknown: [Hm.]
`,
}

// noonish is the clock of the stream tests.
var noonish = time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC)

// streamsGame opens the stream game at its lobby at a fixed time; duo is the chance of
// the duo.
func streamsGame(t *testing.T, duo string) *Game {
	t.Helper()
	g := wldOpenWith(t, Options{FS: streamsFS(t, duo), Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	return g
}

// streamsFS is the stream game's files.
func streamsFS(t *testing.T, duo string) fstest.MapFS {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 9\nwritten_in: en\nlanguages: [en, ja]\nstreams: data/streams.yaml\nreactions: {ann: talk.yaml}\n"), fsys["game.yaml"].Data...)}
	fsys["lang/ja.yaml"] = &fstest.MapFile{Data: []byte("text:\n  \"Hello, everyone.\": \"みなさん、こんにちは。\"\n  \"Which seeds?\": \"どの種？\"\n  \"That is all for today!\": \"今日はここまで！\"\n  \"Settings\": \"設定\"\n")}
	for n, s := range streamsData {
		fsys[n] = &fstest.MapFile{Data: []byte(strings.ReplaceAll(s, "DUO", duo))}
	}
	src := os.DirFS("../../examples/puppet/assets/streamer")
	for _, n := range []string{"puppet.yaml", "streamer.png", "surprised.png", "eyes_closed.png", "mouth_closed.png", "mouth_half.png"} {
		b, err := fs.ReadFile(src, n)
		if err != nil {
			t.Fatal(err)
		}
		fsys["assets/streamer/"+n] = &fstest.MapFile{Data: b}
	}
	return fsys
}

// chatText is the chat of the watch view, one "name: text" a message.
func chatText(w *streamWatch) string {
	var b strings.Builder
	for _, m := range w.chat.log.Recent(150) {
		b.WriteString(m.From + ": " + m.Text + "\n")
	}
	return b.String()
}

// TestStreamsWatchAShow: the lobby is the place's scene with every channel live on the
// first opening, asking the viewer's name over it; watching greets a first visit, posts the beats
// and the comments, answers a comment and a tip from the reaction book, and at the end
// thanks the viewer, moves the channel on to its next episode and keeps it in the save.
func TestStreamsWatchAShow(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok || len(l.cards) != 2 || len(l.entries(g)) == 0 {
		t.Fatalf("scene %T, cards %d", g.stack[0], len(l.cards))
	}
	if l.name == nil {
		t.Fatal("the first opening does not ask the viewer's name")
	}
	wldPlay(t, g, "press right\nwait 5\n", 100)
	if l.name == nil || l.sel != 0 {
		t.Fatal("the lobby takes keys before the name is given")
	}
	wldPlay(t, g, "type Pat\npress ok\nwait 5\n", 100)
	if l.name != nil || g.st.Streams.Viewer != "Pat" {
		t.Fatalf("the name box stays (%v), viewer %q", l.name != nil, g.st.Streams.Viewer)
	}
	l.sel = slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" }) // the cards go by viewers
	wldPlay(t, g, "press ok\nwait 5\n", 100)
	w := g.watching()
	if w == nil {
		t.Fatal("the lobby does not open the show")
	}
	wldPlay(t, g, "press tab\ntype I planted beans\npress ok\nwait 2\n", 100)
	wldFrames(t, g, 600)
	chat := chatText(w)
	for _, want := range []string{"Ann: Welcome, Pat!", "Pat: I planted beans", "Ann: Plants!", "Ann: You said I planted beans."} {
		if !strings.Contains(chat, want) {
			t.Errorf("chat lacks %q:\n%s", want, chat)
		}
	}
	gold := g.st.Gold
	w.tip(g, 10)
	wldFrames(t, g, 300)
	if g.st.Gold != gold-10 || !strings.Contains(chatText(w), "Ann: Thank you, Pat!") {
		t.Errorf("gold %d (was %d), chat:\n%s", g.st.Gold, gold, chatText(w))
	}
	w.choose(g, 0)
	wldFrames(t, g, 1800)
	if !w.ended || !w.seen || !w.thanked {
		t.Fatalf("ended %v seen %v thanked %v, chat:\n%s", w.ended, w.seen, w.thanked, chatText(w))
	}
	p := g.st.Streams.Channels["ann"]
	if p.Next != 1 || p.Points <= 0 || p.Run != 1 {
		t.Fatalf("progress %+v", p)
	}
	b, err := g.st.Save()
	if err != nil {
		t.Fatal(err)
	}
	st, err := rpg.Parse(g.db, b, nil)
	if err != nil || st.Streams.Viewer != "Pat" || st.Streams.Channels["ann"].Next != 1 {
		t.Fatalf("saved streams %+v %v", st.Streams, err)
	}
	wldPlay(t, g, "press ok\nwait 2\n", 10)
	if g.watching() != nil {
		t.Fatal("the end does not go back to the lobby")
	}
}

// TestStreamsDuoAndAway: two live hosts stream their duo together, the other host saying
// its lines; the gold earned while away is added and shown once.
// TestStreamsLobbyMostWatchedFirst: the lobby lists its cards by viewers, most first.
func TestStreamsLobbyMostWatchedFirst(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok || len(l.cards) < 2 {
		t.Fatalf("cards %+v", l.cards)
	}
	if !slices.IsSortedFunc(l.cards, func(a, b *lobbyCard) int { return cmp.Compare(b.viewers, a.viewers) }) {
		t.Fatalf("cards not by viewers: %d, %d", l.cards[0].viewers, l.cards[1].viewers)
	}
}

func TestStreamsDuoAndAway(t *testing.T) {
	g := streamsGame(t, "1")
	l, ok := g.stack[0].(*streamLobby)
	if !ok || len(l.cards) != 1 || len(l.cards[0].show.Hosts) != 2 || len(l.cards[0].bodies) != 2 {
		t.Fatalf("cards %+v", l.cards)
	}
	g.st.Streams.Viewer = "Pat"
	g.push(newStreamWatch(g, l.cards[0].show, 100))
	wldFrames(t, g, 400)
	if !strings.Contains(chatText(g.watching()), "Bo: Hello from Bo!") {
		t.Errorf("chat:\n%s", chatText(g.watching()))
	}
	g.watching().leave(g)
	gold := g.st.Gold
	g.st.Streams.Open = g.streamNow().Add(-2 * time.Hour).Unix()
	broadcast.Progress(g.st.Streams, "ann").Points = 1e9
	wldFrames(t, g, 2)
	if got := g.st.Gold - gold; got != 100*2*(1+0.5*4) || g.desk.away != got {
		t.Fatalf("away gold %d, shown %d", got, g.desk.away)
	}
	wldPlay(t, g, "press ok\nwait 2\n", 10)
	if g.desk.away != 0 {
		t.Fatal("the away gold is shown again")
	}
}

// TestStreamsReplayReadsTheRecordedClock: the trace records the time the run read, and a
// replay plays the streams at that time, not at its own, so it does not diverge.
func TestStreamsReplayReadsTheRecordedClock(t *testing.T) {
	covSlow(t)
	path := filepath.Join(t.TempDir(), "run.jsonl")
	f, err := os.Create(path) //nolint:gosec // a file in the test directory
	if err != nil {
		t.Fatal(err)
	}
	fsys := streamsFS(t, "0")
	g, err := New(Options{
		FS: fsys, NoSound: true, Seed: 3, SaveDir: t.TempDir(), ShotDir: t.TempDir(), Location: "booth", Trace: f,
		Script: "wait 30\npress ok\nwait 300\nquit", Now: func() time.Time { return noonish },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sysFrames(t, g, 3000) {
		t.Fatal("the script did not end")
	}
	sysClose(t, g)
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := New(Options{FS: fsys, NoSound: true, SaveDir: t.TempDir(), Replay: path, Now: func() time.Time { return noonish.Add(5 * time.Hour) }})
	if err != nil {
		t.Fatal(err)
	}
	defer sysClose(t, r)
	if !sysFrames(t, r, 5000) {
		t.Fatal("the replay did not end")
	}
	if r.rep.diverge != "" || !r.rep.finished || !r.desk.base.Equal(noonish) {
		t.Fatalf("diverged %q, finished %v, clock %v", r.rep.diverge, r.rep.finished, r.desk.base)
	}
}

// TestStreamView: a window taller than wide takes the portrait layout at a unit to a pixel
// (narrow phones a little smaller, tablets larger), a phone on its side a UI of its shape
// at 0.9 pixels a unit, and a window where the game's screen draws its units large enough
// keeps the game's screen; the pixels follow the device's, at most three to a unit.
func TestStreamView(t *testing.T) {
	for _, tc := range []struct {
		ow, oh, dsf float64
		want        gameView
	}{
		{390, 844, 3, gameView{w: 1170, h: 2532, ui: 3, tall: true}},
		{390, 844, 1, gameView{w: 390, h: 844, ui: 1, tall: true}},
		{320, 640, 2, gameView{w: 640, h: 1280, ui: 320.0 / 360 * 2, tall: true}},
		{1200, 1600, 2, gameView{w: 1800, h: 2400, ui: 3, tall: true}},
		{844, 390, 3, gameView{w: 2532, h: 1170, ui: 2.7}},
		{640, 360, 1, gameView{w: 640, h: 360, ui: 640.0 / 760}},
		{1920, 1080, 1, gameView{}},
		{1366, 650, 2, gameView{}},
		{0, 844, 1, gameView{}},
	} {
		if got := streamView(tc.ow, tc.oh, tc.dsf, 1280, 720); got != tc.want {
			t.Errorf("%vx%v at %v: %+v, want %+v", tc.ow, tc.oh, tc.dsf, got, tc.want)
		}
	}
}

// TestStreamsPortrait: on a phone held upright the lobby stands the cards one under another
// across the screen, a finger swiping scrolls them without its lifting opening one, and the
// watch view has the chat under the stage, scrolled back by a finger and kept in place as
// messages come, and starts typing when a finger touches the comment field. A screen over the
// streams keeps the game's size, and so does a phone on its side its landscape layout.
func TestStreamsPortrait(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	g.st.Streams.Viewer, l.name = "Pat", nil
	g.fitView(390, 500, 3)
	if sw, sh := g.screen(); !g.view.tall || g.ui != 3 || g.in.ui != 3 || sw != 390 || sh != 500 {
		t.Fatalf("view %+v, screen %vx%v", g.view, sw, sh)
	}
	x0, y0, w0, h0 := l.cardRect(g, 0)
	x1, y1, w1, _ := l.cardRect(g, 1)
	if x0 != tallEdge || x1 != x0 || w0 != 390-2*tallEdge || w1 != w0 || y1 != y0+h0+tallNames || g.beside(0) {
		t.Fatalf("cards %v,%v %v wide, %v,%v %v wide", x0, y0, w0, x1, y1, w1)
	}
	g.in.touching, g.in.swiped, g.in.swipe = true, true, -400
	l.update(g)
	if want := l.end(g) - 500; l.scroll != want || want <= 0 {
		t.Fatalf("swiped to %v, want the end %v", l.scroll, want)
	}
	g.in.touching, g.in.swipe, l.scroll = false, 0, 0
	l.fling.v = -20
	l.update(g)
	if l.scroll <= 0 || l.fling.v >= 0 || l.fling.v <= -20 {
		t.Fatalf("a lifted finger does not glide on, slowing: scroll %v, speed %v", l.scroll, l.fling.v)
	}
	l.fling.v = 0
	_, y, _, _ := l.shown(g, 1)
	g.in.touching, g.in.swipe, g.in.click, g.in.mouse = false, 0, true, image.Pt(100, int(y)+20)
	l.update(g)
	if g.watching() != nil {
		t.Fatal("a swipe's lifting opens a card")
	}
	if c := l.cards[0]; c.line == "" || c.line != lineAt(c.show, c.show.BeatAt(g.streamNow())) {
		t.Fatalf("the card's caption %q is not the show's line now", c.line)
	}
	g.in.swiped = false
	l.update(g)
	w := g.watching()
	if w == nil || w.show != l.cards[1].show {
		t.Fatal("a tap does not open the card")
	}
	g.in.click = false
	g.fitView(390, 844, 3)
	if lay := g.watchLayout(); lay.stageW != 390 || lay.colX != 0 || lay.colW != 390 || lay.colTop != streamBarH+390.0*9/16+tallInfoH {
		t.Fatalf("layout %+v", lay)
	}
	for i := range 40 {
		w.chat.post(chat.Message{From: "leaf", Text: fmt.Sprintf("line %d", i)})
	}
	_, _, cw, h := w.chatRect(g)
	g.in.touching, g.in.swiped, g.in.swipe, g.in.touchFrom = true, true, 60, image.Pt(100, int(g.watchLayout().colTop)+80)
	w.input(g)
	if w.chat.back != 60 || w.chat.width != cw {
		t.Fatalf("chat scrolled back %v, %v wide", w.chat.back, w.chat.width)
	}
	w.chat.post(chat.Message{From: "leaf", Text: "new"})
	if w.chat.back <= 60 {
		t.Fatal("a new message moves the chat scrolled back")
	}
	w.chat.scroll(1e9, h)
	if w.chat.back >= 1e9 || w.chat.back <= 0 {
		t.Fatalf("the chat scrolls back past its oldest message: %v", w.chat.back)
	}
	x, cy, _, _ := w.controlRect(g, [2]int{int(ctlComment), 0})
	g.in.swiped, g.in.began, g.in.touchFrom = false, true, image.Pt(int(x)+20, int(cy)+10)
	w.input(g)
	if !w.typing {
		t.Fatal("a finger on the comment field does not start typing")
	}
	g.in.touching, g.in.began = false, false
	g.fitView(844, 390, 3)
	if sw, sh := g.screen(); g.view.tall || math.Abs(sw-844/0.9) > 1 || math.Abs(sh-390/0.9) > 1 {
		t.Fatalf("on its side: view %+v, screen %vx%v", g.view, sw, sh)
	}
	g.push(newMenu(g))
	g.fitView(390, 844, 3)
	if g.view != (gameView{}) || g.ui != UIScale(&g.db.Game) || g.in.ui != g.ui {
		t.Fatalf("a menu over the streams is drawn at %+v, ui %v", g.view, g.ui)
	}
}

// TestStreamsLineAt: a card's caption is the last line or question up to the beat, kept over
// the chat beats after it, and none before the first line.
func TestStreamsLineAt(t *testing.T) {
	s := &broadcast.Show{Beats: []data.Beat{
		{Chat: []string{"leaf", "hi"}},
		{Line: &data.StreamLine{Text: "Hello."}},
		{Chat: []string{"leaf", "hi"}},
		{Ask: &data.StreamAsk{Q: "Which?"}},
	}}
	for b, want := range []string{"", "Hello.", "Hello.", "Which?"} {
		if got := lineAt(s, b); got != want {
			t.Errorf("beat %d: %q, want %q", b, got, want)
		}
	}
}

// TestStreamsBarLanguageAndSettings: the bar shows the languages side by side with the one
// shown filled, then the place's entries, the settings' with a gear; a language's button
// shows the streams in it and keeps it in the settings, and the settings opened from the bar
// are a page of the stream site, in the stream screens' size, whose bar goes back.
func TestStreamsBarLanguageAndSettings(t *testing.T) {
	g := streamsGame(t, "0")
	t.Cleanup(func() {
		if err := lang.Use(lang.English, nil); err != nil {
			t.Error(err)
		}
	})
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	g.st.Streams.Viewer, l.name = "Pat", nil
	g.fitView(390, 844, 1)
	bs := g.barButtons(l.entries(g))
	if len(bs) != 5 || !bs[0].gear || bs[0].label != "Settings" || bs[1].gear || bs[3].lang != "en" || bs[4].label != "日本語" || bs[4].x != bs[3].x+bs[3].w+2 || bs[4].y != bs[3].y {
		t.Fatalf("bar buttons %+v", bs)
	}
	for _, b := range bs { // two rows on a narrow screen, inside it
		if sw, _ := g.screen(); b.x < 16 || b.x+b.w > sw-16 || b.y != barButtonY && b.y != streamBarH+barButtonY-6 {
			t.Fatalf("the buttons leave the bar: %+v", bs)
		}
	}
	l.update(g)
	if _, y, _, _ := l.cardRect(g, 0); barHeight(bs) == streamBarH || y != barHeight(bs)+16 {
		t.Fatalf("the cards start at %v under a bar of two rows %v", y, barHeight(bs))
	}
	g.in.click, g.in.mouse = true, image.Pt(int(bs[4].x+5), int(bs[4].y+10))
	l.update(g)
	g.in.click = false
	if after := g.barButtons(l.entries(g)); after[3].x != bs[3].x || after[4].x != bs[4].x {
		t.Fatalf("the languages move when switched: %+v, then %+v", bs[3:], after[3:])
	}
	if g.language() != "ja" || g.cfg.Language != "ja" || g.db.Tr("Hello, everyone.") != "みなさん、こんにちは。" {
		t.Fatalf("language %q, settings %q", g.language(), g.cfg.Language)
	}
	if b, err := g.storeRead(g.configPath()); err != nil || !strings.Contains(string(b), `"language": "ja"`) {
		t.Fatalf("saved settings %s, %v", b, err)
	}
	bs = g.barButtons(l.entries(g))
	g.in.click, g.in.mouse = true, image.Pt(int(bs[0].x+5), int(bs[0].y+10))
	l.update(g)
	g.in.click = false
	wldFrames(t, g, 5)
	c, ok := g.stack[len(g.stack)-1].(*configScreen)
	if !ok || !c.streams {
		t.Fatalf("the settings entry opens %T", g.stack[len(g.stack)-1])
	}
	g.fitView(390, 844, 1)
	c.place(g)
	if !g.view.tall || c.list.w != 390-48 || c.list.y != streamBarH+80 {
		t.Fatalf("the settings over the streams: view %+v, list %v wide at %v", g.view, c.list.w, c.list.y)
	}
	g.in.click, g.in.mouse = true, image.Pt(30, 20)
	c.update(g)
	g.in.click = false
	if _, ok := g.stack[len(g.stack)-1].(*streamLobby); !ok {
		t.Fatalf("the bar's way back leads to %T", g.stack[len(g.stack)-1])
	}
}

// voicedShow opens the stream game with recorded voices for the lines of ann named in
// voiced (from internal/play/testdata/tone.ogg) and watches ann's show from its start.
func voicedShow(t *testing.T, voiced ...string) (*Game, *streamWatch, fstest.MapFS) {
	t.Helper()
	fsys := streamsFS(t, "0")
	tone, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	fsys["data/streams.yaml"].Data = append([]byte("voices: voice\n"), fsys["data/streams.yaml"].Data...)
	for _, l := range voiced {
		fsys["assets/voice/ann/"+data.VoiceKey(l)+".ogg"] = &fstest.MapFile{Data: tone}
	}
	fsys["assets/voice/bo/.keep"] = &fstest.MapFile{} // a folder without voices
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	g.st.Streams.Viewer = "Pat"
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	i := slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })
	show := l.cards[i].show
	show.Start = g.streamNow()
	w := newStreamWatch(g, show, 100)
	g.push(w)
	return g, w, fsys
}

// TestStreamsVoiceWaitsForTheLine: a game with recorded voices reads its lines aloud from
// the start, with the read-aloud button even outside a browser; a line with its voice waits
// for the voice to end before the next step, and a line without one lasts its timing.
func TestStreamsVoiceWaitsForTheLine(t *testing.T) {
	g, w, fsys := voicedShow(t, "Hello, everyone.")
	if !g.desk.speak || !slices.Contains(w.controls(g), [2]int{int(ctlSpeak), 0}) {
		t.Fatalf("voices do not read aloud (%v) or show the button: %v", g.desk.speak, w.controls(g))
	}
	wait := func(line string) time.Time {
		for range 2000 {
			if w.caption == line {
				return g.streamNow()
			}
			wldFrames(t, g, 1)
		}
		t.Fatalf("%q is never said (caption %q)", line, w.caption)
		return time.Time{}
	}
	at := wait("Welcome, Pat!")
	if want := at.Add(seconds(g.db.Streams.Timing.LineSeconds("Welcome, Pat!"))); !w.busy.Equal(want) {
		t.Errorf("a line without its voice lasts until %v, not its timing %v", w.busy, want)
	}
	at = wait("Hello, everyone.")
	pcm, err := sound.VoiceClip(fsys, g.db.Streams.Voice("ann", "Hello, everyone."))
	if err != nil {
		t.Fatal(err)
	}
	clip := seconds(sound.ClipSeconds(pcm))
	if want := at.Add(clip + voiceGap); !w.busy.Equal(want) || clip == seconds(g.db.Streams.Timing.LineSeconds("Hello, everyone.")) {
		t.Errorf("the voiced line waits until %v, not the voice's end %v (a clip of %v)", w.busy, want, clip)
	}
}

// TestStreamsNoVoicesNoButton: outside a browser, a game without recorded voices has no
// read-aloud button.
func TestStreamsNoVoicesNoButton(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	w := newStreamWatch(g, l.cards[0].show, 100)
	if g.desk.speak || slices.Contains(w.controls(g), [2]int{int(ctlSpeak), 0}) {
		t.Fatalf("a game without voices reads aloud (%v) or shows the button: %v", g.desk.speak, w.controls(g))
	}
}

// TestStreamsChatMemory: a reaction book remembers what a viewer said and brings it up on
// a later visit, its rules read the visits and the hour of a comment (value.visits,
// value.hour), and a comment it found nothing in is read back two beats later.
func TestStreamsChatMemory(t *testing.T) {
	fsys := streamsFS(t, "0")
	fsys["data/streams.yaml"].Data = append([]byte("echo: {max_runes: 24, chance: 1}\n"), fsys["data/streams.yaml"].Data...)
	fsys["assets/talk.yaml"] = &fstest.MapFile{Data: []byte(`lexicon:
  snack: [cookies, chips]
people:
  - id: ann
    rules:
      - id: night
        when: value.hour >= 22 || value.hour < 5
        say: [Up late?]
      - id: snack
        when: tag.snack > 0
        say: ["{slot.snack}, nice."]
        remember: [{key: snack, set: "{slot.snack}"}]
      - id: regular
        when: value.visits >= 2 && mem.snack != 0 && length > 6
        say: ["{mem.snack} fan {from}, welcome back."]
    dodge:
      unknown: ["Hm, {text}."]
`)}
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	g.st.Streams.Viewer = "Pat"
	watch := func() *streamWatch {
		l, ok := g.stack[0].(*streamLobby)
		if !ok {
			t.Fatalf("scene %T", g.stack[0])
		}
		show := l.cards[slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })].show
		show.Start = g.streamNow()
		w := newStreamWatch(g, show, 100)
		g.push(w)
		return w
	}
	until := func(w *streamWatch, done func() bool) {
		for range 3000 {
			if done() {
				return
			}
			wldFrames(t, g, 1)
		}
		t.Fatalf("never happened; chat:\n%s", chatText(w))
	}
	w := watch()
	w.comments = append(w.comments, "I love cookies")
	until(w, func() bool { return strings.Contains(chatText(w), "Ann: cookies, nice.") })
	w.comments = append(w.comments, "zzz")
	until(w, func() bool { return len(w.comments) == 0 })
	if len(w.later) != 1 || w.later[0].at <= w.next || strings.Contains(chatText(w), "Hm, zzz.") {
		t.Fatalf("an unknown comment is not put off: later %+v at beat %d", w.later, w.next)
	}
	due := w.later[0].at
	until(w, func() bool { return strings.Contains(chatText(w), "Ann: Hm, zzz.") })
	if w.next < due {
		t.Errorf("an unknown comment is read back at beat %d, before %d", w.next, due)
	}
	var noted []unanswered
	if b, err := g.store.Read(unansweredFile); err != nil || json.Unmarshal(b, &noted) != nil || len(noted) != 1 || noted[0].Text != "zzz" || noted[0].Channel != "ann" || noted[0].Language != "en" {
		t.Errorf("the comment found nothing in is not kept alone: %+v %v", noted, err)
	}
	w.leave(g)
	w = watch()
	w.comments = append(w.comments, "hello there")
	until(w, func() bool { return strings.Contains(chatText(w), "Ann: cookies fan Pat, welcome back.") })
}

// TestStreamsUnansweredKeepsTheLast: the file of unanswered comments keeps the last 200,
// and a damaged file starts again.
func TestStreamsUnansweredKeepsTheLast(t *testing.T) {
	g := streamsGame(t, "0")
	if err := g.store.Write(unansweredFile, []byte("{damaged")); err != nil {
		t.Fatal(err)
	}
	for i := range unansweredMax + 5 {
		g.noteUnanswered("ann", fmt.Sprint("comment ", i))
	}
	var noted []unanswered
	b, err := g.store.Read(unansweredFile)
	if err != nil || json.Unmarshal(b, &noted) != nil {
		t.Fatalf("%s: %v", b, err)
	}
	if len(noted) != unansweredMax || noted[0].Text != "comment 5" || noted[len(noted)-1].Text != fmt.Sprint("comment ", unansweredMax+4) || noted[0].At != noonish.Format(time.RFC3339) {
		t.Fatalf("kept %d, from %+v to %+v", len(noted), noted[0], noted[len(noted)-1])
	}
}

// TestStreamsSlot: a stream plays its episodes one after another with the host's bridge
// between them until its slot has passed, marks each episode seen as it is watched, shows
// the title of the episode playing, and thanks the viewer once at the end of the slot.
func TestStreamsSlot(t *testing.T) {
	fsys := streamsFS(t, "0")
	y := string(fsys["data/streams.yaml"].Data)
	y = strings.Replace(y, "slot: {minutes: 0.01}", "slot: {minutes: 0.2}", 1)
	y = strings.Replace(y, "first: [", "bridges: [Water break.], first: [", 1)
	fsys["data/streams.yaml"].Data = []byte(y)
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	g.st.Streams.Viewer = "Pat"
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	show := l.cards[slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })].show
	if len(show.Episodes) != 2 {
		t.Fatalf("a slot of 12 s has %d episodes of about 10 s", len(show.Episodes))
	}
	show.Start = g.streamNow()
	w := newStreamWatch(g, show, 100)
	g.push(w)
	p := g.st.Streams.Channels["ann"]
	for range 3000 {
		if w.part == 1 {
			break
		}
		wldFrames(t, g, 1)
	}
	if w.part != 1 || p.Next != 1 {
		t.Fatalf("the first episode is not seen before the second (part %d, next %d)", w.part, p.Next)
	}
	for range 3000 {
		if w.ended {
			break
		}
		wldFrames(t, g, 1)
	}
	chat := chatText(w)
	if !w.ended || p.Next != 2 || strings.Count(chat, "Ann: Water break.") != 1 || strings.Count(chat, "Ann: Thanks, Pat.") != 1 || strings.Count(chat, "Ann: Hello, everyone.") != 2 {
		t.Fatalf("ended %v, next %d, chat:\n%s", w.ended, p.Next, chat)
	}
	if bridge := strings.Index(chat, "Ann: Water break."); bridge < strings.Index(chat, "Ann: That is all for today!") || bridge > strings.LastIndex(chat, "Ann: Hello, everyone.") {
		t.Errorf("the bridge is not between the episodes:\n%s", chat)
	}
}
