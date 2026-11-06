package play

import (
	"bytes"
	"encoding/json"
	"image"
	"os"
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
	s := newAuthorScreen(g, "work", func() { closed++ }, nil)
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
	s := newAuthorScreen(g, "notes", nil, nil)
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
	for _, phase := range []string{"preview", "dialogue", "completed"} {
		t.Run(phase, func(t *testing.T) {
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
    do:
      - set: {var.opened: 'var.opened + 1'}
      - open: stock
      - set: {var.closed: 'var.closed + 1'}
`})
			g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Seed: 17})
			wldFrames(t, g, 30)
			path := g.slotPath(0)
			if err := g.writeSave(path); err != nil {
				t.Fatal(err)
			}
			previous, err := g.storeRead(path)
			if err != nil {
				t.Fatal(err)
			}
			wldStartEvent(t, g, "stock_open")
			wldFrames(t, g, 4)
			if phase != "preview" {
				wldPlay(t, g, "press z\nwait 8\n", 40)
			}
			if phase == "completed" {
				wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
			}
			wantBoards := float64(0)
			if phase != "preview" {
				wantBoards = 1
			}
			wantFinished := float64(0)
			if phase == "completed" {
				wantFinished = 1
			}
			assertValue := func(name string, want float64) {
				t.Helper()
				value, exists := g.st.Vars[name]
				if !exists && want == 0 {
					return
				}
				got, ok := value.(float64)
				if !ok {
					t.Fatalf("%s: expected number, got %T", name, value)
				}
				if got != want {
					t.Fatalf("%s: got %v, want %v", name, got, want)
				}
			}
			assertValue("opened", 1)
			assertValue("boards", wantBoards)
			assertValue("finished", wantFinished)
			assertValue("closed", 0)
			randomBefore, err := g.pcg.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			// Rejection must happen before overwriting a usable slot, without
			// changing the pending action, state, or random sequence.
			for range 2 {
				if saved, err := g.saveBytes(); err == nil || saved != nil {
					t.Fatal("save accepted an unrepresentable screen continuation")
				}
				if err := g.writeSave(path); err == nil {
					t.Fatal("slot write accepted an unrepresentable continuation")
				}
				after, err := g.storeRead(path)
				if err != nil || !bytes.Equal(previous, after) {
					t.Fatalf("rejected save changed the slot: %v", err)
				}
			}
			// A save menu above the screen cannot bypass the same check.
			menu := newSaveMenu(g, true, func() {})
			g.push(menu)
			menu.write(g, 0)
			if !strings.Contains(menu.msg, "Close this screen") {
				t.Fatalf("missing save refusal: %s", menu.msg)
			}
			g.pop()
			after, err := g.storeRead(path)
			if err != nil || !bytes.Equal(previous, after) {
				t.Fatalf("save menu changed the slot: %v", err)
			}
			randomAfter, err := g.pcg.MarshalBinary()
			if err != nil || !bytes.Equal(randomBefore, randomAfter) {
				t.Fatalf("rejection consumed randomness: %v", err)
			}
			// An uninterrupted action and close resumes the opener exactly once.
			if phase == "dialogue" {
				wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
			}
			wldPlay(t, g, "press x\nwait 8\n", 40)
			assertValue("opened", 1)
			assertValue("closed", 1)
			assertValue("boards", wantBoards)
			if phase == "dialogue" {
				wantFinished = 1
			}
			assertValue("finished", wantFinished)
			// Once closed, saving and repeated restoration are safe again.
			saved, err := g.saveBytes()
			if err != nil {
				t.Fatal(err)
			}
			wantRandom := g.st.Rand.Float64()
			for range 2 {
				restored, err := rpg.Parse(g.db, saved, nil)
				if err != nil {
					t.Fatal(err)
				}
				g.loadSavedGame(restored)
				wldFrames(t, g, 8)
				assertValue("opened", 1)
				assertValue("closed", 1)
				assertValue("boards", wantBoards)
				assertValue("finished", wantFinished)
				if got := g.st.Rand.Float64(); got != wantRandom {
					t.Fatalf("next random draw: got %v, want %v", got, wantRandom)
				}
			}
		})
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
	if _, err := g.saveBytes(); err == nil {
		t.Fatal("nested screen save was accepted")
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

func TestAuthorScreenReloadContinuation(t *testing.T) {
	for _, phase := range []string{"preview", "dialogue", "completed"} {
		t.Run(phase, func(t *testing.T) {
			fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries:
      - label: Add
        do: [{set: {var.boards: 'var.boards + 1'}}, {say: Added.}, {set: {var.finished: 'var.finished + 1'}}]
`, map[string]string{"data/stock.yaml": `events:
  stock_open:
    do:
      - open: stock
      - set: {var.closed: 'var.closed + 1'}
      - say: Returned.
      - set: {var.returned: 'var.returned + 1'}
`})
			g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Seed: 17, Development: true})
			wldFrames(t, g, 30)
			wldStartEvent(t, g, "stock_open")
			wldFrames(t, g, 4)
			if phase != "preview" {
				wldPlay(t, g, "press z\nwait 8\n", 40)
			}
			if phase == "completed" {
				wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
			}
			// A changed definition must wait for the entire suspended call.
			fsys["game.yaml"].Data = bytes.ReplaceAll(fsys["game.yaml"].Data, []byte("title: Stock"), []byte("title: New stock"))
			state, runner, db := g.st, g.events, g.db
			rng, err := g.pcg.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			g.reload()
			if g.st != state || g.events != runner || g.db != db || !g.live.pending {
				t.Error("reload replaced a suspended screen continuation instead of deferring")
			}
			after, err := g.pcg.MarshalBinary()
			if err != nil || !bytes.Equal(rng, after) {
				t.Fatalf("deferred reload changed randomness: %v", err)
			}
			if phase == "dialogue" {
				wldPlay(t, g, "wait 40\npress z\nwait 8\n", 100)
			}
			wldPlay(t, g, "press x\nwait 8\n", 40)
			if g.st.Vars["closed"] != float64(1) || !g.talk.active || !g.live.pending || g.db != db {
				t.Errorf("close lost caller or reloaded before caller finished: %v", g.st.Vars)
			}
			// Repeated reload requests after close must not bypass the queue
			// while the caller still has dialogue and effects to complete.
			g.reload()
			if !g.live.pending || g.db != db || !g.talk.active {
				t.Fatal("repeated reload discarded the caller")
			}
			wldPlay(t, g, "wait 40\npress z\nwait 12\n", 100)
			if g.live.pending || g.db.Game.Screens["stock"].Title != "New stock" {
				t.Fatal("queued reload was not applied after the caller completed")
			}
			if g.st.Vars["closed"] != float64(1) || g.st.Vars["returned"] != float64(1) {
				t.Fatalf("caller did not complete exactly once: %v", g.st.Vars)
			}
			if phase != "preview" && (g.st.Vars["boards"] != float64(1) || g.st.Vars["finished"] != float64(1)) {
				t.Fatalf("reload duplicated/lost entry commands: %v", g.st.Vars)
			}
		})
	}
}

func TestFullScreenContextUsesVisibleTargetActions(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"artisan": {}, "gardener": {}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"workshop": {
		Kind: "islands", Title: "Workshop", View: data.PlanningView{Fullscreen: true},
		Entries: []data.ScreenEntry{
			{Label: "Plan", Bounds: []int{20, 20, 120, 40}},
			{Label: "Rest", Target: data.IDRef{ID: "artisan"}},
			{Label: "Water", Target: data.IDRef{ID: "gardener"}},
			{Label: "Repair", Target: data.IDRef{ID: "artisan"}, When: data.Expr{Expr: testgame.Expr("flag.broken")}},
		},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "workshop", nil, nil)
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
	s := newAuthorScreen(g, "team", nil, nil)
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
		db.Game.Screens["team"].Entries = append(db.Game.Screens["team"].Entries, data.ScreenEntry{Label: "Assign", Target: data.IDRef{ID: "artisan"}})
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "team", nil, nil)
	s.openTarget(g, "artisan", entryBounds(data.ScreenEntry{Bounds: []int{900, 450, 200, 80}}))
	_, sh := g.screen()
	if s.popup.y < 0 || s.popup.y+s.popup.height() > sh || s.popup.rows > 6 {
		t.Fatalf("popup outside viewport: %+v", s.popup)
	}
	if s.popup.x+s.popup.w > 1280*.72 || s.popup.y < 88 || s.popup.y+s.popup.height()+144 > sh-100 {
		t.Fatal("action popup overlaps the sidebar, header or footer")
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
		g.db.Game.Screens["work"].Entries[i].Target = data.IDRef{ID: "artisan"}
	}
	s := newAuthorScreen(g, "work", nil, nil)
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

	s = newAuthorScreen(g, "work", nil, nil)
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
		db.Game.Screens["pick"].Entries = append(db.Game.Screens["pick"].Entries, data.ScreenEntry{Target: data.IDRef{ID: "maker"}, Label: "Maker"})
	}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "pick", nil, nil)
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
	// Bind public columns without exposing a hidden attribute through a dynamic key.
	db.Game.Screens["pick"].View.Fields = []data.PlanningField{{Key: "{arg.field}", Label: "{arg.caption}"}}
	for _, tc := range []struct{ key, want string }{{"load", "Work 3"}, {"icon", ""}, {"missing", ""}} {
		bound := newAuthorScreen(g, "pick", nil, map[string]any{"field": tc.key, "caption": "Work"})
		_, got := bound.candidateInfo(g, bound.entries[0])
		if got != tc.want {
			t.Fatalf("column %s: %q", tc.key, got)
		}
	}
	db.Game.Screens["pick"].View.Portrait = ""
	if newAuthorScreen(g, "pick", nil, nil).list.rowH != 44 {
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
	s := newAuthorScreen(g, "pick", nil, nil)
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

func TestAuthorScreenArguments(t *testing.T) {
	g := wldOpenWith(t, Options{FS: os.DirFS("../../docs/examples/event-arguments"), Location: "workshop", Seed: 17})
	wldFrames(t, g, 30)
	if err := g.events.StartBound("stock_visit", map[string]any{"amount": float64(2)}); err != nil {
		t.Fatal(err)
	}
	wldFrames(t, g, 10)
	screen := func(title string) *authorScreen {
		t.Helper()
		s := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
		if s.title != title {
			t.Fatalf("title %q, want %q", s.title, title)
		}
		return s
	}
	s := screen("Stock 2")
	if s.entries[0].Label != "Collect 2" {
		t.Fatal(s.entries)
	}
	wldPlay(t, g, "press z\nwait 4\npress down\npress z\nwait 12\n", 50)
	screen("Receipt 12")
	if g.st.Vars["total"] != nil && g.st.Vars["total"] != float64(0) {
		t.Fatal("cancel applied stock effect")
	}
	wldPlay(t, g, "press x\nwait 4\npress z\nwait 12\n", 50)
	screen("Stock 2")
	wldPlay(t, g, "press z\nwait 4\npress z\nwait 12\n", 50)
	screen("Receipt 12")
	if g.st.Vars["total"] != float64(2) {
		t.Fatalf("confirmed total: %v", g.st.Vars)
	}
	wldPlay(t, g, "press x\nwait 4\npress z\nwait 12\n", 50)
	screen("Stock 2")
	wldPlay(t, g, "press x\nwait 4\npress z\nwait 12\n", 50)
	if g.st.Vars["total"] != float64(2) {
		t.Fatal("closing repeated the effect")
	}
}

// Exercise recursive authored input, not a synthetic stack of empty views.
func TestAuthorScreenDepthBoundary(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  shelf:
    title: Shelf
    entries:
    - label: Inspect inner shelf
      do:
      - open: shelf
      - set: {var.returned: 'var.returned + 1'}
`, map[string]string{"data/shelves.yaml": `events:
  inspect_shelves:
    do:
    - open: shelf
    - set: {var.closed: 'var.closed + 1'}
`})
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Seed: 17})
	wldFrames(t, g, 30)
	wldStartEvent(t, g, "inspect_shelves")
	wldFrames(t, g, 4)
	for range 63 {
		wldPlay(t, g, "press z\nwait 4\n", 30)
	}
	if len(g.stack) != 65 {
		t.Fatalf("64 screens plus place: %d", len(g.stack))
	}
	before, err := g.pcg.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	wldPlay(t, g, "press z\nwait 8\n", 40)
	if len(g.stack) != 65 || g.st.Vars["returned"] != float64(1) {
		t.Fatalf("excess open did not return to its action: depth=%d vars=%v", len(g.stack), g.st.Vars)
	}
	// The non-event opening path uses the same bound, including beneath menus.
	done := 0
	var diagnostics []string
	originalLog := g.events.Log
	g.events.Log = func(err error) { diagnostics = append(diagnostics, err.Error()) }
	g.push(newSaveMenu(g, true, func() {}))
	g.openScreen("shelf", func() { done++ })
	g.events.Log = originalLog
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0], "nested too deep") {
		t.Fatalf("depth diagnostic: %v", diagnostics)
	}
	if done != 1 || len(g.stack) != 66 {
		t.Fatalf("direct open: done=%d depth=%d", done, len(g.stack))
	}
	g.pop()
	after, err := g.pcg.MarshalBinary()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("open consumed random state: %v", err)
	}
	// Freeing one slot permits opening again; the rejected request is not queued.
	wldPlay(t, g, "press x\nwait 8\npress z\nwait 8\n", 70)
	if len(g.stack) != 65 || g.st.Vars["returned"] != float64(2) {
		t.Fatalf("reopening: depth=%d vars=%v", len(g.stack), g.st.Vars)
	}
	for range 64 {
		wldPlay(t, g, "press x\nwait 4\n", 30)
	}
	if len(g.stack) != 1 || g.st.Vars["closed"] != float64(1) || g.st.Vars["returned"] != float64(65) {
		t.Fatalf("unwinding lost or repeated a caller: depth=%d vars=%v", len(g.stack), g.st.Vars)
	}
}

func TestRepeatedCandidateRefreshKeepsIdentity(t *testing.T) {
	db := testgame.Load(t, nil)
	if err := LoadFonts(db, gameFiles(t)); err != nil {
		t.Fatal(err)
	}
	db.Game.Planning = &data.Planning{}
	db.Game.Screens = map[string]*data.AuthorScreen{"pick": {View: data.PlanningView{Portrait: "icon"}, Entries: []data.ScreenEntry{
		{Each: &data.CallEach{Param: "resident", Items: []string{"potter", "weaver"}}, Label: "{arg.resident}", Text: strings.Repeat("Details\n", 20), When: data.MustExpr(`!flag.hide || arg.resident != "weaver"`)},
	}}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "pick", nil, nil)
	s.timelineContext = &authorScreen{}
	s.list.sel, s.candidatePage = 1, 2
	s = s.refreshed(g)
	if s.list.sel != 1 || s.candidatePage != 2 {
		t.Fatal("selected identity lost")
	}
	g.st.Flags["hide"] = true
	s = s.refreshed(g)
	if len(s.entries) != 1 || s.rows[0].Args["resident"] != "potter" || s.candidatePage != 0 {
		t.Fatal("removed resident retained detail page")
	}
}

func TestAuthorScreenMouseBack(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    title: Stock
    entries: [{label: Inspect, text: Inspect the stock.}]
`, nil)
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	closed := false
	s := newAuthorScreen(g, "stock", func() { closed = true }, nil)
	g.push(s)
	r := s.backBounds(g)
	g.in.click = true
	g.in.mouse.X, g.in.mouse.Y = r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	s.update(g)
	if !closed {
		t.Fatal("mouse back did not return to caller")
	}
	for _, view := range g.stack {
		if view == s {
			t.Fatal("closed screen remains on stack")
		}
	}
}

func TestWorkspacePanelDoesNotInterceptAction(t *testing.T) {
	fsys := wldWorldFiles(t, `planning:
  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [morning]}
  entities: {potter: {name: Potter}}
  activities: {shape: {name: Shape bowls, effort: 2}}
screens:
  studio:
    title: Studio
    kind: workspace
    view: {fullscreen: true}
    entries:
    - {label: Supplies, bounds: [32, 150, 300, 450]}
    - label: Select clay
      bounds: [42, 200, 260, 64]
      do: [{set: {var.selected: 'var.selected+1'}}]
`, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.openScreen("studio", func() {})
	wldPlay(t, g, "click 100 220\nwait 8\n", 40)
	if g.st.Vars["selected"] != float64(1) {
		t.Fatal("decorative panel intercepted the control", g.st.Vars)
	}
	wldPlay(t, g, "press z\nwait 8\n", 40)
	if g.st.Vars["selected"] != float64(2) {
		t.Fatal("keyboard focused a decorative panel", g.st.Vars)
	}
}

// A bounded workspace entry draws its picture inside the card, above the caption.
func TestWorkspaceEntryPictureIsDrawnInsideBounds(t *testing.T) {
	fsys := wldWorldFiles(t, `planning:
  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [morning]}
  entities: {potter: {name: Potter}}
screens:
  studio:
    title: Studio
    kind: workspace
    view: {fullscreen: true}
    entries:
    - {label: Visitor, text: Brings clay on Fridays., bounds: [32, 150, 300, 420], picture: pics/tall1.png}
`, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.openScreen("studio", func() {})
	drawn := map[string]bool{}
	g.pics.seen = func(path string, ok bool) { drawn[path] = drawn[path] || ok }
	g.Draw(covScreen())
	if !drawn["pics/tall1.png"] {
		t.Fatal("bounded entry picture was not drawn", drawn)
	}
	if h := entryPictureHeight(image.Rect(32, 150, 332, 570)); h != 420-12-entryCaptionHeight {
		t.Fatal("picture height leaves no caption", h)
	}
}

func TestAuthorScreenReturnToOpener(t *testing.T) {
	for _, tc := range []struct {
		name, destination          string
		running, bound, home, want bool
	}{
		{name: "same screen", destination: "parent", want: true},
		{name: "bound parent is a different invocation", destination: "parent", bound: true},
		{name: "unrelated screen", destination: "other"},
		{name: "running parent", destination: "parent", running: true, bound: true, want: true},
		{name: "running parent and home", destination: "other", running: true, home: true, want: true},
		{name: "running parent and ordinary destination", destination: "other", running: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testgame.Load(t, nil)
			db.Game.Screens = map[string]*data.AuthorScreen{
				"parent": {Title: "Parent {var.visits}"}, "child": {}, "other": {Home: tc.home},
			}
			g := &Game{db: db, st: testgame.State(db, 1)}
			parent := newAuthorScreen(g, "parent", nil, nil)
			parent.running = tc.running
			if tc.bound {
				parent.args = map[string]any{"person": "weaver"}
			}
			closed := 0
			child := newAuthorScreen(g, "child", func() { closed++ }, nil)
			g.stack = []scene{parent, child}
			g.st.Vars["visits"] = float64(4)
			if got := child.returnTo(g, tc.destination); got != tc.want {
				t.Fatalf("return = %t, want %t", got, tc.want)
			}
			if tc.want {
				if len(g.stack) != 1 || g.stack[0] != parent || closed != 1 || parent.running != tc.running {
					t.Fatalf("lost opener or callback: stack=%v closed=%d running=%t", g.stack, closed, parent.running)
				}
				if !tc.running && parent.title != "Parent 4" {
					t.Fatalf("idle parent was not refreshed: %q", parent.title)
				}
			} else if len(g.stack) != 2 || closed != 0 {
				t.Fatal("refused return changed the stack or called completion")
			}
		})
	}
}

func TestAuthorScreenSaveShortcutsKeepOpener(t *testing.T) {
	for _, saving := range []bool{false, true} {
		t.Run(map[bool]string{false: "load", true: "save"}[saving], func(t *testing.T) {
			fsys := wldWorldFiles(t, "screens: {notes: {title: Notes, entries: [{label: Read, text: Welcome}]}}\n", nil)
			g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
			wldFrames(t, g, 30)
			closed := 0
			g.openScreen("notes", func() { closed++ })
			s := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
			depth := len(g.stack)
			before, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			button := btnLoad
			if saving {
				button = btnSave
			}
			g.in = input{}
			g.in.pressed[button] = true
			s.update(g)
			menu := sysAs[*saveMenu](t, g.stack[len(g.stack)-1])
			if len(g.stack) != depth+1 || !menu.inPlay || menu.saving != saving || closed != 0 {
				t.Fatal("shortcut lost the screen or save/load mode")
			}
			g.in = input{pressed: [numButtons]bool{btnCancel: true}}
			menu.update(g)
			after, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			if len(g.stack) != depth || g.stack[depth-1] != s || closed != 0 || !bytes.Equal(before, after) {
				t.Fatal("canceling save/load changed the screen or simulation")
			}
		})
	}
}

func TestTargetPopupOutsideClickDoesNotActivate(t *testing.T) {
	db := testgame.Load(t, nil)
	db.Game.Planning = &data.Planning{Entities: map[string]data.Entity{"artisan": {}}}
	db.Game.Screens = map[string]*data.AuthorScreen{"work": {
		Kind: "islands", View: data.PlanningView{Fullscreen: true},
		Entries: []data.ScreenEntry{{Label: "Rest", Text: "Confirm rest", Target: data.IDRef{ID: "artisan"}}},
	}}
	g := &Game{db: db, st: testgame.State(db, 1)}
	s := newAuthorScreen(g, "work", nil, nil)
	s.openTarget(g, "artisan", image.Rect(100, 100, 200, 200))
	if s.popup == nil {
		t.Fatal("target actions did not open")
	}
	s.popupArmed = true
	g.in = input{click: true, mouse: image.Pt(0, 0)}
	s.updateSurface(g)
	if s.popup != nil || s.target != "" || s.running {
		t.Fatal("outside click activated or retained target actions")
	}
	s.openTarget(g, "absent", image.Rect(100, 100, 200, 200))
	if s.popup != nil || s.target != "" || s.popupArmed {
		t.Fatal("empty target retained stale confirmation")
	}
}

func TestWorkspaceFocusAfterLastActionDisappears(t *testing.T) {
	fsys := wldWorldFiles(t, `planning:
  calendar: {start: '2030-01-01', end: '2030-01-02', phases: [morning]}
  entities: {potter: {name: Potter}}
screens:
  studio:
    title: Studio
    kind: workspace
    view: {fullscreen: true}
    entries:
    - {label: Count, bounds: [32, 150, 260, 64], do: [{set: {var.counted: 'var.counted+1'}}]}
    - {label: Finish, bounds: [32, 250, 260, 64], when: '!flag.finished', do: [{set: {flag.finished: true}}]}
`, nil)
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp"})
	wldFrames(t, g, 30)
	g.openScreen("studio", func() {})
	wldPlay(t, g, "press down\nwait 2\npress z\nwait 8\n", 40)
	if !g.st.Flags["finished"] || len(top(t, g).entries) != 1 {
		t.Fatal("last action did not disappear")
	}
	wldPlay(t, g, "press z\nwait 8\n", 30)
	if g.st.Vars["counted"] != float64(1) {
		t.Fatal("confirmation stayed focused on a removed action", g.st.Vars)
	}
}
