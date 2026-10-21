package play

import "testing"

// The message box (the default window): the line is typed out, OK completes it first and turns
// to the next only when it is complete; a speaker's portrait stands on the side the line says.

func wldBox(t *testing.T, event string) *Game {
	t.Helper()
	opts := Options{Location: "camp"}
	opts.FS = wldWorldFiles(t, "", map[string]string{"data/novel_cov.yaml": wldNovelData})
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	wldStartEvent(t, g, event)
	if g.novel() {
		t.Fatal("the plain game should use the message box")
	}
	wldFrames(t, g, 2)
	return g
}

// OK while a line is still being typed completes it, and does not skip it.
func TestWldBoxOKCompletesTypingFirst(t *testing.T) {
	g := wldBox(t, "ev_long")
	wldFrames(t, g, 10)
	if g.talk.shown >= g.talk.total {
		t.Fatalf("a long line was shown at once: %d of %d", g.talk.shown, g.talk.total)
	}
	text := wldTalkText(g)
	wldPlay(t, g, "press ok\nwait 2\n", 50)
	if g.talk.shown != g.talk.total || g.talk.page != 0 || wldTalkText(g) != text {
		t.Fatalf("OK while typing: shown %d of %d on page %d", g.talk.shown, g.talk.total, g.talk.page)
	}
}

func TestWldBoxTypesOutThenAdvances(t *testing.T) {
	g := wldBox(t, "ev_box")
	if !g.talk.active || g.talk.say.Text != "Left one." {
		t.Fatalf("talk %+v", g.talk.say)
	}
	if g.talk.shown >= g.talk.total {
		t.Fatalf("the whole line (%d) was shown at once", g.talk.total)
	}
	before := g.talk.shown
	wldFrames(t, g, 1)
	if g.talk.shown <= before {
		t.Fatalf("the typing did not go on: %d -> %d", before, g.talk.shown)
	}
	wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 100)
	if g.talk.say.Text != "Right one." {
		t.Fatalf("OK on a complete line led to %q", g.talk.say.Text)
	}
	if !g.talk.say.Right {
		t.Fatal("side: right was not kept")
	}
	wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 100)
	if g.talk.say.Text != "Left again." || g.talk.say.Right {
		t.Fatalf("third line %q (right %v)", g.talk.say.Text, g.talk.say.Right)
	}
	// a speaker who is not in the party stands on the right by default, one in the party on the left
	wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 100)
	if g.talk.say.Text != "Small portrait." || !g.talk.say.Right {
		t.Fatalf("fourth line %q (right %v)", g.talk.say.Text, g.talk.say.Right)
	}
	wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 100)
	if g.talk.say.Text != "Party default." || g.talk.say.Right {
		t.Fatalf("party line %q (right %v)", g.talk.say.Text, g.talk.say.Right)
	}
	wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 100)
	if g.talk.say.Text != "No name." || g.talk.say.Name != "" {
		t.Fatalf("last line %q by %q", g.talk.say.Text, g.talk.say.Name)
	}
	wldPlay(t, g, "wait 40\npress ok\nwait 10\n", 100)
	if g.talk.active {
		t.Fatal("the window stayed after the last line")
	}
}

func TestWldBoxChoiceAndPages(t *testing.T) {
	g := wldBox(t, "ev_choice")
	wldPlay(t, g, "wait 60\npress ok\nwait 10\n", 200)
	wldPlay(t, g, "wait 60\n", 100)
	if g.talk.choice == nil {
		t.Fatalf("no choice (text %q)", g.talk.say.Text)
	}
	if g.talk.say.Text != "Which way?" {
		t.Fatalf("the choice's prompt is %q", g.talk.say.Text)
	}
	wldPlay(t, g, "press ok\nwait 40\n", 200)
	if !g.st.Flags["went_left"] || g.st.Chose["way"] != "0" {
		t.Fatalf("first option: left %v chose %q", g.st.Flags["went_left"], g.st.Chose["way"])
	}
}

func TestWldBoxLongLineIsPaged(t *testing.T) {
	g := wldBox(t, "ev_long")
	if len(g.talk.pages) < 2 {
		t.Fatalf("pages %d", len(g.talk.pages))
	}
	if g.talk.size <= 0 {
		t.Fatalf("the pages have no text size: %v", g.talk.size)
	}
	for i := 0; i < len(g.talk.pages) && g.talk.say.Text != "After."; i++ {
		wldPlay(t, g, "wait 200\npress ok\nwait 10\n", 400)
	}
	if g.talk.say.Text != "After." {
		t.Fatalf("paging led to %q", g.talk.say.Text)
	}
}
