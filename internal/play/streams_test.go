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

	"github.com/hajimehoshi/ebiten/v2"

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

// TestStreamsKeepTheTimeOpen: while the game stays open on one screen (the settings over the
// lobby too), the autosave keeps the time it was last open at most a minute old, and closing
// the game writes it: opening
// it again at once earns nothing while away, and opening it two hours later two hours.
func TestStreamsKeepTheTimeOpen(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	g.st.Streams.Viewer, l.name = "Pat", nil
	saved := func() *rpg.Streams {
		t.Helper()
		b, err := g.storeRead(g.autoPath())
		if err != nil {
			t.Fatal(err)
		}
		st, err := rpg.Parse(g.db, b, nil)
		if err != nil || st.Streams == nil {
			t.Fatalf("the autosave: %v", err)
		}
		return st.Streams
	}
	wldFrames(t, g, int(2.5*60*ebiten.DefaultTPS))
	if open := time.Unix(saved().Open, 0); g.streamNow().Sub(open) > streamKeep {
		t.Fatalf("after 2.5 minutes in the lobby the autosave has the game open at %v, now %v", open, g.streamNow())
	}
	g.push(newMenu(g)) // the settings opened from the menu over the lobby
	g.push(newConfigScreen(g))
	wldFrames(t, g, int(2.5*60*ebiten.DefaultTPS))
	if open := time.Unix(saved().Open, 0); g.streamNow().Sub(open) > streamKeep {
		t.Fatalf("after 2.5 minutes in the settings the autosave has the game open at %v, now %v", open, g.streamNow())
	}
	sysClose(t, g)
	closed := g.streamNow()
	if open := saved().Open; open != closed.Unix() {
		t.Fatalf("closing keeps the game open at %v, not %v", time.Unix(open, 0), closed)
	}
	if gold := broadcast.Settle(g.db.Streams, saved(), closed.Add(5*time.Second)); gold != 0 {
		t.Errorf("opening again at once earns %d", gold)
	}
	if gold := broadcast.Settle(g.db.Streams, saved(), closed.Add(2*time.Hour)); gold != 200 {
		t.Errorf("opening again two hours later earns %d, not 200", gold)
	}
}

// TestStreamsReplayReadsTheRecordedClock: the trace records the time the run read with its
// offset from UTC, and a replay plays the streams at that time where it was read, not at its
// own time or in its own zone, so it does not diverge: just after midnight in Tokyo is that
// day and hour in a replay on a machine set to UTC.
func TestStreamsReplayReadsTheRecordedClock(t *testing.T) {
	covSlow(t)
	tokyo := time.Date(2026, 10, 28, 0, 30, 0, 0, time.FixedZone("JST", 9*3600))
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })
	path := filepath.Join(t.TempDir(), "run.jsonl")
	f, err := os.Create(path) //nolint:gosec // a file in the test directory
	if err != nil {
		t.Fatal(err)
	}
	fsys := streamsFS(t, "0")
	g, err := New(Options{
		FS: fsys, NoSound: true, Seed: 3, SaveDir: t.TempDir(), ShotDir: t.TempDir(), Location: "booth", Trace: f,
		Script: "wait 30\npress ok\nwait 300\nquit", Now: func() time.Time { return tokyo },
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
	if r.rep.diverge != "" || !r.rep.finished || !r.desk.base.Equal(tokyo) {
		t.Fatalf("diverged %q, finished %v, clock %v", r.rep.diverge, r.rep.finished, r.desk.base)
	}
	if _, off := r.desk.base.Zone(); off != 9*3600 || broadcast.Day(r.desk.base) != broadcast.Day(tokyo) || r.desk.base.Hour() != 0 {
		t.Fatalf("the replay's clock %v is not the day and hour read in Tokyo (%v)", r.desk.base, tokyo)
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
	if c := l.cards[0]; c.line == "" || c.line != lineAt(c.show, g.streamNow()) {
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
		{Chat: []string{"leaf", "hi"}},
		{Ask: &data.StreamAsk{Q: "Which?"}},
	}, Times: []float64{0, 1, 3, 4, 6, 9}, Start: noonish}
	for _, tc := range []struct {
		at   float64
		want string
	}{{0.5, ""}, {1, "Hello."}, {2.9, "Hello."}, {3 + captionLinger.Seconds() - 0.1, "Hello."}, {3 + captionLinger.Seconds(), ""}, {5, ""}, {6, "Which?"}, {9.5, "Which?"}} {
		if got := lineAt(s, noonish.Add(seconds(tc.at))); got != tc.want {
			t.Errorf("at %v s: %q, want %q", tc.at, got, tc.want)
		}
	}
}

// TestStreamsCaptionTimes: the watch view shows a line's caption at least captionLeast before
// the next step, even when the line's timing is shorter, and takes it away captionLinger
// after the chat beat that follows it begins.
func TestStreamsCaptionTimes(t *testing.T) {
	fsys := streamsFS(t, "0")
	fsys["data/streams.yaml"].Data = []byte(strings.Replace(string(fsys["data/streams.yaml"].Data), "chat: 0.3", "chat: 3", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	g.st.Streams.Viewer, l.name = "Pat", nil
	show := l.cards[slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })].show
	show.Start = g.streamNow()
	w := newStreamWatch(g, show, 100)
	g.push(w)
	var said, next time.Time
	for range 3000 {
		wldFrames(t, g, 1)
		if said.IsZero() && w.caption == "Hello, everyone." {
			said = g.streamNow()
		}
		if !said.IsZero() && w.next == 2 { // the chat beat after the line has begun
			next = g.streamNow()
			break
		}
	}
	if said.IsZero() || next.IsZero() {
		t.Fatal("the line and the chat after it never come")
	}
	if g.db.Streams.Timing.LineSeconds("Hello, everyone.") >= captionLeast.Seconds() || next.Sub(said) < captionLeast {
		t.Fatalf("a short line moves on after %v, before %v", next.Sub(said), captionLeast)
	}
	if w.shownCaption(g) != "Hello, everyone." {
		t.Fatal("the caption goes with the chat beat")
	}
	wldFrames(t, g, int(captionLinger.Seconds()*ebiten.DefaultTPS)+1)
	if got := w.shownCaption(g); got != "" {
		t.Fatalf("the caption %q stays %v after the chat beat began", got, captionLinger)
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

// tonePCM is the test tone as a voice clip.
func tonePCM(t *testing.T) []byte {
	t.Helper()
	tone, err := os.ReadFile("testdata/tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := sound.VoiceClip(fstest.MapFS{"tone.ogg": {Data: tone}}, "tone.ogg")
	if err != nil {
		t.Fatal(err)
	}
	return pcm
}

// TestStreamsQuestionWaitsAfterTheVoice: a question read aloud gives the viewers timing.ask
// after its line ends, whether speech synthesis says it has ended or its recorded voice
// loads late (a browser still fetching it) and then plays.
func TestStreamsQuestionWaitsAfterTheVoice(t *testing.T) {
	asked := func(t *testing.T) (*Game, *streamWatch) {
		t.Helper()
		g, w, _ := voicedShow(t)
		for range 3000 {
			if w.ask != nil {
				return g, w
			}
			wldFrames(t, g, 1)
		}
		t.Fatal("the question is never asked")
		return nil, nil
	}
	t.Run("speech synthesis", func(t *testing.T) {
		g, w := asked(t)
		w.spoken = func() bool { return true } // the browser says the line has ended
		wldFrames(t, g, 1)
		if want := g.streamNow().Add(seconds(g.db.Streams.Timing.Ask)); w.ask == nil || w.busy.Before(want) {
			t.Fatalf("the answers close at %v, before %v (asking: %v)", w.busy, want, w.ask != nil)
		}
	})
	t.Run("a voice loaded late", func(t *testing.T) {
		g, w := asked(t)
		pcm := tonePCM(t)
		v := &voiceLoad{file: "assets/voice/ann/late.ogg", done: make(chan struct{})}
		w.voice = v
		wldFrames(t, g, 1)
		v.pcm = pcm
		close(v.done)
		wldFrames(t, g, 1)
		if want := g.streamNow().Add(seconds(sound.ClipSeconds(pcm)) + voiceGap + seconds(g.db.Streams.Timing.Ask)); w.ask == nil || w.busy.Before(want.Add(-time.Second/ebiten.DefaultTPS)) {
			t.Fatalf("the answers close at %v, before %v (asking: %v)", w.busy, want, w.ask != nil)
		}
	})
	t.Run("a voice past its limit", func(t *testing.T) {
		g, w := asked(t)
		v := &voiceLoad{file: "assets/voice/ann/late.ogg", done: make(chan struct{}), pcm: tonePCM(t)}
		limit := g.streamNow()
		w.voice, w.limit, w.busy = v, limit, limit.Add(w.hold)
		wldFrames(t, g, 1)
		close(v.done) // after its limit: the line is not read any more
		wldFrames(t, g, 1)
		if want := limit.Add(seconds(g.db.Streams.Timing.Ask)); !w.busy.Equal(want) || w.spoken != nil {
			t.Fatalf("a voice past its limit moves the answers' end from %v to %v", want, w.busy)
		}
	})
}

// TestStreamsSpeakerWhileTyping: while a comment is typed, a click on the field keeps typing,
// and a click on the read-aloud button stops typing and turns reading aloud off.
func TestStreamsSpeakerWhileTyping(t *testing.T) {
	g, w, _ := voicedShow(t)
	click := func(c watchControl) {
		x, y, cw, ch := w.controlRect(g, [2]int{int(c), 0})
		g.in.click, g.in.mouse = true, image.Pt(int(x+cw/2), int(y+ch/2))
		w.input(g)
		g.in.click = false
	}
	w.typing = true
	click(ctlComment)
	if !w.typing {
		t.Fatal("a click on the comment field stops typing")
	}
	click(ctlSpeak)
	if w.typing || g.desk.speak {
		t.Fatalf("the read-aloud button while typing: typing %v, reading aloud %v", w.typing, g.desk.speak)
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

// TestStreamsFamilyEpisode: a comment on an episode with family: true gives the reaction
// book value.family 1, and on another episode 0.
func TestStreamsFamilyEpisode(t *testing.T) {
	for _, tc := range []struct {
		family, want string
	}{{"family: true\n", "Ann: For the family."}, {"", "Ann: Just us."}} {
		t.Run(tc.want, func(t *testing.T) {
			fsys := streamsFS(t, "0")
			fsys["streams/one.yaml"].Data = append([]byte(tc.family), fsys["streams/one.yaml"].Data...)
			fsys["assets/talk.yaml"] = &fstest.MapFile{Data: []byte(`people:
  - id: ann
    rules:
      - id: family
        when: value.family == 1
        say: [For the family.]
      - id: grown
        when: value.family == 0
        say: [Just us.]
`)}
			g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
			wldFrames(t, g, 40)
			g.st.Streams.Viewer = "Pat"
			l, ok := g.stack[0].(*streamLobby)
			if !ok {
				t.Fatalf("scene %T", g.stack[0])
			}
			show := l.cards[slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })].show
			show.Start = g.streamNow()
			w := newStreamWatch(g, show, 100)
			g.push(w)
			w.comments = append(w.comments, "hello")
			for i := 0; i < 3000 && !strings.Contains(chatText(w), tc.want); i++ {
				wldFrames(t, g, 1)
			}
			if !strings.Contains(chatText(w), tc.want) {
				t.Errorf("%q: no %q in the chat:\n%s", tc.family, tc.want, chatText(w))
			}
		})
	}
}

// TestStreamsRepliesInTheLanguageShown: the reaction book's lines are texts of the game to
// translate, and a reply is said in the language shown, its fields filled after: a line
// with a translation in it, a line without one as written.
func TestStreamsRepliesInTheLanguageShown(t *testing.T) {
	fsys := streamsFS(t, "0")
	fsys["lang/ja.yaml"].Data = append(fsys["lang/ja.yaml"].Data, []byte("  \"Plants!\": \"植物！\"\n")...)
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	texts := map[string]bool{}
	for _, st := range g.db.SourceTexts() {
		texts[st.Text] = true
	}
	if !texts["Plants!"] || !texts["You said {text}."] || !texts["Hm."] {
		t.Fatal("the reaction book's lines are not texts of the game to translate")
	}
	g.setLanguage("ja")
	g.st.Streams.Viewer = "Pat"
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	show := l.cards[slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })].show
	show.Start = g.streamNow()
	w := newStreamWatch(g, show, 100)
	g.push(w)
	w.comments = append(w.comments, "beans")
	for i := 0; i < 3000 && !strings.Contains(chatText(w), "You said beans."); i++ {
		wldFrames(t, g, 1)
	}
	if chat := chatText(w); !strings.Contains(chat, "Ann: 植物！") || !strings.Contains(chat, "Ann: You said beans.") {
		t.Fatalf("the reply is not in the language shown:\n%s", chat)
	}
}

// TestStreamsRepliesFollowTheLanguage: a reply waiting to be said, the rest of one of two
// lines or a comment read back later, is said in the language shown when it is said, its
// fields filled with what they were when the book answered.
func TestStreamsRepliesFollowTheLanguage(t *testing.T) {
	fsys := streamsFS(t, "0")
	fsys["data/streams.yaml"].Data = append([]byte("echo: {max_runes: 24, chance: 1}\n"), fsys["data/streams.yaml"].Data...)
	fsys["lang/ja.yaml"].Data = append(fsys["lang/ja.yaml"].Data, []byte("  \"You said {text}.\": \"「{text}」って言ったね。\"\n  \"Hm.\": \"うーん。\"\n")...)
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	g.st.Streams.Viewer = "Pat"
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	show := l.cards[slices.IndexFunc(l.cards, func(c *lobbyCard) bool { return c.show.Hosts[0].ID == "ann" })].show
	show.Start = g.streamNow()
	w := newStreamWatch(g, show, 100)
	g.push(w)
	until := func(done func() bool) {
		t.Helper()
		for range 3000 {
			if done() {
				return
			}
			wldFrames(t, g, 1)
		}
		t.Fatalf("never happened; chat:\n%s", chatText(w))
	}
	w.comments = append(w.comments, "beans")
	until(func() bool { return w.caption == "Plants!" })
	g.setLanguage("ja")
	until(func() bool {
		return strings.Contains(chatText(w), "「beans」って言ったね。") || strings.Contains(chatText(w), "You said beans.")
	})
	if !strings.Contains(chatText(w), "Ann: 「beans」って言ったね。") {
		t.Fatalf("the second line of a reply is not said in the language shown:\n%s", chatText(w))
	}
	g.setLanguage("en")
	w.comments = append(w.comments, "zzz")
	until(func() bool { return len(w.later) == 1 })
	g.setLanguage("ja")
	until(func() bool {
		return strings.Contains(chatText(w), "Ann: うーん。") || strings.Contains(chatText(w), "Ann: Hm.")
	})
	if !strings.Contains(chatText(w), "Ann: うーん。") {
		t.Fatalf("a comment read back later is not said in the language shown:\n%s", chatText(w))
	}
}

// TestStreamsComeBackWhereLeft: the watch view goes at its own pace (voices, captions),
// ahead of the stream's clock at times; leaving and opening the same stream again comes in
// where it left, not back at the clock's beat to say again what was heard, and at the
// clock's beat once the clock is past it.
func TestStreamsComeBackWhereLeft(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok {
		t.Fatalf("scene %T", g.stack[0])
	}
	g.st.Streams.Viewer, l.name = "Pat", nil
	show := l.cards[0].show
	show.Start = g.streamNow()
	w := newStreamWatch(g, show, 100)
	g.push(w)
	w.next = 3 // ahead of the clock, at beat 0
	w.leave(g)
	if again := newStreamWatch(g, show, 100); again.next != 3 {
		t.Fatalf("opened again at beat %d, not 3 where it was left", again.next)
	}
	show.Start = g.streamNow().Add(-seconds(show.Times[len(show.Beats)-1] + 0.1)) // the clock at the last beat
	if again := newStreamWatch(g, show, 100); again.next != len(show.Beats)-1 {
		t.Fatalf("opened again at beat %d, not the clock's %d", again.next, len(show.Beats)-1)
	}
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

// TestStreamsStartAgain: with slot.rest a stream that ends starts again after the rest, so
// the channel is on the air again in the lobby with a new stream.
func TestStreamsStartAgain(t *testing.T) {
	fsys := streamsFS(t, "0")
	fsys["data/streams.yaml"].Data = []byte(strings.Replace(string(fsys["data/streams.yaml"].Data), "slot: {minutes: 0.01}", "slot: {minutes: 0.01, rest: 0.05}", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return noonish }})
	wldFrames(t, g, 40)
	first := g.desk.shows[0]
	wldFrames(t, g, int((first.Seconds()+3+5)*ebiten.DefaultTPS)) // past its end and a rest of 3 s
	again := g.desk.shows[0]
	if again == first || again.Key != first.Key || !again.Live(g.streamNow()) || !again.Start.Equal(first.Start.Add(seconds(first.Seconds()+3))) {
		t.Fatalf("the stream does not start again after its rest: %v from %v", again.Start, first.Start)
	}
	l, ok := g.stack[0].(*streamLobby)
	if !ok || len(l.cards) != 2 || len(l.offline(g)) != 0 {
		t.Fatalf("the lobby does not show every channel on the air again")
	}
}
