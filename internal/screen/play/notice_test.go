package play

import (
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/model/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

// TestFieldNoticesCanBeDropped: with ui.field_notices off, notices raised on the field are
// dropped; with it on they stay.
func TestFieldNoticesCanBeDropped(t *testing.T) {
	g := sysOpen4(t, Options{Location: "camp"}, map[string]string{"game.yaml+": "ui: {field_notices: false}\n"})
	g.notices.add("Found a coin")
	sysFrames(t, g, 3)
	if len(g.notices.list) != 0 {
		t.Fatalf("notices %v on a field that drops them", g.notices.list)
	}
}

// TestFieldNoticesStayByDefault: notices stay on the field unless the game drops them, the same
// notice is not listed twice and the queue is bounded.
func TestFieldNoticesStayByDefault(t *testing.T) {
	g := sysOpen(t, Options{Location: "camp"}, nil)
	g.notices.add("Found a coin")
	sysFrames(t, g, 3)
	if len(g.notices.list) != 1 {
		t.Fatalf("notices %v", g.notices.list)
	}
	g.notices.add("Found a coin")
	if len(g.notices.list) != 1 {
		t.Fatal("the same notice twice")
	}
	for i := range 30 {
		g.notices.add(strings.Repeat("n", i+1))
	}
	if len(g.notices.list) > noticeMax+noticeQueue {
		t.Fatalf("%d notices queued", len(g.notices.list))
	}
}

func TestSurfaceNoticesWaitTheirTurn(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Screens = map[string]*data.AuthorScreen{
		"board": {View: data.PlanningView{Fullscreen: true}},
		"list":  {},
	}
	g := &Game{db: db}
	surface := &authorScreen{name: "board"}
	g.stack = []scene{surface}
	g.notices.add("First result")
	g.notices.add("Second result")
	second := g.notices.list[1]
	if g.noticeSurface() != surface || g.noticeVisible() != 1 {
		t.Fatal("fullscreen surface did not own one notice")
	}
	for range g.notices.list[0].t {
		g.notices.update(g.noticeVisible())
	}
	if len(g.notices.list) != 1 || g.notices.list[0] != second {
		t.Fatalf("unread result expired in queue: %+v", g.notices.list)
	}
	g.talk.active = true
	if g.noticesShown() {
		t.Fatal("a result would expire behind dialogue")
	}
	g.talk.active = false
	if !g.noticesShown() {
		t.Fatal("result did not return after dialogue")
	}
	g.stack = append(g.stack, &authorScreen{name: "list"})
	if g.noticeSurface() != nil || g.noticeVisible() != noticeMax {
		t.Fatal("ordinary list lost floating notices")
	}
}

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
		b.update(noticeMax)
	}
	if len(b.list) != 4 || b.list[3].t != noticeTicks {
		t.Fatalf("a queued notice aged before it showed: %+v", b.list)
	}
	b.update(noticeMax)
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
