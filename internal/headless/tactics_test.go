package headless

import (
	"reflect"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestActorTacticsRoundTrip(t *testing.T) {
	db := testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
	st := testgame.State(db, 1)
	plan := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual, Rows: []data.TacticRow{
		{Enabled: true, Condition: "any", Target: "enemy", Action: "gone"},
		{Enabled: true, Condition: "any", Target: "enemy", Action: "fire"},
	}}
	if err := st.Actors["scout"].SetTactics(plan); err != nil {
		t.Fatal(err)
	}
	paths := Explore(db, st, []data.Command{{Battle: "one"}}, Config{NoArrive: true})
	if len(paths) != 3 {
		t.Fatalf("勝ち・負け・逃げの枝がない %v", paths)
	}
	for _, path := range paths {
		if len(path.Errs) != 0 || !reflect.DeepEqual(path.St.Actors["scout"].Tactics, &plan) {
			t.Fatalf("分岐探索の保存コピーで作戦を失った %v", path.Errs)
		}
	}
	saved := paths[0].St
	d := New(db, saved)
	var decisions []battle.Decision
	d.OnDecision = func(v battle.Decision) { decisions = append(decisions, v) }
	d.fight("one")
	if len(decisions) == 0 || decisions[0].Selected != 1 || decisions[0].Rules[0].Reason != battle.RuleInvalidTactic || decisions[0].Skill != "fire" {
		t.Fatalf("描画なしの戦闘で作戦を使わない %+v", decisions)
	}
	saved.Actors["scout"].Tactics.Rows[0].Action = "attack"
	if st.Actors["scout"].Tactics.Rows[0].Action != "gone" {
		t.Fatal("探索先の変更が元セーブへ波及した")
	}
}
