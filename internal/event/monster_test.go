package event_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
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

func TestRecruitOfferDoesNotShiftBattleRandomStream(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	plainDB := data.Fork(db)
	plainDB.Enemies["mote"].Recruit = nil
	withOffer, plain := testgame.State(db, 8), testgame.State(plainDB, 8)
	for _, tc := range []struct {
		game *data.DB
		st   *rpg.State
	}{
		{db, withOffer}, {plainDB, plain},
	} {
		r := event.New(tc.game, tc.st, &host{})
		r.Start([]data.Command{{Battle: "one"}})
		if _, ok := r.Next().(event.Battle); !ok {
			t.Fatal("no battle")
		}
		r.Answer(event.BattleWon)
		for r.Running() {
			if _, ok := r.Next().(event.Choice); ok {
				r.Answer(1) // Decline the optional recruit.
			}
		}
	}
	if got, want := withOffer.Rand.Uint64(), plain.Rand.Uint64(); got != want {
		t.Fatalf("adding a recruit offer shifted the next unrelated draw: %d != %d", got, want)
	}
}

func TestCaptureResultNamesReserveAndFirstCompanion(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 1)
	db.Game.MaxParty = len(st.Party)
	r := event.New(db, st, &host{})
	r.Start([]data.Command{{Capture: "mote"}})
	r.Next() // The saved-instance note precedes the staged result line.
	line, ok := r.Next().(event.Say)
	if !ok || !strings.Contains(line.Text, "joined your reserve") || !strings.Contains(line.Text, "First companion of this species") {
		t.Fatalf("reserve result = %+v", line)
	}
	if len(st.ReserveIDs()) != 1 {
		t.Fatalf("reserve = %v", st.ReserveIDs())
	}
	r.Start([]data.Command{{Capture: "mote"}})
	r.Next()
	line, ok = r.Next().(event.Say)
	if !ok || strings.Contains(line.Text, "First companion of this species") {
		t.Fatalf("repeat result = %+v", line)
	}
}
