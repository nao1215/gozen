package play

import (
	"fmt"
	"strings"
	"testing"
)

// More of the novel window: the backlog (kept to ui.backlog lines, scrolled with the arrows), the
// band's Save and Load, skip and scene skip, and the speakers who leave the stage.

func wldManyLines() string {
	var b strings.Builder
	b.WriteString("events:\n  ev_many:\n    do:\n")
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "      - say: \"Line %d.\"\n", i)
	}
	return b.String()
}

func wldNovelWith(t *testing.T, ui, event string) *Game {
	t.Helper()
	opts := Options{Location: "camp", FS: wldWorldFiles(t, "ui: "+ui+"\n", map[string]string{"data/novel_cov.yaml": wldNovelData, "data/many_cov.yaml": wldManyLines()})}
	g := wldOpenWith(t, opts)
	wldFrames(t, g, 10)
	wldStartEvent(t, g, event)
	wldFrames(t, g, 5)
	return g
}

func TestWldNovelBacklogKeepsTheNewestLinesAndScrolls(t *testing.T) {
	g := wldNovelWith(t, "{message: novel, backlog: 5}", "ev_many")
	for i := 0; i < 11; i++ {
		wldAdvance(t, g)
	}
	if wldTalkText(g) != "Line 12." {
		t.Fatalf("at %q", wldTalkText(g))
	}
	if len(g.backlog) != 5 {
		t.Fatalf("backlog keeps %d lines, ui.backlog is 5", len(g.backlog))
	}
	if first, _ := g.backlog[0].shown(g.db); first != "" && g.backlog[0].text != "Line 8." {
		t.Fatalf("first kept line %q, want Line 8.", g.backlog[0].text)
	}
	if g.backlog[4].text != "Line 12." {
		t.Fatalf("last kept line %q", g.backlog[4].text)
	}
	wldPlay(t, g, "press log\nwait 6\n", 50)
	if g.logView == nil {
		t.Fatal("the log did not open")
	}
	last := g.logLastTop()
	if g.logView.top != last {
		t.Fatalf("the log opens at %d, want the last page %d", g.logView.top, last)
	}
	wldPlay(t, g, "press up\nwait 6\n", 50)
	if g.logView == nil || (last > 0 && g.logView.top != last-1) {
		t.Fatalf("up scrolled to %v (last %d)", g.logView, last)
	}
	wldFrames(t, g, 3)
	wldPlay(t, g, "press up\nwait 4\npress up\nwait 4\npress up\nwait 4\npress up\nwait 4\npress up\nwait 4\n", 100)
	if g.logView.top != 0 {
		t.Fatalf("scrolled past the first line: %d", g.logView.top)
	}
	// down to the end closes it
	wldPlay(t, g, strings.Repeat("press down\nwait 4\n", 8), 200)
	if g.logView != nil {
		t.Fatal("scrolling down past the end left the log open")
	}
	// the band's button opens it too, and a click closes it
	rects, buttons := g.novelButtonRects(), g.novelButtons()
	for i, b := range buttons {
		if b.b == btnLog {
			r := rects[i]
			wldPlay(t, g, fmt.Sprintf("click %d %d\nwait 6\n", r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2), 50)
		}
	}
	if g.logView == nil {
		t.Fatal("clicking the log button did not open the log")
	}
	wldPlay(t, g, "click 10 10\nwait 6\n", 50)
	if g.logView != nil {
		t.Fatal("a click did not close the log")
	}
}

func TestWldNovelSaveAndLoadButtonsOpenTheSlots(t *testing.T) {
	g := wldNovelWith(t, "{message: novel}", "ev_auto")
	wldFrames(t, g, 60)
	wldPlay(t, g, "press save\nwait 6\n", 50)
	sm, ok := g.stack[len(g.stack)-1].(*saveMenu)
	if !ok || !sm.saving {
		t.Fatalf("the save button opened %T", g.stack[len(g.stack)-1])
	}
	wldPlay(t, g, "press cancel\nwait 10\n", 50)
	wldPlay(t, g, "press load\nwait 6\n", 50)
	lm, ok := g.stack[len(g.stack)-1].(*saveMenu)
	if !ok || lm.saving || !lm.inPlay {
		t.Fatalf("the load button opened %T", g.stack[len(g.stack)-1])
	}
}

func TestWldNovelSceneSkipPassesAnEventThatWasRead(t *testing.T) {
	g := wldNovelWith(t, "{message: novel}", "ev_auto")
	for i := 0; i < 5; i++ {
		wldAdvance(t, g)
	}
	if g.talk.active || g.events.Running() {
		t.Fatalf("the first reading did not end: %q", wldTalkText(g))
	}
	// read the second time: every line was read, so the band offers scene skip
	wldStartEvent(t, g, "ev_auto")
	wldFrames(t, g, 20)
	if !g.sceneSkippable() {
		t.Fatal("an event whose lines were all read is not skippable")
	}
	found := false
	for _, b := range g.novelButtons() {
		found = found || b.b == btnSceneSkip
	}
	if !found {
		t.Fatal("the band has no scene skip button")
	}
	wldPlay(t, g, "press scene_skip\nwait 30\n", 100)
	wldFrames(t, g, 30)
	if g.talk.active && wldTalkText(g) != "" || g.events.Running() {
		t.Fatalf("scene skip stopped at %q", wldTalkText(g))
	}
}

func TestWldNovelSkipPassesLinesReadAndStopsAtNewOnes(t *testing.T) {
	g := wldNovelWith(t, "{message: novel}", "ev_many")
	for i := 0; i < 3; i++ { // read lines 1 to 3
		wldAdvance(t, g)
	}
	if wldTalkText(g) != "Line 4." {
		t.Fatalf("at %q", wldTalkText(g))
	}
	// start again: skip mode passes the three lines read and stops on the fourth, which was not
	g.events.Stop()
	g.req, g.talk = nil, talk{}
	wldStartEvent(t, g, "ev_many")
	wldFrames(t, g, 10)
	if g.skipping || wldTalkText(g) != "Line 1." {
		t.Fatalf("skipping %v at %q", g.skipping, wldTalkText(g))
	}
	wldPlay(t, g, "press skip\nwait 60\n", 200)
	if got := wldTalkText(g); got != "Line 4." {
		t.Fatalf("skip stopped at %q, want the first line not read", got)
	}
	if g.skipping {
		t.Fatal("skip mode went on over a line not read")
	}
}

func TestWldNovelSpeakerWhoLeavesFadesOut(t *testing.T) {
	g := wldNovelWith(t, "{message: novel}", "ev_auto")
	for i := 0; i < 3; i++ {
		wldAdvance(t, g)
	}
	// the fourth speaker pushes the first off the stage: the one who left is drawn fading
	if wldTalkText(g) != "Four." {
		t.Fatalf("at %q", wldTalkText(g))
	}
	if g.stageLeftT == 0 && g.stageOutT == 0 {
		t.Fatalf("nobody left the stage: out %d left %d", g.stageOutT, g.stageLeftT)
	}
	figs := g.stageFigures(true, func(face string) bool { return face != "" })
	if len(figs) == 0 {
		t.Fatal("no figures on the stage")
	}
	wldFrames(t, g, 30)
}
