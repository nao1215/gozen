package casual

import (
	"math"
	"reflect"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
)

func runnerDefinition() *data.RunnerDefinition {
	return &data.RunnerDefinition{
		Width: 9, ViewRows: 16, PlayerRow: 13, StartX: 4.5, Radius: .15,
		Speed: 3.68, SteerSpeed: 7, SteerStart: 3.5, RampTicks: 12,
		BoostRate: .5, BoostMax: 1.3, Lives: 2, MaxLives: 9, LifeEvery: 100,
		Stock: 1, MaxStock: 3, RetryRows: 10, ClearTicks: 3, FinishRows: 16,
		Ahead: ".........",
		Tiles: map[string]data.RunnerTile{
			".": {Kind: "empty"}, "#": {Kind: "wall"}, "a": {Kind: "pickup", Value: 2},
			"l": {Kind: "life"}, "s": {Kind: "stock"},
		},
		Stages: []data.RunnerStage{{ID: "first", Rows: []string{".........", ".........", ".........", "........."}}},
	}
}

func newRunner(t *testing.T, def *data.RunnerDefinition) *Runner {
	t.Helper()
	r, err := New(def, Config{Seed: 7})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunnerSteeringAndBoost(t *testing.T) {
	t.Parallel()
	r := newRunner(t, runnerDefinition())
	for tick := 1; tick <= 13; tick++ {
		before := r.x
		r.Step(Input{Steer: 1, Boost: true})
		want := (3.5 + 3.5*float64(tick-1)/12) / 60
		if math.Abs(r.x-before-want) > 1e-12 {
			t.Fatalf("tick %d movement %g, want %g", tick, r.x-before, want)
		}
	}
	r.Step(Input{})
	before := r.x
	r.Step(Input{Steer: -1})
	if math.Abs(r.x-before+3.5/60) > 1e-12 {
		t.Fatalf("reversal did not reset acceleration: %g", r.x-before)
	}
	if math.Abs(r.boost-(1+13*.5/60)) > 1e-12 {
		t.Fatalf("boost lost when released: %g", r.boost)
	}
	r.Pause(true)
	paused := r.Snapshot()
	r.Step(Input{Steer: 1, Boost: true, Action: true})
	if !reflect.DeepEqual(paused, r.Snapshot()) {
		t.Fatal("paused step changed the attempt")
	}
	r.Pause(false)
	r.Step(Input{Steer: math.NaN()})
	if r.x != before-3.5/60 {
		t.Fatalf("nonfinite steering changed x: %g", r.x)
	}
}

func TestRunnerHalfRowContactAndEscape(t *testing.T) {
	t.Parallel()
	for _, escape := range []bool{false, true} {
		t.Run(map[bool]string{false: "contact", true: "escape"}[escape], func(t *testing.T) {
			t.Parallel()
			d := runnerDefinition()
			d.Ahead, d.StartX = "....#....", 4.5
			d.Speed, d.BoostMax, d.SteerSpeed, d.SteerStart = 30, 1, 60, 60
			d.Stages[0].Rows = make([]string, 40)
			for i := range d.Stages[0].Rows {
				d.Stages[0].Rows[i] = "........."
			}
			r := newRunner(t, d)
			for range 14 {
				r.advance()
			}
			if r.status != "running" || r.rows[13][4] != '#' {
				t.Fatal("insertion should not collide before halfway")
			}
			in := Input{}
			if escape {
				in.Steer = 1
			}
			r.Step(in)
			want := "missed"
			if escape {
				want = "running"
			}
			if r.status != want || r.scroll != .5 {
				t.Fatalf("status %s scroll %g, want %s at half", r.status, r.scroll, want)
			}
		})
	}
}

func TestRunnerFastTickCannotSkipContact(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	d.Speed, d.BoostMax = 60, 1
	d.Ahead = "....#...."
	r := newRunner(t, d)
	for range 15 {
		r.Step(Input{})
	}
	if r.status != "missed" || r.progress != 14 || r.scroll != .5 {
		t.Fatalf("fast tick skipped wall: %+v", r.Snapshot())
	}
}

func TestRunnerRetryKeepsPickupLedgerAndDistance(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	d.Ahead = "#...a...."
	r := newRunner(t, d)
	for range 14 {
		r.advance()
	}
	if r.score != 2 || r.collected != 1 {
		t.Fatal("pickup was not collected when its row arrived")
	}
	r.clear()
	r.boost = 1.3
	r.crash()
	if !r.Retry() || r.progress != 4 || r.distance != 14 || r.lives != 1 || r.boost != 1 || r.stock != 0 {
		t.Fatalf("incorrect rewind state: %+v", r.Snapshot())
	}
	if r.rows[3][0] != '#' {
		t.Fatal("retry must rebuild cleared walls")
	}
	for range 10 {
		r.advance()
	}
	if r.score != 2 || r.collected != 1 || r.distance != 24 || r.rows[13][4] != '.' {
		t.Fatalf("retry duplicated pickup or reset distance: %+v", r.Snapshot())
	}
	if r.Retry() {
		t.Fatal("live attempt accepted retry")
	}
}

func TestRunnerPreviousRowPickupAndMissedStreak(t *testing.T) {
	t.Parallel()
	r := newRunner(t, runnerDefinition())
	r.progress, r.behind = 20, true
	r.rows[14] = []byte("...aa....")
	r.x = 3.5
	r.pick()
	if r.score != 2 || r.streak != 1 || r.rows[14][3] != '.' {
		t.Fatal("pickup beside body was missed")
	}
	r.reachHalf()
	if r.streak != 0 {
		t.Fatal("uncollected ordinary pickup did not reset streak")
	}
	r.behind, r.streak = true, 4
	r.rows[14] = []byte("...ls....")
	r.passBy()
	if r.streak != 4 {
		t.Fatal("life and stock misses changed ordinary pickup streak")
	}
}

func TestRunnerActionOrderingAndFreeze(t *testing.T) {
	t.Parallel()
	r := newRunner(t, runnerDefinition())
	r.rows[13] = []byte("...#a....")
	r.x = 4.16
	r.Step(Input{Steer: -1, Action: true})
	if r.status != "missed" || r.stock != 1 {
		t.Fatalf("action bypassed steering crash: %+v", r.Snapshot())
	}
	r = newRunner(t, runnerDefinition())
	r.rows[0] = []byte("#...a....")
	r.ahead = []byte("...#.....")
	r.Step(Input{Action: true, Boost: true})
	if r.stock != 0 || r.rows[0][0] != '.' || r.rows[0][4] != 'a' || r.ahead[3] != '.' || r.ticks != 0 || r.boost != 1 {
		t.Fatalf("clear failed or advanced world: %+v", r.Snapshot())
	}
	for range 3 {
		r.Step(Input{Boost: true})
	}
	if r.ticks != 0 || r.scroll != 0 {
		t.Fatal("action freeze advanced road")
	}
	r.Step(Input{})
	if r.ticks != 1 {
		t.Fatal("action freeze never resumed")
	}
}

func TestRunnerStageFinishAndTerminalOwnership(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	d.Stages = []data.RunnerStage{{ID: "one", Speed: 4, Rows: []string{"........."}}, {ID: "two", Speed: 8, Rows: []string{"........."}}}
	d.FinishSpeed = 12
	r := newRunner(t, d)
	if r.speed() != 4 {
		t.Fatal("first stage speed missing")
	}
	r.advance()
	if r.speed() != 8 || r.Snapshot().Stage != 1 {
		t.Fatal("stage transition did not change speed at generation boundary")
	}
	r.advance()
	if r.speed() != 12 || !r.Snapshot().Finishing {
		t.Fatal("finishing speed missing")
	}
	for range d.FinishRows - 1 {
		r.advance()
	}
	if r.status != "running" {
		t.Fatal("completed before last course passed")
	}
	r.advance()
	before := r.Snapshot()
	r.Cancel()
	r.Step(Input{Action: true, Steer: 1})
	if r.Retry() || r.status != "completed" || !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("terminal result changed")
	}
}

func TestRunnerCopiesDefinitionAndObservations(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	r := newRunner(t, d)
	d.Tiles["."] = data.RunnerTile{Kind: "wall"}
	d.Stages[0].Rows[0] = strings.Repeat("#", d.Width)
	s := r.Snapshot()
	s.Rows[0] = "#########"
	r.advance()
	if r.blocked(r.x, 0) || string(r.ahead) != "........." {
		t.Fatal("caller mutation changed running definition")
	}
}

func TestRunnerSafetyBudgets(t *testing.T) {
	t.Parallel()
	r := newRunner(t, runnerDefinition())
	r.ticks = 24 * 60 * 60 * 60
	r.Step(Input{})
	if r.status != "failed" || r.Snapshot().Diagnostic == "" {
		t.Fatal("simulation budget did not fail explicitly")
	}
	r = newRunner(t, runnerDefinition())
	r.score = 1_000_000_000
	r.rows[13][4] = 'a'
	r.pick()
	if r.status != "failed" || r.score != 1_000_000_000 || r.collected != 0 || r.Snapshot().Diagnostic == "" {
		t.Fatal("score overflow was not rejected before mutation")
	}
}

func TestRunnerRejectsInvalidDefinition(t *testing.T) {
	t.Parallel()
	if _, err := New(nil, Config{}); err == nil {
		t.Fatal("nil definition accepted")
	}
	d := runnerDefinition()
	d.Speed = math.Inf(1)
	if _, err := New(d, Config{}); err == nil {
		t.Fatal("nonfinite speed accepted")
	}
}

func TestRunnerEventsKeepRuleOrder(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	d.LifeEvery = 1
	d.Stages[0].Clear = true
	r := newRunner(t, d)
	r.progress = 3
	r.rebuild()
	r.ahead = []byte(".........")
	r.rows[12] = []byte("...#a#...")
	r.advance()
	want := []string{"course", "stage_clear", "pickup", "life", "near_wall"}
	if len(r.events) != len(want) {
		t.Fatalf("events %+v, want %v", r.events, want)
	}
	for i, kind := range want {
		if r.events[i].Kind != kind {
			t.Fatalf("event %d = %+v, want %s", i, r.events[i], kind)
		}
	}
	if r.events[2].Tile != "a" || r.events[2].Streak != 1 || r.events[3].Tile != "" {
		t.Fatalf("pickup/earned-life identity lost: %+v", r.events)
	}
	s := r.Snapshot()
	s.Events[0].Kind = "modified"
	if r.events[0].Kind != "course" {
		t.Fatal("snapshot exposed event buffer")
	}
	r.Step(Input{})
	if len(r.events) != 0 {
		t.Fatalf("old events repeated next tick: %+v", r.events)
	}
}

func TestRunnerProgressCountsPassedCoursesAndSurvivesRetry(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	d.Stages = []data.RunnerStage{{ID: "one", Rows: []string{".........", "........."}}, {ID: "two", Rows: []string{".........", "........."}}}
	r := newRunner(t, d)
	for range 2 {
		r.advance()
	}
	if r.Snapshot().Stage != 1 || r.Snapshot().CompletedStages != 0 {
		t.Fatal("generated next stage was counted as player completion")
	}
	for range d.PlayerRow + 1 {
		r.advance()
	}
	if r.Snapshot().CompletedStages != 0 {
		t.Fatal("last row still under player was counted as completed")
	}
	r.advance()
	if r.Snapshot().CompletedStages != 1 || r.Snapshot().BestCompletedStages != 1 {
		t.Fatalf("passed stage not counted: %+v", r.Snapshot())
	}
	r.crash()
	if !r.Retry() {
		t.Fatal("retry rejected")
	}
	if r.Snapshot().CompletedStages != 0 || r.Snapshot().BestCompletedStages != 1 || r.Snapshot().Misses != 1 {
		t.Fatalf("retry lost best progress or misses: %+v", r.Snapshot())
	}
}

func TestRunnerDeterministicActions(t *testing.T) {
	t.Parallel()
	d := runnerDefinition()
	d.Ahead = "#...a...#"
	d.Stages[0].Rows = []string{"#..a....#", "#....a..#", "#...s...#", "#.......#", "#...l...#"}
	left, right := newRunner(t, d), newRunner(t, d)
	for tick := range 1200 {
		in := Input{Steer: float64(tick/17%3 - 1), Boost: tick%7 < 3, Action: tick%113 == 0}
		left.Step(in)
		right.Step(in)
		if left.status == "missed" {
			left.Retry()
			right.Retry()
		}
		if !reflect.DeepEqual(left.Snapshot(), right.Snapshot()) {
			t.Fatalf("identical input diverged at tick %d", tick)
		}
		if left.status == "failed" || left.status == "completed" {
			return
		}
	}
	t.Fatal("bounded course did not finish or fail")
}

func TestRunnerGiveUpIsFailureWithoutSpendingLife(t *testing.T) {
	t.Parallel()
	r := newRunner(t, runnerDefinition())
	r.GiveUp()
	if r.status != "running" {
		t.Fatal("give up interrupted a live run")
	}
	r.crash()
	r.GiveUp()
	if r.status != "failed" || r.lives != 2 || len(r.events) != 1 || r.events[0].Kind != "failed" {
		t.Fatalf("give-up outcome wrong: %+v", r.Snapshot())
	}
	r.Cancel()
	if r.status != "failed" || r.Retry() {
		t.Fatal("give-up result was overwritten")
	}
}
