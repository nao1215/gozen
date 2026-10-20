package battle_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func autoCapabilityBattle(t *testing.T, seed uint64) (*battle.Battle, *rpg.State) {
	t.Helper()
	db := testgame.Load(t, map[string]string{
		"game.yaml":         testgame.AutoTacticsGame,
		"data/tactics.yaml": testgame.Tactics, "data/auto.yaml": testgame.AutoTacticsData,
	})
	db.Classes["ranger"].AI = data.AIRef{Name: "capability"}
	st := testgame.State(db, seed)
	st.Items["potion"] = 0
	return battle.New(db, st, "two"), st
}

func TestAutoTacticsFollowCapabilities(t *testing.T) {
	initial, st := autoCapabilityBattle(t, 6)
	db := initial.DB
	a := st.Actors["scout"]
	choose := func(want string) *battle.Battle {
		t.Helper()
		b := battle.New(db, st, "two")
		if got := b.Decide(b.Party[0]); got.Skill != want {
			t.Fatalf("選択 %s、必要 %s", got.Skill, want)
		}
		return b
	}
	choose("fire")
	a.GainExp(a.ExpFor(3) - a.Exp)
	a.Recover()
	choose("heal")
	a.Equip["weapon"] = "blade"
	equipped := choose("group_heal")
	delete(a.Equip, "weapon")
	choose("heal")
	if got := equipped.Decide(equipped.Party[0]); got.Skill != "group_heal" {
		t.Fatal("戦闘中に着脱が作戦へ漏れた")
	}
	a.Learnt = []string{"big"}
	choose("big")
	if got := initial.Decide(initial.Party[0]); got.Skill != "fire" {
		t.Fatal("戦闘中に新規習得が作戦へ漏れた")
	}
	a.Learnt = nil
	db.Classes["ranger"].Tree = []data.TreeNode{{ID: "care", Skill: "group_heal", Max: 1}}
	if !a.Raise("care") {
		t.Fatal("ツリーの習得に失敗")
	}
	choose("group_heal")
	a.ResetTree()
	choose("heal")
	st.Join("guide")
	b := battle.New(db, st, "two")
	if b.Decide(b.Party[1]).Skill != "fire" {
		t.Fatal("加入時に本人の習得技を反映しない")
	}
	encoded, err := st.Save()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rpg.Parse(db, encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	b = battle.New(db, loaded, "two")
	if b.Decide(b.Party[0]).Skill != "heal" || b.Decide(b.Party[1]).Skill != "fire" {
		t.Fatal("ロード後におまかせを再生成しない")
	}
	// Defer catalog changes made after import until the next battle.
	db.TacticActions["heal"].Use = "group_heal"
	if b.Decide(b.Party[0]).Skill != "heal" {
		t.Fatal("戦闘中にカタログの変更が漏れた")
	}
}

func TestAutoTacticsPreserveEvaluationOrder(t *testing.T) {
	for _, mode := range []string{"gambit", "priority"} {
		for seed := uint64(1); seed <= 24; seed++ {
			t.Run(fmt.Sprintf("%s/%d", mode, seed), func(t *testing.T) {
				scenario := func(auto bool) *battle.Battle {
					initial, st := autoCapabilityBattle(t, seed)
					db := initial.DB
					db.Game.Battle.AI = mode
					// An unlearned row still consumes RNG for when and chance. The order of items the actor does not hold is kept too.
					for _, index := range []int{0, 3, 4} {
						id := fmt.Sprintf("rng_%d", index)
						condition := data.TacticCondition{
							Name: "おまかせの条件", Kind: "template",
							When: decisionExpr("rand() < 0.9"), Chance: decisionExpr("0.8 + rand() * 0.2"), Priority: index, Weight: 2, Once: true,
						}
						db.TacticConditions[id] = &condition
						db.Game.Battle.AutoTactics["capability"][index].Condition = id
						r := &db.AI["capability"][index]
						r.When, r.Chance, r.Priority, r.Weight, r.Once = condition.When, condition.Chance, condition.Priority, condition.Weight, condition.Once
					}
					if !auto {
						db.Game.Battle.AutoTactics = nil
					}
					b := battle.New(db, st, "two")
					for _, u := range b.Party {
						u.Auto = true
					}
					return b
				}
				legacy, generated := scenario(false), scenario(true)
				var decisions []battle.Decision
				generated.OnDecision = func(d battle.Decision) { decisions = append(decisions, d) }
				if !bytes.Equal(decisionRun(t, legacy), decisionRun(t, generated)) {
					t.Fatal("既存 AI と生成作戦で行動・状態・次の乱数が変わった")
				}
				found := false
				for _, d := range decisions {
					for _, r := range d.Rules {
						found = found || r.Reason == battle.RuleSkillUnlearned
					}
				}
				if !found {
					t.Fatal("未習得の理由を既存の評価位置で記録しない")
				}
			})
		}
	}
}
