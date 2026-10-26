package play

import "testing"

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
