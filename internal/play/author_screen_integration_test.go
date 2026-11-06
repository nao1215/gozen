package play

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/session"
	"github.com/nao1215/gozen/internal/testgame"
)

// The visible answer index must identify the same authored action in both
// frontends, including when a preceding entry is filtered out.
func TestAuthorScreenSessionEntryContract(t *testing.T) {
	for _, amount := range []int{0, 2} {
		t.Run(strconv.Itoa(amount), func(t *testing.T) {
			fsys := wldWorldFiles(t, `screens:
  stock:
    params: {amount: number}
    title: 'Stock {arg.amount}'
    entries:
    - {label: Hidden, when: 'arg.amount < 0', do: [{set: {var.selected: 99}}]}
    - {label: 'Collect {arg.amount}', when: 'arg.amount > 0', do: [{set: {var.selected: 1}}]}
    - {label: Inspect, text: 'Amount {arg.amount}', do: [{set: {var.selected: 2}}]}
`, map[string]string{"data/stock.yaml": fmt.Sprintf(`events:
  inspect_stock:
    do:
    - open: {screen: stock, args: {amount: '%d'}}
    - set: {var.closed: 1}
`, amount)})
			fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
			g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Seed: 17})
			wldFrames(t, g, 30)
			sg, err := session.Load(fsys, ".")
			if err != nil {
				t.Fatal(err)
			}
			ss, err := session.New(sg, session.Config{Seed: 17})
			if err != nil {
				t.Fatal(err)
			}
			if err := ss.Start("- call: inspect_stock"); err != nil {
				t.Fatal(err)
			}
			wldStartEvent(t, g, "inspect_stock")
			wldFrames(t, g, 4)
			q := ss.Next()
			view := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
			want := []string{"Inspect"}
			indices := []int{2}
			if amount > 0 {
				want = []string{"Collect 2", "Inspect"}
				indices = []int{1, 2}
			}
			labels := make([]string, len(view.entries))
			for i, e := range view.entries {
				labels[i] = e.Label
			}
			if q.Kind != "open" || q.Text != fmt.Sprintf("Stock %d", amount) || q.Text != view.title || !reflect.DeepEqual(q.Options, want) || !reflect.DeepEqual(labels, want) || !reflect.DeepEqual(view.indices, indices) {
				t.Fatalf("session=%+v player=%s/%v/%v", q, view.title, labels, view.indices)
			}
			// Caller-owned request labels must not alter subsequent presentation.
			q.Options[0] = "Changed"
			if got := ss.Next(); !reflect.DeepEqual(got.Options, want) {
				t.Fatal(got)
			}
			ss.Answer(len(want) - 1)
			view.list.sel = len(want) - 1
			wldPlay(t, g, "press z\nwait 8\n", 40)
			q = ss.Next()
			var selected float64
			if err := ss.Query("var.selected", &selected); err != nil {
				t.Fatal(err)
			}
			if selected != 2 || g.st.Vars["selected"] != float64(2) || q.Kind != "open" {
				t.Fatalf("wrong authored action: %v/%v/%+v", selected, g.st.Vars, q)
			}
			ss.Answer(-1)
			ss.Next()
			wldPlay(t, g, "press x\nwait 8\n", 40)
			var closed float64
			if err := ss.Query("var.closed", &closed); err != nil {
				t.Fatal(err)
			}
			if closed != 1 || g.st.Vars["closed"] != float64(1) {
				t.Fatal("opener did not continue once")
			}
		})
	}
}

// Cross-load at every semantic input boundary. Both frontends must preserve the
// same caller and exactly-once effects; differing seeds must not affect replay.
func TestAuthorScreenContinuationAcrossFrontends(t *testing.T) {
	fsys := wldWorldFiles(t, `screens:
  stock:
    params: {amount: number}
    title: 'Stock {arg.amount}'
    entries:
    - {label: Hidden, when: 'false'}
    - label: Collect
      do:
      - choice:
        - {label: Confirm, do: [{set: {var.total: 'var.total + arg.amount'}}]}
        - {label: Cancel}
      - open: receipt
      - say: Back.
  receipt:
    title: Receipt
    entries: [{label: Read, do: [{say: Receipt.}]}]
`, map[string]string{"data/stock.yaml": `events:
  visit:
    do:
    - set: {var.opened: 'var.opened + 1', var.roll: 'rand(1000)'}
    - say: Ready.
    - choice: [{label: Continue}]
    - open: {screen: stock, args: {amount: '2'}}
    - open: {screen: stock, args: {amount: '3'}}
    - set: {var.closed: 'var.closed + 1', var.next: 'rand(1000)'}
    - say: Finished.
`})
	fsys["game.yaml"].Data = []byte(strings.Replace(string(fsys["game.yaml"].Data), "schema_version: 4", "schema_version: 10", 1))
	g := wldOpenWith(t, Options{FS: fsys, Location: "camp", Seed: 17})
	wldFrames(t, g, 30)
	sg, err := session.Load(fsys, ".")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	ss, err := session.Restore(sg, initial, session.Config{Seed: 17})
	if err != nil {
		t.Fatal(err)
	}
	if err := ss.Start("- call: visit"); err != nil {
		t.Fatal(err)
	}
	wldStartEvent(t, g, "visit")
	wldFrames(t, g, 4)
	roundtrip := func(want session.Request) {
		t.Helper()
		b, err := g.saveBytes()
		if err != nil {
			t.Fatal(err)
		}
		oracle, err := session.Restore(sg, b, session.Config{Seed: 99})
		if err != nil {
			t.Fatal(err)
		}
		if q := oracle.Next(); !reflect.DeepEqual(q, want) {
			t.Fatalf("player save: got %+v want %+v", q, want)
		}
		state := maps.Clone(g.st.Vars)
		_, err = g.st.Save()
		if err != nil {
			t.Fatal(err)
		}
		random, err := g.pcg.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}

		if want.Kind == "choice" && g.screenTrace.saw {
			// A validly shaped but edited transcript must not replace live play.
			bad, err := rpg.Parse(g.db, b, g.stateRng())
			if err != nil {
				t.Fatal(err)
			}
			bad.Resume.Cursor.Inputs[len(bad.Resume.Cursor.Inputs)-1].Request = strings.Repeat("0", 64)
			runner := g.events
			g.loadSavedGame(bad)
			if g.failure == nil || g.events != runner {
				t.Fatal("damaged continuation replaced the live runner")
			}
			g.failure = nil
			unchanged, err := g.saveBytes()
			if err != nil || !bytes.Equal(b, unchanged) {
				t.Fatalf("failed load changed save: %v", err)
			}
		}
		b, err = ss.Save()
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			st, err := rpg.Parse(g.db, b, g.stateRng())
			if err != nil {
				t.Fatal(err)
			}
			g.loadSavedGame(st)
			if g.failure != nil {
				t.Fatal(g.failure)
			}
			after := maps.Clone(g.st.Vars)
			_, err = g.st.Save()
			if err != nil {
				t.Fatal(err)
			}
			rnd, err := g.pcg.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(state, after) || !bytes.Equal(random, rnd) {
				t.Fatalf("restoration changed state or RNG: %v / %v", state, after)
			}
			b, err = g.saveBytes()
			if err != nil {
				t.Fatal(err)
			}
		}
		var got event.ReplayView
		if want.Kind == "open" {
			view := sysAs[*authorScreen](t, g.stack[len(g.stack)-1])
			got = event.ReplayView{Kind: "open", ID: view.name, Text: view.title}
			for _, e := range view.entries {
				got.Options = append(got.Options, e.Label)
			}
		} else {
			got, _ = event.ReplayRequest(g.req)
		}
		if !reflect.DeepEqual(session.Request(got), want) {
			t.Fatalf("restored view: %+v want %+v", got, want)
		}
	}
	next := func(kind, text string, script string, answer *int) {
		t.Helper()
		var answered []byte
		if answer != nil {
			ss.Answer(*answer)
			ss.Answer(*answer)
			answered, err = ss.Save()
			if err != nil {
				t.Fatal(err)
			}
		}
		if script != "" {
			wldPlay(t, g, script, 160)
		}
		q := ss.Next()
		if q.Kind != kind || q.Text != text {
			t.Fatalf("oracle: %+v want %s/%s", q, kind, text)
		}
		roundtrip(q)
		if answered != nil {
			st, err := rpg.Parse(g.db, answered, g.stateRng())
			if err != nil {
				t.Fatal(err)
			}
			g.loadSavedGame(st)
			if g.failure != nil {
				t.Fatal(g.failure)
			}
			// Re-saving before any update must retain an answered continuation too.
			if _, err := g.saveBytes(); err != nil {
				t.Fatal(err)
			}
			wldFrames(t, g, 12)
			roundtrip(q)
		}
	}
	choice := func(n int) *int { return &n }
	next("say", "Ready.", "", nil)
	next("choice", "", "wait 40\npress z\nwait 8\n", nil)
	next("open", "Stock 2", "press z\nwait 8\n", choice(0))
	for _, amount := range []string{"2", "3"} {
		for _, cancel := range []bool{true, false} {
			next("choice", "", "press z\nwait 8\n", choice(0))
			answer, script := 0, "press z\nwait 8\n"
			if cancel {
				answer, script = 1, "press down\npress z\nwait 8\n"
			}
			next("open", "Receipt", script, choice(answer))
			next("say", "Receipt.", "press z\nwait 8\n", choice(0))
			next("open", "Receipt", "wait 40\npress z\nwait 8\n", nil)
			next("say", "Back.", "press x\nwait 8\n", choice(-1))
			next("open", "Stock "+amount, "wait 40\npress z\nwait 8\n", nil)
		}
		if amount == "2" {
			next("open", "Stock 3", "press x\nwait 8\n", choice(-1))
		}
	}
	next("say", "Finished.", "press x\nwait 8\n", choice(-1))
	if g.st.Vars["total"] != float64(5) || g.st.Vars["closed"] != float64(1) {
		t.Fatal(g.st.Vars)
	}
	wldPlay(t, g, "wait 40\npress z\nwait 8\n", 160)
	if ss.Next().Kind != "" {
		t.Fatal("opener did not finish")
	}
}

// CPU-side drawing may repair view-local selection, but never saved state,
// simulation randomness, or the caller's drawing style.
func TestPlanningSurfaceDrawingPreservesState(t *testing.T) {
	for _, kind := range []string{"entities", "ensemble", "islands", "timeline", "workspace"} {
		for _, fullscreen := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fullscreen=%t", kind, fullscreen), func(t *testing.T) {
				g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
				p := &data.Planning{
					Calendar: data.Calendar{Start: "2030-08-01", End: "2030-08-31", Phases: []string{"morning"}},
					Entities: map[string]data.Entity{"potter": {Name: "Potter"}, "weaver": {Name: "Weaver"}},
					Activities: map[string]data.Activity{
						"pot":   {Name: "Pottery", Effort: 10, Assignee: "potter", From: "2030-07-20", To: "2030-09-01"},
						"cloth": {Name: "Cloth", Effort: 5, Assignee: "weaver"},
					},
				}
				g.db.Game.Planning = p
				v := data.PlanningView{Fullscreen: fullscreen, FaceShape: "square", Detail: "notes", Portrait: "portrait", Camera: "camera", Icon: "icon", Badge: "badge", Mask: "mask", Highlight: "highlight", Fields: []data.PlanningField{{Key: "skill", Label: "Skill", Maximum: 10, Marks: []float64{5}}}}
				g.db.Game.Screens = map[string]*data.AuthorScreen{"work": {Kind: kind, Title: "Workshop", View: v}}
				g.st = testgame.State(g.db, 1)
				g.st.Planning.Date = "2030-08-20"
				g.st.Planning.Activities["pot"].Observed = 2
				g.st.Planning.Entities["potter"].Observations = map[string]any{"notes": "Clay is ready.", "camera": true, "mask": true, "highlight": true, "skill": 4.0, "badge": "Host"}
				g.st.Planning.Entities["weaver"].Observations = map[string]any{"notes": "Thread is ready.", "camera": false, "skill": 8.0}
				s := newAuthorScreen(g, "work", nil, nil)
				s.target = "potter"
				before, err := json.Marshal(g.st)
				if err != nil {
					t.Fatal(err)
				}
				random, err := g.pcg.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				theme, shape := g.th, g.faceSquare
				dst := covScreen()
				for range 3 {
					s.draw(g, dst)
				}
				after, err := json.Marshal(g.st)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("drawing changed saved state", err)
				}
				randomAfter, err := g.pcg.MarshalBinary()
				if err != nil || !bytes.Equal(random, randomAfter) {
					t.Fatal("drawing consumed randomness", err)
				}
				if g.th != theme || g.faceSquare != shape {
					t.Fatal("screen drawing leaked its style")
				}
				if s.target != "potter" {
					t.Fatal("drawing lost an existing selected resident")
				}
			})
		}
	}
}

func TestTimelineCandidateDrawingKeepsSelectionAndState(t *testing.T) {
	for _, light := range []bool{false, true} {
		t.Run(strconv.FormatBool(light), func(t *testing.T) {
			g, _ := btlOpen(t, Options{Location: "camp"}, "", "")
			g.db.Game.Planning = &data.Planning{
				Calendar:   data.Calendar{Start: "2030-08-01", End: "2030-08-31", Phases: []string{"morning"}},
				Entities:   map[string]data.Entity{"weaver": {Name: "Weaver"}},
				Activities: map[string]data.Activity{"cloth": {Name: "Cloth", Assignee: "weaver", Effort: 10}},
			}
			g.db.Game.Screens = map[string]*data.AuthorScreen{
				"timeline": {Kind: "timeline", View: data.PlanningView{Fullscreen: true, Light: light, Rows: []data.TimelineRow{{Label: "Crafts", Items: []data.TimelineItem{{Activity: "cloth", Screen: "pick"}}}}}},
				"pick":     {Title: "Choose a maker", View: data.PlanningView{Portrait: "portrait", Fields: []data.PlanningField{{Key: "skill", Label: "Skill"}}}},
			}
			for i := range 12 {
				g.db.Game.Screens["pick"].Entries = append(g.db.Game.Screens["pick"].Entries, data.ScreenEntry{Label: fmt.Sprintf("Maker %d", i), Text: strings.Repeat("A careful maker.\n", 14), Target: data.IDRef{ID: "weaver"}, When: data.MustExpr("!flag.hidden")})
			}
			g.st = testgame.State(g.db, 1)
			g.st.Planning.Entities["weaver"].Observations["skill"] = 7.0
			parent := newAuthorScreen(g, "timeline", nil, nil)
			child := newAuthorScreen(g, "pick", nil, nil)
			dst := covScreen()
			child.draw(g, dst) // The same candidates also work without a timeline.
			child.timelineContext, child.timelineActivity = parent, "cloth"
			child.layoutTimelineCandidates(g)
			child.list.sel = 11
			child.list.scroll()
			child.candidatePage = 1
			before, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			theme := g.th
			child.draw(g, dst)
			if child.list.sel != 11 || child.candidatePage != 1 || child.list.top == 0 || g.th != theme {
				t.Fatal("candidate drawing lost navigation or leaked its theme")
			}
			after, err := json.Marshal(g.st)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("candidate drawing changed state", err)
			}
			g.st.Flags["hidden"] = true
			child.refresh(g)
			child.draw(g, dst)
			if len(child.entries) != 0 || child.list.sel != 0 {
				t.Fatal("empty candidate refresh retained stale selection")
			}
		})
	}
}

// Rendering variants must preserve action identity and must not consume game
// randomness. This checks CPU-side scene behavior, not rendered GPU pixels.
func TestAuthoredCardDrawingKeepsActionBindings(t *testing.T) {
	for _, tc := range []struct {
		name  string
		entry data.ScreenEntry
	}{
		{"small portrait", data.ScreenEntry{Icon: "x.png", Portrait: true, Bounds: []int{40, 120, 140, 160}, Text: "Potter"}},
		{"large portrait", data.ScreenEntry{Icon: "x.png", Portrait: true, Bounds: []int{40, 120, 240, 420}, Text: "Ready for work."}},
		{"muted portrait", data.ScreenEntry{Icon: "x.png", Muted: data.Expr{Expr: testgame.Expr("true")}, Bounds: []int{40, 120, 320, 120}}},
		{"bottom icon", data.ScreenEntry{Icon: "x.png", Bounds: []int{40, 120, 240, 240}, Text: "Workshop\nAvailable"}},
		{"primary card", data.ScreenEntry{Tone: "primary", Bounds: []int{40, 120, 320, 440}}},
		{"note card", data.ScreenEntry{Tone: "note", Bounds: []int{40, 120, 320, 120}}},
		{"contained picture", data.ScreenEntry{Picture: "x.png", PictureFit: "contain", Bounds: []int{40, 120, 320, 240}}},
		{"cover picture", data.ScreenEntry{Picture: "x.png", Bounds: []int{40, 120, 320, 240}, Text: "Workshop"}},
		{"standing bust", data.ScreenEntry{Bust: &data.EntryBust{Picture: "x.png", Width: .5}, Bounds: []int{40, 120, 320, 240}}},
		{"bare bust", data.ScreenEntry{Bust: &data.EntryBust{Picture: "x.png", Width: 1, Bare: true}, Bounds: []int{40, 120, 320, 240}}},
		{"top bust", data.ScreenEntry{Bust: &data.EntryBust{Picture: "x.png", Height: .5, Fit: "contain"}, Bounds: []int{40, 120, 320, 240}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, s := openPartsBoard(t)
			entry := tc.entry
			entry.Label, entry.Do = "Count", g.db.Game.Screens[s.name].Entries[1].Do
			entry.Dot, entry.Sign = "#55aa66", "x.png"
			def := g.db.Game.Screens[s.name]
			def.Entries = []data.ScreenEntry{entry}
			def.View.Light = true
			def.View.Window = "Workshop"
			*s = *newAuthorScreen(g, s.name, s.done, nil)
			before, err := json.Marshal(g.st)
			if err != nil {
				t.Fatal(err)
			}
			random, err := g.pcg.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			theme := g.th
			s.keyed = true
			s.draw(g, covScreen())
			after, err := json.Marshal(g.st)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("drawing changed game state", err)
			}
			randomAfter, err := g.pcg.MarshalBinary()
			if err != nil || !bytes.Equal(random, randomAfter) || g.th != theme {
				t.Fatal("drawing leaked style or randomness", err)
			}
			if len(s.indices) != 1 || s.indices[0] != 0 || s.list.sel != 0 {
				t.Fatal("drawing changed the action binding")
			}
			r := entryBounds(entry)
			wldPlay(t, g, fmt.Sprintf("click %d %d\nwait 8\n", r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2), 40)
			if g.st.Vars["counted"] != float64(1) {
				t.Fatalf("drawn card did not activate exactly once: %v", g.st.Vars)
			}
		})
	}
}
