package battle_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nao1215/gozen/battle"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestNumericPresenceBattle(t *testing.T) {
	for _, version := range []int{0, 1} {
		t.Run(fmt.Sprintf("版%d", version), func(t *testing.T) {
			main := strings.Replace(testgame.Data, "name: Mote", "name: Mote\n    actions: 0", 1)
			db := testgame.Load(t, map[string]string{
				"game.yaml":          fmt.Sprintf("schema_version: %d\nid: neutral\n", version) + testgame.Game,
				"data/main.yaml":     main,
				"data/presence.yaml": "states: {ward: {damage_rate: 0}}\nskills: {fixed: {effects: [{damage: '20', variance: 0}], crit: '0'}}",
			})
			b := battle.New(db, testgame.State(db, 1), "one")
			u, foe := b.Party[0], b.Foes[0]
			u.States["ward"] = 10
			steps := run(b, battle.Action{User: u, Pass: true})
			acts := 0
			for _, step := range steps {
				if step.Kind == battle.Act && step.User == foe {
					acts++
				}
			}
			want := 1
			if version == 1 {
				want = 0
			}
			if acts != want {
				t.Fatalf("敵の行動回数 %d、期待 %d", acts, want)
			}
			// 仲間の攻撃を守りのある敵へ当て、倍率０が実際にダメージを消すことを見る。
			foe.States["ward"] = 10
			before := foe.HP
			run(b, battle.Action{User: u, Skill: "fixed", Target: foe})
			wantDamage := 20
			if version == 1 {
				wantDamage = 0
			}
			if before-foe.HP != wantDamage {
				t.Fatalf("倍率０のダメージ %d、期待 %d", before-foe.HP, wantDamage)
			}
		})
	}
}
