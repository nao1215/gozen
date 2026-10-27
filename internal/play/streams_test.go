package play

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nao1215/gozen/internal/layer3/broadcast"
	"github.com/nao1215/gozen/internal/rpg"
)

// streamsData is a neutral stream game: two channels of the example puppet, one short
// episode each and a duo, and a reaction book that answers comments and tips.
var streamsData = map[string]string{
	"data/streams_cast.yaml": `actors:
  ann: {name: Ann, face: smile, puppet: streamer/puppet.yaml}
  bo: {name: Bo, face: smile, puppet: streamer/puppet.yaml}
locations:
  booth: {name: Booth, stream: lobby, exits: [{to: camp, label: Camp}]}
`,
	"data/streams.yaml": `timing: {say: 0.5, per_char: 0.01, chat: 0.3, ask: 1}
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

// streamsGame opens the stream game at its lobby at a fixed time; duo is the chance of
// the duo.
func streamsGame(t *testing.T, duo string) *Game {
	t.Helper()
	fsys := gameFiles(t)
	fsys["game.yaml"] = &fstest.MapFile{Data: append([]byte("schema_version: 9\nstreams: data/streams.yaml\nreactions: {ann: talk.yaml}\n"), fsys["game.yaml"].Data...)}
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
	g := wldOpenWith(t, Options{FS: fsys, Location: "booth", Now: func() time.Time { return time.Date(2026, 10, 27, 20, 0, 0, 0, time.UTC) }})
	wldFrames(t, g, 40)
	return g
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
// first opening; watching asks the viewer's name, greets a first visit, posts the beats
// and the comments, answers a comment and a tip from the reaction book, and at the end
// thanks the viewer, moves the channel on to its next episode and keeps it in the save.
func TestStreamsWatchAShow(t *testing.T) {
	g := streamsGame(t, "0")
	l, ok := g.stack[0].(*streamLobby)
	if !ok || len(l.cards) != 2 || len(l.entries(g)) == 0 {
		t.Fatalf("scene %T, cards %d", g.stack[0], len(l.cards))
	}
	wldPlay(t, g, "press ok\nwait 5\ntype Pat\npress ok\nwait 5\n", 100)
	w := g.watching()
	if w == nil || w.naming || g.st.Streams.Viewer != "Pat" {
		t.Fatalf("watch %v, viewer %q", w, g.st.Streams.Viewer)
	}
	wldPlay(t, g, "press tab\ntype I planted beans\npress ok\nwait 2\n", 100)
	wldFrames(t, g, 120)
	chat := chatText(w)
	for _, want := range []string{"Ann: Welcome, Pat!", "Pat: I planted beans", "Ann: Plants!", "Ann: You said I planted beans."} {
		if !strings.Contains(chat, want) {
			t.Errorf("chat lacks %q:\n%s", want, chat)
		}
	}
	gold := g.st.Gold
	w.tip(g, 10)
	wldFrames(t, g, 60)
	if g.st.Gold != gold-10 || !strings.Contains(chatText(w), "Ann: Thank you, Pat!") {
		t.Errorf("gold %d (was %d), chat:\n%s", g.st.Gold, gold, chatText(w))
	}
	w.choose(g, 0)
	wldFrames(t, g, 900)
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
