package event_test

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func gameResultDB(t *testing.T) *data.DB {
	t.Helper()
	db := testgame.Load(t, nil)
	db.Game.SchemaVersion = data.SchemaVersion12
	db.Game.Runners = map[string]*data.RunnerDefinition{"trail": {Width: 5, ViewRows: 8, PlayerRow: 5, StartX: 2.5, Radius: .15, Speed: 3, SteerSpeed: 7, SteerStart: 3, BoostMax: 1, Lives: 2, MaxLives: 3, Stock: 1, MaxStock: 2, FinishRows: 8, Ahead: ".....", Tiles: map[string]data.RunnerTile{".": {Kind: "empty"}}, Stages: []data.RunnerStage{{ID: "first", Rows: []string{"....."}}}}}
	db.Events["trial"] = &data.Event{Do: []data.Command{
		{RunGame: &data.RunGameCommand{ID: "trail", Outputs: map[string]string{"score": "var.score", "completed": "flag.finished"}}},
		{SetV: data.Assignments{"var.rewards": data.MustExpr("var.rewards + 1")}},
		{Say: &data.Say{Text: "Result"}},
	}}
	return db
}

func TestGameResultIdentityAndAtomicAssignments(t *testing.T) {
	db := gameResultDB(t)
	st := testgame.State(db, 8)
	r := event.New(db, st, nil)
	r.Start(db.Events["trial"].Do)
	q, ok := r.Next().(event.RunGame)
	if !ok {
		t.Fatal("missing game request")
	}
	if next, ok := r.Next().(event.RunGame); !ok || next.Identity != q.Identity {
		t.Fatal("pending request changed")
	}
	result := q.Identity
	result.Status, result.Score, result.Lives, result.Stock = "completed", 9, 2, 1
	for _, change := range []func(*rpg.GameOutcome){
		func(g *rpg.GameOutcome) { g.Seed++ }, func(g *rpg.GameOutcome) { g.Invocation++ }, func(g *rpg.GameOutcome) { g.ID = "other" }, func(g *rpg.GameOutcome) { g.Definition = "bad" }, func(g *rpg.GameOutcome) { g.Lives = 4 }, func(g *rpg.GameOutcome) { g.Status = "running" },
	} {
		bad := result
		change(&bad)
		if r.AnswerGame(bad) == nil {
			t.Fatal("invalid result accepted")
		}
		if len(r.GameResults()) != 0 || st.Flags["finished"] {
			t.Fatal("invalid result mutated state")
		}
	}
	db.Events["trial"].Do[0].RunGame.Outputs["stock"] = "invalid.target"
	if r.AnswerGame(result) == nil || st.Vars["score"] != nil || st.Flags["finished"] {
		t.Fatal("partial assignments committed")
	}
	delete(db.Events["trial"].Do[0].RunGame.Outputs, "stock")
	if err := r.AnswerGame(result); err != nil {
		t.Fatal(err)
	}
	if r.AnswerGame(result) == nil {
		t.Fatal("duplicate result accepted")
	}
	if _, ok := r.Next().(event.Say); !ok {
		t.Fatal("event did not continue")
	}
	if st.Vars["score"] != float64(9) || st.Vars["rewards"] != float64(1) || !st.Flags["finished"] {
		t.Fatalf("result not applied: %v %v", st.Vars, st.Flags)
	}
	copyResults := r.GameResults()
	copyResults[0].Score = 999
	if r.GameResults()[0].Score != 9 {
		t.Fatal("result storage alias")
	}
}

func TestGameOutcomeReconstruction(t *testing.T) {
	for _, status := range []string{"completed", "failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			db := gameResultDB(t)
			st := testgame.State(db, 17)
			r := event.New(db, st, nil)
			r.Start(db.Events["trial"].Do)
			request, ok := r.Next().(event.RunGame)
			if !ok {
				t.Fatal("missing game request")
			}
			result := request.Identity
			result.Status, result.Score, result.Lives = status, 12, 1
			if err := r.AnswerGame(result); err != nil {
				t.Fatal(err)
			}
			r.Next()
			nextRandom := st.Rand.Uint64()
			for _, kind := range []string{"valid", "missing", "extra", "seed", "definition"} {
				t.Run(kind, func(t *testing.T) {
					fresh := testgame.State(db, 17)
					replay := event.New(db, fresh, nil)
					replay.Start(db.Events["trial"].Do)
					results := r.GameResults()
					switch kind {
					case "missing":
						results = nil
					case "extra":
						results = append(results, result)
					case "seed":
						results[0].Seed++
					case "definition":
						results[0].Definition = "changed"
					}
					q, err := replay.ReplayTo(1, nil, results)
					if kind != "valid" {
						if err == nil {
							t.Fatal("invalid replay accepted")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if _, ok := q.(event.Say); !ok {
						t.Fatal("wrong replay destination")
					}
					if !reflect.DeepEqual(st.Vars, fresh.Vars) || !reflect.DeepEqual(st.Flags, fresh.Flags) || fresh.Rand.Uint64() != nextRandom {
						t.Fatal("reconstruction changed rewards or random sequence")
					}
				})
			}
		})
	}
}

func TestGameInvocationBudgetStopsBeforeUnanswerableRequest(t *testing.T) {
	db := gameResultDB(t)
	commands := make([]data.Command, 0, 8194)
	for range 4097 {
		commands = append(commands, db.Events["trial"].Do[:2]...)
	}
	st := testgame.State(db, 23)
	expected := testgame.State(db, 23)
	r := event.New(db, st, nil)
	var diagnostic error
	r.Log = func(err error) { diagnostic = err }
	r.Start(commands)
	for i := range 4096 {
		q, ok := r.Next().(event.RunGame)
		if !ok || q.Identity.Invocation != i+1 {
			t.Fatalf("invocation %d: %T", i+1, q)
		}
		if q.Identity.Seed != expected.Rand.Uint64() {
			t.Fatal("random sequence changed")
		}
		result := q.Identity
		result.Status = "cancelled"
		if err := r.AnswerGame(result); err != nil {
			t.Fatal(err)
		}
	}
	if q := r.Next(); q != nil {
		t.Fatalf("over-budget request: %T", q)
	}
	if diagnostic == nil || r.Running() || r.GamePending() {
		t.Fatal("limit left an unanswerable game pending")
	}
	if len(r.GameResults()) != 4096 || st.Vars["rewards"] != float64(4096) {
		t.Fatal("limit changed results or awarded an unplayed game")
	}
	if st.Rand.Uint64() != expected.Rand.Uint64() {
		t.Fatal("rejected invocation consumed randomness")
	}
	// The bound belongs to a replay root, not the player's lifetime.
	r.Start(db.Events["trial"].Do)
	q, ok := r.Next().(event.RunGame)
	if !ok || q.Identity.Invocation != 1 {
		t.Fatal("new event did not reset invocation count")
	}
}
