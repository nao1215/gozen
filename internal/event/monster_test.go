package event_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestCaptureAfterVictoryIsAChoice(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	for _, answer := range []int{0, 1} {
		st := testgame.State(db, 1)
		r := event.New(db, st, &host{})
		r.Log = func(err error) { t.Error(err) }
		r.Start([]data.Command{{Battle: "one"}})
		if _, ok := r.Next().(event.Battle); !ok {
			t.Fatal("no battle request")
		}
		r.Answer(event.BattleWon)
		line, ok := r.Next().(event.Say)
		if !ok || !strings.Contains(line.Text, "rises") || line.Recruit == nil || line.Recruit.Image == "" {
			t.Fatalf("no staged rising line: %+v", line)
		}
		choice, ok := r.Next().(event.Choice)
		if !ok || len(choice.Options) != 2 || choice.Options[0] != "Yes" || choice.Recruit == nil || choice.Recruit.Sex != line.Recruit.Sex {
			t.Fatalf("join request = %+v", choice)
		}
		if !strings.Contains(choice.Prompt, "♂") && !strings.Contains(choice.Prompt, "♀") {
			t.Fatalf("the offer has no sex sign: %q", choice.Prompt)
		}
		r.Answer(answer)
		for r.Running() {
			r.Next()
		}
		want := float64(1 - answer)
		if got := st.Vars["bestiary.mote.caught"]; answer == 0 && got != want || answer == 1 && got != nil {
			t.Fatalf("answer %d caught %v", answer, got)
		}
		if st.Vars["bestiary.mote.defeated"] != float64(1) {
			t.Fatalf("defeated = %v", st.Vars)
		}
		if answer == 0 {
			if got := st.Actors["monster_1"]; got == nil || got.Sex != choice.Recruit.Sex {
				t.Fatalf("joined monster sex differs from invitation: %+v", got)
			}
		}
	}
}
