package headless_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	"github.com/nao1215/gozen/internal/headless"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
	data "github.com/nao1215/gozen/schema"
)

func TestCanceledRunDoesNotChangeState(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 7)
	d := headless.New(db, st)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.Run(ctx, []data.Command{{Gold: data.Expr{Expr: testgame.Expr("20")}}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if st.Gold != 100 {
		t.Fatalf("canceled run changed gold to %d", st.Gold)
	}
}
func TestRunErrorsAreNotLostAtClearOrReusedAfterRestore(t *testing.T) {
	db := testgame.Load(t, nil)
	st := testgame.State(db, 8)
	d := headless.New(db, st)
	if err := d.Run(context.Background(), []data.Command{{}, {Clear: true}}); err == nil {
		t.Fatal("clear swallowed invalid command")
	}
	d.Restore(testgame.State(db, 9))
	if err := d.Run(context.Background(), db.Events["sub"].Do); err != nil {
		t.Fatal(err)
	}
	if d.St.Gold != 105 {
		t.Fatal(d.St.Gold)
	}
}
func TestExploreRejectsEmptyOutcomes(t *testing.T) {
	db := testgame.Load(t, nil)
	paths := headless.Explore(db, testgame.State(db, 1), []data.Command{{Battle: "one"}}, headless.Config{
		Outcomes: func(*rpg.State, event.Battle) []int { return nil },
	})
	if len(paths) != 1 || len(paths[0].Errs) != 1 {
		t.Fatalf("empty outcomes not reported: %+v", paths)
	}
}
