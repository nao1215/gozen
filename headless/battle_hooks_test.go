package headless_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/nao1215/gozen/data"
	"github.com/nao1215/gozen/event"
	"github.com/nao1215/gozen/headless"
	"github.com/nao1215/gozen/internal/testgame"
	"github.com/nao1215/gozen/rpg"
)

func TestBattleHooksOrderAndReturn(t *testing.T) {
	for _, result := range []int{event.BattleWon, event.BattleLost, event.BattleFled} {
		for _, next := range []string{"choice", "battle", "end"} {
			t.Run(fmt.Sprintf("%d/%s", result, next), func(t *testing.T) {
				db := testgame.Load(t, map[string]string{
					"game.yaml": testgame.Game + "  before: prepare\n  after: finish\n",
					"data/hooks.yaml": `events:
  prepare:
    do: [{set: {var.energy: "var.energy - 1", var.prepared: "var.prepared + 1"}}]
  finish:
    do:
      - set: {var.finished: "var.finished + 1", var.observed: "var.branch"}
      - move: camp
  encounter:
    do:
      - battle: one
        on_win: [{set: {var.branch: 1}}]
        on_lose: [{set: {var.branch: 2}}]
        on_escape: [{set: {var.branch: 3}}]
`})
				cmds := append([]data.Command{}, db.Events["encounter"].Do...)
				switch next {
				case "choice":
					cmds = append(cmds, data.Command{Choice: []data.ChoiceOption{{Label: "Continue", Do: []data.Command{{Battle: "one"}}}}})
				case "battle":
					cmds = append(cmds, data.Command{Battle: "one"})
				}
				st := testgame.State(db, 17)
				st.Location, st.Vars["energy"] = "vault", 2.
				d := headless.New(db, st)
				calls := 0
				d.Result = func(*rpg.State, string) (int, bool) { calls++; return result, true }
				d.Choose = func(*rpg.State, event.Choice) int { t.Error("choice shown after return"); return 0 }
				if err := d.Run(context.Background(), cmds); err != nil {
					t.Fatal(err)
				}
				if calls != 1 || st.Location != "camp" || st.Vars["energy"] != 1. || st.Vars["prepared"] != 1. || st.Vars["finished"] != 1. || st.Vars["observed"] != float64(result) {
					t.Fatalf("calls=%d location=%s vars=%v", calls, st.Location, st.Vars)
				}
			})
		}
	}
}

func TestBattleHooksCancellationAndSequence(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		energy                    float64
		result                    int
		wantBattles, wantFinishes float64
		over                      bool
	}{
		{"before move cancels", 0, event.BattleWon, 0, 0, false},
		{"each encounter once", 4, event.BattleWon, 2, 2, false},
		{"ordinary loss", 4, event.BattleLost, 1, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testgame.Load(t, map[string]string{"game.yaml": testgame.Game + "  before: prepare\n  after: finish\n", "data/hooks.yaml": `events:
  prepare:
    do:
      - if: "var.energy <= 0"
        then: [{move: camp}]
        else: [{set: {var.energy: "var.energy - 1"}}]
  finish:
    do: [{set: {var.finished: "var.finished + 1"}}]
`})
			st := testgame.State(db, 18)
			st.Location, st.Vars["energy"], st.Vars["finished"] = "vault", tc.energy, 0.
			d := headless.New(db, st)
			calls := 0.
			d.Result = func(*rpg.State, string) (int, bool) { calls++; return tc.result, true }
			if err := d.Run(context.Background(), []data.Command{{Battle: "one"}, {Battle: "one"}}); err != nil {
				t.Fatal(err)
			}
			if calls != tc.wantBattles || st.Vars["finished"] != tc.wantFinishes || d.Over != tc.over {
				t.Fatalf("battles=%g vars=%v over=%v", calls, st.Vars, d.Over)
			}
		})
	}
}
