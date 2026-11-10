package play

import (
	"bytes"
	"encoding/json"
	"image"
	"testing"

	"github.com/nao1215/gozen/internal/base/traceformat"
)

// A list's headings are passed over by the cursor, and a pinned last item keeps the last
// row while the rest scroll above it.
func TestListHeadingsAndPinnedLastRow(t *testing.T) {
	items := []listItem{{label: "A", heading: true}}
	for range 8 {
		items = append(items, listItem{label: "x"})
	}
	items = append(items, listItem{label: "B", heading: true}, listItem{label: "y"}, listItem{label: "end"})
	l := newList(items, 0, 0, 400, 5)
	l.pin = true
	if l.sel != 1 {
		t.Fatalf("the cursor starts on %d, want 1 (past the heading)", l.sel)
	}
	l.step(-1) // round to the end: the pinned item
	if l.sel != len(items)-1 {
		t.Fatalf("up from the first item went to %d", l.sel)
	}
	l.scroll()
	if shown := l.shown(); shown[len(shown)-1] != len(items)-1 || len(shown) != 5 || l.top != 0 {
		t.Fatalf("shown %v (top %d), want 4 rows from the top and the pinned item", shown, l.top)
	}
	if got, want := l.rowRect(len(items)-1).Min.Y, l.rowRect(3).Min.Y+int(l.rowH); got != want {
		t.Errorf("the pinned item's row is at %d, want %d (the last row)", got, want)
	}
	l.step(-1) // over the heading B to y
	if l.sel != 10 {
		t.Fatalf("up from the pinned item went to %d, want 10", l.sel)
	}
	l.scroll()
	if shown := l.shown(); shown[len(shown)-2] != 10 || shown[len(shown)-1] != 11 {
		t.Fatalf("shown %v, want y then the pinned item last", shown)
	}
	l.step(1)
	if l.sel != 11 {
		t.Fatalf("down from y went to %d", l.sel)
	}
	l.step(1) // round to the first item, past heading A
	if l.sel != 1 {
		t.Fatalf("down from the end went to %d", l.sel)
	}
	l.scroll()
	if l.top != 0 {
		t.Errorf("top %d, want 0 so the first heading shows", l.top)
	}
	// a short list draws its items in order: nothing is pinned
	short := newList(items[8:], 0, 0, 400, 5)
	short.pin = true
	if short.pinned() {
		t.Error("a list that fits pins its last item")
	}
}

func TestListDecisionTrace(t *testing.T) {
	g, _ := wldTracedGame(t, "")
	var b bytes.Buffer
	g.tr = newTracer(&b)
	l := newList([]listItem{{label: "Continue", disabled: true}}, 0, 0, 200, 1)
	g.in = input{}
	g.in.pressed[btnOK] = true
	if l.updateChoice(g) {
		t.Fatal("disabled choice accepted")
	}
	if len(traceLines(t, b.Bytes(), "decision")) != 0 {
		t.Fatal("disabled choice recorded")
	}
	l.items[0].disabled = false
	if !l.updateChoice(g) {
		t.Fatal("choice not accepted")
	}
	lines := traceLines(t, b.Bytes(), "decision")
	if len(lines) != 1 || lines[0]["kind"] != "menu" || lines[0]["label"] != "Continue" {
		t.Fatalf("decision: %v", lines)
	}
	b.Reset()
	g.talk.choice = l
	l.updateChoice(g)
	if len(traceLines(t, b.Bytes(), "decision")) != 0 {
		t.Fatal("dialogue answer duplicated")
	}
	g.talk.choice = nil
	g.in = input{}
	g.in.pressed[btnCancel] = true
	chose, cancel := l.update(g)
	lines = traceLines(t, b.Bytes(), "decision")
	if chose || !cancel || len(lines) != 1 || lines[0]["kind"] != "menu_cancel" {
		t.Fatalf("cancel: %v", lines)
	}
}

// uiLongList is a list of ten rows of which three show.
func uiLongList() *list {
	var items []listItem
	for i := range 10 {
		items = append(items, listItem{label: string(rune('A' + i))})
	}
	return newList(items, 0, 0, 400, 3)
}

// uiFingerDrag are the ticks of a finger that goes down on the list's last row shown, goes
// up the screen by rows of its rows and lifts: a touch's click comes when the finger lifts.
func uiFingerDrag(l *list, rows int) []input {
	row := l.rowRect(l.top + l.rows - 1)
	at := image.Pt(row.Min.X+50, row.Min.Y+10)
	ticks := []input{{mouse: at, moved: true, pdown: true, pheld: true}}
	for range 2 * rows {
		at.Y -= int(l.rowH) / 2
		ticks = append(ticks, input{mouse: at, moved: true, pheld: true})
	}
	return append(ticks, input{mouse: at, moved: true, pup: true, click: true}, input{mouse: at})
}

// A finger dragged up a list longer than its rows scrolls it and chooses nothing, the
// finger's lift included: a touch screen has no wheel and no arrow keys to reach the rows
// below. The cursor stays on a row shown.
func TestListScrollsByTouchDrag(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	l := uiLongList()
	for _, in := range uiFingerDrag(l, 2) {
		g.in = in
		if l.updateChoice(g) {
			t.Fatalf("the drag chose row %d (lift %v)", l.sel, in.pup)
		}
	}
	g.in = input{}
	if l.top != 2 {
		t.Errorf("a drag of two rows left the list at row %d of %d items in %d rows", l.top, len(l.items), l.rows)
	}
	if l.sel < l.top || l.sel >= l.top+l.rows {
		t.Errorf("the cursor is on row %d, not one of the rows shown from %d", l.sel, l.top)
	}
}

// A finger that goes down outside a list, or a list whose rows all show, scrolls nothing,
// and its lift on a row still chooses it (a finger slid onto a row).
func TestListTouchOutsideOrShortListStillChooses(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	short := newList([]listItem{{label: "A"}, {label: "B"}, {label: "C"}}, 0, 0, 400, 3)
	for name, l := range map[string]*list{"short": short, "outside": uiLongList()} {
		row := l.rowRect(1)
		from := image.Pt(row.Min.X+50, row.Min.Y+100)
		if name == "outside" {
			from = image.Pt(row.Max.X+200, row.Min.Y+100)
		}
		to := image.Pt(row.Min.X+50, row.Min.Y+10)
		chose := false
		for _, in := range []input{{mouse: from, moved: true, pdown: true, pheld: true}, {mouse: to, moved: true, pheld: true}, {mouse: to, moved: true, pup: true, click: true}} {
			g.in = in
			chose = l.updateChoice(g)
		}
		g.in = input{}
		if !chose || l.sel != 1 || l.top != 0 {
			t.Errorf("%s: chose %v row %d top %d, want the second row chosen and no scroll", name, chose, l.sel, l.top)
		}
	}
}

// The mouse clicks as it presses: the row under it is chosen, and the mouse dragged on
// with its button held scrolls nothing.
func TestListMouseDragScrollsNothing(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	l := uiLongList()
	ticks := uiFingerDrag(l, 2)
	ticks[0].click = true             // the button goes down: a click at once
	ticks[len(ticks)-2].click = false // and its release is none
	chose := 0
	for _, in := range ticks {
		g.in = in
		if l.updateChoice(g) {
			chose++
		}
	}
	g.in = input{}
	if chose != 1 || l.top != 0 {
		t.Fatalf("chose %d times, top %d: the press chooses once and the drag scrolls nothing", chose, l.top)
	}
}

// The ▲ and ▼ of a list longer than its rows are controls: a tap scrolls a page, chooses
// nothing and leaves the cursor on a row shown.
func TestListArrowsScrollWithATap(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	l := uiLongList()
	tap := func(cx, y float64) {
		t.Helper()
		g.in = input{click: true, pdown: true, pup: true, moved: true, mouse: image.Pt(int(cx), int(y+sizeS/2))}
		if l.updateChoice(g) {
			t.Fatalf("a tap on an arrow chose row %d", l.sel)
		}
		g.in = input{}
		if l.sel < l.top || l.sel >= l.top+l.rows {
			t.Fatalf("the cursor is on row %d, not one of the rows shown from %d", l.sel, l.top)
		}
	}
	cx, up, down := l.markPlaces()
	tap(cx, down)
	if l.top != 2 {
		t.Fatalf("a tap on ▼ left the list at row %d, want a page (2 rows) on", l.top)
	}
	for range 5 {
		tap(cx, down)
	}
	if l.top != 7 {
		t.Fatalf("▼ stopped at row %d, want the last page (7)", l.top)
	}
	tap(cx, up)
	if l.top != 5 {
		t.Fatalf("a tap on ▲ left the list at row %d, want a page back (5)", l.top)
	}
	// the arrows are drawn as controls a script and the layout checks can see
	var buf bytes.Buffer
	g.tr = newTracer(&buf)
	g.tr.layout = &layoutFrame{}
	l.draw(g, covScreen(), true)
	var marks []string
	for _, c := range g.tr.layout.controls {
		if c.Label == "▲" || c.Label == "▼" {
			marks = append(marks, c.Label)
		}
	}
	g.tr = nil
	if len(marks) != 2 {
		t.Fatalf("the arrows drawn as controls: %v, want ▲ and ▼", marks)
	}
}

// A finger that scrolled past a heading keeps the rows where it left them: the heading is
// brought back only when the cursor moves again.
func TestListSwipePastAHeadingStays(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	l := uiLongList()
	l.items[3] = listItem{label: "Group", heading: true}
	for _, in := range uiFingerDrag(l, 4) {
		g.in = in
		l.updateChoice(g)
	}
	g.in = input{}
	l.updateChoice(g)
	if l.top != 4 || l.sel != 4 {
		t.Fatalf("after a drag of four rows: top %d cursor %d, want 4 and 4 (the heading above stays away)", l.top, l.sel)
	}
	g.in.pressed[btnDown] = true
	l.updateChoice(g)
	g.in = input{}
	if l.top != 3 || l.sel != 5 {
		t.Fatalf("after the cursor moved: top %d cursor %d, want the heading back (3) and the cursor on 5", l.top, l.sel)
	}
}

// A swipe reads the pointer only (down, held, up and where it is), which a trace records:
// the inputs played back from the trace scroll the list as the recorded run did, tick by
// tick.
func TestListSwipeReplaysFromATrace(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	live, again := uiLongList(), uiLongList()
	ticks := uiFingerDrag(live, 3)
	var buf bytes.Buffer
	rec := &Game{tr: newTracer(&buf)}
	for i := range ticks {
		rec.tick = i + 1
		rec.traceInput(&ticks[i])
	}
	r := &replay{inputs: map[int]traceLine{}}
	for ln := range bytes.Lines(buf.Bytes()) {
		var l traceLine
		if err := json.Unmarshal(ln, &l); err != nil {
			t.Fatal(err)
		}
		if l["t"] == "input" {
			r.inputs[int(traceformat.Number(l["tick"]))] = l
		}
	}
	if len(r.inputs) < len(ticks)-1 {
		t.Fatalf("the trace has %d of %d inputs: %s", len(r.inputs), len(ticks), buf.String())
	}
	var played input
	for i, in := range ticks {
		g.in = in
		choseLive := live.updateChoice(g)
		r.feed(&played, i+1)
		g.in = played
		choseAgain := again.updateChoice(g)
		played = g.in
		if choseLive != choseAgain || live.top != again.top || live.sel != again.sel {
			t.Fatalf("tick %d: the run had top %d cursor %d chose %v, its replay top %d cursor %d chose %v", i+1, live.top, live.sel, choseLive, again.top, again.sel, choseAgain)
		}
	}
	g.in = input{}
	if live.top != 3 {
		t.Fatalf("the swipe of three rows left the list at row %d", live.top)
	}
}

// uiSwipe are the ticks of a finger that goes down at from, goes dy down the screen in
// steps and lifts (a touch's click comes with the lift).
func uiSwipe(from image.Point, dy, steps int) []input {
	at := from
	ticks := []input{{mouse: at, moved: true, pdown: true, pheld: true}}
	for range steps {
		at.Y += dy / steps
		ticks = append(ticks, input{mouse: at, moved: true, pheld: true})
	}
	return append(ticks, input{mouse: at, moved: true, pup: true, click: true})
}
