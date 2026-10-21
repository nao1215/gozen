package play

import (
	"bytes"
	"strings"
	"testing"
)

// Text is kept as it is written whatever it holds: full-width letters, emoji and joined emoji,
// combining marks, half-width kana, a very long word and a very long line. The window lays it
// out in pages, the backlog and the trace keep the text itself.

const wldOddText = "Ｈｅｌｌｏ　ｗｏｒｌｄ。😀 👨‍👩‍👧‍👦 é ｱｲｳｴｵ ﾊﾟｽ " + "ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ" + " 日本語の長い文章がここに続きます。"

const wldOddData = `events:
  ev_odd:
    do:
      - say: {who: "Ｚｏｅ😀", face: pics/tall1.png, text: "Ｈｅｌｌｏ　ｗｏｒｌｄ。😀 👨‍👩‍👧‍👦 é ｱｲｳｴｵ ﾊﾟｽ ＡＢＣＤＥＦＧＨＩＪＫＬＭＮＯＰＱＲＳＴＵＶＷＸＹＺ 日本語の長い文章がここに続きます。"}
      - say: "WWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWWW"
      - notice: "通知😀 ＡＢＣ 👨‍👩‍👧‍👦"
      - choice:
          - {label: "はい😀", do: [{set: {flag.odd_yes: true}}]}
          - {label: "いいえ👨‍👩‍👧‍👦", do: []}
        prompt: "ほんとうに？😀"
      - say: "é after."
`

func wldOddGame(t *testing.T, ui string, trace *bytes.Buffer) *Game {
	t.Helper()
	opts := Options{Location: "camp", FS: wldWorldFiles(t, ui, map[string]string{"data/odd_cov.yaml": wldOddData})}
	if trace != nil {
		opts.Trace, opts.Seed = trace, 4
	}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	wldStartEvent(t, g, "ev_odd")
	wldFrames(t, g, 6)
	return g
}

func wldOddRun(t *testing.T, g *Game) {
	t.Helper()
	if g.talk.say.Text != wldOddText {
		t.Fatalf("the line as the window has it: %q", g.talk.say.Text)
	}
	if len(g.talk.pages) == 0 {
		t.Fatal("no pages")
	}
	for _, p := range g.talk.pages {
		if len(p) == 0 || len(p) > talkLines {
			t.Fatalf("a page of %d lines (the window shows %d)", len(p), talkLines)
		}
	}
	wldFrames(t, g, 150)
	if g.talk.shown < g.talk.total {
		t.Fatalf("typed %d of %d letters", g.talk.shown, g.talk.total)
	}
	for i := 0; i < len(g.talk.pages) && g.talk.say.Text == wldOddText; i++ {
		wldPlay(t, g, "press ok\nwait 20\n", 100)
		wldFrames(t, g, 40)
	}
	if got := g.talk.say.Text; !strings.HasPrefix(got, "WWWW") {
		t.Fatalf("the next line is %q", got)
	}
	for i := 0; i < 8 && g.talk.choice == nil; i++ { // the pages of the long line, then the notice and the question
		wldPlay(t, g, "wait 100\npress ok\nwait 10\n", 300)
	}
	wldFrames(t, g, 20)
	if len(g.notices.list) != 1 || g.notices.list[0].text != "通知😀 ＡＢＣ 👨‍👩‍👧‍👦" {
		t.Fatalf("notices %+v", g.notices.list)
	}
	if g.talk.choice == nil || g.talk.choice.items[0].label != "はい😀" {
		t.Fatalf("choice %+v", g.talk.choice)
	}
	wldPlay(t, g, "press ok\nwait 30\n", 100)
	if !g.st.Flags["odd_yes"] {
		t.Fatal("the first option was not taken")
	}
	var log []string
	for _, l := range g.backlog {
		log = append(log, l.text)
	}
	if !strings.Contains(strings.Join(log, "|"), wldOddText) || !strings.Contains(strings.Join(log, "|"), "→ はい😀") {
		t.Fatalf("backlog %q", log)
	}
	if g.failure != nil {
		t.Fatal(g.failure)
	}
}

func TestWldOddTextInTheNovelWindow(t *testing.T) {
	var trace bytes.Buffer
	g := wldOddGame(t, "ui: {message: novel}\n", &trace)
	wldOddRun(t, g)
	var found bool
	for _, l := range traceLines(t, trace.Bytes(), "say") {
		found = found || l["text"] == wldOddText
	}
	if !found {
		t.Fatal("the trace does not keep the text as it is")
	}
}

func TestWldOddTextInTheMessageBox(t *testing.T) {
	g := wldOddGame(t, "", nil)
	wldOddRun(t, g)
}
