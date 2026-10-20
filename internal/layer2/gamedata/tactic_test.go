package gamedata_test

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/nao1215/gozen/internal/expr"
	data "github.com/nao1215/gozen/internal/layer2/gamedata"
	"github.com/nao1215/gozen/internal/rpg"
	"github.com/nao1215/gozen/internal/testgame"
)

func tacticDB(t *testing.T) *data.DB {
	t.Helper()
	return testgame.Load(t, map[string]string{"data/tactics.yaml": testgame.Tactics})
}

func tacticRow(condition, target, action string) data.TacticRow {
	return data.TacticRow{Enabled: true, Condition: condition, Target: target, Action: action}
}

func TestTacticCatalogValidation(t *testing.T) {
	t.Run("旧新版で定義順を保持", func(t *testing.T) {
		for _, version := range []string{"", "schema_version: 1\nid: neutral\n"} {
			db := testgame.Load(t, map[string]string{"game.yaml": version + testgame.Game, "data/tactics.yaml": testgame.Tactics})
			if errs := db.CheckTacticCatalog(); len(errs) != 0 {
				t.Fatal(errs)
			}
			if got := db.Ordered("tactic_conditions"); len(got) != 9 || got[0] != "any" || got[8] != "guard_complex" {
				t.Fatalf("定義順が違う %v", got)
			}
		}
	})
	for _, tc := range []struct{ name, src, path string }{
		{"空条件", "tactic_conditions:\n  bad: null", "tactic_conditions.bad"},
		{"空対象", "tactic_targets:\n  bad: null", "tactic_targets.bad"},
		{"空行動", "tactic_actions:\n  bad: null", "tactic_actions.bad"},
		{"形", "tactic_targets:\n  bad: []", "tactic_targets.bad"},
		{"入れ子の空", "tactic_targets:\n  bad: {name: x, scope: one_enemy, selector: null}", "tactic_targets.bad.selector"},
		{"未知項目", "tactic_conditions:\n  bad: {name: x, kind: always, typo: true}", "tactic_conditions.bad.typo"},
		{"表示名", "tactic_conditions:\n  bad: {kind: always}", "tactic_conditions.bad.name"},
		{"表示名をID", "tactic_conditions:\n  表示名: {name: x, kind: always}", "tactic_conditions.表示名"},
		{"未知種類", "tactic_conditions:\n  bad: {name: x, kind: typo}", "tactic_conditions.bad.kind"},
		{"未知主体", "tactic_conditions:\n  bad: {name: x, kind: alive, subject: party}", "tactic_conditions.bad.subject"},
		{"未知状態", "tactic_conditions:\n  bad: {name: x, kind: state, subject: allies, state: missing}", "tactic_conditions.bad.state"},
		{"状態欠落", "tactic_conditions:\n  bad: {name: x, kind: state, subject: allies}", "tactic_conditions.bad.state"},
		{"余分な状態", "tactic_conditions:\n  bad: {name: x, kind: alive, subject: allies, state: poison}", "tactic_conditions.bad.state"},
		{"余分な主体", "tactic_conditions:\n  bad: {name: x, kind: always, subject: self}", "tactic_conditions.bad"},
		{"式欠落", "tactic_conditions:\n  bad: {name: x, kind: template}", "tactic_conditions.bad.when"},
		{"編集不可部分の配置", "tactic_conditions:\n  bad: {name: x, kind: always, chance: 0.5}", "tactic_conditions.bad"},
		{"式の構文", "tactic_conditions:\n  bad: {name: x, kind: template, when: 'self.hp <'}", "tactic_conditions.bad.when"},
		{"式の参照", "tactic_conditions:\n  bad: {name: x, kind: template, when: 'self.typo > 1'}", "tactic_conditions.bad.when"},
		{"抽選の参照", "tactic_conditions:\n  bad: {name: x, kind: template, when: true, chance: missing}", "tactic_conditions.bad.chance"},
		{"重みのnull", "tactic_conditions:\n  bad: {name: x, kind: template, when: true, weight: null}", "tactic_conditions.bad.weight"},
		{"重みの文字列", "tactic_conditions:\n  bad: {name: x, kind: template, when: true, weight: '1'}", "tactic_conditions.bad.weight"},
		{"優先度の小数", "tactic_conditions:\n  bad: {name: x, kind: template, when: true, priority: 1.5}", "tactic_conditions.bad.priority"},
		{"一度指定のnull", "tactic_conditions:\n  bad: {name: x, kind: template, when: true, once: null}", "tactic_conditions.bad.once"},
		{"重み", "tactic_conditions:\n  bad: {name: x, kind: template, when: true, weight: .nan}", "tactic_conditions.bad.weight"},
		{"未知範囲", "tactic_targets:\n  bad: {name: x, scope: typo}", "tactic_targets.bad.scope"},
		{"全体の単体絞込", "tactic_targets:\n  bad: {name: x, scope: all_allies, selector: {where: t.hp_rate < 0.5}}", "tactic_targets.bad.selector"},
		{"抽選の単体選択", "tactic_targets:\n  bad: {name: x, scope: random_enemy, selector: first}", "tactic_targets.bad.selector"},
		{"自己の単体選択", "tactic_targets:\n  bad: {name: x, scope: self, selector: first}", "tactic_targets.bad.selector"},
		{"未知の略記", "tactic_targets:\n  bad: {name: x, scope: one_enemy, selector: typo}", "tactic_targets.bad.selector"},
		{"比較式欠落", "tactic_targets:\n  bad: {name: x, scope: one_enemy, selector: {pick: min}}", "tactic_targets.bad.selector"},
		{"無視される比較式", "tactic_targets:\n  bad: {name: x, scope: one_enemy, selector: {by: t.hp}}", "tactic_targets.bad.selector.by"},
		{"敵を味方へ", "tactic_targets:\n  bad: {name: x, scope: one_enemy, selector: {side: allies}}", "tactic_targets.bad.selector"},
		{"味方を敵へ", "tactic_targets:\n  bad: {name: x, scope: one_ally, selector: {side: foes}}", "tactic_targets.bad.selector"},
		{"蘇生を自己へ", "tactic_targets:\n  bad: {name: x, scope: dead_ally, selector: self}", "tactic_targets.bad.selector"},
		{"対象の式参照", "tactic_targets:\n  bad: {name: x, scope: one_enemy, selector: {where: t.typo}}", "tactic_targets.bad.selector.where"},
		{"行動なし", "tactic_actions:\n  bad: {name: x}", "tactic_actions.bad"},
		{"行動二つ", "tactic_actions:\n  bad: {name: x, use: attack, item: potion}", "tactic_actions.bad"},
		{"未知技", "tactic_actions:\n  bad: {name: x, use: missing}", "tactic_actions.bad.use"},
		{"未知品", "tactic_actions:\n  bad: {name: x, item: missing}", "tactic_actions.bad.item"},
		{"装備品", "tactic_actions:\n  bad: {name: x, item: stick}", "tactic_actions.bad.item"},
		{"メニュー専用", "tactic_actions:\n  bad: {name: x, use: menu_heal}", "tactic_actions.bad"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, version := range []string{"", "schema_version: 1\nid: neutral\n"} {
				_, err := testgame.TryLoad(map[string]string{"game.yaml": version + testgame.Game, "data/tactics.yaml": testgame.Tactics, "data/bad.yaml": tc.src})
				if err == nil || !strings.Contains(err.Error(), "data/bad.yaml:2:") || !strings.Contains(err.Error(), tc.path) {
					t.Fatalf("位置付きの診断がない %v", err)
				}
			}
		})
	}
	t.Run("別ファイル重複", func(t *testing.T) {
		_, err := testgame.TryLoad(map[string]string{"data/tactics.yaml": testgame.Tactics, "data/z.yaml": "tactic_conditions:\n  any: {name: x, kind: always}"})
		if err == nil || !strings.Contains(err.Error(), "data/z.yaml:2:") || !strings.Contains(err.Error(), "data/tactics.yaml:") {
			t.Fatalf("両方の位置がない %v", err)
		}
	})
	t.Run("直接構築の空値と非有限数", func(t *testing.T) {
		db := tacticDB(t)
		db.TacticConditions["nil"] = nil
		db.TacticTargets["nil"] = nil
		db.TacticActions["nil"] = nil
		db.TacticConditions["complex"].Weight = math.Inf(1)
		if errs := db.CheckTacticCatalog(); len(errs) != 4 {
			t.Fatalf("全マップを検査していない %v", errs)
		}
		if errs := db.Check(); len(errs) != 4 {
			t.Fatalf("DB 全体も同じ診断でない %v", errs)
		}
	})
}

func TestTacticCompile(t *testing.T) {
	db := tacticDB(t)
	known := []string{"fire", "heal", "group_heal", "raise", "group_raise", "big", "random_hit", "all_hit", "retreat"}
	for _, tc := range []struct{ target, action string }{
		{"enemy", "attack"},
		{"ally", "heal"},
		{"self", "heal"},
		{"self", "guard"},
		{"ally", "potion"},
		{"allies", "group_heal"},
		{"enemies", "big"},
		{"fallen", "raise"},
		{"fallen_all", "group_raise"},
		{"random", "random_hit"},
		{"everyone", "all_hit"},
		{"none", "retreat"},
	} {
		t.Run(tc.target+"/"+tc.action, func(t *testing.T) {
			rule, errs := db.CompileTactic(tacticRow("any", tc.target, tc.action), known)
			if len(errs) != 0 || rule == nil {
				t.Fatalf("適合する行を拒否 %v", errs)
			}
			if errs := db.CheckRules([]data.Rule{*rule}); len(errs) != 0 {
				t.Fatal(errs)
			}
		})
	}
	t.Run("保存はIDと数値のみ", func(t *testing.T) {
		for _, percent := range []int{0, 100} {
			row := tacticRow("target_hp", "ally", "heal")
			row.Percent = new(percent)
			before, errs := db.CompileTactic(row, known)
			if len(errs) != 0 {
				t.Fatal(errs)
			}
			for _, yamlFormat := range []bool{false, true} {
				var encoded []byte
				var err error
				var decoded data.TacticRow
				if yamlFormat {
					encoded, err = yaml.Marshal(row)
					if err == nil {
						err = yaml.UnmarshalWithOptions(encoded, &decoded, yaml.Strict())
					}
				} else {
					encoded, err = json.Marshal(row)
					if err == nil {
						err = json.Unmarshal(encoded, &decoded)
					}
				}
				if err != nil || !reflect.DeepEqual(row, decoded) {
					t.Fatalf("往復失敗 %s %v", encoded, err)
				}
				after, errs := db.CompileTactic(decoded, known)
				if len(errs) != 0 || !reflect.DeepEqual(before, after) {
					t.Fatalf("変換結果が変わった %v", errs)
				}
			}
		}
	})
	for _, tc := range []struct {
		name   string
		change func(*data.TacticRow)
		want   string
	}{
		{"負数", func(r *data.TacticRow) { r.Percent = new(-1) }, "percent"},
		{"上限超過", func(r *data.TacticRow) { r.Percent = new(101) }, "percent"},
		{"省略", func(r *data.TacticRow) { r.Percent = nil }, "percent"},
		{"不要な閾値", func(r *data.TacticRow) { r.Condition = "any" }, "percent"},
		{"未知条件", func(r *data.TacticRow) { r.Condition = "missing" }, "condition"},
		{"未知対象", func(r *data.TacticRow) { r.Target = "missing" }, "target"},
		{"敵専用技の直指定", func(r *data.TacticRow) { r.Action = "slap" }, "action"},
		{"回復を敵へ", func(r *data.TacticRow) { r.Target = "enemy" }, "target"},
		{"全体の対象条件", func(r *data.TacticRow) { r.Target, r.Action = "allies", "group_heal" }, "condition"},
		{"単体に全体技", func(r *data.TacticRow) { r.Action = "group_heal" }, "target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := tacticRow("target_hp", "ally", "heal")
			row.Percent = new(50)
			tc.change(&row)
			before, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			rule, errs := db.CompileTactic(row, known)
			after, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			if rule != nil || !strings.Contains(fmt.Sprint(errs), "tactic."+tc.want) || string(before) != string(after) {
				t.Fatalf("不正行の扱い %v %v", rule, errs)
			}
		})
	}
	t.Run("未習得と敵専用のカタログ登録", func(t *testing.T) {
		db := tacticDB(t)
		db.TacticActions["enemy_skill"] = &data.TacticAction{Name: "敵の技", Use: "slap"}
		for _, action := range []string{"heal", "enemy_skill"} {
			target := "ally"
			if action == "enemy_skill" {
				target = "enemy"
			}
			if rule, errs := db.CompileTactic(tacticRow("any", target, action), []string{"fire"}); rule != nil || !strings.Contains(fmt.Sprint(errs), "習得技") {
				t.Fatalf("習得できない技を通した %v", errs)
			}
		}
	})
	t.Run("無効行も値を検査", func(t *testing.T) {
		row := tacticRow("any", "enemy", "attack")
		row.Enabled = false
		if rule, errs := db.CompileTactic(row, nil); rule != nil || len(errs) != 0 {
			t.Fatalf("無効行 %v %v", rule, errs)
		}
		row.Action = "missing"
		if rule, errs := db.CompileTactic(row, nil); rule != nil || len(errs) == 0 {
			t.Fatal("無効行の未知 ID を失った")
		}
	})
	t.Run("複合条件を読み戻す", func(t *testing.T) {
		for _, tc := range []struct{ condition, target, action, original string }{
			{"complex", "complex", "fire", "original_complex"}, {"group_complex", "allies", "group_heal", "original_group"}, {"guard_complex", "self", "guard", "original_guard"},
		} {
			row := tacticRow(tc.condition, tc.target, tc.action)
			b, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			var decoded data.TacticRow
			if err := json.Unmarshal(b, &decoded); err != nil {
				t.Fatal(err)
			}
			fresh := tacticDB(t)
			fresh.TacticConditions[tc.condition].Name = "表示名を変更"
			rule, errs := fresh.CompileTactic(decoded, known)
			if len(errs) != 0 || !reflect.DeepEqual(rule, &fresh.AI[tc.original][0]) {
				t.Fatalf("編集不可部分が変わった %v\n%+v\n%+v", errs, rule, fresh.AI[tc.original][0])
			}
			if rule.When.Expr == fresh.TacticConditions[tc.condition].When.Expr {
				t.Fatal("可変の式ポインタを共有した")
			}
		}
	})
	t.Run("対象の固定条件を保持", func(t *testing.T) {
		rule, errs := db.CompileTactic(tacticRow("poisoned", "complex", "fire"), known)
		if len(errs) != 0 {
			t.Fatal(errs)
		}
		for _, poison := range []int{0, 1} {
			for _, sleep := range []int{0, 1} {
				for _, resist := range []float64{0.5, 2} {
					ok, err := rule.Target.Where.Bool(expr.Map{"t": expr.Map{"resist_fire": resist, "states": expr.Map{"poison": poison, "sleep": sleep}}})
					if err != nil || ok != (poison > 0 && sleep == 0 && resist > 1.2) {
						t.Fatalf("固定部分が消えた %v %v", ok, err)
					}
				}
			}
		}
	})
	t.Run("閾値と空集団", func(t *testing.T) {
		for _, kind := range []string{"hp_below", "hp_at_most", "mp_below", "mp_at_most"} {
			for _, subject := range []string{"self", "allies", "foes", "target"} {
				for _, percent := range []int{0, 50, 100} {
					db.TacticConditions["probe"] = &data.TacticCondition{Name: "検査", Kind: kind, Subject: subject}
					row := tacticRow("probe", "ally", "heal")
					row.Percent = new(percent)
					rule, errs := db.CompileTactic(row, known)
					if len(errs) != 0 {
						t.Fatal(errs)
					}
					for _, rate := range []float64{0, 0.49, 0.5, 0.51, 1} {
						unit := expr.Map{"hp_rate": rate, "mp_rate": rate, "alive": true}
						group := rpg.NewGroup([]expr.Object{unit})
						env := expr.Map{"self": unit, "t": unit, "allies": group, "foes": group}
						e := rule.When
						if subject == "target" {
							e = rule.Target.Where
						}
						ok, err := e.Bool(env)
						want := rate < float64(percent)/100
						if strings.HasSuffix(kind, "at_most") {
							want = rate <= float64(percent)/100
						}
						if err != nil || ok != want {
							t.Fatalf("%s/%s/%d/%g %v %v", kind, subject, percent, rate, ok, err)
						}
						if subject == "allies" || subject == "foes" {
							env[subject] = rpg.NewGroup(nil)
							if ok, err := e.Bool(env); err != nil || ok {
								t.Fatalf("空集団で成立 %v %v", ok, err)
							}
						}
					}
				}
			}
		}
	})
}

func TestTacticCompileStatesAndLife(t *testing.T) {
	db := tacticDB(t)
	db.States["test-state"] = &data.State{Name: "検査状態"}
	for _, kind := range []string{"state", "not_state", "alive", "dead"} {
		for _, subject := range []string{"self", "allies", "foes", "target"} {
			t.Run(kind+"/"+subject, func(t *testing.T) {
				d := &data.TacticCondition{Name: "検査条件", Kind: kind, Subject: subject}
				if kind == "state" || kind == "not_state" {
					d.State = "test-state"
				}
				db.TacticConditions["probe"] = d
				row := tacticRow("probe", "ally", "heal")
				if kind == "dead" && subject == "target" {
					db.TacticTargets["fallen_detail"] = &data.TacticTarget{Name: "検査対象", Scope: "dead_ally"}
					row.Target, row.Action = "fallen_detail", "raise"
				}
				rule, errs := db.CompileTactic(row, []string{"heal", "raise"})
				if len(errs) != 0 {
					t.Fatal(errs)
				}
				for _, alive := range []bool{false, true} {
					for _, state := range []int{0, 2} {
						u := expr.Map{"alive": alive, "dead": !alive, "states": expr.Map{"test-state": state}}
						g := rpg.NewGroup([]expr.Object{u})
						env := expr.Map{"self": u, "t": u, "allies": g, "foes": g}
						e := rule.When
						if subject == "target" {
							e = rule.Target.Where
						}
						want := false
						switch kind {
						case "state":
							want = state > 0
						case "not_state":
							want = state == 0
						case "alive":
							want = alive
						case "dead":
							want = !alive
						}
						if (subject == "allies" || subject == "foes") && (kind == "state" || kind == "not_state") {
							want = alive && want
						}
						if got, err := e.Bool(env); err != nil || got != want {
							t.Fatalf("生存 %v 状態 %d で %v、期待 %v。%v", alive, state, got, want, err)
						}
					}
				}
			})
		}
	}
	for _, tc := range []struct{ kind, target, action string }{{"alive", "fallen", "raise"}, {"dead", "ally", "heal"}} {
		db.TacticConditions["probe"] = &data.TacticCondition{Name: "検査条件", Kind: tc.kind, Subject: "target"}
		if _, errs := db.CompileTactic(tacticRow("probe", tc.target, tc.action), []string{"heal", "raise"}); len(errs) == 0 {
			t.Fatal("生死が矛盾する行を通した")
		}
	}
}

func TestTacticCompileSchemaAndIsolation(t *testing.T) {
	for _, version := range []string{"", "schema_version: 1\nid: neutral\n"} {
		db := testgame.Load(t, map[string]string{"game.yaml": version + testgame.Game, "data/tactics.yaml": testgame.Tactics})
		for _, tc := range []struct{ condition, target, action, original string }{
			{"complex", "complex", "fire", "original_complex"}, {"group_complex", "allies", "group_heal", "original_group"}, {"guard_complex", "self", "guard", "original_guard"},
		} {
			rule, errs := db.CompileTactic(tacticRow(tc.condition, tc.target, tc.action), []string{"fire", "group_heal"})
			if len(errs) != 0 || !reflect.DeepEqual(rule, &db.AI[tc.original][0]) {
				t.Fatalf("版 %q の複合条件 %v", version, errs)
			}
			if errs := db.CheckRules([]data.Rule{*rule}); len(errs) != 0 {
				t.Fatal(errs)
			}
		}
		rule, errs := db.CompileTactic(tacticRow("any", "ally", "heal"), []string{"heal"})
		if len(errs) != 0 {
			t.Fatal(errs)
		}
		if errs := db.CheckRules([]data.Rule{*rule}); len(errs) != 0 {
			t.Fatal(errs)
		}
		rule, errs = db.CompileTactic(tacticRow("complex", "complex", "fire"), []string{"fire"})
		if len(errs) != 0 {
			t.Fatal(errs)
		}
		for _, e := range []data.Expr{rule.When, rule.Chance, rule.Target.Where, rule.Target.By} {
			if err := e.UnmarshalText([]byte("false")); err != nil {
				t.Fatal(err)
			}
		}
		rule, errs = db.CompileTactic(tacticRow("complex", "complex", "fire"), []string{"fire"})
		if len(errs) != 0 || !reflect.DeepEqual(rule, &db.AI["original_complex"][0]) {
			t.Fatal("戻り値の変更がカタログに波及した")
		}
	}
}

func TestSimpleTacticCatalog(t *testing.T) {
	setup := func() *data.DB {
		db := tacticDB(t)
		db.Game.Battle.SimpleTactics = true
		db.TacticConditions = map[string]*data.TacticCondition{"any": {Name: "いつでも", Kind: "always"}}
		db.TacticActions = map[string]*data.TacticAction{"attack": {Name: "攻撃", Use: "attack", Target: "enemy"}}
		db.AI["simple"] = []data.Rule{{Use: "attack"}}
		db.Game.Battle.AutoTactics = map[string][]data.TacticRow{"simple": {{Enabled: true, Condition: "any", Action: "attack"}}}
		return db
	}
	for _, tc := range []struct {
		name   string
		change func(*data.DB)
		bad    bool
	}{
		{"対象は行動が決める", func(*data.DB) {}, false},
		{"６行", func(db *data.DB) {
			for range 5 {
				db.Game.Battle.AutoTactics["simple"] = append(db.Game.Battle.AutoTactics["simple"], db.Game.Battle.AutoTactics["simple"][0])
			}
		}, true},
		{"９条件", func(db *data.DB) {
			for i := range 8 {
				db.TacticConditions[strconv.Itoa(i)] = &data.TacticCondition{Name: "条件", Kind: "always"}
			}
		}, true},
		{"行に対象", func(db *data.DB) { db.Game.Battle.AutoTactics["simple"][0].Target = "enemy" }, true},
		{"行に閾値", func(db *data.DB) { db.Game.Battle.AutoTactics["simple"][0].Percent = new(50) }, true},
		{"型に職業がない", func(db *data.DB) { db.Game.Battle.TacticPresets = map[string]data.TacticPreset{"other": {Name: "型"}} }, true},
		{"行動に対象がない", func(db *data.DB) { db.TacticActions["attack"].Target = "" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setup()
			tc.change(db)
			errs := db.CheckTacticCatalog()
			if (len(errs) > 0) != tc.bad {
				t.Fatalf("定義検査 %v", errs)
			}
		})
	}
	db := setup()
	a := testgame.State(db, 1).Actors["scout"]
	plan := rpg.ActorTactics{Version: rpg.TacticsVersion, Mode: rpg.TacticsManual}
	for range 5 {
		plan.Rows = append(plan.Rows, db.Game.Battle.AutoTactics["simple"][0])
	}
	if err := a.SetTactics(plan); err != nil {
		t.Fatal(err)
	}
	plan.Rows = append(plan.Rows, plan.Rows[0])
	if err := a.SetTactics(plan); err == nil || len(a.Tactics.Rows) != 5 {
		t.Fatal("６行で確定済み作戦を上書きした")
	}
}
