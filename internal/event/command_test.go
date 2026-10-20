package event_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestCommandFieldRuntime(t *testing.T) {
	for _, outcome := range []int{event.BattleWon, event.BattleLost, event.BattleFled} {
		t.Run(fmt.Sprint(outcome), func(t *testing.T) {
			db := testgame.Load(t, map[string]string{
				"game.yaml": "schema_version: 1\nid: neutral\n" + testgame.Game,
				"data/z.yaml": `events: {contract: {do: [
{wait: 0},
{battle: one, fighters: [scout], retry: false,
 on_win: [{set: {var.result: 1}}], on_lose: [{set: {var.result: 2}}], on_escape: [{set: {var.result: 3}}]},
{choice: [{label: Yes, do: [{gold: 1}]}], prompt: "Gold {gold}"}
]}}`,
			})
			st := testgame.State(db, 1)
			r := event.New(db, st, &host{})
			r.Log = func(err error) { t.Error(err) }
			r.Start(db.Events["contract"].Do)
			if wait, ok := r.Next().(event.Wait); !ok || wait.Seconds != 0 {
				t.Fatalf("待ち時間０が失われた。%+v", wait)
			}
			if battle, ok := r.Next().(event.Battle); !ok || !battle.NoRetry || !battle.CanLose || strings.Join(battle.Fighters, ",") != "scout" {
				t.Fatalf("戦闘の付随項目が失われた。%+v", battle)
			}
			r.Answer(outcome)
			if choice, ok := r.Next().(event.Choice); !ok || choice.Prompt != "Gold 100" {
				t.Fatalf("問いが失われた。%+v", choice)
			}
			want := map[int]float64{event.BattleWon: 1, event.BattleLost: 2, event.BattleFled: 3}[outcome]
			if st.Vars["result"] != want {
				t.Fatalf("戦闘結果の枝が違う。%v", st.Vars)
			}
			r.Answer(0)
			if req := r.Next(); req != nil || st.Gold != 101 {
				t.Fatalf("選択の続きが違う。%v, %d", req, st.Gold)
			}
		})
	}
	// Even if handed to the Runner without passing load validation, a command with a wrong accompanying field is not executed.
	for _, src := range []string{`{gold: 50, fighters: []}`, `{gold: 50, retry: false}`, `{gold: 50, floor: 0}`} {
		t.Run(src, func(t *testing.T) {
			db := testgame.Load(t, nil)
			st := testgame.State(db, 1)
			var cmd data.Command
			if err := yaml.Unmarshal([]byte(src), &cmd); err != nil {
				t.Fatal(err)
			}
			r := event.New(db, st, &host{})
			var logged []error
			r.Log = func(err error) { logged = append(logged, err) }
			r.Start([]data.Command{cmd, {Done: "tile"}})
			if req := r.Next(); req != nil || st.Gold != 100 || len(logged) != 1 || !st.Done["tile"] {
				t.Fatalf("不正命令と内部命令の扱いが違う。%v, %d, %v, %v", req, st.Gold, logged, st.Done)
			}
		})
	}
}
