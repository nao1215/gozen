package headless

import (
	"bytes"
	"math/rand/v2"
	"testing"

	"github.com/nao1215/gozen/internal/event"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestExploreCaptureChoiceKeepsOriginalState(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 4)
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	paths := Explore(db, st, []data.Command{{Battle: "one"}}, Config{
		NoArrive: true,
		Fight:    func(_ *rpg.State, _ event.Battle) int { return event.BattleWon },
	})
	if len(paths) != 2 {
		t.Fatalf("capture choice paths = %d, want yes and no", len(paths))
	}
	accepted := 0
	for _, path := range paths {
		if len(path.Errs) != 0 {
			t.Fatalf("capture path failed: %v", path.Errs)
		}
		if path.St.Actors["monster_1"] != nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted paths = %d", accepted)
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("Explore changed the source state: %v", err)
	}
}

func TestExploreScriptedBreedCanBeSavedWithoutChangingSource(t *testing.T) {
	db := testgame.Load(t, testgame.MonsterFiles())
	st := testgame.State(db, 6)
	first, ok := st.RecruitWithSex("mote", 2, 0, nil, "male")
	if !ok {
		t.Fatal("first parent")
	}
	second, ok := st.RecruitWithSex("mote", 2, 0, nil, "female")
	if !ok {
		t.Fatal("second parent")
	}
	before, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	cmds, errs := data.ParseScript("- breed: {parents: [" + first + ", " + second + "]}\n")
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	paths := Explore(db, st, cmds, Config{NoArrive: true})
	if len(paths) != 1 || len(paths[0].Errs) != 0 {
		t.Fatalf("scripted breed paths: %+v", paths)
	}
	child := paths[0].St.Actors["monster_3"]
	if child == nil || child.Monster != "boss" || paths[0].St.Actors[first] != nil || paths[0].St.Actors[second] != nil {
		t.Fatalf("breed transition: child=%+v", child)
	}
	saved, err := paths[0].St.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, saved, rand.New(rand.NewPCG(6, 6))) //nolint:gosec // Deterministic save round trip.
	if err != nil || loaded.Actors["monster_3"] == nil || loaded.Actors["monster_3"].Monster != "boss" {
		t.Fatalf("bred child did not survive save: %v", err)
	}
	after, err := st.Save()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("Explore changed the source state: %v", err)
	}
}
