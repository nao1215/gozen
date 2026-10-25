package play

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/rpg"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestAuthorScreenVisibilityAndReload(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Screens = map[string]*data.AuthorScreen{"notes": {
		Title: "Visits {var.visits}", Entries: []data.ScreenEntry{
			{Label: "Arrival", Text: "Welcome"},
			{Label: "Secret", Text: "Found", When: data.Expr{Expr: testgame.Expr("flag.found")}},
		},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	g.st.Vars["visits"] = float64(3)
	s := newAuthorScreen(g, "notes", nil)
	if s.title != "Visits 3" || len(s.entries) != 1 || s.entries[0].Label != "Arrival" {
		t.Fatalf("unexpected screen: %+v", s)
	}
	g.st.Flags["found"] = true
	n := sysAs[*authorScreen](t, s.liveKeep(g))
	if len(n.entries) != 2 || n.entries[1].Text != "Found" {
		t.Fatalf("reload did not reevaluate visibility: %+v", n)
	}
	db.Game.Screens["notes"].Entries = db.Game.Screens["notes"].Entries[1:]
	g.st.Flags["found"] = false
	n = sysAs[*authorScreen](t, n.liveKeep(g))
	if len(n.entries) != 0 || n.list.sel != 0 {
		t.Fatalf("empty screen selection: %+v", n)
	}
}

func TestAuthorScreenEntryActionRefresh(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries:
      - label: Add
        text: 'Boards: {var.boards}'
        do: [{set: {var.boards: 'var.boards + 1'}}]
      - label: Finish
        when: '!flag.finished'
        do: [{set: {flag.finished: true}}]
`, nil)
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	closed := 0
	g.openScreen("stock", func() { closed++ })
	wldPlay(t, g, "press z\nwait 8\n", 40)
	if g.st.Vars["boards"] != float64(1) {
		t.Fatalf("confirmation did not execute exactly once: %v", g.st.Vars)
	}
	s := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
	if s.entries[0].Text != "Boards: 1" || closed != 0 {
		t.Fatalf("action did not refresh in place: %v closed=%d", s.entries, closed)
	}
	wldPlay(t, g, "press down\npress z\nwait 8\n", 50)
	s = sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
	if !g.st.Flags["finished"] || len(s.entries) != 1 || s.list.sel != 0 {
		t.Fatalf("removed selection was not repaired: %+v", s)
	}
}

func TestAuthorScreenDialogueSaveAndOpener(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries:
      - label: Add
        do:
          - set: {var.boards: 'var.boards + 1'}
          - say: Board added.
          - set: {var.finished: 'var.finished + 1'}
`, map[string]string{"data/screen.yaml": `events:
  stock_open:
    do: [{open: stock}, {set: {flag.opener_finished: true}}]
`})
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	wldStartEvent(t, g, "stock_open")
	wldFrames(t, g, 4)
	wldPlay(t, g, "press z\nwait 8\n", 40)
	if !g.talk.active || g.st.Vars["boards"] != float64(1) || g.st.Flags["opener_finished"] {
		t.Fatalf("entry did not suspend opener: talk=%v vars=%v flags=%v", g.talk.active, g.st.Vars, g.st.Flags)
	}
	saved, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := rpg.Parse(g.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Resume == nil || restored.Resume.Cursor.Event != "screen:stock:0" {
		t.Fatalf("missing action cursor: %+v", restored.Resume)
	}
	wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
	if g.st.Vars["finished"] != float64(1) || g.st.Flags["opener_finished"] {
		t.Fatalf("action/opener completion: %v %v", g.st.Vars, g.st.Flags)
	}
	wldPlay(t, g, "press x\nwait 8\n", 40)
	if !g.st.Flags["opener_finished"] {
		t.Fatal("closing did not resume opener")
	}
	g.loadSavedGame(restored)
	wldFrames(t, g, 8)
	if !g.talk.active || g.st.Vars["boards"] != float64(1) {
		t.Fatalf("resume duplicated or lost action: %v", g.st.Vars)
	}
	wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
	if g.st.Vars["boards"] != float64(1) || g.st.Vars["finished"] != float64(1) {
		t.Fatalf("resume did not finish exactly once: %v", g.st.Vars)
	}
}

func TestAuthorScreenHiddenActionAndEmptyRefresh(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries: [{label: Once, when: '!flag.hidden', do: [{gold: 5}, {set: {flag.hidden: true}}]}]
`, nil)
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.openScreen("stock", func() {})
	before := g.st.Gold
	g.st.Flags["hidden"] = true
	wldPlay(t, g, "press z\nwait 5\n", 40)
	s := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
	if g.st.Gold != before || len(s.entries) != 0 || s.list.sel != 0 {
		t.Fatalf("hidden entry executed: gold=%d entries=%v", g.st.Gold, s.entries)
	}
	g.st.Flags["hidden"] = false
	s.refresh(g)
	wldPlay(t, g, "press z\nwait 5\n", 40)
	if g.st.Gold != before+5 || len(s.entries) != 0 {
		t.Fatalf("last entry did not disappear safely: gold=%d entries=%v", g.st.Gold, s.entries)
	}
}

func TestAuthorScreenNestedActionsAndMove(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries:
      - {label: More, do: [{open: details}, {set: {flag.returned: true}}]}
      - {label: Leave, do: [{move: fx_maze}]}
  details:
    title: Details
    entries: [{label: Mark, do: [{set: {flag.marked: true}}]}]
`, nil)
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	closed := 0
	g.openScreen("stock", func() { closed++ })
	wldPlay(t, g, "press z\nwait 5\npress z\nwait 5\n", 70)
	if !g.st.Flags["marked"] || g.st.Flags["returned"] {
		t.Fatalf("nested action order: %v", g.st.Flags)
	}
	wldPlay(t, g, "press x\nwait 8\n", 40)
	if !g.st.Flags["returned"] || closed != 0 {
		t.Fatalf("nested close did not return: %v closed=%d", g.st.Flags, closed)
	}
	wldPlay(t, g, "press down\npress z\nwait 8\n", 50)
	if g.st.Location != "fx_maze" || len(g.stack) != 1 || closed != 1 {
		t.Fatalf("move did not close owner: location=%s stack=%d closed=%d", g.st.Location, len(g.stack), closed)
	}
}

func TestAuthorScreenTraceReplay(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries: [{label: Add, text: 'Boards: {var.boards}', do: [{set: {var.boards: 'var.boards + 1'}}]}]
`, map[string]string{"data/stock.yaml": "locations: {workshop: {name: Workshop, on_enter: [{open: stock}]}}"})
	var trace bytes.Buffer
	g := wldOpenWith(t, Options{FS: fsys, Location: "workshop", Trace: &trace, Script: "wait 30\npress z\nwait 10\npress x\nwait 10\nquit\n"})
	screen := wldScreen(g)
	for i := 0; i < 200 && wldStep(t, g, screen); i++ {
	}
	if g.st.Vars["boards"] != float64(1) {
		t.Fatalf("script action: %v", g.st.Vars)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(trace.String(), `"t":"screen_action"`) {
		t.Fatal("action missing from trace")
	}
	replayed, err := wldReplayGame(t, trace.Bytes(), func(opts *Options) { opts.FS = fsys })
	if err != nil {
		t.Fatal(err)
	}
	if report := wldReplay(t, replayed); !strings.HasPrefix(report, "replay fully matched:") {
		t.Fatal(report)
	}
	if replayed.st.Vars["boards"] != float64(1) {
		t.Fatalf("replay action: %v", replayed.st.Vars)
	}
}

func TestAuthorScreenReloadCompletedAction(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries: [{label: Add, text: 'Boards: {var.boards}', do: [{set: {var.boards: 'var.boards + 1'}}]}]
`, map[string]string{"data/stock.yaml": "locations: {workshop: {name: Workshop, on_enter: [{open: stock}]}}"})
	g := wldOpenWith(t, Options{FS: fsys, Location: "workshop"})
	wldFrames(t, g, 30)
	wldPlay(t, g, "press z\nwait 10\n", 50)
	if g.st.Vars["boards"] != float64(1) {
		t.Fatalf("before reload: %v", g.st.Vars)
	}
	g.reload()
	wldFrames(t, g, 10)
	if g.st.Vars["boards"] != float64(1) {
		t.Fatalf("reload lost completed action: %v", g.st.Vars)
	}
}

func TestAuthorScreenReloadActiveDialogue(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries:
      - label: Add
        do: [{set: {var.boards: 'var.boards + 1'}}, {say: Added.}, {set: {var.finished: 'var.finished + 1'}}]
`, map[string]string{"data/stock.yaml": "locations: {workshop: {name: Workshop, on_enter: [{open: stock}]}}"})
	g := wldOpenWith(t, Options{FS: fsys, Location: "workshop"})
	wldFrames(t, g, 30)
	wldPlay(t, g, "press z\nwait 10\n", 50)
	if !g.talk.active || g.st.Vars["boards"] != float64(1) {
		t.Fatalf("before reload: %v", g.st.Vars)
	}
	g.reload()
	wldFrames(t, g, 10)
	if !g.talk.active || g.st.Vars["boards"] != float64(1) {
		t.Fatalf("reload dialogue: %v talk %v", g.st.Vars, g.talk.active)
	}
	wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
	if g.st.Vars["boards"] != float64(1) || g.st.Vars["finished"] != float64(1) {
		t.Fatalf("reload duplicated/lost commands: %v", g.st.Vars)
	}
	if s := sysAs[*authorScreen](t, g.stack[len(g.stack)-1]); s.running {
		t.Fatal("list still running")
	}
}
