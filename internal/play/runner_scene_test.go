package play

import (
	"encoding/json"
	"image"
	"math"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/layer2/gamepackage"
	"github.com/nao1215/gozen/internal/layer3/casual"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/savefile"
)

func beginTestRunner(t *testing.T, g *Game) *runnerScene {
	t.Helper()
	g.db.Game.SchemaVersion = data.SchemaVersion12
	g.db.Game.Runners = map[string]*data.RunnerDefinition{"course": {
		Title: "Short course", Width: 5, ViewRows: 8, PlayerRow: 5, StartX: 2.5, Radius: .15,
		Speed: 3, SteerSpeed: 7, SteerStart: 3.5, RampTicks: 12, BoostMax: 1,
		Lives: 2, MaxLives: 3, Stock: 1, MaxStock: 3, FinishRows: 8, Ahead: ".....",
		Tiles:  map[string]data.RunnerTile{".": {Kind: "empty"}, "#": {Kind: "wall"}},
		Stages: []data.RunnerStage{{ID: "first", Rows: []string{"....."}}},
	}}
	g.db.Events["course_test"] = &data.Event{Do: []data.Command{
		{RunGame: &data.RunGameCommand{ID: "course", Outputs: map[string]string{"score": "var.score", "cancelled": "flag.cancelled"}}},
		{SetV: data.Assignments{"var.rewards": data.MustExpr("var.rewards + 1")}},
		{Say: &data.Say{Text: "After course"}},
	}}
	g.events.Stop()
	g.events.Start(g.db.Events["course_test"].Do)
	g.req = g.events.Next()
	if _, ok := g.req.(event.RunGame); !ok {
		t.Fatalf("expected run request, got %T", g.req)
	}
	g.begin(g.req)
	if g.failure != nil || g.runnerActive() == nil {
		t.Fatalf("cannot start runner: %v", g.failure)
	}
	return g.runnerActive()
}

func TestRunnerSceneEntryAndExitConsumeInput(t *testing.T) {
	g := sysInn(t, Options{})
	s := beginTestRunner(t, g)
	g.in = input{}
	g.in.held[btnOK], g.in.pressed[btnOK] = true, true
	s.update(g)
	if !s.ready || s.view.Ticks != 0 {
		t.Fatal("entry press leaked into ready screen")
	}
	g.in = input{}
	s.update(g)
	g.in.pressed[btnOK] = true
	s.update(g)
	if s.ready || !s.blocked || s.view.Ticks != 0 {
		t.Fatal("start press advanced gameplay")
	}
	g.in = input{}
	s.update(g)
	s.update(g)
	if s.view.Ticks != 1 {
		t.Fatal("released start did not advance once")
	}
	s.rules.Cancel()
	s.view = s.rules.Snapshot()
	g.in.pressed[btnOK], g.in.held[btnOK], g.in.click, g.in.pheld = true, true, true, true
	s.leave(g)
	if g.runnerActive() != nil || g.req != nil || g.in.pressed[btnOK] || g.in.held[btnOK] || g.in.click || g.in.pheld {
		t.Fatal("exit input leaked into parent event")
	}
	if len(g.events.GameResults()) != 1 || !g.st.Flags["cancelled"] {
		t.Fatal("cancelled outcome was not accepted exactly once")
	}
}

func TestRunnerSceneSaveBoundaryAndOutcomeRestore(t *testing.T) {
	g := sysInn(t, Options{Seed: 13})
	s := beginTestRunner(t, g)
	for _, phase := range []string{"ready", "running", "paused", "unsubmitted"} {
		s.ready, s.paused = phase == "ready", phase == "paused"
		if phase == "unsubmitted" {
			s.rules.Cancel()
			s.view = s.rules.Snapshot()
		}
		if _, err := g.saveBytes(); err == nil {
			t.Fatalf("save accepted in %s", phase)
		}
	}
	s.leave(g)
	g.req = g.events.Next()
	g.begin(g.req)
	if g.st.Vars["rewards"] != float64(1) {
		t.Fatal("event result handling did not run")
	}
	saved, err := g.saveBytes()
	if err != nil {
		t.Fatal(err)
	}
	st, err := rpg.Parse(g.db, saved, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.loadSavedGame(st)
	if g.failure != nil || g.runnerActive() != nil || g.st.Vars["rewards"] != float64(1) || !g.st.Flags["cancelled"] {
		t.Fatalf("restoration reran game or reward: %v, %+v", g.failure, g.st.Vars)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(saved, &raw); err != nil {
		t.Fatal(err)
	}
	var cursor rpg.Cursor
	if err := json.Unmarshal(raw["cursor"], &cursor); err != nil {
		t.Fatal(err)
	}
	if len(cursor.Games) != 1 {
		t.Fatal("save has no recorded game outcome")
	}
	cursor.Games[0].Definition = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	raw["cursor"], err = json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	st, err = rpg.Parse(g.db, bad, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := g.st
	g.loadSavedGame(st)
	if g.failure == nil || g.st != before || g.st.Vars["rewards"] != float64(1) {
		t.Fatal("bad result replaced live state")
	}
	g.failure = nil // Expected load failure must not become a Close failure.
}

func TestRunnerPhoneFocusLossPausesChild(t *testing.T) {
	for _, pauseOption := range []bool{true, false} {
		t.Run(map[bool]string{true: "background_pauses", false: "background_runs"}[pauseOption], func(t *testing.T) {
			parent := phoneGame(t, data.PhoneCue{App: "store"})
			parent.db.Game.Phone = &data.PhoneApps{Apps: []data.PhoneApp{{ID: "child"}}}
			parent.packages = &phonePackages{opened: map[string]*gamepackage.Installed{"child": {FS: gameFiles(t), Release: gamepackage.Release{ID: "neutral/runner", Version: "1.0.0"}}}}
			if err := parent.startPhoneGame(parent.phone, "child"); err != nil {
				t.Fatal(err)
			}
			parent.phone.child.game.newGame()
			s := beginTestRunner(t, parent.phone.child.game)
			s.ready, s.blocked = false, false
			s.rules.Step(casual.Input{Steer: 1})
			s.view = s.rules.Snapshot()
			s.controls.pointers = []runnerGesture{{id: 1, x: 50, anchorX: 10}}
			child := parent.phone.child.game
			child.rendering = true
			if child.pauseForFocus(false) || s.paused {
				t.Fatal("embedded child polled window focus independently of its host")
			}
			parent.rendering, parent.pauseUnfocused = true, pauseOption
			if !parent.pauseForFocus(false) || !s.paused || !s.rules.Snapshot().Paused || !s.blocked || len(s.controls.pointers) != 0 {
				t.Fatal("host focus loss did not pause child and cancel capture")
			}
			before := s.rules.Snapshot()
			s.rules.Step(casual.Input{Steer: 1, Action: true})
			if s.rules.Snapshot().Ticks != before.Ticks || s.rules.Snapshot().X != before.X {
				t.Fatal("paused child advanced")
			}
			if parent.pauseForFocus(true) || !s.paused {
				t.Fatal("focus return resumed without explicit input")
			}
		})
	}
}

func TestRunnerSceneCountdownPausesAndAcknowledgesInput(t *testing.T) {
	g := sysInn(t, Options{})
	s := beginTestRunner(t, g)
	s.ready, s.blocked, s.countdown = false, false, 10
	offer := &scriptOffer{command: scriptCmd{verb: "press"}}
	g.in = input{esc: true, offer: offer}
	s.update(g)
	if !s.paused || s.countdown != 10 || !offer.accepted {
		t.Fatal("countdown consumed pause without pausing or acknowledging input")
	}
	g.in = input{}
	s.update(g) // Release the pause input.
	for range 5 {
		s.update(g)
	}
	if s.countdown != 10 || s.view.Ticks != 0 {
		t.Fatal("paused countdown advanced")
	}
	offer = &scriptOffer{command: scriptCmd{verb: "press"}}
	g.in = input{offer: offer}
	g.in.pressed[btnOK] = true
	s.update(g)
	if s.paused || !offer.accepted || s.countdown != 10 {
		t.Fatal("resume was not accepted without advancing countdown")
	}
	g.in = input{}
	s.update(g) // Release the resume input.
	for range 10 {
		s.update(g)
	}
	if s.countdown != 0 || s.view.Ticks != 0 {
		t.Fatal("countdown advanced simulation")
	}
	s.update(g)
	if s.view.Ticks != 1 {
		t.Fatal("completed countdown did not start simulation")
	}
}

func TestRunnerAuthoredRectFitsViewport(t *testing.T) {
	g := sysInn(t, Options{})
	g.db.Game.Screen.Width, g.db.Game.Screen.Height = 720, 900
	for _, tc := range []struct {
		name string
		w, h int
		ui   float64
		want [4]float64
	}{
		{"authored", 720, 900, 1, [4]float64{40, 60, 360, 720}},
		{"landscape", 720, 405, 1, [4]float64{216, 27, 162, 324}},
		{"portrait", 360, 540, 1, [4]float64{20, 75, 180, 360}},
		{"scaled_ui", 720, 900, 2, [4]float64{20, 30, 180, 360}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g.view.w, g.view.h, g.ui = tc.w, tc.h, tc.ui
			got := runnerAuthoredRect(g, []float64{40, 60, 360, 720})
			for i := range got {
				if math.Abs(got[i]-tc.want[i]) > 1e-9 {
					t.Fatalf("rect %v, want %v", got, tc.want)
				}
			}
			sw, sh := g.screen()
			if got[0] < 0 || got[1] < 0 || got[0]+got[2] > sw || got[1]+got[3] > sh {
				t.Fatalf("authored rect escaped viewport: %v inside %g,%g", got, sw, sh)
			}
		})
	}
}

func TestRunnerSceneKeyboardCanExitPausedAttempt(t *testing.T) {
	g := sysInn(t, Options{})
	s := beginTestRunner(t, g)
	s.ready, s.blocked = false, false
	g.in = input{esc: true}
	s.update(g)
	g.in = input{}
	s.update(g)
	g.in.pressed[btnDown] = true
	s.update(g)
	if !s.paused || !s.pauseExit {
		t.Fatal("keyboard could not select exit")
	}
	g.in = input{}
	g.in.pressed[btnOK] = true
	s.update(g)
	if g.runnerActive() != nil || !g.st.Flags["cancelled"] || len(g.events.GameResults()) != 1 {
		t.Fatal("keyboard exit did not return one cancelled result")
	}
}

func TestRunnerBoardClipAndPlayerGeometry(t *testing.T) {
	board := [4]float64{7.25, 35.25, 207, 368}
	if got, want := runnerBoardClip(board, 2, image.Rect(0, 0, 720, 900)), image.Rect(14, 70, 429, 807); got != want {
		t.Fatalf("pixel clip = %v, want %v", got, want)
	}
	if got, want := runnerBoardClip(board, 2, image.Rect(0, 0, 100, 200)), image.Rect(14, 70, 100, 200); got != want {
		t.Fatalf("bounded pixel clip = %v, want %v", got, want)
	}
	d := &data.RunnerDefinition{PlayerRow: 13, Visuals: &data.RunnerVisuals{PlayerSize: []float64{1, 1.25}, PlayerOffset: []float64{0, .875}}}
	got := runnerPlayerRect(d, 4.5, 14, 70, 46)
	want := [4]float64{198, 702.5, 46, 57.5}
	if got != want {
		t.Fatalf("player rect = %v, want %v", got, want)
	}
}

func TestRunnerResultsRefreshAutosaveAtSameLocation(t *testing.T) {
	g := sysInn(t, Options{Store: savefile.MemStore()})
	for attempt := 1; attempt <= 2; attempt++ {
		s := beginTestRunner(t, g)
		for range 1000 {
			s.rules.Step(casual.Input{})
			if s.rules.Snapshot().Status == "completed" {
				break
			}
		}
		s.view = s.rules.Snapshot()
		if s.view.Status != "completed" {
			t.Fatal("fixture did not complete")
		}
		s.leave(g)
		g.req = g.events.Next()
		g.begin(g.req)
		// Finish the authored post-result dialogue before returning to the idle scene.
		g.req, g.talk = nil, talk{}
		if next := g.events.Next(); next != nil {
			t.Fatalf("unexpected continuation: %T", next)
		}
		g.autoSave()
		body, err := g.store.Read(g.autoPath())
		if err != nil {
			t.Fatal(err)
		}
		saved, err := rpg.Parse(g.db, body, nil)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Vars["rewards"] != float64(attempt) || g.autoKey != g.st.Location {
			t.Fatalf("attempt %d did not refresh autosave: rewards=%v key=%q", attempt, saved.Vars["rewards"], g.autoKey)
		}
	}
}
