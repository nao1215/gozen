package gamedata_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/testgame"
)

func TestAIRulesShareValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule data.Rule
		want string
	}{
		{"使用可能", data.Rule{Use: "fire"}, ""},
		{"品", data.Rule{Item: "potion"}, ""},
		{"行動なし", data.Rule{}, "exactly one"},
		{"行動二つ", data.Rule{Use: "fire", Item: "potion"}, "exactly one"},
		{"未知の技", data.Rule{Use: "missing"}, "unknown skill"},
		{"未知の品", data.Rule{Item: "missing"}, "unknown item"},
		{"使用不可の品", data.Rule{Item: "stick"}, "use がなく"},
		{"対象名", data.Rule{Use: "fire", Target: data.Target{Preset: "bad"}}, "unknown target"},
		{"対象の側", data.Rule{Use: "fire", Target: data.Target{Side: "bad"}}, "want foes or allies"},
		{"対象の選び方", data.Rule{Use: "fire", Target: data.Target{Pick: "bad"}}, "want min, max"},
		{"比較式なし", data.Rule{Use: "fire", Target: data.Target{Pick: "min"}}, "needs by"},
		{"式の名前", data.Rule{Use: "fire", When: data.Expr{Expr: testgame.Expr("not_defined > 0")}}, "unknown name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testgame.Load(t, nil)
			rules := []data.Rule{tc.rule}
			db.AI["test"] = rules
			db.Order["ai"] = []string{"test"}
			isolated := fmt.Sprint(db.CheckRules(rules))
			whole := strings.ReplaceAll(fmt.Sprint(db.Check()), "ai.test", "rules")
			if isolated != whole {
				t.Fatalf("共通検査と DB 検査が違う\n%s\n%s", isolated, whole)
			}
			if tc.want == "" && isolated != "[]" || tc.want != "" && !strings.Contains(isolated, tc.want) {
				t.Fatalf("診断 %s、期待 %q", isolated, tc.want)
			}
		})
	}
	t.Run("使用不可の品の元位置", func(t *testing.T) {
		_, err := testgame.TryLoad(map[string]string{"data/bad.yaml": "ai:\n  unusable:\n    - item: stick\n"})
		if err == nil || !strings.Contains(err.Error(), "data/bad.yaml:3:7: ai.unusable[0].item:") {
			t.Fatalf("位置付きの診断がない %v", err)
		}
	})
}

func TestAITargetCompatibilityWarnings(t *testing.T) {
	for _, scope := range []string{"one_enemy", "one_ally", "dead_ally", "all_enemies", "all_allies", "all_dead_allies", "everyone", "none", "random_enemy"} {
		t.Run(scope, func(t *testing.T) {
			db := testgame.Load(t, nil)
			db.Skills["fire"].Scope = scope
			rules := []data.Rule{{Use: "fire", Target: data.Target{Where: data.Expr{Expr: testgame.Expr("false")}, Pick: "min", By: data.Expr{Expr: testgame.Expr("rand()")}}}}
			before := rules[0]
			if errs := db.CheckRules(rules); len(errs) != 0 {
				t.Fatalf("互換性の警告で読み込みを拒否した %v", errs)
			}
			warnings := db.RuleWarnings(rules)
			want := scope != "one_enemy" && scope != "one_ally" && scope != "dead_ally"
			if (len(warnings) > 0) != want {
				t.Fatalf("scope %s の警告 %v", scope, warnings)
			}
			if !reflect.DeepEqual(before, rules[0]) {
				t.Fatal("警告のために規則を書き換えた")
			}
			if got := db.RuleWarnings([]data.Rule{{Use: "fire"}}); len(got) != 0 {
				t.Fatalf("対象省略への不要な警告 %v", got)
			}
		})
	}
	t.Run("位置と安定した順序", func(t *testing.T) {
		db := testgame.Load(t, map[string]string{"data/warnings.yaml": "ai:\n  ignored:\n    - use: big\n      target: {where: 'false'}\n"})
		first := fmt.Sprint(db.AIWarnings())
		if !strings.Contains(first, "data/warnings.yaml:4:7: ai.ignored[0].target:") {
			t.Fatalf("警告の位置がない %s", first)
		}
		for range 5 {
			if got := fmt.Sprint(db.AIWarnings()); got != first {
				t.Fatalf("警告の順序が変わった %s", got)
			}
		}
	})
	t.Run("直接書いた規則も診断", func(t *testing.T) {
		db := testgame.Load(t, nil)
		rules := []data.Rule{{Use: "big", Target: data.Target{Preset: "first"}}}
		db.Classes["ranger"].AI = data.AIRef{Rules: rules}
		db.Actors["scout"].AI = data.AIRef{Rules: rules}
		db.Enemies["mote"].AI = data.AIRef{Rules: rules}
		warnings := fmt.Sprint(db.AIWarnings())
		for _, path := range []string{"classes.ranger.ai[0].target", "actors.scout.ai[0].target", "enemies.mote.ai[0].target"} {
			if !strings.Contains(warnings, path) {
				t.Fatalf("直接書いた規則 %s が診断されない %s", path, warnings)
			}
		}
	})
}
