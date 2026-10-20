package gamedata_test

import (
	"strings"
	"testing"

	"github.com/nao1215/gozen/internal/testgame"
)

func TestAutoTacticValidation(t *testing.T) {
	for _, version := range []string{"", "schema_version: 1\nid: neutral\n"} {
		for _, tc := range []struct{ name, change, replacement, want string }{
			{"正しい設定", "", "", ""},
			{"未知AI", "capability:", "missing:", "battle.auto_tactics.missing"},
			{"空AI", "capability:", "'':", "battle.auto_tactics."},
			{"未知条件", "condition: any", "condition: gone", "[0].condition"},
			{"未知対象", "target: enemies", "target: gone", "[0].target"},
			{"未知行動", "action: big", "action: gone", "[0].action"},
			{"範囲不一致", "action: big", "action: fire", "[0].target"},
			{"閾値省略", "condition: any", "condition: low_hp", "[0].percent"},
			{"閾値上限", "condition: any", "condition: low_hp, percent: 101", "[0].percent"},
			{"OFFも検査", "condition: any", "condition: gone", "[0].condition"},
			{"行のnull", "{enabled: true, condition: any, target: enemies, action: big}", "null", "[0]"},
			{"真偽値の型", "enabled: true", "enabled: 'true'", "[0].enabled"},
			{"閾値の型", "condition: any", "condition: low_hp, percent: '20'", "[0].percent"},
			{"閾値のnull", "condition: any", "condition: low_hp, percent: null", "[0].percent"},
			{"未知項目", "action: big", "action: big, typo: true", "[0].typo"},
		} {
			t.Run(version+tc.name, func(t *testing.T) {
				src := version + testgame.AutoTacticsGame
				if tc.change != "" {
					src = strings.Replace(src, tc.change, tc.replacement, 1)
				}
				if tc.name == "OFFも検査" {
					src = strings.Replace(src, "enabled: true", "enabled: false", 1)
				}
				_, err := testgame.TryLoad(map[string]string{
					"game.yaml":         src,
					"data/tactics.yaml": testgame.Tactics, "data/auto.yaml": testgame.AutoTacticsData,
				})
				if tc.want == "" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "game.yaml:") {
					t.Fatalf("位置付き診断 %q がない %v", tc.want, err)
				}
			})
		}
	}
}
