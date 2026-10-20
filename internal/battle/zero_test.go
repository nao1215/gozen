package battle_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestZeroVarianceIsDeterministic(t *testing.T) {
	for _, tc := range []struct{ name, global, effect string }{
		{"共通設定のゼロ", "0", ""},
		{"効果のゼロで上書き", "0.2", ", variance: 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			game := "schema_version: 1\nid: neutral\n" + strings.Replace(testgame.Game, "variance: 0.0001", "variance: "+tc.global, 1)
			db := testgame.Load(t, map[string]string{"game.yaml": game, "data/zero.yaml": fmt.Sprintf(`
skills:
  fixed_hit: {scope: one_enemy, crit: "0", effects: [{damage: "1000001"%s}]}
  fixed_heal: {scope: self, effects: [{heal: "1000001"%s}]}
`, tc.effect, tc.effect)})
			db.Enemies["boss"].Stats["mhp"] = 10000000
			db.Classes["ranger"].Stats["mhp"] = ex("10000000")
			// Compare only damage and healing, at a size not hidden by rounding even with a tiny variance.
			for seed := uint64(1); seed <= 32; seed++ {
				for _, skill := range []string{"fixed_hit", "fixed_heal"} {
					st := testgame.State(db, seed)
					b := battle.New(db, st, "boss")
					u, target := b.Party[0], b.Foes[0]
					kind := battle.Damage
					if skill == "fixed_heal" {
						// Do not require fixed criticals or action order; look only at the amount healed.
						target, kind = u, battle.Heal
						u.HP = 1
					}
					steps := run(b, battle.Action{User: u, Skill: skill, Target: target})
					found := false
					for _, step := range steps {
						if step.Kind == kind && step.User == u && step.Target == target {
							found = true
							if step.Amount != 1000001 {
								t.Fatalf("seed %d、%s の量 %d", seed, skill, step.Amount)
							}
						}
					}
					if !found {
						t.Fatalf("seed %d、%s の効果が出ていない", seed, skill)
					}
				}
			}
		})
	}
}

func TestZeroDamageMultipliers(t *testing.T) {
	for _, field := range []string{"critical_rate", "guard_rate"} {
		t.Run(field, func(t *testing.T) {
			game := "schema_version: 1\nid: neutral\n" + strings.Replace(testgame.Game, "battle:\n", "battle:\n  "+field+": 0\n", 1)
			db := testgame.Load(t, map[string]string{"game.yaml": game, "data/zero.yaml": `
skills:
  certain_hit: {scope: one_enemy, crit: "1", effects: [{damage: "20"}]}
`})
			b := battle.New(db, testgame.State(db, 1), "one")
			u, target := b.Party[0], b.Foes[0]
			if field == "guard_rate" {
				target.Guard = true
			}
			before := target.HP
			found := false
			for _, step := range run(b, battle.Action{User: u, Skill: "certain_hit", Target: target}) {
				if step.Kind == battle.Damage && step.User == u && step.Target == target {
					found = true
					if !step.Crit || step.Amount != 0 {
						t.Fatalf("会心または防御の０が効かない: %+v", step)
					}
				}
			}
			if !found || target.HP != before {
				t.Fatal("ダメージ０が保たれていない")
			}
		})
	}
}

func TestZeroRowWeights(t *testing.T) {
	for _, tc := range []struct {
		front, back         int
		wantFront, wantBack bool
	}{{0, 1, false, true}, {1, 0, true, false}, {0, 0, true, true}} {
		t.Run(fmt.Sprintf("前%d後%d", tc.front, tc.back), func(t *testing.T) {
			game := strings.Replace(testgame.Game, "battle:\n", fmt.Sprintf("battle:\n  front_weight: %d\n  back_weight: %d\n", tc.front, tc.back), 1)
			db := testgame.Load(t, map[string]string{"game.yaml": game})
			seenFront, seenBack := false, false
			for seed := uint64(1); seed <= 32; seed++ {
				st := testgame.State(db, seed)
				st.Join("guide")
				st.Actors["guide"].Back = true
				b := battle.New(db, st, "two")
				for _, step := range run(b, battle.Action{User: b.Party[0], Skill: "guard"}, battle.Action{User: b.Party[1], Skill: "guard"}) {
					if step.Kind == battle.Damage && step.Target == b.Party[0] {
						seenFront = true
					}
					if step.Kind == battle.Damage && step.Target == b.Party[1] {
						seenBack = true
					}
				}
			}
			if seenFront != tc.wantFront || seenBack != tc.wantBack {
				t.Fatalf("狙われた列は前%v、後%v", seenFront, seenBack)
			}
		})
	}
}
