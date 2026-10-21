package rpg_test

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestMonsterReserveBreedAndSave(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 4)
	db.Game.MaxParty = 2
	a, ok := st.Recruit("mote", 1, 0, nil)
	if !ok || len(st.Party) != 2 {
		t.Fatalf("first capture: %q %v party=%v", a, ok, st.Party)
	}
	b, ok := st.Recruit("mote", 1, 0, nil)
	if !ok || len(st.ReserveIDs()) != 1 || st.ReserveIDs()[0] != b {
		t.Fatalf("reserve: %q %v %v", b, ok, st.ReserveIDs())
	}
	st.Actors[a].Sex, st.Actors[b].Sex = "male", "female"
	if value, err := testgame.Expr("monster." + a + ".sex").Eval(st.Env()); err != nil || value != "male" {
		t.Fatalf("monster sex query = %v, %v", value, err)
	}
	st.Actors[b].Sex = "male"
	if _, err := st.Breed(a, b, nil); err == nil {
		t.Fatal("same-sex breeding accepted")
	}
	st.Actors[b].Sex = "female"
	if !st.SwapMonster(b, a) || st.Party[1] != b || st.ReserveIDs()[0] != a {
		t.Fatalf("swap: party=%v reserve=%v", st.Party, st.ReserveIDs())
	}
	if _, err := st.Breed(a, b, []string{"fire", "fire"}); err == nil {
		t.Fatal("duplicate inherited skill accepted")
	}
	st.Actors[a].GainExp(40)
	st.Actors[b].GainExp(40)
	child, err := st.Breed(a, b, []string{"fire"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Actors[child].Monster != "boss" || st.Actors[child].Level != 1 || st.Actors[child].Plus < 1 || len(st.Actors[child].Learnt) != 1 {
		t.Fatalf("child = %+v", st.Actors[child])
	}
	if st.Party[1] != child || st.Actors[child].HP != st.Actors[child].Stat("mhp") {
		t.Fatalf("child did not take the parent's slot at full HP: party=%v HP=%d", st.Party, st.Actors[child].HP)
	}
	if st.Actors[a] != nil || st.Actors[b] != nil {
		t.Fatal("parents survived")
	}
	if st.Vars["bestiary.boss.caught"] != float64(1) {
		t.Fatalf("caught = %v", st.Vars)
	}
	saved, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	got, err := rpg.Parse(db, saved, rand.New(rand.NewPCG(4, 4))) //nolint:gosec // Fixed seed keeps the save round-trip reproducible.
	if err != nil {
		t.Fatal(err)
	}
	if got.Actors[child] == nil || got.Actors[child].Monster != "boss" || got.Actors[child].Plus != st.Actors[child].Plus || got.Actors[child].Sex != st.Actors[child].Sex || got.Actors[child].Name() != "Sentinel" {
		t.Fatalf("restored child = %+v", got.Actors[child])
	}
	var envelope map[string]any
	if err := json.Unmarshal(saved, &envelope); err != nil {
		t.Fatal(err)
	}
	state := envelope["state"].(map[string]any)
	actor := state["actors"].(map[string]any)[child].(map[string]any)
	delete(actor, "sex")
	legacy, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	old, err := rpg.Parse(db, legacy, rand.New(rand.NewPCG(4, 4))) //nolint:gosec // Fixed seed tests migration.
	if err != nil || old.Actors[child].Sex == "" || !old.LoadReport.Changed() {
		t.Fatalf("legacy sex migration = %v %+v", err, old)
	}
	if !got.ReleaseMonster(child) || got.Actors[child] != nil {
		t.Fatal("release did not remove the child")
	}
}

func TestMonsterSexUsesSessionRandomStream(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	first, second := testgame.State(db, 87), testgame.State(db, 87)
	for i := 0; i < 8; i++ {
		a, _ := first.Recruit("mote", 1, 0, nil)
		b, _ := second.Recruit("mote", 1, 0, nil)
		if first.Actors[a].Sex != second.Actors[b].Sex {
			t.Fatalf("capture %d differs: %s vs %s", i, first.Actors[a].Sex, second.Actors[b].Sex)
		}
	}
}
