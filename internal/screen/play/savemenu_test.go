package play

import "testing"

func TestSaveMenuReportsBrokenSlots(t *testing.T) {
	g := openGame(t, Options{Location: "camp", Seed: 3})
	if err := g.store.Write(g.slotPath(0), []byte("not json")); err != nil {
		t.Fatal(err)
	}
	sm := newSaveMenu(g, false, func() {})
	g.push(sm)
	if !sm.slots.items[0].disabled {
		t.Fatal("a broken save is offered")
	}
	sm.slots.items[0].disabled = false
	sm.slots.sel = 0
	mnuPress(g, btnOK)
	if sm.msgT == 0 || sm.msg == "" {
		t.Fatal("no message about the broken save")
	}
	mnuDrawAll(g)
	done := 0
	sm.done = func() { done++ }
	for range 100 {
		mnuPress(g)
	}
	if done != 1 {
		t.Fatalf("done called %d times", done)
	}
}
