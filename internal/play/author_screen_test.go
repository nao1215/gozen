package play

import (
	"bytes"
	"encoding/json"
	"image"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/rpg"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestSurfaceTabsPreserveStateAndOpener(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{}
	db.Game.Screens = map[string]*data.AuthorScreen{}
	for _, name := range []string{"work", "people", "dates"} {
		db.Game.Screens[name] = &data.AuthorScreen{Title: name, Kind: "entities", View: data.PlanningView{Fullscreen: true, Tabs: []string{"work", "people", "dates"}}}
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	closed := 0
	s := newAuthorScreen(g, "work", func() { closed++ })
	g.stack = []scene{&authorScreen{}, s}
	state, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		g.in = input{pressed: [numButtons]bool{btnRight: true}}
		s.updateSurface(g)
		if len(g.stack) != 2 || closed != 0 {
			t.Fatal("tab navigation stacked a screen or resumed its opener")
		}
	}
	if s.name != "dates" {
		t.Fatalf("wrong keyboard tab: %s", s.name)
	}
	r := s.tabBounds(g)[0]
	g.in = input{click: true, mouse: image.Pt(r.Min.X+10, r.Min.Y+10)}
	s.updateSurface(g)
	if s.name != "work" {
		t.Fatal("pointer did not select the first tab")
	}
	after, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(state, after) {
		t.Fatal("navigation changed simulation state")
	}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	s.updateSurface(g)
	if len(g.stack) != 1 || closed != 1 {
		t.Fatal("close did not resume the original opener exactly once")
	}
}

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

func TestFullScreenContextUsesVisibleTargetActions(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"artisan": {}, "gardener": {}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"workshop": {
		Kind: "islands", Title: "Workshop", View: data.PlanningView{Fullscreen: true},
		Entries: []data.ScreenEntry{
			{Label: "Plan", Bounds: []int{20, 20, 120, 40}},
			{Label: "Rest", Target: "artisan"},
			{Label: "Water", Target: "gardener"},
			{Label: "Repair", Target: "artisan", When: data.Expr{Expr: testgame.Expr("flag.broken")}},
		},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "workshop", nil)
	ids, cells := s.surfaceTargets(g)
	if len(ids) != 2 || ids[0] != "artisan" {
		t.Fatalf("default seat order: %v", ids)
	}
	s.openTarget(g, ids[0], cells[0])
	if len(s.popupIndices) != 1 || s.entries[s.popupIndices[0]].Label != "Rest" {
		t.Fatalf("unrelated or invisible action leaked: %v", s.popupIndices)
	}
	g.st.Flags["broken"] = true
	s.refresh(g)
	s.openTarget(g, ids[0], cells[0])
	if len(s.popupIndices) != 2 || s.entries[s.popupIndices[1]].Label != "Repair" {
		t.Fatal("visibility was not refreshed")
	}
	if r := entryBounds(s.entries[0]); r.Dx() != 120 || r.Dy() != 40 {
		t.Fatal(r)
	}
	db.Game.Screens["workshop"].View.Visible = "present"
	g.st.Planning.Entities["gardener"].Observations["present"] = true
	ids, cells = s.surfaceTargets(g)
	if len(ids) != 1 || len(cells) != 1 || ids[0] != "gardener" {
		t.Fatalf("hidden resident retained an input target: %v", ids)
	}
	g.st.Planning.Entities["artisan"].Observations["present"] = true
	ids, cells = s.surfaceTargets(g)
	if len(ids) != 2 || len(cells) != 2 || ids[0] != "artisan" {
		t.Fatalf("revealed resident missing from input targets: %v", ids)
	}
}

func TestEntityDetailSelectionSurvivesRefresh(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"artisan": {}, "gardener": {}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"team": {
		Kind: "entities", View: data.PlanningView{Fullscreen: true, Detail: "notes"},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "team", nil)
	ids, cells := s.surfaceTargets(g)
	if len(ids) != 2 || len(cells) != 2 {
		t.Fatal("missing member targets")
	}
	s.openTarget(g, ids[1], cells[1])
	s.refresh(g)
	if s.target != "gardener" || s.popup != nil {
		t.Fatal("member selection lost or opened an action popup")
	}
}

func TestTargetActionsScrollInsideViewport(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Screens = map[string]*data.AuthorScreen{"team": {
		Kind: "islands", View: data.PlanningView{Fullscreen: true},
	}}
	for range 30 {
		db.Game.Screens["team"].Entries = append(db.Game.Screens["team"].Entries, data.ScreenEntry{Label: "Assign", Target: "artisan"})
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "team", nil)
	s.openTarget(g, "artisan", entryBounds(data.ScreenEntry{Bounds: []int{900, 450, 200, 80}}))
	_, sh := g.screen()
	if s.popup.y < 0 || s.popup.y+s.popup.height() > sh || s.popup.rows > 6 {
		t.Fatalf("popup outside viewport: %+v", s.popup)
	}
	if s.popup.x < 1280*.72 || s.popup.y < 40 {
		t.Fatal("action popup covers resident seats or has no heading space")
	}
	for range 29 {
		s.popup.step(1)
		s.popup.scroll()
	}
	shown := s.popup.shown()
	if s.popupIndices[s.popup.sel] != 29 || shown[len(shown)-1] != 29 {
		t.Fatal("last assignment is inaccessible")
	}
}

func TestScriptReadStopsAtAuthorScreen(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  desk:
    title: Desk
    entries: [{label: Continue, do: [{set: {flag.acted: true}}]}]
`, map[string]string{"data/desk.yaml": `events:
  greeting:
    do: [{say: Welcome.}, {open: desk}]
`})
	var trace bytes.Buffer
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Trace: &trace})
	wldFrames(t, g, 30)
	wldStartEvent(t, g, "greeting")
	wldPlay(t, g, "read 1\nsnapshot\n", 300)
	if !strings.Contains(trace.String(), `"t":"snapshot"`) || g.st.Flags["acted"] {
		t.Fatal("read did not stop at the screen before choosing an action")
	}
	wldPlay(t, g, "press ok\nwait 10\n", 30)
	if !g.st.Flags["acted"] {
		t.Fatal("the action was not selectable after reading")
	}
}

func TestAuthorScreenUntilIdleDoesNotActivateEntry(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries: [{label: Add, do: [{set: {var.boards: 'var.boards + 1'}}]}]
`, map[string]string{"data/stock.yaml": "locations: {workshop: {name: Workshop, on_enter: [{say: Welcome}, {open: stock}]}}"})
	g := wldOpenWith(t, Options{FS: fsys, Location: "workshop", Script: "wait 10\nuntil idle\nquit\n"})
	screen := wldScreen(g)
	for range 300 {
		if !wldStep(t, g, screen) {
			break
		}
	}
	if g.tick >= 300 || g.st.Vars["boards"] != nil {
		t.Fatalf("idle activated menu or timed out: tick=%d vars=%v", g.tick, g.st.Vars)
	}
	if _, ok := g.stack[len(g.stack)-1].(*authorScreen); !ok {
		t.Fatal("script did not reach menu")
	}
}

func TestTargetActionPreviewConfirmation(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  work:
    title: Work
    entries:
      - {label: Rest, text: 'Rest for this turn. Select again to confirm.', do: [{set: {var.boards: 'var.boards + 1'}}]}
      - {label: Repair, text: 'Repair for this turn.', do: [{set: {var.boards: 'var.boards + 2'}}]}
`, nil)
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.db.Game.Screens["work"].Kind = "islands"
	g.db.Game.Screens["work"].View.Fullscreen = true
	for i := range g.db.Game.Screens["work"].Entries {
		g.db.Game.Screens["work"].Entries[i].Target = "artisan"
	}
	s := newAuthorScreen(g, "work", nil)
	open := func() { s.openTarget(g, "artisan", image.Rect(0, 0, 100, 100)) }
	click := func(row int) {
		r := s.popup.rowRect(row)
		g.in = input{click: true, mouse: image.Pt(r.Min.X+10, r.Min.Y+10)}
		s.updateSurface(g)
	}
	open()
	before, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	click(0)
	after, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	if !s.popupArmed || s.running || !bytes.Equal(before, after) {
		t.Fatal("preview mutated state or started an event")
	}
	click(1)
	if !s.popupArmed || s.running {
		t.Fatal("switching rows confirmed the other action")
	}
	g.in = input{pressed: [numButtons]bool{btnCancel: true}}
	s.updateSurface(g)
	if s.popup != nil || s.running {
		t.Fatal("cancel started an action")
	}
	open()
	click(0)
	g.in = input{pressed: [numButtons]bool{btnDown: true}}
	s.updateSurface(g)
	if s.popupArmed {
		t.Fatal("moving selection retained confirmation")
	}
	click(0)
	click(0)
	if !s.running || s.popup != nil {
		t.Fatal("second selection did not start action")
	}
	g.in = input{}
	s.update(g)
	if g.st.Vars["boards"] != float64(1) {
		t.Fatalf("action: %v", g.st.Vars)
	}

	s = newAuthorScreen(g, "work", nil)
	open()
	click(0)
	g.in = input{pressed: [numButtons]bool{btnOK: true}}
	s.updateSurface(g)
	if !s.running {
		t.Fatal("Enter did not confirm the preview")
	}
	g.in = input{}
	s.update(g)
	if g.st.Vars["boards"] != float64(2) {
		t.Fatal("Enter did not execute exactly once")
	}
}

func TestCandidateListPublicValuesAndSelection(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{
		Calendar:   data.Calendar{Start: "2030-08-01", End: "2030-08-31", Phases: []string{"morning"}},
		Attributes: map[string]data.Attribute{"icon": {Type: "string", Initial: "private.png", Hidden: true}, "load": {Type: "number", Initial: 3}},
		Entities:   map[string]data.Entity{"maker": {Name: "Maker"}},
	}
	db.Game.Screens = map[string]*data.AuthorScreen{"pick": {Title: "Choose a maker", View: data.PlanningView{Portrait: "icon", Fields: []data.PlanningField{{Key: "load", Label: "Load"}}}}}
	for range 12 {
		db.Game.Screens["pick"].Entries = append(db.Game.Screens["pick"].Entries, data.ScreenEntry{Target: "maker", Label: "Maker"})
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "pick", nil)
	before, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	portrait, summary := s.candidateInfo(g, s.entries[0])
	if portrait != "" || summary != "Load 3" {
		t.Fatalf("hidden fallback or missing public value: %q %q", portrait, summary)
	}
	g.st.Planning.Entities["maker"].Observations["icon"] = "public.png"
	portrait, _ = s.candidateInfo(g, s.entries[0])
	if portrait != "public.png" {
		t.Fatal("public portrait missing")
	}
	delete(g.st.Planning.Entities["maker"].Observations, "icon")
	s.list.sel = 11
	s.list.scroll()
	if s.list.top == 0 || s.list.rowH != 64 {
		t.Fatal("candidate rows did not scroll")
	}
	r := s.list.rowRect(11)
	if r.Max.Y > 720 {
		t.Fatal("candidate clipped below viewport")
	}
	g.in = input{click: true, mouse: image.Pt(r.Min.X+20, r.Min.Y+20)}
	chose, cancel := s.list.update(g)
	if !chose || cancel || s.list.sel != 11 {
		t.Fatal("candidate hit region diverged")
	}
	after, err := json.Marshal(g.st.Planning)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("candidate browsing mutated simulation")
	}
	db.Game.Screens["pick"].View.Portrait = ""
	if newAuthorScreen(g, "pick", nil).list.rowH != 44 {
		t.Fatal("ordinary list compatibility lost")
	}
}

func TestCandidateExplanationPaging(t *testing.T) {
	db := testgame.Load(t, nil)
	if err := LoadFonts(db, gameFiles(t)); err != nil {
		t.Fatal(err)
	}
	db.Game.Planning = &data.Planning{}
	explanation := "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nFinal instruction"
	db.Game.Screens = map[string]*data.AuthorScreen{
		"pick": {View: data.PlanningView{Portrait: "icon"}, Entries: []data.ScreenEntry{
			{Label: "Maker", Text: explanation}, {Label: "Helper", Text: "Short explanation"},
		}},
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "pick", nil)
	parent := &authorScreen{}
	s.timelineContext = parent
	s.layoutTimelineCandidates(g)
	g.stack = []scene{parent, s}
	before, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	lines, pages := s.candidateTextPage()
	if pages != 3 || strings.Join(lines, "\n") != "One\nTwo\nThree\nFour\nFive\nSix" {
		t.Fatal(lines, pages)
	}
	// Mouse button and wheel expose the complete final instruction, without
	// selecting another candidate or running its authored action.
	r := s.candidatePageButtons(g)[1]
	g.in = input{click: true, mouse: r.Min.Add(image.Pt(8, 8))}
	s.update(g)
	if s.candidatePage != 1 {
		t.Fatal("next button failed")
	}
	g.in = input{wheel: -1, mouse: image.Pt(1000, 180)}
	s.update(g)
	lines, _ = s.candidateTextPage()
	if s.candidatePage != 2 || len(lines) != 1 || lines[0] != "Final instruction" || s.list.sel != 0 || s.running {
		t.Fatal("lost text or changed candidate", lines)
	}
	s.update(g)
	if s.candidatePage != 2 {
		t.Fatal("last page wrapped")
	}
	s = s.refreshed(g)
	g.stack[1] = s
	if s.candidatePage != 2 {
		t.Fatal("refresh lost page")
	}
	g.in = input{pressed: [numButtons]bool{btnLeft: true}}
	s.update(g)
	if s.candidatePage != 1 {
		t.Fatal("keyboard previous failed")
	}
	g.in = input{pressed: [numButtons]bool{btnRight: true}}
	s.update(g)
	if s.candidatePage != 2 {
		t.Fatal("keyboard next failed")
	}
	// Moving between candidates starts their explanation at the beginning.
	g.in = input{pressed: [numButtons]bool{btnDown: true}}
	s.update(g)
	if s.list.sel != 1 || s.candidatePage != 0 {
		t.Fatal("selection retained stale page")
	}
	g.in = input{wheel: -1, mouse: image.Pt(1000, 180)}
	s.update(g)
	if s.list.sel != 1 || s.candidatePage != 0 {
		t.Fatal("short text wheel selected a candidate")
	}
	// Wheel over the list still navigates candidates.
	g.in = input{wheel: 1, mouse: image.Pt(1000, 310)}
	s.update(g)
	if s.list.sel != 0 {
		t.Fatal("candidate list wheel stopped working")
	}
	g.in = input{rclick: true}
	s.update(g)
	if len(g.stack) != 1 || g.stack[0] != parent {
		t.Fatal("cancel lost parent")
	}
	after, err := json.Marshal(g.st)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("reading explanation changed saved state")
	}
	// A live refresh that removes the selected entry must reset its page.
	s.candidatePage = 2
	db.Game.Screens["pick"].Entries[0].When = data.Expr{Expr: testgame.Expr("false")}
	s = s.refreshed(g)
	if s.candidatePage != 0 {
		t.Fatal("removed entry retained page")
	}
}
